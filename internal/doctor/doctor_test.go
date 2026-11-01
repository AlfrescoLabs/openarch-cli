package doctor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/AlfrescoLabs/openarch-cli/internal/output"
)

// fake is a healthy system. Tests change one field to break one thing.
type fake struct {
	goos         string
	noCLI        bool
	infoJSON     string
	infoErr      string
	composeOut   string
	composeErr   string
	busyPorts    map[int]bool
	offline      bool
	maxMapCount  string
	dockerCalled int
}

func newFake() *fake {
	return &fake{
		goos:        "darwin",
		infoJSON:    info("Docker Desktop", 8, 8*gib),
		composeOut:  "v2.29.1\n",
		maxMapCount: "262144\n",
	}
}

func info(os string, cpus int, mem int64) string {
	return fmt.Sprintf(`{"ServerVersion":"29.0.0","OperatingSystem":%q,"NCPU":%d,"MemTotal":%d}`, os, cpus, mem)
}

func (f *fake) env() Env {
	return Env{
		GOOS: f.goos,
		LookPath: func(string) (string, error) {
			if f.noCLI {
				return "", errors.New("not found")
			}
			return "/usr/bin/docker", nil
		},
		Run: func(_ context.Context, _ string, args ...string) ([]byte, []byte, error) {
			f.dockerCalled++
			switch args[0] {
			case "info":
				if f.infoErr != "" {
					return nil, []byte(f.infoErr + "\n"), errors.New("exit status 1")
				}
				return []byte(f.infoJSON), nil, nil
			case "compose":
				if f.composeErr != "" {
					return nil, []byte(f.composeErr), errors.New("exit status 1")
				}
				return []byte(f.composeOut), nil, nil
			}
			return nil, nil, fmt.Errorf("unexpected command %v", args)
		},
		PortFree: func(_ context.Context, p int) bool { return !f.busyPorts[p] },
		HTTPStatus: func(context.Context, string) (int, error) {
			if f.offline {
				return 0, errors.New("dial tcp: no route to host")
			}
			return 401, nil
		},
		ReadFile: func(name string) ([]byte, error) {
			if name != "/proc/sys/vm/max_map_count" {
				return nil, os.ErrNotExist
			}
			return []byte(f.maxMapCount), nil
		},
	}
}

func run(t *testing.T, f *fake, opts Options) Report {
	t.Helper()
	r, err := Run(context.Background(), f.env(), opts)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func status(r Report) map[string]Status {
	m := map[string]Status{}
	for _, c := range r.Checks {
		m[c.ID] = c.Status
	}
	return m
}

func find(t *testing.T, r Report, id string) Result {
	t.Helper()
	for _, c := range r.Checks {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("check %s not in report", id)
	return Result{}
}

func TestHealthySystem(t *testing.T) {
	f := newFake()
	r := run(t, f, Options{})
	if r.Failed() || r.Summary.Warn != 0 || r.Summary.Skip != 0 {
		t.Fatalf("unexpected report: %+v", r)
	}
	want := []string{"docker.cli", "docker.daemon", "docker.compose", "docker.memory", "docker.cpus", "port.8080", "network.ghcr"}
	if got := IDs(f.env(), Options{}); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("IDs = %v, want %v", got, want)
	}
	// docker info runs once and is shared by the daemon, memory and CPU checks.
	if f.dockerCalled != 2 {
		t.Errorf("docker called %d times, want 2 (info + compose)", f.dockerCalled)
	}
}

func TestNoDockerCLI(t *testing.T) {
	f := newFake()
	f.noCLI = true
	s := status(run(t, f, Options{}))
	if s["docker.cli"] != Fail {
		t.Errorf("docker.cli = %s", s["docker.cli"])
	}
	for _, id := range []string{"docker.daemon", "docker.compose", "docker.memory", "docker.cpus"} {
		if s[id] != Skip {
			t.Errorf("%s = %s, want skip", id, s[id])
		}
	}
}

func TestDaemonDown(t *testing.T) {
	f := newFake()
	f.infoErr = "failed to connect to the docker API at unix:///var/run/docker.sock"
	r := run(t, f, Options{})
	d := find(t, r, "docker.daemon")
	if d.Status != Fail || !strings.Contains(d.Message, "failed to connect") || d.Fix == "" {
		t.Errorf("daemon = %+v", d)
	}
	if s := status(r); s["docker.memory"] != Skip || s["docker.compose"] != OK {
		t.Errorf("statuses = %v", s)
	}
}

func TestDaemonServerErrors(t *testing.T) {
	f := newFake()
	f.infoJSON = `{"ServerErrors":["Cannot connect to the Docker daemon"]}`
	if d := find(t, run(t, f, Options{}), "docker.daemon"); d.Status != Fail {
		t.Errorf("daemon = %+v", d)
	}
}

func TestComposeVersion(t *testing.T) {
	cases := []struct {
		out, errOut string
		want        Status
	}{
		{"v2.29.1", "", OK},
		{"5.6.1", "", OK},
		{"1.29.2", "", Fail},
		{"garbage", "", Warn},
		{"", "docker: 'compose' is not a docker command.", Fail},
	}
	for _, c := range cases {
		f := newFake()
		f.composeOut, f.composeErr = c.out, c.errOut
		if got := find(t, run(t, f, Options{}), "docker.compose"); got.Status != c.want {
			t.Errorf("compose %q/%q = %+v, want %s", c.out, c.errOut, got, c.want)
		}
	}
}

func TestResources(t *testing.T) {
	cases := []struct {
		cpus     int
		mem      int64
		wantMem  Status
		wantCPUs Status
	}{
		{8, 8 * gib, OK, OK},
		{2, 3*gib + gib/2, Warn, OK},
		{1, 2 * gib, Fail, Warn},
	}
	for _, c := range cases {
		f := newFake()
		f.infoJSON = info("Docker Desktop", c.cpus, c.mem)
		s := status(run(t, f, Options{}))
		if s["docker.memory"] != c.wantMem || s["docker.cpus"] != c.wantCPUs {
			t.Errorf("cpus=%d mem=%d: memory=%s cpus=%s", c.cpus, c.mem, s["docker.memory"], s["docker.cpus"])
		}
	}
}

func TestMaxMapCount(t *testing.T) {
	f := newFake()
	if _, ok := status(run(t, f, Options{}))["linux.max_map_count"]; ok {
		t.Error("linux.max_map_count must only run on Linux")
	}

	f.goos = "linux"
	f.infoJSON = info("Ubuntu 24.04 LTS", 4, 8*gib)
	f.maxMapCount = "65530\n"
	c := find(t, run(t, f, Options{}), "linux.max_map_count")
	if c.Status != Fail || !strings.Contains(c.Fix, "sysctl") {
		t.Errorf("low value: %+v", c)
	}

	f.infoJSON = info("Docker Desktop", 4, 8*gib)
	if c := find(t, run(t, f, Options{}), "linux.max_map_count"); c.Status != OK {
		t.Errorf("Docker Desktop: %+v", c)
	}
}

func TestPortInUse(t *testing.T) {
	f := newFake()
	f.busyPorts = map[int]bool{8080: true}
	c := find(t, run(t, f, Options{}), "port.8080")
	if c.Status != Fail || !strings.Contains(c.Fix, "lsof") {
		t.Errorf("unix: %+v", c)
	}
	f.goos = "windows"
	if c := find(t, run(t, f, Options{}), "port.8080"); !strings.Contains(c.Fix, "netstat") {
		t.Errorf("windows: %+v", c)
	}
}

func TestCustomPorts(t *testing.T) {
	f := newFake()
	f.busyPorts = map[int]bool{9090: true}
	s := status(run(t, f, Options{Ports: []int{8081, 9090}}))
	if _, ok := s["port.8080"]; ok || s["port.8081"] != OK || s["port.9090"] != Fail {
		t.Errorf("statuses = %v", s)
	}
}

func TestOffline(t *testing.T) {
	f := newFake()
	f.offline = true
	r := run(t, f, Options{})
	if c := find(t, r, "network.ghcr"); c.Status != Warn || r.Failed() {
		t.Errorf("offline: %+v", c)
	}
}

func TestOnly(t *testing.T) {
	f := newFake()
	f.noCLI = true
	r := run(t, f, Options{Only: []string{"port.8080", "docker.memory"}})
	if len(r.Checks) != 2 || r.Checks[0].ID != "docker.memory" || r.Checks[1].ID != "port.8080" {
		t.Fatalf("checks = %+v", r.Checks)
	}
	if r.Checks[0].Status != Skip {
		t.Errorf("docker.memory without CLI = %s, want skip", r.Checks[0].Status)
	}
}

func TestUnknownCheck(t *testing.T) {
	_, err := Run(context.Background(), newFake().env(), Options{Only: []string{"nope"}})
	var e *output.Error
	if !errors.As(err, &e) || e.Code != output.ExitUsage || !strings.Contains(e.Hint, "docker.cli") {
		t.Errorf("err = %v", err)
	}
}

func TestWriteText(t *testing.T) {
	f := newFake()
	f.busyPorts = map[int]bool{8080: true}
	var b bytes.Buffer
	if err := run(t, f, Options{}).WriteText(&b); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{"[ok]   docker.cli", "[fail] port.8080", "fix: Find the process", "6 ok, 0 warn, 1 fail, 0 skip"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}
