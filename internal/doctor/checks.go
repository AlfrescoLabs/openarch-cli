package doctor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	gib = 1 << 30
	// Memory thresholds are estimates: 512 MB OpenSearch heap, MongoDB and the OpenArch JVM.
	minMemory         = 3 * gib
	recommendedMemory = 4 * gib
	recommendedCPUs   = 2
	// OpenSearch refuses to start on Linux hosts below this value.
	minMaxMapCount = 262144
	registryURL    = "https://ghcr.io/v2/"
	commandTimeout = 20 * time.Second
)

const installDockerFix = "Install Docker Desktop: https://docs.docker.com/get-started/get-docker/"

type dockerInfo struct {
	ServerVersion   string
	OperatingSystem string
	NCPU            int
	MemTotal        int64
	ServerErrors    []string
}

// state caches data shared by several checks.
type state struct {
	env Env

	cliDone bool
	cliPath string
	cliErr  error

	infoDone bool
	info     dockerInfo
	infoErr  error
}

func (s *state) dockerCLI() (string, error) {
	if !s.cliDone {
		s.cliPath, s.cliErr = s.env.LookPath("docker")
		s.cliDone = true
	}
	return s.cliPath, s.cliErr
}

func (s *state) run(ctx context.Context, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()
	stdout, stderr, err := s.env.Run(ctx, name, args...)
	if err != nil {
		if msg := firstLine(stderr); msg != "" {
			return stdout, errors.New(msg)
		}
		return stdout, err
	}
	return stdout, nil
}

func (s *state) dockerInfo(ctx context.Context) (dockerInfo, error) {
	if !s.infoDone {
		s.infoDone = true
		out, err := s.run(ctx, "docker", "info", "--format", "{{json .}}")
		switch {
		case err != nil:
			s.infoErr = err
		case json.Unmarshal(out, &s.info) != nil:
			s.infoErr = errors.New("cannot parse the output of 'docker info'")
		case len(s.info.ServerErrors) > 0:
			s.infoErr = errors.New(s.info.ServerErrors[0])
		}
	}
	return s.info, s.infoErr
}

func checks(env Env, opts Options) []check {
	cs := []check{
		{"docker.cli", "Docker CLI", checkDockerCLI},
		{"docker.daemon", "Docker daemon", checkDockerDaemon},
		{"docker.compose", "Docker Compose v2 or later", checkCompose},
		{"docker.memory", "Docker memory", checkMemory},
		{"docker.cpus", "Docker CPUs", checkCPUs},
	}
	if env.GOOS == "linux" {
		cs = append(cs, check{"linux.max_map_count", "vm.max_map_count for OpenSearch", checkMaxMapCount})
	}
	ports := opts.Ports
	if len(ports) == 0 {
		ports = DefaultPorts
	}
	for _, p := range ports {
		cs = append(cs, check{fmt.Sprintf("port.%d", p), fmt.Sprintf("Port %d is free", p), portCheck(p)})
	}
	return append(cs, check{"network.ghcr", "Access to ghcr.io", checkRegistry})
}

func checkDockerCLI(_ context.Context, s *state) Result {
	path, err := s.dockerCLI()
	if err != nil {
		return Result{Status: Fail, Message: "docker command not found in PATH", Fix: installDockerFix}
	}
	return Result{Status: OK, Message: "found " + path}
}

func checkDockerDaemon(ctx context.Context, s *state) Result {
	if _, err := s.dockerCLI(); err != nil {
		return skipped("docker CLI is missing")
	}
	info, err := s.dockerInfo(ctx)
	if err != nil {
		fix := "Start Docker Desktop."
		if s.env.GOOS == "linux" {
			fix = "Start Docker Desktop, or start the Docker service: sudo systemctl start docker"
		}
		return Result{Status: Fail, Message: "cannot connect to the Docker daemon: " + err.Error(), Fix: fix}
	}
	return Result{Status: OK, Message: fmt.Sprintf("Docker %s (%s)", info.ServerVersion, info.OperatingSystem)}
}

func checkCompose(ctx context.Context, s *state) Result {
	if _, err := s.dockerCLI(); err != nil {
		return skipped("docker CLI is missing")
	}
	const fix = "Install the Docker Compose plugin (included in Docker Desktop): https://docs.docker.com/compose/install/"
	out, err := s.run(ctx, "docker", "compose", "version", "--short")
	if err != nil {
		return Result{Status: Fail, Message: "'docker compose' is not available: " + err.Error(), Fix: fix}
	}
	v := strings.TrimPrefix(strings.TrimSpace(string(out)), "v")
	major, err := strconv.Atoi(strings.SplitN(v, ".", 2)[0])
	if err != nil {
		return Result{Status: Warn, Message: fmt.Sprintf("cannot parse Docker Compose version %q", v)}
	}
	if major < 2 {
		return Result{Status: Fail, Message: "Docker Compose " + v + " is too old", Fix: fix}
	}
	return Result{Status: OK, Message: "Docker Compose " + v}
}

func checkMemory(ctx context.Context, s *state) Result {
	info, ok := s.infoOrSkip(ctx)
	if !ok {
		return skipped("Docker daemon is not reachable")
	}
	mem := fmt.Sprintf("%.1f GiB available to Docker", float64(info.MemTotal)/gib)
	fix := fmt.Sprintf("Give Docker at least %d GiB of memory (Docker Desktop: Settings > Resources).", recommendedMemory/gib)
	switch {
	case info.MemTotal < minMemory:
		return Result{Status: Fail, Message: mem + fmt.Sprintf(", at least %d GiB needed", minMemory/gib), Fix: fix}
	case info.MemTotal < recommendedMemory:
		return Result{Status: Warn, Message: mem + fmt.Sprintf(", %d GiB recommended", recommendedMemory/gib), Fix: fix}
	}
	return Result{Status: OK, Message: mem}
}

func checkCPUs(ctx context.Context, s *state) Result {
	info, ok := s.infoOrSkip(ctx)
	if !ok {
		return skipped("Docker daemon is not reachable")
	}
	msg := fmt.Sprintf("%d CPUs available to Docker", info.NCPU)
	if info.NCPU < recommendedCPUs {
		return Result{Status: Warn, Message: msg + fmt.Sprintf(", %d recommended", recommendedCPUs),
			Fix: "Give Docker more CPUs (Docker Desktop: Settings > Resources)."}
	}
	return Result{Status: OK, Message: msg}
}

func checkMaxMapCount(ctx context.Context, s *state) Result {
	if info, ok := s.infoOrSkip(ctx); ok && strings.Contains(info.OperatingSystem, "Docker Desktop") {
		return Result{Status: OK, Message: "set by Docker Desktop"}
	}
	data, err := s.env.ReadFile("/proc/sys/vm/max_map_count")
	if err != nil {
		return Result{Status: Warn, Message: "cannot read /proc/sys/vm/max_map_count: " + err.Error()}
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return Result{Status: Warn, Message: fmt.Sprintf("cannot parse vm.max_map_count %q", strings.TrimSpace(string(data)))}
	}
	if n < minMaxMapCount {
		return Result{Status: Fail, Message: fmt.Sprintf("vm.max_map_count is %d, OpenSearch needs at least %d", n, minMaxMapCount),
			Fix: fmt.Sprintf("sudo sysctl -w vm.max_map_count=%d (add it to /etc/sysctl.conf to keep it after a reboot)", minMaxMapCount)}
	}
	return Result{Status: OK, Message: fmt.Sprintf("vm.max_map_count is %d", n)}
}

func portCheck(port int) func(context.Context, *state) Result {
	return func(ctx context.Context, s *state) Result {
		if s.env.PortFree(ctx, port) {
			return Result{Status: OK, Message: fmt.Sprintf("port %d is free", port)}
		}
		find := fmt.Sprintf("lsof -nP -iTCP:%d -sTCP:LISTEN", port)
		if s.env.GOOS == "windows" {
			find = fmt.Sprintf("netstat -ano | findstr :%d", port)
		}
		return Result{Status: Fail, Message: fmt.Sprintf("port %d is in use", port),
			Fix: fmt.Sprintf("Find the process with '%s' and stop it, or use another port.", find)}
	}
}

func checkRegistry(ctx context.Context, s *state) Result {
	// Any HTTP response (401 is normal without a token) proves the registry is reachable.
	if _, err := s.env.HTTPStatus(ctx, registryURL); err != nil {
		return Result{Status: Warn, Message: "cannot reach ghcr.io: " + err.Error(),
			Fix: "Check your network and proxy settings. OpenArch images are pulled from ghcr.io; images already pulled still work offline."}
	}
	return Result{Status: OK, Message: "ghcr.io is reachable"}
}

func (s *state) infoOrSkip(ctx context.Context) (dockerInfo, bool) {
	if _, err := s.dockerCLI(); err != nil {
		return dockerInfo{}, false
	}
	info, err := s.dockerInfo(ctx)
	return info, err == nil
}

func skipped(reason string) Result {
	return Result{Status: Skip, Message: "skipped: " + reason}
}

func firstLine(b []byte) string {
	line, _, _ := strings.Cut(strings.TrimSpace(string(b)), "\n")
	return strings.TrimSpace(line)
}
