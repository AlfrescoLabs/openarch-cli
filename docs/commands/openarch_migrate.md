## openarch migrate

Update openarch.yaml to the schema of this openarch

### Synopsis

Update openarch.yaml to the current schema (schemaVersion 2).

Commands read an older openarch.yaml and print a warning. 'render' and
'upgrade' save the file with the current schema too. Only openarch.yaml
changes: the other workspace files do not depend on the schema.

A file written by a newer openarch gives exit code 3. Update openarch with
'openarch self-update'.

```
openarch migrate [flags]
```

### Examples

```
  openarch migrate --dry-run
  openarch migrate --dir my-openarch
  openarch migrate --output json
```

### Options

```
      --dry-run   show the change as a diff, and write nothing
  -h, --help      help for migrate
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

