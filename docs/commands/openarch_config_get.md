## openarch config get

Show a context

### Synopsis

Show a context. Passwords are masked unless --show-secrets is set.

```
openarch config get [flags]
```

### Examples

```
  openarch config get
  openarch config get --name prod --output json
```

### Options

```
  -h, --help           help for get
      --name string    context name (default: the current context)
      --show-secrets   show the passwords
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

* [openarch config](openarch_config.md)	 - Manage saved connections (contexts)

