package cli

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/AlfrescoLabs/openarch-cli/internal/contexts"
	"github.com/AlfrescoLabs/openarch-cli/internal/workspace"
)

// Credential is a user and its password, in the result of 'credentials'.
type Credential struct {
	Username string `json:"username"`
	Password string `json:"password"`
	// Context is the context that uses these credentials, if any.
	Context string `json:"context,omitempty"`
	Admin   bool   `json:"admin,omitempty"`
}

// Credentials is the result of 'credentials'.
type Credentials struct {
	URL   string       `json:"url"`
	Users []Credential `json:"users"`
}

func (c Credentials) WriteText(w io.Writer) error {
	var b strings.Builder
	fmt.Fprintf(&b, "OpenArch URL: %s\n", c.URL)
	for _, u := range c.Users {
		fmt.Fprintf(&b, "%-8s  %s", u.Username, u.Password)
		if u.Context != "" {
			fmt.Fprintf(&b, "  (context %s)", u.Context)
		}
		b.WriteString("\n")
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func newCredentialsCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "credentials",
		Short: "Show the users and passwords of a local workspace",
		Long: `Show the users and passwords of a local workspace: the admin user from
.env.local, and the users of the contexts that point to the workspace URL,
such as the users of 'openarch demo users', from the credential store.

The passwords are shown in clear text. Use this on your own machine only.`,
		Example: `  openarch credentials
  openarch credentials --dir my-openarch --output json`,
		Args: noArgs,
		RunE: func(*cobra.Command, []string) error {
			dir, err := workspace.Find(app.Opts.Dir)
			if err != nil {
				return err
			}
			cfg, err := workspace.Load(filepath.Join(dir, workspace.ConfigFile))
			if err != nil {
				return err
			}
			ctxs, err := app.contexts()
			if err != nil {
				return err
			}
			secrets := readLocalSecrets(dir)
			url := strings.TrimSuffix(cfg.URL(), "/")
			res := Credentials{URL: url, Users: []Credential{{Username: secrets.AdminUser, Password: secrets.AdminPassword,
				Context: ctxs.NameForWorkspace(dir), Admin: true}}}
			st, err := app.store()
			if err != nil {
				return err
			}
			for _, name := range ctxs.Names() {
				c := ctxs.Contexts[name]
				ep, ok := c.Endpoints[contexts.SystemOpenArch]
				if c.Workspace != "" || !ok || strings.TrimSuffix(ep.URL, "/") != url {
					continue
				}
				pw, _ := st.Get(url, ep.Username)
				res.Users = append(res.Users, Credential{Username: ep.Username, Password: pw, Context: name})
			}
			app.Printer.Warn("The passwords are shown in clear text.")
			return app.Printer.Print(res)
		},
	}
}
