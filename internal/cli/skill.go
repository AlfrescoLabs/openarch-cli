package cli

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/AlfrescoLabs/openarch-cli/internal/output"
	"github.com/AlfrescoLabs/openarch-cli/skills"
)

func newSkillCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skill",
		Short: "Install the Agent Skills that teach AI agents to use openarch",
		Long: `Install the Agent Skills that teach AI agents to use openarch.

A skill is a SKILL.md file that an agent loads when a task needs it.
- openarch: when to use the CLI, the install flow, how to read the JSON
  output and the exit codes, and the fixes for common failures.
- openarch-search: how to answer questions from the documents as one user,
  with that user's context, so the answer has only what the user may read.

Claude Code, Codex, GitHub Copilot, Docker Agent and other agents that read
Agent Skills load them. The skills are part of the binary, so they match the
commands of this release.`,
		Example: `  openarch skill install
  openarch skill install --dir ~
  openarch skill show openarch-search`,
		Args: noArgs,
	}
	cmd.AddCommand(newSkillInstallCmd(app), newSkillShowCmd(app))
	return cmd
}

// SkillInstallResult is the outcome of 'skill install'.
type SkillInstallResult struct {
	Files []skills.File `json:"files"`
}

func (r SkillInstallResult) WriteText(w io.Writer) error {
	var b strings.Builder
	for _, f := range r.Files {
		if f.Action == skills.Unchanged {
			fmt.Fprintf(&b, "Up to date: %s\n", f.Path)
		} else {
			fmt.Fprintf(&b, "Installed:  %s\n", f.Path)
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func newSkillInstallCmd(app *App) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Write the skills to .agents/skills and .claude/skills",
		Long: `Write each skill to .agents/skills/<name>/SKILL.md and
.claude/skills/<name>/SKILL.md under the folder given by --dir, or the current
folder. Agents that follow the cross-client convention of Agent Skills read
.agents/skills; Claude Code reads .claude/skills. In a project folder, agents
use the skills for that project. With --dir ~ they use them in every project.

Run it again after an update of openarch to get the skills of the new release.
A SKILL.md that differs from this release is replaced only with --force;
without it, nothing is written.`,
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
			files, err := skills.Install(dir, force)
			if errors.Is(err, skills.ErrChanged) {
				return output.Usage(files[0].Path+" exists and differs from the skill of this release", "Use --force to replace it.")
			}
			if err != nil {
				return output.NewError(output.ExitError, "cannot write the skills: "+err.Error(), "")
			}
			return app.Printer.Print(SkillInstallResult{Files: files})
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
	var names []string
	for _, s := range skills.All {
		names = append(names, s.Name)
	}
	return &cobra.Command{
		Use:   "show [<name>]",
		Short: "Print a skill",
		Long: `Print the SKILL.md of a skill of this release: ` + strings.Join(names, ", ") + `.
The default is openarch. Use it for agents that read skills from another
folder, or to read a skill before you install it.`,
		Example: `  openarch skill show
  openarch skill show openarch-search > SKILL.md`,
		Args:      maxArgs(1),
		ValidArgs: names,
		RunE: func(_ *cobra.Command, args []string) error {
			name := "openarch"
			if len(args) == 1 {
				name = args[0]
			}
			s, ok := skills.Get(name)
			if !ok {
				return output.Usage(fmt.Sprintf("no skill named %q", name), "The skills are: "+strings.Join(names, ", ")+".")
			}
			return app.Printer.Print(SkillContent{Name: s.Name, Content: string(s.Content)})
		},
	}
}
