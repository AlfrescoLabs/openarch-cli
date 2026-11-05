package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/AlfrescoLabs/openarch-cli/internal/output"
)

// pluginPrefix is the file name prefix of plugin executables: 'openarch docs'
// runs 'openarch-docs' from PATH.
const pluginPrefix = "openarch-"

var pluginName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// pluginCall is a command line that runs a plugin.
type pluginCall struct {
	name string
	// args are the arguments after the plugin name. They go to the plugin as they are.
	args []string
	// env holds the global flags given before the plugin name, as environment variables.
	env []string
}

// parsePluginCall finds the plugin name in args: the first argument that is
// not a global flag. Global flags before the name become environment
// variables. It returns false when args do not start with a plugin name.
func parsePluginCall(args []string) (pluginCall, bool) {
	var env []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "-") {
			if !pluginName.MatchString(a) {
				return pluginCall{}, false
			}
			return pluginCall{name: a, args: args[i+1:], env: env}, true
		}
		key, value, hasValue := strings.Cut(a, "=")
		if strings.HasPrefix(a, "-o") && len(a) > 2 && !strings.HasPrefix(a, "--") {
			key, value, hasValue = "-o", strings.TrimPrefix(a[2:], "="), true
		}
		switch key {
		case "-o", "--output", "--dir", "--context":
			if !hasValue {
				if i+1 >= len(args) {
					return pluginCall{}, false
				}
				i++
				value = args[i]
			}
			env = append(env, globalFlagEnv(key, value))
		case "--non-interactive", "-v", "--verbose", "--no-color":
			if hasValue {
				return pluginCall{}, false
			}
			env = append(env, globalFlagEnv(key, "1"))
		default:
			return pluginCall{}, false
		}
	}
	return pluginCall{}, false
}

// globalFlagEnv maps a global flag to the variable a plugin reads. The
// variables for --output, --dir and --context are the ones openarch reads too,
// so a plugin that runs openarch gets the same workspace and context.
func globalFlagEnv(flag, value string) string {
	switch flag {
	case "-o", "--output":
		return envOutput + "=" + value
	case "--dir":
		if abs, err := filepath.Abs(value); err == nil {
			value = abs
		}
		return "OPENARCH_WORKSPACE=" + value
	case "--context":
		return "OPENARCH_CONTEXT=" + value
	case "--non-interactive":
		return "OPENARCH_NON_INTERACTIVE=1"
	case "-v", "--verbose":
		return "OPENARCH_VERBOSE=1"
	default: // --no-color
		return "NO_COLOR=1"
	}
}

// isBuiltin reports whether name is a command of root. Built-in commands always win over plugins.
func isBuiltin(root *cobra.Command, name string) bool {
	if name == "help" {
		return true
	}
	for _, c := range root.Commands() {
		if c.Name() == name || c.HasAlias(name) {
			return true
		}
	}
	return false
}

// runPlugin runs the plugin named in args, if there is one. It returns false
// when args are for a built-in command or no plugin has the name.
func runPlugin(ctx context.Context, app *App, root *cobra.Command, args []string) (int, bool) {
	call, ok := parsePluginCall(args)
	if !ok || isBuiltin(root, call.name) {
		return 0, false
	}
	path, err := exec.LookPath(pluginPrefix + call.name)
	if err != nil {
		return 0, false
	}
	cmd := exec.CommandContext(ctx, path, call.args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = app.Streams.In, app.Streams.Out, app.Streams.Err
	cmd.Env = append(os.Environ(), call.env...)
	if self, err := os.Executable(); err == nil {
		cmd.Env = append(cmd.Env, "OPENARCH_BIN="+self)
	}
	err = cmd.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), true
	}
	if err != nil {
		p := output.NewPrinter(formatFromArgs(args), app.Streams.Out, app.Streams.Err)
		e := output.NewError(output.ExitError, fmt.Sprintf("cannot run plugin %s: %v", path, err), "")
		e.Err = err
		p.Error(e)
		return int(output.ExitError), true
	}
	return int(output.ExitOK), true
}

// Plugin is one plugin found on PATH.
type Plugin struct {
	Name string `json:"name"`
	Path string `json:"path"`
	// Shadowed is true when a built-in command has the same name, so the plugin never runs.
	Shadowed bool `json:"shadowed"`
}

// PluginListResult is the outcome of 'plugin list'.
type PluginListResult struct {
	Plugins []Plugin `json:"plugins"`
}

func (r PluginListResult) WriteText(w io.Writer) error {
	if len(r.Plugins) == 0 {
		_, err := fmt.Fprintln(w, "No plugins found on PATH.")
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "NAME\tPATH\tNOTE"); err != nil {
		return err
	}
	for _, p := range r.Plugins {
		note := ""
		if p.Shadowed {
			note = "ignored: built-in command with the same name"
		}
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\n", p.Name, p.Path, note); err != nil {
			return err
		}
	}
	return tw.Flush()
}

// findPlugins lists the plugin executables on PATH. When two folders have the
// same plugin, the first one wins, as in exec.LookPath.
func findPlugins(root *cobra.Command) []Plugin {
	plugins := []Plugin{}
	seen := map[string]bool{}
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			name, ok := pluginFileName(e.Name())
			if !ok || seen[name] {
				continue
			}
			path := filepath.Join(dir, e.Name())
			if _, err := exec.LookPath(path); err != nil {
				continue
			}
			seen[name] = true
			plugins = append(plugins, Plugin{Name: name, Path: path, Shadowed: isBuiltin(root, name)})
		}
	}
	slices.SortFunc(plugins, func(a, b Plugin) int { return strings.Compare(a.Name, b.Name) })
	return plugins
}

// pluginFileName returns the plugin name of an executable file name.
func pluginFileName(file string) (string, bool) {
	if runtime.GOOS == "windows" {
		ext := filepath.Ext(file)
		if !slices.Contains(strings.Split(strings.ToLower(os.Getenv("PATHEXT")), ";"), strings.ToLower(ext)) {
			return "", false
		}
		file = strings.TrimSuffix(file, ext)
	}
	name, ok := strings.CutPrefix(file, pluginPrefix)
	if !ok || !pluginName.MatchString(name) {
		return "", false
	}
	return name, true
}

func newPluginCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plugin",
		Short: "List the plugins that extend openarch",
		Long: `List the plugins that extend openarch.

A plugin is an executable named openarch-<name> on PATH. 'openarch <name> ...'
runs it with the arguments after the name. Built-in commands always win over
plugins with the same name.

Global flags given before the name reach the plugin as environment variables:
--output as OPENARCH_OUTPUT, --dir as OPENARCH_WORKSPACE, --context as
OPENARCH_CONTEXT, --non-interactive as OPENARCH_NON_INTERACTIVE=1, --verbose as
OPENARCH_VERBOSE=1 and --no-color as NO_COLOR=1. OPENARCH_BIN holds the path of
openarch, so a plugin can run the same binary. openarch reads the first three
variables too, so commands run by the plugin use the same workspace and context.
Passwords are never passed: a plugin that needs OpenArch runs openarch.`,
		Example: `  openarch plugin list
  openarch docs --help`,
		Args: noArgs,
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List the openarch-<name> executables on PATH",
		Example: `  openarch plugin list
  openarch plugin list --output json`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return app.Printer.Print(PluginListResult{Plugins: findPlugins(cmd.Root())})
		},
	})
	return cmd
}
