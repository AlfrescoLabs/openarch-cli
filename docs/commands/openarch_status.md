## openarch status

Show the state of each service and of the OpenArch API

### Synopsis

Show the state and health of each service, and the result of a request to
the OpenArch health endpoint.

The stack state is healthy, starting, unhealthy or stopped.
Exit code 0 means healthy. Exit code 4 means any other state.

```
openarch status [flags]
```

### Examples

```
  openarch status
  openarch status --output json
```

### Options

```
  -h, --help   help for status
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

