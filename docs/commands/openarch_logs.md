## openarch logs

Show the logs of the services

### Synopsis

Show the logs of all services, or of the given ones.

With --output json, the lines are returned as one JSON document, so --follow
cannot be used.

```
openarch logs [service...] [flags]
```

### Examples

```
  openarch logs openarch-app
  openarch logs --follow --tail 100
  openarch logs mongodb-init --output json
```

### Options

```
  -f, --follow         keep printing new lines
  -h, --help           help for logs
      --since string   show lines since a time (2026-11-03T10:00:00) or a duration (10m)
      --tail string    number of lines from the end of each log, or all
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

