package cli

import (
	"errors"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/AlfrescoLabs/openarch-cli/internal/output"
	"github.com/AlfrescoLabs/openarch-cli/internal/stack"
)

func newBackupCmd(app *App) *cobra.Command {
	var parent string
	cmd := &cobra.Command{
		Use:   "backup",
		Short: "Back up MongoDB, the OpenArch files and the OpenSearch indices",
		Long: `Back up the data of a running workspace into a new folder:
  mongodb.archive.gz   a mongodump of the OpenArch database
  files.tar.gz         the OpenArch files (binary content)
  opensearch.tar.gz    an OpenSearch snapshot of the OpenArch indices
  manifest.json        the version, and a SHA-256 for each file

The folder is backups/<time> in the workspace (git-ignored), or in --to.
The backup runs while OpenArch runs: changes made during the backup may be
missing from it. The backup holds all the data: keep it private.`,
		Example: `  openarch backup
  openarch backup --to /srv/backups/openarch --output json`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			w, err := app.loadWorkspace()
			if err != nil {
				return err
			}
			to := parent
			if to == "" {
				to = filepath.Join(w.Dir, stack.BackupsDir)
			}
			res, err := stack.Backup(cmd.Context(), app.stackEnv(), w, to, app.progress())
			if err != nil {
				return err
			}
			return app.Printer.Print(res)
		},
	}
	cmd.Flags().StringVar(&parent, "to", "", "folder for the backup (default: backups/ in the workspace)")
	return cmd
}

func newRestoreCmd(app *App) *cobra.Command {
	var yes bool
	opts := stack.UpOptions{Wait: true}
	cmd := &cobra.Command{
		Use:   "restore <backup-folder>",
		Short: "Replace the data of the workspace with a backup",
		Long: `Replace the data of the workspace with a backup made by 'openarch backup'.

restore checks every file of the backup first. Then it stops OpenArch,
restores MongoDB, the OpenArch files and the OpenSearch indices, starts the
stack and waits until it is healthy. The current data is lost.

It works on a running stack, and after 'openarch clean'.
Without a terminal, --yes is required.`,
		Example: `  openarch restore backups/20261103T120000Z
  openarch restore /srv/backups/openarch/20261103T120000Z --yes`,
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			w, err := app.loadWorkspace()
			if err != nil {
				return err
			}
			dir, err := filepath.Abs(args[0])
			if err != nil {
				return err
			}
			m, err := stack.ReadManifest(dir)
			if err != nil {
				return err
			}
			if m.OpenArchVersion != w.Config.Version {
				app.Printer.Warn("The backup comes from OpenArch " + m.OpenArchVersion + ", and the workspace runs " + w.Config.Version + ".")
			}
			if err := app.confirm(yes, "Replace all the data of "+w.Dir+" with the backup "+dir+"?"); err != nil {
				return err
			}
			opts.Progress = app.progress()
			res, err := stack.Restore(cmd.Context(), app.stackEnv(), w, dir, opts)
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
			return app.Printer.Print(res)
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "do not ask for confirmation")
	cmd.Flags().DurationVar(&opts.Timeout, "timeout", DefaultUpTimeout, "how long to wait until the stack is healthy")
	return cmd
}
