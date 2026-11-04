package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var getTool = Tool{
	Name:    "documents_get",
	Command: []string{"documents", "get"},
	Kind:    ReadOnly,
	Flags: []Flag{
		{Name: "sync", Type: "bool", Default: "false", Usage: "wait for indexing"},
		{Name: "limit", Type: "int64", Default: "100", Usage: "max results"},
		{Name: "sort", Type: "stringSlice", Default: "[]", Usage: "sort keys"},
		{Name: "storage", Type: "string", Default: "volume", Usage: "storage", Values: []string{"none", "volume", "bind"}},
		{Name: "tail", Type: "string", Usage: "lines"},
	},
	Args:     []Arg{{Name: "id_or_path", Description: "id or path", Required: true}},
	Defaults: map[string]string{"tail": "200"},
	Extra:    []string{"--yes"},
}

func TestArgs(t *testing.T) {
	got, err := Args(getTool, json.RawMessage(`{"id_or_path": "--output=text", "sync": true, "sort": ["a", "b,c"], "limit": 5}`))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"documents", "get", "--sync=true", "--limit=5", "--sort=a", "--sort=b,c", "--tail=200", "--yes", "--", "--output=text"}
	if !slices.Equal(got, want) {
		t.Errorf("args = %q\nwant   %q", got, want)
	}
}

func TestArgsErrors(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{`{}`, `missing parameter "id_or_path"`},
		{`{"id_or_path": "/", "bogus": 1}`, `unknown parameter "bogus"`},
		{`{"id_or_path": "/", "sync": "yes"}`, `parameter "sync" must be a boolean`},
		{`{"id_or_path": "/", "limit": 1.5}`, `parameter "limit" must be an integer`},
		{`{"id_or_path": 3}`, `parameter "id_or_path" must be a string`},
		{`[1]`, `the arguments must be a JSON object`},
	} {
		_, err := Args(getTool, json.RawMessage(c.in))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want %q", c.in, err, c.want)
		}
	}
}

func TestInputSchema(t *testing.T) {
	s := InputSchema(getTool)
	if s["additionalProperties"] != false || !slices.Equal(s["required"].([]string), []string{"id_or_path"}) {
		t.Errorf("schema = %v", s)
	}
	props := s["properties"].(map[string]any)
	if p := props["sync"].(map[string]any); p["type"] != "boolean" || p["default"] != false {
		t.Errorf("sync = %v", p)
	}
	if p := props["limit"].(map[string]any); p["type"] != "integer" || p["default"] != int64(100) {
		t.Errorf("limit = %v", p)
	}
	if p := props["storage"].(map[string]any); p["default"] != "volume" || !slices.Equal(p["enum"].([]string), []string{"none", "volume", "bind"}) {
		t.Errorf("storage = %v", p)
	}
	if p := props["tail"].(map[string]any); p["default"] != "200" {
		t.Errorf("tail = %v", p)
	}
	if p := props["sort"].(map[string]any); p["type"] != "array" {
		t.Errorf("sort = %v", p)
	}
}

// connect starts a server with opts and returns a connected client session.
func connect(t *testing.T, opts Options, copts *mcp.ClientOptions) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	if _, err := New(opts).Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, copts).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func toolNames(t *testing.T, cs *mcp.ClientSession) []string {
	t.Helper()
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
	}
	slices.Sort(names)
	return names
}

func TestDestructiveToolsNeedPermission(t *testing.T) {
	tools := []Tool{{Name: "status", Kind: ReadOnly}, {Name: "up", Kind: Write}, {Name: "clean", Kind: Destructive}}
	run := func(context.Context, []string, io.Writer) (int, []byte) { return 0, []byte("{}") }
	if got := toolNames(t, connect(t, Options{Name: "x", Version: "1", Tools: tools, Run: run}, nil)); !slices.Equal(got, []string{"status", "up"}) {
		t.Errorf("tools = %v", got)
	}
	if got := toolNames(t, connect(t, Options{Name: "x", Version: "1", Tools: tools, Run: run, AllowDestructive: true}, nil)); !slices.Equal(got, []string{"clean", "status", "up"}) {
		t.Errorf("tools with --allow-destructive = %v", got)
	}
}

func TestCall(t *testing.T) {
	var gotArgs []string
	run := func(_ context.Context, args []string, progress io.Writer) (int, []byte) {
		gotArgs = args
		_, _ = io.WriteString(progress, "pulling images\nstarting")
		return 0, []byte(`{"state": "healthy"}`)
	}
	var mu sync.Mutex
	var messages []string
	cs := connect(t, Options{Name: "x", Version: "1", Tools: []Tool{{Name: "up", Command: []string{"up"}, Kind: Write}}, Run: run, Prefix: []string{"--dir", "ws"}},
		&mcp.ClientOptions{ProgressNotificationHandler: func(_ context.Context, req *mcp.ProgressNotificationClientRequest) {
			mu.Lock()
			defer mu.Unlock()
			messages = append(messages, req.Params.Message)
		}})
	params := &mcp.CallToolParams{Name: "up", Arguments: map[string]any{}}
	params.SetProgressToken("t1")
	res, err := cs.CallTool(context.Background(), params)
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError || fmt.Sprint(res.StructuredContent) != "map[state:healthy]" {
		t.Errorf("result = %+v", res)
	}
	if want := []string{"--dir", "ws", "--output", "json", "--non-interactive", "up"}; !slices.Equal(gotArgs, want) {
		t.Errorf("args = %q, want %q", gotArgs, want)
	}
	mu.Lock()
	defer mu.Unlock()
	if !slices.Equal(messages, []string{"pulling images", "starting"}) {
		t.Errorf("progress = %q", messages)
	}
}

func TestCallError(t *testing.T) {
	run := func(_ context.Context, _ []string, progress io.Writer) (int, []byte) {
		_, _ = io.WriteString(progress, "Warning: opensearch is unhealthy\n")
		return 4, []byte(`{"state": "unhealthy"}`)
	}
	cs := connect(t, Options{Name: "x", Version: "1", Tools: []Tool{{Name: "up", Command: []string{"up"}, Kind: Write}}, Run: run}, nil)
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "up", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	text := res.Content[0].(*mcp.TextContent).Text
	if !res.IsError || !strings.HasPrefix(text, "exit code 4 (unhealthy)\n") ||
		!strings.Contains(text, `"state": "unhealthy"`) || !strings.Contains(text, "Warning: opensearch is unhealthy") {
		t.Errorf("result = %+v, text %q", res, text)
	}
	res, err = cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "up", Arguments: map[string]any{"bogus": true}})
	if err != nil {
		t.Fatal(err)
	}
	if text := res.Content[0].(*mcp.TextContent).Text; !res.IsError || text != `unknown parameter "bogus"` {
		t.Errorf("bad input: %+v, text %q", res, text)
	}
}
