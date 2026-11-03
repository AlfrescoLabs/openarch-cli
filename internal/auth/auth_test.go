package auth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlfrescoLabs/openarch-cli/internal/localenv"
	"github.com/AlfrescoLabs/openarch-cli/internal/output"
	"github.com/AlfrescoLabs/openarch-cli/internal/workspace"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func testWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cfg := workspace.Default("ws")
	cfg.Port = 18080
	s := localenv.Env{AdminUser: "admin", AdminPassword: "ws-password"}
	if err := os.WriteFile(filepath.Join(dir, workspace.ConfigFile), cfg.Marshal(), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, localenv.File), s.Marshal(), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestWorkspaceCredentials(t *testing.T) {
	c, err := Resolve(Input{WorkspaceDir: testWorkspace(t), Getenv: env(nil)})
	if err != nil {
		t.Fatal(err)
	}
	if c.URL != "http://localhost:18080" || c.User != "admin" || c.Password != "ws-password" || c.DialAddr != "127.0.0.1:18080" {
		t.Errorf("credentials = %+v", c)
	}
	if c.Sources != (Sources{URL: SourceWorkspace, User: SourceWorkspace, Password: SourceWorkspace}) {
		t.Errorf("sources = %+v", c.Sources)
	}
}

func TestOrder(t *testing.T) {
	dir := testWorkspace(t)
	e := env(map[string]string{EnvUser: "env-user", EnvPassword: "env-pw"})
	c, err := Resolve(Input{WorkspaceDir: dir, Getenv: e, User: "flag-user"})
	if err != nil {
		t.Fatal(err)
	}
	// The URL still comes from the workspace, but the user and password do not.
	if c.User != "flag-user" || c.Password != "env-pw" || c.Sources.User != SourceFlag || c.Sources.Password != SourceEnv {
		t.Errorf("credentials = %+v", c)
	}
	c, err = Resolve(Input{WorkspaceDir: dir, Getenv: e, PasswordStdin: true, Stdin: strings.NewReader("stdin-pw\n")})
	if err != nil || c.Password != "stdin-pw" || c.User != "env-user" {
		t.Errorf("credentials = %+v, %v", c, err)
	}
}

// Workspace credentials must never go to a URL that did not come from the workspace.
func TestWorkspaceSecretsOnlyForWorkspaceURL(t *testing.T) {
	_, err := Resolve(Input{WorkspaceDir: testWorkspace(t), Getenv: env(nil), URL: "https://other.example.com"})
	if output.CodeOf(err) != output.ExitAuth {
		t.Errorf("Resolve = %v, want an auth error", err)
	}
	_, err = Resolve(Input{WorkspaceDir: testWorkspace(t), Getenv: env(nil), User: "someone-else"})
	if output.CodeOf(err) != output.ExitAuth || !strings.Contains(err.Error(), "no password") {
		t.Errorf("workspace password used for another user: %v", err)
	}
}

func TestPrompt(t *testing.T) {
	var asked []string
	prompt := func(label string, secret bool) (string, error) {
		asked = append(asked, label)
		if secret {
			return "typed-pw", nil
		}
		return "typed-user", nil
	}
	c, err := Resolve(Input{Getenv: env(map[string]string{EnvURL: "http://x:8080/"}), Prompt: prompt})
	if err != nil {
		t.Fatal(err)
	}
	if c.URL != "http://x:8080" || c.User != "typed-user" || c.Password != "typed-pw" || len(asked) != 2 || c.Sources.Password != SourcePrompt {
		t.Errorf("credentials = %+v, asked = %v", c, asked)
	}
}

func TestErrors(t *testing.T) {
	if _, err := Resolve(Input{Getenv: env(nil), WorkspaceDir: t.TempDir()}); output.CodeOf(err) != output.ExitUsage {
		t.Errorf("no URL: %v", err)
	}
	if _, err := Resolve(Input{Getenv: env(map[string]string{EnvURL: "localhost:8080", EnvUser: "a", EnvPassword: "b"})}); output.CodeOf(err) != output.ExitUsage {
		t.Errorf("URL without scheme: %v", err)
	}
	if _, err := Resolve(Input{Getenv: env(map[string]string{EnvURL: "http://x", EnvUser: "a"})}); output.CodeOf(err) != output.ExitAuth {
		t.Errorf("no password: %v", err)
	}
	if _, err := Resolve(Input{Getenv: env(map[string]string{EnvURL: "http://x", EnvUser: "a", EnvPassword: "b", EnvInsecure: "maybe"})}); output.CodeOf(err) != output.ExitUsage {
		t.Errorf("bad insecure value: %v", err)
	}
	c, err := Resolve(Input{Getenv: env(map[string]string{EnvURL: "http://x", EnvUser: "a", EnvPassword: "b", EnvInsecure: "true"})})
	if err != nil || !c.Insecure {
		t.Errorf("insecure from env: %+v, %v", c, err)
	}
	c, _ = Resolve(Input{Getenv: env(map[string]string{EnvURL: "http://x", EnvUser: "a", EnvPassword: "b", EnvInsecure: "true"}), InsecureSet: true})
	if c.Insecure {
		t.Error("--insecure=false did not win over the env var")
	}
}

func TestDescribeHasNoPassword(t *testing.T) {
	c, err := Resolve(Input{WorkspaceDir: testWorkspace(t), Getenv: env(nil)})
	if err != nil {
		t.Fatal(err)
	}
	if d := c.Describe(); strings.Contains(d, c.Password) || !strings.Contains(d, ".env.local") {
		t.Errorf("Describe = %q", d)
	}
}
