package stack

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/AlfrescoLabs/openarch-cli/internal/compose"
	"github.com/AlfrescoLabs/openarch-cli/internal/localenv"
	"github.com/AlfrescoLabs/openarch-cli/internal/output"
	"github.com/AlfrescoLabs/openarch-cli/internal/workspace"
)

func TestRotatePlan(t *testing.T) {
	cfg := workspace.Default("ws")
	do, skip, err := RotatePlan(Workspace{Config: cfg}, nil)
	if err != nil || !slices.Equal(do, RotateTargets) || len(skip) != 0 {
		t.Errorf("plan = %v, %v, %v", do, skip, err)
	}
	cfg.InsecureBackends, cfg.DefaultUsers = true, true
	do, skip, _ = RotatePlan(Workspace{Config: cfg}, nil)
	if len(do) != 0 || len(skip) != 4 {
		t.Errorf("plan with security off = %v, %v", do, skip)
	}
	if _, _, err := RotatePlan(Workspace{Config: workspace.Default("ws")}, []string{"x"}); output.CodeOf(err) != output.ExitUsage {
		t.Errorf("unknown target: %v", err)
	}
}

func TestRotate(t *testing.T) {
	w := initWorkspace(t)
	old := w.Secrets
	var calls []string
	var stdin []string
	f := &fakeCompose{ps: healthyContainers()}
	e := env(f, true)
	base := e.Compose
	e.Compose = func(ctx context.Context, c compose.Command) error {
		if c.Stdin != nil {
			data, _ := io.ReadAll(c.Stdin)
			stdin = append(stdin, string(data))
		}
		calls = append(calls, strings.Join(c.Args, " "))
		return base(ctx, c)
	}
	res, err := Rotate(context.Background(), e, genEnv(), w, nil, UpOptions{Wait: true, Progress: io.Discard})
	if err != nil || !slices.Equal(res.Rotated, RotateTargets) || !res.Applied {
		t.Fatalf("Rotate = %+v, %v", res, err)
	}
	data, _ := os.ReadFile(filepath.Join(w.Dir, localenv.File))
	s := localenv.Parse(data)
	for name, pair := range map[string][2]string{
		"admin": {old.AdminPassword, s.AdminPassword}, "mongodb root": {old.MongoRootPassword, s.MongoRootPassword},
		"mongodb app": {old.MongoAppPassword, s.MongoAppPassword}, "opensearch admin": {old.OpenSearchAdminPassword, s.OpenSearchAdminPassword},
		"opensearch app": {old.OpenSearchAppPassword, s.OpenSearchAppPassword}, "dashboards": {old.DashboardsPassword, s.DashboardsPassword},
	} {
		if pair[0] == pair[1] || pair[1] == "" {
			t.Errorf("%s password did not change", name)
		}
	}
	// Passwords go on stdin, never in the command arguments.
	all := strings.Join(calls, "\n")
	for _, pw := range []string{s.MongoRootPassword, s.OpenSearchAdminPassword, s.OpenSearchAppPassword, old.OpenSearchAdminPassword} {
		if strings.Contains(all, pw) {
			t.Errorf("a password is in the arguments:\n%s", all)
		}
	}
	// The OpenSearch admin changes itself last, with its old password.
	if len(stdin) != 4 || !strings.Contains(stdin[3], "admin:"+old.OpenSearchAdminPassword) || !strings.Contains(stdin[3], s.OpenSearchAdminPassword) {
		t.Errorf("stdin = %q", stdin)
	}
}

func TestRotateNeedsRunningStack(t *testing.T) {
	w := initWorkspace(t)
	_, err := Rotate(context.Background(), env(&fakeCompose{}, true), genEnv(), w, []string{RotateOpenSearch}, UpOptions{Progress: io.Discard})
	if output.CodeOf(err) != output.ExitUnhealthy {
		t.Errorf("Rotate = %v", err)
	}
	res, err := Rotate(context.Background(), env(&fakeCompose{}, true), genEnv(), w, []string{RotateAdmin}, UpOptions{Progress: io.Discard})
	if err != nil || res.Applied || !slices.Equal(res.Rotated, []string{RotateAdmin}) {
		t.Errorf("admin on a stopped stack = %+v, %v", res, err)
	}
}
