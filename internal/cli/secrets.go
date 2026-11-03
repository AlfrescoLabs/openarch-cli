package cli

import (
	"errors"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/AlfrescoLabs/openarch-cli/internal/output"
	"github.com/AlfrescoLabs/openarch-cli/internal/stack"
)

// rotatePlan is the result of 'secrets rotate --dry-run'.
type rotatePlan struct {
	DryRun  bool               `json:"dryRun"`
	Rotate  []string           `json:"rotate"`
	Skipped []stack.RotateSkip `json:"skipped,omitempty"`
}

func (p rotatePlan) WriteText(w io.Writer) error {
	var b strings.Builder
	b.WriteString("Dry run. New passwords would be set for: " + strings.Join(p.Rotate, ", ") + "\n")
	for _, s := range p.Skipped {
		b.WriteString("Skipped " + s.Target + ": " + s.Reason + "\n")
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func newSecretsCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "secrets",
		Short: "Manage the passwords of a workspace",
		Args:  noArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	var only []string
	var dry bool
	opts := stack.UpOptions{Wait: true}
	rotate := &cobra.Command{
		Use:   "rotate",
		Short: "Give new passwords to the users of OpenArch, MongoDB and OpenSearch",
		Long: `Give new random passwords to the workspace users:
  mongodb-root   the MongoDB root user
  mongodb-app    the MongoDB user of OpenArch
  opensearch     the OpenSearch users admin, openarch and kibanaserver
  admin          the OpenArch admin

The MongoDB root and OpenSearch passwords change in the running services, so
the stack must run. Each change is saved in .env.local right away. Then the
workspace is rendered, and the services that use a new password are
recreated. Workspace contexts use the new passwords with no extra step.

With --insecure-backends only admin is rotated. Passwords are never printed.`,
		Example: `  openarch secrets rotate
  openarch secrets rotate --only admin,mongodb-app
  openarch secrets rotate --dry-run --output json`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			w, err := app.loadWorkspace()
			if err != nil {
				return err
			}
			if dry {
				do, skip, err := stack.RotatePlan(w, only)
				if err != nil {
					return err
				}
				return app.Printer.Print(rotatePlan{DryRun: true, Rotate: do, Skipped: skip})
			}
			opts.Progress = app.progress()
			res, err := stack.Rotate(cmd.Context(), app.stackEnv(), app.generateEnv(), w, only, opts)
			var oerr *output.Error
			if errors.As(err, &oerr) && oerr.Code == output.ExitUnhealthy && len(res.Rotated) > 0 {
				if perr := app.Printer.Print(res); perr != nil {
					return perr
				}
				app.Printer.Warn(oerr.Message + ". " + oerr.Hint)
				return output.Exit(output.ExitUnhealthy, oerr.Message)
			}
			if err != nil {
				if len(res.Rotated) > 0 {
					app.Printer.Warn("These passwords were changed and saved before the error: " + strings.Join(res.Rotated, ", ") + ".")
				}
				return err
			}
			return app.Printer.Print(res)
		},
	}
	rotate.Flags().StringSliceVar(&only, "only", nil, "rotate only these: "+strings.Join(stack.RotateTargets, ", "))
	rotate.Flags().BoolVar(&dry, "dry-run", false, "show what would be rotated, and change nothing")
	rotate.Flags().DurationVar(&opts.Timeout, "timeout", DefaultUpTimeout, "how long to wait until the stack is healthy")
	cmd.AddCommand(rotate)
	return cmd
}
