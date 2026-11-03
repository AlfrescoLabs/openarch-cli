package templates

import (
	"bytes"
	"context"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AlfrescoLabs/openarch-cli/internal/workspace"
)

var update = flag.Bool("update", false, "rewrite the golden files")

// hash is a fixed bcrypt hash, so the golden files do not change between runs.
const hash = "{bcrypt}$2a$10$7EqJtq98hPqEX7fNZaFWoOhi5BWX4Z3RZbQ0Lq4n3Cf8/7xQ9b7Fy"

func goldenCases() map[string]workspace.Config {
	base := workspace.Default("golden")
	cases := map[string]workspace.Config{"default": base}

	c := base
	c.Storage = workspace.Storage{Mode: workspace.StorageNone}
	cases["storage-none"] = c

	c = base
	c.Storage = workspace.Storage{Mode: workspace.StorageBind, DataDir: "./data"}
	cases["storage-bind"] = c

	c = base
	c.Storage = workspace.Storage{Mode: workspace.StorageBind, DataDir: "/srv/open$arch"}
	cases["storage-bind-absolute"] = c

	c = base
	c.Expose = []string{workspace.ExposeMongoDB, workspace.ExposeOpenSearch, workspace.ExposeDebug}
	c.With = []string{workspace.WithDashboards}
	c.BindIP = "0.0.0.0"
	c.Memory = "16g"
	cases["extras"] = c

	c = base
	c.TLS = workspace.TLS{Mode: workspace.TLSSelfSigned}
	c.Port = 8443
	c.ServerName = "openarch.local"
	cases["tls-self-signed"] = c

	c = base
	c.TLS = workspace.TLS{Mode: workspace.TLSCustom, Cert: "/etc/ssl/openarch.pem", Key: "/etc/ssl/openarch.key"}
	c.Port = 443
	c.ServerName = "openarch.example.com"
	c.Expose = []string{workspace.ExposeDebug}
	cases["tls-custom"] = c
	return cases
}

func render(t *testing.T, cfg workspace.Config) []File {
	t.Helper()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("invalid test config: %v", err)
	}
	budget, _ := workspace.ParseMemory(cfg.Memory)
	if budget == 0 {
		budget = workspace.FallbackMemory
	}
	files, err := Render(Input{Config: cfg, Sizing: workspace.SizingFor(budget), AdminUser: "admin", AdminHash: hash})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestGolden(t *testing.T) {
	for name, cfg := range goldenCases() {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join("testdata", name)
			for _, f := range render(t, cfg) {
				golden := filepath.Join(dir, goldenName(f.Path))
				if *update {
					if err := os.MkdirAll(filepath.Dir(golden), 0o755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(golden, f.Data, 0o644); err != nil {
						t.Fatal(err)
					}
					continue
				}
				want, err := os.ReadFile(golden)
				if err != nil {
					t.Fatalf("%v (run 'go test ./internal/templates -update')", err)
				}
				if !bytes.Equal(f.Data, want) {
					t.Errorf("%s differs from %s (run 'go test ./internal/templates -update' and review the diff)", f.Path, golden)
				}
			}
		})
	}
}

// goldenName maps a workspace path to its golden file. Dot files get a "dot-"
// prefix: a golden .gitignore would act as a real one.
func goldenName(p string) string {
	dir, base := filepath.Split(filepath.FromSlash(p))
	if strings.HasPrefix(base, ".") {
		base = "dot-" + base[1:]
	}
	return filepath.Join(dir, base)
}

// TestComposeConfig checks every golden case with 'docker compose config'.
// It is skipped when Docker Compose is not installed.
func TestComposeConfig(t *testing.T) {
	if err := exec.CommandContext(context.Background(), "docker", "compose", "version").Run(); err != nil {
		t.Skip("docker compose is not available")
	}
	for name, cfg := range goldenCases() {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			for _, f := range render(t, cfg) {
				p := filepath.Join(dir, filepath.FromSlash(f.Path))
				if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, f.Data, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, "docker", "compose", "config", "--quiet")
			cmd.Dir = dir
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Errorf("docker compose config: %v\n%s", err, out)
			}
		})
	}
}

func TestRenderNeedsAdmin(t *testing.T) {
	if _, err := Render(Input{Config: workspace.Default("x"), Sizing: workspace.SizingFor(0)}); err == nil {
		t.Error("Render without admin succeeded")
	}
}

func TestGitignorePattern(t *testing.T) {
	for in, want := range map[string]string{
		"./data":      "/data/",
		"data/db":     "/data/db/",
		"../data":     "",
		"/srv/data":   "",
		".":           "",
		"./x/../data": "/data/",
	} {
		if got := gitignorePattern(in); got != want {
			t.Errorf("gitignorePattern(%q) = %q, want %q", in, got, want)
		}
	}
}
