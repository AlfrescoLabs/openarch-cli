## openarch skill install

Write the skill to .claude/skills/openarch/SKILL.md

### Synopsis

Write the skill to .claude/skills/openarch/SKILL.md under the folder given by
--dir, or the current folder. In a project folder, agents use the skill for
that project. With --dir ~ they use it in every project.

Run it again after an update of openarch to get the skill of the new release.
A SKILL.md that differs from this release is replaced only with --force.

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

* [openarch skill](openarch_skill.md)	 - Install the Agent Skill that teaches AI coding agents to use openarch

