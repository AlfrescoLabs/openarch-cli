package cli

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/AlfrescoLabs/openarch-cli/internal/contexts"
	"github.com/AlfrescoLabs/openarch-cli/internal/doctor"
	"github.com/AlfrescoLabs/openarch-cli/internal/output"
	"github.com/AlfrescoLabs/openarch-cli/internal/stack"
	"github.com/AlfrescoLabs/openarch-cli/internal/workspace"
)

// addCompletion adds the completion command and the completions of
// arguments and flag values. It runs after all commands are added.
func addCompletion(app *App, root *cobra.Command) {
	root.InitDefaultCompletionCmd()
	comp := mustFind(root, "completion")
	comp.Args = noArgs
	// Without RunE, cobra shows the help and skips Args, so an unknown shell would exit 0.
	comp.RunE = func(cmd *cobra.Command, _ []string) error { return cmd.Help() }
	for _, c := range comp.Commands() {
		c.Args = noArgs
	}

	pf := root.PersistentFlags()
	must(root.RegisterFlagCompletionFunc("output", fixed(string(output.Text), string(output.JSON))))
	must(root.RegisterFlagCompletionFunc("context", app.completeContexts))
	must(pf.SetAnnotation("dir", cobra.BashCompSubdirsInDir, []string{}))

	initCmd := mustFind(root, "init")
	must(initCmd.RegisterFlagCompletionFunc("storage", fixed(workspace.StorageModes...)))
	must(initCmd.RegisterFlagCompletionFunc("tls", fixed(workspace.TLSModes...)))
	must(initCmd.RegisterFlagCompletionFunc("bind-ip", fixed("127.0.0.1", "0.0.0.0")))
	must(initCmd.RegisterFlagCompletionFunc("expose", fixed(workspace.ExposeOptions...)))
	must(initCmd.RegisterFlagCompletionFunc("with", fixed(workspace.WithOptions...)))
	must(initCmd.MarkFlagFilename("cert", "pem", "crt"))
	must(initCmd.MarkFlagFilename("key", "pem", "key"))
	must(initCmd.MarkFlagFilename("from", "yaml", "yml"))
	must(initCmd.MarkFlagDirname("data-dir"))

	must(mustFind(root, "doctor").RegisterFlagCompletionFunc("check", func(*cobra.Command, []string, string) ([]cobra.Completion, cobra.ShellCompDirective) {
		return doctor.IDs(app.doctorEnv(), doctor.Options{}), cobra.ShellCompDirectiveNoFileComp
	}))
	must(mustFind(root, "secrets", "rotate").RegisterFlagCompletionFunc("only", fixed(stack.RotateTargets...)))
	must(mustFind(root, "config", "set").RegisterFlagCompletionFunc("system", fixed(contexts.Systems...)))
	must(mustFind(root, "config", "get").RegisterFlagCompletionFunc("name", app.completeContexts))
	for _, name := range []string{"use", "delete"} {
		c := mustFind(root, "config", name)
		c.ValidArgsFunction = func(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
			if len(args) > 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			return app.completeContexts(cmd, args, toComplete)
		}
	}
	mustFind(root, "logs").ValidArgsFunction = app.completeServices
}

// completeContexts lists the saved context names.
func (a *App) completeContexts(*cobra.Command, []string, string) ([]cobra.Completion, cobra.ShellCompDirective) {
	cfg, err := a.contexts()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return cfg.Names(), cobra.ShellCompDirectiveNoFileComp
}

// completeServices lists the services of the workspace that are not in args yet.
// It reads only openarch.yaml, never the secrets.
func (a *App) completeServices(_ *cobra.Command, args []string, _ string) ([]cobra.Completion, cobra.ShellCompDirective) {
	dir, err := workspace.Find(a.Opts.Dir)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	cfg, err := workspace.Load(filepath.Join(dir, workspace.ConfigFile))
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	var names []string
	for _, s := range cfg.ServiceNames() {
		if !slices.Contains(args, s) {
			names = append(names, s)
		}
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

func fixed(values ...string) cobra.CompletionFunc {
	return cobra.FixedCompletions(values, cobra.ShellCompDirectiveNoFileComp)
}

// mustFind returns the command at path. A wrong path is a programming error.
func mustFind(root *cobra.Command, path ...string) *cobra.Command {
	c, _, err := root.Find(path)
	if err != nil || c.CommandPath() != root.Name()+" "+strings.Join(path, " ") {
		panic("completion: no command " + strings.Join(path, " "))
	}
	return c
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
