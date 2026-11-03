package stack

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/AlfrescoLabs/openarch-cli/internal/compose"
	"github.com/AlfrescoLabs/openarch-cli/internal/output"
	"github.com/AlfrescoLabs/openarch-cli/internal/workspace"
)

// backupFake answers ps with a running stack and exec with fixed data, and records the calls.
type backupFake struct {
	calls []string
	read  map[string]int // bytes read from stdin per service
}

func (f *backupFake) exec(ctx context.Context, c compose.Command) error {
	args := strings.Join(c.Args, " ")
	f.calls = append(f.calls, args)
	switch {
	case slices.Contains(c.Args, "ps"):
		return (&fakeCompose{ps: healthyContainers()}).exec(ctx, c)
	case c.Stdin != nil:
		data, _ := io.ReadAll(c.Stdin)
		if f.read == nil {
			f.read = map[string]int{}
		}
		for _, s := range []string{"mongodb", "openarch-app", "opensearch"} {
			if slices.Contains(c.Args, s) {
				f.read[s] = len(data)
			}
		}
	case slices.Contains(c.Args, "exec"):
		_, err := io.WriteString(c.Stdout, "data of "+c.Args[len(c.Args)-4])
		return err
	}
	return nil
}

func TestBackupAndRestore(t *testing.T) {
	w := newWorkspace(t, workspace.Default("ws"))
	f := &backupFake{}
	e := Env{Compose: f.exec, Doctor: fakeDoctor(true), CheckAPI: func(context.Context, string, workspace.Config) APICheck { return APICheck{OK: true} }}
	res, err := Backup(context.Background(), e, w, filepath.Join(w.Dir, BackupsDir), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Manifest.Files) != 3 || res.Manifest.OpenArchVersion != workspace.DefaultVersion {
		t.Errorf("manifest = %+v", res.Manifest)
	}
	for _, name := range []string{MongoFile, FilesFile, OpenSearchFile, ManifestFile} {
		info, err := os.Stat(filepath.Join(res.Path, name))
		if err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
			t.Errorf("%s mode = %v", name, info.Mode().Perm())
		}
	}
	// The scripts read the passwords from the container secrets, never from the CLI.
	all := strings.Join(f.calls, "\n")
	if !strings.Contains(all, "/run/secrets/mongodb_root_password") || !strings.Contains(all, "/run/secrets/opensearch_app_password") ||
		strings.Contains(all, w.Secrets.MongoRootPassword) || strings.Contains(all, w.Secrets.OpenSearchAppPassword) {
		t.Errorf("backup commands:\n%s", all)
	}

	f.calls = nil
	if _, err := Restore(context.Background(), e, w, res.Path, UpOptions{Timeout: 60e9, Progress: io.Discard}); err != nil {
		t.Fatal(err)
	}
	// OpenArch stops first, mongodb-init runs on its own, and the stack starts at the end.
	order := []string{"stop openarch-app", "--wait --wait-timeout 60 mongodb opensearch", "--no-deps mongodb-init", "wait mongodb-init", "mongorestore", "run --rm --no-deps", "_restore", "up --detach --remove-orphans --wait"}
	i := 0
	for _, c := range f.calls {
		if i < len(order) && strings.Contains(c, order[i]) {
			i++
		}
	}
	if i != len(order) {
		t.Errorf("restore calls out of order (matched %d of %d):\n%s", i, len(order), strings.Join(f.calls, "\n"))
	}
	for _, s := range []string{"mongodb", "openarch-app", "opensearch"} {
		if f.read[s] == 0 {
			t.Errorf("restore sent no data to %s", s)
		}
	}
}

func TestBackupNeedsRunningStack(t *testing.T) {
	w := newWorkspace(t, workspace.Default("ws"))
	_, err := Backup(context.Background(), env(&fakeCompose{}, true), w, t.TempDir(), io.Discard)
	if output.CodeOf(err) != output.ExitUnhealthy {
		t.Errorf("Backup = %v", err)
	}
}

func TestReadManifestChecksFiles(t *testing.T) {
	w := newWorkspace(t, workspace.Default("ws"))
	f := &backupFake{}
	res, err := Backup(context.Background(), Env{Compose: f.exec}, w, t.TempDir(), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ReadManifest(res.Path); err != nil {
		t.Fatalf("ReadManifest = %v", err)
	}
	if err := os.WriteFile(filepath.Join(res.Path, FilesFile), []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadManifest(res.Path); err == nil || !strings.Contains(err.Error(), "damaged") {
		t.Errorf("damaged file: %v", err)
	}
	if err := os.Remove(filepath.Join(res.Path, MongoFile)); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadManifest(res.Path); output.CodeOf(err) != output.ExitUsage {
		t.Errorf("missing file: %v", err)
	}
	if _, err := ReadManifest(t.TempDir()); output.CodeOf(err) != output.ExitUsage {
		t.Errorf("not a backup: %v", err)
	}
}
