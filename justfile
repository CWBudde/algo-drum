set shell := ["bash", "-uc"]

export GOPRIVATE := "github.com/cwbudde"

# Default recipe - show available commands
default:
    @just --list

# ── Formatting ───────────────────────────────────────────────────────────────

# Format all code (Go + frontend) using treefmt
fmt:
    treefmt --allow-missing-formatter

# Check if code is formatted correctly
check-formatted:
    treefmt --allow-missing-formatter --fail-on-change

# ── Go / WASM ────────────────────────────────────────────────────────────────

# Run Go linter (WASM target)
lint:
    GOOS=js GOARCH=wasm GOCACHE="${GOCACHE:-/tmp/gocache}" GOMODCACHE="${GOMODCACHE:-/tmp/gomodcache}" GOLANGCI_LINT_CACHE="${GOLANGCI_LINT_CACHE:-/tmp/golangci-lint-cache}" golangci-lint run --timeout=2m ./...

# Run Go linter with auto-fix (WASM target)
lint-fix:
    GOOS=js GOARCH=wasm GOCACHE="${GOCACHE:-/tmp/gocache}" GOMODCACHE="${GOMODCACHE:-/tmp/gomodcache}" GOLANGCI_LINT_CACHE="${GOLANGCI_LINT_CACHE:-/tmp/golangci-lint-cache}" golangci-lint run --fix --timeout=2m ./...

# Run the Go test suite
test:
    go test ./...

# Run the Go test suite with the engine self-check compiled into Render
# (internal/drum/assert_debug.go); a broken invariant panics where it happens
# instead of surfacing as plausible-sounding audio.
test-assert:
    go test -tags drumassert ./...

# Ensure go.mod / go.sum are tidy
check-tidy:
    GOARCH=wasm GOOS=js go mod tidy
    git diff --exit-code go.mod go.sum

# Build the WASM binary and copy wasm_exec.js to web/public/
build-wasm:
    bash scripts/build-wasm.sh

# Regenerate the TypeScript mirror of the engine's voice parameter table
gen-params:
    go run ./cmd/gen-voiceparams -o web/src/engine/voiceParams.generated.ts
    cd web && bunx prettier@3.9.5 --write src/engine/voiceParams.generated.ts

# Regenerate the deterministic physical-model calibration metrics
gen-physical-reference:
    go run ./cmd/analyze-physical -suite -o testdata/physical-reference-v2.json

# Fail if the generated voice parameter table is stale (Go table changed without `just gen-params`)
check-params: gen-params
    git diff --exit-code web/src/engine/voiceParams.generated.ts

# Fit the physical Tom's parameter bank to a recorded hit.
#
# Minutes, not seconds, and it needs a reference recording that is not in the
# repository — so it is deliberately not part of `just ci`. See
# docs/physical-measured-fit.md.
fit-physical reference="reference/tom.wav" *args="":
    go run ./cmd/fit-physical -reference {{reference}} -o fit-report.json {{args}}

# Fail if the physical-model calibration metrics are stale
check-physical-reference: gen-physical-reference
    git diff --exit-code testdata/physical-reference-v2.json

# ── Frontend ─────────────────────────────────────────────────────────────────

# Install frontend dependencies
web-install:
    cd web && bun install

# Type-check the frontend
web-typecheck:
    cd web && bun run typecheck

# Run the frontend unit tests (vitest)
web-test:
    cd web && bun run test

# Run the Vite dev server (WASM must be built first)
dev: build-wasm
    cd web && bun run dev

# Build the production frontend bundle (WASM + Vite)
build: build-wasm
    cd web && bun run build

# Preview the production build locally
preview: build
    cd web && bun run preview

# ── Quality gates ────────────────────────────────────────────────────────────

# Run all CI checks, mirroring .github/workflows/ci.yml (a green `just ci` must mean the same as a green CI run)
ci: check-formatted lint check-tidy check-params check-physical-reference test test-assert web-typecheck web-test

# ── Housekeeping ─────────────────────────────────────────────────────────────

# Remove build artifacts
clean:
    rm -f web/public/algo_drum.wasm web/public/wasm_exec.js
    rm -rf web/dist

fix:
    just lint-fix
    just fmt
