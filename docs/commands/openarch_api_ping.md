## openarch api ping

Check that OpenArch answers and that the credentials work

```
openarch api ping [flags]
```

### Examples

```
  openarch api ping
  openarch api ping --dir my-openarch --output json
  OPENARCH_URL=https://openarch.example.com OPENARCH_USER=admin openarch api ping --password-stdin < pw.txt
```

### Options

```
  -h, --help   help for ping
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

* [openarch api](openarch_api.md)	 - Call the OpenArch API

