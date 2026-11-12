package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/AlfrescoLabs/openarch-cli/internal/compose"
	"github.com/AlfrescoLabs/openarch-cli/internal/generate"
	"github.com/AlfrescoLabs/openarch-cli/internal/output"
	"github.com/AlfrescoLabs/openarch-cli/internal/stack"
	"github.com/AlfrescoLabs/openarch-cli/internal/workspace"
)

// psLines is the docker compose ps output of a stack whose OpenSearch never gets healthy.
const psLines = `{"Service":"mongodb","State":"running"}
{"Service":"mongodb-init","State":"exited","ExitCode":0}
{"Service":"opensearch","State":"running","Health":"starting"}
{"Service":"openarch-app","State":"created"}
`

func runStack(t *testing.T, ps string, upErr error, args ...string) (int, string, string) {
	t.Helper()
	t.Setenv(envOutput, "")
	dir := filepath.Join(t.TempDir(), "ws")
	newTestApp := func(out, errw *bytes.Buffer) *App {
		app := newApp(Streams{In: strings.NewReader(""), Out: out, Err: errw})
		app.generateEnv = func() generate.Env {
			return generate.Env{HostMemory: func(context.Context) (int64, error) { return 8 << 30, nil }, Now: time.Now}
		}
		app.doctorEnv = fakeDoctorEnv(false)
		app.stackEnv = func() stack.Env {
			return stack.Env{
				Compose: func(_ context.Context, c compose.Command) error {
					if slices.Contains(c.Args, "ps") {
						_, _ = fmt.Fprint(c.Stdout, ps)
					}
					if slices.Contains(c.Args, "up") {
						return upErr
					}
					return nil
				},
				Doctor:   fakeDoctorEnv(false),
				CheckAPI: func(context.Context, string, workspace.Config) stack.APICheck { return stack.APICheck{OK: true} },
			}
		}
		return app
	}
	var out, errw bytes.Buffer
	if code := execute(context.Background(), newTestApp(&out, &errw), []string{"init", "--dir", dir, "--non-interactive"}); code != 0 {
		t.Fatalf("init failed: %s", errw.String())
	}
	out.Reset()
	errw.Reset()
	code := execute(context.Background(), newTestApp(&out, &errw), append(args, "--dir", dir))
	return code, out.String(), errw.String()
}

func TestUpTimeoutJSON(t *testing.T) {
	code, stdout, stderr := runStack(t, psLines, errors.New("exit status 1"), "up", "--skip-doctor", "--output", "json")
	if code != int(output.ExitUnhealthy) {
		t.Errorf("exit code = %d, want 4", code)
	}
	var st stack.Status
	dec := json.NewDecoder(strings.NewReader(stdout))
	if err := dec.Decode(&st); err != nil || dec.More() {
		t.Fatalf("stdout is not one JSON document: %q", stdout)
	}
	if st.State != stack.Starting || !strings.Contains(stderr, "Warning: OpenArch is starting after waiting") {
		t.Errorf("state = %s, stderr = %q", st.State, stderr)
	}
}

func TestStatusStoppedExitCode(t *testing.T) {
	code, stdout, _ := runStack(t, "", nil, "status")
	if code != int(output.ExitUnhealthy) || !strings.Contains(stdout, ": stopped") {
		t.Errorf("code = %d, stdout = %q", code, stdout)
	}
}

func TestStatusJSONHasSettings(t *testing.T) {
	_, stdout, _ := runStack(t, "", nil, "status", "--output", "json")
	var st stack.Status
	if err := json.Unmarshal([]byte(stdout), &st); err != nil {
		t.Fatalf("stdout is not JSON: %q", stdout)
	}
	if st.Settings == nil || st.Settings.Version != workspace.DefaultVersion || st.Settings.Port != 8080 || st.Settings.Memory == "" {
		t.Errorf("settings = %+v", st.Settings)
	}
}

func TestUpDryRun(t *testing.T) {
	code, stdout, _ := runStack(t, "", errors.New("must not run"), "up", "--dry-run")
	if code != 0 || !strings.Contains(stdout, "docker compose --env-file .env --env-file .env.local up") {
		t.Errorf("code = %d, stdout = %q", code, stdout)
	}
}

func TestLogsFollowJSON(t *testing.T) {
	code, stdout, _ := runStack(t, "", nil, "logs", "--follow", "--output", "json")
	if code != int(output.ExitUsage) || !strings.Contains(stdout, "--follow cannot be used") {
		t.Errorf("code = %d, stdout = %q", code, stdout)
	}
}

func TestStackCommandsNeedWorkspace(t *testing.T) {
	t.Setenv(envOutput, "")
	var out, errw bytes.Buffer
	app := newApp(Streams{In: strings.NewReader(""), Out: &out, Err: &errw})
	code := execute(context.Background(), app, []string{"status", "--dir", t.TempDir()})
	if code != int(output.ExitUsage) || !strings.Contains(errw.String(), "openarch init") {
		t.Errorf("code = %d, stderr = %q", code, errw.String())
	}
}

func TestCleanNeedsYes(t *testing.T) {
	code, _, stderr := runStack(t, "", nil, "clean")
	if code != int(output.ExitUsage) || !strings.Contains(stderr, "needs --yes") {
		t.Errorf("code = %d, stderr = %q", code, stderr)
	}
	if code, stdout, _ := runStack(t, "", nil, "clean", "--yes"); code != 0 || !strings.Contains(stdout, "Removed the containers and volumes") {
		t.Errorf("code = %d, stdout = %q", code, stdout)
	}
}
