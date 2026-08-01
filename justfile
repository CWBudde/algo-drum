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

# Run Go linter (WASM target), then the nested figure-tool module.
#
# tools/paper-figures is its own module and needs the `purego` tag to resolve the
# raster backend, so it cannot ride along on the main invocation.
lint:
    GOOS=js GOARCH=wasm GOCACHE="${GOCACHE:-/tmp/gocache}" GOMODCACHE="${GOMODCACHE:-/tmp/gomodcache}" GOLANGCI_LINT_CACHE="${GOLANGCI_LINT_CACHE:-/tmp/golangci-lint-cache}" golangci-lint run --timeout=2m ./...
    cd tools/paper-figures && GOCACHE="${GOCACHE:-/tmp/gocache}" GOMODCACHE="${GOMODCACHE:-/tmp/gomodcache}" GOLANGCI_LINT_CACHE="${GOLANGCI_LINT_CACHE:-/tmp/golangci-lint-cache}" golangci-lint run --build-tags purego --timeout=2m ./...

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
#
# Reports and checkpoints go to fits/, audio to renders/; both are gitignored
# and created on demand, so nothing a run produces lands in the project root.
# The progress log is stderr, which the tool does not open, so redirect it into
# fits/ alongside the report it belongs to:
#
#   just fit-physical reference/tom.wav "-o fits/fit-x.json …" 2> fits/fit-x.log
#
# -channel right is spelled out rather than left to the tool's `mono` default:
# reference/tom.wav is a stereo capture, and its right channel is the target
# every number in docs/physical-measured-fit.md and in
# testdata/physical-fit-tom.json was fitted to. Averaging the pair instead is a
# legitimate reduction of a *different* signal, so a run that took the default
# could not be compared with any of them. A -channel in {{args}} comes later on
# the command line and still wins, for a reference that is not this one.
fit-physical reference="reference/tom.wav" *args="":
    go run ./cmd/fit-physical -reference {{reference}} -channel right \
        -o fits/fit-report.json -checkpoint fits/fit-report.checkpoint {{args}}

# Derive the measurement tables from recordings of a real drum.
#
# The capture protocol is docs/physical-measurement-protocol.md. Like
# fit-physical this needs files that are not in the repository, so it is not
# part of `just ci`; unlike fit-physical its output is meant to be committed.
measure-tom +files:
    go run ./cmd/measure-tom -o tom-measurements.json {{files}}

# Fail if the physical-model calibration metrics are stale
check-physical-reference: gen-physical-reference
    git diff --exit-code testdata/physical-reference-v2.json

# Regenerate the derived artefact behind the paper's model figures.
#
# Unlike testdata/physical-reference-v2.json this is not CI-diffed: it is a paper
# artefact, regenerated deliberately when the model changes, in the same way the
# committed PNGs are. Everything in it is closed-form — the modal bank and the
# continuous-time cavity solve — so it needs no render and no recording.
paper-data:
    go run ./cmd/analyze-physical -paper-data -o docs/paper/model-data.json

# Regenerate the paper's figures.
#
# tools/paper-figures is its own module so that matplotlib-go's graphics tree
# stays out of the engine's go.mod; `purego` selects its pure-Go rasteriser, so
# this needs no FreeType headers. comb.png is not produced here: it is measured
# from the two channels of the recording, which the repository does not contain.
#
# The default report is the one the committed fit figures were drawn from — the
# run the paper's @results reads, total 11.252. It matters: passing a different
# report silently redraws bands/decay/partials/terms.png from a fit the prose
# does not describe, and the figures carry no record of which run made them.
# That is why a later fit gets its own recipe below rather than this one's
# argument: two runs, two suffixes, neither able to overwrite the other.
#
# Fit reports are gitignored (.gitignore: fits/), so this recipe needs a local
# fit run first. The committed PNGs, not this recipe, are what lets the paper
# build from the repository alone.
paper-figures report="fits/fit-final-prescribed.json":
    report="$(realpath {{report}})"; \
    cd tools/paper-figures && go run -tags purego . \
        -report "$report" \
        -model-data "{{justfile_directory()}}/docs/paper/model-data.json" \
        -o "{{justfile_directory()}}/docs/paper/figures"

# Regenerate the two figures of the paper's @refit chapter.
#
# The refit under the corrected glide estimator (2026-08-01, total 10.382) is a
# different run from the one @results describes, so it gets its own figures under
# a -refit suffix instead of redrawing that chapter's. Only terms and decay are
# drawn: the chapter argues coverage and spectral envelope in prose, and an
# uncited PNG in docs/paper/figures is drift waiting to happen.
paper-figures-refit report="fits/s3-right-prescribed.json":
    report="$(realpath {{report}})"; \
    cd tools/paper-figures && go run -tags purego . \
        -report "$report" -suffix -refit -only terms,decay \
        -o "{{justfile_directory()}}/docs/paper/figures"

# Build the model-matching paper (docs/paper/paper.typ -> PDF).
#
# Needs typst on PATH. Deliberately not part of `just ci`: the figures are
# committed, so the PDF is reproducible from the repository alone, but nothing
# in the build depends on it.
paper:
    cd docs/paper && typst compile --root . \
        --input revision="$(git rev-parse --short HEAD)" \
        paper.typ physical-tom-matching.pdf

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
