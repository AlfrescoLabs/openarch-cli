## openarch documents

Get, create, list, upload and download documents

### Synopsis

Get, create, list, upload and download documents. A document is given by
its id, or by its path when the value starts with "/" (the root is "/").

With --output json, a document is the server response as is.

```
openarch documents [flags]
```

### Examples

```
  openarch documents get /
  openarch documents list /projects
  openarch documents create --parent / --type SysFolder --name projects
  openarch documents upload report.pdf --parent /projects
  openarch documents content /projects/report.pdf --out report.pdf
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

* [openarch](openarch.md)	 - CLI for OpenArch: install and operate Hyland OpenArch
* [openarch documents content](openarch_documents_content.md)	 - Download the file of a document
* [openarch documents create](openarch_documents_create.md)	 - Create a document under a parent
* [openarch documents get](openarch_documents_get.md)	 - Show a document
* [openarch documents list](openarch_documents_list.md)	 - List the documents in a folder
* [openarch documents update](openarch_documents_update.md)	 - Change the properties of a document
* [openarch documents upload](openarch_documents_upload.md)	 - Upload a file as a new document

