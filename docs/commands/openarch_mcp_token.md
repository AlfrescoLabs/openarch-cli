## openarch mcp token

Print the bearer token of 'openarch mcp --listen'

### Synopsis

Print the bearer token that MCP clients send to 'openarch mcp --listen'.

The first call makes the token. It is kept in the credential store, like the
OpenArch passwords, and every HTTP server of this user uses it. --rotate makes
a new token; a running server keeps the old one until it starts again.

```
openarch mcp token [flags]
```

### Examples

```
  openarch mcp token
  openarch mcp token --rotate
  claude mcp add --transport http openarch http://127.0.0.1:8765/mcp --header "Authorization: Bearer $(openarch mcp token)"
```

### Options

```
  -h, --help     help for token
      --rotate   make a new token
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

* [openarch mcp](openarch_mcp.md)	 - Run an MCP server that exposes the commands as tools

