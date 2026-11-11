## openarch mcp

Run an MCP server that exposes the commands as tools

### Synopsis

Run a Model Context Protocol (MCP) server, so that an AI agent can install
and operate OpenArch with the same commands as the CLI.

The server runs on stdin and stdout. With --listen it serves Streamable HTTP
on host:port instead, at the path /mcp, for remote agents or several
clients. A port alone means 127.0.0.1, so --listen 8765 is 127.0.0.1:8765.
It prints its URL, then runs until it is stopped. Every HTTP request needs the header "Authorization: Bearer
<token>"; 'openarch mcp token' prints the token. --no-auth turns the token
off. On an address that is not loopback, --no-auth also needs
--allow-remote-no-auth.

Each tool runs one command with --output json and returns its JSON result.
A failed command gives an error result with its exit code. Command progress
becomes MCP progress notifications, and logs go to stderr.

--dir and --context pin the server to one workspace or connection; the
default is the current folder. Tools never take passwords, and no tool
returns one.

Tools that stop services or delete data (down, clean, restore, secrets
rotate, users delete, config delete) exist only with --allow-destructive.

The init tool always keeps the secure defaults. Its options that can turn
security off (default-users, insecure-backends, bind-ip, expose, tls, cert,
key, from) exist only with --allow-insecure.

A tool result longer than --max-result-bytes is cut, with a note that names
the parameters for a smaller result (tail, limit, offset).

```
openarch mcp [flags]
```

### Examples

```
  openarch mcp --dir my-openarch
  openarch mcp --dir my-openarch --allow-destructive
  claude mcp add openarch -- openarch mcp --dir /path/to/my-openarch
  openarch mcp --dir my-openarch --listen 8765
  claude mcp add --transport http openarch http://127.0.0.1:8765/mcp --header "Authorization: Bearer $(openarch mcp token)"
```

### Options

```
      --allow-destructive      add the tools that stop services or delete data
      --allow-insecure         let the init tool take the options that turn security off
      --allow-remote-no-auth   allow --no-auth on an address that is not loopback
  -h, --help                   help for mcp
      --listen string          serve Streamable HTTP on host:port instead of stdio; a port alone means 127.0.0.1
      --max-result-bytes int   cut longer tool results, for models with a small context; 0: no limit (default 16384)
      --no-auth                serve HTTP without the bearer token
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
* [openarch mcp token](openarch_mcp_token.md)	 - Print the bearer token of 'openarch mcp --listen'

