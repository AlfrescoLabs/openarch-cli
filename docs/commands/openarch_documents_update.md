## openarch documents update

Change the properties of a document

### Synopsis

Change the properties of a document, given by id or path.

The changes are a JSON object from --file (use - for stdin), or built from
--title and --description. These flags also change the values of --file.
Properties that are not in the changes keep their value.

```
openarch documents update <id-or-path> [flags]
```

### Examples

```
  openarch documents update /projects/report.pdf --description "Final version"
  openarch documents update dae8e1bb-ef08-44e5-81ae-451d785a3643 --title "Report 2026"
  echo '{"sys_description":"checked"}' | openarch documents update /projects/report.pdf --file -
```

### Options

```
      --description string   new sys_description
      --file string          JSON file with the changes, or - for stdin
  -h, --help                 help for update
      --title string         new sys_title
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

