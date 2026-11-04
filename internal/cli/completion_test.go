package cli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/AlfrescoLabs/openarch-cli/internal/contexts"
	"github.com/AlfrescoLabs/openarch-cli/internal/workspace"
)

// complete runs the hidden __complete command and returns the suggestions.
func complete(t *testing.T, args ...string) []string {
	t.Helper()
	code, stdout, stderr := run(t, append([]string{"__complete"}, args...)...)
	if code != 0 {
		t.Fatalf("__complete %v: exit %d, stderr %q", args, code, stderr)
	}
	var got []string
	for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		if line != "" && !strings.HasPrefix(line, ":") {
			got = append(got, strings.SplitN(line, "\t", 2)[0])
		}
	}
	return got
}

func TestCompleteFlagValues(t *testing.T) {
	cases := []struct {
		args []string
		want []string
	}{
		{[]string{"version", "--output", ""}, []string{"text", "json"}},
		{[]string{"init", "--storage", ""}, workspace.StorageModes},
		{[]string{"init", "--tls", ""}, workspace.TLSModes},
		{[]string{"init", "--with", ""}, workspace.WithOptions},
		{[]string{"secrets", "rotate", "--only", ""}, []string{"mongodb-root", "mongodb-app", "opensearch", "admin"}},
	}
	for _, c := range cases {
		if got := complete(t, c.args...); !slices.Equal(got, c.want) {
			t.Errorf("%v: got %v, want %v", c.args, got, c.want)
		}
	}
}

func TestCompleteContexts(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(contexts.EnvConfigDir, dir)
	cfg := contexts.Config{Contexts: map[string]contexts.Context{"prod": {Workspace: "/x"}, "dev": {Workspace: "/y"}}}
	if err := contexts.Save(dir, cfg); err != nil {
		t.Fatal(err)
	}
	want := []string{"dev", "prod"}
	for _, args := range [][]string{
		{"config", "use", ""},
		{"config", "delete", ""},
		{"config", "get", "--name", ""},
		{"status", "--context", ""},
	} {
		if got := complete(t, args...); !slices.Equal(got, want) {
			t.Errorf("%v: got %v, want %v", args, got, want)
		}
	}
	if got := complete(t, "config", "use", "prod", ""); len(got) != 0 {
		t.Errorf("second argument: got %v, want nothing", got)
	}
}

func TestCompleteLogsServices(t *testing.T) {
	dir := t.TempDir()
	cfg := workspace.Default("ws")
	cfg.TLS.Mode = workspace.TLSSelfSigned
	if err := os.WriteFile(filepath.Join(dir, workspace.ConfigFile), cfg.Marshal(), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, want := complete(t, "logs", "--dir", dir, ""), cfg.ServiceNames(); !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	got := complete(t, "logs", "--dir", dir, "proxy", "")
	if slices.Contains(got, "proxy") || !slices.Contains(got, "openarch-app") {
		t.Errorf("after proxy: got %v", got)
	}
	if got := complete(t, "logs", "--dir", t.TempDir(), ""); len(got) != 0 {
		t.Errorf("no workspace: got %v, want nothing", got)
	}
}
