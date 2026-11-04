## openarch doctor

Check the prerequisites to run OpenArch

### Synopsis

Check the prerequisites to run OpenArch: Docker, Docker Compose,
Docker resources, free ports and access to the image registry.

Each check reports ok, warn, fail or skip, and a fix when something is wrong.
Exit code 3 means at least one check failed. Warnings do not change the exit code.

Checks: docker.cli, docker.daemon, docker.compose, docker.memory, docker.cpus, port.8080, network.ghcr
(linux.max_map_count runs only on Linux; one port.<n> check runs for each --port)

Inside a workspace, the default ports are the ones openarch.yaml publishes.

```
openarch doctor [flags]
```

### Examples

```
  openarch doctor
  openarch doctor --output json
  openarch doctor --check docker.daemon --check docker.memory
  openarch doctor --port 8080 --port 9200
```

### Options

```
      --check strings   run only this check (repeatable)
  -h, --help            help for doctor
      --port ints       host port that must be free (repeatable, default: the workspace ports, or 8080)
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

