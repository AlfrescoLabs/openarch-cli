package stack

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AlfrescoLabs/openarch-cli/internal/compose"
	"github.com/AlfrescoLabs/openarch-cli/internal/doctor"
	"github.com/AlfrescoLabs/openarch-cli/internal/localenv"
	"github.com/AlfrescoLabs/openarch-cli/internal/output"
	"github.com/AlfrescoLabs/openarch-cli/internal/workspace"
)

func ctr(service, state, health string, exit int, ports ...int) compose.Container {
	c := compose.Container{Name: "ws-" + service + "-1", Service: service, State: state, Health: health, ExitCode: exit}
	for _, p := range ports {
		c.Publishers = append(c.Publishers, compose.Publisher{PublishedPort: p, TargetPort: p})
	}
	return c
}

func healthyContainers() []compose.Container {
	return []compose.Container{
		ctr("mongodb", "running", "", 0),
		ctr("mongodb-init", "exited", "", 0),
		ctr("opensearch", "running", "healthy", 0),
		ctr("openarch-app", "running", "healthy", 0, 8080),
	}
}

func TestEvaluate(t *testing.T) {
	cfg := workspace.Default("ws")
	ok := &APICheck{OK: true}
	replace := func(service string, c compose.Container) []compose.Container {
		cs := healthyContainers()
		for i := range cs {
			if cs[i].Service == service {
				cs[i] = c
			}
		}
		return cs
	}
	tests := []struct {
		name string
		cs   []compose.Container
		api  *APICheck
		want string
	}{
		{"healthy", healthyContainers(), ok, Healthy},
		{"api down", healthyContainers(), &APICheck{OK: false, Error: "refused"}, Unhealthy},
		{"health starting", replace("opensearch", ctr("opensearch", "running", "starting", 0)), nil, Starting},
		{"waiting for dependencies", replace("openarch-app", ctr("openarch-app", "created", "", 0)), nil, Starting},
		{"init running", replace("mongodb-init", ctr("mongodb-init", "running", "", 0)), nil, Starting},
		{"init failed", replace("mongodb-init", ctr("mongodb-init", "exited", "", 1)), nil, Unhealthy},
		{"unhealthy", replace("openarch-app", ctr("openarch-app", "running", "unhealthy", 0)), nil, Unhealthy},
		{"app exited", replace("openarch-app", ctr("openarch-app", "exited", "", 137)), nil, Unhealthy},
		{"missing service", healthyContainers()[:3], nil, Unhealthy},
		{"no containers", nil, nil, Stopped},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Evaluate(cfg, tt.cs, tt.api).State; got != tt.want {
				t.Errorf("state = %s, want %s", got, tt.want)
			}
		})
	}
	st := Evaluate(cfg, healthyContainers(), ok)
	if app := st.Services[3]; app.Name != "openarch-app" || !slices.Equal(app.Ports, []int{8080}) {
		t.Errorf("openarch-app = %+v", app)
	}
}

func TestParseLogs(t *testing.T) {
	lines := ParseLogs([]byte("openarch-app-1  | started\nmongodb-init-1  | a | b\nnoise\nopensearch-1  |\n"))
	want := []LogLine{
		{Service: "openarch-app", Container: "openarch-app-1", Text: "started"},
		{Service: "mongodb-init", Container: "mongodb-init-1", Text: "a | b"},
		{Service: "opensearch", Container: "opensearch-1", Text: ""},
	}
	if !slices.Equal(lines, want) {
		t.Errorf("ParseLogs = %+v", lines)
	}
}

// fakeCompose answers docker compose calls and records them.
type fakeCompose struct {
	ps    []compose.Container
	upErr error
	// upBlocks makes up wait until its context is done, like a stack that never becomes healthy.
	upBlocks bool
	// inspect is the output of docker inspect.
	inspect string
	mu      sync.Mutex
	calls   [][]string
	envs    [][]string
}

func (f *fakeCompose) exec(ctx context.Context, c compose.Command) error {
	f.mu.Lock()
	f.calls = append(f.calls, c.Args)
	f.envs = append(f.envs, c.Env)
	f.mu.Unlock()
	if c.Args[0] == "inspect" {
		_, err := io.WriteString(c.Stdout, f.inspect)
		return err
	}
	// The workspace paths are absolute, so the first plain sub-command word is the command.
	sub := ""
	for _, a := range c.Args {
		if slices.Contains([]string{"ps", "up", "down", "logs"}, a) {
			sub = a
			break
		}
	}
	switch sub {
	case "ps":
		enc := json.NewEncoder(c.Stdout)
		for _, ct := range f.ps {
			if err := enc.Encode(ct); err != nil {
				return err
			}
		}
	case "up":
		if f.upBlocks {
			<-ctx.Done()
			return ctx.Err()
		}
		return f.upErr
	case "logs":
		_, err := io.WriteString(c.Stdout, "openarch-app-1  | hello\n")
		return err
	}
	return nil
}

func fakeDoctor(portFree bool) func() doctor.Env {
	return func() doctor.Env {
		return doctor.Env{
			GOOS:     "darwin",
			LookPath: func(string) (string, error) { return "/usr/bin/docker", nil },
			Run: func(_ context.Context, _ string, args ...string) ([]byte, []byte, error) {
				if args[0] == "info" {
					return []byte(`{"ServerVersion":"29.0.0","OperatingSystem":"Docker Desktop","NCPU":8,"MemTotal":8589934592}`), nil, nil
				}
				return []byte("v2.29.1"), nil, nil
			},
			PortFree:   func(context.Context, int) bool { return portFree },
			HTTPStatus: func(context.Context, string) (int, error) { return 401, nil },
			ReadFile:   func(string) ([]byte, error) { return nil, errors.New("unused") },
		}
	}
}

func newWorkspace(t *testing.T, cfg workspace.Config) Workspace {
	t.Helper()
	dir := t.TempDir()
	var s localenv.Env
	s.Fill("admin")
	for name, data := range map[string][]byte{
		workspace.ConfigFile: cfg.Marshal(),
		localenv.File:        s.Marshal(),
		"compose.yaml":       []byte("services: {}\n"),
		".env":               nil,
		"config/users.yml":   []byte("users: []\n"),
	} {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	w, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func env(f *fakeCompose, portFree bool) Env {
	return Env{
		Compose:  f.exec,
		Doctor:   fakeDoctor(portFree),
		CheckAPI: func(context.Context, string, workspace.Config) APICheck { return APICheck{OK: true} },
	}
}

func TestUpHealthy(t *testing.T) {
	w := newWorkspace(t, workspace.Default("ws"))
	f := &fakeCompose{ps: healthyContainers()}
	st, err := Up(context.Background(), env(f, true), w, UpOptions{Wait: true, Timeout: 60e9, Progress: &strings.Builder{}})
	if err != nil || st.State != Healthy {
		t.Fatalf("Up = %s, %v", st.State, err)
	}
	var up []string
	for _, c := range f.calls {
		if slices.Contains(c, "up") {
			up = c
		}
	}
	if !slices.Contains(up, "--wait") || !slices.Contains(up, "60") || !slices.Contains(up, filepath.Join(w.Dir, ".env.local")) {
		t.Errorf("up args = %v", up)
	}
	if !slices.ContainsFunc(f.envs[len(f.envs)-1], func(e string) bool { return strings.HasPrefix(e, workspace.HashVarApp+"=") }) {
		t.Errorf("no config hash in env: %v", f.envs[len(f.envs)-1])
	}
}

func TestUpOwnPortIsNotBusy(t *testing.T) {
	w := newWorkspace(t, workspace.Default("ws"))
	f := &fakeCompose{ps: healthyContainers()} // openarch-app publishes 8080
	if _, err := Up(context.Background(), env(f, false), w, UpOptions{Wait: true, Progress: &strings.Builder{}}); err != nil {
		t.Errorf("Up with own port busy: %v", err)
	}
}

func TestUpDoctorFails(t *testing.T) {
	w := newWorkspace(t, workspace.Default("ws"))
	f := &fakeCompose{} // nothing running, so port 8080 is busy because of something else
	_, err := Up(context.Background(), env(f, false), w, UpOptions{Wait: true, Progress: &strings.Builder{}})
	if output.CodeOf(err) != output.ExitPrerequisite || !strings.Contains(err.Error(), "port.8080") {
		t.Errorf("Up = %v", err)
	}
	if slices.ContainsFunc(f.calls, func(c []string) bool { return slices.Contains(c, "up") }) {
		t.Error("up ran after a failed doctor")
	}
}

func TestUpTimeout(t *testing.T) {
	w := newWorkspace(t, workspace.Default("ws"))
	cs := healthyContainers()
	cs[2] = ctr("opensearch", "running", "starting", 0)
	f := &fakeCompose{ps: cs, upErr: errors.New("exit status 1")}
	st, err := Up(context.Background(), env(f, true), w, UpOptions{Wait: true, SkipDoctor: true, Progress: &strings.Builder{}})
	if output.CodeOf(err) != output.ExitUnhealthy || st.State != Starting {
		t.Errorf("Up = %s, %v", st.State, err)
	}
}

func TestUpStopsOnAuthLoop(t *testing.T) {
	w := newWorkspace(t, workspace.Default("ws"))
	cs := healthyContainers()
	cs[2] = ctr("opensearch", "running", "starting", 0)
	var log []string
	for i := range 5 {
		start := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC).Add(time.Duration(i) * 15 * time.Second).Format(time.RFC3339)
		log = append(log, fmt.Sprintf(`{"Start": %q, "ExitCode": 22, "Output": "curl: (22) The requested URL returned error: 401"}`, start))
	}
	f := &fakeCompose{ps: cs, upBlocks: true, inspect: `[{"Config": {"Labels": {"com.docker.compose.service": "opensearch"}},
		"State": {"Health": {"Status": "starting", "Log": [` + strings.Join(log, ",") + `]}}}]`}
	e := env(f, true)
	e.HealthPoll = time.Millisecond
	st, err := Up(context.Background(), e, w, UpOptions{Wait: true, SkipDoctor: true, Timeout: 600e9, Progress: &strings.Builder{}})
	var oerr *output.Error
	if !errors.As(err, &oerr) || oerr.Code != output.ExitUnhealthy || !strings.Contains(oerr.Message, "opensearch healthcheck failed with HTTP 401") ||
		!strings.Contains(oerr.Hint, "Restart Docker Desktop") || st.State != Starting {
		t.Errorf("Up = %s, %v", st.State, err)
	}
}

func TestAuthLoopIn(t *testing.T) {
	at := func(sec, exit int, out string) compose.HealthResult {
		return compose.HealthResult{Start: time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC).Add(time.Duration(sec) * time.Second), ExitCode: exit, Output: out}
	}
	e401 := "curl: (22) The requested URL returned error: 401"
	tests := []struct {
		name string
		log  []compose.HealthResult
		want bool
	}{
		{"one minute of 401", []compose.HealthResult{at(0, 22, e401), at(15, 22, e401), at(30, 22, e401), at(45, 22, e401), at(60, 22, e401)}, true},
		{"only 45 s", []compose.HealthResult{at(0, 22, e401), at(15, 22, e401), at(30, 22, e401), at(45, 22, e401)}, false},
		{"a success in between", []compose.HealthResult{at(0, 22, e401), at(15, 0, "{}"), at(30, 22, e401), at(45, 22, e401), at(60, 22, e401)}, false},
		{"not running yet", []compose.HealthResult{at(0, 7, "curl: (7) Failed to connect"), at(30, 7, ""), at(60, 7, "")}, false},
		{"server error", []compose.HealthResult{at(0, 22, "returned error: 503"), at(60, 22, "returned error: 503")}, false},
		{"empty", nil, false},
	}
	for _, tt := range tests {
		if got := authLoopIn(tt.log); got != tt.want {
			t.Errorf("%s: authLoopIn = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestUpFailsBeforeStart(t *testing.T) {
	w := newWorkspace(t, workspace.Default("ws"))
	f := &fakeCompose{upErr: errors.New("exit status 1")}
	_, err := Up(context.Background(), env(f, true), w, UpOptions{Wait: true, SkipDoctor: true, Progress: &strings.Builder{}})
	if output.CodeOf(err) != output.ExitError {
		t.Errorf("Up = %v, want a generic error", err)
	}
}

func TestDownKeepsData(t *testing.T) {
	f := &fakeCompose{}
	w := newWorkspace(t, workspace.Default("ws"))
	if res, err := Down(context.Background(), env(f, true), w, &strings.Builder{}); err != nil || res.DataRemoved {
		t.Errorf("Down = %+v, %v", res, err)
	}
	if slices.Contains(f.calls[0], "--volumes") {
		t.Errorf("down removed volumes: %v", f.calls[0])
	}

	cfg := workspace.Default("ws")
	cfg.Storage.Mode = workspace.StorageNone
	f = &fakeCompose{}
	if res, err := Down(context.Background(), env(f, true), newWorkspace(t, cfg), &strings.Builder{}); err != nil || !res.DataRemoved || !slices.Contains(f.calls[0], "--volumes") {
		t.Errorf("Down with storage none = %+v, %v, %v", res, err, f.calls)
	}
}

func TestLogs(t *testing.T) {
	w := newWorkspace(t, workspace.Default("ws"))
	f := &fakeCompose{}
	res, err := Logs(context.Background(), env(f, true), w, LogsOptions{Services: []string{"openarch-app"}, Tail: "5"})
	if err != nil || len(res.Lines) != 1 || res.Lines[0].Service != "openarch-app" {
		t.Errorf("Logs = %+v, %v", res, err)
	}
	if c := f.calls[0]; !slices.Contains(c, "--tail") || c[len(c)-1] != "openarch-app" {
		t.Errorf("logs args = %v", c)
	}
	if _, err := Logs(context.Background(), env(f, true), w, LogsOptions{Services: []string{"proxy"}}); output.CodeOf(err) != output.ExitUsage {
		t.Errorf("Logs for a service of another config = %v", err)
	}
}

// A change to the users file changes only the hash of openarch-app.
func TestConfigHashes(t *testing.T) {
	cfg := workspace.Default("ws")
	cfg.With = []string{workspace.WithDashboards}
	w := newWorkspace(t, cfg)
	before := w.ConfigHashes()
	if err := os.WriteFile(filepath.Join(w.Dir, "config", "users.yml"), []byte("users: [changed]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	after := w.ConfigHashes()
	if len(before) != 4 || len(after) != 4 {
		t.Fatalf("hashes = %v", before)
	}
	for i := range before {
		changed := before[i] != after[i]
		if changed != strings.HasPrefix(before[i], workspace.HashVarApp+"=") {
			t.Errorf("%s changed=%v", before[i], changed)
		}
	}
}

func TestLoadNeedsSecrets(t *testing.T) {
	w := newWorkspace(t, workspace.Default("ws"))
	if err := os.Remove(filepath.Join(w.Dir, localenv.File)); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(w.Dir); output.CodeOf(err) != output.ExitUsage {
		t.Errorf("Load = %v", err)
	}
}

func TestClean(t *testing.T) {
	f := &fakeCompose{}
	res, err := Clean(context.Background(), env(f, true), newWorkspace(t, workspace.Default("ws")), &strings.Builder{})
	if err != nil || res.DataFolder != "" || !slices.Contains(f.calls[0], "--volumes") {
		t.Errorf("Clean = %+v, %v, %v", res, err, f.calls)
	}
	cfg := workspace.Default("ws")
	cfg.Storage = workspace.Storage{Mode: workspace.StorageBind, DataDir: "./data"}
	w := newWorkspace(t, cfg)
	if res, _ := Clean(context.Background(), env(&fakeCompose{}, true), w, &strings.Builder{}); res.DataFolder != filepath.Join(w.Dir, "data") {
		t.Errorf("bind data folder = %q", res.DataFolder)
	}
}
