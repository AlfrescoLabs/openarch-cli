package cli

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra/doc"

	"github.com/AlfrescoLabs/openarch-cli/internal/doctor"
)

var updateDocs = flag.Bool("update", false, "rewrite docs/commands and docs/llms.txt")

const docsDir = "../../docs/commands"

// TestCommandDocs checks that docs/commands matches the cobra commands.
// go test ./internal/cli -run TestCommandDocs -update rewrites it.
func TestCommandDocs(t *testing.T) {
	t.Setenv(envOutput, "")
	tmp := t.TempDir()
	app := newApp(Streams{})
	// The doctor help lists the checks for the host OS. Linux has the most checks.
	app.doctorEnv = func() doctor.Env {
		env := doctor.SystemEnv()
		env.GOOS = "linux"
		return env
	}
	root := newRootCmd(app)
	root.DisableAutoGenTag = true
	if err := doc.GenMarkdownTree(root, tmp); err != nil {
		t.Fatal(err)
	}
	if *updateDocs {
		old, _ := filepath.Glob(filepath.Join(docsDir, "*.md"))
		for _, f := range old {
			if err := os.Remove(f); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.MkdirAll(docsDir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	got, _ := filepath.Glob(filepath.Join(tmp, "*.md"))
	for _, f := range got {
		want, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		dst := filepath.Join(docsDir, filepath.Base(f))
		if *updateDocs {
			if err := os.WriteFile(dst, want, 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		have, err := os.ReadFile(dst)
		if err != nil || !bytes.Equal(have, want) {
			t.Errorf("%s is out of date; run: go test ./internal/cli -run TestCommandDocs -update", dst)
		}
	}
	if !*updateDocs {
		have, _ := filepath.Glob(filepath.Join(docsDir, "*.md"))
		if len(have) != len(got) {
			t.Errorf("docs/commands has %d files, want %d; run: go test ./internal/cli -run TestCommandDocs -update", len(have), len(got))
		}
	}
}

const llmsFile = "../../docs/llms.txt"

// TestLLMsTxt checks that docs/llms.txt matches the cobra commands. It is
// built from the same data as the JSON help.
// go test ./internal/cli -run TestLLMsTxt -update rewrites it.
func TestLLMsTxt(t *testing.T) {
	t.Setenv(envOutput, "")
	want := llmsTxt(buildHelp(newRootCmd(newApp(Streams{}))))
	if *updateDocs {
		if err := os.WriteFile(llmsFile, want, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	have, err := os.ReadFile(llmsFile)
	if err != nil || !bytes.Equal(have, want) {
		t.Errorf("%s is out of date; run: go test ./internal/cli -run TestLLMsTxt -update", llmsFile)
	}
}

// llmsTxt writes the llms.txt format (https://llmstxt.org): a title, a
// summary, then lists of links. Links are relative to the docs folder.
func llmsTxt(doc helpDoc) []byte {
	var b bytes.Buffer
	b.WriteString(`# OpenArch CLI

> openarch is a command line tool to install and run OpenArch (https://github.com/hyland/OpenArch) with Docker Compose. Every command supports --output json, and the exit codes are stable.

How to call it from a program or an AI agent:

- Add --output json (or set OPENARCH_OUTPUT=json). stdout then holds exactly one JSON document, also for errors: {"error": {"code": "...", "message": "...", "hint": "..."}}. Logs and warnings go to stderr.
- Add --non-interactive, so that no command waits for input.
- Pass passwords on stdin (--password-stdin, --admin-password-stdin), never as arguments.
- Run 'openarch help --output json' for every command, flag, default value, allowed value and exit code. 'openarch <command> --help --output json' describes one command.
- Start with 'openarch doctor', then 'openarch init --dir <folder> --non-interactive', then 'openarch up --dir <folder>'.
- For MCP clients, 'openarch mcp --dir <folder>' runs an MCP server on stdio with the same commands as tools. Add --allow-destructive for down, clean, restore and the other tools that delete data.
- For coding agents that read Agent Skills, 'openarch skill install' writes .claude/skills/openarch/SKILL.md with the install flow and the fixes for common failures.
- Plugins: 'openarch <name>' runs an executable named openarch-<name> from PATH. 'openarch plugin list' shows them.

## Exit codes

`)
	for _, e := range doc.ExitCodes {
		fmt.Fprintf(&b, "- %d %s: %s\n", e.Code, e.Name, e.Meaning)
	}
	b.WriteString("\n## Commands\n\n")
	for _, c := range doc.Commands[1:] {
		fmt.Fprintf(&b, "- [%s](commands/%s.md): %s\n", c.Path, strings.ReplaceAll(c.Path, " ", "_"), c.Short)
	}
	b.WriteString(`
## Docs

- [README](../README.md): install, quick start, workspace options, security
- [Walkthrough](walkthrough.md): a tour of every feature in a temporary folder
- [Design](design.md): how the CLI and the workspace files are built
`)
	return b.Bytes()
}
