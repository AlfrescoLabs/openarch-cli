## openarch completion

Generate the autocompletion script for the specified shell

### Synopsis

Generate the autocompletion script for openarch for the specified shell.
See each sub-command's help for details on how to use the generated script.


```
openarch completion [flags]
```

### Examples

```
  source <(openarch completion zsh)
  openarch completion fish > ~/.config/fish/completions/openarch.fish
```

### Options

```
  -h, --help   help for completion
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
* [openarch completion bash](openarch_completion_bash.md)	 - Generate the autocompletion script for bash
* [openarch completion fish](openarch_completion_fish.md)	 - Generate the autocompletion script for fish
* [openarch completion powershell](openarch_completion_powershell.md)	 - Generate the autocompletion script for powershell
* [openarch completion zsh](openarch_completion_zsh.md)	 - Generate the autocompletion script for zsh

