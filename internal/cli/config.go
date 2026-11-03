package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/AlfrescoLabs/openarch-cli/internal/auth"
	"github.com/AlfrescoLabs/openarch-cli/internal/contexts"
	"github.com/AlfrescoLabs/openarch-cli/internal/credstore"
	"github.com/AlfrescoLabs/openarch-cli/internal/localenv"
	"github.com/AlfrescoLabs/openarch-cli/internal/output"
	"github.com/AlfrescoLabs/openarch-cli/internal/term"
	"github.com/AlfrescoLabs/openarch-cli/internal/workspace"
)

// DefaultContextName is the name used by 'config set' without --name.
const DefaultContextName = "default"

const masked = "********"

func newConfigCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage saved connections (contexts)",
		Long: `Manage saved connections, called contexts.

A context is a local workspace, or the URL and user of a remote OpenArch.
Contexts are in config.yaml in the CLI config folder (OPENARCH_CONFIG_DIR,
else $XDG_CONFIG_HOME/openarch, else ~/.config/openarch, or %AppData%\openarch
on Windows). That file never holds a password.

Passwords of remote contexts are in the OS credential store: Keychain on
macOS, Credential Manager on Windows, Secret Service on Linux. When it is
not available, they are in credentials.json (mode 0600) in the same folder,
with a warning. OPENARCH_CREDENTIAL_STORE=keychain|file forces a backend.

Commands that call the API use, in this order: --context or
OPENARCH_CONTEXT, then the workspace in --dir or the current folder, then the
current context.`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(newConfigSetCmd(app), newConfigGetCmd(app), newConfigListCmd(app),
		newConfigUseCmd(app), newConfigDeleteCmd(app), newConfigWhoamiCmd(app))
	return cmd
}

// exactArgs is cobra.ExactArgs with a usage error.
func exactArgs(n int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) != n {
			return output.Usage(fmt.Sprintf("%q needs %d argument(s), got %d", cmd.CommandPath(), n, len(args)),
				"Run '"+cmd.CommandPath()+" --help' for usage.")
		}
		return nil
	}
}

func (a *App) saveContexts(cfg contexts.Config) error {
	dir, err := a.configDir()
	if err != nil {
		return err
	}
	return contexts.Save(dir, cfg)
}

// contextName returns name, or the current context when name is empty.
func contextName(cfg contexts.Config, name string) (string, error) {
	if name != "" {
		return name, nil
	}
	if cfg.CurrentContext == "" {
		return "", output.Usage("no current context", "Use --name, or select one with 'openarch config use <name>'.")
	}
	return cfg.CurrentContext, nil
}

// ConfigSetResult is the outcome of 'config set'.
type ConfigSetResult struct {
	Name     string `json:"name"`
	System   string `json:"system"`
	URL      string `json:"url"`
	User     string `json:"user"`
	Store    string `json:"store"`
	Current  bool   `json:"current"`
	Verified bool   `json:"verified"`
}

func (r ConfigSetResult) WriteText(w io.Writer) error {
	v := "verified"
	if !r.Verified {
		v = "not verified"
	}
	cur := ""
	if r.Current {
		cur = " It is the current context."
	}
	_, err := fmt.Fprintf(w, "Saved context %q: %s as %s (%s). Password in the %s store.%s\n", r.Name, r.URL, r.User, v, r.Store, cur)
	return err
}

func newConfigSetCmd(app *App) *cobra.Command {
	var name, system, url, user string
	var passwordStdin, insecure, noVerify bool
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Save a remote connection and its password",
		Long: `Save the URL and user of an OpenArch server as a context, and its password in
the credential store. The password comes from stdin (--password-stdin) or a
hidden prompt. It is never a command argument.

Before saving, set checks the connection with 'api ping'. --no-verify skips it.
The first context saved becomes the current context.`,
		Example: `  openarch config set -s https://openarch.example.com -u admin
  printf '%s' "$PASSWORD" | openarch config set --name prod -s https://openarch.example.com -u admin --password-stdin`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := contexts.ValidateName(name); err != nil {
				return err
			}
			if err := contexts.ValidSystem(system); err != nil {
				return err
			}
			if url == "" || user == "" {
				return output.Usage("--server and --username are required", "Example: openarch config set -s https://openarch.example.com -u admin")
			}
			url = strings.TrimSuffix(url, "/")
			if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
				return output.Usage(fmt.Sprintf("invalid URL %q", url), "Use a URL such as https://openarch.example.com.")
			}
			cfg, err := app.contexts()
			if err != nil {
				return err
			}
			if old, ok := cfg.Contexts[name]; ok && old.Workspace != "" {
				return output.Usage(fmt.Sprintf("context %q is a workspace context", name), "Use another --name.")
			}
			var pw string
			switch {
			case passwordStdin:
				pw, err = readSecret(app.Streams.In)
			case app.Interactive():
				pw, err = term.ReadPassword(app.Streams.In, app.Streams.Err, fmt.Sprintf("Password for %s at %s: ", user, url))
			default:
				return output.NewError(output.ExitAuth, "no password", "Use --password-stdin, or run in a terminal to be asked.")
			}
			if err != nil {
				return err
			}
			if pw == "" {
				return output.Usage("the password is empty", "")
			}
			res := ConfigSetResult{Name: name, System: system, URL: url, User: user}
			if !noVerify {
				c, err := app.newClient(auth.Credentials{URL: url, User: user, Password: pw, Insecure: insecure,
					Sources: auth.Sources{URL: auth.SourceFlag, User: auth.SourceFlag, Password: auth.SourceFlag}})
				if err != nil {
					return err
				}
				if _, err := c.Ping(cmd.Context()); err != nil {
					return err
				}
				res.Verified = true
			}
			st, err := app.store()
			if err != nil {
				return err
			}
			if err := st.Set(url, user, pw); err != nil {
				return fmt.Errorf("save the password in the %s store: %w", st.Name(), err)
			}
			if cfg.Contexts == nil {
				cfg.Contexts = map[string]contexts.Context{}
			}
			ctx := cfg.Contexts[name]
			if ctx.Endpoints == nil {
				ctx.Endpoints = map[string]contexts.Endpoint{}
			}
			if old, ok := ctx.Endpoints[system]; ok && (old.URL != url || old.Username != user) {
				// The endpoint moves: remove the password of the old one.
				_ = st.Delete(old.URL, old.Username)
			}
			ctx.Endpoints[system] = contexts.Endpoint{URL: url, Username: user, Insecure: insecure}
			cfg.Contexts[name] = ctx
			if cfg.CurrentContext == "" {
				cfg.CurrentContext = name
			}
			if err := app.saveContexts(cfg); err != nil {
				return err
			}
			res.Store, res.Current = st.Name(), cfg.CurrentContext == name
			return app.Printer.Print(res)
		},
	}
	fl := cmd.Flags()
	fl.StringVar(&name, "name", DefaultContextName, "context name")
	fl.StringVar(&system, "system", contexts.SystemOpenArch, "system of the endpoint: "+strings.Join(contexts.Systems, ", "))
	fl.StringVarP(&url, "server", "s", "", "server URL")
	fl.StringVarP(&user, "username", "u", "", "user name")
	fl.BoolVar(&passwordStdin, "password-stdin", false, "read the password from stdin")
	fl.BoolVar(&insecure, "insecure", false, "do not verify the server certificate")
	fl.BoolVar(&noVerify, "no-verify", false, "save without checking the connection")
	return cmd
}

// ContextView is a context as shown by 'config get' and 'config list'.
type ContextView struct {
	Name      string                  `json:"name"`
	Kind      string                  `json:"kind"`
	Current   bool                    `json:"current"`
	Workspace string                  `json:"workspace,omitempty"`
	Endpoints map[string]EndpointView `json:"endpoints,omitempty"`
}

// EndpointView is an endpoint with its password masked, or shown with --show-secrets.
type EndpointView struct {
	URL      string `json:"url"`
	Username string `json:"username"`
	Insecure bool   `json:"insecure,omitempty"`
	Password string `json:"password"`
}

func (v ContextView) WriteText(w io.Writer) error {
	var b strings.Builder
	cur := ""
	if v.Current {
		cur = " (current)"
	}
	fmt.Fprintf(&b, "Context %s%s: %s\n", v.Name, cur, v.Kind)
	if v.Workspace != "" {
		fmt.Fprintf(&b, "  workspace  %s\n", v.Workspace)
	}
	for system, e := range v.Endpoints {
		fmt.Fprintf(&b, "  %s\n    url       %s\n    username  %s\n    password  %s\n", system, e.URL, e.Username, e.Password)
		if e.Insecure {
			b.WriteString("    insecure  true\n")
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// view builds the view of a context. Passwords are read only with showSecrets.
func (a *App) view(cfg contexts.Config, name string, showSecrets bool) (ContextView, error) {
	ctx, err := cfg.Get(name)
	if err != nil {
		return ContextView{}, err
	}
	v := ContextView{Name: name, Kind: ctx.Kind(), Current: cfg.CurrentContext == name, Workspace: ctx.Workspace}
	reveal := func(get func() (string, error)) string {
		if !showSecrets {
			return masked
		}
		pw, err := get()
		switch {
		case errors.Is(err, credstore.ErrNotFound) || (err == nil && pw == ""):
			return "(not stored)"
		case err != nil:
			return "(error: " + err.Error() + ")"
		}
		return pw
	}
	if ctx.Workspace != "" {
		ws, err := workspace.Load(filepath.Join(ctx.Workspace, workspace.ConfigFile))
		if err != nil {
			return v, nil // The view still shows the path; commands that use it report the problem.
		}
		data, _ := os.ReadFile(filepath.Join(ctx.Workspace, localenv.File))
		secrets := localenv.Parse(data)
		v.Endpoints = map[string]EndpointView{contexts.SystemOpenArch: {
			URL: ws.URL(), Username: secrets.AdminUser,
			Password: reveal(func() (string, error) { return secrets.AdminPassword, nil }),
		}}
		return v, nil
	}
	v.Endpoints = map[string]EndpointView{}
	for system, e := range ctx.Endpoints {
		e := e
		v.Endpoints[system] = EndpointView{URL: e.URL, Username: e.Username, Insecure: e.Insecure,
			Password: reveal(func() (string, error) {
				st, err := a.store()
				if err != nil {
					return "", err
				}
				return st.Get(e.URL, e.Username)
			})}
	}
	return v, nil
}

func newConfigGetCmd(app *App) *cobra.Command {
	var name string
	var showSecrets bool
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Show a context",
		Long:  "Show a context. Passwords are masked unless --show-secrets is set.",
		Example: `  openarch config get
  openarch config get --name prod --output json`,
		Args: noArgs,
		RunE: func(*cobra.Command, []string) error {
			cfg, err := app.contexts()
			if err != nil {
				return err
			}
			n, err := contextName(cfg, name)
			if err != nil {
				return err
			}
			v, err := app.view(cfg, n, showSecrets)
			if err != nil {
				return err
			}
			return app.Printer.Print(v)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "context name (default: the current context)")
	cmd.Flags().BoolVar(&showSecrets, "show-secrets", false, "show the passwords")
	return cmd
}

// ContextList is the result of 'config list'.
type ContextList struct {
	Current  string        `json:"current,omitempty"`
	Contexts []ContextView `json:"contexts"`
}

func (l ContextList) WriteText(w io.Writer) error {
	if len(l.Contexts) == 0 {
		_, err := io.WriteString(w, "No contexts. Create one with 'openarch config set' or 'openarch init'.\n")
		return err
	}
	var b strings.Builder
	width := 0
	for _, c := range l.Contexts {
		width = max(width, len(c.Name))
	}
	for _, c := range l.Contexts {
		mark := " "
		if c.Current {
			mark = "*"
		}
		target := c.Workspace
		if e, ok := c.Endpoints[contexts.SystemOpenArch]; ok && c.Workspace == "" {
			target = e.URL + " as " + e.Username
		}
		fmt.Fprintf(&b, "%s %-*s  %-9s  %s\n", mark, width, c.Name, c.Kind, target)
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func newConfigListCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Short:   "List the contexts",
		Example: "  openarch config list --output json",
		Args:    noArgs,
		RunE: func(*cobra.Command, []string) error {
			cfg, err := app.contexts()
			if err != nil {
				return err
			}
			l := ContextList{Current: cfg.CurrentContext, Contexts: []ContextView{}}
			for _, n := range cfg.Names() {
				ctx := cfg.Contexts[n]
				v := ContextView{Name: n, Kind: ctx.Kind(), Current: n == cfg.CurrentContext, Workspace: ctx.Workspace}
				if ctx.Workspace == "" {
					v.Endpoints = map[string]EndpointView{}
					for system, e := range ctx.Endpoints {
						v.Endpoints[system] = EndpointView{URL: e.URL, Username: e.Username, Insecure: e.Insecure, Password: masked}
					}
				}
				l.Contexts = append(l.Contexts, v)
			}
			return app.Printer.Print(l)
		},
	}
}

// contextChange is the result of 'config use' and 'config delete'.
type contextChange struct {
	Action  string `json:"action"`
	Name    string `json:"name"`
	Current string `json:"current"`
}

func (c contextChange) WriteText(w io.Writer) error {
	var err error
	switch c.Action {
	case "use":
		_, err = fmt.Fprintf(w, "Current context: %s\n", c.Name)
	default:
		_, err = fmt.Fprintf(w, "Deleted context %s and its stored passwords.\n", c.Name)
	}
	return err
}

func newConfigUseCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:     "use <name>",
		Short:   "Select the current context",
		Example: "  openarch config use prod",
		Args:    exactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			cfg, err := app.contexts()
			if err != nil {
				return err
			}
			if _, err := cfg.Get(args[0]); err != nil {
				return err
			}
			cfg.CurrentContext = args[0]
			if err := app.saveContexts(cfg); err != nil {
				return err
			}
			return app.Printer.Print(contextChange{Action: "use", Name: args[0], Current: args[0]})
		},
	}
}

func newConfigDeleteCmd(app *App) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a context and its stored passwords",
		Long: `Delete a context and the passwords it has in the credential store.
A workspace context only points to the workspace: the workspace is not changed.

Without a terminal, --yes is required.`,
		Example: "  openarch config delete prod --yes",
		Args:    exactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			name := args[0]
			cfg, err := app.contexts()
			if err != nil {
				return err
			}
			ctx, err := cfg.Get(name)
			if err != nil {
				return err
			}
			if !yes {
				if !app.Interactive() {
					return output.Usage("delete needs --yes when there is no terminal", "")
				}
				answer, err := readPromptLine(app.Streams.In, app.Streams.Err, fmt.Sprintf("Delete context %q and its stored passwords? [y/N] ", name))
				if err != nil {
					return err
				}
				if a := strings.ToLower(answer); a != "y" && a != "yes" {
					return output.NewError(output.ExitError, "delete cancelled", "")
				}
			}
			if len(ctx.Endpoints) > 0 {
				st, err := app.store()
				if err != nil {
					return err
				}
				for _, e := range ctx.Endpoints {
					if err := st.Delete(e.URL, e.Username); err != nil {
						return fmt.Errorf("remove the password of %s from the %s store: %w", e.URL, st.Name(), err)
					}
				}
			}
			delete(cfg.Contexts, name)
			if cfg.CurrentContext == name {
				cfg.CurrentContext = ""
			}
			if err := app.saveContexts(cfg); err != nil {
				return err
			}
			return app.Printer.Print(contextChange{Action: "delete", Name: name, Current: cfg.CurrentContext})
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "do not ask for confirmation")
	return cmd
}

// Whoami is the result of 'config whoami'. It never holds a password.
type Whoami struct {
	Context     string       `json:"context,omitempty"`
	Workspace   string       `json:"workspace,omitempty"`
	URL         string       `json:"url"`
	User        string       `json:"user"`
	HasPassword bool         `json:"hasPassword"`
	Insecure    bool         `json:"insecure,omitempty"`
	Sources     auth.Sources `json:"sources"`
}

func (w Whoami) WriteText(out io.Writer) error {
	var b strings.Builder
	from := func(s auth.Source) string {
		switch s {
		case "":
			return "not found"
		case auth.SourceContext:
			return "context " + w.Context
		case auth.SourceWorkspace:
			return "workspace " + w.Workspace
		}
		return string(s)
	}
	fmt.Fprintf(&b, "url       %s  (from %s)\n", w.URL, from(w.Sources.URL))
	fmt.Fprintf(&b, "user      %s  (from %s)\n", w.User, from(w.Sources.User))
	pw := "not found"
	if w.HasPassword {
		pw = "set  (from " + from(w.Sources.Password) + ")"
	}
	fmt.Fprintf(&b, "password  %s\n", pw)
	if w.Insecure {
		b.WriteString("insecure  true\n")
	}
	_, err := io.WriteString(out, b.String())
	return err
}

func newConfigWhoamiCmd(app *App) *cobra.Command {
	var f connFlags
	cmd := &cobra.Command{
		Use:   "whoami",
		Short: "Show which URL and user a command would use, and where they come from",
		Long: `Show the URL, user and password source that commands calling the API would
use, with the same flags and environment variables. It never shows the password
and never asks for one.`,
		Example: `  openarch config whoami
  openarch config whoami --context prod --output json`,
		Args: noArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			dir, err := workspace.Find(app.Opts.Dir)
			if err != nil {
				return err
			}
			cfg, err := app.contexts()
			if err != nil {
				return err
			}
			creds, err := auth.Resolve(auth.Input{
				URL: f.url, User: f.user, Password: f.password, Insecure: f.insecure,
				WorkspaceDir: dir, Context: app.Opts.Context, Contexts: &cfg, Store: app.store,
				AllowNoPassword: true,
			})
			if err != nil {
				return err
			}
			return app.Printer.Print(Whoami{Context: creds.Context, Workspace: creds.Workspace, URL: creds.URL, User: creds.User,
				HasPassword: creds.Password != "", Insecure: creds.Insecure, Sources: creds.Sources})
		},
	}
	f.register(cmd)
	_ = cmd.PersistentFlags().MarkHidden("password-stdin")
	return cmd
}
