package contexts

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/AlfrescoLabs/openarch-cli/internal/output"
)

func TestDir(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	if d, _ := Dir(env(map[string]string{EnvConfigDir: "/x", "XDG_CONFIG_HOME": "/y"})); d != "/x" {
		t.Errorf("Dir = %s", d)
	}
	if d, _ := Dir(env(map[string]string{"XDG_CONFIG_HOME": "/y"})); d != filepath.Join("/y", "openarch") {
		t.Errorf("Dir = %s", d)
	}
	if runtime.GOOS != "windows" {
		home, _ := os.UserHomeDir()
		if d, _ := Dir(env(nil)); d != filepath.Join(home, ".config", "openarch") {
			t.Errorf("Dir = %s", d)
		}
	}
}

func TestRoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cfg")
	c, err := Load(dir)
	if err != nil || len(c.Contexts) != 0 {
		t.Fatalf("Load of missing file = %+v, %v", c, err)
	}
	c = Config{CurrentContext: "prod", Contexts: map[string]Context{
		"prod":  {Endpoints: map[string]Endpoint{SystemOpenArch: {URL: "https://a", Username: "admin", Insecure: true}}},
		"local": {Workspace: "/ws"},
	}}
	if err := Save(dir, c); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.CurrentContext != "prod" || got.Contexts["prod"].Endpoints[SystemOpenArch].Username != "admin" || got.Contexts["local"].Kind() != "workspace" {
		t.Errorf("Load = %+v", got)
	}
	if got.NameForWorkspace("/ws") != "local" || got.NameForWorkspace("/other") != "" {
		t.Error("NameForWorkspace")
	}
	if n := got.Names(); len(n) != 2 || n[0] != "local" {
		t.Errorf("Names = %v", n)
	}
	if _, err := got.Get("nope"); output.CodeOf(err) != output.ExitUsage {
		t.Errorf("Get(nope) = %v", err)
	}
}

func TestLoadRejectsUnknownKeys(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(File(dir), []byte("current-context: a\npassword: x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); output.CodeOf(err) != output.ExitUsage {
		t.Errorf("Load = %v", err)
	}
}

func TestValidateName(t *testing.T) {
	for _, ok := range []string{"local", "prod-1", "a.b_c"} {
		if ValidateName(ok) != nil {
			t.Errorf("%q rejected", ok)
		}
	}
	for _, bad := range []string{"", "-x", "a b", "a/b"} {
		if ValidateName(bad) == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}
