// Package cli defines the cobra commands. Commands only parse flags and call
// functions in other internal packages; they hold no business logic.
package cli

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/AlfrescoLabs/openarch-cli/internal/credstore"
	"github.com/AlfrescoLabs/openarch-cli/internal/doctor"
	"github.com/AlfrescoLabs/openarch-cli/internal/generate"
	"github.com/AlfrescoLabs/openarch-cli/internal/output"
	"github.com/AlfrescoLabs/openarch-cli/internal/selfupdate"
	"github.com/AlfrescoLabs/openarch-cli/internal/stack"
	"github.com/AlfrescoLabs/openarch-cli/internal/term"
)

const envOutput = "OPENARCH_OUTPUT"

// Streams holds the standard streams used by commands.
type Streams struct {
	In  io.Reader
	Out io.Writer
	Err io.Writer
}

// GlobalOptions holds the flags shared by every command.
type GlobalOptions struct {
	Output         string
	Dir            string
	Context        string
	NonInteractive bool
	Verbose        bool
	NoColor        bool
}

// App is the state shared by commands during one execution.
type App struct {
	Opts    GlobalOptions
	Streams Streams
	Printer *output.Printer
	Log     *slog.Logger

	// doctorEnv and generateEnv give commands access to the system. Tests replace them with fakes.
	doctorEnv   func() doctor.Env
	generateEnv func() generate.Env
	stackEnv    func() stack.Env
	// selfUpdateEnv gives self-update access to GitHub, Sigstore and the executable.
	selfUpdateEnv func() selfupdate.Env
	// credStore is opened on first use. Tests set it to a memory store.
	credStore credstore.Store
}

func newApp(s Streams) *App {
	return &App{Streams: s, doctorEnv: doctor.SystemEnv, generateEnv: generate.SystemEnv, stackEnv: stack.SystemEnv, selfUpdateEnv: selfupdate.SystemEnv}
}

// Interactive reports whether commands may prompt the user.
func (a *App) Interactive() bool {
	return !a.Opts.NonInteractive && term.IsTerminal(a.Streams.In) && term.IsTerminal(a.Streams.Out)
}

// Color reports whether text output may use colors.
func (a *App) Color() bool {
	return !a.Opts.NoColor && os.Getenv("NO_COLOR") == "" && term.IsTerminal(a.Streams.Out)
}

func (a *App) setup() error {
	format, err := output.ParseFormat(a.Opts.Output)
	if err != nil {
		return err
	}
	a.Printer = output.NewPrinter(format, a.Streams.Out, a.Streams.Err)
	level := slog.LevelWarn
	if a.Opts.Verbose {
		level = slog.LevelDebug
	}
	a.Log = slog.New(slog.NewTextHandler(a.Streams.Err, &slog.HandlerOptions{Level: level}))
	return nil
}

// Execute runs the CLI with args and returns the process exit code.
func Execute(ctx context.Context, args []string, s Streams) int {
	return execute(ctx, newApp(s), args)
}

func execute(ctx context.Context, app *App, args []string) int {
	s := app.Streams
	root := newRootCmd(app)
	root.SetArgs(args)
	err := root.ExecuteContext(ctx)
	if err == nil {
		return int(output.ExitOK)
	}
	p := app.Printer
	if p == nil {
		// The error happened before flags were fully parsed.
		p = output.NewPrinter(formatFromArgs(args), s.Out, s.Err)
	}
	p.Error(err)
	return int(output.CodeOf(err))
}

func newRootCmd(app *App) *cobra.Command {
	root := &cobra.Command{
		Use:   "openarch",
		Short: "Install and operate OpenArch",
		Long: `openarch installs and operates OpenArch with Docker Compose.

Every command supports --output json. Results go to stdout and logs go to stderr.
Exit codes: 0 ok, 1 error, 2 bad usage, 3 prerequisite missing,
4 service not healthy, 5 remote API error, 6 authentication failed.`,
		Example: `  openarch doctor
  openarch init --dir my-openarch
  openarch up --dir my-openarch
  openarch status --output json
  openarch logs openarch-app --tail 50
  openarch down`,
		Args:          noArgs,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
		PersistentPreRunE: func(*cobra.Command, []string) error {
			return app.setup()
		},
	}
	root.SetIn(app.Streams.In)
	root.SetOut(app.Streams.Out)
	root.SetErr(app.Streams.Err)
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return &output.Error{Code: output.ExitUsage, Message: err.Error(), Hint: "Run 'openarch --help' for usage.", Err: err}
	})

	pf := root.PersistentFlags()
	pf.StringVarP(&app.Opts.Output, "output", "o", envOr(envOutput, string(output.Text)), "output format: text or json (env "+envOutput+")")
	pf.StringVar(&app.Opts.Dir, "dir", "", "workspace directory (default: current directory)")
	pf.StringVar(&app.Opts.Context, "context", "", "connection context to use")
	pf.BoolVar(&app.Opts.NonInteractive, "non-interactive", false, "never prompt; fail when a value is missing")
	pf.BoolVarP(&app.Opts.Verbose, "verbose", "v", false, "print debug logs to stderr")
	pf.BoolVar(&app.Opts.NoColor, "no-color", false, "disable colors (env NO_COLOR)")

	root.AddCommand(newDoctorCmd(app), newInitCmd(app), newRenderCmd(app),
		newUpCmd(app), newStatusCmd(app), newLogsCmd(app), newDownCmd(app), newCleanCmd(app), newAPICmd(app), newConfigCmd(app),
		newQueryCmd(app), newDocumentsCmd(app), newUsersCmd(app), newSmokeTestCmd(app), newUpgradeCmd(app), newBackupCmd(app), newRestoreCmd(app), newSecretsCmd(app), newMCPCmd(app), newSkillCmd(app), newSelfUpdateCmd(app), newVersionCmd(app))
	addCompletion(app, root)
	setJSONHelp(app, root)
	return root
}

// noArgs rejects positional arguments with a usage error.
func noArgs(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return nil
	}
	if cmd.HasAvailableSubCommands() {
		hint := fmt.Sprintf("Run '%s --help' for the list of commands.", cmd.CommandPath())
		if s := cmd.SuggestionsFor(args[0]); len(s) > 0 {
			hint = fmt.Sprintf("Did you mean %q?", s[0])
		}
		return output.Usage(fmt.Sprintf("unknown command %q for %q", args[0], cmd.CommandPath()), hint)
	}
	return output.Usage(fmt.Sprintf("%q accepts no arguments", cmd.CommandPath()), "")
}

// formatFromArgs finds the output format without cobra, for errors raised
// before flag parsing finished.
func formatFromArgs(args []string) output.Format {
	value := os.Getenv(envOutput)
	for i := 0; i < len(args) && args[i] != "--"; i++ {
		a := args[i]
		switch {
		case a == "-o" || a == "--output":
			if i+1 < len(args) {
				value = args[i+1]
			}
		case strings.HasPrefix(a, "--output="):
			value = strings.TrimPrefix(a, "--output=")
		case strings.HasPrefix(a, "-o") && len(a) > 2:
			value = strings.TrimPrefix(strings.TrimPrefix(a, "-o"), "=")
		}
	}
	if output.Format(value) == output.JSON {
		return output.JSON
	}
	return output.Text
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
