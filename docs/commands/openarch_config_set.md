## openarch config set

Save a remote connection and its password

### Synopsis

Save the URL and user of an OpenArch server as a context, and its password in
the credential store. The password comes from stdin (--password-stdin) or a
hidden prompt. It is never a command argument.

Before saving, set checks the connection with 'api ping'. --no-verify skips it.
The first context saved becomes the current context.

```
openarch config set [flags]
```

### Examples

```
  openarch config set -s https://openarch.example.com -u admin
  printf '%s' "$PASSWORD" | openarch config set --name prod -s https://openarch.example.com -u admin --password-stdin
```

### Options

```
  -h, --help              help for set
      --insecure          do not verify the server certificate
      --name string       context name (default "default")
      --no-verify         save without checking the connection
      --password-stdin    read the password from stdin
  -s, --server string     server URL
      --system string     system of the endpoint: openarch (default "openarch")
  -u, --username string   user name
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

* [openarch config](openarch_config.md)	 - Manage saved connections (contexts)

