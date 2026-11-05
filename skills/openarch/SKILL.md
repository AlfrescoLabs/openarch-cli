---
name: openarch
description: Install, run and check OpenArch (github.com/hyland/OpenArch) on this machine with the openarch CLI and Docker Compose. Use it when the user wants a local OpenArch, wants to start, stop, upgrade, back up or reset one, or wants to create, upload, list or query documents in it.
---

# OpenArch with the openarch CLI

openarch installs and operates OpenArch with Docker Compose. A workspace is a
folder with `openarch.yaml` (the source of truth) and the files made from it.
Pass `--dir <folder>` to every command, or run it inside the workspace.

## Rules

- Add `--output json --non-interactive` to every command. stdout then holds
  exactly one JSON document, also for errors. Logs go to stderr.
- Every fact you report must come from a command result. Never guess a URL, a
  port, a password or a document id.
- Never print or ask for a password in an argument. Passwords are in
  `<workspace>/.env.local`; the CLI reads them by itself. Tell the user where
  the admin password is (`OPENARCH_ADMIN_PASSWORD` in `.env.local`) instead of
  showing it.
- Keep the secure defaults of `init`. Add `--default-users`,
  `--insecure-backends` or `--bind-ip 0.0.0.0` only when the user asks for it.
- Ask the user before `down`, `clean`, `restore`, `secrets rotate`,
  `users delete` and `config delete`. `clean` and `restore` delete data.
- For the flags of a command, run `openarch <command> --help --output json`.
  `openarch help --output json` describes every command.

## Install flow

1. `openarch doctor --output json`. Each check has a `status` (`ok`, `warn`,
   `fail`, `skip`) and, when it is not ok, a `fix`. Exit code 3 means a check
   failed: show the `fix` to the user and stop. Warnings do not stop the flow.
2. `openarch init --dir <folder> --non-interactive --output json`. The result
   has `url`, `adminUser` and `passwordFile`. Add options only when the user
   asks, for example `--port 9080`, `--tls self-signed` or `--version 1.0.90`.
3. `openarch up --dir <folder> --output json`. The first start pulls about
   2 GB of images and can take several minutes. Exit code 4 means a service
   did not become healthy: run `status` and `logs`.
4. `openarch smoke-test --dir <folder> --output json`. Exit code 0 means
   OpenArch works end to end.
5. Tell the user the URL, the Swagger UI URL from `status`, the admin user and
   the file that holds the password.

## Exit codes

| Code | JSON `code` | Meaning | What to do |
| --- | --- | --- | --- |
| 0 | | OK | |
| 1 | `error` | Error | Read `message` and `hint` |
| 2 | `usage` | Bad usage or input | Fix the command; the `hint` says how |
| 3 | `prerequisite` | A prerequisite is missing | Show the `fix` or `hint` to the user |
| 4 | `unhealthy` | A service is not healthy | Run `status`, then `logs <service> --tail 100` |
| 5 | `remote` | Remote API error | Read `message`; check the id, path or query |
| 6 | `auth` | Authentication failed | Run `config whoami`; do not try other passwords |

An error looks like `{"error": {"code": "usage", "message": "...", "hint": "..."}}`.
Follow the `hint` before you try anything else.

## Common failures

- `openarch.yaml not found`: the folder is not a workspace. Use the right
  `--dir`, or run `init`.
- `already has openarch.yaml`: the workspace exists. To change it, edit
  `openarch.yaml` and run `openarch render`, then `openarch up`. Use
  `init --force` only when the user asks.
- `docker.daemon` fails: Docker is not running. Ask the user to start it.
- `port.<n>` fails: another program uses the port. Use `init --port <other>`,
  or ask the user to stop that program.
- `docker.memory` fails or warns: give Docker more memory, or use
  `init --memory 4g`.
- `linux.max_map_count` fails: OpenSearch needs
  `sudo sysctl -w vm.max_map_count=262144`. Ask the user to run it.
- `up` ends with exit code 4: run `status` to find the service that is not
  healthy, then `logs <service> --tail 100`.

## Daily commands

| Task | Command |
| --- | --- |
| State of each service | `openarch status` |
| Logs | `openarch logs [service] --tail 100` |
| Stop, keep the data | `openarch down` |
| Start again | `openarch up` |
| Change the OpenArch version | `openarch upgrade --version <tag>` |
| Back up, restore | `openarch backup`, `openarch restore <folder> --yes` |
| Delete all the data | `openarch clean --yes` |
| Check the API and the credentials | `openarch api ping` |

## Documents

Documents have an id, or a path that starts with `/`. The root is `/`.

- `openarch documents list /` lists a folder.
- `openarch documents create --parent / --type SysFolder --name projects` creates a folder.
- `openarch documents upload <file> --parent /projects` uploads a file.
- `openarch documents content /projects/<name> --out <file>` downloads it.
- `openarch query "SELECT * FROM SysFolder"` runs a query (HxQL).
  `SysContent` matches every document.

OpenArch does not extract text from files. Full-text queries find words in the
name, title and description only.
