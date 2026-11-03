package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/AlfrescoLabs/openarch-cli/internal/doctor"
	"github.com/AlfrescoLabs/openarch-cli/internal/output"
)

func fakeDoctorEnv(portBusy bool) func() doctor.Env {
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
			PortFree:   func(context.Context, int) bool { return !portBusy },
			HTTPStatus: func(context.Context, string) (int, error) { return 401, nil },
			ReadFile:   func(string) ([]byte, error) { return nil, errors.New("unused") },
		}
	}
}

func runDoctor(t *testing.T, portBusy bool, args ...string) (int, string, string) {
	t.Helper()
	t.Setenv(envOutput, "")
	t.Setenv("OPENARCH_WORKSPACE", t.TempDir())
	var out, errw bytes.Buffer
	app := newApp(Streams{In: strings.NewReader(""), Out: &out, Err: &errw})
	app.doctorEnv = fakeDoctorEnv(portBusy)
	code := execute(context.Background(), app, append([]string{"doctor"}, args...))
	return code, out.String(), errw.String()
}

func TestDoctorHealthy(t *testing.T) {
	code, stdout, stderr := runDoctor(t, false)
	if code != 0 || !strings.Contains(stdout, "7 ok, 0 warn, 0 fail, 0 skip") || stderr != "" {
		t.Errorf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestDoctorFailureJSON(t *testing.T) {
	code, stdout, stderr := runDoctor(t, true, "--output", "json")
	if code != int(output.ExitPrerequisite) {
		t.Errorf("exit code = %d, want %d", code, output.ExitPrerequisite)
	}
	// Exactly one JSON document: the report, with no error object after it.
	var report doctor.Report
	dec := json.NewDecoder(strings.NewReader(stdout))
	if err := dec.Decode(&report); err != nil {
		t.Fatalf("stdout is not JSON: %q", stdout)
	}
	if dec.More() {
		t.Errorf("more than one JSON document on stdout: %q", stdout)
	}
	if report.Summary.Fail != 1 || stderr != "" {
		t.Errorf("summary=%+v stderr=%q", report.Summary, stderr)
	}
}

func TestDoctorUnknownCheck(t *testing.T) {
	code, _, stderr := runDoctor(t, false, "--check", "nope")
	if code != int(output.ExitUsage) || !strings.Contains(stderr, "Valid checks: docker.cli") {
		t.Errorf("code=%d stderr=%q", code, stderr)
	}
}

func TestDoctorBadPort(t *testing.T) {
	if code, _, _ := runDoctor(t, false, "--port", "abc"); code != int(output.ExitUsage) {
		t.Errorf("code=%d, want %d", code, output.ExitUsage)
	}
}
