## openarch init

Create an OpenArch workspace

### Synopsis

Create an OpenArch workspace: openarch.yaml, compose.yaml, .env,
.env.local (secrets, mode 0600), config/users.yml, .gitignore and README.md.

openarch.yaml is the source of truth. To change the deployment later, edit it
and run 'openarch render'. Every option below is also a key in openarch.yaml.

The workspace is the folder given by --dir, then OPENARCH_WORKSPACE, then the
current folder. It is created if needed.

Security is on by default, and every part can be turned off:
  - OpenArch: a generated admin password replaces the users bundled in the
    image. --default-users keeps the bundled users (admin:password).
  - MongoDB and OpenSearch: generated passwords, given to the containers as
    Docker Compose secrets. --insecure-backends turns authentication off.
  - HTTPS: --tls self-signed or --tls custom. The default is plain HTTP.

The admin password is generated unless it is read from stdin with
--admin-password-stdin. All passwords are stored in .env.local and never printed.

In a terminal, init asks for every option that was not given as a flag, then
prints the equivalent non-interactive command. --non-interactive, --from or a
missing terminal turn the questions off. Set OPENARCH_ACCESSIBLE=1 for plain
prompts that work with screen readers.

```
openarch init [flags]
```

### Examples

```
  openarch init --dir my-openarch
  openarch init --storage bind --data-dir /srv/openarch
  openarch init --tls self-signed --server-name openarch.local
  openarch init --expose opensearch --with dashboards --memory 8g
  openarch init --insecure-backends --default-users     # no passwords, plain HTTP
  printf '%s' "$PASSWORD" | openarch init --admin-password-stdin --non-interactive
  openarch init --from openarch.yaml --dir copy
  openarch init --dry-run --output json
```

### Options

```
      --admin-password-stdin   read the admin password from stdin
      --bind-ip string         address the ports listen on: 127.0.0.1 (this machine only) or 0.0.0.0 (all interfaces) (default "127.0.0.1")
      --cert string            PEM certificate file for --tls custom
      --context-name string    name of the context that points to this workspace (default "local")
      --data-dir string        data folder for --storage bind (default ./data)
      --default-users          keep the users bundled in the OpenArch image (admin:password and others)
      --dry-run                list the files that would be written, and write nothing
      --expose strings         publish an extra port: mongodb, opensearch, debug (repeatable)
      --force                  overwrite an existing workspace (keeps the passwords in .env.local unless a new admin password is given)
      --from string            create the workspace from an existing openarch.yaml
  -h, --help                   help for init
      --insecure-backends      run MongoDB and OpenSearch with no authentication
      --key string             PEM private key file for --tls custom
      --memory string          memory for the stack: auto (from docker info) or a size such as 4g (default "auto")
      --port int               host port (default 8080, or 8443 with TLS)
      --server-name string     host name for URLs and the certificate (default "localhost")
      --storage string         where data is kept: none, volume, bind (default "volume")
      --tls string             HTTPS mode: none, self-signed, custom (default "none")
      --version string         OpenArch image tag (default "1.0.90")
      --with strings           add a service: dashboards (repeatable)
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

