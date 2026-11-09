package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/AlfrescoLabs/openarch-cli/internal/output"
	"github.com/AlfrescoLabs/openarch-cli/internal/stack"
	"github.com/AlfrescoLabs/openarch-cli/internal/workspace"
)

// DefaultUpTimeout covers the first start, which pulls about 2 GB of images.
const DefaultUpTimeout = 10 * time.Minute

func (a *App) loadWorkspace() (stack.Workspace, error) {
	dir, err := workspace.Find(a.Opts.Dir)
	if err != nil {
		return stack.Workspace{}, err
	}
	return stack.Load(dir)
}

// progress is where docker compose output goes: stderr, so stdout keeps only the result.
func (a *App) progress() io.Writer { return a.Streams.Err }

// dryRun is the result of a command run with --dry-run.
type dryRun struct {
	DryRun  bool   `json:"dryRun"`
	Command string `json:"command"`
}

func (d dryRun) WriteText(w io.Writer) error {
	_, err := fmt.Fprintf(w, "Dry run. This command would run:\n  %s\n", d.Command)
	return err
}

func composeCommand(w stack.Workspace, args ...string) string {
	return "docker compose --env-file .env --env-file .env.local " + strings.Join(args, " ") + " (in " + w.Dir + ")"
}

func newUpCmd(app *App) *cobra.Command {
	opts := stack.UpOptions{}
	var dry bool
	cmd := &cobra.Command{
		Use:   "up",
		Short: "Start OpenArch and wait until it is healthy",
		Long: `Start the workspace stack with docker compose and wait until every service
is healthy. The first start pulls the images, which can take several minutes.

up runs the doctor checks first (skip them with --skip-doctor). Ports that
the running stack of this workspace already uses are not reported as busy.

When a config file or a password changed (after 'openarch render'), up
recreates only the services that read it.

Exit code 3 means a prerequisite failed. Exit code 4 means the stack did not
become healthy before --timeout; the status of each service is still printed.
up also stops with exit code 4 when a healthcheck fails with HTTP 401 for one
minute, because a wrong password never becomes healthy.`,
		Example: `  openarch up
  openarch up --dir my-openarch --timeout 15m
  openarch up --wait=false
  openarch up --output json`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			w, err := app.loadWorkspace()
			if err != nil {
				return err
			}
			if dry {
				return app.Printer.Print(dryRun{DryRun: true, Command: composeCommand(w, "up", "--detach", "--wait")})
			}
			opts.Progress = app.progress()
			st, err := stack.Up(cmd.Context(), app.stackEnv(), w, opts)
			var oerr *output.Error
			if errors.As(err, &oerr) && oerr.Code == output.ExitUnhealthy {
				// Print the status, so the caller sees which service failed, then exit 4.
				if perr := app.Printer.Print(st); perr != nil {
					return perr
				}
				app.Printer.Warn(oerr.Message + ". " + oerr.Hint)
				return output.Exit(output.ExitUnhealthy, oerr.Message)
			}
			if err != nil {
				return err
			}
			return app.Printer.Print(st)
		},
	}
	fl := cmd.Flags()
	fl.BoolVar(&opts.SkipDoctor, "skip-doctor", false, "do not run the doctor checks first")
	fl.BoolVar(&opts.Wait, "wait", true, "wait until every service is healthy")
	fl.DurationVar(&opts.Timeout, "timeout", DefaultUpTimeout, "how long to wait with --wait")
	fl.BoolVar(&dry, "dry-run", false, "print the docker compose command, and run nothing")
	return cmd
}

func newStatusCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the state of each service and of the OpenArch API",
		Long: `Show the state and health of each service, and the result of a request to
the OpenArch health endpoint.

The stack state is healthy, starting, unhealthy or stopped.
Exit code 0 means healthy. Exit code 4 means any other state.`,
		Example: `  openarch status
  openarch status --output json`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			w, err := app.loadWorkspace()
			if err != nil {
				return err
			}
			st, err := stack.GetStatus(cmd.Context(), app.stackEnv(), w)
			if err != nil {
				return err
			}
			if err := app.Printer.Print(st); err != nil {
				return err
			}
			if st.State != stack.Healthy {
				return output.Exit(output.ExitUnhealthy, "OpenArch is "+st.State)
			}
			return nil
		},
	}
}

func newDownCmd(app *App) *cobra.Command {
	var dry bool
	cmd := &cobra.Command{
		Use:   "down",
		Short: "Stop OpenArch and remove its containers, keeping the data",
		Long: `Stop the stack and remove its containers and network. Named volumes and
data folders are kept, so 'openarch up' starts again with the same data.

With storage none there is no data to keep, so its volumes are removed too.`,
		Example: `  openarch down
  openarch down --dir my-openarch`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			w, err := app.loadWorkspace()
			if err != nil {
				return err
			}
			if dry {
				return app.Printer.Print(dryRun{DryRun: true, Command: composeCommand(w, "down")})
			}
			res, err := stack.Down(cmd.Context(), app.stackEnv(), w, app.progress())
			if err != nil {
				return err
			}
			return app.Printer.Print(res)
		},
	}
	cmd.Flags().BoolVar(&dry, "dry-run", false, "print the docker compose command, and run nothing")
	return cmd
}

func newCleanCmd(app *App) *cobra.Command {
	var yes, dry bool
	cmd := &cobra.Command{
		Use:   "clean",
		Short: "Stop OpenArch and delete its data",
		Long: `Stop the stack and delete its containers, network and volumes: all the data
in MongoDB, OpenSearch and the OpenArch files. The workspace files stay, so
'openarch up' starts again with an empty OpenArch.

With storage bind, the data folders are not deleted.
Make a backup first with 'openarch backup'. Without a terminal, --yes is required.`,
		Example: `  openarch clean
  openarch clean --dir my-openarch --yes`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			w, err := app.loadWorkspace()
			if err != nil {
				return err
			}
			if dry {
				return app.Printer.Print(dryRun{DryRun: true, Command: composeCommand(w, "down", "--volumes")})
			}
			if err := app.confirm(yes, fmt.Sprintf("Delete all the data of the workspace %s?", w.Dir)); err != nil {
				return err
			}
			res, err := stack.Clean(cmd.Context(), app.stackEnv(), w, app.progress())
			if err != nil {
				return err
			}
			return app.Printer.Print(res)
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "do not ask for confirmation")
	cmd.Flags().BoolVar(&dry, "dry-run", false, "print the docker compose command, and run nothing")
	return cmd
}

// confirm asks a yes/no question in a terminal. Without one, yes must be set.
func (a *App) confirm(yes bool, question string) error {
	if yes {
		return nil
	}
	if !a.Interactive() {
		return output.Usage("this command needs --yes when there is no terminal", "")
	}
	answer, err := readPromptLine(a.Streams.In, a.Streams.Err, question+" [y/N] ")
	if err != nil {
		return err
	}
	if v := strings.ToLower(answer); v != "y" && v != "yes" {
		return output.NewError(output.ExitError, "cancelled", "")
	}
	return nil
}

func newLogsCmd(app *App) *cobra.Command {
	var opts stack.LogsOptions
	cmd := &cobra.Command{
		Use:   "logs [service...]",
		Short: "Show the logs of the services",
		Long: `Show the logs of all services, or of the given ones.

With --output json, the lines are returned as one JSON document, so --follow
cannot be used.`,
		Example: `  openarch logs openarch-app
  openarch logs --follow --tail 100
  openarch logs mongodb-init --output json`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.Follow && app.Printer.Format() == output.JSON {
				return output.Usage("--follow cannot be used with --output json", "Use --tail or --since to limit the lines instead.")
			}
			w, err := app.loadWorkspace()
			if err != nil {
				return err
			}
			opts.Services = args
			if app.Printer.Format() == output.Text {
				opts.Stream, opts.Stderr = app.Streams.Out, app.Streams.Err
			}
			res, err := stack.Logs(cmd.Context(), app.stackEnv(), w, opts)
			if err != nil || opts.Stream != nil {
				return err
			}
			return app.Printer.Print(res)
		},
	}
	fl := cmd.Flags()
	fl.BoolVarP(&opts.Follow, "follow", "f", false, "keep printing new lines")
	fl.StringVar(&opts.Tail, "tail", "", "number of lines from the end of each log, or all")
	fl.StringVar(&opts.Since, "since", "", "show lines since a time (2026-11-03T10:00:00) or a duration (10m)")
	return cmd
}
