package cli

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra/doc"

	"github.com/AlfrescoLabs/openarch-cli/internal/doctor"
)

var updateDocs = flag.Bool("update", false, "rewrite docs/commands")

const docsDir = "../../docs/commands"

// TestCommandDocs checks that docs/commands matches the cobra commands.
// go test ./internal/cli -run TestCommandDocs -update rewrites it.
func TestCommandDocs(t *testing.T) {
	t.Setenv(envOutput, "")
	tmp := t.TempDir()
	app := newApp(Streams{})
	// The doctor help lists the checks for the host OS. Linux has the most checks.
	app.doctorEnv = func() doctor.Env {
		env := doctor.SystemEnv()
		env.GOOS = "linux"
		return env
	}
	root := newRootCmd(app)
	root.DisableAutoGenTag = true
	if err := doc.GenMarkdownTree(root, tmp); err != nil {
		t.Fatal(err)
	}
	if *updateDocs {
		old, _ := filepath.Glob(filepath.Join(docsDir, "*.md"))
		for _, f := range old {
			if err := os.Remove(f); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.MkdirAll(docsDir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	got, _ := filepath.Glob(filepath.Join(tmp, "*.md"))
	for _, f := range got {
		want, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		dst := filepath.Join(docsDir, filepath.Base(f))
		if *updateDocs {
			if err := os.WriteFile(dst, want, 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		have, err := os.ReadFile(dst)
		if err != nil || !bytes.Equal(have, want) {
			t.Errorf("%s is out of date; run: go test ./internal/cli -run TestCommandDocs -update", dst)
		}
	}
	if !*updateDocs {
		have, _ := filepath.Glob(filepath.Join(docsDir, "*.md"))
		if len(have) != len(got) {
			t.Errorf("docs/commands has %d files, want %d; run: go test ./internal/cli -run TestCommandDocs -update", len(have), len(got))
		}
	}
}
