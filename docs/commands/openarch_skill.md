## openarch skill

Install the Agent Skills that teach AI agents to use openarch

### Synopsis

Install the Agent Skills that teach AI agents to use openarch.

A skill is a SKILL.md file that an agent loads when a task needs it.
- openarch: when to use the CLI, the install flow, how to read the JSON
  output and the exit codes, and the fixes for common failures.
- openarch-search: how to answer questions from the documents as one user,
  with that user's context, so the answer has only what the user may read.

Claude Code, Codex, GitHub Copilot, Docker Agent and other agents that read
Agent Skills load them. The skills are part of the binary, so they match the
commands of this release.

### Examples

```
  openarch skill install
  openarch skill install --dir ~
  openarch skill show openarch-search
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
* [openarch skill install](openarch_skill_install.md)	 - Write the skills to .agents/skills and .claude/skills
* [openarch skill show](openarch_skill_show.md)	 - Print a skill

