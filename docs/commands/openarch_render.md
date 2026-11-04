## openarch render

Write the workspace files again from openarch.yaml

### Synopsis

Write compose.yaml, .env, config/users.yml, .gitignore, README.md and the
TLS files again from openarch.yaml and .env.local.

This is the declarative path: edit openarch.yaml (or the admin password in
.env.local), then run render. Files that would not change are not touched.
With --dry-run, render shows a diff and writes nothing. Secret files are never
shown in the diff.

```
openarch render [flags]
```

### Examples

```
  openarch render
  openarch render --dry-run
  openarch render --dir my-openarch --output json
```

### Options

```
      --dry-run   show what would change, and write nothing
  -h, --help      help for render
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

* [openarch](openarch.md)	 - Install and operate OpenArch

