# Design

`openarch` is one Go binary that installs and operates OpenArch (the `openarch-app-jvm` image,
MongoDB and OpenSearch). Humans and AI agents use the same commands, output and exit codes.
App for OpenArch is a separate plugin (see [App for OpenArch](#app-for-openarch)).

## Problem and goals

The upstream OpenArch repository ships one Compose file for development. It keeps MongoDB on tmpfs,
uses default passwords, has no TLS, and has no commands for backup, upgrade or users. An AI agent
also needs stable output and exit codes to drive it.

Goals:

- One command creates a workspace, and one more starts a healthy stack with generated passwords.
- Every operation (start, stop, upgrade, backup, restore, users, secrets) is a command.
- A stable output contract for scripts and agents, plus an MCP server and an Agent Skill.
- The workspace stays plain files that run without the CLI.

## Guiding rules

Every command follows these rules.

1. **Every prompt has a flag.** With no terminal, or with `--non-interactive`, the CLI never prompts. A missing value with a default uses the default. A missing value with no default is exit code 2.
2. **Output contract.** Every command supports `--output text|json` (or `OPENARCH_OUTPUT`). Data goes to stdout. Logs and progress go to stderr. In JSON mode errors also go to stdout, so stdout always holds exactly one JSON document. Keys that the CLI makes are stable: they are not removed or renamed except in a major release. Server responses that commands pass through (documents, queries) follow the OpenArch API and are not covered.
3. **Exit codes.** `0` ok, `1` generic error, `2` bad usage or input, `3` prerequisite missing, `4` service not healthy, `5` remote API error, `6` authentication failed (missing, wrong or unreadable credentials). The JSON error codes are public API.
4. **Safe by default.** Commands that write workspace files or recreate services have `--dry-run` (`init`, `render`, `up`, `down`, `clean`, `upgrade`, `migrate`, `secrets rotate`). Commands that delete data (`clean`, `restore`, `users delete`, `config delete`) ask for confirmation, and need `--yes` when there is no terminal.
5. **Logic outside cobra.** Each command calls a function in `internal/` with a typed input and a typed result. Cobra and the JSON printer are thin layers on top. The MCP server runs the cobra commands, so it is a layer on top of them.
6. **Transparent workspace.** The CLI writes plain files. A user can read them and run `docker compose` by hand.
7. **No secrets in stdout** unless `--show-secrets` is set. Secrets go through `--password-stdin`, env vars or a hidden prompt. `--password` is accepted only with a warning.
8. **Every security feature is optional.** A user can install everything with plain HTTP and no generated passwords.
9. **Machine-readable help.** With `--output json`, `help` and `--help` print the command tree: flags with type, default and the fixed values from completion, examples and the exit codes. `docs/llms.txt` and `docs/commands` are built from the cobra tree, and tests fail when they are out of date. Every command has examples.
10. **MCP server.** `openarch mcp` exposes commands as tools, on stdio or, with `--listen`, on Streamable HTTP. HTTP needs a bearer token from the credential store by default; `--no-auth` on a non-loopback address also needs `--allow-remote-no-auth`. TLS has the modes of `init`. The tool table in `internal/cli/mcp.go` is the list.
    - Each tool has a kind: read-only, write or destructive. Destructive tools (`down`, `clean`, `restore`, `secrets rotate`, `users delete`, `config delete`) are registered only with `--allow-destructive`. The raw `api` command is not a tool.
    - A call runs its cobra command in-process with `--output json --non-interactive`, so tools and commands share all logic and exit codes. Each call builds a new App and command tree. Write and destructive calls run one at a time.
    - The input schema comes from the command flags. Connection, password, `--yes` and `--follow` flags are never parameters. Positional arguments go after `--`.
    - The `init` options that can turn security off (`default-users`, `insecure-backends`, `bind-ip`, `expose`, `tls`, `cert`, `key`, `from`) are parameters only with `--allow-insecure`, so a model cannot turn security off without the operator.
    - Tools run as the user of the context, which is `admin` for a workspace context.
11. **Agent Skill.** `skills/openarch/SKILL.md` is embedded in the binary (package `skills`). `openarch skill install` writes it to `<dir>/.claude/skills/openarch/SKILL.md` and replaces a changed file only with `--force`; `openarch skill show` prints it. The skill is a short guide (rules, install flow, exit codes, common failures) and points to `--help --output json` for flags, so the details stay in one place.
12. **Plugins.** `openarch <name> ...` runs the executable `openarch-<name>` from `PATH` with the arguments after the name, as git and kubectl do. Built-in commands always win. `openarch plugin list` shows the plugins on `PATH`.
    - A plugin is trusted code. It runs with the rights of the user, and it can run any `openarch` command, `config get --show-secrets` too.
    - The plugin gets the environment of `openarch`. Global flags before the name are added as environment variables (`OPENARCH_OUTPUT`, `OPENARCH_WORKSPACE`, `OPENARCH_CONTEXT`, `OPENARCH_NON_INTERACTIVE`, `OPENARCH_VERBOSE`, `NO_COLOR`), and `OPENARCH_BIN` is the path of `openarch`. `openarch` adds no passwords of its own, and removes `OPENARCH_PASSWORD`.
    - A plugin that needs OpenArch runs `openarch` (`api`, `documents`, `query`), so it needs no copy of the credentials. Those commands find the password in the workspace or the context, so a setup with only environment variables needs one of them for plugins.
    - The exit code of the plugin is the exit code of `openarch`. Plugins follow the output contract only by convention.

## Technical choices

| Topic | Choice | Reason |
| --- | --- | --- |
| Language | Go 1.26+ | One static binary. `golang.org/x/term` v0.47 needs Go 1.26. Builds use the `toolchain` line in `go.mod`, which is kept at the latest patched Go. |
| Commands | `spf13/cobra` | Subcommands, shell completion and help out of the box. |
| Interactive forms | `charmbracelet/huh` | Built on bubbletea. Has an accessible mode. |
| Config | `gopkg.in/yaml.v3` + flags + env vars | Config comes from one file plus flags and env vars, so viper adds dependencies with no gain. |
| Templates | `text/template` + `embed` | The templates ship inside the binary. |
| TLS proxy | Caddy | One small config for self-signed and custom certificates. |
| Docker | The `docker compose` CLI | Uses the user's real Docker setup. `--format json` gives output the CLI can parse. |
| REST client | `oapi-codegen` from `api/openapi.json` | The client is generated from the stored OpenArch spec. CI fails when the generated code is out of date. |
| Credential store | `zalando/go-keyring` + file fallback | Pure Go. The Docker credential helpers for macOS and Secret Service need cgo, which blocks cross-compiling. |
| MCP | `modelcontextprotocol/go-sdk` | The official Go SDK. |
| CLI tests | `rogpeppe/go-internal/testscript` | Golden tests of real CLI runs. |
| Release | GoReleaser + cosign + SBOM | Cross-platform builds and signed artifacts. |
| Signature check | `sigstore-go` | `self-update` checks the keyless release signature inside the binary, with no cosign install. |
| Lint | `golangci-lint` | One runner for many Go linters. |

## Alternatives considered

| Decision | Rejected option | Trade-off |
| --- | --- | --- |
| Run Compose through the `docker compose` CLI | The Compose Go SDK | The SDK is a large dependency and can differ from the Compose version the user has. The CLI matches what the user runs by hand. |
| MCP tools run cobra commands in-process | A separate MCP server with its own code | Two code paths would drift. In-process tools share flags, validation and exit codes with the CLI. |
| Users from a YAML file | Users in MongoDB through an API | OpenArch has no user API. The YAML file is what OpenArch reads at startup. |
| Caddy as the TLS proxy | nginx or Traefik | Both need more config for the same two TLS modes. |
| App for OpenArch as a plugin | Built into the CLI (`init --with app-for-openarch`) | Built in, it would add schema keys, context endpoints and lifecycle cases to the core for a proof of concept. |

## Workspace

```
my-openarch/
  openarch.yaml        # CLI config: the source of truth
  compose.yaml         # rendered from openarch.yaml
  .env                 # non-secret values
  .env.local           # generated secrets, mode 0600, git-ignored
  config/              # users.yml, Caddyfile, OpenSearch security files
  .gitignore
  README.md            # endpoints and how to run without the CLI
  backups/
```

The CLI finds the workspace with `--dir`, then `OPENARCH_WORKSPACE`, then the current folder.

`openarch.yaml` is the source of truth. `openarch render` writes the other files from it, so an
agent or a user can edit `openarch.yaml` and run `render` with no flags. `render --dry-run` shows
a diff. `init --from <file>` creates a workspace from an existing `openarch.yaml`.

`openarch.yaml` has a `schemaVersion`. The current version is 2. Every new or changed key needs a
new version and a migration in `workspace.Parse`. Parse reads the version first, so:

- An older file is migrated in memory. Commands work and print a warning, and `openarch migrate`
  (with `--dry-run` for a diff) rewrites the file. `render` and `upgrade` save it with the current
  version too. Only `openarch.yaml` changes.
- A newer file gives exit code 3 with the hint to run `openarch self-update`.
- From version 1 to 2: `insecureBackends: true` becomes `security.backendAuth: false`, and
  `defaultUsers` becomes `security.defaultUsers`. A missing key keeps its secure default.

## Deployment options

`init` offers every option as a flag and a wizard question. Every option except the admin password
is also a key in `openarch.yaml`. The wizard asks only for values that were not given as flags, and
prints the equivalent non-interactive command at the end.

| Option | Flag | Key | Values | Default |
| --- | --- | --- | --- | --- |
| OpenArch version | `--version` | `version` | image tag | latest tested tag |
| Storage | `--storage` | `storage.mode` | `none` (data is lost when containers are removed), `volume` (named volumes), `bind` (local folders) | `volume` |
| Data folder | `--data-dir` | `storage.dataDir` | path, used with `--storage bind` | `./data` |
| TLS | `--tls` | `tls.mode` | `none`, `self-signed` (certificate made by the CLI), `custom` (your own files) | `none` |
| Certificate files | `--cert`, `--key` | `tls.cert`, `tls.key` | paths, used with `--tls custom` | - |
| Server name | `--server-name` | `serverName` | host name for URLs and the certificate | `localhost` |
| Port | `--port` | `port` | host port | 8080, or 8443 with TLS |
| Bind address | `--bind-ip` | `bindIP` | `127.0.0.1` (this machine only) or `0.0.0.0` (all interfaces) | `127.0.0.1` |
| Extra ports | `--expose` | `expose` | `mongodb`, `opensearch`, `debug` (Java debug 5005), repeatable | none |
| Extra services | `--with` | `with` | `dashboards` (OpenSearch Dashboards), repeatable | none |
| Admin password | `--admin-password-stdin` or prompt | - | - | generated |
| Resources | `--memory` | `memory` | `auto` or a size such as `4g` | `auto` (from `docker info`) |
| Backend security | `--insecure-backends` | `security.backendAuth` (inverted: the flag sets it to `false`) | off: MongoDB and OpenSearch use generated passwords. On: no authentication | off |
| OpenArch users | `--default-users` | `security.defaultUsers` | off: only `admin`, with a generated password. On: the users bundled in the image (`admin:password` and others) | off |

`init --version` selects the OpenArch image tag. The version of the CLI comes from `openarch version`.

Design notes:

- With `--tls none --insecure-backends --default-users`, everything runs on plain HTTP with no generated passwords. OpenArch itself always needs HTTP Basic auth, except on its public endpoints, so `--default-users` is the closest to "no password" there.
- Storage `none` uses no volumes. `bind` creates the folders, and a one-shot `data-init` container (running as root) gives them to the container users: OpenSearch uid 1000, OpenArch uid 33010. MongoDB fixes its own folder. No `sudo` is needed.
- Storage covers the OpenArch files too (`/deployments/nxserver/data`), not only MongoDB and OpenSearch.
- Users: OpenArch reads its users from a YAML file at startup. The CLI mounts `config/users.yml` with the admin password as a bcrypt hash. The bundled users are not loaded unless `--default-users` is set. `openarch users` edits this file, and `up` restarts OpenArch.
- Secrets: every password is generated into `.env.local`. Compose reads them as secrets with an `environment:` source and copies them into the containers as files in `/run/secrets`. Config files hold only bcrypt hashes. When the backends use passwords (no `--insecure-backends`), Compose needs `--env-file .env --env-file .env.local`.
- MongoDB: a root user and an `openarch` user with `readWrite` on its database, created or updated by `mongodb-init`. The replica set key file is created inside the container at each start.
- OpenSearch: the security plugin is on, with TLS between nodes from a certificate created inside the container at each start. HTTP on port 9200 is plain, and users are required. The users are `admin`, `openarch` and `kibanaserver`.
- TLS uses a Caddy reverse proxy in front of `openarch-app`. With TLS, `openarch-app` is internal and only the proxy port is published.
- The self-signed certificate is made per workspace with Go `crypto/x509` (SANs: the server name, `localhost`, `127.0.0.1`). No private key ships inside the binary. The CLI trusts the workspace certificate file itself, for both `self-signed` and `custom`, and connects to the published port on this machine.
- Differences from the upstream OpenArch compose file: storage is configurable (upstream uses tmpfs for MongoDB), `openarch-app` has a healthcheck, and only the main port is published. The MongoDB replica set (`rs0`) and its init job stay, because OpenArch needs them.
- Warnings on stderr: `--insecure-backends`, `--default-users`, exposed backend ports, and `--bind-ip 0.0.0.0` without TLS.
- `doctor` inside a workspace checks the ports from `openarch.yaml`, not the defaults.

## Authentication

### Model

- OpenArch uses HTTP Basic auth.
- A **context** is a named connection. It has one endpoint: `openarch`.
- There are two kinds of context:
  - **Workspace context.** It points to a local workspace. URLs come from `openarch.yaml` and credentials from `.env.local`. Docker Compose needs the passwords in `.env.local` anyway, so they are not copied to the OS store. `init` creates this context (default name `local`, or `--context-name`).
  - **Remote context.** URLs are in the config file and passwords in the credential store.

### Config file

`~/.config/openarch/config.yaml` (follows `XDG_CONFIG_HOME`; `%AppData%\openarch` on Windows;
`OPENARCH_CONFIG_DIR` overrides it). It never holds passwords.

```yaml
current-context: local
contexts:
  local:
    workspace: /Users/me/my-openarch
  prod:
    endpoints:
      openarch:
        url: https://openarch.example.com
        username: admin
        insecure: false
```

### Credential store

- Backends: Keychain on macOS, Credential Manager on Windows, Secret Service over D-Bus on Linux, and a file `credentials.json` next to the config file. The file holds the passwords as plain text, with mode 0600 (on Windows the mode has little effect).
- Entries use service `openarch` and account `<user>@<url>`.
- When the OS store is not available (headless Linux, containers, CI), the CLI uses the file backend and prints a warning on stderr.
- `OPENARCH_CREDENTIAL_STORE=keychain|file` forces a backend. `keychain` selects the OS store on every platform and never falls back to the file, so a missing OS store is an error.

### Resolution order

For each value, the first source that has it wins:

1. Flags: `--url`, `--user`, `--password-stdin`, `--insecure`. `--password` also works, with a warning, because the value ends up in shell history.
2. Env vars: `OPENARCH_URL`, `OPENARCH_USER`, `OPENARCH_PASSWORD`, `OPENARCH_INSECURE`. Agents and CI use these.
3. An explicit context: `--context <name>`, then `OPENARCH_CONTEXT`.
4. The workspace in `--dir`, `OPENARCH_WORKSPACE` or the current folder.
5. `current-context`. It comes after the workspace, so a command inside one workspace never uses the connection of another by accident.
6. If there is still no password and a terminal is present, a hidden prompt. Without a terminal, exit code 6 with a hint.

The user and password of a context or a workspace are used only with its own URL. When the URL
comes from a flag or an env var, stored credentials are not sent to it.

`openarch config whoami` shows which source won for each value. It never shows the password.

### Rules

- `config set` checks the connection with `api ping` before saving. `--no-verify` skips the check. A wrong password is not saved.
- `config get` masks passwords unless `--show-secrets` is set.
- `config delete` removes the context and its passwords from the store.
- Credential functions return errors. They never call `os.Exit`.
- Passwords never appear in logs, `--verbose` output or error messages. The HTTP client redacts the `Authorization` header.
- `--insecure` prints a warning every time it is used.
- A 401 or 403 gives exit code 6. For a 401 the hint names the context and endpoint that were used; a 403 says that the user has no permission.

## Self-update

`openarch self-update` replaces the running binary with a GitHub release:

1. The latest tag comes from the redirect of `/releases/latest`, so no GitHub API token is needed.
2. `checksums.txt.sigstore.json` must be a valid Sigstore bundle for `checksums.txt`. The certificate identity must be the release workflow of this repository at that exact tag, issued by GitHub Actions. The bundle needs a signed certificate timestamp, a transparency log entry and a signed or integrated timestamp. The Sigstore trusted root comes through TUF and is cached in the user cache folder.
3. Only then the archive is downloaded, and its SHA-256 must match `checksums.txt`.
4. The new binary is written next to the old one and renamed over it. On Windows the running binary is first renamed to `openarch.exe.old`, which the next run deletes.

A binary in a Homebrew or Scoop path, or a build from source (its version is not a release tag), is not replaced
unless `--force` is set.

## Limits

- One host. Docker Compose only, no high availability, no multi-node setup.
- `upgrade` has no automatic rollback. Make a backup first.
- `backup` runs while OpenArch runs. Changes made during the backup may be missing from it.
- Plugins are trusted code with the rights of the user.
- MCP tools return document content and server data to the model. That text is not trusted input.
- The file credential backend stores passwords and the MCP bearer token as plain text.
- The MCP bearer token is one shared secret for every client, with no expiry. OAuth for the HTTP server is not built.

The steps of `upgrade`, `backup`, `restore` and `clean` are in the README.

## Non-goals

- Kubernetes and Helm. OpenArch is deployed with Docker Compose only.
- High availability and multi-node deployments.
- Text extraction and search inside files. OpenArch indexes metadata only.

## App for OpenArch

App for OpenArch is not part of the core CLI. It is the `openarch-app` plugin (`openarch app ...`), which runs through the plugin hook (rule 12) and is in development in its own repository. The plugin wraps the upstream repository [content-lake-app-deployment](https://github.com/AlfrescoLabs/content-lake-app-deployment) at a pinned commit, and it uses the Hyland OpenArch that the upstream stack bundles.
