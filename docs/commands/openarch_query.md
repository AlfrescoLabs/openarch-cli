## openarch query

Run a query and list the documents found

### Synopsis

Run a query with POST /api/query and list the documents found.

The default syntax is HxQL, a SQL-like language, for example:
  SELECT * FROM SysFolder WHERE sys_parentPath = '/'
Only SELECT * returns documents. A query that names columns, such as
SELECT sys_name FROM SysFile, finds no documents.
Use --syntax expr for the expression syntax.

New documents can take a moment to reach the search index. --sync waits for it.
With --output json, the result is the server response as is.

--semantic finds the documents nearest in meaning to a text: the text becomes a
vector with a local model on Docker Model Runner, and POST /api/query/embeddings
compares it with the stored vectors of the same model (see 'openarch demo load
--embeddings'). The query, when given, selects the documents to search. Only
documents the caller may read are found.

```
openarch query [<query>] [flags]
```

### Examples

```
  openarch query "SELECT * FROM SysContent"
  openarch query "SELECT * FROM SysFolder" --limit 50 --sort sys_title
  openarch query "SELECT * FROM SysContent" --sync --total --output json
  openarch query --semantic "How much is the pay for each job level?"
  openarch query --semantic "office move" "SELECT * FROM SysFile WHERE sys_path STARTSWITH '/Demo/'" --limit 3
```

### Options

```
  -h, --help               help for query
      --insecure           do not verify the server certificate (env OPENARCH_INSECURE)
      --limit int          maximum number of documents (default 10)
      --model string       embedding model on Docker Model Runner (1024 dimensions or fewer) (default "ai/mxbai-embed-large")
      --model-url string   OpenAI-compatible API of Docker Model Runner (default "http://localhost:12434/engines/v1")
      --offset int         number of documents to skip
      --password string    password (insecure: it stays in the shell history; prefer --password-stdin or env OPENARCH_PASSWORD)
      --password-stdin     read the password from stdin
      --semantic string    find the documents nearest in meaning to this text
      --sort strings       sort by these fields (repeatable)
      --sync               wait until the search index has the latest changes
      --syntax string      query syntax: hxql (default) or expr
      --total              count every match, not only up to the server limit
      --url string         OpenArch URL (env OPENARCH_URL; default: the workspace URL)
      --user string        user name (env OPENARCH_USER; default: the workspace admin)
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

