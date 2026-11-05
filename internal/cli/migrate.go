package cli

import (
	"strconv"

	"github.com/spf13/cobra"

	"github.com/AlfrescoLabs/openarch-cli/internal/generate"
	"github.com/AlfrescoLabs/openarch-cli/internal/workspace"
)

func newMigrateCmd(app *App) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Update openarch.yaml to the schema of this openarch",
		Long: `Update openarch.yaml to the current schema (schemaVersion ` + strconv.Itoa(workspace.SchemaVersion) + `).

Commands read an older openarch.yaml and print a warning. 'render' and
'upgrade' save the file with the current schema too. Only openarch.yaml
changes: the other workspace files do not depend on the schema.

A file written by a newer openarch gives exit code 3. Update openarch with
'openarch self-update'.`,
		Example: `  openarch migrate --dry-run
  openarch migrate --dir my-openarch
  openarch migrate --output json`,
		Args: noArgs,
		RunE: func(*cobra.Command, []string) error {
			dir, err := workspace.Find(app.Opts.Dir)
			if err != nil {
				return err
			}
			res, err := generate.Migrate(dir, dryRun)
			if err != nil {
				return err
			}
			return app.Printer.Print(res)
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show the change as a diff, and write nothing")
	return cmd
}
