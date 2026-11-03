package cli

import (
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/AlfrescoLabs/openarch-cli/internal/output"
	"github.com/AlfrescoLabs/openarch-cli/internal/smoke"
	"github.com/AlfrescoLabs/openarch-cli/internal/stack"
)

// UpgradeResult is the outcome of 'upgrade'.
type UpgradeResult struct {
	From      string        `json:"from"`
	To        string        `json:"to"`
	Status    stack.Status  `json:"status"`
	SmokeTest *smoke.Report `json:"smokeTest,omitempty"`
}

func (r UpgradeResult) WriteText(w io.Writer) error {
	if _, err := fmt.Fprintf(w, "Upgraded OpenArch from %s to %s.\n\n", r.From, r.To); err != nil {
		return err
	}
	if err := r.Status.WriteText(w); err != nil {
		return err
	}
	if r.SmokeTest == nil {
		return nil
	}
	if _, err := io.WriteString(w, "\n"); err != nil {
		return err
	}
	return r.SmokeTest.WriteText(w)
}

func newUpgradeCmd(app *App) *cobra.Command {
	var version string
	var dry, skipSmoke bool
	opts := stack.UpOptions{Wait: true}
	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Change the OpenArch version and check the result",
		Long: `Change the OpenArch version of the workspace. upgrade pulls the new image,
writes the version into openarch.yaml, renders the workspace, starts the stack,
waits until it is healthy, and runs the smoke test.

The pull comes first: when the version does not exist, nothing changes.
There is no automatic rollback. To go back, run upgrade with the old version,
or restore a backup made before the upgrade ('openarch backup').

Exit code 4 means the stack did not become healthy, or the smoke test failed.`,
		Example: `  openarch upgrade --version 1.0.90
  openarch upgrade --version latest --dry-run
  openarch upgrade --version 1.0.90 --output json`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if version == "" {
				return output.Usage("--version is required", "Use an image tag such as 1.0.90, or latest.")
			}
			w, err := app.loadWorkspace()
			if err != nil {
				return err
			}
			plan, err := stack.PlanUpgrade(w, version, !skipSmoke)
			if err != nil {
				return err
			}
			if plan.Downgrade {
				app.Printer.Warn(fmt.Sprintf("%s is older than %s. OpenArch may not read data written by a newer version.", plan.To, plan.From))
			}
			if dry {
				return app.Printer.Print(plan)
			}
			opts.Progress = app.progress()
			res := UpgradeResult{From: plan.From, To: plan.To}
			st, err := stack.Upgrade(cmd.Context(), app.stackEnv(), app.generateEnv(), w, version, opts)
			res.Status = st
			var oerr *output.Error
			if errors.As(err, &oerr) && oerr.Code == output.ExitUnhealthy {
				if perr := app.Printer.Print(res); perr != nil {
					return perr
				}
				app.Printer.Warn(oerr.Message + ". " + oerr.Hint)
				return output.Exit(output.ExitUnhealthy, oerr.Message)
			}
			if err != nil {
				return err
			}
			if !skipSmoke {
				var f connFlags
				c, err := app.client(cmd, &f)
				if err != nil {
					return err
				}
				report := smoke.Run(cmd.Context(), c, smoke.Options{URL: c.URL(), IndexTimeout: smokeIndexTimeout})
				res.SmokeTest = &report
				if err := app.Printer.Print(res); err != nil {
					return err
				}
				if !report.Passed() {
					return output.Exit(output.ExitUnhealthy, "smoke test failed after the upgrade")
				}
				return nil
			}
			return app.Printer.Print(res)
		},
	}
	fl := cmd.Flags()
	fl.StringVar(&version, "version", "", "OpenArch image tag to run")
	fl.BoolVar(&dry, "dry-run", false, "show the plan, and change nothing")
	fl.BoolVar(&skipSmoke, "skip-smoke-test", false, "do not run the smoke test after the upgrade")
	fl.BoolVar(&opts.SkipDoctor, "skip-doctor", false, "do not run the doctor checks first")
	fl.DurationVar(&opts.Timeout, "timeout", DefaultUpTimeout, "how long to wait until the stack is healthy")
	return cmd
}
