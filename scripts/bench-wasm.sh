#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"

# Locate the js/wasm exec wrapper Go 1.21+ ships under lib/wasm; older
# toolchains keep it in misc/wasm. `go test` finds it by *name* on PATH, so its
# directory is prepended to the PATH handed to the run below.
GO_JS_WASM_EXEC=""
for candidate in \
	"$(go env GOROOT)/lib/wasm/go_js_wasm_exec" \
	"$(go env GOROOT)/misc/wasm/go_js_wasm_exec"; do
	if [[ -x $candidate ]]; then
		GO_JS_WASM_EXEC="$candidate"
		break
	fi
done

if [[ -z $GO_JS_WASM_EXEC ]]; then
	echo "ERROR: go_js_wasm_exec not found under GOROOT=$(go env GOROOT)" >&2
	exit 1
fi

if ! command -v node >/dev/null 2>&1; then
	echo "ERROR: node not found on PATH; go_js_wasm_exec runs the test binary under Node.js" >&2
	exit 1
fi

# The whole reason this script exists.
#
# wasm_exec.js caps the *combined* argv + environ block at 4096 bytes, and
# go_js_wasm_exec forwards the entire environment of the calling shell. A normal
# developer shell (or anything with a fat env: direnv, ssh-agent, a desktop
# session) blows straight through that and the run dies with
#
#     total length of command line and environment variables exceeds limit
#
# before a single benchmark iteration executes. So re-exec under `env -i`,
# carrying only the variables the toolchain and the wrapper actually need.
# GOCACHE/GOMODCACHE are pinned the same way the justfile's lint recipes pin
# them, because `env -i` drops HOME-derived defaults along with everything else.
GOCACHE_VALUE="${GOCACHE:-/tmp/gocache}"
GOMODCACHE_VALUE="${GOMODCACHE:-/tmp/gomodcache}"

args=("$@")
if [[ ${#args[@]} -eq 0 ]]; then
	args=(-run '^$' -bench . ./internal/physical/)
fi

cd "$ROOT_DIR"
exec env -i \
	PATH="$(dirname "$GO_JS_WASM_EXEC"):$PATH" \
	HOME="$HOME" \
	GOOS=js \
	GOARCH=wasm \
	GOCACHE="$GOCACHE_VALUE" \
	GOMODCACHE="$GOMODCACHE_VALUE" \
	"$(command -v go)" test "${args[@]}"
