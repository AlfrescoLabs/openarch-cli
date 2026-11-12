package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/AlfrescoLabs/openarch-cli/internal/contexts"
	"github.com/AlfrescoLabs/openarch-cli/internal/demo"
	"github.com/AlfrescoLabs/openarch-cli/internal/localenv"
	"github.com/AlfrescoLabs/openarch-cli/internal/output"
	"github.com/AlfrescoLabs/openarch-cli/internal/users"
)

// demoPasswordLength is the length of the generated demo passwords.
const demoPasswordLength = 20

func newDemoCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "demo",
		Short: "Add sample users and documents that show permission-aware search",
		Long: `Add sample users and documents that show permission-aware search: the
same query returns different documents for each user.

'demo users' adds the users alice (group demo-hr) and bob (group demo-finance)
with generated passwords, and saves a context for each one. OpenArch reads its
users only at startup, so run it before 'openarch up', or run 'openarch up'
after it.`,
		Example: `  openarch demo users
  openarch up
  openarch query "SELECT * FROM SysFile" --context local-alice`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(newDemoUsersCmd(app))
	return cmd
}

// DemoUser is a demo user in the result of 'demo users'. It never has the password.
type DemoUser struct {
	Username string `json:"username"`
	Group    string `json:"group"`
	Context  string `json:"context"`
	// Action is "added", "updated" or "unchanged".
	Action string `json:"action"`
}

// DemoUsers is the result of 'demo users'.
type DemoUsers struct {
	Users []DemoUser `json:"users"`
	// Restart is true when the users file changed: OpenArch reads it only at startup.
	Restart bool `json:"restartNeeded"`
}

func (r DemoUsers) WriteText(w io.Writer) error {
	var b strings.Builder
	for _, u := range r.Users {
		fmt.Fprintf(&b, "%-9s  %-5s  group %s, context %s\n", u.Action, u.Username, u.Group, u.Context)
	}
	if r.Restart {
		b.WriteString("Run 'openarch up' to apply the change: OpenArch reads its users at startup.\n")
	}
	b.WriteString("Show the passwords with 'openarch credentials'.\n")
	_, err := io.WriteString(w, b.String())
	return err
}

func newDemoUsersCmd(app *App) *cobra.Command {
	var prefix string
	cmd := &cobra.Command{
		Use:   "users",
		Short: "Add the demo users and a context for each one",
		Long: `Add the demo users alice and bob to config/users.yml, with generated
passwords. Each user gets a context named <prefix>-<user>, with the password in
the credential store, for use with --context.

The prefix is the name of the workspace context, for example local. A user
that already has a working stored password keeps it, so the command can run
again. Workspaces with the users bundled in the OpenArch image
(security.defaultUsers) cannot have demo users.`,
		Example: `  openarch demo users
  openarch demo users --context-prefix demo --output json`,
		Args: noArgs,
		RunE: func(*cobra.Command, []string) error {
			dir, cfg, f, err := app.usersWorkspace()
			if err != nil {
				return err
			}
			ctxs, err := app.contexts()
			if err != nil {
				return err
			}
			if prefix == "" {
				prefix = ctxs.NameForWorkspace(dir)
			}
			if prefix == "" {
				prefix = cfg.Name
			}
			url := strings.TrimSuffix(cfg.URL(), "/")
			names := map[string]string{}
			for _, u := range demo.Users {
				name := prefix + "-" + u.Name
				if err := contexts.ValidateName(name); err != nil {
					return err
				}
				if c, ok := ctxs.Contexts[name]; ok {
					ep := c.Endpoints[contexts.SystemOpenArch]
					if c.Workspace != "" || ep.URL != url || ep.Username != u.Name {
						return output.Usage(fmt.Sprintf("context %q already exists and points somewhere else", name),
							"Use another --context-prefix, or delete it with 'openarch config delete "+name+"'.")
					}
				}
				names[u.Name] = name
			}
			st, err := app.store()
			if err != nil {
				return err
			}
			res := DemoUsers{Users: []DemoUser{}}
			for _, u := range demo.Users {
				action := "unchanged"
				pw, _ := st.Get(url, u.Name)
				i := f.Find(u.Name)
				// A stored password that matches the users file is kept.
				works := i >= 0 && pw != "" && localenv.Matches(users.HashOf(f.Users[i].Password), pw)
				if !works || !demo.HasGroups(f, u) {
					action = "added"
					if i >= 0 {
						action = "updated"
					}
					hash := ""
					if works {
						hash = f.Users[i].Password
					} else {
						pw = localenv.Password(demoPasswordLength)
						if hash, err = users.Hash(pw); err != nil {
							return err
						}
						if err := st.Set(url, u.Name, pw); err != nil {
							return err
						}
					}
					demo.AddUser(&f, u, hash)
					res.Restart = true
				}
				if ctxs.Contexts == nil {
					ctxs.Contexts = map[string]contexts.Context{}
				}
				ctxs.Contexts[names[u.Name]] = contexts.Context{Endpoints: map[string]contexts.Endpoint{
					contexts.SystemOpenArch: {URL: url, Username: u.Name},
				}}
				res.Users = append(res.Users, DemoUser{Username: u.Name, Group: u.Group, Context: names[u.Name], Action: action})
			}
			if res.Restart {
				if err := users.Save(dir, f); err != nil {
					return err
				}
			}
			if err := app.saveContexts(ctxs); err != nil {
				return err
			}
			return app.Printer.Print(res)
		},
	}
	cmd.Flags().StringVar(&prefix, "context-prefix", "", "prefix of the context names (default: the name of the workspace context)")
	return cmd
}
