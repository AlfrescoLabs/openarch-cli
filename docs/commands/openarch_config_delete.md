## openarch config delete

Delete a context and its stored passwords

### Synopsis

Delete a context and the passwords it has in the credential store.
A workspace context only points to the workspace: the workspace is not changed.

Without a terminal, --yes is required.

```
openarch config delete <name> [flags]
```

### Examples

```
  openarch config delete prod --yes
```

### Options

```
  -h, --help   help for delete
      --yes    do not ask for confirmation
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

