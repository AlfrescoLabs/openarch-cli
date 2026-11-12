## openarch demo reset

Delete the demo folders and documents

### Synopsis

Delete /Demo and everything in it. The demo users stay. Without a terminal, --yes is required.

```
openarch demo reset [flags]
```

### Examples

```
  openarch demo reset --yes
```

### Options

```
  -h, --help              help for reset
      --insecure          do not verify the server certificate (env OPENARCH_INSECURE)
      --password string   password (insecure: it stays in the shell history; prefer --password-stdin or env OPENARCH_PASSWORD)
      --password-stdin    read the password from stdin
      --url string        OpenArch URL (env OPENARCH_URL; default: the workspace URL)
      --user string       user name (env OPENARCH_USER; default: the workspace admin)
      --yes               do not ask for confirmation
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

* [openarch demo](openarch_demo.md)	 - Add sample users and documents that show permission-aware search

