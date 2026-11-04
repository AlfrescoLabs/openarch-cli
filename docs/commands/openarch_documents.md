## openarch documents

Get and create documents

### Synopsis

Get and create documents. A document is given by its id, or by its path
when the value starts with "/" (the root is "/").

With --output json, a document is the server response as is.

```
openarch documents [flags]
```

### Examples

```
  openarch documents get /
  openarch documents create --parent / --type SysFolder --name projects
```

### Options

```
  -h, --help              help for documents
      --insecure          do not verify the server certificate (env OPENARCH_INSECURE)
      --password string   password (insecure: it stays in the shell history; prefer --password-stdin or env OPENARCH_PASSWORD)
      --password-stdin    read the password from stdin
      --url string        OpenArch URL (env OPENARCH_URL; default: the workspace URL)
      --user string       user name (env OPENARCH_USER; default: the workspace admin)
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
* [openarch documents create](openarch_documents_create.md)	 - Create a document under a parent
* [openarch documents get](openarch_documents_get.md)	 - Show a document

