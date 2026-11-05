## openarch skill show

Print the skill

### Synopsis

Print the SKILL.md of this release. Use it for agents that read skills from
another folder, or to read the skill before you install it.

```
openarch skill show [flags]
```

### Examples

```
  openarch skill show
  openarch skill show > SKILL.md
```

### Options

```
  -h, --help   help for show
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

