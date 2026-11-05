package cli

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/AlfrescoLabs/openarch-cli/internal/output"
	"github.com/AlfrescoLabs/openarch-cli/skills"
)

func newSkillCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skill",
		Short: "Install the Agent Skill that teaches AI coding agents to use openarch",
		Long: `Install the Agent Skill that teaches AI coding agents to use openarch.

The skill is a SKILL.md file: when to use the CLI, the install flow, how to
read the JSON output and the exit codes, and the fixes for common failures.
Claude Code and other agents that read Agent Skills load it when a task needs
it. The skill is part of the binary, so it matches the commands of this release.`,
		Example: `  openarch skill install
  openarch skill install --dir ~
  openarch skill show`,
		Args: noArgs,
	}
	cmd.AddCommand(newSkillInstallCmd(app), newSkillShowCmd(app))
	return cmd
}

// SkillInstallResult is the outcome of 'skill install'.
type SkillInstallResult struct {
	Path   string `json:"path"`
	Action string `json:"action"`
}

func (r SkillInstallResult) WriteText(w io.Writer) error {
	if r.Action == skills.Unchanged {
		_, err := fmt.Fprintf(w, "The skill is up to date: %s\n", r.Path)
		return err
	}
	_, err := fmt.Fprintf(w, "Installed the skill: %s\n", r.Path)
	return err
}

func newSkillInstallCmd(app *App) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Write the skill to .claude/skills/openarch/SKILL.md",
		Long: `Write the skill to .claude/skills/openarch/SKILL.md under the folder given by
--dir, or the current folder. In a project folder, agents use the skill for
that project. With --dir ~ they use it in every project.

Run it again after an update of openarch to get the skill of the new release.
A SKILL.md that differs from this release is replaced only with --force.`,
		Example: `  openarch skill install
  openarch skill install --dir ~/projects/my-app
  openarch skill install --dir ~ --force`,
		Args: noArgs,
		RunE: func(*cobra.Command, []string) error {
			dir := app.Opts.Dir
			if dir == "" {
				dir = "."
			}
			dir, err := filepath.Abs(dir)
			if err != nil {
				return err
			}
			path, action, err := skills.Install(dir, force)
			if errors.Is(err, skills.ErrChanged) {
				return output.Usage(path+" exists and differs from the skill of this release", "Use --force to replace it.")
			}
			if err != nil {
				return output.NewError(output.ExitError, "cannot write the skill: "+err.Error(), "")
			}
			return app.Printer.Print(SkillInstallResult{Path: path, Action: action})
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "replace a SKILL.md that differs from this release")
	return cmd
}

// SkillContent is the result of 'skill show'. The text output is the file itself.
type SkillContent struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

func (s SkillContent) WriteText(w io.Writer) error {
	_, err := io.WriteString(w, s.Content)
	return err
}

func newSkillShowCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Print the skill",
		Long: `Print the SKILL.md of this release. Use it for agents that read skills from
another folder, or to read the skill before you install it.`,
		Example: `  openarch skill show
  openarch skill show > SKILL.md`,
		Args: noArgs,
		RunE: func(*cobra.Command, []string) error {
			return app.Printer.Print(SkillContent{Name: skills.Name, Content: string(skills.SKILL)})
		},
	}
}
