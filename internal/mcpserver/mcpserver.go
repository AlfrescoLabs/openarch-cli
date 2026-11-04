// Package mcpserver exposes CLI commands as MCP tools over stdio.
//
// A tool runs one CLI command with --output json and --non-interactive, and
// returns its JSON result. The input schema of a tool comes from the flags of
// its command, so the tools and the commands cannot drift apart. Command
// progress (stderr) becomes MCP progress notifications. Stdout is the MCP
// channel, so nothing else may write to it.
package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/AlfrescoLabs/openarch-cli/internal/output"
)

// Kind tells what a tool may change.
type Kind int

const (
	ReadOnly    Kind = iota // reads only
	Write                   // changes files or services, keeps the data
	Destructive             // can delete data or stop services
)

// Flag is a command flag that the tool accepts as a parameter.
type Flag struct {
	Name    string
	Type    string // pflag type: bool, string, int, int64, duration, stringSlice, intSlice
	Default string
	Usage   string
	Values  []string // allowed values, when fixed
}

// Arg is a positional argument of the command.
type Arg struct {
	Name        string
	Description string
	Required    bool
	Variadic    bool // takes a list
}

// Tool is one CLI command exposed as an MCP tool.
type Tool struct {
	Name        string
	Title       string
	Description string
	Command     []string // for example ["documents", "get"]
	Kind        Kind
	Flags       []Flag
	Args        []Arg
	// Defaults are flag values used when the caller does not give the parameter.
	Defaults map[string]string
	// Extra arguments always passed, for example --yes.
	Extra []string
}

// Runner runs the CLI with args. Progress receives stderr. It returns the
// exit code and stdout.
type Runner func(ctx context.Context, args []string, progress io.Writer) (code int, stdout []byte)

// Options configure the server.
type Options struct {
	Name         string
	Version      string
	Instructions string
	Tools        []Tool
	Run          Runner
	// Prefix comes before the command of every call, for example --dir <folder>.
	Prefix []string
	// AllowDestructive registers the destructive tools. Without it they do not exist.
	AllowDestructive bool
	// Log receives the stderr of every command.
	Log io.Writer
}

// New returns a server with the tools of opts.
func New(opts Options) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: opts.Name, Version: opts.Version},
		&mcp.ServerOptions{Instructions: opts.Instructions})
	// Commands that change the workspace run one at a time.
	var mu sync.Mutex
	for _, t := range opts.Tools {
		if t.Kind == Destructive && !opts.AllowDestructive {
			continue
		}
		s.AddTool(&mcp.Tool{
			Name:        t.Name,
			Title:       t.Title,
			Description: t.Description,
			InputSchema: InputSchema(t),
			Annotations: annotations(t),
		}, handler(opts, t, &mu))
	}
	return s
}

// Serve runs the server on stdin and stdout until the client disconnects.
func Serve(ctx context.Context, opts Options) error {
	return New(opts).Run(ctx, &mcp.StdioTransport{})
}

func annotations(t Tool) *mcp.ToolAnnotations {
	destructive := t.Kind == Destructive
	return &mcp.ToolAnnotations{
		Title:           t.Title,
		ReadOnlyHint:    t.Kind == ReadOnly,
		IdempotentHint:  t.Kind == ReadOnly,
		DestructiveHint: &destructive,
	}
}

// InputSchema returns the JSON schema of the tool parameters.
func InputSchema(t Tool) map[string]any {
	props := map[string]any{}
	required := []string{}
	for _, a := range t.Args {
		var p map[string]any
		if a.Variadic {
			p = map[string]any{"type": "array", "items": map[string]any{"type": "string"}}
		} else {
			p = map[string]any{"type": "string"}
		}
		p["description"] = a.Description
		props[a.Name] = p
		if a.Required {
			required = append(required, a.Name)
		}
	}
	for _, f := range t.Flags {
		props[f.Name] = flagSchema(f, t.Defaults[f.Name])
	}
	return map[string]any{
		"type":                 "object",
		"properties":           props,
		"required":             required,
		"additionalProperties": false,
	}
}

func flagSchema(f Flag, toolDefault string) map[string]any {
	p := map[string]any{"description": f.Usage}
	def := f.Default
	if toolDefault != "" {
		def = toolDefault
	}
	switch f.Type {
	case "bool":
		p["type"] = "boolean"
		if b, err := strconv.ParseBool(def); err == nil {
			p["default"] = b
		}
	case "int", "int64":
		p["type"] = "integer"
		if n, err := strconv.ParseInt(def, 10, 64); err == nil {
			p["default"] = n
		}
	case "stringSlice", "intSlice":
		item := map[string]any{"type": "string"}
		if f.Type == "intSlice" {
			item["type"] = "integer"
		}
		if len(f.Values) > 0 {
			item["enum"] = f.Values
		}
		p["type"] = "array"
		p["items"] = item
	default:
		p["type"] = "string"
		if f.Type == "duration" {
			p["description"] = f.Usage + " (a duration, for example 90s or 10m)"
		}
		if def != "" {
			p["default"] = def
		}
		if len(f.Values) > 0 {
			p["enum"] = f.Values
		}
	}
	return p
}

// Args builds the CLI arguments of one call: the command, its flags, then
// "--" and the positional arguments, so that a value can never become a flag.
func Args(t Tool, raw json.RawMessage) ([]string, error) {
	params := map[string]json.RawMessage{}
	if len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, &params); err != nil {
			return nil, fmt.Errorf("the arguments must be a JSON object: %w", err)
		}
	}
	args := append([]string{}, t.Command...)
	for name := range params {
		if !slices.ContainsFunc(t.Flags, func(f Flag) bool { return f.Name == name }) &&
			!slices.ContainsFunc(t.Args, func(a Arg) bool { return a.Name == name }) {
			return nil, fmt.Errorf("unknown parameter %q", name)
		}
	}
	for _, f := range t.Flags {
		v, ok := params[f.Name]
		if !ok {
			if d := t.Defaults[f.Name]; d != "" {
				args = append(args, "--"+f.Name+"="+d)
			}
			continue
		}
		values, err := flagValues(f, v)
		if err != nil {
			return nil, err
		}
		for _, s := range values {
			args = append(args, "--"+f.Name+"="+s)
		}
	}
	args = append(args, t.Extra...)
	var positional []string
	for _, a := range t.Args {
		v, ok := params[a.Name]
		if !ok {
			if a.Required {
				return nil, fmt.Errorf("missing parameter %q", a.Name)
			}
			continue
		}
		if a.Variadic {
			var list []string
			if err := json.Unmarshal(v, &list); err != nil {
				return nil, fmt.Errorf("parameter %q must be a list of strings", a.Name)
			}
			positional = append(positional, list...)
			continue
		}
		var s string
		if err := json.Unmarshal(v, &s); err != nil {
			return nil, fmt.Errorf("parameter %q must be a string", a.Name)
		}
		positional = append(positional, s)
	}
	if len(positional) > 0 {
		args = append(append(args, "--"), positional...)
	}
	return args, nil
}

// flagValues converts one JSON parameter to flag values. A list gives one
// value per item, because repeated slice flags add up.
func flagValues(f Flag, v json.RawMessage) ([]string, error) {
	switch f.Type {
	case "bool":
		var b bool
		if err := json.Unmarshal(v, &b); err != nil {
			return nil, fmt.Errorf("parameter %q must be a boolean", f.Name)
		}
		return []string{strconv.FormatBool(b)}, nil
	case "int", "int64":
		var n int64
		if err := json.Unmarshal(v, &n); err != nil {
			return nil, fmt.Errorf("parameter %q must be an integer", f.Name)
		}
		return []string{strconv.FormatInt(n, 10)}, nil
	case "stringSlice":
		var list []string
		if err := json.Unmarshal(v, &list); err != nil {
			return nil, fmt.Errorf("parameter %q must be a list of strings", f.Name)
		}
		return list, nil
	case "intSlice":
		var list []int64
		if err := json.Unmarshal(v, &list); err != nil {
			return nil, fmt.Errorf("parameter %q must be a list of integers", f.Name)
		}
		var out []string
		for _, n := range list {
			out = append(out, strconv.FormatInt(n, 10))
		}
		return out, nil
	}
	var s string
	if err := json.Unmarshal(v, &s); err != nil {
		return nil, fmt.Errorf("parameter %q must be a string", f.Name)
	}
	return []string{s}, nil
}

func handler(opts Options, t Tool, mu *sync.Mutex) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, err := Args(t, req.Params.Arguments)
		if err != nil {
			return errorResult(err.Error()), nil
		}
		if t.Kind != ReadOnly {
			mu.Lock()
			defer mu.Unlock()
		}
		// The fixed flags come first: after "--" in args, they would be positional arguments.
		full := append(append(append([]string{}, opts.Prefix...), "--output", "json", "--non-interactive"), args...)
		prog := &progressWriter{ctx: ctx, req: req, log: opts.Log}
		code, stdout := opts.Run(ctx, full, prog)
		prog.flush()
		return result(code, stdout, prog.tail()), nil
	}
}

func result(code int, stdout []byte, stderrTail []string) *mcp.CallToolResult {
	text := strings.TrimSpace(string(stdout))
	if code == 0 {
		res := &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}
		var v any
		if json.Unmarshal(stdout, &v) == nil {
			res.StructuredContent = v
		}
		return res
	}
	msg := fmt.Sprintf("exit code %d (%s)", code, output.ExitCode(code).Name())
	if text != "" {
		msg += "\n" + text
	}
	if len(stderrTail) > 0 {
		msg += "\n\nLast lines of stderr:\n" + strings.Join(stderrTail, "\n")
	}
	return errorResult(msg)
}

func errorResult(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: msg}}}
}

// tailLines is how many stderr lines an error result keeps.
const tailLines = 20

// progressWriter turns stderr lines into progress notifications, copies
// them to the log, and keeps the last lines for error results.
type progressWriter struct {
	ctx   context.Context
	req   *mcp.CallToolRequest
	log   io.Writer
	buf   []byte
	lines []string
	count float64
}

func (w *progressWriter) Write(p []byte) (int, error) {
	if w.log != nil {
		_, _ = w.log.Write(p)
	}
	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		w.line(string(w.buf[:i]))
		w.buf = w.buf[i+1:]
	}
	return len(p), nil
}

func (w *progressWriter) flush() {
	if len(w.buf) > 0 {
		w.line(string(w.buf))
		w.buf = nil
	}
}

func (w *progressWriter) line(s string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return
	}
	w.lines = append(w.lines, s)
	if len(w.lines) > tailLines {
		w.lines = w.lines[1:]
	}
	token := w.req.Params.GetProgressToken()
	if token == nil || w.req.Session == nil {
		return
	}
	w.count++
	_ = w.req.Session.NotifyProgress(w.ctx, &mcp.ProgressNotificationParams{ProgressToken: token, Progress: w.count, Message: s})
}

func (w *progressWriter) tail() []string { return w.lines }
