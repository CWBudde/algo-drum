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
#
# **Every fit file carries the reference it was made against**, so this recipe
# derives its own names rather than taking them: reference/tt08x08/lp/hd/v08.wav
# gives fits/fit-tt08x08-lp-hd-v08.{json,checkpoint}. A fit report is only
# meaningful beside the recording it targeted — the gates, the totals and the
# whole partial table are properties of that drum at that tuning — and a
# directory of fit-report.json, fit-v2.json, fit-final.json says nothing about
# which. Hand-run commands should follow the same rule; see AGENTS.md.
#
# The progress log is stderr, which the tool does not open, so redirect it into
# fits/ under the matching name:
#
#   just fit-physical reference/tt08x08/lp/hd/v08.wav 2> fits/fit-tt08x08-lp-hd-v08.log
#
# The default reference is the committed, CC BY 4.0, 8"x8" tom at **low** pitch
# and mid velocity — see reference/CREDITS.md for licence, provenance and the
# measured properties. It replaced reference/tom.wav, which had unknown
# provenance and no licence and was deleted on 2026-08-01 (PLAN.md P10/N8).
#
# Low pitch rather than the medium set this started on, chosen on the sound. It
# is also the better-behaved target of the two, which was not the reason but is
# worth knowing: the objective disagrees with itself less on every one of the
# nine terms, and on glide by a factor of 12, because this drum's fundamental
# outlives the estimator's late probe and the medium one's does not. The gates in
# internal/physical/match/distance.go are measured on *this* set and do not
# transfer to another.
#
# -channel mono is spelled out rather than left implicit because the choice is
# load-bearing and was wrong for the reference before last. This pair is
# *coincident* (peak inter-channel correlation at 0 samples of lag on thirteen of
# the sixteen and 1 sample on the rest), so averaging it is a clean reduction —
# a single sample at 48 kHz is 21 µs and combs nothing in band. The pair before
# it had channels 1.56 ms apart and correlated 0.36 at zero lag, so summing
# *that* file combed the target, which is why every archived number was fitted to
# its right channel alone. A -channel in {{args}} comes later on the command line
# and still wins.
#
# **The drum's geometry is pinned, not fitted.** Every committed pack states its
# shell as <diameter>x<depth> inches in the directory name, so this recipe reads
# it off the path and passes it as -set SIZE=/-set DEPTH= in metres. Leaving them
# free let the search answer an 8"x8" recording with a 12" head on a 20 cm shell —
# the shipped defaults — and then report the result as a fit. Two parameters of
# eighteen become constants, and they are two the recording cannot argue with.
#
# Derived rather than typed so it cannot drift from the reference: point the
# recipe at another pack and the geometry follows. A recording outside
# reference/<WxH>/ has no geometry to read, so the recipe leaves both free and
# says so — that is the honest default for an unknown drum.
#
# One hit does not identify this model's parameters — the fit wants the whole
# sixteen-velocity series jointly (PLAN.md P10/N5), which is what
# `just fit-physical-series` runs. Treat a single-file run as a diagnostic.
fit-physical reference="reference/tt08x08/lp/hd/v08.wav" *args="":
    #!/usr/bin/env bash
    set -euo pipefail
    slug="$(printf '%s' '{{reference}}' | sed -e 's|^reference/||' -e 's|\.wav$||' -e 's|/|-|g')"
    geometry=()
    if [[ '{{reference}}' =~ reference/[a-z]*([0-9]+)x([0-9]+)/ ]]; then
        diameter="$(awk -v inches="${BASH_REMATCH[1]}" 'BEGIN { printf "%.4f", inches * 0.0254 }')"
        depth="$(awk -v inches="${BASH_REMATCH[2]}" 'BEGIN { printf "%.4f", inches * 0.0254 }')"
        geometry=(-set "SIZE=$diameter" -set "DEPTH=$depth")
        echo "geometry from the path: ${BASH_REMATCH[1]}\" x ${BASH_REMATCH[2]}\" = $diameter m x $depth m" >&2
    else
        echo "no <diameter>x<depth> in the reference path: SIZE and DEPTH stay free" >&2
    fi
    go run ./cmd/fit-physical -reference '{{reference}}' -channel mono \
        "${geometry[@]}" \
        -o "fits/fit-$slug.json" -checkpoint "fits/fit-$slug.checkpoint" {{args}}

# Fit one parameter bank jointly to a whole velocity series.
#
# This is the fit PLAN.md P10/N5 asks for, and the one whose result is worth
# quoting. `just fit-physical` scores a bank against a single hit, which does not
# identify this model: the strike velocity is itself fitted, so one recording
# leaves the contact parameters and the Berger nonlinearity free to trade against
# how hard the drum is assumed to have been hit. A series pins that trade —
# sixteen takes, one shared bank, and one velocity per take.
#
# **The file order is not used as evidence, and this matters.** The takes are
# named v01…v16 in what the pack says is increasing strike order, but they were
# played by hand and nothing verified the labelling. So each take carries its own
# free velocity, no take is constrained to be harder than the one before it, and
# a series whose middle files are swapped costs the fit nothing. The summary then
# prints the fitted velocities against the file order and counts where the two
# disagree — the labelling is measured rather than assumed, and neither the tool
# nor this recipe renames anything to make the number look better.
#
# Sixteen takes is sixteen renders and sixteen feature extractions per candidate,
# so this is roughly sixteen times a single-file run: hours, not minutes. It
# checkpoints like any other fit, so an interrupt keeps what it found — and the
# checkpoint fingerprint carries the take list *in order*, since the velocities
# occupy the tail of every stored position and a re-ordered list would hand each
# take another take's velocity without saying so.
#
# The directory names the drum, so geometry is read off it exactly as
# fit-physical does, and the fit is named after the series rather than a file.
fit-physical-series directory="reference/tt08x08/lp/hd" *args="":
    #!/usr/bin/env bash
    set -euo pipefail
    slug="$(printf '%s' '{{directory}}' | sed -e 's|^reference/||' -e 's|/$||' -e 's|/|-|g')"
    takes=()
    for wav in $(ls '{{directory}}'/*.wav | sort); do
        takes+=(-reference "$wav")
    done
    if [[ ${#takes[@]} -eq 0 ]]; then
        echo "no .wav files under {{directory}}" >&2
        exit 1
    fi
    geometry=()
    if [[ '{{directory}}' =~ reference/[a-z]*([0-9]+)x([0-9]+)/ ]]; then
        diameter="$(awk -v inches="${BASH_REMATCH[1]}" 'BEGIN { printf "%.4f", inches * 0.0254 }')"
        depth="$(awk -v inches="${BASH_REMATCH[2]}" 'BEGIN { printf "%.4f", inches * 0.0254 }')"
        geometry=(-set "SIZE=$diameter" -set "DEPTH=$depth")
        echo "geometry from the path: ${BASH_REMATCH[1]}\" x ${BASH_REMATCH[2]}\" = $diameter m x $depth m" >&2
    else
        echo "no <diameter>x<depth> in the reference path: SIZE and DEPTH stay free" >&2
    fi
    echo "fitting $(( ${#takes[@]} / 2 )) takes jointly" >&2
    go run ./cmd/fit-physical "${takes[@]}" -channel mono \
        "${geometry[@]}" \
        -o "fits/fit-$slug-series.json" -checkpoint "fits/fit-$slug-series.checkpoint" {{args}}

# Derive the measurement tables from recordings of a real drum.
#
# The capture protocol is docs/physical-measurement-protocol.md. Like
# fit-physical this needs files that are not in the repository, so it is not
# part of `just ci`; unlike fit-physical its output is meant to be committed.
measure-tom +files:
    go run ./cmd/measure-tom -o tom-measurements.json {{files}}

# Reduce a set of takes as an ordered *series* rather than as repeats.
#
# `measure-tom` alone summarises scatter across takes, which is the right
# reduction for repeats at one dynamic and the wrong one for a deliberate
# velocity ramp: there a clean monotone trend reads as enormous "spread". This
# adds the rank correlation of each measured quantity against the take index,
# and the cross-take partial correspondence table — which is what makes a
# partial that is present in some takes and absent in others visible at all.
#
# The take order is a claim, not a measurement (AGENTS.md). Correlating against
# it tests that claim; it never assumes it.
measure-series directory="reference/tt08x08/lp/hd":
    #!/usr/bin/env bash
    set -euo pipefail
    slug="$(echo "{{directory}}" | sed 's#^reference/##; s#/#-#g')"
    go run ./cmd/measure-tom -channel mono -series \
        -o "fits/measure-$slug-series.json" {{directory}}/v*.wav

# Compare two or more fit reports: do two runs of the search agree?
#
# cmd/measure-objective asks this of the objective and calls the disagreement
# the floor. This asks it of the search, which has the larger opportunity to
# disagree with itself — it is stochastic and it is stopped by hand. The headline
# is the rank correlation of the per-take fitted velocities: a quantity two runs
# cannot agree on is not a measurement, and nothing may be read off it.
#
# It refuses reports scored under different weight sets, because a total is a
# property of its weights and comparing across them is meaningless.
compare-fits +reports:
    go run ./cmd/compare-fits {{reports}}

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
