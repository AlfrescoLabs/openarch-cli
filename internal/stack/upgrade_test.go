package stack

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AlfrescoLabs/openarch-cli/internal/compose"
	"github.com/AlfrescoLabs/openarch-cli/internal/generate"
	"github.com/AlfrescoLabs/openarch-cli/internal/output"
	"github.com/AlfrescoLabs/openarch-cli/internal/workspace"
)

func TestCompareVersions(t *testing.T) {
	for _, tt := range []struct {
		a, b string
		want int
	}{
		{"1.0.90", "1.0.55", 1}, {"1.0.9", "1.0.10", -1}, {"1.0", "1.0.0", 0}, {"1.0.90", "1.0.90", 0}, {"latest", "1.0.90", 0},
	} {
		if got := CompareVersions(tt.a, tt.b); got != tt.want {
			t.Errorf("CompareVersions(%s, %s) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestPlanUpgrade(t *testing.T) {
	cfg := workspace.Default("ws")
	cfg.Version = "1.0.90"
	w := Workspace{Dir: "/ws", Config: cfg}
	p, err := PlanUpgrade(w, "1.0.55", true)
	if err != nil || !p.Downgrade || len(p.Steps) != 4 || p.From != "1.0.90" {
		t.Errorf("plan = %+v, %v", p, err)
	}
	if _, err := PlanUpgrade(w, "bad tag!", false); output.CodeOf(err) != output.ExitUsage {
		t.Errorf("bad tag: %v", err)
	}
}

// genEnv returns a generate.Env with no Docker.
func genEnv() generate.Env {
	return generate.Env{HostMemory: func(context.Context) (int64, error) { return 8 << 30, nil }, Now: time.Now}
}

func initWorkspace(t *testing.T) Workspace {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "ws")
	cfg := workspace.Default("ws")
	cfg.Version = "1.0.55"
	if _, err := generate.Init(context.Background(), genEnv(), generate.InitOptions{Dir: dir, Config: cfg}); err != nil {
		t.Fatal(err)
	}
	w, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func TestUpgradePullFailsChangesNothing(t *testing.T) {
	w := initWorkspace(t)
	exec := func(_ context.Context, c compose.Command) error {
		if c.Args[0] == "pull" {
			return errors.New("not found")
		}
		t.Errorf("unexpected call %v", c.Args)
		return nil
	}
	_, err := Upgrade(context.Background(), Env{Compose: exec}, genEnv(), w, "9.9.9", UpOptions{SkipDoctor: true, Progress: &strings.Builder{}})
	if err == nil || !strings.Contains(err.Error(), "cannot pull") {
		t.Errorf("Upgrade = %v", err)
	}
	if after, _ := Load(w.Dir); after.Config.Version != "1.0.55" {
		t.Errorf("version changed to %s", after.Config.Version)
	}
}

func TestUpgrade(t *testing.T) {
	w := initWorkspace(t)
	f := &fakeCompose{ps: healthyContainers()}
	e := env(f, true)
	exec := e.Compose
	var pulled string
	e.Compose = func(ctx context.Context, c compose.Command) error {
		if c.Args[0] == "pull" {
			pulled = c.Args[1]
			return nil
		}
		return exec(ctx, c)
	}
	st, err := Upgrade(context.Background(), e, genEnv(), w, "1.0.90", UpOptions{Wait: true, SkipDoctor: true, Progress: &strings.Builder{}})
	if err != nil || st.State != Healthy || pulled != Image+":1.0.90" {
		t.Fatalf("Upgrade = %s, %v, pulled %q", st.State, err, pulled)
	}
	if after, _ := Load(w.Dir); after.Config.Version != "1.0.90" {
		t.Errorf("version = %s", after.Config.Version)
	}
}
