## openarch users

Manage the OpenArch users of a workspace

### Synopsis

Manage the OpenArch users of a workspace.

OpenArch has no API to manage users. It reads them from config/users.yml when
it starts. These commands change that file (passwords as bcrypt hashes), and
'openarch up' then restarts OpenArch to apply the change.

The admin user belongs to the workspace: its password is in .env.local, and
'users set-password admin' changes both files. The passwords of other users are
stored nowhere else.

```
openarch users [flags]
```

### Options

```
  -h, --help   help for users
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
* [openarch users add](openarch_users_add.md)	 - Add a user
* [openarch users delete](openarch_users_delete.md)	 - Delete a user
* [openarch users list](openarch_users_list.md)	 - List the users
* [openarch users set-password](openarch_users_set-password.md)	 - Change the password of a user

