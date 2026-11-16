#!/usr/bin/env bash
# End-to-end test against a real Docker stack: init, up, smoke-test, the demo
# with its permission check, restart, check that data is kept, then clean.
#
# Usage: test/e2e.sh <openarch binary> [init flags...]
# Example: test/e2e.sh bin/openarch --storage bind
#
# E2E_CHECK_OWNERS=1 also checks the owners of the bind folders. Use it on
# native Linux Docker only: Docker Desktop maps owners to the host user.
set -euo pipefail

bin=$(cd "$(dirname "$1")" && pwd)/$(basename "$1")
shift
repo=$(cd "$(dirname "$0")/.." && pwd)
# Workspaces with the users bundled in the OpenArch image have no demo users.
demo=1
for arg in "$@"; do
	if [ "$arg" = --default-users ]; then
		demo=
	fi
done
tmp=$(mktemp -d)
# Compose names the project after the folder, so a unique name keeps old
# volumes of another run out of this one.
ws=$tmp/oa-e2e-$$
# Keep the test away from the real CLI config and the OS credential store.
export OPENARCH_CONFIG_DIR=$tmp/config
export OPENARCH_CREDENTIAL_STORE=file

oa() { "$bin" --dir "$ws" --non-interactive "$@"; }

on_error() {
	if [ -n "${fake:-}" ]; then
		kill "$fake" 2>/dev/null || true
	fi
	echo "::group::status and logs"
	oa status || true
	oa logs --tail 200 || true
	echo "::endgroup::"
}
trap on_error ERR

step() { echo "== $*"; }

step "init $*"
"$bin" init --dir "$ws" --non-interactive "$@"

if [ -n "$demo" ]; then
	# OpenArch reads its users only at startup, so they come before up.
	step "demo users"
	oa demo users
fi

step "up"
oa up

step "smoke-test"
oa smoke-test

if [ -n "$demo" ]; then
	step "demo load and demo check, with vectors from a fake model"
	(cd "$repo" && go build -o "$tmp/fakemodel" ./test/fakemodel)
	"$tmp/fakemodel" 127.0.0.1:18434 &
	fake=$!
	for _ in $(seq 50); do
		if curl -fsS -X POST -d '{"input":["ready"]}' http://127.0.0.1:18434/v1/embeddings >/dev/null 2>&1; then
			break
		fi
		sleep 0.2
	done
	model=(--model-url http://127.0.0.1:18434/v1)
	oa demo load --embeddings "${model[@]}"
	oa demo check --semantic "${model[@]}"
	kill "$fake"
fi

step "create a folder, then restart"
oa documents create --parent / --name e2e-persist --type SysFolder
oa down
oa up

step "the folder is still there"
oa query --sync "SELECT * FROM SysFolder WHERE sys_name = 'e2e-persist'" | grep -q '/e2e-persist'

step "no OpenSearch permission errors"
errors=$(oa logs openarch-app | grep -c security_exception || true)
if [ "$errors" != 0 ]; then
	echo "found $errors security_exception lines in the openarch-app logs"
	exit 1
fi

if [ "${E2E_CHECK_OWNERS:-}" = 1 ]; then
	step "owners of the bind folders"
	for pair in opensearch:1000 openarch:33010 mongodb:999; do
		dir=$ws/data/${pair%%:*}
		want=${pair#*:}
		got=$(stat -c %u "$dir")
		if [ "$got" != "$want" ]; then
			echo "$dir is owned by uid $got, want $want"
			exit 1
		fi
	done
fi

step "clean"
oa clean --yes
echo "e2e passed"
