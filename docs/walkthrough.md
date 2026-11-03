# Walkthrough

This walkthrough uses every feature of `openarch` in a temporary folder. Run the blocks in order, in a
terminal. The first `up` pulls about 2 GB of images, so it takes a few minutes. Run one stack at a time
to save memory.

## 0. Build the CLI and make a test folder

From the repository folder:

```
make build
export PATH="$PWD/bin:$PATH"
openarch version

BASE=$(mktemp -d /tmp/openarch-demo.XXXX)
# Keep saved connections away from your real CLI config:
export OPENARCH_CONFIG_DIR="$BASE/cli-config"
# Optional: keep passwords in a file instead of the OS credential store:
# export OPENARCH_CREDENTIAL_STORE=file
echo "$BASE"
```

## 1. Prerequisites

```
openarch doctor
openarch doctor --output json
```

## 2. Create a secure workspace

The workspace uses an older OpenArch version, so that step 10 can upgrade it.

```
openarch init --dir "$BASE/main" --version 1.0.55 --with dashboards --non-interactive
cd "$BASE/main"
ls -la . config config/opensearch
cat openarch.yaml
openarch render --dry-run
```

## 3. Start, check and read the logs

```
openarch up
openarch status
openarch status --output json
openarch logs openarch-app --tail 20
openarch doctor          # the ports of this stack are not reported as busy
```

The Swagger UI is at http://localhost:8080/swagger-ui/index.html. The admin password is
`OPENARCH_ADMIN_PASSWORD` in `.env.local`.

## 4. API, documents and queries

```
openarch api ping
openarch api ping --verbose            # the Authorization header is shown as REDACTED
openarch documents get /
openarch documents create --parent / --type SysFolder --name projects --title Projects
openarch documents create --parent /projects --type SysFolder --name reports
openarch documents get /projects/reports --output json
openarch query "SELECT * FROM SysFolder" --sync
OPENARCH_PASSWORD=wrong openarch api ping; echo "exit code: $?"   # 6
```

## 5. Smoke test

```
openarch smoke-test
openarch smoke-test --output json
```

## 6. Users

```
printf 'Alice-pass-1' | openarch users add alice --email alice@example.com --password-stdin
openarch users list
openarch up                            # recreates only openarch-app
printf 'Alice-pass-1' | openarch api ping --user alice --password-stdin
openarch users delete alice --yes
openarch up
```

## 7. Saved connections (contexts)

```
openarch config list                   # init saved this workspace as "local"
(cd /tmp && openarch api ping --context local)
grep '^OPENARCH_ADMIN_PASSWORD=' .env.local | cut -d= -f2- |
  openarch config set --name remote -s http://127.0.0.1:8080 -u admin --password-stdin
openarch config get --name remote      # the password is masked
openarch config whoami
openarch config whoami --context remote
openarch config delete remote --yes    # also removes the stored password
```

`config set` checks the connection before it saves it. The password goes to the OS credential store,
or to `credentials.json` with `OPENARCH_CREDENTIAL_STORE=file`.

## 8. Backup, clean and restore

```
openarch backup
B=$(ls -d backups/*/ | tail -1); echo "$B"
openarch clean --yes
openarch up
openarch documents get /projects; echo "exit code: $?"   # 5: not found, the data is gone
openarch restore "$B" --yes
openarch documents get /projects/reports                 # back again
```

## 9. Password rotation

```
openarch secrets rotate --dry-run
openarch secrets rotate
openarch api ping                      # the workspace uses the new password
openarch smoke-test
```

## 10. Upgrade

```
openarch upgrade --version latest --dry-run
openarch upgrade --version latest      # pull, render, start, smoke test
openarch documents get /projects       # the data is kept
openarch down                          # stop, and keep the data
```

## 11. HTTPS with a self-signed certificate

```
openarch init --dir "$BASE/tls" --tls self-signed --server-name openarch.local --non-interactive
cd "$BASE/tls"
openarch up
openarch api ping                      # works although openarch.local does not resolve
curl -s --cacert certs/cert.pem --resolve openarch.local:8443:127.0.0.1 https://openarch.local:8443/actuator/health
openarch down
```

## 12. Every security feature off

Plain HTTP, no passwords for MongoDB and OpenSearch, and the users bundled in the OpenArch image.

```
openarch init --dir "$BASE/open" --insecure-backends --default-users --expose opensearch --non-interactive
cd "$BASE/open"
docker compose up -d --wait            # plain Docker Compose works too
curl -s -u admin:password "http://127.0.0.1:8080/api/user?name=admin"
curl -s http://127.0.0.1:9200/_cat/indices
openarch status
openarch down
```

## 13. The wizard

Optional, in a terminal. It asks for every option, then prints the same command with flags.

```
openarch init --dir "$BASE/wizard"
```

## 14. Clean up

```
for w in main tls open; do (cd "$BASE/$w" && openarch clean --yes); done
rm -rf "$BASE"
unset OPENARCH_CONFIG_DIR OPENARCH_CREDENTIAL_STORE
```

## Notes

- Add `--dir <workspace>` to run a command from another folder.
- Add `--output json` to any command to get one JSON document on stdout.
