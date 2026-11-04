## openarch completion bash

Generate the autocompletion script for bash

### Synopsis

Generate the autocompletion script for the bash shell.

This script depends on the 'bash-completion' package.
If it is not installed already, you can install it via your OS's package manager.

To load completions in your current shell session:

	source <(openarch completion bash)

To load completions for every new session, execute once:

#### Linux:

	openarch completion bash > /etc/bash_completion.d/openarch

#### macOS:

	openarch completion bash > $(brew --prefix)/etc/bash_completion.d/openarch

You will need to start a new shell for this setup to take effect.


```
openarch completion bash
```

### Options

```
  -h, --help              help for bash
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

