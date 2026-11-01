package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/AlfrescoLabs/openarch-cli/internal/output"
)

func run(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errw bytes.Buffer
	code = Execute(context.Background(), args, Streams{In: strings.NewReader(""), Out: &out, Err: &errw})
	return code, out.String(), errw.String()
}

func TestExitCodes(t *testing.T) {
	t.Setenv(envOutput, "")
	cases := []struct {
		name string
		args []string
		want output.ExitCode
	}{
		{"no args shows help", nil, output.ExitOK},
		{"help flag", []string{"--help"}, output.ExitOK},
		{"version", []string{"version"}, output.ExitOK},
		{"unknown flag", []string{"--bogus"}, output.ExitUsage},
		{"unknown command", []string{"bogus"}, output.ExitUsage},
		{"extra argument", []string{"version", "extra"}, output.ExitUsage},
		{"invalid output format", []string{"version", "--output", "yaml"}, output.ExitUsage},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			code, _, stderr := run(t, c.args...)
			if output.ExitCode(code) != c.want {
				t.Errorf("exit code = %d, want %d (stderr %q)", code, c.want, stderr)
			}
		})
	}
}

func TestJSONErrorBeforeFlagParsing(t *testing.T) {
	t.Setenv(envOutput, "")
	for _, args := range [][]string{
		{"--output", "json", "--bogus"},
		{"--output=json", "--bogus"},
		{"-ojson", "--bogus"},
		{"--bogus", "-o", "json"},
	} {
		code, stdout, stderr := run(t, args...)
		var got struct {
			Error struct{ Code string } `json:"error"`
		}
		if err := json.Unmarshal([]byte(stdout), &got); err != nil {
			t.Fatalf("%v: stdout is not JSON: %q", args, stdout)
		}
		if code != int(output.ExitUsage) || got.Error.Code != "usage" || stderr != "" {
			t.Errorf("%v: code=%d error.code=%q stderr=%q", args, code, got.Error.Code, stderr)
		}
	}
}

func TestOutputFromEnv(t *testing.T) {
	t.Setenv(envOutput, "json")
	code, stdout, _ := run(t, "version")
	if code != 0 || !json.Valid([]byte(stdout)) {
		t.Errorf("code=%d stdout=%q", code, stdout)
	}
}

func TestFlagOverridesEnv(t *testing.T) {
	t.Setenv(envOutput, "json")
	_, stdout, _ := run(t, "version", "--output", "text")
	if !strings.HasPrefix(stdout, "openarch ") {
		t.Errorf("stdout=%q", stdout)
	}
}

func TestSuggestion(t *testing.T) {
	t.Setenv(envOutput, "")
	_, _, stderr := run(t, "versio")
	if !strings.Contains(stderr, `Did you mean "version"?`) {
		t.Errorf("stderr=%q", stderr)
	}
}

func TestNotInteractiveWithoutTTY(t *testing.T) {
	app := &App{Streams: Streams{In: strings.NewReader(""), Out: &bytes.Buffer{}}}
	if app.Interactive() {
		t.Error("Interactive() = true without a TTY")
	}
}
