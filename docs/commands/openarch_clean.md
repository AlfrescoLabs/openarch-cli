## openarch clean

Stop OpenArch and delete its data

### Synopsis

Stop the stack and delete its containers, network and volumes: all the data
in MongoDB, OpenSearch and the OpenArch files. The workspace files stay, so
'openarch up' starts again with an empty OpenArch.

With storage bind, the data folders are not deleted.
Make a backup first with 'openarch backup'. Without a terminal, --yes is required.

```
openarch clean [flags]
```

### Examples

```
  openarch clean
  openarch clean --dir my-openarch --yes
```

### Options

```
      --dry-run   print the docker compose command, and run nothing
  -h, --help      help for clean
      --yes       do not ask for confirmation
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

