## openarch documents list

List the documents in a folder

### Synopsis

List the documents in a folder, given by id or path (default: the root "/"),
sorted by name.

With --output json, the result is the server query result as is.

```
openarch documents list [folder] [flags]
```

### Examples

```
  openarch documents list
  openarch documents list /projects --limit 20 --offset 20
  openarch documents list /projects --output json
```

### Options

```
  -h, --help         help for list
      --limit int    maximum number of documents (default 100)
      --offset int   number of documents to skip
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

