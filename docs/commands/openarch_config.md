## openarch config

Manage saved connections (contexts)

### Synopsis

Manage saved connections, called contexts.

A context is a local workspace, or the URL and user of a remote OpenArch.
Contexts are in config.yaml in the CLI config folder (OPENARCH_CONFIG_DIR,
else $XDG_CONFIG_HOME/openarch, else ~/.config/openarch, or %AppData%\openarch
on Windows). That file never holds a password.

Passwords of remote contexts are in the OS credential store: Keychain on
macOS, Credential Manager on Windows, Secret Service on Linux. When it is
not available, they are in credentials.json (mode 0600) in the same folder,
with a warning. OPENARCH_CREDENTIAL_STORE=keychain|file forces a backend.

Commands that call the API use, in this order: --context or
OPENARCH_CONTEXT, then the workspace in --dir or the current folder, then the
current context.

```
openarch config [flags]
```

### Examples

```
  openarch config set --name prod -s https://openarch.example.com -u admin
  openarch config list
  openarch config use prod
  openarch config whoami --output json
```

### Options

```
  -h, --help   help for config
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
* [openarch config delete](openarch_config_delete.md)	 - Delete a context and its stored passwords
* [openarch config get](openarch_config_get.md)	 - Show a context
* [openarch config list](openarch_config_list.md)	 - List the contexts
* [openarch config set](openarch_config_set.md)	 - Save a remote connection and its password
* [openarch config use](openarch_config_use.md)	 - Select the current context
* [openarch config whoami](openarch_config_whoami.md)	 - Show which URL and user a command would use, and where they come from

