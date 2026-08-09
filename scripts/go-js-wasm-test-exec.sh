#!/usr/bin/env bash

set -euo pipefail

# wasm_exec.js serializes the complete process environment into the WASM
# module. Agent shells and some developer environments can exceed its fixed
# argv+environment limit, although the test itself only needs PATH and a temp
# directory. Keep compilation in the caller's normal environment and sanitize
# only the Node-hosted test process.
exec env -i \
	PATH="$PATH" \
	TMPDIR="${TMPDIR:-/tmp}" \
	"$(go env GOROOT)/lib/wasm/go_js_wasm_exec" "$@"
