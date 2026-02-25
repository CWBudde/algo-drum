#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
OUT_DIR="$ROOT_DIR/web/public"

mkdir -p "$OUT_DIR"

# Copy wasm_exec.js from Go installation
WASM_EXEC=""
for candidate in \
	"$(go env GOROOT)/lib/wasm/wasm_exec.js" \
	"$(go env GOROOT)/misc/wasm/wasm_exec.js"; do
	if [[ -f $candidate ]]; then
		WASM_EXEC="$candidate"
		break
	fi
done

if [[ -z $WASM_EXEC ]]; then
	echo "ERROR: wasm_exec.js not found under GOROOT=$(go env GOROOT)" >&2
	exit 1
fi

cp "$WASM_EXEC" "$OUT_DIR/wasm_exec.js"
GOOS=js GOARCH=wasm go build -o "$OUT_DIR/algo_drum.wasm" "$ROOT_DIR/cmd/wasm/"

echo "Built $OUT_DIR/algo_drum.wasm"
echo "Copied $OUT_DIR/wasm_exec.js"
