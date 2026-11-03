package cli

import (
	"bufio"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/AlfrescoLabs/openarch-cli/internal/api"
	"github.com/AlfrescoLabs/openarch-cli/internal/auth"
	"github.com/AlfrescoLabs/openarch-cli/internal/contexts"
	"github.com/AlfrescoLabs/openarch-cli/internal/credstore"
	"github.com/AlfrescoLabs/openarch-cli/internal/output"
	"github.com/AlfrescoLabs/openarch-cli/internal/term"
	"github.com/AlfrescoLabs/openarch-cli/internal/workspace"
)

// connFlags are the connection flags of commands that call the OpenArch API.
type connFlags struct {
	url           string
	user          string
	password      string
	passwordStdin bool
	insecure      bool
}

func (f *connFlags) register(cmd *cobra.Command) {
	pf := cmd.PersistentFlags()
	pf.StringVar(&f.url, "url", "", "OpenArch URL (env "+auth.EnvURL+"; default: the workspace URL)")
	pf.StringVar(&f.user, "user", "", "user name (env "+auth.EnvUser+"; default: the workspace admin)")
	pf.BoolVar(&f.passwordStdin, "password-stdin", false, "read the password from stdin")
	pf.StringVar(&f.password, "password", "", "password (insecure: it stays in the shell history; prefer --password-stdin or env "+auth.EnvPassword+")")
	pf.BoolVar(&f.insecure, "insecure", false, "do not verify the server certificate (env "+auth.EnvInsecure+")")
}

// client resolves the credentials and returns an API client.
func (a *App) client(cmd *cobra.Command, f *connFlags) (*api.Client, error) {
	dir, err := workspace.Find(a.Opts.Dir)
	if err != nil {
		return nil, err
	}
	cfg, err := a.contexts()
	if err != nil {
		return nil, err
	}
	in := auth.Input{
		URL: f.url, User: f.user, Password: f.password, PasswordStdin: f.passwordStdin,
		Insecure: f.insecure, InsecureSet: cmd.Flags().Changed("insecure"),
		Stdin: a.Streams.In, WorkspaceDir: dir,
		Context: a.Opts.Context, Contexts: &cfg, Store: a.store,
	}
	if a.Interactive() && !f.passwordStdin {
		in.Prompt = func(label string, secret bool) (string, error) {
			if secret {
				return term.ReadPassword(a.Streams.In, a.Streams.Err, label)
			}
			return readPromptLine(a.Streams.In, a.Streams.Err, label)
		}
	}
	if f.password != "" {
		a.Printer.Warn("--password puts the password in the shell history. Prefer --password-stdin or " + auth.EnvPassword + ".")
	}
	creds, err := auth.Resolve(in)
	if err != nil {
		return nil, err
	}
	return a.newClient(creds)
}

// newClient warns about --insecure and returns a client for creds.
func (a *App) newClient(creds auth.Credentials) (*api.Client, error) {
	if creds.Insecure {
		a.Printer.Warn("TLS certificate verification is off (--insecure).")
	}
	a.Log.Debug("credentials", "context", creds.Context, "url", creds.URL, "user", creds.User,
		"urlSource", string(creds.Sources.URL), "userSource", string(creds.Sources.User), "passwordSource", string(creds.Sources.Password))
	return api.New(creds, a.Log)
}

func newAPICmd(app *App) *cobra.Command {
	var f connFlags
	cmd := &cobra.Command{
		Use:   "api",
		Short: "Call the OpenArch API",
		Long: `Call the OpenArch API.

The URL, user and password come from, in this order: flags, environment
variables (OPENARCH_URL, OPENARCH_USER, OPENARCH_PASSWORD), the workspace
(openarch.yaml and .env.local), and a hidden prompt in a terminal.
Workspace credentials are used only with the workspace URL.

Exit code 6 means the credentials are missing or wrong.`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	f.register(cmd)
	cmd.AddCommand(&cobra.Command{
		Use:   "ping",
		Short: "Check that OpenArch answers and that the credentials work",
		Example: `  openarch api ping
  openarch api ping --dir my-openarch --output json
  OPENARCH_URL=https://openarch.example.com OPENARCH_USER=admin openarch api ping --password-stdin < pw.txt`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := app.client(cmd, &f)
			if err != nil {
				return err
			}
			res, err := c.Ping(cmd.Context())
			if err != nil {
				return err
			}
			return app.Printer.Print(res)
		},
	})
	return cmd
}

// configDir returns the CLI config folder.
func (a *App) configDir() (string, error) {
	return contexts.Dir(os.Getenv)
}

// contexts loads the CLI config file.
func (a *App) contexts() (contexts.Config, error) {
	dir, err := a.configDir()
	if err != nil {
		return contexts.Config{}, err
	}
	return contexts.Load(dir)
}

// store opens the credential store once per command.
func (a *App) store() (credstore.Store, error) {
	if a.credStore != nil {
		return a.credStore, nil
	}
	dir, err := a.configDir()
	if err != nil {
		return nil, err
	}
	st, err := credstore.Open(dir, os.Getenv, a.Printer.Warn)
	if err != nil {
		return nil, output.Usage(err.Error(), "")
	}
	a.credStore = st
	return st, nil
}

// readSecret reads an existing password from stdin. One trailing newline is removed.
func readSecret(r io.Reader) (string, error) {
	data, err := io.ReadAll(io.LimitReader(r, 4096))
	if err != nil {
		return "", err
	}
	pw := strings.TrimSuffix(strings.TrimSuffix(string(data), "\n"), "\r")
	if pw == "" {
		return "", output.Usage("the password from stdin is empty", "")
	}
	return pw, nil
}

// readPromptLine asks for a visible value on the terminal.
func readPromptLine(in io.Reader, out io.Writer, label string) (string, error) {
	if _, err := io.WriteString(out, label); err != nil {
		return "", err
	}
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimSpace(line), nil
}
