package cli

import (
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

func TestParsePluginCall(t *testing.T) {
	abs, _ := filepath.Abs("ws")
	tests := []struct {
		args []string
		ok   bool
		name string
		rest []string
		env  []string
	}{
		{args: []string{"docs", "ingest", "--output", "text"}, ok: true, name: "docs", rest: []string{"ingest", "--output", "text"}},
		{args: []string{"-o", "json", "docs"}, ok: true, name: "docs", rest: []string{}, env: []string{"OPENARCH_OUTPUT=json"}},
		{args: []string{"-ojson", "--dir=ws", "docs"}, ok: true, name: "docs", rest: []string{}, env: []string{"OPENARCH_OUTPUT=json", "OPENARCH_WORKSPACE=" + abs}},
		{args: []string{"--context", "dev", "-v", "docs", "x"}, ok: true, name: "docs", rest: []string{"x"}, env: []string{"OPENARCH_CONTEXT=dev", "OPENARCH_VERBOSE=1"}},
		{args: []string{}, ok: false},
		{args: []string{"--output"}, ok: false},
		{args: []string{"--unknown", "docs"}, ok: false},
		{args: []string{"--verbose=false", "docs"}, ok: false},
		{args: []string{"--", "docs"}, ok: false},
		{args: []string{"../evil"}, ok: false},
		{args: []string{"Docs"}, ok: false},
	}
	for _, tt := range tests {
		got, ok := parsePluginCall(tt.args)
		if ok != tt.ok {
			t.Errorf("%v: ok = %v, want %v", tt.args, ok, tt.ok)
			continue
		}
		if !ok {
			continue
		}
		if got.name != tt.name || !slices.Equal(got.args, tt.rest) || !slices.Equal(got.env, tt.env) {
			t.Errorf("%v: got %+v, want name %q args %q env %q", tt.args, got, tt.name, tt.rest, tt.env)
		}
	}
}

func TestIsBuiltin(t *testing.T) {
	root := newRootCmd(newApp(Streams{}))
	for _, name := range []string{"version", "help", "plugin", "completion"} {
		if !isBuiltin(root, name) {
			t.Errorf("%s is a built-in command", name)
		}
	}
	if isBuiltin(root, "docs") {
		t.Error("docs is not a built-in command")
	}
}

func TestPluginFileName(t *testing.T) {
	file := "openarch-docs"
	if runtime.GOOS == "windows" {
		t.Setenv("PATHEXT", ".COM;.EXE;.BAT")
		file = "openarch-docs.exe"
		if _, ok := pluginFileName("openarch-docs.txt"); ok {
			t.Error("a .txt file is not a plugin")
		}
	}
	if name, ok := pluginFileName(file); !ok || name != "docs" {
		t.Errorf("pluginFileName(%q) = %q, %v", file, name, ok)
	}
	for _, f := range []string{"openarch", "openarch-", "other-docs", "openarch-Docs"} {
		if _, ok := pluginFileName(f); ok {
			t.Errorf("%q is not a plugin", f)
		}
	}
}
