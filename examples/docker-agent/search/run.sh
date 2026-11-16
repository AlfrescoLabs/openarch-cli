#!/usr/bin/env bash
# Starts a new workspace with the permission demo, then asks the agent in
# agent.yaml the same question as alice and as bob. The agent has a shell, the
# Agent Skills of the CLI and a local model. The script checks the answers
# itself, and always deletes the stack at the end.
#
# Usage: ./run.sh
# Environment: MODEL (a Docker Model Runner model), OPENARCH (the CLI binary),
# PORT (the OpenArch port of the new workspace, default 18089).
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

tmp=$(mktemp -d)
project=$tmp/project
# Compose names the project after the folder, so the name must be unique.
name=openarch-search-$$
ws=$tmp/$name
question="How much is the pay for each job level?"
# The demo never touches the saved contexts or the OS credential store.
export OPENARCH_CONFIG_DIR=$tmp/config OPENARCH_CREDENTIAL_STORE=file
export PATH="$(dirname "$openarch"):$PATH"

cleanup() {
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
# The data must be right before the agent starts.
openarch demo check --dir "$ws" --semantic | tail -1

mkdir -p "$project"
openarch skill install --dir "$project" >/dev/null

for user in alice bob; do
	echo
	echo "== agent ($model) as $user: $question"
	(cd "$project" && docker agent run "$here/agent.yaml" --exec --safety restricted --model "root=dmr/$model" \
		"Search as the openarch context $name-$user. $question") | tee "$tmp/$user.txt"
done

# Do not trust the answers: check them for the salary bands, which alice may
# read and bob may not.
echo
echo "== check"
grep -q "Salary bands 2026" "$tmp/alice.txt" || { echo "FAIL: alice's answer does not use the salary bands" >&2; exit 1; }
if grep -q -e "Salary bands" -e "oafx-03-sp2n" "$tmp/bob.txt"; then
	echo "FAIL: bob's answer has the salary bands, which bob may not read" >&2
	exit 1
fi
echo "alice got the salary bands, bob did not"
echo "PASS"
