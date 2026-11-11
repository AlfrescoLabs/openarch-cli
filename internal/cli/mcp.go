package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/AlfrescoLabs/openarch-cli/internal/credstore"
	"github.com/AlfrescoLabs/openarch-cli/internal/localenv"
	"github.com/AlfrescoLabs/openarch-cli/internal/mcpserver"
	"github.com/AlfrescoLabs/openarch-cli/internal/output"
	"github.com/AlfrescoLabs/openarch-cli/internal/version"
)

// mcpTool names one command that the MCP server exposes.
type mcpTool struct {
	name     string
	title    string
	command  []string
	kind     mcpserver.Kind
	args     []mcpserver.Arg
	defaults map[string]string
	extra    []string
	// note comes before the command help in the tool description.
	note string
	// insecure are the flags that turn a security default off. The tool
	// accepts them only when the server runs with --allow-insecure.
	insecure []string
}

// mcpTools is the list of tools. Commands that need a password as input
// (config set, users add, users set-password) or that change the CLI itself
// (self-update, completion, skill) are not tools. config get is not a tool, because
// it can print secrets.
var mcpTools = []mcpTool{
	{name: "doctor", title: "Check the prerequisites", command: []string{"doctor"}, kind: mcpserver.ReadOnly},
	{name: "status", title: "Show the stack status", command: []string{"status"}, kind: mcpserver.ReadOnly},
	{name: "logs", title: "Show service logs", command: []string{"logs"}, kind: mcpserver.ReadOnly,
		args:     []mcpserver.Arg{{Name: "services", Description: "service names (default: all services)", Variadic: true}},
		defaults: map[string]string{"tail": "200"}},
	{name: "api_ping", title: "Check the OpenArch API", command: []string{"api", "ping"}, kind: mcpserver.ReadOnly},
	{name: "query", title: "Run a query", command: []string{"query"}, kind: mcpserver.ReadOnly,
		args: []mcpserver.Arg{{Name: "query", Description: "the query, for example SELECT * FROM SysFolder. Only SELECT * returns documents", Required: true}}},
	{name: "documents_get", title: "Get a document", command: []string{"documents", "get"}, kind: mcpserver.ReadOnly,
		args: []mcpserver.Arg{{Name: "id_or_path", Description: `document id, or path when it starts with "/" (the root is "/")`, Required: true}}},
	{name: "documents_list", title: "List the documents in a folder", command: []string{"documents", "list"}, kind: mcpserver.ReadOnly,
		args: []mcpserver.Arg{{Name: "folder", Description: `folder id, or path starting with "/" (default: the root "/")`}}},
	{name: "users_list", title: "List the users", command: []string{"users", "list"}, kind: mcpserver.ReadOnly},
	{name: "config_list", title: "List the saved contexts", command: []string{"config", "list"}, kind: mcpserver.ReadOnly},
	{name: "config_whoami", title: "Show the connection in use", command: []string{"config", "whoami"}, kind: mcpserver.ReadOnly},
	{name: "version", title: "Show the CLI version", command: []string{"version"}, kind: mcpserver.ReadOnly},

	{name: "init", title: "Create a workspace", command: []string{"init"}, kind: mcpserver.Write, note: mcpSecureDefaults,
		insecure: []string{"default-users", "insecure-backends", "bind-ip", "expose", "tls", "cert", "key", "from"}},
	{name: "render", title: "Write the workspace files again", command: []string{"render"}, kind: mcpserver.Write},
	{name: "up", title: "Start the stack", command: []string{"up"}, kind: mcpserver.Write},
	{name: "smoke_test", title: "Test OpenArch end to end", command: []string{"smoke-test"}, kind: mcpserver.Write},
	{name: "documents_create", title: "Create a document", command: []string{"documents", "create"}, kind: mcpserver.Write},
	{name: "documents_update", title: "Change the properties of a document", command: []string{"documents", "update"}, kind: mcpserver.Write,
		args: []mcpserver.Arg{{Name: "document", Description: "id, or path starting with /", Required: true}}},
	{name: "documents_upload", title: "Upload a file", command: []string{"documents", "upload"}, kind: mcpserver.Write,
		args: []mcpserver.Arg{{Name: "file", Description: "path of a local file; a relative path is relative to the folder where the MCP server runs", Required: true}}},
	{name: "upgrade", title: "Change the OpenArch version", command: []string{"upgrade"}, kind: mcpserver.Write},
	{name: "migrate", title: "Update openarch.yaml to the current schema", command: []string{"migrate"}, kind: mcpserver.Write},
	{name: "backup", title: "Back up the data", command: []string{"backup"}, kind: mcpserver.Write},

	{name: "down", title: "Stop the stack", command: []string{"down"}, kind: mcpserver.Destructive},
	{name: "clean", title: "Stop the stack and delete its data", command: []string{"clean"}, kind: mcpserver.Destructive,
		extra: []string{"--yes"}},
	{name: "restore", title: "Restore a backup", command: []string{"restore"}, kind: mcpserver.Destructive,
		args:  []mcpserver.Arg{{Name: "backup_folder", Description: "the folder made by backup", Required: true}},
		extra: []string{"--yes"}},
	{name: "secrets_rotate", title: "Give new passwords to the services", command: []string{"secrets", "rotate"}, kind: mcpserver.Destructive},
	{name: "users_delete", title: "Delete a user", command: []string{"users", "delete"}, kind: mcpserver.Destructive,
		args:  []mcpserver.Arg{{Name: "username", Description: "the user to delete", Required: true}},
		extra: []string{"--yes"}},
	{name: "config_delete", title: "Delete a saved context", command: []string{"config", "delete"}, kind: mcpserver.Destructive,
		args:  []mcpserver.Arg{{Name: "name", Description: "the context to delete", Required: true}},
		extra: []string{"--yes"}},
}

// mcpHiddenFlags are flags that tools never accept: the connection comes from
// the server pin, passwords never come from a tool call, the server answers
// the confirmations, and a tool call must end.
var mcpHiddenFlags = []string{"url", "user", "password", "password-stdin", "insecure", "admin-password-stdin", "yes", "follow"}

// mcpPort is the port of the examples. A --listen value with no host is a
// port on 127.0.0.1.
const mcpPort = "8765"

// mcpAddExample adds the HTTP server of the examples to Claude Code.
const mcpAddExample = `claude mcp add --transport http openarch http://127.0.0.1:` + mcpPort + mcpserver.Path +
	` --header "Authorization: Bearer $(openarch mcp token)"`

// The bearer token has one entry in the credential store, for every server.
const mcpTokenURL, mcpTokenUser = "mcp", "bearer-token"

// mcpHTTP are the flags of the Streamable HTTP transport.
type mcpHTTP struct {
	listen            string
	noAuth            bool
	allowRemoteNoAuth bool
}

func newMCPCmd(app *App) *cobra.Command {
	var allowDestructive, allowInsecure bool
	var maxResult int
	var h mcpHTTP
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Run an MCP server that exposes the commands as tools",
		Long: `Run a Model Context Protocol (MCP) server, so that an AI agent can install
and operate OpenArch with the same commands as the CLI.

The server runs on stdin and stdout. With --listen it serves Streamable HTTP
on host:port instead, at the path ` + mcpserver.Path + `, for remote agents or several
clients. A port alone means 127.0.0.1, so --listen ` + mcpPort + ` is 127.0.0.1:` + mcpPort + `.
It prints its URL, then runs until it is stopped. Every HTTP request needs the header "Authorization: Bearer
<token>"; 'openarch mcp token' prints the token. --no-auth turns the token
off. On an address that is not loopback, --no-auth also needs
--allow-remote-no-auth.

Each tool runs one command with --output json and returns its JSON result.
A failed command gives an error result with its exit code. Command progress
becomes MCP progress notifications, and logs go to stderr.

--dir and --context pin the server to one workspace or connection; the
default is the current folder. Tools never take passwords, and no tool
returns one.

Tools that stop services or delete data (down, clean, restore, secrets
rotate, users delete, config delete) exist only with --allow-destructive.

The init tool always keeps the secure defaults. Its options that can turn
security off (default-users, insecure-backends, bind-ip, expose, tls, cert,
key, from) exist only with --allow-insecure.

A tool result longer than --max-result-bytes is cut, with a note that names
the parameters for a smaller result (tail, limit, offset).`,
		Example: `  openarch mcp --dir my-openarch
  openarch mcp --dir my-openarch --allow-destructive
  claude mcp add openarch -- openarch mcp --dir /path/to/my-openarch
  openarch mcp --dir my-openarch --listen ` + mcpPort + `
  ` + mcpAddExample,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts := app.mcpOptions(cmd.Root(), allowDestructive, allowInsecure, maxResult)
			if h.listen == "" {
				if h.noAuth || h.allowRemoteNoAuth {
					return output.Usage("--no-auth and --allow-remote-no-auth need --listen", "")
				}
				return mcpserver.Serve(cmd.Context(), opts)
			}
			return app.serveMCPHTTP(cmd.Context(), opts, h)
		},
	}
	cmd.Flags().BoolVar(&allowDestructive, "allow-destructive", false, "add the tools that stop services or delete data")
	cmd.Flags().BoolVar(&allowInsecure, "allow-insecure", false, "let the init tool take the options that turn security off")
	cmd.Flags().IntVar(&maxResult, "max-result-bytes", defaultMaxResultBytes, "cut longer tool results, for models with a small context; 0: no limit")
	cmd.Flags().StringVar(&h.listen, "listen", "", "serve Streamable HTTP on host:port instead of stdio; a port alone means 127.0.0.1")
	cmd.Flags().BoolVar(&h.noAuth, "no-auth", false, "serve HTTP without the bearer token")
	cmd.Flags().BoolVar(&h.allowRemoteNoAuth, "allow-remote-no-auth", false, "allow --no-auth on an address that is not loopback")
	cmd.AddCommand(newMCPTokenCmd(app))
	return cmd
}

// mcpListening is the result printed when the HTTP server starts.
type mcpListening struct {
	URL  string `json:"url"`
	Auth string `json:"auth"` // "token" or "none"
}

func (l mcpListening) WriteText(w io.Writer) error {
	msg := "MCP server on " + l.URL + "\n"
	if l.Auth == "token" {
		msg += "Clients send the header \"Authorization: Bearer <token>\". 'openarch mcp token' prints the token.\n"
	}
	_, err := io.WriteString(w, msg)
	return err
}

func (a *App) serveMCPHTTP(ctx context.Context, opts mcpserver.Options, h mcpHTTP) error {
	addr := h.listen
	if _, err := strconv.ParseUint(addr, 10, 16); err == nil {
		addr = net.JoinHostPort("127.0.0.1", addr)
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return output.Usage(fmt.Sprintf("invalid --listen address %q: %v", h.listen, err),
			"Use host:port or a port, for example 127.0.0.1:"+mcpPort+" or "+mcpPort+".")
	}
	local := isLoopback(host)
	if h.noAuth && !local && !h.allowRemoteNoAuth {
		return output.Usage(fmt.Sprintf("refusing to serve %s without authentication: every client that reaches it could run the tools", addr),
			"Remove --no-auth, listen on 127.0.0.1, or add --allow-remote-no-auth.")
	}
	res := mcpListening{Auth: "none"}
	var token string
	if !h.noAuth {
		if token, err = a.mcpToken(false); err != nil {
			return err
		}
		res.Auth = "token"
	}
	l, err := (&net.ListenConfig{}).Listen(ctx, "tcp", addr)
	if err != nil {
		return output.NewError(output.ExitError, err.Error(), "Choose another address with --listen.")
	}
	switch {
	case h.noAuth:
		a.Printer.Warn("The MCP server has no authentication: every client that reaches it can run the tools.")
	case !local:
		a.Printer.Warn("The bearer token goes over plain HTTP: anyone on the network path can read it.")
	}
	res.URL = "http://" + l.Addr().String() + mcpserver.Path
	if err := a.Printer.Print(res); err != nil {
		_ = l.Close()
		return err
	}
	return mcpserver.ServeHTTP(ctx, l, opts, mcpserver.HTTPOptions{Token: token})
}

// isLoopback reports whether host is localhost or a loopback IP address.
func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// mcpToken returns the bearer token from the credential store. It makes a new
// one when there is none, or when rotate is set.
func (a *App) mcpToken(rotate bool) (string, error) {
	st, err := a.store()
	if err != nil {
		return "", err
	}
	if !rotate {
		token, err := st.Get(mcpTokenURL, mcpTokenUser)
		if err == nil {
			return token, nil
		}
		if !errors.Is(err, credstore.ErrNotFound) {
			return "", err
		}
	}
	token := localenv.Password(40)
	if err := st.Set(mcpTokenURL, mcpTokenUser, token); err != nil {
		return "", err
	}
	return token, nil
}

// mcpTokenResult is the result of 'mcp token'.
type mcpTokenResult struct {
	Token string `json:"token"`
}

func (r mcpTokenResult) WriteText(w io.Writer) error {
	_, err := io.WriteString(w, r.Token+"\n")
	return err
}

func newMCPTokenCmd(app *App) *cobra.Command {
	var rotate bool
	cmd := &cobra.Command{
		Use:   "token",
		Short: "Print the bearer token of 'openarch mcp --listen'",
		Long: `Print the bearer token that MCP clients send to 'openarch mcp --listen'.

The first call makes the token. It is kept in the credential store, like the
OpenArch passwords, and every HTTP server of this user uses it. --rotate makes
a new token; a running server keeps the old one until it starts again.`,
		Example: `  openarch mcp token
  openarch mcp token --rotate
  ` + mcpAddExample,
		Args: noArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			token, err := app.mcpToken(rotate)
			if err != nil {
				return err
			}
			return app.Printer.Print(mcpTokenResult{Token: token})
		},
	}
	cmd.Flags().BoolVar(&rotate, "rotate", false, "make a new token")
	return cmd
}

// mcpOptions builds the server options from the cobra tree.
// defaultMaxResultBytes keeps one tool result to about 4000 tokens.
const defaultMaxResultBytes = 16384

func (a *App) mcpOptions(root *cobra.Command, allowDestructive, allowInsecure bool, maxResult int) mcpserver.Options {
	var prefix []string
	if a.Opts.Dir != "" {
		prefix = append(prefix, "--dir", a.Opts.Dir)
	}
	if a.Opts.Context != "" {
		prefix = append(prefix, "--context", a.Opts.Context)
	}
	return mcpserver.Options{
		Name:             "openarch",
		Version:          version.Get().Version,
		Instructions:     mcpInstructions,
		Tools:            buildMCPTools(root, allowInsecure),
		Run:              a.runInProcess,
		Prefix:           prefix,
		AllowDestructive: allowDestructive,
		MaxResultBytes:   maxResult,
		Log:              a.Streams.Err,
	}
}

// mcpToolUse keeps models from writing tool results themselves instead of
// calling the tools.
const mcpToolUse = "Every fact must come from a tool call. Call one tool, wait for its real result, " +
	"then decide the next step. Never write a tool result yourself."

// mcpSecureDefaults keeps agents from turning security off on their own.
const mcpSecureDefaults = "Keep the defaults: they are secure. Leave out default-users, insecure-backends, " +
	"bind-ip, expose and the tls options unless the user names that option. " +
	"Words such as local, development or test are not a reason to change them."

const mcpInstructions = `These tools install and operate OpenArch with Docker Compose, in one workspace folder.
` + mcpToolUse + `
` + mcpSecureDefaults + `
To install: doctor, then init, then up (it waits until OpenArch is healthy), then smoke_test.
Every result is the JSON output of the matching openarch command. An error result starts with the exit code:
1 error, 2 bad usage, 3 prerequisite missing, 4 service not healthy, 5 remote API error, 6 authentication failed.
Follow the hint in the error. The admin password is in .env.local in the workspace; no tool returns it.`

func buildMCPTools(root *cobra.Command, allowInsecure bool) []mcpserver.Tool {
	var tools []mcpserver.Tool
	for _, t := range mcpTools {
		c := mustFind(root, t.command...)
		desc := c.Short
		if c.Long != "" {
			desc = c.Long
		}
		if t.note != "" {
			desc = t.note + "\n\n" + desc
		}
		var flags []mcpserver.Flag
		for _, f := range helpFlags(c.NonInheritedFlags()) {
			if slices.Contains(mcpHiddenFlags, f.Name) || !allowInsecure && slices.Contains(t.insecure, f.Name) {
				continue
			}
			flags = append(flags, mcpserver.Flag{Name: f.Name, Type: f.Type, Default: f.Default, Usage: f.Usage, Values: f.Values})
		}
		tools = append(tools, mcpserver.Tool{
			Name: t.name, Title: t.title, Description: desc, Command: t.command, Kind: t.kind,
			Flags: flags, Args: t.args, Defaults: t.defaults, Extra: t.extra,
		})
	}
	return tools
}

// runInProcess runs one CLI command in this process, with the same system
// access as the server.
func (a *App) runInProcess(ctx context.Context, args []string, progress io.Writer) (int, []byte) {
	var out bytes.Buffer
	sub := newApp(Streams{In: strings.NewReader(""), Out: &out, Err: progress})
	sub.doctorEnv, sub.generateEnv, sub.stackEnv, sub.selfUpdateEnv = a.doctorEnv, a.generateEnv, a.stackEnv, a.selfUpdateEnv
	sub.credStore = a.credStore
	return execute(ctx, sub, args), out.Bytes()
}
