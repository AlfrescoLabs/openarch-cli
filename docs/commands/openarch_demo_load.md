## openarch demo load

Create the demo folders and documents

### Synopsis

Create the demo folders and documents under /Demo, as the workspace admin.
Public can be read by every user, HR by demo-hr, Finance by demo-finance, and
Board by administrators only. Some folders and documents have their own access
entries: Public/Facilities blocks what it inherits and gives bob access, a
Finance document adds alice, and another one gave alice access that has ended.
The documents have metadata only (title and description).

--embeddings also stores a vector for each document, made from its title and
description with a local model on Docker Model Runner, so that
'openarch query --semantic' finds documents by meaning. OpenArch keeps the
vector with the access entries of its document.

The demo users must exist in OpenArch: run 'openarch demo users', then
'openarch up'. When /Demo exists, nothing is loaded; --force deletes it first.

```
openarch demo load [flags]
```

### Examples

```
  openarch demo load
  openarch demo load --force --output json
  openarch demo load --force --embeddings
```

### Options

```
      --embeddings         also store a vector for each document, made with a local model
      --force              delete /Demo first when it exists
  -h, --help               help for load
      --insecure           do not verify the server certificate (env OPENARCH_INSECURE)
      --model string       embedding model on Docker Model Runner (1024 dimensions or fewer) (default "ai/mxbai-embed-large")
      --model-url string   OpenAI-compatible API of Docker Model Runner (default "http://localhost:12434/engines/v1")
      --password string    password (insecure: it stays in the shell history; prefer --password-stdin or env OPENARCH_PASSWORD)
      --password-stdin     read the password from stdin
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

