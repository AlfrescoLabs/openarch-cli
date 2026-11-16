## openarch skill install

Write the skills to .agents/skills and .claude/skills

### Synopsis

Write each skill to .agents/skills/<name>/SKILL.md and
.claude/skills/<name>/SKILL.md under the folder given by --dir, or the current
folder. Agents that follow the cross-client convention of Agent Skills read
.agents/skills; Claude Code reads .claude/skills. In a project folder, agents
use the skills for that project. With --dir ~ they use them in every project.

Run it again after an update of openarch to get the skills of the new release.
A SKILL.md that differs from this release is replaced only with --force;
without it, nothing is written.

```
openarch skill install [flags]
```

### Examples

```
  openarch skill install
  openarch skill install --dir ~/projects/my-app
  openarch skill install --dir ~ --force
```

### Options

```
      --force   replace a SKILL.md that differs from this release
  -h, --help    help for install
```

### Options inherited from parent commands

```
      --context string    connection context to use
      --dir string        workspace directory (default: current directory)
      --no-color          disable colors (env NO_COLOR)
      --non-interactive   never prompt; fail when a value is missing
  -o, --output string     output format: text or json (env OPENARCH_OUTPUT) (default "text")
  -v, --verbose           print debug logs to stderr
```

### SEE ALSO

* [openarch skill](openarch_skill.md)	 - Install the Agent Skills that teach AI agents to use openarch

