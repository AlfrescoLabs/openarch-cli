#!/usr/bin/env bash
# Starts a new workspace with the permission demo and the documents of
# ../load/plans, then asks one agent the same question as alice and as bob.
# For each user, two copies of `openarch mcp --read-only --listen` run behind
# a round-robin proxy, so the requests of one agent go to both copies. The
# script checks the answers and the wire log itself, and always deletes the
# stack at the end.
#
# Usage: ./run.sh
# Environment: MODEL (a Docker Model Runner model), OPENARCH (the CLI binary),
# PORT (the OpenArch port of the new workspace, default 18089). The proxy
# listens on 127.0.0.1:18790 (the URL in agent.yaml), the copies on 18791 and
# 18792.
set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)
model=${MODEL:-glm-4.7-flash:31B}
port=${PORT:-18089}
openarch=$(command -v "${OPENARCH:-openarch}") || { echo "openarch not found; set OPENARCH to the binary" >&2; exit 3; }

docker agent version >/dev/null 2>&1 || { echo "Docker Agent not found: https://github.com/docker/docker-agent" >&2; exit 3; }
docker model status >/dev/null 2>&1 || { echo "Docker Model Runner is not running: enable it in Docker Desktop" >&2; exit 3; }
for m in "$model" ai/mxbai-embed-large; do
	docker model inspect "$m" >/dev/null 2>&1 || { echo "model $m not found; pull it with: docker model pull $m" >&2; exit 3; }
done
command -v go >/dev/null 2>&1 || { echo "Go not found: the round-robin proxy is built with go build" >&2; exit 3; }

tmp=$(mktemp -d)
# Compose names the project after the folder, so the name must be unique.
name=openarch-stateless-$$
ws=$tmp/$name
question="How much is level 3 paid?"
# The demo never touches the saved contexts or the OS credential store.
export OPENARCH_CONFIG_DIR=$tmp/config OPENARCH_CREDENTIAL_STORE=file
export PATH="$(dirname "$openarch"):$PATH"

pids=()
stop_servers() {
	for p in "${pids[@]:-}"; do
		[ -n "$p" ] && kill "$p" 2>/dev/null || true
	done
	for p in "${pids[@]:-}"; do
		[ -n "$p" ] && wait "$p" 2>/dev/null || true
	done
	pids=()
}
cleanup() {
	stop_servers
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
openarch load "$here/../load/plans/" --dir "$ws" 2>/dev/null | head -1
# The vectors become searchable about a second after the load ends.
sleep 2

(cd "$here/rrproxy" && go build -o "$tmp/rrproxy" .)
# Make the token before any server starts: two first calls at once can race.
OPENARCH_MCP_TOKEN=$(openarch mcp token)
export OPENARCH_MCP_TOKEN

for user in alice bob; do
	echo
	echo "== two copies of the MCP server as $user, behind a round-robin proxy"
	for p in 18791 18792; do
		openarch mcp --context "$name-$user" --read-only --listen "$p" >"$tmp/server-$user-$p.log" 2>&1 &
		pids+=($!)
	done
	"$tmp/rrproxy" -listen 127.0.0.1:18790 -log "$tmp/wire-$user.jsonl" \
		http://127.0.0.1:18791 http://127.0.0.1:18792 2>"$tmp/proxy-$user.log" &
	pids+=($!)
	for _ in $(seq 1 40); do
		if grep -q "MCP server on" "$tmp/server-$user-18791.log" 2>/dev/null &&
			grep -q "MCP server on" "$tmp/server-$user-18792.log" 2>/dev/null &&
			grep -q "rrproxy on" "$tmp/proxy-$user.log" 2>/dev/null; then
			break
		fi
		sleep 0.25
	done

	echo "== agent ($model) as $user: $question"
	(cd "$tmp" && docker agent run "$here/agent.yaml" --exec --model "root=dmr/$model" "$question") | tee "$tmp/$user.txt"
	stop_servers
	echo
	echo "== wire log of $user"
	cat "$tmp/wire-$user.jsonl"
done

# Do not trust the answers: check them, and check what went over the wire.
echo
echo "== check"
grep -q "52,000" "$tmp/alice.txt" || { echo "FAIL: alice's answer does not have the pay of level 3" >&2; exit 1; }
if grep -q -e "52,000" -e "Pay scale 2027" "$tmp/bob.txt"; then
	echo "FAIL: bob's answer has the pay scale, which bob may not read" >&2
	exit 1
fi
for user in alice bob; do
	wire=$tmp/wire-$user.jsonl
	grep -q '"mcpMethod":"tools/call"' "$wire" || { echo "FAIL: the agent of $user made no tool call" >&2; exit 1; }
	if grep '"httpMethod":"POST"' "$wire" | grep -v -q '"protocolVersion":"2026-07-28"'; then
		echo "FAIL: a request of $user did not use MCP 2026-07-28" >&2
		exit 1
	fi
	if grep -q '"sessionId' "$wire"; then
		echo "FAIL: a request or reply of $user has a session id" >&2
		exit 1
	fi
	for p in 18791 18792; do
		grep -q "\"upstream\":\"127.0.0.1:$p\"" "$wire" || { echo "FAIL: no request of $user went to the copy on $p" >&2; exit 1; }
	done
done
echo "alice got the pay of level 3, bob did not"
echo "every request used MCP 2026-07-28 with no session, on both copies of the server"
echo "PASS"
