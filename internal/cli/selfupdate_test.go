package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AlfrescoLabs/openarch-cli/internal/output"
	"github.com/AlfrescoLabs/openarch-cli/internal/selfupdate"
	"github.com/AlfrescoLabs/openarch-cli/internal/version"
)

func runSelfUpdate(t *testing.T, current, exe string, args ...string) (int, string, string) {
	t.Helper()
	t.Setenv(envOutput, "")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/"+selfupdate.Repo+"/releases/latest" {
			w.Header().Set("Location", "https://github.com/"+selfupdate.Repo+"/releases/tag/v0.2.0")
			w.WriteHeader(http.StatusFound)
			return
		}
		t.Errorf("unexpected request %s", r.URL.Path)
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	old := version.Version
	version.Version = current
	t.Cleanup(func() { version.Version = old })

	var out, errw bytes.Buffer
	app := newApp(Streams{In: strings.NewReader(""), Out: &out, Err: &errw})
	app.selfUpdateEnv = func() selfupdate.Env {
		return selfupdate.Env{HTTP: srv.Client(), BaseURL: srv.URL, GOOS: "linux", GOARCH: "amd64",
			Executable: func() (string, error) { return exe, nil }}
	}
	code := execute(context.Background(), app, append([]string{"self-update"}, args...))
	return code, out.String(), errw.String()
}

func TestSelfUpdateCheck(t *testing.T) {
	code, stdout, stderr := runSelfUpdate(t, "0.1.0", "/usr/local/bin/openarch", "--check", "-o", "json")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	var res struct {
		Current, Target string
		UpdateAvailable bool
		CheckOnly       bool
	}
	if err := json.Unmarshal([]byte(stdout), &res); err != nil {
		t.Fatal(err)
	}
	if res.Target != "0.2.0" || !res.UpdateAvailable || !res.CheckOnly {
		t.Errorf("result = %+v", res)
	}
}

func TestSelfUpdateUpToDate(t *testing.T) {
	for _, exe := range []string{
		"/usr/local/bin/openarch",
		"/opt/homebrew/Caskroom/openarch/0.2.0/openarch",
		`C:\Users\me\scoop\apps\openarch\current\openarch.exe`,
	} {
		code, stdout, stderr := runSelfUpdate(t, "0.2.0", exe)
		if code != 0 || !strings.Contains(stdout, "up to date") {
			t.Errorf("%s: exit %d, stdout %q, stderr %q", exe, code, stdout, stderr)
		}
	}
}

func TestSelfUpdateRefuses(t *testing.T) {
	cases := []struct {
		name, current, exe, want string
	}{
		{"build from source", "v0.1.0-3-gabc1234", "/usr/local/bin/openarch", "--force"},
		{"homebrew", "0.1.0", "/opt/homebrew/Cellar/openarch/0.1.0/bin/openarch", "brew upgrade openarch"},
		{"scoop", "0.1.0", `C:\Users\me\scoop\apps\openarch\current\openarch.exe`, "scoop update openarch"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			code, _, stderr := runSelfUpdate(t, c.current, c.exe)
			if output.ExitCode(code) != output.ExitError || !strings.Contains(stderr, c.want) {
				t.Errorf("exit %d, stderr %q", code, stderr)
			}
		})
	}
}
