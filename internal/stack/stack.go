package stack

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/AlfrescoLabs/openarch-cli/internal/compose"
	"github.com/AlfrescoLabs/openarch-cli/internal/doctor"
	"github.com/AlfrescoLabs/openarch-cli/internal/localenv"
	"github.com/AlfrescoLabs/openarch-cli/internal/output"
	"github.com/AlfrescoLabs/openarch-cli/internal/templates"
	"github.com/AlfrescoLabs/openarch-cli/internal/workspace"
)

// Env is the access to the system. Tests replace it with fakes.
type Env struct {
	Compose compose.Exec
	Doctor  func() doctor.Env
	// CheckAPI calls the OpenArch health endpoint of a workspace.
	CheckAPI func(ctx context.Context, dir string, cfg workspace.Config) APICheck
	// HealthPoll is how often 'up --wait' reads the healthchecks. Zero means 15 s.
	HealthPoll time.Duration
}

// SystemEnv returns an Env backed by the real system.
func SystemEnv() Env {
	return Env{Compose: compose.SystemExec, Doctor: doctor.SystemEnv, CheckAPI: checkAPI}
}

// Workspace is a loaded workspace.
type Workspace struct {
	Dir     string
	Config  workspace.Config
	Secrets localenv.Env
}

// Load reads openarch.yaml and .env.local from dir.
func Load(dir string) (Workspace, error) {
	cfg, err := workspace.Load(filepath.Join(dir, workspace.ConfigFile))
	if err != nil {
		return Workspace{}, err
	}
	w := Workspace{Dir: dir, Config: cfg}
	data, err := os.ReadFile(filepath.Join(dir, localenv.File))
	switch {
	case err == nil:
		w.Secrets = localenv.Parse(data)
	case errors.Is(err, os.ErrNotExist):
		if cfg.Security.BackendAuth {
			return Workspace{}, output.Usage(localenv.File+" not found in "+dir, "Run 'openarch render' to create the missing secrets.")
		}
	default:
		return Workspace{}, err
	}
	if _, err := os.Stat(filepath.Join(dir, "compose.yaml")); err != nil {
		return Workspace{}, output.Usage("compose.yaml not found in "+dir, "Run 'openarch render' to write the workspace files.")
	}
	return w, nil
}

func (w Workspace) runner(env Env) compose.Runner {
	return compose.Runner{Dir: w.Dir, Exec: env.Compose, Env: w.ConfigHashes()}
}

// ConfigHashes returns the hash variables of the compose.yaml labels. Each
// hash covers the files and secrets a service reads only at start, which
// Docker Compose cannot see change by itself.
func (w Workspace) ConfigHashes() []string {
	s := w.Secrets
	inputs := map[string][]string{
		workspace.HashVarApp:        {"file:config/users.yml", s.MongoAppURI(), s.OpenSearchAppPassword},
		workspace.HashVarMongoDB:    {s.MongoRootPassword, s.MongoAppPassword},
		workspace.HashVarOpenSearch: {"file:config/opensearch/opensearch.yml", "file:" + templates.OpenSearchUsersFile, "file:config/opensearch/roles.yml", "file:config/opensearch/roles_mapping.yml", s.OpenSearchAppPassword},
		workspace.HashVarDashboards: {s.DashboardsPassword},
		workspace.HashVarProxy:      {"file:config/Caddyfile", "file:" + certFile(w), "file:" + keyFile(w)},
	}
	var env []string
	seen := map[string]bool{}
	for _, svc := range w.Config.Services() {
		if svc.HashVar == "" || seen[svc.HashVar] {
			continue
		}
		seen[svc.HashVar] = true
		h := sha256.New()
		for _, in := range inputs[svc.HashVar] {
			if p, ok := strings.CutPrefix(in, "file:"); ok {
				data, _ := os.ReadFile(resolve(w.Dir, p)) // A missing file hashes as empty.
				h.Write(data)
			} else {
				h.Write([]byte(in))
			}
			h.Write([]byte{0})
		}
		env = append(env, svc.HashVar+"="+hex.EncodeToString(h.Sum(nil))[:16])
	}
	return env
}

func certFile(w Workspace) string {
	if w.Config.TLS.Mode == workspace.TLSCustom {
		return w.Config.TLS.Cert
	}
	return templates.SelfSignedCert
}

func keyFile(w Workspace) string {
	if w.Config.TLS.Mode == workspace.TLSCustom {
		return w.Config.TLS.Key
	}
	return templates.SelfSignedKey
}

func resolve(dir, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(dir, filepath.FromSlash(p))
}

// GetStatus returns the state of the stack, with an API check when OpenArch is running.
func GetStatus(ctx context.Context, env Env, w Workspace) (Status, error) {
	cs, err := w.runner(env).Ps(ctx)
	if err != nil {
		return Status{}, err
	}
	var api *APICheck
	for _, c := range cs {
		if c.Service == "openarch-app" && c.State == "running" {
			a := env.CheckAPI(ctx, w.Dir, w.Config)
			api = &a
		}
	}
	st := Evaluate(w.Config, cs, api)
	st.Workspace, st.AdminUser = w.Dir, w.Secrets.AdminUser
	cfg := w.Config
	st.Settings = &cfg
	return st, nil
}

// UpOptions are the inputs of Up.
type UpOptions struct {
	SkipDoctor bool
	Wait       bool
	Timeout    time.Duration
	// Progress receives the output of docker compose.
	Progress io.Writer
}

// Up checks the prerequisites, starts the stack and, with Wait, waits until it is healthy.
// A stack that is not healthy in time gives a status and an error with exit code 4.
func Up(ctx context.Context, env Env, w Workspace, opts UpOptions) (Status, error) {
	r := w.runner(env)
	if !opts.SkipDoctor {
		if err := preflight(ctx, env, w, r); err != nil {
			return Status{}, err
		}
	}
	args := []string{"up", "--detach", "--remove-orphans"}
	if opts.Wait {
		args = append(args, "--wait", "--wait-timeout", strconv.Itoa(int(opts.Timeout.Seconds())))
	}
	var upErr error
	authSvc := ""
	if opts.Wait {
		authSvc, upErr = upWatched(ctx, env, r, opts.Progress, args)
	} else {
		upErr = r.Run(ctx, opts.Progress, opts.Progress, args...)
	}
	if ctx.Err() != nil {
		return Status{}, ctx.Err()
	}
	st, err := GetStatus(ctx, env, w)
	if err != nil {
		if upErr != nil {
			return Status{}, fmt.Errorf("docker compose up failed: %w", upErr)
		}
		return Status{}, err
	}
	st.Command = "up"
	switch {
	case authSvc != "":
		return st, output.NewError(output.ExitUnhealthy,
			fmt.Sprintf("the %s healthcheck failed with HTTP 401 (authentication failed) for %s, so the stack cannot become healthy", authSvc, authLoop),
			"The container may see an old copy of a config file: after a workspace folder is moved, or made again at the same path, "+
				"Docker Desktop can keep a file mount tied to the old file. Restart Docker Desktop, or use a new folder. "+
				"'openarch logs "+authSvc+"' shows the server side.")
	case upErr != nil && st.State == Stopped:
		// Nothing started: a problem in the files or with Docker, not a health problem.
		return st, output.NewError(output.ExitError, "docker compose up failed: "+upErr.Error(), "Read the docker compose output above.")
	case upErr != nil || (opts.Wait && st.State != Healthy):
		return st, output.NewError(output.ExitUnhealthy,
			fmt.Sprintf("OpenArch is %s after waiting %s", st.State, opts.Timeout),
			"Run 'openarch status' to see each service, and 'openarch logs openarch-app' for details.")
	}
	return st, nil
}

// authLoop is how long a healthcheck must fail with HTTP 401 before 'up' stops waiting.
const authLoop = time.Minute

// upWatched runs 'docker compose up --wait' and reads the healthchecks while it
// waits. A service that fails with HTTP 401 for authLoop never becomes healthy,
// so upWatched stops compose and returns that service.
func upWatched(ctx context.Context, env Env, r compose.Runner, progress io.Writer, args []string) (string, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- r.Run(ctx, progress, progress, args...) }()
	poll := env.HealthPoll
	if poll == 0 {
		poll = 15 * time.Second
	}
	tick := time.NewTicker(poll)
	defer tick.Stop()
	for {
		select {
		case err := <-done:
			return "", err
		case <-tick.C:
			if svc := authLoopService(ctx, r); svc != "" {
				cancel()
				return svc, <-done
			}
		}
	}
}

// authLoopService returns a service whose healthcheck failed with HTTP 401 in
// every result of the last authLoop, or "". Errors only mean no answer yet.
func authLoopService(ctx context.Context, r compose.Runner) string {
	cs, err := r.Ps(ctx)
	if err != nil {
		return ""
	}
	var names []string
	for _, c := range cs {
		if c.State == "running" && c.Health != "" && c.Health != "healthy" {
			names = append(names, c.Name)
		}
	}
	if len(names) == 0 {
		return ""
	}
	hs, err := r.Health(ctx, names...)
	if err != nil {
		return ""
	}
	for _, h := range hs {
		if authLoopIn(h.Log) {
			return h.Service
		}
	}
	return ""
}

// authLoopIn reports whether the results of the last authLoop are all HTTP 401
// failures, and the log goes back at least authLoop.
func authLoopIn(log []compose.HealthResult) bool {
	if len(log) == 0 {
		return false
	}
	last := log[len(log)-1].Start
	for i := len(log) - 1; i >= 0; i-- {
		res := log[i]
		if res.ExitCode == 0 || !strings.Contains(res.Output, "returned error: 401") {
			return false
		}
		if last.Sub(res.Start) >= authLoop {
			return true
		}
	}
	return false
}

// preflight runs doctor. Ports used by this stack itself are not a problem.
func preflight(ctx context.Context, env Env, w Workspace, r compose.Runner) error {
	var ports, own []int
	for _, p := range w.Config.HostPorts() {
		ports = append(ports, p.Port)
	}
	if cs, err := r.Ps(ctx); err == nil {
		for _, c := range cs {
			for _, p := range c.Publishers {
				if c.State == "running" && !slices.Contains(own, p.PublishedPort) {
					own = append(own, p.PublishedPort)
				}
			}
		}
	}
	report, err := doctor.Run(ctx, env.Doctor(), doctor.Options{Ports: ports, OwnPorts: own})
	if err != nil {
		return err
	}
	if !report.Failed() {
		return nil
	}
	var failed []string
	for _, c := range report.Checks {
		if c.Status == doctor.Fail {
			failed = append(failed, c.ID+": "+c.Message)
		}
	}
	return output.NewError(output.ExitPrerequisite, "prerequisite check failed: "+strings.Join(failed, "; "),
		"Run 'openarch doctor' for the fixes, or use --skip-doctor.")
}

// DownResult is the outcome of Down.
type DownResult struct {
	Workspace   string `json:"workspace"`
	DataRemoved bool   `json:"dataRemoved"`
}

// Down stops and removes the containers. Named volumes and data folders are kept.
// With storage none, the anonymous volumes are removed too, as that mode keeps no data.
func Down(ctx context.Context, env Env, w Workspace, progress io.Writer) (DownResult, error) {
	args := []string{"down", "--remove-orphans"}
	none := w.Config.Storage.Mode == workspace.StorageNone
	if none {
		args = append(args, "--volumes")
	}
	if err := w.runner(env).Run(ctx, progress, progress, args...); err != nil {
		return DownResult{}, fmt.Errorf("docker compose down failed: %w", err)
	}
	return DownResult{Workspace: w.Dir, DataRemoved: none}, nil
}

// WriteText writes the human-readable form.
func (d DownResult) WriteText(out io.Writer) error {
	msg := "Stopped. The data is kept: 'openarch up' starts the stack again.\n"
	if d.DataRemoved {
		msg = "Stopped. Storage is none, so the data was removed.\n"
	}
	_, err := io.WriteString(out, msg)
	return err
}

// CleanResult is the outcome of Clean.
type CleanResult struct {
	Workspace string `json:"workspace"`
	// DataFolder is set with storage bind: Clean does not delete it.
	DataFolder string `json:"dataFolder,omitempty"`
}

// Clean stops the stack and deletes its containers, network and volumes.
// The data folders of storage bind are not deleted.
func Clean(ctx context.Context, env Env, w Workspace, progress io.Writer) (CleanResult, error) {
	if err := w.runner(env).Run(ctx, progress, progress, "down", "--volumes", "--remove-orphans"); err != nil {
		return CleanResult{}, fmt.Errorf("docker compose down failed: %w", err)
	}
	res := CleanResult{Workspace: w.Dir}
	if w.Config.Storage.Mode == workspace.StorageBind {
		res.DataFolder = resolve(w.Dir, w.Config.Storage.DataDir)
	}
	return res, nil
}

// WriteText writes the human-readable form.
func (c CleanResult) WriteText(out io.Writer) error {
	msg := "Removed the containers and volumes. 'openarch up' starts again with no data.\n"
	if c.DataFolder != "" {
		msg = "Removed the containers and volumes. The data folder " + c.DataFolder + " is kept: delete it by hand to remove the data.\n"
	}
	_, err := io.WriteString(out, msg)
	return err
}

// LogsOptions are the inputs of Logs.
type LogsOptions struct {
	Services []string
	Follow   bool
	Tail     string
	Since    string
	// Stream receives the log lines as text. When nil, Logs returns them parsed.
	Stream io.Writer
	Stderr io.Writer
}

// LogLine is one parsed log line.
type LogLine struct {
	Service   string `json:"service"`
	Container string `json:"container"`
	Text      string `json:"text"`
}

// LogsResult holds parsed log lines, for JSON output.
type LogsResult struct {
	Lines []LogLine `json:"lines"`
}

// Logs shows the logs of some or all services.
func Logs(ctx context.Context, env Env, w Workspace, opts LogsOptions) (LogsResult, error) {
	names := w.Config.ServiceNames()
	for _, s := range opts.Services {
		if !slices.Contains(names, s) {
			return LogsResult{}, output.Usage(fmt.Sprintf("unknown service %q", s), "Services of this workspace: "+strings.Join(names, ", ")+".")
		}
	}
	args := []string{"logs", "--no-color"}
	if opts.Follow {
		args = append(args, "--follow")
	}
	if opts.Tail != "" {
		args = append(args, "--tail", opts.Tail)
	}
	if opts.Since != "" {
		args = append(args, "--since", opts.Since)
	}
	args = append(args, opts.Services...)
	r := w.runner(env)
	if opts.Stream != nil {
		err := r.Run(ctx, opts.Stream, opts.Stderr, args...)
		if err != nil && ctx.Err() != nil {
			return LogsResult{}, nil // Ctrl+C ends --follow; that is not an error.
		}
		return LogsResult{}, err
	}
	out, err := r.Output(ctx, args...)
	if err != nil {
		return LogsResult{}, err
	}
	return LogsResult{Lines: ParseLogs(out)}, nil
}

// ParseLogs splits 'docker compose logs --no-color' output, where each line
// starts with "<container> | ".
func ParseLogs(data []byte) []LogLine {
	lines := []LogLine{}
	s := bufio.NewScanner(bytes.NewReader(data))
	s.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for s.Scan() {
		prefix, text, ok := strings.Cut(s.Text(), " | ")
		if !ok {
			// Some compose versions write "<container>  |" with no text for an empty line.
			prefix, ok = strings.CutSuffix(s.Text(), " |")
			if !ok {
				continue
			}
		}
		container := strings.TrimSpace(prefix)
		lines = append(lines, LogLine{Service: serviceOf(container), Container: container, Text: text})
	}
	return lines
}

// serviceOf removes the "-<n>" container number.
func serviceOf(container string) string {
	if i := strings.LastIndex(container, "-"); i > 0 {
		if _, err := strconv.Atoi(container[i+1:]); err == nil {
			return container[:i]
		}
	}
	return container
}

// checkAPI calls <url>/actuator/health. It connects to the published port on
// this machine, and keeps the server name for the Host header and for TLS.
func checkAPI(ctx context.Context, dir string, cfg workspace.Config) APICheck {
	url := cfg.URL() + "/actuator/health"
	res := APICheck{URL: url}
	host := cfg.BindIP
	if ip := net.ParseIP(host); ip == nil || ip.IsUnspecified() {
		host = "127.0.0.1"
	}
	addr := net.JoinHostPort(host, strconv.Itoa(cfg.Port))
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, addr)
		},
	}
	if cfg.TLSEnabled() {
		pool, err := x509.SystemCertPool()
		if err != nil {
			pool = x509.NewCertPool()
		}
		// Trust the workspace certificate too: it is self-signed, or a custom one that may be.
		if pem, err := os.ReadFile(resolve(dir, certFile(Workspace{Dir: dir, Config: cfg}))); err == nil {
			pool.AppendCertsFromPEM(pem)
		}
		transport.TLSClientConfig = &tls.Config{RootCAs: pool, ServerName: cfg.ServerName, MinVersion: tls.VersionTLS12}
	}
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	resp, err := client.Do(req)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	defer func() { _ = resp.Body.Close() }()
	res.StatusCode = resp.StatusCode
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	res.OK = resp.StatusCode == http.StatusOK && bytes.Contains(body, []byte(`"UP"`))
	if !res.OK && res.Error == "" && resp.StatusCode == http.StatusOK {
		res.Error = "health status is not UP"
	}
	return res
}

// writeFile replaces path with data through a temporary file.
func writeFile(path string, data []byte, mode os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
