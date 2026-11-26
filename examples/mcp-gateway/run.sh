#!/usr/bin/env bash
# Starts a new workspace with the permission demo, then runs the image of
# `openarch mcp` through the Docker MCP Gateway, once as alice and once as bob,
# and asks both the same semantic query. The script checks the results itself.
# It adds its own server entry, profile and secret, with names that end in the
# process id, and always deletes them and the stack at the end.
#
# Usage: ./run.sh
# Environment: IMAGE (default ghcr.io/alfrescolabs/openarch:latest), OPENARCH
# (the CLI binary), PORT (the OpenArch port of the new workspace, default 18089).
set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)
image=${IMAGE:-ghcr.io/alfrescolabs/openarch:latest}
port=${PORT:-18089}
openarch=$(command -v "${OPENARCH:-openarch}") || { echo "openarch not found; set OPENARCH to the binary" >&2; exit 3; }

docker mcp version >/dev/null 2>&1 || { echo "Docker MCP Toolkit not found: enable it in Docker Desktop" >&2; exit 3; }
docker model status >/dev/null 2>&1 || { echo "Docker Model Runner is not running: enable it in Docker Desktop" >&2; exit 3; }
docker model inspect ai/mxbai-embed-large >/dev/null 2>&1 || { echo "model not found; pull it with: docker model pull ai/mxbai-embed-large" >&2; exit 3; }
command -v jq >/dev/null 2>&1 || { echo "jq not found: it reads the demo passwords" >&2; exit 3; }

tmp=$(mktemp -d)
# Compose names the project after the folder, so the name must be unique.
name=openarch-gateway-$$
ws=$tmp/$name
# The gateway keeps secrets for every profile, so the test server has its own name.
server=openarch-example-$$
profile=openarch_example_$$
catalogs=$HOME/.docker/mcp/catalogs
entry=$catalogs/$server.yaml
# The demo never touches the saved contexts or the OS credential store.
export OPENARCH_CONFIG_DIR=$tmp/config OPENARCH_CREDENTIAL_STORE=file
export PATH="$(dirname "$openarch"):$PATH"

cleanup() {
	docker mcp profile remove "$profile" >/dev/null 2>&1 || true
	docker mcp secret rm "$server.password" >/dev/null 2>&1 || true
	rm -f "$entry"
	if [ -f "$ws/openarch.yaml" ]; then
		echo "== clean"
		openarch clean --dir "$ws" --yes >/dev/null 2>&1 || echo "clean failed; run: openarch clean --dir $ws --yes" >&2
	fi
}
trap cleanup EXIT

echo "== demo workspace on port $port"
openarch init --dir "$ws" --context-name "$name" --port "$port" --non-interactive >/dev/null
openarch demo users --dir "$ws" >/dev/null
openarch up --dir "$ws" >/dev/null
openarch demo load --dir "$ws" --embeddings >/dev/null
# The vectors become searchable about a second after the load ends.
sleep 2

echo "== server entry $server with the image $image"
mkdir -p "$catalogs"
sed -E -e "s|^name: openarch$|name: $server|" \
	-e "s|^  - name: openarch$|  - name: $server|" \
	-e "s/openarch\.(url|user|password)/$server.\1/g" \
	-e "s|^image: .*|image: $image|" \
	"$here/openarch.yaml" >"$entry"
docker mcp profile create --name "$profile" --id "$profile" --server "file://$entry" >/dev/null

for user in alice bob; do
	echo
	echo "== gateway as $user"
	password=$(openarch credentials --dir "$ws" --output json 2>/dev/null | jq -r --arg u "$user" '.users[] | select(.username == $u) | .password')
	[ -n "$password" ] || { echo "no password for $user" >&2; exit 1; }
	# secret set keeps an old value, and a newline from stdin: remove it first, then send no newline.
	docker mcp secret rm "$server.password" >/dev/null 2>&1 || true
	printf '%s' "$password" | docker mcp secret set "$server.password" >/dev/null
	docker mcp profile config "$profile" --set "$server.url=http://host.docker.internal:$port" --set "$server.user=$user" >/dev/null
	docker mcp tools call --gateway-arg="--profile=$profile" config_whoami | grep '^  "user": "'
	docker mcp tools call --gateway-arg="--profile=$profile" query "query=SELECT * FROM SysContent" "semantic=salary levels" >"$tmp/$user.json"
	# The text of each hit starts with the document title.
	sed -n '/"hits"/,$p' "$tmp/$user.json" | grep -o '"text": "[^.]*' | sed 's/"text": "/  hit: /' || echo "  no hits"
done

# Do not trust the gateway setup: check what each user got.
echo
echo "== check"
grep -q "Salary bands 2026" "$tmp/alice.json" || { echo "FAIL: alice did not find Salary bands 2026" >&2; exit 1; }
if grep -q -e "Salary bands 2026" -e "oafx-03-" "$tmp/bob.json"; then
	echo "FAIL: bob found Salary bands 2026, which bob may not read" >&2
	exit 1
fi
grep -q '"hits"' "$tmp/bob.json" || { echo "FAIL: the query of bob returned no hits list" >&2; exit 1; }
echo "alice found Salary bands 2026, bob did not"
echo "PASS"
