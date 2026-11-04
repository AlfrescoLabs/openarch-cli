package cli

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/AlfrescoLabs/openarch-cli/internal/mcpserver"
)

// mcpSession connects an in-memory client to the MCP server of app.
func mcpSession(t *testing.T, app *App, allowDestructive bool) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	if _, err := mcpserver.New(app.mcpOptions(newRootCmd(app), allowDestructive)).Connect(ctx, st, nil); err != nil {
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
	tools := buildMCPTools(newRootCmd(newApp(Streams{})))
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
	if byName["clean"] != nil || byName["init"] == nil || len(byName) != 17 {
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
	for _, tool := range buildMCPTools(newRootCmd(newApp(Streams{}))) {
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
