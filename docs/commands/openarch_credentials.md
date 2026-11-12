## openarch credentials

Show the users and passwords of a local workspace

### Synopsis

Show the users and passwords of a local workspace: the admin user from
.env.local, and the users of the contexts that point to the workspace URL,
such as the users of 'openarch demo users', from the credential store.

The passwords are shown in clear text. Use this on your own machine only.

```
openarch credentials [flags]
```

### Examples

```
  openarch credentials
  openarch credentials --dir my-openarch --output json
```

### Options

```
  -h, --help   help for credentials
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

