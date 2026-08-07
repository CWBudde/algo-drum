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
internal/drum/engine.go   — Sequencer: four rhythm banks (7×16 each), queued/chain switching, fixed-point tempo/swing clock, probability + centered per-cell humanize (allocation-free pending-trigger mask), smoothed per-track volumes, Render()
internal/drum/state.go    — Engine-owned semantic snapshot/replacement contract: full pattern, controls, engine-major mixer/mute and voice/Tom parameter banks
internal/drum/voices.go   — Drum synthesizer voices (BassDrum, Snare, HiHat, Tom, Cymbal, Tom 2, Percussion); all tuning is runtime-settable
internal/drum/params.go   — Per-voice synthesis parameter specs (ranges, curves, defaults) + the normalized→engineering mapping
internal/drum/validate.go — Engine.Validate(): every invariant Render relies on (step lengths, playhead, pending triggers, gains, voice params), joined into one error
internal/drum/protocol.go — ProtocolVersion: the semantics of the AlgoDrum JS API, pinned again in audioWorker.ts and asserted equal by protocol_test.go
internal/drum/assert.go   — assertValid(): a no-op in shipped builds; `-tags drumassert` (assert_debug.go) makes Render panic on a broken invariant (`just test-assert`)
cmd/gen-voiceparams/      — Generates web/src/engine/voiceParams.generated.ts from params.go (`just gen-params`; CI diffs it)
internal/drum/*_test.go   — Go unit tests: sequencing, clamping, bit-exact render determinism, per-voice envelopes
internal/drum/physical_tom.go — Adapter wrapping algo-tom's physical.DoubleHead as a Voice; the model itself lives in github.com/cwbudde/algo-tom → "The physical Tom voice"
web/src/engine/engineState.ts — Canonical semantic EngineState shape shared by the bridge, React reducer and persistence; includes four engine-major rhythm banks, per-cell velocity/probability/humanize/condition grids, per-track lengths and chain configuration
web/src/engine/wasmEngine.ts  — Main-thread bridge: spawns the worker, wires the worklet, sends commands, exposes authoritative configuration/transport snapshots and dispose()
web/src/engine/audioWorker.ts — Web Worker hosting the WASM engine; gates the load on the engine's protocol version and method list, renders audio chunks and echoes the complete authoritative state after every configuration edit
web/src/engine/stateMirror.ts — Reconciles full engine snapshots with in-flight optimistic UI edits (Go engine = single source of truth)
web/src/engine/voiceParams.ts   — Curve renderer + readout formatting over voiceParams.generated.ts (the committed mirror of internal/drum/params.go)
web/public/worklet.js         — AudioWorkletProcessor: consumes chunks, reports the audible engine transport snapshot
web/src/components/DrumMachine.tsx — Main UI shell over StepGrid/TrackStrip/Transport: continuous velocity, per-cell probability/conditions, polymetric lengths, Fill mode, voice editing and persistence/share wiring
web/src/components/AlgoPanel.tsx    — Algorithmic tools panel: preset selector, CLEAR, MUTATE, per-track Euclidean fill (E(k,n) + rotation), undo/redo and SHARE (copy link)
web/src/components/VoiceEditor.tsx — Per-voice synthesis editor: native <dialog> modal of knobs driven by the generated parameter table, plus AUDITION and RESET
web/src/components/Knob.tsx        — Reusable rotary knob (SVG; drag, wheel, and keyboard accessible)
web/src/components/knobMath.ts     — Pure knob math extracted from Knob.tsx: value↔angle, drag/wheel/key deltas (unit-tested without a DOM)
web/src/components/ErrorBoundary.tsx — App-wide React error boundary; a render crash shows a themed panel instead of a blank page
web/src/algo/euclid.ts     — Pure Bjorklund/Euclidean E(pulses, steps) rhythm generator with rotation
web/src/algo/mutate.ts     — Pure musical random-walk mutation of a flat pattern
web/src/algo/presets.ts    — Classic 16-step preset patterns (rock, house, breakbeat, hip-hop, techno, funk) + Clear
web/src/algo/persistence.ts — Pure versioned EngineState encode/decode → base64url (v16 preserves v15 as Bank A and appends per-cell humanize, Banks B–D and chain configuration; v1–v15 still decode); localStorage + URL-hash glue
web/src/algo/pattern.ts    — Shared pattern constants (dims, velocities, flat-index helper) for the algo modules
web/src/App.tsx           — Root: loads WASM on mount, renders DrumMachine inside the ErrorBoundary, shows a retryable fault panel if the engine fails
web/src/main.tsx          — Browser entry: mounts App and registers the service worker (production builds only)
web/src/**/*.test.ts      — Vitest unit tests, colocated with the pure modules they cover (algo/, knobMath, stateMirror, worker bridge)
web/e2e/smoke.spec.ts     — Playwright smoke test against the production build: engine ready, cell toggles, Space plays, playhead advances
web/public/sw.js          — Service worker: precaches the app shell + WASM, network-first for algo_drum.wasm / wasm_exec.js and navigations
web/public/site.webmanifest — PWA manifest (name, icons, standalone display, relative start_url/scope)
docs/voices.md            — Per-voice synthesis reference: recipes, named constants, master chain, determinism
PLAN.md                   — Point-in-time review backlog (numbered items); referenced from code comments and docs
```

### Audio Signal Flow

`Engine.Render(buf)` → Go voices mix mono samples → continuously advancing FDN reverb (smoothed wet/dry mix, so the knob does not raise the master level) → true lookahead peak limiter + safety clamp → `Float32Array` → 512-sample chunks posted from the Web Worker to the `AudioWorklet` over a direct `MessageChannel` → `AudioContext` at 48 kHz (~2048 samples buffered). Each chunk carries the engine-owned transport state, step and transition revision from its first sample; the worklet reports that snapshot when it becomes audible, so the UI rejects stale buffered epochs and follows the audible step.

The worklet pulls with a credit counter (four outstanding `need` requests), which is self-healing at both ends: the worker replies to **every** request — silence with a stopped transport snapshot if the engine is not ready or a render throws — and the worklet writes off credit that goes unanswered for ~133 ms and re-requests, so no dropped message can deadlock audio. Chunks also carry the engine's `isIdle()` state; once the output has been below −120 dBFS for 50 ms with the transport stopped, the main thread suspends the `AudioContext` (Play or an audition resumes it), and `Engine.Render` takes a zero-fill fast path instead of running seven voices, the reverb and the limiter forever. Underruns are counted in the worklet and reported to the main thread (`onUnderrun`); the queue target is fixed, so latency does not drift. Drained chunk buffers are transferred back to the worker and reused rather than reallocated ~94×/s.

### Track Order (index 0–6)

| Index | Voice           |
| ----- | --------------- |
| 0     | Bass Drum       |
| 1     | Snare           |
| 2     | Hi-Hat (closed) |
| 3     | Tom             |
| 4     | Cymbal          |
| 5     | Tom 2           |
| 6     | Percussion      |

UI displays Cymbal, Percussion, Tom 2, Tom, Hi-Hat, Snare, Bass from top to bottom.

### WASM JS API (`AlgoDrum` on the worker's global scope)

| Method                         | Description                                                                        |
| ------------------------------ | ---------------------------------------------------------------------------------- |
| `init(sampleRate)`             | Initialize engine (called once at WASM load)                                       |
| `setRunning(bool)`             | Play / stop (stop resets to step 0)                                                |
| `beginStart()`                 | Enter the non-advancing starting state while browser audio output resumes         |
| `pause()`                      | Freeze sequencer time and pending hits; active voice/effect tails keep ringing     |
| `setTempo(bpm)`                | Set tempo in BPM (clamped to 30–300)                                               |
| `setSwing(0–0.5)`              | Set swing amount (0.5 = full shuffle)                                              |
| `setStepCount(bank,n)`         | Set one bank's master length and reset that bank's track lengths (clamped 1–16)    |
| `setCell(bank,t,s,0–1)`        | Set cell velocity (0 = off; UI uses 0.7 = normal, 1.0 = accent)                    |
| `setCellProbability(b,t,s,p)`  | Set one cell's probability multiplier                                              |
| `setCellHumanize(b,t,s,h)`     | Set one cell's timing/velocity humanize multiplier                                 |
| `setCellCondition(b,t,s,0–6)`  | Set one cell's always/loop/fill/previous-step condition                            |
| `setTrackLength(bank,track,n)` | Set one track's independent loop length within a bank (clamped to 1–16)            |
| `setFillMode(bool)`            | Enable or disable cells carrying the fill-only condition                           |
| `setPattern(bank,Float32Array)` | Atomically replace one bank's flat track-major 7×16 velocity pattern              |
| `setPatternBank(bank,state)`   | Atomically replace a complete rhythm bank                                          |
| `requestBank(bank)`            | Select immediately while stopped or queue the bank for the next loop boundary      |
| `setChain(Uint8Array)`         | Set the stopped-only 1–16-entry A–D bank chain                                     |
| `setChainEnabled(bool)`        | Enable/disable chain playback while stopped                                        |
| `setState(EngineState)`        | Validate/clamp and replace every persistent sound/configuration value              |
| `getState()`                   | Return the complete authoritative semantic state snapshot                          |
| `setVolume(track, 0–1)`        | Set track volume (ramped over ~8 ms to avoid zipper noise)                         |
| `setDecay(track, 0–1)`         | Trim the track's base decay time by 0.5×–1.5×                                      |
| `setMuted(track, bool)`        | Ramp a track to/from silence without overwriting its stored volume                 |
| `setVoiceParam(track, i, 0–1)` | Set one per-voice synthesis parameter (tables in `docs/voices.md`)                 |
| `triggerVoice(track, 0–1)`     | Fire one voice immediately, independent of the sequencer (audition)                |
| `setReverb(0–1)`               | Set the smoothed global reverb amount                                               |
| `setProbability(0–1)`          | Global probability multiplier (1 = unchanged, default; 0 = silence)               |
| `setHumanize(0–1)`             | Master timing/velocity randomization (steady-state timing ±h·7.5 ms)               |
| `render(n)`                    | Render n samples → Float32Array                                                    |
| `currentStep()`                | Returns active/paused step index (-1 if stopped or starting)                       |
| `transportState()`             | Engine-owned `stopped` / `starting` / `playing` / `paused` state                   |
| `transportRevision()`          | Monotonic state-transition revision used to reject stale buffered chunks           |
| `activeBank()`                 | Runtime bank currently driving the sequencer                                       |
| `queuedBank()`                 | Pending standalone bank, or -1 when no manual switch is queued                     |
| `chainPosition()`              | Runtime chain entry, or -1 while chain mode is disabled                            |
| `isIdle()`                     | True once output has stayed below −120 dBFS for 50 ms and nothing can wake it      |
| `protocolVersion`              | Number (not a method): the API semantics this engine speaks (`internal/drum/protocol.go`) |

`protocolVersion` is the gate on everything above it, including the transport,
active/queued-bank and chain-position snapshot carried through `worklet.js`.
`algo_drum.wasm` and `worklet.js` are
unhashed artifacts that cache independently of the hashed JS bundle, so they
can drift in a user's browser; `REQUIRED_METHODS` in `audioWorker.ts` catches a
*missing* method, but only the version catches an existing one whose argument
order, units or `EngineState` shape changed — the failure mode otherwise being a
silently wrong-sounding engine. The worker refuses to load a mismatching engine
and the app shows its fault panel; the same version cache-busts the worklet URL.
Bump `drum.ProtocolVersion`, the worker's `PROTOCOL_VERSION` and the service
worker's worklet precache query together whenever an entry point or audio-chunk
message changes meaning; `TestProtocolVersionAgreesWithWorker` asserts all three
match.

## The physical Tom voice

An experimental double-headed physical Tom, selectable per Tom track and
independent of the procedural voices in `internal/drum`. **The model, the fitting
objective, the offline tooling, the reference recordings and the whole evidence
record now live in [github.com/cwbudde/algo-tom](https://github.com/cwbudde/algo-tom)**,
which this repository consumes as an ordinary module dependency.

What remains here is the adapter: `internal/drum/physical_tom.go` wraps
`physical.DoubleHead` as a `Voice`, and `internal/drum/params.go` binds the knob
table to `tomparams`.

**The knob→SI mapping is `tomparams.Config`, and it must not be copied.** The
constant-ζ retune rule, the DAMP/DEC/D.TILT composition and the resonant head's
reduced asymmetry are calibration decisions with their own evidence, and
algo-tom's fitter scores candidates through that same function. A local
reimplementation would mean every offline fit described an instrument this
repository does not ship. For the same reason `ParamSpec` is a **type alias** for
`tomparams.Spec` rather than a defined type: a defined type would fork `Map`, its
byte-step snap and its `Default` derivation, and a drift between the two copies
would silently retune the shipped sound with nothing but ears to catch it. The
alias is also what keeps `web/src/engine/voiceParams.generated.ts` byte-identical
across the extraction.

`decayScaleMin` is the one constant that genuinely exists in both repositories:
it defines what the persisted `setDecay` byte means for all seven procedural
voices, so it cannot leave `internal/drum`, and `tomparams.Config` needs the same
number to compose DEC with DAMP. `TestDecayScaleMinAgreesWithTomparams` asserts
they match rather than hoping.

**`TestPhysicalTomRenderIsBitExact` is the gate on all of this.** Two seconds at
the default bank, 48 kHz, velocity 1, hashed. It is the only assertion that hears
a change to the calibration — the config-comparison tests compare structs and the
level test tolerates a 0.25 window. Like every digest in this repository it is an
amd64 fact: a mismatch on js/wasm is `math.Exp`, whose FMA-accelerated
`exp_amd64.s` differs by one ULP, and a mismatch on the host is the calibration
having moved. algo-tom's AGENTS.md carries the full argument under "The portable
path is what ships".

`test-purego` and the `purego` CI job survive here as a **convention guard, not a
numerical one**: no architecture-gated code remains in this repository, so the
tag now selects nothing and only the build is run. algo-tom gates both halves.

Bumping the dependency is a change to the shipped sound until proven otherwise —
run the render digest, and regenerate `voiceParams.generated.ts` and confirm it
does not move.


## Key Dependencies

Versions below are the pinned ones in `go.mod` / `web/package.json` — check those files rather than trusting this list after a bump.

**Runtime**

- **Go 1.25** — toolchain for the engine (`go.mod`)
- **[algo-dsp](https://github.com/cwbudde/algo-dsp) `v0.0.0-20260729115219-8ea972cf5f07`** — compatibility commit for `algo-fft` v0.7.3; used for biquad filters in voices (`biquad.Section`, `design.Highpass`, `design.Bandpass`) and master effects (`reverb.FDNReverb`, `dynamics.Limiter`)
- **[algo-tom](https://github.com/cwbudde/algo-tom) v0.1.0** — the physical Tom: `physical` (the model) and `tomparams` (the knob→SI mapping). Everything else in that module — the fitting objective, the six offline commands, the reference recordings — is unreachable from this repository's non-test code and is dead-code-eliminated out of the browser build. `go tool nm -size web/public/algo_drum.wasm` finding any `mayfly`, `go-audio` or `algo-fft` symbol means that elimination failed across the module boundary. It is `v0.x` with nothing behind `internal/`, so treat every name it exports as movable
- **[algo-fft](https://github.com/cwbudde/algo-fft) v0.7.3** — reached only transitively, through `algo-dsp` and `algo-tom`'s offline half; not linked into the WASM build
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
