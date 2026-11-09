## openarch mcp

Run an MCP server that exposes the commands as tools

### Synopsis

Run a Model Context Protocol (MCP) server on stdin and stdout, so that an AI
agent can install and operate OpenArch with the same commands as the CLI.

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
```

### Options

```
      --allow-destructive      add the tools that stop services or delete data
      --allow-insecure         let the init tool take the options that turn security off
  -h, --help                   help for mcp
      --max-result-bytes int   cut longer tool results, for models with a small context; 0: no limit (default 16384)
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

