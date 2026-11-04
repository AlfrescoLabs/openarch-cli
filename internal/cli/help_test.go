package cli

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func jsonHelp(t *testing.T, args ...string) helpDoc {
	t.Helper()
	code, stdout, stderr := run(t, args...)
	var doc helpDoc
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("%v: stdout is not JSON (code %d, stderr %q): %v", args, code, stderr, err)
	}
	if code != 0 {
		t.Fatalf("%v: exit code %d", args, code)
	}
	return doc
}

// TestJSONHelpCoversTree checks every command and flag of the cobra tree
// against 'openarch help --output json'.
func TestJSONHelpCoversTree(t *testing.T) {
	t.Setenv(envOutput, "")
	doc := jsonHelp(t, "help", "--output", "json")
	byPath := map[string]helpCommand{}
	for _, c := range doc.Commands {
		byPath[c.Path] = c
	}
	names := func(flags []helpFlag) []string {
		var list []string
		for _, f := range flags {
			list = append(list, f.Name)
		}
		return list
	}
	root := newRootCmd(newApp(Streams{}))
	root.PersistentFlags().VisitAll(func(f *pflag.Flag) {
		if !slices.Contains(names(doc.GlobalFlags), f.Name) {
			t.Errorf("global flag --%s is missing", f.Name)
		}
	})
	count := 0
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		count++
		hc, ok := byPath[c.CommandPath()]
		if !ok {
			t.Errorf("command %q is missing", c.CommandPath())
		}
		if c != root {
			c.NonInheritedFlags().VisitAll(func(f *pflag.Flag) {
				if !f.Hidden && !slices.Contains(names(hc.Flags), f.Name) {
					t.Errorf("%s: flag --%s is missing", c.CommandPath(), f.Name)
				}
			})
		}
		for _, s := range c.Commands() {
			if s.IsAvailableCommand() {
				walk(s)
			}
		}
	}
	walk(root)
	if len(doc.Commands) != count {
		t.Errorf("JSON help has %d commands, the tree has %d", len(doc.Commands), count)
	}
	if len(doc.ExitCodes) != 7 || doc.ExitCodes[6].Name != "auth" {
		t.Errorf("exit codes = %+v", doc.ExitCodes)
	}
}

func TestJSONHelpForOneCommand(t *testing.T) {
	t.Setenv(envOutput, "")
	for _, args := range [][]string{
		{"help", "init", "--output", "json"},
		{"init", "--help", "-o", "json"},
	} {
		doc := jsonHelp(t, args...)
		if len(doc.Commands) != 1 || doc.Commands[0].Path != "openarch init" {
			t.Fatalf("%v: commands = %+v", args, doc.Commands)
		}
		var storage *helpFlag
		for i, f := range doc.Commands[0].Flags {
			if f.Name == "storage" {
				storage = &doc.Commands[0].Flags[i]
			}
		}
		if storage == nil || storage.Default != "volume" || !slices.Equal(storage.Values, []string{"none", "volume", "bind"}) {
			t.Errorf("%v: storage flag = %+v", args, storage)
		}
	}
}

func TestTextHelpUnchanged(t *testing.T) {
	t.Setenv(envOutput, "")
	_, stdout, _ := run(t, "help", "init")
	if !strings.HasPrefix(stdout, "Create an OpenArch workspace") {
		t.Errorf("text help = %.60q", stdout)
	}
}

func TestEveryCommandHasExamples(t *testing.T) {
	t.Setenv(envOutput, "")
	for _, c := range jsonHelp(t, "help", "-o", "json").Commands {
		if len(c.Examples) == 0 {
			t.Errorf("%s has no examples", c.Path)
		}
	}
}
