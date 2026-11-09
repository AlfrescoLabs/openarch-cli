## openarch restore

Replace the data of the workspace with a backup

### Synopsis

Replace the data of the workspace with a backup made by 'openarch backup'.

restore checks every file of the backup first. Then it stops OpenArch,
restores MongoDB, the OpenArch files and the OpenSearch indices, starts the
stack and waits until it is healthy. The current data is lost.

It works on a running stack, and after 'openarch clean'.
Without a terminal, --yes is required.

```
openarch restore <backup-folder> [flags]
```

### Examples

```
  openarch restore backups/20261103T120000Z
  openarch restore /srv/backups/openarch/20261103T120000Z --yes
```

### Options

```
  -h, --help               help for restore
      --timeout duration   how long to wait until the stack is healthy (default 10m0s)
      --yes                do not ask for confirmation
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

