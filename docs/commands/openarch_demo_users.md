## openarch demo users

Add the demo users and a context for each one

### Synopsis

Add the demo users alice, bob and carol to config/users.yml, with
generated passwords. Each user gets a context named <prefix>-<user>, with the password in
the credential store, for use with --context.

The prefix is the name of the workspace context, for example local. A user
that already has a working stored password keeps it, so the command can run
again. Workspaces with the users bundled in the OpenArch image
(security.defaultUsers) cannot have demo users.

```
openarch demo users [flags]
```

### Examples

```
  openarch demo users
  openarch demo users --context-prefix demo --output json
```

### Options

```
      --context-prefix string   prefix of the context names (default: the name of the workspace context)
  -h, --help                    help for users
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

