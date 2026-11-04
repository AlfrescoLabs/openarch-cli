#!/usr/bin/env bash
# End-to-end test against a real Docker stack: init, up, smoke-test, restart,
# check that data is kept, then clean.
#
# Usage: test/e2e.sh <openarch binary> [init flags...]
# Example: test/e2e.sh bin/openarch --storage bind
#
# E2E_CHECK_OWNERS=1 also checks the owners of the bind folders. Use it on
# native Linux Docker only: Docker Desktop maps owners to the host user.
set -euo pipefail

bin=$(cd "$(dirname "$1")" && pwd)/$(basename "$1")
shift
tmp=$(mktemp -d)
# Compose names the project after the folder, so a unique name keeps old
# volumes of another run out of this one.
ws=$tmp/oa-e2e-$$
# Keep the test away from the real CLI config and the OS credential store.
export OPENARCH_CONFIG_DIR=$tmp/config
export OPENARCH_CREDENTIAL_STORE=file

oa() { "$bin" --dir "$ws" --non-interactive "$@"; }

on_error() {
	echo "::group::status and logs"
	oa status || true
	oa logs --tail 200 || true
	echo "::endgroup::"
}
trap on_error ERR

step() { echo "== $*"; }

step "init $*"
"$bin" init --dir "$ws" --non-interactive "$@"

step "up"
oa up

step "smoke-test"
oa smoke-test

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
