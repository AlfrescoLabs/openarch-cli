## openarch secrets rotate

Give new passwords to the users of OpenArch, MongoDB and OpenSearch

### Synopsis

Give new random passwords to the workspace users:
  mongodb-root   the MongoDB root user
  mongodb-app    the MongoDB user of OpenArch
  opensearch     the OpenSearch users admin, openarch and kibanaserver
  admin          the OpenArch admin

The MongoDB root and OpenSearch passwords change in the running services, so
the stack must run. Each change is saved in .env.local right away. Then the
workspace is rendered, and the services that use a new password are
recreated. Workspace contexts use the new passwords with no extra step.

With --insecure-backends only admin is rotated. Passwords are never printed.

```
openarch secrets rotate [flags]
```

### Examples

```
  openarch secrets rotate
  openarch secrets rotate --only admin,mongodb-app
  openarch secrets rotate --dry-run --output json
```

### Options

```
      --dry-run            show what would be rotated, and change nothing
  -h, --help               help for rotate
      --only strings       rotate only these: mongodb-root, mongodb-app, opensearch, admin
      --timeout duration   how long to wait until the stack is healthy (default 10m0s)
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

* [openarch secrets](openarch_secrets.md)	 - Manage the passwords of a workspace

