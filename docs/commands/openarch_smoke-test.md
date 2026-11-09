## openarch smoke-test

Check OpenArch end to end: create, read, query and delete a document

### Synopsis

Check OpenArch end to end. The test creates a folder under the root, reads it
by id and by path, finds it with a synchronized query and in the background
search index, deletes it, and checks that it is gone. Each step is reported.

The folder is deleted even when a step fails.

Exit code 0 means every step passed. Exit code 4 means a step failed, and 6
means the credentials were rejected.

```
openarch smoke-test [flags]
```

### Examples

```
  openarch smoke-test
  openarch smoke-test --context prod --output json
```

### Options

```
  -h, --help                     help for smoke-test
      --index-timeout duration   how long to wait for the background search index (default 1m0s)
      --insecure                 do not verify the server certificate (env OPENARCH_INSECURE)
      --password string          password (insecure: it stays in the shell history; prefer --password-stdin or env OPENARCH_PASSWORD)
      --password-stdin           read the password from stdin
      --url string               OpenArch URL (env OPENARCH_URL; default: the workspace URL)
      --user string              user name (env OPENARCH_USER; default: the workspace admin)
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

