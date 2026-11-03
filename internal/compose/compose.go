// Package compose runs the docker compose CLI on a workspace.
package compose

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Exec runs a command. Tests replace it with a fake.
type Exec func(ctx context.Context, cmd Command) error

// Command is one docker compose call.
type Command struct {
	Args   []string
	Env    []string // added to the environment of the CLI
	Stdout io.Writer
	Stderr io.Writer
}

// Runner runs docker compose with the files of one workspace.
type Runner struct {
	Dir  string
	Exec Exec
	// Env is added to every call, for example the config hashes used in labels.
	Env []string
}

// SystemExec runs the real docker CLI.
func SystemExec(ctx context.Context, c Command) error {
	cmd := exec.CommandContext(ctx, "docker", c.Args...)
	cmd.Env = append(os.Environ(), c.Env...)
	cmd.Stdout, cmd.Stderr = c.Stdout, c.Stderr
	return cmd.Run()
}

// baseArgs selects the workspace files. .env.local is passed only when it exists,
// so that a workspace with no secrets still works.
func (r Runner) baseArgs() []string {
	args := []string{"compose", "--project-directory", r.Dir, "-f", filepath.Join(r.Dir, "compose.yaml"),
		"--env-file", filepath.Join(r.Dir, ".env")}
	if _, err := os.Stat(filepath.Join(r.Dir, ".env.local")); err == nil {
		args = append(args, "--env-file", filepath.Join(r.Dir, ".env.local"))
	}
	return args
}

// Run runs 'docker compose <args>' and writes its output to stdout and stderr.
func (r Runner) Run(ctx context.Context, stdout, stderr io.Writer, args ...string) error {
	return r.Exec(ctx, Command{Args: append(r.baseArgs(), args...), Env: r.Env, Stdout: stdout, Stderr: stderr})
}

// Output runs 'docker compose <args>' and returns its stdout. stderr is in the error.
func (r Runner) Output(ctx context.Context, args ...string) ([]byte, error) {
	var out, errb bytes.Buffer
	if err := r.Run(ctx, &out, &errb, args...); err != nil {
		if msg := strings.TrimSpace(errb.String()); msg != "" {
			return nil, fmt.Errorf("docker compose %s: %s", args[0], msg)
		}
		return nil, fmt.Errorf("docker compose %s: %w", args[0], err)
	}
	return out.Bytes(), nil
}

// Publisher is a published port of a container.
type Publisher struct {
	URL           string `json:"URL"`
	TargetPort    int    `json:"TargetPort"`
	PublishedPort int    `json:"PublishedPort"`
	Protocol      string `json:"Protocol"`
}

// Container is one line of 'docker compose ps --format json'.
type Container struct {
	Name       string      `json:"Name"`
	Service    string      `json:"Service"`
	State      string      `json:"State"`
	Health     string      `json:"Health"`
	ExitCode   int         `json:"ExitCode"`
	Publishers []Publisher `json:"Publishers"`
	Labels     string      `json:"Labels"`
}

// OneOff reports whether the container was started by 'docker compose run'.
func (c Container) OneOff() bool {
	for _, l := range strings.Split(c.Labels, ",") {
		if l == "com.docker.compose.oneoff=True" {
			return true
		}
	}
	return false
}

// Ps returns every container of the project, also the stopped ones.
func (r Runner) Ps(ctx context.Context) ([]Container, error) {
	out, err := r.Output(ctx, "ps", "--all", "--format", "json")
	if err != nil {
		return nil, err
	}
	all, err := ParsePs(out)
	if err != nil {
		return nil, err
	}
	var cs []Container
	for _, c := range all {
		if !c.OneOff() {
			cs = append(cs, c)
		}
	}
	return cs, nil
}

// ParsePs reads 'docker compose ps --format json'. Compose writes one JSON
// object per line; versions before 2.21 wrote one JSON array.
func ParsePs(data []byte) ([]Container, error) {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return nil, nil
	}
	if data[0] == '[' {
		var cs []Container
		if err := json.Unmarshal(data, &cs); err != nil {
			return nil, fmt.Errorf("parse docker compose ps: %w", err)
		}
		return cs, nil
	}
	var cs []Container
	s := bufio.NewScanner(bytes.NewReader(data))
	s.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for s.Scan() {
		line := bytes.TrimSpace(s.Bytes())
		if len(line) == 0 {
			continue
		}
		var c Container
		if err := json.Unmarshal(line, &c); err != nil {
			return nil, fmt.Errorf("parse docker compose ps: %w", err)
		}
		cs = append(cs, c)
	}
	return cs, s.Err()
}
