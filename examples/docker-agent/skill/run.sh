#!/usr/bin/env bash
# Installs the openarch skill in a new project in a temp folder, runs the agent
# in agent.yaml there, checks the result with 'openarch smoke-test', and always
# deletes the stack at the end.
#
# Usage: ./run.sh
# Environment: MODEL (a Docker Model Runner model), OPENARCH (the CLI binary).
set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)
model=${MODEL:-glm-4.7-flash:31B}
openarch=$(command -v "${OPENARCH:-openarch}") || { echo "openarch not found; set OPENARCH to the binary" >&2; exit 3; }

docker agent version >/dev/null 2>&1 || { echo "Docker Agent not found: https://github.com/docker/docker-agent" >&2; exit 3; }
docker model status >/dev/null 2>&1 || { echo "Docker Model Runner is not running: enable it in Docker Desktop" >&2; exit 3; }
docker model inspect "$model" >/dev/null 2>&1 || { echo "model $model not found; pull it with: docker model pull $model" >&2; exit 3; }

tmp=$(mktemp -d)
project=$tmp/project
# Compose names the project after the folder, so the name must be unique.
name=openarch-skill-$$
ws=$project/$name
task="Set up OpenArch for local development in ./$name."
# The agent never touches the saved contexts or the OS credential store.
export OPENARCH_CONFIG_DIR=$tmp/config OPENARCH_CREDENTIAL_STORE=file
export PATH="$(dirname "$openarch"):$PATH"

cleanup() {
	if [ -f "$ws/openarch.yaml" ]; then
		echo "== clean"
		openarch clean --dir "$ws" --yes >/dev/null 2>&1 || echo "clean failed; run: openarch clean --dir $ws --yes" >&2
	fi
}
trap cleanup EXIT

mkdir -p "$project"
openarch skill install --dir "$project"

echo "== agent ($model)"
echo "== task: $task"
(cd "$project" && docker agent run "$here/agent.yaml" --exec --safety restricted --model "root=dmr/$model" "$task")

# Do not trust the report of the agent: check the stack directly.
echo
echo "== check"
if [ ! -f "$ws/openarch.yaml" ]; then
	echo "FAIL: the agent created no workspace" >&2
	exit 1
fi
openarch smoke-test --dir "$ws"
echo "PASS"
