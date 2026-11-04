## openarch backup

Back up MongoDB, the OpenArch files and the OpenSearch indices

### Synopsis

Back up the data of a running workspace into a new folder:
  mongodb.archive.gz   a mongodump of the OpenArch database
  files.tar.gz         the OpenArch files (binary content)
  opensearch.tar.gz    an OpenSearch snapshot of the OpenArch indices
  manifest.json        the version, and a SHA-256 for each file

The folder is backups/<time> in the workspace (git-ignored), or in --to.
The backup runs while OpenArch runs: changes made during the backup may be
missing from it. The backup holds all the data: keep it private.

```
openarch backup [flags]
```

### Examples

```
  openarch backup
  openarch backup --to /srv/backups/openarch --output json
```

### Options

```
  -h, --help        help for backup
      --to string   folder for the backup (default: backups/ in the workspace)
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

