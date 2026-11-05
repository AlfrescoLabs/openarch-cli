package cli

import (
	"bytes"
	"context"
	"io"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/AlfrescoLabs/openarch-cli/internal/mcpserver"
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
		args: []mcpserver.Arg{{Name: "query", Description: "the query, for example SELECT * FROM SysFolder", Required: true}}},
	{name: "documents_get", title: "Get a document", command: []string{"documents", "get"}, kind: mcpserver.ReadOnly,
		args: []mcpserver.Arg{{Name: "id_or_path", Description: `document id, or path when it starts with "/" (the root is "/")`, Required: true}}},
	{name: "documents_list", title: "List the documents in a folder", command: []string{"documents", "list"}, kind: mcpserver.ReadOnly,
		args: []mcpserver.Arg{{Name: "folder", Description: `folder id, or path starting with "/" (default: the root "/")`}}},
	{name: "users_list", title: "List the users", command: []string{"users", "list"}, kind: mcpserver.ReadOnly},
	{name: "config_list", title: "List the saved contexts", command: []string{"config", "list"}, kind: mcpserver.ReadOnly},
	{name: "config_whoami", title: "Show the connection in use", command: []string{"config", "whoami"}, kind: mcpserver.ReadOnly},
	{name: "version", title: "Show the CLI version", command: []string{"version"}, kind: mcpserver.ReadOnly},

	{name: "init", title: "Create a workspace", command: []string{"init"}, kind: mcpserver.Write, note: mcpSecureDefaults},
	{name: "render", title: "Write the workspace files again", command: []string{"render"}, kind: mcpserver.Write},
	{name: "up", title: "Start the stack", command: []string{"up"}, kind: mcpserver.Write},
	{name: "smoke_test", title: "Test OpenArch end to end", command: []string{"smoke-test"}, kind: mcpserver.Write},
	{name: "documents_create", title: "Create a document", command: []string{"documents", "create"}, kind: mcpserver.Write},
	{name: "documents_upload", title: "Upload a file", command: []string{"documents", "upload"}, kind: mcpserver.Write,
		args: []mcpserver.Arg{{Name: "file", Description: "path of a local file; a relative path is relative to the folder where the MCP server runs", Required: true}}},
	{name: "upgrade", title: "Change the OpenArch version", command: []string{"upgrade"}, kind: mcpserver.Write},
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

func newMCPCmd(app *App) *cobra.Command {
	var allowDestructive bool
	var maxResult int
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Run an MCP server that exposes the commands as tools",
		Long: `Run a Model Context Protocol (MCP) server on stdin and stdout, so that an AI
agent can install and operate OpenArch with the same commands as the CLI.

Each tool runs one command with --output json and returns its JSON result.
A failed command gives an error result with its exit code. Command progress
becomes MCP progress notifications, and logs go to stderr.

--dir and --context pin the server to one workspace or connection; the
default is the current folder. Tools never take passwords, and no tool
returns one.

Tools that stop services or delete data (down, clean, restore, secrets
rotate, users delete, config delete) exist only with --allow-destructive.

A tool result longer than --max-result-bytes is cut, with a note that names
the parameters for a smaller result (tail, limit, offset).`,
		Example: `  openarch mcp --dir my-openarch
  openarch mcp --dir my-openarch --allow-destructive
  claude mcp add openarch -- openarch mcp --dir /path/to/my-openarch`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return mcpserver.Serve(cmd.Context(), app.mcpOptions(cmd.Root(), allowDestructive, maxResult))
		},
	}
	cmd.Flags().BoolVar(&allowDestructive, "allow-destructive", false, "add the tools that stop services or delete data")
	cmd.Flags().IntVar(&maxResult, "max-result-bytes", defaultMaxResultBytes, "cut longer tool results, for models with a small context; 0: no limit")
	return cmd
}

// mcpOptions builds the server options from the cobra tree.
// defaultMaxResultBytes keeps one tool result to about 4000 tokens.
const defaultMaxResultBytes = 16384

func (a *App) mcpOptions(root *cobra.Command, allowDestructive bool, maxResult int) mcpserver.Options {
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
		Tools:            buildMCPTools(root),
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
const mcpSecureDefaults = "Keep the defaults: they are secure. Use default-users, insecure-backends, " +
	"bind-ip 0.0.0.0, expose or the tls options only when the user asks for them."

const mcpInstructions = `These tools install and operate OpenArch with Docker Compose, in one workspace folder.
` + mcpToolUse + `
` + mcpSecureDefaults + `
To install: doctor, then init, then up (it waits until OpenArch is healthy), then smoke_test.
Every result is the JSON output of the matching openarch command. An error result starts with the exit code:
1 error, 2 bad usage, 3 prerequisite missing, 4 service not healthy, 5 remote API error, 6 authentication failed.
Follow the hint in the error. The admin password is in .env.local in the workspace; no tool returns it.`

func buildMCPTools(root *cobra.Command) []mcpserver.Tool {
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
			if slices.Contains(mcpHiddenFlags, f.Name) {
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
