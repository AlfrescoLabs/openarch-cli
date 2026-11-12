## openarch demo

Add sample users and documents that show permission-aware search

### Synopsis

Add sample users and documents that show permission-aware search: the
same query returns different documents for each user.

'demo users' adds the users alice (group demo-hr) and bob (group demo-finance)
with generated passwords, and saves a context for each one. OpenArch reads its
users only at startup, so run it before 'openarch up', or run 'openarch up'
after it.

```
openarch demo [flags]
```

### Examples

```
  openarch demo users
  openarch up
  openarch query "SELECT * FROM SysFile" --context local-alice
```

### Options

```
  -h, --help   help for demo
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
* [openarch demo users](openarch_demo_users.md)	 - Add the demo users and a context for each one

