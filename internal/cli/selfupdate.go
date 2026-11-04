package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/AlfrescoLabs/openarch-cli/internal/output"
	"github.com/AlfrescoLabs/openarch-cli/internal/selfupdate"
	"github.com/AlfrescoLabs/openarch-cli/internal/version"
)

// selfUpdateResult is the result of self-update.
type selfUpdateResult struct {
	selfupdate.Plan
	Updated bool `json:"updated"`
	Checked bool `json:"checkOnly,omitempty"`
}

func (r selfUpdateResult) WriteText(w io.Writer) error {
	var err error
	switch {
	case r.Updated:
		_, err = fmt.Fprintf(w, "Updated %s from %s to %s.\n", r.Executable, r.Current, r.Target)
	case r.Checked && r.UpdateAvailable:
		_, err = fmt.Fprintf(w, "openarch %s is available (this is %s). Run 'openarch self-update' to install it.\n", r.Target, r.Current)
	case r.Checked && !selfupdate.IsRelease(r.Current):
		_, err = fmt.Fprintf(w, "The latest release is %s. This is a build from source (%s).\n", r.Target, r.Current)
	default:
		_, err = fmt.Fprintf(w, "openarch %s is up to date.\n", r.Current)
	}
	return err
}

func newSelfUpdateCmd(app *App) *cobra.Command {
	var check, force bool
	var target string
	cmd := &cobra.Command{
		Use:   "self-update",
		Short: "Replace this binary with the latest release",
		Long: `Download the latest release (or --version) from GitHub and replace this binary.

Before anything changes, self-update checks the keyless Sigstore signature of
checksums.txt (made by the release workflow of AlfrescoLabs/openarch-cli for
that tag), then the SHA-256 of the archive. If a check fails, nothing changes.
The signature check needs network access to GitHub and to Sigstore.

--check only reports if a newer release exists.

A binary installed by Homebrew or Scoop is not replaced: use 'brew upgrade
openarch' or 'scoop update openarch'. A build from source is not replaced
either. --force replaces both.`,
		Example: `  openarch self-update --check
  openarch self-update
  openarch self-update --version 0.2.0`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			env := app.selfUpdateEnv()
			current := version.Version
			plan, err := selfupdate.Prepare(cmd.Context(), env, current, target)
			if err != nil {
				return err
			}
			selfupdate.RemoveOld(plan.Executable)
			res := selfUpdateResult{Plan: plan}
			if check {
				res.Checked = true
				return app.Printer.Print(res)
			}
			if !force {
				if pm := selfupdate.PackageManager(plan.Executable); pm != "" {
					hint := "Run: brew upgrade openarch"
					if pm == "scoop" {
						hint = "Run: scoop update openarch"
					}
					return output.NewError(output.ExitError, plan.Executable+" was installed by "+pm, hint)
				}
				if !selfupdate.IsRelease(current) {
					return output.NewError(output.ExitError, "this is a build from source ("+current+"), not a release",
						"Use --force to replace it with release "+plan.Target+".")
				}
			}
			needed := plan.UpdateAvailable || (target != "" && plan.Target != current)
			if !needed && !force {
				return app.Printer.Print(res)
			}
			app.Log.Info("downloading", "asset", plan.Asset)
			bin, err := selfupdate.Download(cmd.Context(), env, plan)
			if err != nil {
				return err
			}
			if err := selfupdate.Install(plan.Executable, bin, env.GOOS); err != nil {
				return err
			}
			res.Updated = true
			return app.Printer.Print(res)
		},
	}
	cmd.Flags().BoolVar(&check, "check", false, "only report if a newer release exists")
	cmd.Flags().StringVar(&target, "version", "", "install this release, for example 0.2.0 (default: the latest)")
	cmd.Flags().BoolVar(&force, "force", false, "also replace a build from source or a package manager install")
	return cmd
}
