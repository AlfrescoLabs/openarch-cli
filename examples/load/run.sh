#!/usr/bin/env bash
# Starts a new workspace with the permission demo, loads the documents of
# plans/ with their passages, and asks three questions by meaning as alice,
# bob and carol. The script checks the answers itself, and always deletes the
# stack at the end.
#
# Usage: ./run.sh
# Environment: OPENARCH (the CLI binary), PORT (the OpenArch port of the new
# workspace, default 18089).
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
port=${PORT:-18089}
openarch=$(command -v "${OPENARCH:-openarch}") || { echo "openarch not found; set OPENARCH to the binary" >&2; exit 3; }
docker model status >/dev/null 2>&1 || { echo "Docker Model Runner is not running: enable it in Docker Desktop" >&2; exit 3; }
docker model inspect ai/mxbai-embed-large >/dev/null 2>&1 || { echo "model ai/mxbai-embed-large not found; pull it with: docker model pull ai/mxbai-embed-large" >&2; exit 3; }
tmp=$(mktemp -d)
# Compose names the project after the folder, so the name must be unique.
name=openarch-load-$$
ws=$tmp/$name
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
# The demo makes the folders and their access entries. The load puts each
# document in one of them, so the document gets the access of its folder.
openarch demo load --dir "$ws" >/dev/null
echo "== load"
openarch load "$here/plans/" --dir "$ws"

# ask <user> <question>: the passages that this user gets, one per line.
ask() {
	openarch query --semantic "$2" --context "$name-$1" --output json |
		sed -n 's/^ *"text": "\(.*\)",\{0,1\}$/\1/p' | tail -n +2
}
# The vectors become searchable about a second after the load.
for _ in $(seq 1 30); do
	[ -n "$(ask alice "When is the office open?")" ] && break
	sleep 1
done
fail=0
# check <user> <question> <text that must be in the first answer> <text that must not be anywhere>
check() {
	local got
	got=$(ask "$1" "$2")
	echo
	echo "== $1: $2"
	echo "$got" | head -1
	if [ -n "$3" ] && ! echo "$got" | head -1 | grep -q -- "$3"; then
		echo "FAIL: the first passage for $1 does not have \"$3\"" >&2
		fail=1
	fi
	if [ -n "$4" ] && echo "$got" | grep -q -- "$4"; then
		echo "FAIL: $1 got \"$4\", which $1 may not read" >&2
		fail=1
	fi
}
pay="How much is level 3 paid?"
rate="What does a senior developer cost a day?"
hours="When is the office open?"
check alice "$pay" "52,000 EUR" ""
check bob "$pay" "" "52,000 EUR"
check carol "$pay" "" "52,000 EUR"
check bob "$rate" "650 EUR" ""
check alice "$rate" "" "650 EUR"
check carol "$rate" "" "650 EUR"
check carol "$hours" "8:00 to 19:00" ""

echo
if [ "$fail" -ne 0 ]; then
	echo "FAIL"
	exit 1
fi
echo "PASS: alice gets the pay scale, bob the rate card, carol only the office hours"
