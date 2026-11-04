## openarch down

Stop OpenArch and remove its containers, keeping the data

### Synopsis

Stop the stack and remove its containers and network. Named volumes and
data folders are kept, so 'openarch up' starts again with the same data.

With storage none there is no data to keep, so its volumes are removed too.

```
openarch down [flags]
```

### Examples

```
  openarch down
  openarch down --dir my-openarch
```

### Options

```
      --dry-run   print the docker compose command, and run nothing
  -h, --help      help for down
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

