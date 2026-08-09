#!/usr/bin/env bash

set -euo pipefail

readonly MIN_COVERAGE=90.0
profile="$(mktemp "${TMPDIR:-/tmp}/algo-drum-coverage.XXXXXX")"
trap 'rm -f "$profile"' EXIT

go test ./internal/drum -coverprofile="$profile"

coverage="$(go tool cover -func="$profile" | awk '/^total:/ { gsub(/%/, "", $3); print $3 }')"
if [[ -z $coverage ]]; then
	echo "could not read total coverage" >&2
	exit 1
fi

awk -v coverage="$coverage" -v minimum="$MIN_COVERAGE" 'BEGIN { exit !(coverage + 0 >= minimum + 0) }' || {
	echo "Go engine coverage ${coverage}% is below ${MIN_COVERAGE}%" >&2
	exit 1
}

echo "Go engine coverage ${coverage}% meets the ${MIN_COVERAGE}% floor"
