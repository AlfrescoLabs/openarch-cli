## openarch users set-password

Change the password of a user

### Synopsis

Change the password of a user in config/users.yml. For the workspace admin,
the password in .env.local changes too, so the CLI keeps working.

```
openarch users set-password <username> [flags]
```

### Examples

```
  openarch users set-password alice
  printf '%s' "$PASSWORD" | openarch users set-password admin --password-stdin
```

### Options

```
  -h, --help             help for set-password
      --password-stdin   read the password from stdin
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

