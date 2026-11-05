## openarch documents upload

Upload a file as a new document

### Synopsis

Upload a local file as a new SysFile document under a parent, given by id
or path.

The name is the file name unless --name is set. The MIME type comes from the
file extension, else from the first bytes of the file, unless --mime-type is
set. OpenArch stores the SHA-256 of the content; 'documents content' checks it.

With --replace, a file document with the same name under the parent gets the
new content, title and description instead of a second document. Without a
document of that name, a new one is uploaded.

OpenArch keeps the file but does not extract its text, so words inside the
file are not found by full-text queries.

```
openarch documents upload <file> [flags]
```

### Examples

```
  openarch documents upload report.pdf --parent /projects
  openarch documents upload notes.txt --parent / --name 2026-notes.txt --title "Notes 2026"
  openarch documents upload photo.jpg --parent /case --description "Sent by Ana on 2026-09-10"
  openarch documents upload report.pdf --parent /projects --replace
```

### Options

```
      --description string   sys_description
  -h, --help                 help for upload
      --mime-type string     MIME type of the content (default: from the extension or the content)
      --name string          sys_name (default: the file name)
      --parent string        parent document: id, or path starting with /
      --replace              put the content into the file document with the same name, if there is one
      --title string         sys_title (default: the name)
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

