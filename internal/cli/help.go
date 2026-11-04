package cli

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/AlfrescoLabs/openarch-cli/internal/output"
	"github.com/AlfrescoLabs/openarch-cli/internal/version"
)

// helpDoc is the JSON help: the commands, their flags and the exit codes, so
// that a program or an AI agent can find every command without the docs.
type helpDoc struct {
	Name        string                `json:"name"`
	Version     string                `json:"version"`
	Description string                `json:"description"`
	GlobalFlags []helpFlag            `json:"globalFlags"`
	ExitCodes   []output.ExitCodeInfo `json:"exitCodes"`
	Commands    []helpCommand         `json:"commands"`
}

type helpCommand struct {
	Path        string     `json:"path"`
	Usage       string     `json:"usage"`
	Short       string     `json:"short"`
	Long        string     `json:"long,omitempty"`
	Examples    []string   `json:"examples,omitempty"`
	Flags       []helpFlag `json:"flags"`
	Subcommands []string   `json:"subcommands,omitempty"`
}

type helpFlag struct {
	Name      string   `json:"name"`
	Shorthand string   `json:"shorthand,omitempty"`
	Type      string   `json:"type"`
	Default   string   `json:"default"`
	Usage     string   `json:"usage"`
	Values    []string `json:"values,omitempty"`
}

// setJSONHelp makes 'openarch help' and --help print the JSON help when the
// output format is json. The text help does not change.
func setJSONHelp(app *App, root *cobra.Command) {
	textHelp := root.HelpFunc()
	root.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		if output.Format(app.Opts.Output) != output.JSON {
			textHelp(cmd, args)
			return
		}
		_ = output.NewPrinter(output.JSON, cmd.OutOrStdout(), cmd.ErrOrStderr()).Print(buildHelp(cmd))
	})
}

// buildHelp describes cmd and every command below it.
func buildHelp(cmd *cobra.Command) helpDoc {
	root := cmd.Root()
	doc := helpDoc{
		Name:        root.Name(),
		Version:     version.Get().Version,
		Description: root.Short,
		GlobalFlags: helpFlags(root.PersistentFlags()),
		ExitCodes:   output.ExitCodes(),
	}
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		hc := helpCommand{
			Path:     c.CommandPath(),
			Usage:    c.UseLine(),
			Short:    c.Short,
			Long:     c.Long,
			Examples: examples(c.Example),
			Flags:    helpFlags(c.NonInheritedFlags()),
		}
		if c == root {
			// The root flags are the global flags.
			hc.Flags = []helpFlag{}
		}
		var subs []*cobra.Command
		for _, s := range c.Commands() {
			if s.IsAvailableCommand() {
				hc.Subcommands = append(hc.Subcommands, s.CommandPath())
				subs = append(subs, s)
			}
		}
		doc.Commands = append(doc.Commands, hc)
		for _, s := range subs {
			walk(s)
		}
	}
	walk(cmd)
	return doc
}

func helpFlags(fs *pflag.FlagSet) []helpFlag {
	flags := []helpFlag{}
	fs.VisitAll(func(f *pflag.Flag) {
		if f.Hidden || f.Name == "help" {
			return
		}
		flags = append(flags, helpFlag{
			Name:      f.Name,
			Shorthand: f.Shorthand,
			Type:      f.Value.Type(),
			Default:   f.DefValue,
			Usage:     f.Usage,
			Values:    f.Annotations[valuesAnnotation],
		})
	})
	return flags
}

// examples returns one entry per line of a cobra Example text.
func examples(text string) []string {
	var list []string
	for line := range strings.SplitSeq(text, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			list = append(list, line)
		}
	}
	return list
}
