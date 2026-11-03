package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/AlfrescoLabs/openarch-cli/internal/generate"
	"github.com/AlfrescoLabs/openarch-cli/internal/output"
	"github.com/AlfrescoLabs/openarch-cli/internal/workspace"
)

// optionFlags are the init flags that set a key of openarch.yaml. They cannot be combined with --from.
var optionFlags = []string{
	"version", "storage", "data-dir", "tls", "cert", "key", "server-name",
	"port", "bind-ip", "expose", "with", "memory",
}

type initFlags struct {
	cfg           workspace.Config
	from          string
	passwordStdin bool
	force         bool
	dryRun        bool
}

func newInitCmd(app *App) *cobra.Command {
	return initCmd(app, &initFlags{})
}

// initCmd builds the init command with its flags bound to f.
func initCmd(app *App, f *initFlags) *cobra.Command {
	def := workspace.Default("")
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create an OpenArch workspace",
		Long: `Create an OpenArch workspace: openarch.yaml, compose.yaml, .env,
.env.local (secrets, mode 0600), config/users.yml, .gitignore and README.md.

openarch.yaml is the source of truth. To change the deployment later, edit it
and run 'openarch render'. Every option below is also a key in openarch.yaml.

The workspace is the folder given by --dir, then OPENARCH_WORKSPACE, then the
current folder. It is created if needed.

The admin password is generated unless it is read from stdin with
--admin-password-stdin. It is stored in .env.local and never printed.

In a terminal, init asks for every option that was not given as a flag, then
prints the equivalent non-interactive command. --non-interactive, --from or a
missing terminal turn the questions off. Set ` + envAccessible + `=1 for plain
prompts that work with screen readers.`,
		Example: `  openarch init --dir my-openarch
  openarch init --storage bind --data-dir /srv/openarch
  openarch init --tls self-signed --server-name openarch.local
  openarch init --expose opensearch --with dashboards --memory 8g
  printf '%s' "$PASSWORD" | openarch init --admin-password-stdin --non-interactive
  openarch init --from openarch.yaml --dir copy
  openarch init --dry-run --output json`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dir, err := workspace.Find(app.Opts.Dir)
			if err != nil {
				return err
			}
			wizard := f.from == "" && app.Interactive()
			var typed string
			if wizard {
				if typed, err = runWizard(cmd.Context(), cmd.Flags(), f, app.Streams.In, app.Streams.Err); err != nil {
					return err
				}
			}
			opts, err := f.options(cmd.Flags(), dir, app.Streams.In)
			if err != nil {
				return err
			}
			if typed != "" {
				opts.AdminPassword = typed
			}
			if wizard {
				_, _ = fmt.Fprintf(app.Streams.Err, "Equivalent command:\n  %s\n\n", equivalentCommand(app.Opts.Dir, f, typed != ""))
			}
			res, err := generate.Init(cmd.Context(), app.generateEnv(), opts)
			if err != nil {
				return err
			}
			return app.printResult(res)
		},
	}
	fl := cmd.Flags()
	fl.StringVar(&f.cfg.Version, "version", def.Version, "OpenArch image tag")
	fl.StringVar(&f.cfg.Storage.Mode, "storage", def.Storage.Mode, "where data is kept: "+strings.Join(workspace.StorageModes, ", "))
	fl.StringVar(&f.cfg.Storage.DataDir, "data-dir", "", "data folder for --storage bind (default "+workspace.DefaultDataDir+")")
	fl.StringVar(&f.cfg.TLS.Mode, "tls", def.TLS.Mode, "HTTPS mode: "+strings.Join(workspace.TLSModes, ", "))
	fl.StringVar(&f.cfg.TLS.Cert, "cert", "", "PEM certificate file for --tls custom")
	fl.StringVar(&f.cfg.TLS.Key, "key", "", "PEM private key file for --tls custom")
	fl.StringVar(&f.cfg.ServerName, "server-name", def.ServerName, "host name for URLs and the certificate")
	fl.IntVar(&f.cfg.Port, "port", 0, "host port (default 8080, or 8443 with TLS)")
	fl.StringVar(&f.cfg.BindIP, "bind-ip", def.BindIP, "address the ports listen on: 127.0.0.1 (this machine only) or 0.0.0.0 (all interfaces)")
	fl.StringSliceVar(&f.cfg.Expose, "expose", nil, "publish an extra port: "+strings.Join(workspace.ExposeOptions, ", ")+" (repeatable)")
	fl.StringSliceVar(&f.cfg.With, "with", nil, "add a service: "+strings.Join(workspace.WithOptions, ", ")+" (repeatable)")
	fl.StringVar(&f.cfg.Memory, "memory", def.Memory, "memory for the stack: auto (from docker info) or a size such as 4g")
	fl.StringVar(&f.from, "from", "", "create the workspace from an existing openarch.yaml")
	fl.BoolVar(&f.passwordStdin, "admin-password-stdin", false, "read the admin password from stdin")
	fl.BoolVar(&f.force, "force", false, "overwrite an existing workspace (keeps the password in .env.local unless a new one is given)")
	fl.BoolVar(&f.dryRun, "dry-run", false, "list the files that would be written, and write nothing")
	return cmd
}

// options turns the flags into generate options. dir is the workspace folder.
func (f *initFlags) options(fl *pflag.FlagSet, dir string, stdin io.Reader) (generate.InitOptions, error) {
	opts := generate.InitOptions{Dir: dir, Force: f.force, DryRun: f.dryRun}
	if f.from != "" {
		for _, name := range optionFlags {
			if fl.Changed(name) {
				return opts, output.Usage(fmt.Sprintf("--from cannot be used with --%s", name), "Edit the file given to --from instead.")
			}
		}
		data, err := os.ReadFile(f.from)
		if err != nil {
			return opts, output.Usage("cannot read --from file: "+err.Error(), "")
		}
		cfg, err := workspace.Parse(data, f.from)
		if err != nil {
			return opts, err
		}
		opts.Config = cfg
	} else {
		cfg := f.cfg
		cfg.SchemaVersion = workspace.SchemaVersion
		cfg.Name = workspace.ProjectName(filepath.Base(dir))
		if cfg.Port == 0 {
			cfg.Port = workspace.DefaultPort(cfg.TLS.Mode)
		}
		if cfg.Storage.Mode == workspace.StorageBind && cfg.Storage.DataDir == "" {
			cfg.Storage.DataDir = workspace.DefaultDataDir
		}
		// Certificate paths are relative to the current folder on the command line,
		// but relative to the workspace in openarch.yaml.
		for _, p := range []*string{&cfg.TLS.Cert, &cfg.TLS.Key} {
			if *p != "" {
				abs, err := filepath.Abs(*p)
				if err != nil {
					return opts, err
				}
				*p = abs
			}
		}
		opts.Config = cfg
	}
	if f.passwordStdin {
		pw, err := readPassword(stdin)
		if err != nil {
			return opts, err
		}
		opts.AdminPassword = pw
	}
	return opts, nil
}

// readPassword reads a password from r. One trailing newline is removed.
func readPassword(r io.Reader) (string, error) {
	data, err := io.ReadAll(io.LimitReader(r, 4096))
	if err != nil {
		return "", err
	}
	pw := strings.TrimSuffix(strings.TrimSuffix(string(data), "\n"), "\r")
	switch {
	case pw == "":
		return "", output.Usage("the admin password from stdin is empty", "Pipe the password, for example: printf '%s' \"$PASSWORD\" | openarch init --admin-password-stdin")
	case len(pw) < 8:
		return "", output.Usage("the admin password is too short", "Use at least 8 characters.")
	case strings.ContainsAny(pw, "\n\r"):
		return "", output.Usage("the admin password has more than one line", "")
	}
	return pw, nil
}

func newRenderCmd(app *App) *cobra.Command {
	var opts generate.RenderOptions
	cmd := &cobra.Command{
		Use:   "render",
		Short: "Write the workspace files again from openarch.yaml",
		Long: `Write compose.yaml, .env, config/users.yml, .gitignore, README.md and the
TLS files again from openarch.yaml and .env.local.

This is the declarative path: edit openarch.yaml (or the admin password in
.env.local), then run render. Files that would not change are not touched.
With --dry-run, render shows a diff and writes nothing. Secret files are never
shown in the diff.`,
		Example: `  openarch render
  openarch render --dry-run
  openarch render --dir my-openarch --output json`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dir, err := workspace.Find(app.Opts.Dir)
			if err != nil {
				return err
			}
			opts.Dir = dir
			res, err := generate.Render(cmd.Context(), app.generateEnv(), opts)
			if err != nil {
				return err
			}
			return app.printResult(res)
		},
	}
	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", false, "show what would change, and write nothing")
	return cmd
}

func (a *App) printResult(res generate.Result) error {
	for _, w := range res.Warnings {
		a.Printer.Warn(w)
	}
	return a.Printer.Print(res)
}
