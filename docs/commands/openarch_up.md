## openarch up

Start OpenArch and wait until it is healthy

### Synopsis

Start the workspace stack with docker compose and wait until every service
is healthy. The first start pulls the images, which can take several minutes.

up runs the doctor checks first (skip them with --skip-doctor). Ports that
the running stack of this workspace already uses are not reported as busy.

When a config file or a password changed (after 'openarch render'), up
recreates only the services that read it.

Exit code 3 means a prerequisite failed. Exit code 4 means the stack did not
become healthy before --timeout; the status of each service is still printed.
up also stops with exit code 4 when a healthcheck fails with HTTP 401 for one
minute, because a wrong password never becomes healthy.

```
openarch up [flags]
```

### Examples

```
  openarch up
  openarch up --dir my-openarch --timeout 15m
  openarch up --wait=false
  openarch up --output json
```

### Options

```
      --dry-run            print the docker compose command, and run nothing
  -h, --help               help for up
      --skip-doctor        do not run the doctor checks first
      --timeout duration   how long to wait with --wait (default 10m0s)
      --wait               wait until every service is healthy (default true)
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

