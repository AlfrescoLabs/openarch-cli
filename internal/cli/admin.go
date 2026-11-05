package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/AlfrescoLabs/openarch-cli/internal/api"
	"github.com/AlfrescoLabs/openarch-cli/internal/localenv"
	"github.com/AlfrescoLabs/openarch-cli/internal/output"
	"github.com/AlfrescoLabs/openarch-cli/internal/term"
	"github.com/AlfrescoLabs/openarch-cli/internal/users"
	"github.com/AlfrescoLabs/openarch-cli/internal/workspace"
)

func newQueryCmd(app *App) *cobra.Command {
	var f connFlags
	var o api.QueryOptions
	cmd := &cobra.Command{
		Use:   "query <query>",
		Short: "Run a query and list the documents found",
		Long: `Run a query with POST /api/query and list the documents found.

The default syntax is HxQL, a SQL-like language, for example:
  SELECT * FROM SysFolder WHERE sys_parentPath = '/'
Use --syntax expr for the expression syntax.

New documents can take a moment to reach the search index. --sync waits for it.
With --output json, the result is the server response as is.`,
		Example: `  openarch query "SELECT * FROM SysContent"
  openarch query "SELECT * FROM SysFolder" --limit 50 --sort sys_title
  openarch query "SELECT * FROM SysContent" --sync --total --output json`,
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if o.Syntax != "" && o.Syntax != "hxql" && o.Syntax != "expr" {
				return output.Usage(fmt.Sprintf("invalid syntax %q", o.Syntax), "Use hxql or expr.")
			}
			c, err := app.client(cmd, &f)
			if err != nil {
				return err
			}
			o.Query = args[0]
			res, err := c.Query(cmd.Context(), o)
			if err != nil {
				return err
			}
			return app.Printer.Print(res)
		},
	}
	f.register(cmd)
	fl := cmd.Flags()
	fl.Int64Var(&o.Limit, "limit", 10, "maximum number of documents")
	fl.Int64Var(&o.Offset, "offset", 0, "number of documents to skip")
	fl.StringSliceVar(&o.Sort, "sort", nil, "sort by these fields (repeatable)")
	fl.StringVar(&o.Syntax, "syntax", "", "query syntax: hxql (default) or expr")
	fl.BoolVar(&o.Sync, "sync", false, "wait until the search index has the latest changes")
	fl.BoolVar(&o.Total, "total", false, "count every match, not only up to the server limit")
	return cmd
}

func newDocumentsCmd(app *App) *cobra.Command {
	var f connFlags
	cmd := &cobra.Command{
		Use:   "documents",
		Short: "Get, create, list, upload and download documents",
		Long: `Get, create, list, upload and download documents. A document is given by
its id, or by its path when the value starts with "/" (the root is "/").

With --output json, a document is the server response as is.`,
		Example: `  openarch documents get /
  openarch documents list /projects
  openarch documents create --parent / --type SysFolder --name projects
  openarch documents upload report.pdf --parent /projects
  openarch documents content /projects/report.pdf --out report.pdf`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	f.register(cmd)
	cmd.AddCommand(&cobra.Command{
		Use:   "get <id-or-path>",
		Short: "Show a document",
		Example: `  openarch documents get /
  openarch documents get /projects/report --output json
  openarch documents get dae8e1bb-ef08-44e5-81ae-451d785a3643`,
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := app.client(cmd, &f)
			if err != nil {
				return err
			}
			doc, err := c.GetDocument(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return app.Printer.Print(doc)
		},
	})
	cmd.AddCommand(newDocumentsCreateCmd(app, &f), newDocumentsListCmd(app, &f),
		newDocumentsUploadCmd(app, &f), newDocumentsUpdateCmd(app, &f), newDocumentsContentCmd(app, &f))
	return cmd
}

func newDocumentsCreateCmd(app *App, f *connFlags) *cobra.Command {
	var parent, file, typ, name, title, description string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a document under a parent",
		Long: `Create a document under a parent, given by id or path.

The document is a JSON object from --file (use - for stdin), or built from
--type, --name, --title and --description. These flags also change the values
of --file.
sys_primaryType is required, for example SysFolder.`,
		Example: `  openarch documents create --parent / --type SysFolder --name projects --title Projects
  openarch documents create --parent /projects --file report.json
  echo '{"sys_primaryType":"SysFolder","sys_name":"x"}' | openarch documents create --parent / --file -`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if parent == "" {
				return output.Usage("--parent is required", "Use an id, or a path such as /.")
			}
			body := map[string]any{}
			if file != "" {
				var data []byte
				var err error
				if file == "-" {
					data, err = io.ReadAll(io.LimitReader(app.Streams.In, 64<<20))
				} else {
					data, err = os.ReadFile(file)
				}
				if err != nil {
					return output.Usage("cannot read --file: "+err.Error(), "")
				}
				if err := json.Unmarshal(data, &body); err != nil {
					return output.Usage("--file is not a JSON object: "+err.Error(), "")
				}
			}
			for k, v := range map[string]string{"sys_primaryType": typ, "sys_name": name, "sys_title": title, "sys_description": description} {
				if v != "" {
					body[k] = v
				}
			}
			if _, ok := body["sys_primaryType"]; !ok {
				return output.Usage("the document has no sys_primaryType", "Use --type, for example --type SysFolder.")
			}
			data, err := json.Marshal(body)
			if err != nil {
				return err
			}
			c, err := app.client(cmd, f)
			if err != nil {
				return err
			}
			doc, err := c.CreateDocument(cmd.Context(), parent, data)
			if err != nil {
				return err
			}
			return app.Printer.Print(doc)
		},
	}
	fl := cmd.Flags()
	fl.StringVar(&parent, "parent", "", "parent document: id, or path starting with /")
	fl.StringVar(&file, "file", "", "JSON file with the document, or - for stdin")
	fl.StringVar(&typ, "type", "", "sys_primaryType, for example SysFolder")
	fl.StringVar(&name, "name", "", "sys_name")
	fl.StringVar(&title, "title", "", "sys_title")
	fl.StringVar(&description, "description", "", "sys_description")
	return cmd
}

func newDocumentsUpdateCmd(app *App, f *connFlags) *cobra.Command {
	var file, title, description string
	cmd := &cobra.Command{
		Use:   "update <id-or-path>",
		Short: "Change the properties of a document",
		Long: `Change the properties of a document, given by id or path.

The changes are a JSON object from --file (use - for stdin), or built from
--title and --description. These flags also change the values of --file.
Properties that are not in the changes keep their value.`,
		Example: `  openarch documents update /projects/report.pdf --description "Final version"
  openarch documents update dae8e1bb-ef08-44e5-81ae-451d785a3643 --title "Report 2026"
  echo '{"sys_description":"checked"}' | openarch documents update /projects/report.pdf --file -`,
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]any{}
			if file != "" {
				var data []byte
				var err error
				if file == "-" {
					data, err = io.ReadAll(io.LimitReader(app.Streams.In, 64<<20))
				} else {
					data, err = os.ReadFile(file)
				}
				if err != nil {
					return output.Usage("cannot read --file: "+err.Error(), "")
				}
				if err := json.Unmarshal(data, &body); err != nil {
					return output.Usage("--file is not a JSON object: "+err.Error(), "")
				}
			}
			if cmd.Flags().Changed("title") {
				body["sys_title"] = title
			}
			if cmd.Flags().Changed("description") {
				body["sys_description"] = description
			}
			if len(body) == 0 {
				return output.Usage("nothing to change", "Use --title, --description or --file.")
			}
			data, err := json.Marshal(body)
			if err != nil {
				return err
			}
			c, err := app.client(cmd, f)
			if err != nil {
				return err
			}
			doc, err := c.UpdateDocument(cmd.Context(), args[0], data)
			if err != nil {
				return err
			}
			return app.Printer.Print(doc)
		},
	}
	fl := cmd.Flags()
	fl.StringVar(&file, "file", "", "JSON file with the changes, or - for stdin")
	fl.StringVar(&title, "title", "", "new sys_title")
	fl.StringVar(&description, "description", "", "new sys_description")
	return cmd
}

// usersWorkspace loads the workspace for the users commands.
func (a *App) usersWorkspace() (string, workspace.Config, users.File, error) {
	dir, err := workspace.Find(a.Opts.Dir)
	if err != nil {
		return "", workspace.Config{}, users.File{}, err
	}
	cfg, err := workspace.Load(filepath.Join(dir, workspace.ConfigFile))
	if err != nil {
		return "", cfg, users.File{}, err
	}
	if cfg.Security.DefaultUsers {
		return "", cfg, users.File{}, output.Usage("this workspace uses the users bundled in the OpenArch image (security.defaultUsers)",
			"Set security.defaultUsers to false in openarch.yaml and run 'openarch render' to manage users.")
	}
	f, err := users.Load(dir)
	if err != nil {
		return "", cfg, f, err
	}
	if len(f.Users) == 0 {
		return "", cfg, f, output.Usage(users.Path+" not found or empty in "+dir, "Run 'openarch render' to write it.")
	}
	return dir, cfg, f, nil
}

// userPassword reads a new password from stdin or a hidden prompt.
func (a *App) userPassword(stdin bool, username string) (string, error) {
	var pw string
	var err error
	switch {
	case stdin:
		pw, err = readSecret(a.Streams.In)
	case a.Interactive():
		pw, err = term.ReadPassword(a.Streams.In, a.Streams.Err, "New password for "+username+": ")
		if err == nil {
			var again string
			again, err = term.ReadPassword(a.Streams.In, a.Streams.Err, "Repeat it: ")
			if err == nil && again != pw {
				return "", output.Usage("the passwords do not match", "")
			}
		}
	default:
		return "", output.Usage("no password", "Use --password-stdin, or run in a terminal to be asked.")
	}
	if err != nil {
		return "", err
	}
	if len(pw) < 8 {
		return "", output.Usage("the password is too short", "Use at least 8 characters.")
	}
	return pw, nil
}

// UserView is a user as shown by the users commands. It never has the password.
type UserView struct {
	Username  string   `json:"username"`
	ID        string   `json:"id"`
	Admin     bool     `json:"admin"`
	Groups    []string `json:"groups"`
	Email     string   `json:"email,omitempty"`
	FirstName string   `json:"firstName,omitempty"`
	LastName  string   `json:"lastName,omitempty"`
	// Workspace marks the admin user of the workspace, whose password is in .env.local.
	Workspace bool `json:"workspaceAdmin,omitempty"`
}

// UserList is the result of 'users list'.
type UserList struct {
	Users []UserView `json:"users"`
}

func (l UserList) WriteText(w io.Writer) error {
	var b strings.Builder
	width := 8
	for _, u := range l.Users {
		width = max(width, len(u.Username))
	}
	for _, u := range l.Users {
		role := "member"
		if u.Admin {
			role = "admin"
		}
		fmt.Fprintf(&b, "%-*s  %-6s  %s  %s\n", width, u.Username, role, strings.Join(u.Groups, ","), u.Email)
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// UserChange is the result of 'users add', 'set-password' and 'delete'.
type UserChange struct {
	Action   string `json:"action"`
	Username string `json:"username"`
	// Restart is true: OpenArch reads the users file only at startup.
	Restart bool `json:"restartNeeded"`
}

func (c UserChange) WriteText(w io.Writer) error {
	verb := map[string]string{"add": "Added user", "set-password": "Changed the password of", "delete": "Deleted user"}[c.Action]
	_, err := fmt.Fprintf(w, "%s %s. Run 'openarch up' to apply the change: OpenArch reads its users at startup.\n", verb, c.Username)
	return err
}

func view(f users.File, u users.User, adminName string) UserView {
	return UserView{Username: u.Username, ID: u.ID, Admin: f.IsAdmin(u), Groups: u.Groups, Email: u.Email,
		FirstName: u.FirstName, LastName: u.LastName, Workspace: u.ID == users.AdminID && u.Username == adminName}
}

func newUsersCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "users",
		Short: "Manage the OpenArch users of a workspace",
		Long: `Manage the OpenArch users of a workspace.

OpenArch has no API to manage users. It reads them from config/users.yml when
it starts. These commands change that file (passwords as bcrypt hashes), and
'openarch up' then restarts OpenArch to apply the change.

The admin user belongs to the workspace: its password is in .env.local, and
'users set-password admin' changes both files. The passwords of other users are
stored nowhere else.`,
		Example: `  openarch users list
  openarch users add alice
  openarch users set-password alice`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(&cobra.Command{
		Use:     "list",
		Short:   "List the users",
		Example: "  openarch users list --output json",
		Args:    noArgs,
		RunE: func(*cobra.Command, []string) error {
			dir, _, f, err := app.usersWorkspace()
			if err != nil {
				return err
			}
			secrets := readLocalSecrets(dir)
			l := UserList{Users: []UserView{}}
			for _, u := range f.Users {
				l.Users = append(l.Users, view(f, u, secrets.AdminUser))
			}
			return app.Printer.Print(l)
		},
	})
	cmd.AddCommand(newUsersAddCmd(app), newUsersSetPasswordCmd(app), newUsersDeleteCmd(app))
	return cmd
}

func readLocalSecrets(dir string) localenv.Env {
	data, _ := os.ReadFile(filepath.Join(dir, localenv.File))
	return localenv.Parse(data)
}

func newUsersAddCmd(app *App) *cobra.Command {
	var passwordStdin, admin bool
	var groups []string
	var first, last, email string
	cmd := &cobra.Command{
		Use:   "add <username>",
		Short: "Add a user",
		Long: `Add a user to config/users.yml. The user is in the members group, and also in
administrators with --admin. Other groups given with --group are created.`,
		Example: `  openarch users add alice
  printf '%s' "$PASSWORD" | openarch users add bob --admin --email bob@example.com --password-stdin`,
		Args: exactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			name := args[0]
			if err := users.ValidateUsername(name); err != nil {
				return err
			}
			dir, _, f, err := app.usersWorkspace()
			if err != nil {
				return err
			}
			if f.Find(name) >= 0 {
				return output.Usage(fmt.Sprintf("user %q already exists", name), "Use 'openarch users set-password' to change the password.")
			}
			pw, err := app.userPassword(passwordStdin, name)
			if err != nil {
				return err
			}
			hash, err := users.Hash(pw)
			if err != nil {
				return err
			}
			g := []string{users.MembersGroup}
			if admin {
				g = append(g, users.AdministratorsGroup)
			}
			for _, x := range groups {
				if !slices.Contains(g, x) {
					g = append(g, x)
				}
			}
			for _, x := range g {
				f.EnsureGroup(x)
			}
			f.Users = append(f.Users, users.User{ID: users.NewID(), Username: name, Password: hash,
				FirstName: first, LastName: last, Email: email, Groups: g})
			if err := users.Save(dir, f); err != nil {
				return err
			}
			return app.Printer.Print(UserChange{Action: "add", Username: name, Restart: true})
		},
	}
	fl := cmd.Flags()
	fl.BoolVar(&passwordStdin, "password-stdin", false, "read the password from stdin")
	fl.BoolVar(&admin, "admin", false, "add the user to the administrators group")
	fl.StringSliceVar(&groups, "group", nil, "add the user to this group (repeatable)")
	fl.StringVar(&first, "first-name", "", "first name")
	fl.StringVar(&last, "last-name", "", "last name")
	fl.StringVar(&email, "email", "", "email address")
	return cmd
}

func newUsersSetPasswordCmd(app *App) *cobra.Command {
	var passwordStdin bool
	cmd := &cobra.Command{
		Use:   "set-password <username>",
		Short: "Change the password of a user",
		Long: `Change the password of a user in config/users.yml. For the workspace admin,
the password in .env.local changes too, so the CLI keeps working.`,
		Example: `  openarch users set-password alice
  printf '%s' "$PASSWORD" | openarch users set-password admin --password-stdin`,
		Args: exactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			name := args[0]
			dir, _, f, err := app.usersWorkspace()
			if err != nil {
				return err
			}
			i := f.Find(name)
			if i < 0 {
				return output.Usage(fmt.Sprintf("unknown user %q", name), "Run 'openarch users list'.")
			}
			pw, err := app.userPassword(passwordStdin, name)
			if err != nil {
				return err
			}
			hash, err := users.Hash(pw)
			if err != nil {
				return err
			}
			f.Users[i].Password = hash
			secrets := readLocalSecrets(dir)
			if f.Users[i].ID == users.AdminID && name == secrets.AdminUser {
				secrets.AdminPassword = pw
				if err := os.WriteFile(filepath.Join(dir, localenv.File), secrets.Marshal(), 0o600); err != nil {
					return err
				}
			}
			if err := users.Save(dir, f); err != nil {
				return err
			}
			return app.Printer.Print(UserChange{Action: "set-password", Username: name, Restart: true})
		},
	}
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "read the password from stdin")
	return cmd
}

func newUsersDeleteCmd(app *App) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:     "delete <username>",
		Short:   "Delete a user",
		Long:    "Delete a user from config/users.yml. The workspace admin cannot be deleted. Without a terminal, --yes is required.",
		Example: "  openarch users delete alice --yes",
		Args:    exactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			name := args[0]
			dir, _, f, err := app.usersWorkspace()
			if err != nil {
				return err
			}
			i := f.Find(name)
			if i < 0 {
				return output.Usage(fmt.Sprintf("unknown user %q", name), "Run 'openarch users list'.")
			}
			if f.Users[i].ID == users.AdminID {
				return output.Usage("the workspace admin cannot be deleted", "Change its password with 'openarch users set-password'.")
			}
			if err := app.confirm(yes, fmt.Sprintf("Delete user %q?", name)); err != nil {
				return err
			}
			f.Users = slices.Delete(f.Users, i, i+1)
			if err := users.Save(dir, f); err != nil {
				return err
			}
			return app.Printer.Print(UserChange{Action: "delete", Username: name, Restart: true})
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "do not ask for confirmation")
	return cmd
}
