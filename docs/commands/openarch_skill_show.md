## openarch skill show

Print a skill

### Synopsis

Print the SKILL.md of a skill of this release: openarch, openarch-search.
The default is openarch. Use it for agents that read skills from another
folder, or to read a skill before you install it.

```
openarch skill show [<name>] [flags]
```

### Examples

```
  openarch skill show
  openarch skill show openarch-search > SKILL.md
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

* [openarch skill](openarch_skill.md)	 - Install the Agent Skills that teach AI agents to use openarch

