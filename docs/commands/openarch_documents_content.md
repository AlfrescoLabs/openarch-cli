## openarch documents content

Download the file of a document

### Synopsis

Download the file of a document to --out, and check its SHA-256 against
the digest stored by OpenArch.

--out - writes the file to stdout. Then nothing else is printed there, so it
cannot be used with --output json. An existing file is replaced only with
--force.

```
openarch documents content <id-or-path> [flags]
```

### Examples

```
  openarch documents content /projects/report.pdf --out report.pdf
  openarch documents content /notes.txt --out - | less
  openarch documents content dae8e1bb-ef08-44e5-81ae-451d785a3643 --out copy.pdf --output json
```

### Options

```
      --force        replace an existing file
  -h, --help         help for content
      --out string   file to write, or - for stdout
```

### Options inherited from parent commands

```
      --context string    connection context to use
      --dir string        workspace directory (default: current directory)
      --insecure          do not verify the server certificate (env OPENARCH_INSECURE)
      --no-color          disable colors (env NO_COLOR)
      --non-interactive   never prompt; fail when a value is missing
  -o, --output string     output format: text or json (env OPENARCH_OUTPUT) (default "text")
      --password string   password (insecure: it stays in the shell history; prefer --password-stdin or env OPENARCH_PASSWORD)
      --password-stdin    read the password from stdin
      --url string        OpenArch URL (env OPENARCH_URL; default: the workspace URL)
      --user string       user name (env OPENARCH_USER; default: the workspace admin)
  -v, --verbose           print debug logs to stderr
```

### SEE ALSO

* [openarch documents](openarch_documents.md)	 - Get, create, list, upload and download documents

