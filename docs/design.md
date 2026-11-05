# Design

`openarch` is one Go binary that installs and operates OpenArch (the `openarch-app-jvm` image,
MongoDB and OpenSearch) and, later, the Content Lake App stack. Humans and AI agents use it in
the same way.

## Guiding rules

Every command follows these rules.

1. **Every prompt has a flag.** With no terminal, or with `--non-interactive`, a missing value is an error, never a prompt.
2. **Output contract.** Every command supports `--output text|json` (or `OPENARCH_OUTPUT`). Data goes to stdout. Logs and progress go to stderr. In JSON mode errors also go to stdout, so stdout always holds exactly one JSON document. JSON keys are stable.
3. **Exit codes.** `0` ok, `1` generic error, `2` bad usage or input, `3` prerequisite missing, `4` service not healthy, `5` remote API error, `6` authentication failed (missing, wrong or unreadable credentials). The JSON error codes are public API.
4. **Safe by default.** Commands that write workspace files or recreate services have `--dry-run` (`init`, `render`, `up`, `down`, `clean`, `upgrade`, `secrets rotate`). Commands that delete data (`clean`, `restore`, `users delete`, `config delete`) ask for confirmation, and need `--yes` when there is no terminal.
5. **Logic outside cobra.** Each command calls a function in `internal/` with a typed input and a typed result. Cobra, the JSON printer and a future MCP server are thin layers on top.
6. **Transparent workspace.** The CLI writes plain files. A user can read them and run `docker compose` by hand.
7. **No secrets in stdout** unless `--show-secrets` is set. No secrets in command arguments either: use `--password-stdin`, env vars or a hidden prompt.
8. **Every security feature is optional.** A user can install everything with plain HTTP and no generated passwords.
9. **Machine-readable help.** With `--output json`, `help` and `--help` print the command tree: flags with type, default and the fixed values from completion, examples and the exit codes. `docs/llms.txt` and `docs/commands` are built from the cobra tree, and tests fail when they are out of date. Every command has examples.
10. **MCP server.** `openarch mcp` (stdio only) exposes 26 commands as tools (`internal/cli/mcp.go`, `internal/mcpserver`). A tool runs its cobra command in-process with `--output json --non-interactive`, so tools and commands share all logic and exit codes. The input schema comes from the command flags; connection, password, `--yes` and `--follow` flags are never parameters. Positional arguments go after `--`. Destructive tools are registered only with `--allow-destructive`. The `init` options that can turn security off are parameters only with `--allow-insecure`, because models turned them on unasked. Calls of tools that change things run one at a time.
11. **Agent Skill.** `skills/openarch/SKILL.md` is embedded in the binary (package `skills`). `openarch skill install` writes it to `<dir>/.claude/skills/openarch/SKILL.md` and replaces a changed file only with `--force`; `openarch skill show` prints it. The skill is a short guide (rules, install flow, exit codes, common failures) and points to `--help --output json` for flags, so the details stay in one place.

## Technical choices

| Topic | Choice | Reason |
| --- | --- | --- |
| Language | Go 1.26+ | One static binary. Current `golang.org/x/term` needs 1.26. |
| Commands | `spf13/cobra` | Standard Go CLI library. |
| Interactive forms | `charmbracelet/huh` | Built on bubbletea. Has an accessible mode. |
| Styling | `charmbracelet/lipgloss` | Works with huh. |
| Config | `gopkg.in/yaml.v3` + flags + env vars | Simple. No need for viper. |
| Templates | `text/template` + `embed` | The templates ship inside the binary. |
| TLS proxy | Caddy | One small config for self-signed and custom certificates. |
| Docker | The `docker compose` CLI | Uses the user's real Docker setup. `--format json` gives parsed output. |
| REST client | `oapi-codegen` from the OpenArch OpenAPI spec | Admin commands stay in sync with the API. |
| Credential store | `zalando/go-keyring` + file fallback | Pure Go. The Docker credential helpers for macOS and Secret Service need cgo, which blocks cross-compiling. |
| CLI tests | `rogpeppe/go-internal/testscript` | Golden tests of real CLI runs. |
| Release | GoReleaser + cosign + SBOM | Cross-platform builds and signed artifacts. |
| Signature check | `sigstore-go` | `self-update` checks the keyless release signature inside the binary, with no cosign install. |
| Lint | `golangci-lint` | Standard. |

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

`openarch.yaml` has a `schemaVersion` (now 2). Every new or changed key needs a new version and a
migration in `workspace.Parse`. Parse reads the version first, so:

- An older file is migrated in memory. Commands work and print a warning, and `openarch migrate`
  (with `--dry-run` for a diff) rewrites the file. `render` and `upgrade` save it with the current
  version too. Only `openarch.yaml` changes.
- A newer file gives exit code 3 with the hint to run `openarch self-update`. Releases up to 0.3.1
  read the keys first, so they report an unknown key instead.
- Version 2 moved `insecureBackends` and `defaultUsers` into `security:` as `backendAuth` and
  `defaultUsers`. A missing key keeps its secure default.

## Deployment options

`init` offers every option as a flag, a wizard question and a key in `openarch.yaml`. The wizard
asks only for values that were not given as flags, and prints the equivalent non-interactive
command at the end.

| Option | Flag | Values | Default |
| --- | --- | --- | --- |
| OpenArch version | `--version` | image tag | latest tested tag |
| Storage | `--storage` | `none` (data is lost when containers are removed), `volume` (named volumes), `bind` (local folders) | `volume` |
| Data folder | `--data-dir` | path, used with `--storage bind` | `./data` |
| TLS | `--tls` | `none`, `self-signed` (certificate made by the CLI), `custom` (your own files) | `none` |
| Certificate files | `--cert`, `--key` | paths, used with `--tls custom` | - |
| Server name | `--server-name` | host name for URLs and the certificate | `localhost` |
| Port | `--port` | host port | 8080, or 8443 with TLS |
| Bind address | `--bind-ip` | `127.0.0.1` (this machine only) or `0.0.0.0` (all interfaces) | `127.0.0.1` |
| Extra ports | `--expose` | `mongodb`, `opensearch`, `debug` (Java debug 5005), repeatable | none |
| Extra services | `--with` | `dashboards` (OpenSearch Dashboards), repeatable | none |
| Admin password | `--admin-password-stdin` or prompt | - | generated |
| Resources | `--memory` | `auto` or a size such as `4g` | `auto` (from `docker info`) |
| Backend security | `--insecure-backends` | off: MongoDB and OpenSearch use generated passwords. On: no authentication | off |
| OpenArch users | `--default-users` | off: only `admin`, with a generated password. On: the users bundled in the image (`admin:password` and others) | off |

Design notes:

- With `--tls none --insecure-backends --default-users`, everything runs on plain HTTP with no generated passwords. OpenArch itself always needs HTTP Basic auth, except on its public endpoints, so `--default-users` is the closest to "no password" there.
- Storage `none` uses no volumes. `bind` creates the folders, and a one-shot `data-init` container (running as root) gives them to the container users: OpenSearch uid 1000, OpenArch uid 33010. MongoDB fixes its own folder. No `sudo` is needed.
- Storage covers the OpenArch files too (`/deployments/nxserver/data`), not only MongoDB and OpenSearch.
- Users: OpenArch reads its users from a YAML file at startup. The CLI mounts `config/users.yml` with the admin password as a bcrypt hash. The bundled users are not loaded unless `--default-users` is set. `openarch users` edits this file, and `up` restarts OpenArch.
- Secrets: every password is generated into `.env.local`. Compose reads them as secrets with an `environment:` source and copies them into the containers as files in `/run/secrets`. Config files hold only bcrypt hashes. In secure mode Compose needs `--env-file .env --env-file .env.local`.
- MongoDB: a root user and an `openarch` user with `readWrite` on its database, created or updated by `mongodb-init`. The replica set key file is created inside the container at each start.
- OpenSearch: the security plugin is on, with TLS between nodes from a certificate created inside the container at each start. HTTP on port 9200 is plain, and users are required. The users are `admin`, `openarch` and `kibanaserver`.
- TLS uses a Caddy reverse proxy in front of `openarch-app`. With TLS, `openarch-app` is internal and only the proxy port is published.
- The self-signed certificate is made per workspace with Go `crypto/x509` (SANs: the server name, `localhost`, `127.0.0.1`). No private key ships inside the binary.
- Differences from the upstream OpenArch compose file: storage is configurable (upstream uses tmpfs for MongoDB), `openarch-app` has a healthcheck, and only the main port is published. The MongoDB replica set (`rs0`) and its init job stay, because OpenArch needs them.
- Warnings on stderr: `--insecure-backends`, `--default-users`, exposed backend ports, and `--bind-ip 0.0.0.0` without TLS.
- `doctor` inside a workspace checks the ports from `openarch.yaml`, not the defaults.

## Authentication

### Model

- OpenArch uses HTTP Basic auth. The same model covers the other Content Lake systems (Alfresco, Nuxeo, RAG service).
- A **context** is a named connection. It has one endpoint per system: `openarch`, `alfresco`, `nuxeo`, `rag`.
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

- Backends: Keychain on macOS, Credential Manager on Windows, Secret Service over D-Bus on Linux, and a file `credentials.json` (mode 0600) next to the config file.
- Entries use service `openarch` and account `<user>@<url>`.
- When the OS store is not available (headless Linux, containers, CI), the CLI uses the file backend and prints a warning on stderr.
- `OPENARCH_CREDENTIAL_STORE=keychain|file` forces a backend.

### Resolution order

For each value, the first source that has it wins:

1. Flags: `--url`, `--user`, `--password-stdin`, `--insecure`. `--password` also works, with a warning, because the value ends up in shell history.
2. Env vars: `OPENARCH_URL`, `OPENARCH_USER`, `OPENARCH_PASSWORD`, `OPENARCH_INSECURE`. Agents and CI use these.
3. An explicit context: `--context <name>`, then `OPENARCH_CONTEXT`.
4. The workspace in `--dir` or in the current folder.
5. `current-context`. It comes after the workspace, so a command inside one workspace never uses the connection of another by accident.
6. If there is still no password and a terminal is present, a hidden prompt. Without a terminal, exit code 6 with a hint.

`openarch config whoami` shows which source won for each value. It never shows the password.

### Rules

- `config set` checks the connection with `api ping` before saving. `--no-verify` skips the check. A wrong password is not saved.
- `config get` masks passwords unless `--show-secrets` is set.
- `config delete` removes the context and its passwords from the store.
- Credential functions return errors. They never call `os.Exit`.
- Passwords never appear in logs, `--verbose` output or error messages. The HTTP client redacts the `Authorization` header.
- `--insecure` prints a warning every time it is used.
- A 401 or 403 gives exit code 6. The hint names the context and endpoint that were used.

## Self-update

`openarch self-update` replaces the running binary with a GitHub release:

1. The latest tag comes from the redirect of `/releases/latest`, so no GitHub API token is needed.
2. `checksums.txt.sigstore.json` must be a valid Sigstore bundle for `checksums.txt`. The certificate identity must be the release workflow of this repository at that exact tag, issued by GitHub Actions. The bundle needs a certificate timestamp, a transparency log entry and a timestamp. The Sigstore trusted root comes through TUF and is cached in the user cache folder.
3. Only then the archive is downloaded, and its SHA-256 must match `checksums.txt`.
4. The new binary is written next to the old one and renamed over it. On Windows the running binary is first renamed to `openarch.exe.old`, which the next run deletes.

Builds from source and installs by Homebrew or Scoop are not replaced unless `--force` is set.

## Content Lake integration

The CLI wraps the upstream repository
[content-lake-app-deployment](https://github.com/AlfrescoLabs/content-lake-app-deployment).
It does not embed its own copy of the Content Lake templates.

How it works:

- `openarch init --with content-lake` downloads a pinned ref of the upstream repository into the workspace.
- The CLI drives the upstream `compose.yaml` with Compose profiles and a generated `.env.local`.
- The pinned ref is stored in `openarch.yaml`. A newer ref is a CLI update, or an explicit `--content-lake-ref`.

Reasons:

- Content Lake is a proof of concept and changes often. Copied templates would drift from upstream.
- The upstream stack uses many Compose files, profiles and remote BuildKit contexts. Wrapping it keeps that logic in one place.
- The upstream Makefile has no visible license, so the CLI does not copy it. The CLI runs `docker compose` directly instead.

Costs:

- The CLI depends on the layout of the upstream repository at the pinned ref. Each ref bump needs a test run.
- `init --with content-lake` needs network access to download the ref.

## Alfresco for Content Lake

The CLI supports two Alfresco setups, in this order:

1. The Alfresco bundled with the Content Lake stack. This comes first.
2. An existing Alfresco installation. The CLI then installs the `content-lake-repo-model` module into it and connects the stack to it. This comes after the bundled setup works.

## Non-goals

- Kubernetes and Helm. OpenArch is deployed with Docker Compose only.
