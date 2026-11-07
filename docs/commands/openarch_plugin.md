## openarch plugin

List the plugins that extend openarch

### Synopsis

List the plugins that extend openarch.

A plugin is an executable named openarch-<name> on PATH. 'openarch <name> ...'
runs it with the arguments after the name. Built-in commands always win over
plugins with the same name.

Global flags given before the name reach the plugin as environment variables:
--output as OPENARCH_OUTPUT, --dir as OPENARCH_WORKSPACE, --context as
OPENARCH_CONTEXT, --non-interactive as OPENARCH_NON_INTERACTIVE=1, --verbose as
OPENARCH_VERBOSE=1 and --no-color as NO_COLOR=1. OPENARCH_BIN holds the path of
openarch, so a plugin can run the same binary. openarch reads the first three
variables too, so commands run by the plugin use the same workspace and context.
Passwords are never passed, and OPENARCH_PASSWORD is removed from the
environment of the plugin: a plugin that needs OpenArch runs openarch, which
finds the password in the workspace or the context. A setup with only
environment variables needs a workspace (--dir) or a context ('openarch config
set') for plugins.

### Examples

```
  openarch plugin list
  openarch docs --help
```

### Options

```
  -h, --help   help for plugin
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
* [openarch plugin list](openarch_plugin_list.md)	 - List the openarch-<name> executables on PATH

