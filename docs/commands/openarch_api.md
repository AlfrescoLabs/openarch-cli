## openarch api

Call the OpenArch API

### Synopsis

Call the OpenArch API.

The URL, user and password come from, in this order: flags, environment
variables (OPENARCH_URL, OPENARCH_USER, OPENARCH_PASSWORD), the workspace
(openarch.yaml and .env.local), and a hidden prompt in a terminal.
Workspace credentials are used only with the workspace URL.

Exit code 6 means the credentials are missing or wrong.

```
openarch api [flags]
```

### Options

```
  -h, --help              help for api
      --insecure          do not verify the server certificate (env OPENARCH_INSECURE)
      --password string   password (insecure: it stays in the shell history; prefer --password-stdin or env OPENARCH_PASSWORD)
      --password-stdin    read the password from stdin
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

* [openarch](openarch.md)	 - Install and operate OpenArch
* [openarch api ping](openarch_api_ping.md)	 - Check that OpenArch answers and that the credentials work

