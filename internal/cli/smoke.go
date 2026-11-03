package cli

import (
	"time"

	"github.com/spf13/cobra"

	"github.com/AlfrescoLabs/openarch-cli/internal/output"
	"github.com/AlfrescoLabs/openarch-cli/internal/smoke"
)

func newSmokeTestCmd(app *App) *cobra.Command {
	var f connFlags
	var o smoke.Options
	cmd := &cobra.Command{
		Use:   "smoke-test",
		Short: "Check OpenArch end to end: create, read, query and delete a document",
		Long: `Check OpenArch end to end. The test creates a folder under the root, reads it
by id and by path, finds it with a synchronized query and in the background
search index, deletes it, and checks that it is gone. Each step is reported.

The folder is deleted even when a step fails.

Exit code 0 means every step passed. Exit code 4 means a step failed, and 6
means the credentials were rejected.`,
		Example: `  openarch smoke-test
  openarch smoke-test --context prod --output json`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := app.client(cmd, &f)
			if err != nil {
				return err
			}
			o.URL = c.URL()
			report := smoke.Run(cmd.Context(), c, o)
			if err := app.Printer.Print(report); err != nil {
				return err
			}
			switch {
			case report.AuthFailed():
				return output.Exit(output.ExitAuth, "authentication failed")
			case !report.Passed():
				return output.Exit(output.ExitUnhealthy, "smoke test failed")
			}
			return nil
		},
	}
	f.register(cmd)
	cmd.Flags().DurationVar(&o.IndexTimeout, "index-timeout", 60*time.Second, "how long to wait for the background search index")
	return cmd
}
