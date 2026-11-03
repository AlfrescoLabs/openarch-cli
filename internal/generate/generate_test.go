package generate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AlfrescoLabs/openarch-cli/internal/certs"
	"github.com/AlfrescoLabs/openarch-cli/internal/localenv"
	"github.com/AlfrescoLabs/openarch-cli/internal/output"
	"github.com/AlfrescoLabs/openarch-cli/internal/workspace"
)

var now = time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)

func fakeEnv(memory int64) Env {
	return Env{
		HostMemory: func(context.Context) (int64, error) {
			if memory == 0 {
				return 0, errors.New("no docker")
			}
			return memory, nil
		},
		Now: func() time.Time { return now },
	}
}

func initWorkspace(t *testing.T, cfg workspace.Config, password string) (string, Result) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "ws")
	res, err := Init(context.Background(), fakeEnv(8<<30), InitOptions{Dir: dir, Config: cfg, AdminPassword: password})
	if err != nil {
		t.Fatal(err)
	}
	return dir, res
}

func actions(r Result) map[string]string {
	m := map[string]string{}
	for _, f := range r.Files {
		m[f.Path] = f.Action
	}
	return m
}

func readSecrets(t *testing.T, dir string) localenv.Env {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, localenv.File))
	if err != nil {
		t.Fatal(err)
	}
	return localenv.Parse(data)
}

func TestInitCreatesWorkspace(t *testing.T) {
	dir, res := initWorkspace(t, workspace.Default("ws"), "")
	for _, f := range res.Files {
		if f.Action != Create {
			t.Errorf("%s: action %s", f.Path, f.Action)
		}
	}
	info, err := os.Stat(filepath.Join(dir, localenv.File))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 && os.PathSeparator == '/' {
		t.Errorf(".env.local mode = %v", info.Mode().Perm())
	}
	s := readSecrets(t, dir)
	if s.AdminUser != "admin" || len(s.AdminPassword) != PasswordLength {
		t.Errorf("secrets = %+v", s)
	}
	users, _ := os.ReadFile(filepath.Join(dir, "config", "users.yml"))
	if strings.Contains(string(users), s.AdminPassword) || !strings.Contains(string(users), "{bcrypt}") {
		t.Error("users.yml must hold a hash, not the password")
	}
	if res.Sizing.OpenArchHeap != "2g" {
		t.Errorf("sizing = %+v, want the 8g budget", res.Sizing)
	}
}

func TestInitRefusesExistingWorkspace(t *testing.T) {
	dir, _ := initWorkspace(t, workspace.Default("ws"), "")
	_, err := Init(context.Background(), fakeEnv(8<<30), InitOptions{Dir: dir, Config: workspace.Default("ws")})
	if output.CodeOf(err) != output.ExitUsage || !strings.Contains(err.Error(), "already has openarch.yaml") {
		t.Errorf("Init = %v", err)
	}
}

func TestInitForceKeepsPassword(t *testing.T) {
	dir, _ := initWorkspace(t, workspace.Default("ws"), "first-password")
	cfg := workspace.Default("ws")
	cfg.Port = 9090
	res, err := Init(context.Background(), fakeEnv(8<<30), InitOptions{Dir: dir, Config: cfg, Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if s := readSecrets(t, dir); s.AdminPassword != "first-password" {
		t.Errorf("password = %q", s.AdminPassword)
	}
	a := actions(res)
	if a["openarch.yaml"] != Update || a["config/users.yml"] != Unchanged || a[localenv.File] != Unchanged {
		t.Errorf("actions = %v", a)
	}
}

func TestInitDryRunWritesNothing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ws")
	cfg := workspace.Default("ws")
	cfg.Storage = workspace.Storage{Mode: workspace.StorageBind, DataDir: "./data"}
	res, err := Init(context.Background(), fakeEnv(8<<30), InitOptions{Dir: dir, Config: cfg, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("dry run created %s", dir)
	}
	if len(res.Files) == 0 || len(res.Folders) != 3 {
		t.Errorf("result = %+v", res)
	}
}

func TestInitBindCreatesFolders(t *testing.T) {
	cfg := workspace.Default("ws")
	cfg.Storage = workspace.Storage{Mode: workspace.StorageBind, DataDir: "./data"}
	dir, _ := initWorkspace(t, cfg, "")
	for _, sub := range []string{"mongodb", "opensearch", "openarch"} {
		if info, err := os.Stat(filepath.Join(dir, "data", sub)); err != nil || !info.IsDir() {
			t.Errorf("data/%s: %v", sub, err)
		}
	}
}

func TestMemoryFallbackWarns(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ws")
	res, err := Init(context.Background(), fakeEnv(0), InitOptions{Dir: dir, Config: workspace.Default("ws"), DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Sizing.Budget != workspace.FallbackMemory || !strings.Contains(strings.Join(res.Warnings, " "), "did not report its memory") {
		t.Errorf("sizing = %+v, warnings = %v", res.Sizing, res.Warnings)
	}
}

func TestRenderIsStable(t *testing.T) {
	cfg := workspace.Default("ws")
	cfg.TLS.Mode, cfg.Port = workspace.TLSSelfSigned, 8443
	dir, _ := initWorkspace(t, cfg, "")
	res, err := Render(context.Background(), fakeEnv(8<<30), RenderOptions{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range res.Files {
		if f.Action != Unchanged {
			t.Errorf("%s: action %s, want unchanged", f.Path, f.Action)
		}
	}
}

func TestRenderAppliesChanges(t *testing.T) {
	cfg := workspace.Default("ws")
	cfg.TLS.Mode, cfg.Port = workspace.TLSSelfSigned, 8443
	dir, _ := initWorkspace(t, cfg, "")

	cfg.TLS.Mode, cfg.Port = workspace.TLSNone, 8080
	if err := os.WriteFile(filepath.Join(dir, workspace.ConfigFile), cfg.Marshal(), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Render(context.Background(), fakeEnv(8<<30), RenderOptions{Dir: dir, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	var diff string
	for _, f := range res.Files {
		if f.Path == "compose.yaml" {
			diff = f.Diff
		}
	}
	if a := actions(res); a["compose.yaml"] != Update || a["config/Caddyfile"] != Delete {
		t.Errorf("actions = %v", a)
	}
	if !strings.Contains(diff, "-  proxy:") {
		t.Errorf("diff does not remove the proxy:\n%s", diff)
	}
	if _, err := Render(context.Background(), fakeEnv(8<<30), RenderOptions{Dir: dir}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "config", "Caddyfile")); !errors.Is(err, os.ErrNotExist) {
		t.Error("Caddyfile was not deleted")
	}
}

func TestRenderNewPassword(t *testing.T) {
	dir, _ := initWorkspace(t, workspace.Default("ws"), "old-password")
	s := localenv.Env{AdminUser: "admin", AdminPassword: "new-password"}
	if err := os.WriteFile(filepath.Join(dir, localenv.File), s.Marshal(), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := Render(context.Background(), fakeEnv(8<<30), RenderOptions{Dir: dir, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if actions(res)["config/users.yml"] != Update {
		t.Errorf("actions = %v", actions(res))
	}
	for _, f := range res.Files {
		if strings.Contains(f.Diff, "new-password") {
			t.Errorf("%s diff shows the password", f.Path)
		}
	}
}

func TestRenderWithoutSecrets(t *testing.T) {
	dir, _ := initWorkspace(t, workspace.Default("ws"), "")
	if err := os.Remove(filepath.Join(dir, localenv.File)); err != nil {
		t.Fatal(err)
	}
	_, err := Render(context.Background(), fakeEnv(8<<30), RenderOptions{Dir: dir})
	if output.CodeOf(err) != output.ExitUsage || !strings.Contains(err.Error(), ".env.local not found") {
		t.Errorf("Render = %v", err)
	}
}

func writeCustomCert(t *testing.T, dir, serverName string, matching bool) (string, string) {
	t.Helper()
	certPEM, keyPEM, err := certs.SelfSigned(serverName, now)
	if err != nil {
		t.Fatal(err)
	}
	if !matching {
		_, keyPEM, _ = certs.SelfSigned(serverName, now)
	}
	certFile, keyFile := filepath.Join(dir, "c.pem"), filepath.Join(dir, "k.pem")
	if err := os.WriteFile(certFile, certPEM, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, keyPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	return certFile, keyFile
}

func TestCustomTLS(t *testing.T) {
	tmp := t.TempDir()
	cfg := workspace.Default("ws")
	cfg.TLS.Mode, cfg.Port, cfg.ServerName = workspace.TLSCustom, 8443, "oa.example.com"

	cfg.TLS.Cert, cfg.TLS.Key = writeCustomCert(t, tmp, "oa.example.com", false)
	_, err := Init(context.Background(), fakeEnv(8<<30), InitOptions{Dir: filepath.Join(tmp, "a"), Config: cfg})
	if output.CodeOf(err) != output.ExitUsage || !strings.Contains(err.Error(), "does not match") {
		t.Errorf("mismatched key: %v", err)
	}

	cfg.TLS.Cert, cfg.TLS.Key = writeCustomCert(t, tmp, "other.example.com", true)
	res, err := Init(context.Background(), fakeEnv(8<<30), InitOptions{Dir: filepath.Join(tmp, "b"), Config: cfg})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(res.Warnings, " "), "not valid for oa.example.com") {
		t.Errorf("warnings = %v", res.Warnings)
	}
	if _, ok := actions(res)["certs/key.pem"]; ok {
		t.Error("custom TLS must not write certs/")
	}
}
