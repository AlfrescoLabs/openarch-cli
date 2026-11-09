## openarch skill

Install the Agent Skill that teaches AI coding agents to use openarch

### Synopsis

Install the Agent Skill that teaches AI coding agents to use openarch.

The skill is a SKILL.md file: when to use the CLI, the install flow, how to
read the JSON output and the exit codes, and the fixes for common failures.
Claude Code and other agents that read Agent Skills load it when a task needs
it. The skill is part of the binary, so it matches the commands of this release.

### Examples

```
  openarch skill install
  openarch skill install --dir ~
  openarch skill show
```

### Options

```
  -h, --help   help for skill
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

* [openarch](openarch.md)	 - CLI for OpenArch: install and operate Hyland OpenArch
* [openarch skill install](openarch_skill_install.md)	 - Write the skill to .claude/skills/openarch/SKILL.md
* [openarch skill show](openarch_skill_show.md)	 - Print the skill

