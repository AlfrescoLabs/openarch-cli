package generate

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/rogpeppe/go-internal/diff"

	"github.com/AlfrescoLabs/openarch-cli/internal/output"
	"github.com/AlfrescoLabs/openarch-cli/internal/workspace"
)

// MigrateResult is the outcome of Migrate.
type MigrateResult struct {
	Workspace string `json:"workspace"`
	DryRun    bool   `json:"dryRun"`
	From      int    `json:"from"`
	To        int    `json:"to"`
	// Action is Update, or Unchanged when the file already has the current schema.
	Action string `json:"action"`
	// Diff is set with dry run when the file changes.
	Diff string `json:"diff,omitempty"`
}

// Migrate rewrites openarch.yaml in dir with the current schemaVersion. The
// other workspace files do not depend on the schema, so they stay as they are.
func Migrate(dir string, dryRun bool) (MigrateResult, error) {
	path := filepath.Join(dir, workspace.ConfigFile)
	r := MigrateResult{Workspace: dir, DryRun: dryRun, To: workspace.SchemaVersion}
	old, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return r, output.Usage(path+" not found", "Run 'openarch init' to create a workspace, or use --dir.")
	}
	if err != nil {
		return r, err
	}
	// Parse checks the version and the keys before anything is written.
	cfg, err := workspace.Parse(old, path)
	if err != nil {
		return r, err
	}
	if r.From, err = workspace.FileSchemaVersion(old); err != nil {
		return r, err
	}
	if r.From == workspace.SchemaVersion {
		r.Action = Unchanged
		return r, nil
	}
	data := cfg.Marshal()
	r.Action = Update
	if dryRun {
		r.Diff = string(diff.Diff("a/"+workspace.ConfigFile, old, "b/"+workspace.ConfigFile, data))
		return r, nil
	}
	return r, writeFile(path, data, 0o644)
}

// WriteText writes the human-readable form.
func (r MigrateResult) WriteText(w io.Writer) error {
	var err error
	switch {
	case r.Action == Unchanged:
		_, err = fmt.Fprintf(w, "%s already has schemaVersion %d.\n", workspace.ConfigFile, r.To)
	case r.DryRun:
		_, err = fmt.Fprintf(w, "Dry run: %s would change from schemaVersion %d to %d.\n\n%s", workspace.ConfigFile, r.From, r.To, r.Diff)
	default:
		_, err = fmt.Fprintf(w, "Migrated %s from schemaVersion %d to %d.\n", filepath.Join(r.Workspace, workspace.ConfigFile), r.From, r.To)
	}
	return err
}
