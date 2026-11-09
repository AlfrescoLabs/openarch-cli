## openarch secrets

Manage the passwords of a workspace

```
openarch secrets [flags]
```

### Examples

```
  openarch secrets rotate
  openarch secrets rotate --dry-run --output json
```

### Options

```
  -h, --help   help for secrets
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
* [openarch secrets rotate](openarch_secrets_rotate.md)	 - Give new passwords to the users of OpenArch, MongoDB and OpenSearch

