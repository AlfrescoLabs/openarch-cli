## openarch users add

Add a user

### Synopsis

Add a user to config/users.yml. The user is in the members group, and also in
administrators with --admin. Other groups given with --group are created.

```
openarch users add <username> [flags]
```

### Examples

```
  openarch users add alice
  printf '%s' "$PASSWORD" | openarch users add bob --admin --email bob@example.com --password-stdin
```

### Options

```
      --admin               add the user to the administrators group
      --email string        email address
      --first-name string   first name
      --group strings       add the user to this group (repeatable)
  -h, --help                help for add
      --last-name string    last name
      --password-stdin      read the password from stdin
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

