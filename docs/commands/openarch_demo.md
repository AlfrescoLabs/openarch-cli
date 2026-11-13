## openarch demo

Add sample users and documents that show permission-aware search

### Synopsis

Add sample users and documents that show permission-aware search: the
same query returns different documents for each user.

1. 'demo users' adds the users alice (group demo-hr), bob (group
   demo-finance) and carol (no extra group) with generated passwords, and
   saves a context for each one.
2. 'openarch up' starts OpenArch, or restarts it: it reads its users only at
   startup.
3. 'demo load' creates folders and documents under /Demo with different read
   access. 'demo reset' deletes them.

```
openarch demo [flags]
```

### Examples

```
  openarch demo users
  openarch up
  openarch demo load
  openarch query "SELECT * FROM SysFile WHERE sys_path STARTSWITH '/Demo/'" --context local-alice
  openarch query "SELECT * FROM SysFile WHERE sys_path STARTSWITH '/Demo/'" --context local-bob
  openarch query "SELECT * FROM SysFile WHERE sys_path STARTSWITH '/Demo/'" --context local-carol
```

### Options

```
  -h, --help   help for demo
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
* [openarch demo load](openarch_demo_load.md)	 - Create the demo folders and documents
* [openarch demo reset](openarch_demo_reset.md)	 - Delete the demo folders and documents
* [openarch demo users](openarch_demo_users.md)	 - Add the demo users and a context for each one

