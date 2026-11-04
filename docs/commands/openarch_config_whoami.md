## openarch config whoami

Show which URL and user a command would use, and where they come from

### Synopsis

Show the URL, user and password source that commands calling the API would
use, with the same flags and environment variables. It never shows the password
and never asks for one.

```
openarch config whoami [flags]
```

### Examples

```
  openarch config whoami
  openarch config whoami --context prod --output json
```

### Options

```
  -h, --help              help for whoami
      --insecure          do not verify the server certificate (env OPENARCH_INSECURE)
      --password string   password (insecure: it stays in the shell history; prefer --password-stdin or env OPENARCH_PASSWORD)
      --url string        OpenArch URL (env OPENARCH_URL; default: the workspace URL)
      --user string       user name (env OPENARCH_USER; default: the workspace admin)
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

