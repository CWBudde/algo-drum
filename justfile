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

# Ensure go.mod / go.sum are tidy
check-tidy:
    GOARCH=wasm GOOS=js go mod tidy
    git diff --exit-code go.mod go.sum

# Build the WASM binary and copy wasm_exec.js to web/public/
build-wasm:
    bash scripts/build-wasm.sh

# ── Frontend ─────────────────────────────────────────────────────────────────

# Install frontend dependencies
web-install:
    cd web && bun install

# Type-check the frontend
web-typecheck:
    cd web && bun run tsc --noEmit

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

# Run all CI checks: formatting, linting, tidy, typecheck
ci: check-formatted lint check-tidy web-typecheck

# ── Housekeeping ─────────────────────────────────────────────────────────────

# Remove build artifacts
clean:
    rm -f web/public/algo_drum.wasm web/public/wasm_exec.js
    rm -rf web/dist

fix:
    just lint-fix
    just fmt
