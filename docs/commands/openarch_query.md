## openarch query

Run a query and list the documents found

### Synopsis

Run a query with POST /api/query and list the documents found.

The default syntax is HxQL, a SQL-like language, for example:
  SELECT * FROM SysFolder WHERE sys_parentPath = '/'
Use --syntax expr for the expression syntax.

New documents can take a moment to reach the search index. --sync waits for it.
With --output json, the result is the server response as is.

```
openarch query <query> [flags]
```

### Examples

```
  openarch query "SELECT * FROM SysContent"
  openarch query "SELECT * FROM SysFolder" --limit 50 --sort sys_title
  openarch query "SELECT * FROM SysContent" --sync --total --output json
```

### Options

```
  -h, --help              help for query
      --insecure          do not verify the server certificate (env OPENARCH_INSECURE)
      --limit int         maximum number of documents (default 10)
      --offset int        number of documents to skip
      --password string   password (insecure: it stays in the shell history; prefer --password-stdin or env OPENARCH_PASSWORD)
      --password-stdin    read the password from stdin
      --sort strings      sort by these fields (repeatable)
      --sync              wait until the search index has the latest changes
      --syntax string     query syntax: hxql (default) or expr
      --total             count every match, not only up to the server limit
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

