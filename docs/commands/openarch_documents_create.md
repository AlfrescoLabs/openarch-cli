## openarch documents create

Create a document under a parent

### Synopsis

Create a document under a parent, given by id or path.

The document is a JSON object from --file (use - for stdin), or built from
--type, --name, --title and --description. These flags also change the values
of --file.
sys_primaryType is required, for example SysFolder.

```
openarch documents create [flags]
```

### Examples

```
  openarch documents create --parent / --type SysFolder --name projects --title Projects
  openarch documents create --parent /projects --file report.json
  echo '{"sys_primaryType":"SysFolder","sys_name":"x"}' | openarch documents create --parent / --file -
```

### Options

```
      --description string   sys_description
      --file string          JSON file with the document, or - for stdin
  -h, --help                 help for create
      --name string          sys_name
      --parent string        parent document: id, or path starting with /
      --title string         sys_title
      --type string          sys_primaryType, for example SysFolder
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

