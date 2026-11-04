## openarch users delete

Delete a user

### Synopsis

Delete a user from config/users.yml. The workspace admin cannot be deleted. Without a terminal, --yes is required.

```
openarch users delete <username> [flags]
```

### Examples

```
  openarch users delete alice --yes
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

* [openarch users](openarch_users.md)	 - Manage the OpenArch users of a workspace

