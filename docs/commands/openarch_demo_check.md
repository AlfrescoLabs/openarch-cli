## openarch demo check

Check that each demo user gets only the documents they may read

### Synopsis

Check that each demo user gets only the documents they may read. The check
logs in as the workspace admin and as alice, bob and carol, with the passwords
that 'demo users' stored, and compares what OpenArch returns with the access
entries of the demo data: document lists from the search index and from the
database, title queries, and reads of every document by id.

Each demo document has a unique code in its description. A response with the
code, the name or the content of a document the user may not read is a leak.
A document the user may read but does not get is a problem.

--semantic also asks five questions with vector search (see 'demo load
--embeddings'). The questions become vectors with a local model on Docker
Model Runner.

The last line sums up the result and the versions, and can be shared as is.
Exit code 0 means no leak and no problem, 4 means at least one.

```
openarch demo check [flags]
```

### Examples

```
  openarch demo check
  openarch demo check --semantic
  openarch demo check --output json
```

### Options

```
  -h, --help               help for check
      --insecure           do not verify the server certificate (env OPENARCH_INSECURE)
      --model string       embedding model on Docker Model Runner (1024 dimensions or fewer) (default "ai/mxbai-embed-large")
      --model-url string   OpenAI-compatible API of Docker Model Runner (default "http://localhost:12434/engines/v1")
      --password string    password (insecure: it stays in the shell history; prefer --password-stdin or env OPENARCH_PASSWORD)
      --password-stdin     read the password from stdin
      --semantic           also check vector search, with a local model
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

* [openarch demo](openarch_demo.md)	 - Add sample users and documents that show permission-aware search

