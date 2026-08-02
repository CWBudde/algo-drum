# AGENTS.md

This file provides guidance to ai agents (Claude Code, Codex etc.) when working with code in this repository.

## Project Overview

algo-drum is an algorithmic drum machine built with a **Go WASM audio engine** and a **React/TypeScript frontend**. The Go code compiles to WebAssembly and runs inside a **Web Worker**, where it registers an `AlgoDrum` API on the _worker's_ global scope (there is no `window.AlgoDrum` on the main thread). React renders the UI on the main thread and talks to the worker by message; rendered audio reaches the speakers through an `AudioWorklet`.

## Build Commands

### just — the primary dev interface

The `justfile` wraps every workflow below; `just` (or `just --list`) prints the
current recipe list. The ones that matter:

```bash
just build-wasm      # Go → web/public/algo_drum.wasm (+ wasm_exec.js)
just dev             # build-wasm, then the Vite dev server
just build           # build-wasm, then the production bundle → web/dist/
just preview         # build, then serve web/dist/ locally
just test            # Go test suite
just test-assert     # Go test suite with the engine self-check compiled into Render
just web-test        # frontend unit tests (Vitest)
just fmt             # format everything via treefmt (gofumpt, gci, shfmt, prettier)
just lint            # golangci-lint against the js/wasm target
just gen-params      # regenerate the TS mirror of the voice parameter table
just fix             # lint --fix, then fmt
just ci              # the full local gate — run this before pushing
```

`just ci` is meant to mirror `.github/workflows/ci.yml`; treat a green `just ci`
as the bar, and check the justfile rather than this list for its exact steps.

### WASM Engine (Go)

```bash
# Build the WASM binary + copy wasm_exec.js to web/public/
bash scripts/build-wasm.sh

# Verify the WASM build compiles
GOOS=js GOARCH=wasm go build ./cmd/wasm/
```

### Frontend (React/TypeScript)

```bash
cd web

bun install          # Install dependencies
bun run dev          # Dev server (Vite) — requires WASM built first
bun run build        # Type-check + production build → web/dist/
bun run preview      # Preview the production build
bun run lint         # ESLint
bun run test         # Vitest unit tests (web/src/**/*.test.ts)
bun run test:e2e     # Playwright smoke test (see below)
```

### Tests

There is a real test suite; run it before claiming a change works.

```bash
go test ./...        # Go engine + voice tests (host arch — cmd/wasm is js/wasm-only)

cd web
bun run test         # Vitest: the pure algo/, knobMath and patternMirror modules
bun run test:e2e     # Playwright: builds WASM + a production bundle, serves it on
                     # :4173, then drives the real app in headless Chromium
```

`bun run test:e2e` deliberately runs against the **production** build — worker
and worklet bundling differ between dev and prod. CI runs all three suites plus
type-check, lint and a treefmt check on every PR.

### Full Development Workflow

```bash
# Step 1: build WASM (output goes to web/public/)
bash scripts/build-wasm.sh

# Step 2: start frontend dev server
cd web && bun run dev
```

Vite serves `web/public/` as static assets, so `algo_drum.wasm` and `wasm_exec.js` are available at runtime. Vite's `base` is `/algo-drum/`, so the dev server serves the app at `http://localhost:5173/algo-drum/`.

## Architecture

```
cmd/wasm/main.go          — WASM entry point; registers the AlgoDrum JS API (worker global scope)
internal/drum/engine.go   — Sequencer: velocity pattern grid (5×16), runtime step count, tempo/swing, probability + humanize (allocation-free pending-trigger list), smoothed per-track volumes, Render()
internal/drum/voices.go   — Drum synthesizer voices (BassDrum, Snare, HiHat, Tom, Cymbal); all tuning is runtime-settable
internal/drum/params.go   — Per-voice synthesis parameter specs (ranges, curves, defaults) + the normalized→engineering mapping
internal/drum/validate.go — Engine.Validate(): every invariant Render relies on (step lengths, playhead, pending triggers, gains, voice params), joined into one error
internal/drum/assert.go   — assertValid(): a no-op in shipped builds; `-tags drumassert` (assert_debug.go) makes Render panic on a broken invariant (`just test-assert`)
cmd/gen-voiceparams/      — Generates web/src/engine/voiceParams.generated.ts from params.go (`just gen-params`; CI diffs it)
internal/drum/*_test.go   — Go unit tests: sequencing, clamping, bit-exact render determinism, per-voice envelopes
internal/physical/        — The experimental double-headed physical Tom, selectable per Tom track and independent of the procedural voices → "The physical drum model"
internal/physical/analysis/ — Offline report/suite generation for `cmd/analyze-physical`; backs testdata/physical-reference-v2.json
internal/physical/match/  — Feature extraction and the nine-term perceptual distance a fit is scored with → "The physical drum model"
cmd/measure-objective/    — Measures the objective's own reproducibility floor and proposes the gates `match.DefaultWeights` inverts
cmd/fit-physical/         — Fits the model's parameter bank to one or more recordings, with the Mayfly Optimization Algorithm
cmd/measure-tom/          — Turns recordings into the committable tables docs/physical-measurement-protocol.md asks for
cmd/analyze-physical/     — Emits the analysis report and regenerates the reference fixture (`just gen-physical-reference`; CI diffs it)
cmd/render-physical/      — Renders the physical Tom to a WAV for offline auditioning
internal/wavio/           — Mono 16-bit PCM WAV export, shared by cmd/render-physical and cmd/fit-physical's `-wav` (reading WAVs lives in internal/physical/match, which drags in the whole FFT stack)
docs/physical-*.md        — The physical model's design and evidence record → "The physical drum model"
reference/CREDITS.md      — Licence and provenance for the committed reference recordings (CC BY 4.0)
docs/paper/               — Typst working paper on the matching method (`just paper` → physical-tom-matching.pdf); figures/ holds the committed PNGs
tools/paper-figures/      — Draws the paper's figures from a `cmd/fit-physical -o` report (`just paper-figures`). Its own Go module, and deliberately: matplotlib-go's graphics tree and its FreeType-linking raster backend have no business in the engine's go.mod, and `go mod tidy` ignores the `purego` build tag the pure-Go rasteriser needs
web/src/engine/wasmEngine.ts  — Main-thread bridge: spawns the worker, wires the worklet, sends commands, exposes onPattern (engine-owned pattern snapshots) and dispose() (tears the worker + audio graph down)
web/src/engine/audioWorker.ts — Web Worker hosting the WASM engine; renders audio chunks on demand, echoes the authoritative pattern after each edit
web/src/engine/patternMirror.ts — Reconciles the engine's pattern echoes with in-flight optimistic UI edits (engine = single source of truth)
web/src/engine/voiceParams.ts   — Curve renderer + readout formatting over voiceParams.generated.ts (the committed mirror of internal/drum/params.go)
web/public/worklet.js         — AudioWorkletProcessor: consumes chunks, reports the audible step
web/src/components/DrumMachine.tsx — Main UI: 5×16 step grid (DOM/CSS; clicking a cell cycles off → on → accent) mirroring the engine-owned pattern, transport (play, tempo + TAP, swing, STEPS, PROB, HUMAN, reverb), per-track volume/decay knobs + mute LEDs + a per-voice editor button; persistence/share wiring
web/src/components/AlgoPanel.tsx    — Algorithmic tools panel: preset selector, CLEAR, MUTATE, per-track Euclidean fill (E(k,n) + rotation), SHARE (copy link)
web/src/components/VoiceEditor.tsx — Per-voice synthesis editor: native <dialog> modal of knobs driven by the generated parameter table, plus AUDITION and RESET
web/src/components/Knob.tsx        — Reusable rotary knob (SVG; drag, wheel, and keyboard accessible)
web/src/components/knobMath.ts     — Pure knob math extracted from Knob.tsx: value↔angle, drag/wheel/key deltas (unit-tested without a DOM)
web/src/components/ErrorBoundary.tsx — App-wide React error boundary; a render crash shows a themed panel instead of a blank page
web/src/algo/euclid.ts     — Pure Bjorklund/Euclidean E(pulses, steps) rhythm generator with rotation
web/src/algo/mutate.ts     — Pure musical random-walk mutation of a flat pattern
web/src/algo/presets.ts    — Classic 16-step preset patterns (rock, house, breakbeat, hip-hop, techno, funk) + Clear
web/src/algo/persistence.ts — Pure versioned encode/decode of full state → base64url (v2 appends the voice parameters; v1 blobs still decode); localStorage + URL-hash glue
web/src/algo/pattern.ts    — Shared pattern constants (dims, velocities, flat-index helper) for the algo modules
web/src/App.tsx           — Root: loads WASM on mount, renders DrumMachine inside the ErrorBoundary, shows a retryable fault panel if the engine fails
web/src/main.tsx          — Browser entry: mounts App and registers the service worker (production builds only)
web/src/**/*.test.ts      — Vitest unit tests, colocated with the pure modules they cover (algo/, knobMath, patternMirror)
web/e2e/smoke.spec.ts     — Playwright smoke test against the production build: engine ready, cell toggles, Space plays, playhead advances
web/public/sw.js          — Service worker: precaches the app shell + WASM, network-first for algo_drum.wasm / wasm_exec.js and navigations
web/public/site.webmanifest — PWA manifest (name, icons, standalone display, relative start_url/scope)
docs/voices.md            — Per-voice synthesis reference: recipes, named constants, master chain, determinism
PLAN.md                   — Point-in-time review backlog (numbered items); referenced from code comments and docs
```

### Audio Signal Flow

`Engine.Render(buf)` → Go voices mix mono samples → FDN reverb (wet amount) → lookahead limiter + hard clamp → `Float32Array` → 512-sample chunks posted from the Web Worker to the `AudioWorklet` over a direct `MessageChannel` → `AudioContext` at 48 kHz (~2048 samples buffered). Each chunk carries the sequencer step it starts on; the worklet reports it back so the UI playhead tracks the audible step.

### Track Order (index 0–4)

| Index | Voice           |
| ----- | --------------- |
| 0     | Bass Drum       |
| 1     | Snare           |
| 2     | Hi-Hat (closed) |
| 3     | Tom             |
| 4     | Cymbal          |

UI displays tracks in **reverse order** (Cymbal on top, Bass on bottom).

### WASM JS API (`AlgoDrum` on the worker's global scope)

| Method                         | Description                                                                        |
| ------------------------------ | ---------------------------------------------------------------------------------- |
| `init(sampleRate)`             | Initialize engine (called once at WASM load)                                       |
| `setRunning(bool)`             | Play / stop (stop resets to step 0)                                                |
| `setTempo(bpm)`                | Set tempo in BPM (clamped to 30–300)                                               |
| `setSwing(0–0.5)`              | Set swing amount (0.5 = full shuffle)                                              |
| `setStepCount(n)`              | Set active pattern length (clamped to 1–16); steps are 16th notes                  |
| `setCell(track, step, 0–1)`    | Set cell velocity (0 = off; UI uses 0.7 = normal, 1.0 = accent)                    |
| `setPattern(Float32Array)`     | Replace pattern from a flat track-major array of 5×16 velocities (`track*16+step`) |
| `getPattern()`                 | Returns the pattern in the same flat Float32Array layout                           |
| `setVolume(track, 0–1)`        | Set track volume (ramped over ~8 ms to avoid zipper noise)                         |
| `setDecay(track, 0–1)`         | Trim the track's base decay time by 0.5×–1.5×                                      |
| `setVoiceParam(track, i, 0–1)` | Set one per-voice synthesis parameter (tables in `docs/voices.md`)                 |
| `triggerVoice(track, 0–1)`     | Fire one voice immediately, independent of the sequencer (audition)                |
| `setReverb(0–1)`               | Set global reverb amount                                                           |
| `setProbability(0–1)`          | Per-hit trigger chance (1 = every hit fires, default; 0 = silence)                 |
| `setHumanize(0–1)`             | Timing/velocity randomization (delay ≤ h·15 ms, velocity ±h·20%; 0 = mechanical)   |
| `render(n)`                    | Render n samples → Float32Array                                                    |
| `currentStep()`                | Returns active step index (-1 if stopped)                                          |

## The physical drum model

An experimental double-headed physical Tom, selectable per Tom track and independent of
the procedural voices in `internal/drum`. It is a research line with its own tooling,
its own measurement discipline, and its own evidence record; the sections below are the
parts a change to it has to respect.

### The model (`internal/physical/`)

Modal banks per head (`modes.go`), the two-head + lumped-cavity + Berger-tension
real-time model (`double_head.go`), the P2 linear single-head reference
(`single_head.go`), the three-band stochastic attack layer that covers what modal
synthesis cannot reach, decaying at rates read off the head's own loss law
(`attack.go`), a versioned SI-valued config with a migration chain (`config.go`), and an
offline continuous-time reference solve (`frequency_response.go`).

The elementwise half of the midpoint solve is factored into `midpoint.go` with an AVX2
kernel beside it (`midpoint_amd64.{go,s}`, `!purego`) and a portable fallback
(`midpoint_noasm.go`). The shipped voice is js/wasm and always takes the fallback, so
the assembly only runs in the offline tools. **The kernel is bit-exact against the Go
reference, and that is a requirement rather than a courtesy:** the calibration fixture
and the rendered-WAV digest both compare exactly, so no FMA and no reassociation
(`midpoint_exact_test.go` pins it).

### The objective (`internal/physical/match/`)

What a fit is scored with, and the instrument that judges that instrument.

- `features.go` — the fast estimator `cmd/fit-physical` runs per candidate (FFT peak
  picking, heterodyned envelopes).
- `decay.go` — fits each partial's ring time as an exponential standing on a stationary
  noise floor (Karjalainen et al., JAES 50(11), 2002) rather than a straight line
  through a truncated trace. `slowestSupportedT60` carries the admissibility criterion:
  **evidence is the fall, not the duration** — ISO 3382's ≥20 dB inside the fit window.
- `distance.go` — the nine-term perceptual distance, aggregated as a trimmed RMS and
  weighted by reciprocals of measured reproducibility gates (`AdoptionGates`). A term at
  its gate contributes exactly 1.0, so **a total is a property of a weight set** and
  totals from different gate sets are not comparable.
- `esprit.go` / `linalg.go` — subband ESPRIT with a stabilisation sweep, deliberately
  **not** in any fit loop: high resolution, seconds per extraction, used to establish
  what the fast estimator is getting wrong. `linalg.go` holds the dense complex
  eigen/least-squares routines it needs, which exist because no linear-algebra
  dependency belongs in a module compiled for js/wasm.

### The tools

| Tool | What it is for |
| ---- | -------------- |
| `cmd/measure-tom/` | Turns recordings into the committable tables [docs/physical-measurement-protocol.md](docs/physical-measurement-protocol.md) asks for, through the same code a fit scores with. `-high-resolution` adds the ESPRIT table and the partial-by-partial agreement between the two estimators (PLAN.md §N2) |
| `cmd/measure-objective/` | Measures the objective's reproducibility floor and proposes the gates `DefaultWeights` inverts. It scores each channel of a **coincident** stereo pair against the other through `match.Distance` itself, so the floor is measured by the shipped code rather than a reimplementation of it, and it refuses a spaced pair, where the disagreement would be two arrival times |
| `cmd/fit-physical/` | Searches the parameter bank for the drum closest to a recording. `-inspect` reads a running checkpoint and stops, emitting the full report — per-term breakdown, per-take terms and velocities, parameter table — without disturbing the run |
| `cmd/analyze-physical/` | The analysis report and the reference fixture (`just gen-physical-reference`; CI diffs it) |
| `cmd/render-physical/` | Renders the voice to a WAV for offline auditioning |

**Gates are hand-edited.** `measure-objective` proposes; a human applies. There is
deliberately no generated or CI-diffed gate fixture — a gate is a judgement about what
counts as agreement, and it should cost a person a decision.

### Fitting discipline

`-reference` is repeatable, and every take given is fitted by **one shared bank** with
**one free strike velocity each**, scored as the mean of the per-take distances
(`just fit-physical-series <directory>`). A single-file run is a diagnostic, since with
one recording the contact parameters and the Berger nonlinearity trade freely against an
assumed strike. `-set` pins a parameter in its own unit (metres, N/m) where `-fix` takes
a normalized position; the geometry of a committed pack is pinned this way, read off
`reference/<WxH>/`.

**Anything a fit produces is named after the reference it was made against.**
`just fit-physical` derives `fits/fit-tt08x08-lp-hd-v08.{json,checkpoint}` from the path
and `just fit-physical-series` derives `fits/fit-tt08x08-lp-hd-series.{json,checkpoint}`
from the directory; a hand-run `-o` must carry at least the drum/tuning/style class. A
fit report is only meaningful beside the recording it targeted — the gates, the totals
and the whole partial table are properties of that drum at that tuning, and they do not
transfer between sets.

Parallelism is one goroutine per restart and nothing below it: the takes, the feature
extraction and the FFTs inside one evaluation are sequential, so `-restarts N` occupies
N cores and no more. Throughput is memory-bandwidth bound rather than core bound — on a
12-core machine 1/4/11 restarts measured 1.0×/1.8×/2.9× aggregate.

### What the takes do and do not tell you

The reference series was played by hand with velocity in mind, which means **velocity is
not the only thing that varied between strikes** — position, mallet angle and contact
obliquity moved too, and the fit has nowhere to put them.

- **The file order is never read as evidence.** The v01…v16 labelling is a claim, so
  reversing the take list leaves the cost bit-identical.
- **The fitted per-take velocities are not a measurement of anything**, and were once
  described here as an independent measurement of the file order. Two searches over the
  same sixteen takes (5,002 and 8,976 evaluations, totals 15.835 and 15.186) returned
  velocity vectors correlating **ρ = +0.15 with each other**, −0.07 and −0.05 with the
  file index, and −0.22 and −0.18 with the takes' own attack brightness. Velocity is a
  nuisance parameter this objective does not identify, so the search fills those sixteen
  dimensions with noise.
- **What does measure the order is the recordings alone.** Crest factor over the first
  50 ms runs **ρ = +0.92** against the file index and attack balance **ρ = +0.85**, both
  gain-invariant and so unharmed by the per-file peak normalisation that leaves absolute
  level at ρ = +0.16. Read a velocity ramp off these, never off a fit.
- **One shared bank is structurally wrong across the series.** On `tt08x08/lp/hd` the
  358 Hz partial's level relative to the fundamental scatters over 14.8 dB (sd 3.5 dB),
  and a partial at 255.7 Hz is absent from all nine of v01–v09 and present in six of the
  seven of v10–v16 at a consistent height (sd 0.7 dB). A mode that switches on above a
  strike level is a nonlinear signature, not a positional one — so the loud takes are
  not the quiet takes scaled up, and a single linear bank fitted to both must compromise
  in exactly the spectrum and level terms that dominate the residual.

### The design record

Read [docs/physical-objective-validation.md](docs/physical-objective-validation.md)
before quoting any fit number or adoption gate — it is the evidence record for how far
the objective can be trusted, and it carries the results that have been superseded as
well as the ones that stand.

| Document | Subject |
| -------- | ------- |
| [physical-model-research.md](docs/physical-model-research.md) | The literature the model is built from |
| [physical-calibration.md](docs/physical-calibration.md) | Calibration and the microphone model |
| [physical-cavity.md](docs/physical-cavity.md) | The lumped cavity and the two-head coupling |
| [physical-nonlinearity.md](docs/physical-nonlinearity.md) | Berger tension, the tanh cap, discrete-gradient passivity |
| [physical-contact.md](docs/physical-contact.md) | The strike: prescribed half-sine and Hunt–Crossley |
| [physical-excitation-gap.md](docs/physical-excitation-gap.md) | What the excitation could not reach |
| [physical-hybrid.md](docs/physical-hybrid.md) | Why the attack layer exists; quality tiers |
| [physical-product-integration.md](docs/physical-product-integration.md) | How the voice reaches the product |
| [physical-measurement-protocol.md](docs/physical-measurement-protocol.md) | How a recording becomes a committable table |
| [physical-objective-validation.md](docs/physical-objective-validation.md) | How far the fitting objective can be trusted |
| [physical-measured-fit.md](docs/physical-measured-fit.md) | The measured fit results and their gates |
| [physical-tom-review.md](docs/physical-tom-review.md) | The measured review the P8 work came from |
| [physical-sound-audit.md](docs/physical-sound-audit.md) | Listening audit of the rendered voice |
| [physical-real-instrument-departures.md](docs/physical-real-instrument-departures.md) | What the model is not, and the gate for adding it |
| [reference/CREDITS.md](reference/CREDITS.md) | Licence and provenance (CC BY 4.0). Recordings are laid out `reference/<drum>/<tuning>/<style>/v<NN>.wav`; `reference/` is otherwise gitignored, and only the 8"x8" low-pitch head-strike subset (`tt08x08/lp/hd`) is tracked |

## Key Dependencies

Versions below are the pinned ones in `go.mod` / `web/package.json` — check those files rather than trusting this list after a bump.

**Runtime**

- **Go 1.25** — toolchain for the engine (`go.mod`)
- **[algo-dsp](https://github.com/cwbudde/algo-dsp) `v0.0.0-20260729115219-8ea972cf5f07`** — compatibility commit for `algo-fft` v0.7.3; used for biquad filters in voices (`biquad.Section`, `design.Highpass`, `design.Bandpass`), master effects (`reverb.FDNReverb`, `dynamics.Limiter`), and physical-analysis metrics
- **[algo-fft](https://github.com/cwbudde/algo-fft) v0.7.3** — FFT backend used directly by the physical-analysis tooling and transitively by `algo-dsp`
- **React 19.2.8** (`react` + `react-dom`) — the only runtime npm dependencies; everything else is a devDependency

**Build & tooling**

- **bun** — package manager and script runner for the frontend
- **Vite 8.1.5** — frontend bundler; configured with `@vitejs/plugin-react` 6.0.5
- **TypeScript 7.0.2 and 6.0.3, side by side** — the native TS 7 compiler is installed under the
  alias `@typescript/native`, and type-checking runs it explicitly (`bun run typecheck` →
  `node node_modules/@typescript/native/bin/tsc --noEmit`) rather than via `node_modules/.bin/tsc`,
  which both packages claim. The bare `typescript` name resolves to 6.0.3 because
  `typescript-eslint` imports the TS 6 compiler API and hard-fails on TS ≥ 7
  ([typescript-eslint#10940](https://github.com/typescript-eslint/typescript-eslint/issues/10940)).
  Microsoft's recommended `@typescript/typescript6` shim does not work under bun — its internal
  `@typescript/old": "npm:typescript@^6"` alias resolves back to the shim itself, so
  `require('typescript')` yields an empty object. Drop the alias split once typescript-eslint
  supports TS 7.
- **Vitest 4.1.10** — unit tests (`bun run test`, config in `web/vitest.config.ts`)
- **Playwright 1.62.0** (`@playwright/test`) — e2e smoke test (`bun run test:e2e`, config in `web/playwright.config.ts`)
- **ESLint 10.8.0** with `typescript-eslint` 8.65.0 — frontend linting (`bun run lint`, flat config in `web/eslint.config.js`)
- **treefmt** — multi-language formatter runner (`treefmt.toml`: gofumpt, gci, shellcheck, shfmt, prettier); note it deliberately excludes `AGENTS.md` and `PLAN.md`
- **golangci-lint** (v2 config schema, `.golangci.yml`) — Go linting; always run against the `js/wasm` target, since `cmd/wasm/main.go` is invisible on the host GOOS

## Deployment

GitHub Actions (`.github/workflows/deploy.yml`) builds WASM + frontend and deploys `web/dist/` to GitHub Pages on every push to `main`. The Vite build must use correct `base` URL for asset paths in production.
