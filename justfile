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

# Build under the `purego` tag — a convention guard, not a numerical one.
#
# This repository no longer contains any architecture-gated code: the assembly
# kernel and its portable twin moved to github.com/cwbudde/algo-tom, which gates
# both halves in its own CI. So there is nothing here for the tag to *select*,
# and running the tests under it would recompile an identical program.
#
# The build stays because `purego` remains a repo-wide convention. Anything added
# here that gates on it — or any dependency that does — must keep compiling under
# it, and a build is what states that. Mirrors the `purego` job in ci-go.yml.
test-purego:
    go build -tags purego ./...

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

# Fail if the generated voice parameter table is stale (Go table changed without `just gen-params`)
check-params: gen-params
    git diff --exit-code web/src/engine/voiceParams.generated.ts

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
ci: check-formatted lint check-tidy check-params test test-assert test-purego web-typecheck web-test

# ── Housekeeping ─────────────────────────────────────────────────────────────

# Remove build artifacts
clean:
    rm -f web/public/algo_drum.wasm web/public/wasm_exec.js
    rm -rf web/dist

fix:
    just lint-fix
    just fmt
