package cli

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/AlfrescoLabs/openarch-cli/internal/api"
	"github.com/AlfrescoLabs/openarch-cli/internal/contexts"
	"github.com/AlfrescoLabs/openarch-cli/internal/demo"
	"github.com/AlfrescoLabs/openarch-cli/internal/localenv"
	"github.com/AlfrescoLabs/openarch-cli/internal/output"
	"github.com/AlfrescoLabs/openarch-cli/internal/users"
	"github.com/AlfrescoLabs/openarch-cli/internal/version"
)

// demoPasswordLength is the length of the generated demo passwords.
const demoPasswordLength = 20

func newDemoCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "demo",
		Short: "Add sample users and documents that show permission-aware search",
		Long: `Add sample users and documents that show permission-aware search: the
same query returns different documents for each user.

1. 'demo users' adds the users alice (group demo-hr), bob (group
   demo-finance) and carol (no extra group) with generated passwords, and
   saves a context for each one.
2. 'openarch up' starts OpenArch, or restarts it: it reads its users only at
   startup.
3. 'demo load' creates folders and documents under /Demo with different read
   access. 'demo reset' deletes them.
4. 'demo check' logs in as each user and checks that each one gets only the
   documents they may read.`,
		Example: `  openarch demo users
  openarch up
  openarch demo load
  openarch query "SELECT * FROM SysFile WHERE sys_path STARTSWITH '/Demo/'" --context local-alice
  openarch query "SELECT * FROM SysFile WHERE sys_path STARTSWITH '/Demo/'" --context local-bob
  openarch query "SELECT * FROM SysFile WHERE sys_path STARTSWITH '/Demo/'" --context local-carol
  openarch demo check`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(newDemoUsersCmd(app), newDemoLoadCmd(app), newDemoCheckCmd(app), newDemoResetCmd(app))
	return cmd
}

// checkVectorTimeout is how long 'demo check --semantic' waits for new vectors
// to reach the search index.
const checkVectorTimeout = 30 * time.Second

func newDemoCheckCmd(app *App) *cobra.Command {
	var f connFlags
	var m modelFlags
	var semantic bool
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Check that each demo user gets only the documents they may read",
		Long: `Check that each demo user gets only the documents they may read. The check
logs in as the workspace admin and as alice, bob and carol, with the passwords
that 'demo users' stored, and compares what OpenArch returns with the access
entries of the demo data: document lists from the search index and from the
database, title queries, and reads of every document by id.

Each demo document has a unique code in its description. A response with the
code, the name or the content of a document the user may not read is a leak.
A document the user may read but does not get is a problem.

--semantic also asks five questions with vector search (see 'demo load
--embeddings'). The questions become vectors with a local model on Docker
Model Runner.

The last line sums up the result and the versions, and can be shared as is.
Exit code 0 means no leak and no problem, 4 means at least one.`,
		Example: `  openarch demo check
  openarch demo check --semantic
  openarch demo check --output json`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, cfg, uf, err := app.usersWorkspace()
			if err != nil {
				return err
			}
			for _, u := range demo.Users {
				if !demo.HasGroups(uf, u) {
					return output.NewError(output.ExitPrerequisite, "demo user "+u.Name+" is not in "+users.Path,
						"Run 'openarch demo users', then 'openarch up'.")
				}
			}
			admin, err := app.client(cmd, &f)
			if err != nil {
				return err
			}
			st, err := app.store()
			if err != nil {
				return err
			}
			// 'demo users' stores the passwords under the workspace URL.
			url := strings.TrimSuffix(cfg.URL(), "/")
			o := demo.CheckOptions{Admin: admin, Wait: admin.WaitForIndexing, Users: map[string]demo.Reader{}}
			for _, u := range demo.Users {
				pw, _ := st.Get(url, u.Name)
				if pw == "" {
					return output.NewError(output.ExitPrerequisite, "no stored password for demo user "+u.Name,
						"Run 'openarch demo users', then 'openarch up'.")
				}
				c, err := admin.As(u.Name, pw, app.Log)
				if err != nil {
					return err
				}
				o.Users[u.Name] = c
			}
			if semantic {
				o.Embed, o.EmbeddingType = m.embedder(), api.EmbeddingType(m.model)
				o.VectorTimeout, o.PollInterval = checkVectorTimeout, time.Second
			}
			rep, err := demo.Check(cmd.Context(), o)
			if err != nil {
				return err
			}
			rep.CLIVersion = version.Get().Version
			if p, err := admin.Ping(cmd.Context()); err == nil {
				rep.OpenArchVersion = p.Version
			}
			if err := app.Printer.Print(rep); err != nil {
				return err
			}
			if !rep.Passed {
				return output.Exit(output.ExitUnhealthy, "demo check failed")
			}
			return nil
		},
	}
	f.register(cmd)
	cmd.Flags().BoolVar(&semantic, "semantic", false, "also check vector search, with a local model")
	m.register(cmd)
	return cmd
}

// demoClient returns an admin client for the demo commands, and the user and
// group ids of the workspace users file.
func (a *App) demoClient(cmd *cobra.Command, f *connFlags) (*api.Client, demo.IDs, error) {
	_, _, uf, err := a.usersWorkspace()
	if err != nil {
		return nil, demo.IDs{}, err
	}
	c, err := a.client(cmd, f)
	if err != nil {
		return nil, demo.IDs{}, err
	}
	return c, demo.IDsOf(uf), nil
}

func newDemoLoadCmd(app *App) *cobra.Command {
	var f connFlags
	var m modelFlags
	var force, embeddings bool
	cmd := &cobra.Command{
		Use:   "load",
		Short: "Create the demo folders and documents",
		Long: `Create the demo folders and documents under /Demo, as the workspace admin.
Public can be read by every user, HR by demo-hr, Finance by demo-finance, and
Board by administrators only. Some folders and documents have their own access
entries: Public/Facilities blocks what it inherits and gives bob access, a
Finance document adds alice, and another one gave alice access that has ended.
The documents have metadata only (title and description).

--embeddings also stores a vector for each document, made from its title and
description with a local model on Docker Model Runner, so that
'openarch query --semantic' finds documents by meaning. OpenArch keeps the
vector with the access entries of its document.

The demo users must exist in OpenArch: run 'openarch demo users', then
'openarch up'. When /Demo exists, nothing is loaded; --force deletes it first.`,
		Example: `  openarch demo load
  openarch demo load --force --output json
  openarch demo load --force --embeddings`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, ids, err := app.demoClient(cmd, &f)
			if err != nil {
				return err
			}
			opts := demo.LoadOptions{Force: force}
			if embeddings {
				opts.Embed, opts.EmbeddingType = m.embedder(), api.EmbeddingType(m.model)
			}
			res, err := demo.Load(cmd.Context(), c, ids, opts)
			if err != nil {
				return err
			}
			return app.Printer.Print(res)
		},
	}
	f.register(cmd)
	cmd.Flags().BoolVar(&force, "force", false, "delete /Demo first when it exists")
	cmd.Flags().BoolVar(&embeddings, "embeddings", false, "also store a vector for each document, made with a local model")
	m.register(cmd)
	return cmd
}

func newDemoResetCmd(app *App) *cobra.Command {
	var f connFlags
	var yes bool
	cmd := &cobra.Command{
		Use:     "reset",
		Short:   "Delete the demo folders and documents",
		Long:    "Delete /Demo and everything in it. The demo users stay. Without a terminal, --yes is required.",
		Example: "  openarch demo reset --yes",
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, _, err := app.demoClient(cmd, &f)
			if err != nil {
				return err
			}
			if err := app.confirm(yes, "Delete "+demo.Root+" and everything in it?"); err != nil {
				return err
			}
			res, err := demo.Reset(cmd.Context(), c)
			if err != nil {
				return err
			}
			return app.Printer.Print(res)
		},
	}
	f.register(cmd)
	cmd.Flags().BoolVar(&yes, "yes", false, "do not ask for confirmation")
	return cmd
}

// DemoUser is a demo user in the result of 'demo users'. It never has the password.
type DemoUser struct {
	Username string `json:"username"`
	// Groups are the groups of the user other than members.
	Groups  []string `json:"groups"`
	Context string   `json:"context"`
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
		groups := "no extra group"
		if len(u.Groups) > 0 {
			groups = "group " + strings.Join(u.Groups, ", ")
		}
		fmt.Fprintf(&b, "%-9s  %-5s  %s, context %s\n", u.Action, u.Username, groups, u.Context)
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
		Long: `Add the demo users alice, bob and carol to config/users.yml, with
generated passwords. Each user gets a context named <prefix>-<user>, with the password in
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
				res.Users = append(res.Users, DemoUser{Username: u.Name, Groups: u.ExtraGroups(), Context: names[u.Name], Action: action})
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
