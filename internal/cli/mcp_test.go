package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/AlfrescoLabs/openarch-cli/internal/credstore"
	"github.com/AlfrescoLabs/openarch-cli/internal/mcpserver"
)

// mcpSession connects an in-memory client to the MCP server of app.
func mcpSession(t *testing.T, app *App, allowDestructive bool) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	if _, err := mcpserver.New(app.mcpOptions(newRootCmd(app), allowDestructive, false, defaultMaxResultBytes)).Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func TestMCPToolsMatchCommands(t *testing.T) {
	tools := buildMCPTools(newRootCmd(newApp(Streams{})), false)
	if len(tools) != len(mcpTools) {
		t.Fatalf("%d tools, want %d", len(tools), len(mcpTools))
	}
	for _, tool := range tools {
		for _, f := range tool.Flags {
			if slices.Contains(mcpHiddenFlags, f.Name) {
				t.Errorf("%s: flag --%s must not be a parameter", tool.Name, f.Name)
			}
		}
		if tool.Description == "" {
			t.Errorf("%s has no description", tool.Name)
		}
	}
}

func TestMCPListTools(t *testing.T) {
	t.Setenv(envOutput, "")
	app := newApp(Streams{Err: &strings.Builder{}})
	res, err := mcpSession(t, app, false).ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]*mcp.Tool{}
	for _, tool := range res.Tools {
		byName[tool.Name] = tool
	}
	if byName["clean"] != nil || byName["init"] == nil || len(byName) != 21 {
		t.Errorf("tools without --allow-destructive: %d, clean=%v", len(byName), byName["clean"] != nil)
	}
	if a := byName["status"].Annotations; !a.ReadOnlyHint {
		t.Errorf("status annotations = %+v", a)
	}
	if a := byName["up"].Annotations; a.ReadOnlyHint || a.DestructiveHint == nil || *a.DestructiveHint {
		t.Errorf("up annotations = %+v", a)
	}
	props := byName["init"].InputSchema.(map[string]any)["properties"].(map[string]any)
	if props["storage"] == nil || props["admin-password-stdin"] != nil {
		t.Errorf("init parameters = %v", props)
	}

	res, err = mcpSession(t, app, true).ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Tools) != len(mcpTools) {
		t.Errorf("tools with --allow-destructive: %d, want %d", len(res.Tools), len(mcpTools))
	}
}

func TestMCPCallDoctor(t *testing.T) {
	t.Setenv(envOutput, "")
	t.Setenv("OPENARCH_WORKSPACE", t.TempDir())
	for _, c := range []struct {
		portBusy bool
		isError  bool
		want     string
	}{
		{false, false, `"ok": 7`},
		{true, true, "exit code 3 (prerequisite)"},
	} {
		app := newApp(Streams{Err: &strings.Builder{}})
		app.doctorEnv = fakeDoctorEnv(c.portBusy)
		res, err := mcpSession(t, app, false).CallTool(context.Background(), &mcp.CallToolParams{Name: "doctor", Arguments: map[string]any{}})
		if err != nil {
			t.Fatal(err)
		}
		text := res.Content[0].(*mcp.TextContent).Text
		if res.IsError != c.isError || !strings.Contains(text, c.want) {
			t.Errorf("portBusy=%v: isError=%v text=%q", c.portBusy, res.IsError, text)
		}
	}
}

// TestMCPPositionalArgument checks that a positional argument reaches the
// command, and that the output is still JSON.
func TestMCPPositionalArgument(t *testing.T) {
	t.Setenv(envOutput, "")
	app := newApp(Streams{Err: &strings.Builder{}})
	res, err := mcpSession(t, app, true).CallTool(context.Background(),
		&mcp.CallToolParams{Name: "config_delete", Arguments: map[string]any{"name": "no-such-context"}})
	if err != nil {
		t.Fatal(err)
	}
	text := res.Content[0].(*mcp.TextContent).Text
	if !res.IsError || !strings.Contains(text, `"error"`) || !strings.Contains(text, "no-such-context") {
		t.Errorf("isError=%v text=%q", res.IsError, text)
	}
}

func TestMCPInitKeepsSecureDefaults(t *testing.T) {
	for _, tool := range buildMCPTools(newRootCmd(newApp(Streams{})), false) {
		if tool.Name == "init" && !strings.HasPrefix(tool.Description, mcpSecureDefaults) {
			t.Errorf("init description = %.80q", tool.Description)
		}
	}
	if !strings.Contains(mcpInstructions, mcpSecureDefaults) {
		t.Error("the server instructions do not ask to keep the secure defaults")
	}
	if !strings.Contains(mcpInstructions, mcpToolUse) {
		t.Error("the server instructions do not ask to call the tools for every fact")
	}
}

func TestMCPInitHidesInsecureOptions(t *testing.T) {
	flags := func(allowInsecure bool) map[string]bool {
		m := map[string]bool{}
		for _, tool := range buildMCPTools(newRootCmd(newApp(Streams{})), allowInsecure) {
			if tool.Name == "init" {
				for _, f := range tool.Flags {
					m[f.Name] = true
				}
			}
		}
		return m
	}
	insecure := []string{"default-users", "insecure-backends", "bind-ip", "expose", "tls", "cert", "key", "from"}
	safe, all := flags(false), flags(true)
	if !safe["storage"] || !safe["port"] {
		t.Errorf("init flags without --allow-insecure = %v, want storage and port", safe)
	}
	for _, name := range insecure {
		if safe[name] {
			t.Errorf("init takes %s without --allow-insecure", name)
		}
		if !all[name] {
			t.Errorf("init does not take %s with --allow-insecure", name)
		}
	}
}

// syncBuffer is a bytes.Buffer that a test can read while a command writes it.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// bearer adds the Authorization header to every request.
type bearer struct{ token string }

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(r)
}

func TestMCPListen(t *testing.T) {
	t.Setenv(envOutput, "")
	var out, errw syncBuffer
	app := newApp(Streams{In: strings.NewReader(""), Out: &out, Err: &errw})
	app.credStore = &credstore.Memory{}
	token, err := app.mcpToken(false)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan int, 1)
	go func() { done <- execute(ctx, app, []string{"mcp", "--listen", "0", "--output", "json"}) }()
	var started mcpListening
	deadline := time.Now().Add(5 * time.Second)
	for json.Unmarshal([]byte(out.String()), &started) != nil && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if started.Auth != "token" || !strings.HasPrefix(started.URL, "http://127.0.0.1:") {
		t.Fatalf("start result %q, stderr %q", out.String(), errw.String())
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil).Connect(context.Background(),
		&mcp.StreamableClientTransport{Endpoint: started.URL, HTTPClient: &http.Client{Transport: bearer{token}}, MaxRetries: -1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "version", Arguments: map[string]any{}})
	if err != nil || res.IsError || !strings.Contains(res.Content[0].(*mcp.TextContent).Text, `"version"`) {
		t.Errorf("version tool: %+v, %v", res, err)
	}
	_ = cs.Close()
	cancel()
	if code := <-done; code != 0 {
		t.Errorf("exit code %d, stderr %q", code, errw.String())
	}
}

func TestIsLoopback(t *testing.T) {
	for host, want := range map[string]bool{
		"localhost": true, "127.0.0.1": true, "127.0.0.2": true, "::1": true,
		"": false, "0.0.0.0": false, "::": false, "192.168.1.10": false, "openarch.example.com": false,
	} {
		if got := isLoopback(host); got != want {
			t.Errorf("isLoopback(%q) = %v", host, got)
		}
	}
}
