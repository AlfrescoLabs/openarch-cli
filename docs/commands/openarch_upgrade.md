## openarch upgrade

Change the OpenArch version and check the result

### Synopsis

Change the OpenArch version of the workspace. upgrade pulls the new image,
writes the version into openarch.yaml, renders the workspace, starts the stack,
waits until it is healthy, and runs the smoke test.

The pull comes first: when the version does not exist, nothing changes.
There is no automatic rollback. To go back, run upgrade with the old version,
or restore a backup made before the upgrade ('openarch backup').

Exit code 4 means the stack did not become healthy, or the smoke test failed.

```
openarch upgrade [flags]
```

### Examples

```
  openarch upgrade --version 1.0.90
  openarch upgrade --version latest --dry-run
  openarch upgrade --version 1.0.90 --output json
```

### Options

```
      --dry-run            show the plan, and change nothing
  -h, --help               help for upgrade
      --skip-doctor        do not run the doctor checks first
      --skip-smoke-test    do not run the smoke test after the upgrade
      --timeout duration   how long to wait until the stack is healthy (default 10m0s)
      --version string     OpenArch image tag to run
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

