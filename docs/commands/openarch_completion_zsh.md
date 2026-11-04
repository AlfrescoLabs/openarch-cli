## openarch completion zsh

Generate the autocompletion script for zsh

### Synopsis

Generate the autocompletion script for the zsh shell.

If shell completion is not already enabled in your environment you will need
to enable it.  You can execute the following once:

	echo "autoload -U compinit; compinit" >> ~/.zshrc

To load completions in your current shell session:

	source <(openarch completion zsh)

To load completions for every new session, execute once:

#### Linux:

	openarch completion zsh > "${fpath[1]}/_openarch"

#### macOS:

	openarch completion zsh > $(brew --prefix)/share/zsh/site-functions/_openarch

You will need to start a new shell for this setup to take effect.


```
openarch completion zsh [flags]
```

### Options

```
  -h, --help              help for zsh
      --no-descriptions   disable completion descriptions
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

* [openarch completion](openarch_completion.md)	 - Generate the autocompletion script for the specified shell

