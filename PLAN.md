# algo-drum — Repository Review & Improvement Plan

Reviewed: 2026-07-09 · Scope: entire repository at `main` (commit `00f7727`).

> The original implementation plan (`docs/plans/2026-02-25-algo-drum-impl.md`, 11 tasks)
> was fully completed and has been removed from the tree — nothing from it is carried
> over. This document is a fresh review; every item below is **open** unless checked.

## Scorecard

| # | Category | Score | Verdict |
| - | -------- | :---: | ------- |
| 1 | Correctness & robustness | **3/10** | Several real bugs, incl. a showstopper: pattern programmed before first Play is silently lost |
| 2 | Audio pipeline & WASM bridge | **3/10** | Deprecated `ScriptProcessorNode`, per-sample JS boundary copies, ~85 ms latency, per-buffer allocations |
| 3 | Go engine / DSP | **6/10** | Clean, readable voices; missing input validation, velocity, and any tests |
| 4 | Architecture & state management | **5/10** | Sensible layering, but UI and engine each hold pattern state with no sync-on-init |
| 5 | Frontend code quality | **5/10** | Typechecks strict-clean, but 640-line canvas component, inline styles everywhere, `tsc -b` misconfigured |
| 6 | UI / UX | **5/10** | Attractive skeuomorphic look, but blurry on hi-DPI, fragile %-overlay layout, constant 60 fps repaint |
| 7 | Accessibility | **1/10** | Canvas grid is invisible to assistive tech; no keyboard path at all |
| 8 | Testing | **0/10** | Zero tests of any kind (Go, unit, e2e) |
| 9 | CI/CD & tooling | **4/10** | Deploy-only workflow; `just ci` exists but nothing runs it; golangci config mixes v1/v2 keys |
| 10 | Repo hygiene | **4/10** | Compiled `.js` + `.tsbuildinfo` committed next to sources; **no LICENSE** |
| 11 | Documentation | **5/10** | Good README skeleton, but AGENTS.md is stale in ≥4 places |
| 12 | PWA & deployment | **5/10** | Works, but SW cache version never bumps → one-visit-stale `.wasm`; COOP/COEP only in dev |
| 13 | Feature depth vs. the name | **3/10** | "algo-drum" contains no algorithmic features — it's a plain 5×8 step sequencer |

**Overall: 4/10** — a nice start with a solid skeleton, held back by correctness bugs,
a deprecated audio path, zero tests, and a UI approach (canvas) that fights the web platform.

---

## 1. Correctness & robustness — 3/10

- [x] **C1 (critical): state programmed before first Play is lost.**
      `AlgoDrum.init` only runs inside `startAudio()` on the first Play click
      (`web/src/engine/wasmEngine.ts:41`). Until then `engine == nil` in Go, so every
      `setCell` / `setVolume` / `setTempo` / `setSwing` / `setDecay` / `setReverb` call is
      silently dropped (`cmd/wasm/main.go` guards). A user who programs a pattern and then
      hits Play hears *silence* while the UI shows active cells.
      **Fix:** create the engine at WASM load (sample rate is forced to 48 kHz anyway), or
      re-sync the complete UI state (pattern, volumes, decays, tempo, swing, reverb) right
      after `init`.
- [x] **C2: swing is double-scaled.** The UI sends `swing * 0.5` (`DrumMachine.tsx:372`)
      and the engine scales by another 0.5 (`engine.go:72`), so max shuffle is ±12.5%
      instead of the documented "0.5 = full shuffle". Pick one scaling point.
- [x] **C3: `SetTempo` accepts any value.** `bpm <= 0` → division by zero in
      `recomputeStepLengths` → `Inf` → bogus `int64` step lengths; the sequencer wedges.
      Clamp to a sane range (e.g. 30–300) like `SetDecay` already does.
- [x] **C4: `SetVolume` doesn't clamp** (negative or huge gains pass straight into the mix);
      `SetSwing` doesn't clamp either. Mirror the `SetDecay` clamping.
- [x] **C5: default mismatch** — UI volume knobs start at 0.75, engine volumes at 1.0
      (and decay 0.5 both sides only by luck). Currently masked by C1; after fixing C1,
      define one source of truth for initial values.
- [x] **C6: `AudioContext` is never `resume()`d.** On iOS Safari and autoplay-restricted
      browsers the context can be created `suspended` → permanent silence. `await ctx.resume()`
      in the Play handler; also handle the page being backgrounded.
- [x] **C7: duplicate/unstable SVG ids in `Knob`.** The gradient id is derived from the
      label: all five "DEC" knobs collide (duplicate DOM ids), and the tempo knob's id
      changes on every BPM change because the label embeds the value. Use React `useId()`.
- [x] **C8: step indicator leads the audio.** `currentStep()` reflects the render-ahead
      position, not what's audible — with a 4096-sample buffer the LED runs up to ~85 ms +
      output latency ahead. Track step-to-time mapping in JS (or return timestamps) and
      display against `ctx.currentTime`.
- [ ] **C9: dead code** — `HiHat`'s open mode (`closed=false`) is never used; `age` is
      unused in the noise voices (`Snare`, `HiHat`, `Cymbal`). Remove or wire up
      (an open-hat track is a natural 6th voice).

## 2. Audio pipeline & WASM bridge — 3/10

- [x] **B1: replace `ScriptProcessorNode` with an `AudioWorklet`.** It has been deprecated
      for years and runs audio on the *main* thread — the same thread doing full-canvas
      60 fps repaints (§6), which is a recipe for glitches. Target design: WASM instance
      inside the worklet (or a Worker feeding a ring buffer); the main thread only sends
      control messages.
- [x] **B2: per-sample boundary copies.** `render` in `cmd/wasm/main.go:96` calls
      `arr.SetIndex` 4096 times per buffer. Use `js.CopyBytesToJS` over the float32
      buffer's byte view — one copy instead of 4096 calls.
- [x] **B3: allocation churn** — a new Go slice *and* a new `Float32Array` per render
      call (~12/s). Allocate once at init and reuse.
- [x] **B4: latency** — fixed 4096-sample buffer ≈ 85 ms. With an AudioWorklet the
      quantum is 128 samples; until then, make buffer size configurable and smaller.
- [ ] **B5: no error propagation** from Go to JS (silent `nil` returns when args are
      missing/engine is nil) and errors from `SetWet`/`SetRT60`/`SetThreshold` are
      discarded (`engine.go:56–62`, `133–142`). Log once or surface them.
- [x] **B6: no `instantiateStreaming` fallback** for servers that mis-serve
      `application/wasm` (fine on Pages, breaks on naive static hosts). Add the standard
      `arrayBuffer()` fallback.

## 3. Go engine / DSP — 6/10

- [ ] **E1: no velocity/accent.** Every hit is full-strength. Per-step velocity (even
      just accent on/off) transforms how the machine feels.
- [ ] **E2: fixed 5 tracks × 8 steps.** 16 steps is the genre standard; make
      `StepCount` a runtime pattern length (1–16) rather than a compile-time constant.
- [ ] **E3: bulk pattern API.** Only per-cell `setCell` exists. Add `setPattern`/
      `getPattern` (bit-packed) so the UI can re-sync state cheaply (needed by C1) and
      presets/persistence become trivial (§7).
- [ ] **E4: migrate `math/rand` → `math/rand/v2`** (current API, faster). Keeping fixed
      per-voice seeds for reproducibility is fine.
- [ ] **E5: voice parameters are hardcoded** (pitch sweep, filter freqs). Expose tune/
      snap per voice later; at minimum lift magic numbers into named consts.
- [ ] **E6: reverb tail cutoff on stop** — `SetRunning(false)` stops triggers but voices
      and reverb keep ringing (good); however Stop also resets to step 0, so there is no
      pause. Consider separate stop vs. pause semantics.
- [x] **E7: master output clips — the limiter isn't limiting.** Measured at runtime
      (analyser tapped on the ScriptProcessor output): peak **1.86** with an ordinary
      bass/snare/hat pattern, despite `Limiter` at −0.1 dB threshold — samples beyond ±1.0
      are hard-clipped by the browser at the destination. Likely attack overshoot without
      lookahead, or a threshold/units mismatch in the algo-dsp limiter usage. Investigate,
      and until fixed add headroom (scale the voice mix down ~6 dB).

## 4. Architecture & state management — 5/10

- [ ] **A1: single source of truth for the pattern.** Today React state and the Go engine
      each hold a copy with no reconciliation (root cause of C1). Decide: engine owns
      state, UI mirrors it (recommended), with a full-state sync on init/reset.
- [x] **A2: typed message layer** — replace the loose `window.AlgoDrum` global surface
      with a small versioned command interface (also what an AudioWorklet port needs, B1).
- [ ] **A3: parameter smoothing** — volume/decay changes apply instantly per-sample; add
      short ramps to avoid zipper noise when twisting knobs during playback.

## 5. Frontend code quality — 5/10

- [x] **F1: fix the build script.** `"build": "tsc -b && vite build"` with a tsconfig
      lacking `noEmit` is what emitted `.js` files *next to the sources* (now committed,
      see H1). Set `"noEmit": true` and use `tsc --noEmit` (or `-b` with proper project
      refs) — Vite does the transpiling.
- [x] **F2: split `DrumMachine.tsx`** (640 lines: painting helpers + layout math + state +
      controls). Extract drawing, hit-testing, and control panels into modules — or make
      it moot via the DOM rewrite (§6).
- [x] **F3: move styling out of inline objects** — every component styles via large
      inline `style` props; introduce CSS modules (or plain CSS custom properties for the
      panel theme).
- [ ] **F4: add ESLint** (typescript-eslint + react-hooks) — currently only prettier via
      treefmt; hook rules would have flagged several issues here.
- [ ] **F5: error handling** — no React error boundary; the WASM load error renders raw
      `String(e)`. Add a boundary and a friendly retry UI.
- [ ] **F6: upgrade React 18 → 19** (already on Vite 7 / TS 5.6; low risk at this size).

## 6. UI / UX — 5/10 (rewrite recommended — see plan below)

The canvas approach is the root of most UI problems: blurry rendering, fragile overlays,
zero accessibility, constant repaints.

- [x] **U1: canvas is not DPR-aware** — fixed 1020×700 backing store CSS-scaled up ⇒
      visibly blurry on any hi-DPI display.
- [x] **U2: full-scene redraw at 60 fps forever**, even when idle/stopped — wasted CPU
      and battery, on the same thread as audio (B1).
- [x] **U3: HTML controls absolutely positioned by magic percentages over the canvas** —
      already caused two "fix positioning" commits; breaks whenever geometry changes.
- [x] **U4: knob interaction is drag-only** — add mouse wheel, double-click-to-default,
      and fine-adjust (shift-drag); show the value while dragging.
- [x] **U5: no keyboard shortcuts** — at minimum Space = play/stop.
- [x] **U6: no visual feedback for "loading" beyond a text swap**; the machine renders
      dead-looking until WASM arrives.

### UI rewrite plan (replace canvas with DOM/CSS)

- [x] Rebuild the sequencer as a CSS-grid of real `<button>` cells: free hit-testing,
      focus, hover, keyboard, ARIA; LEDs via `box-shadow` glow; the skeuomorphic panel
      (bevels, grooves, brushed gradients) is straightforward CSS.
- [x] Only the playhead column changes per step — drive it with a CSS class toggle from a
      lightweight rAF (or `requestAnimationFrame` only while playing), not full repaints.
- [x] Keep the SVG `Knob` (it's good), fixing C7/U4 and adding `role="slider"` +
      `aria-valuenow` + arrow-key support.
- [ ] 16-step grid with 4-step bar shading (depends on E2); per-track row = mute LED,
      name, cells, volume, decay — no more overlay math.
- [x] Responsive: grid scales via container queries; controls wrap below on narrow
      screens (manifest currently claims `portrait`, see P4).
- [x] `prefers-reduced-motion` support for LED pulse/glow animations.

## 7. Feature depth — 3/10 ("algo" is missing from algo-drum)

- [ ] **G1: Euclidean rhythm generator** per track (classic, cheap, immediately
      "algorithmic": E(3,8), E(5,16)…).
- [ ] **G2: per-step probability / humanize** (chance a hit fires, timing jitter).
- [ ] **G3: pattern mutate / evolve button** (small random walk on the current pattern).
- [ ] **G4: preset patterns** (rock, house, breakbeat…) + Clear button.
- [ ] **G5: persistence** — save pattern+params to `localStorage`; encode in the URL
      hash for shareable links.
- [ ] **G6: tap tempo.**
- [ ] **G7 (later): accent row, open hi-hat track (C9), master volume.**

## 8. Accessibility — 1/10

- [x] **X1: the entire sequencer is a `<canvas>`** — no semantics, no focus, no screen
      reader access, no keyboard operation. Fixed structurally by the §6 rewrite.
- [x] **X2: mute button is an 11×11 px target** with color-only state — enlarge to ≥24 px
      hit area and add `aria-pressed` + label.
- [ ] **X3: low-contrast labels** (e.g. `rgba(195,185,165,0.60)` 9 px text) — check
      WCAG AA and bump sizes/contrast.
- [x] **X4: knobs need `role="slider"`, ARIA values, and keyboard** (see rewrite plan).

## 9. Testing — 0/10

- [x] **T1: Go unit tests** for the engine: step timing math (incl. swing sums to
      constant bar length), tempo/decay/volume clamping (C3/C4), trigger-on-step
      behavior, stop-resets-to-zero.
- [x] **T2: Go voice tests**: envelopes decay monotonically to inactive, no NaN/Inf
      output, peak levels bounded, deterministic with fixed seeds.
- [x] **T3: golden render test** — render N buffers of a known pattern, assert RMS/peak
      per step window (guards DSP regressions without brittle sample equality).
- [ ] **T4: frontend unit tests (Vitest)** — Knob value/angle math, drag delta logic,
      pattern state reducers.
- [ ] **T5: e2e smoke (Playwright)** — page loads, WASM initializes, Play toggles,
      toggling a cell before Play still produces audible output (regression test for C1;
      assert via `OfflineAudioContext` or engine state).
- [x] **T6: run T1–T4 in CI** (see CI1). Note: engine tests need a non-WASM build —
      `internal/drum` is pure Go, so plain `go test ./internal/...` works.

## 10. CI/CD & tooling — 4/10

- [x] **CI1: add a CI workflow** (PRs + pushes) running `just ci` — format check, lint,
      `go mod tidy` check, typecheck — plus `go test` and a WASM compile check. Today the
      only workflow is deploy-on-main; nothing gates a PR.
- [x] **CI2: fix `.golangci.yml`** — it declares `version: "2"` but uses v1 keys
      (`linters-settings:`, `issues.exclude-use-default`, `run.timeout`), which v2 ignores
      or rejects; move settings under `linters.settings`, verify with
      `golangci-lint config verify`.
- [ ] **CI3: deploy workflow polish** — cache bun store, add `workflow` path filters,
      and build with `-ldflags="-s -w"` (plus optional `wasm-opt -Oz`) to cut the
      multi-MB WASM payload; report the size in the job summary.
- [ ] **CI4: treefmt lists gofumpt/gci/shellcheck/shfmt/prettier but nothing installs
      them in CI** — the `ci` recipe will no-op with `--allow-missing-formatter`. Pin and
      install formatters in the CI job so format checks actually check.

## 11. Repo hygiene — 4/10

- [x] **H1: remove committed build artifacts** — `web/src/**/*.js` (App.js, main.js,
      DrumMachine.js, Knob.js, wasmEngine.js) and `web/tsconfig.tsbuildinfo` are compiler
      output sitting next to the sources (dead code; `index.html` loads `main.tsx`).
      Delete, and gitignore `*.tsbuildinfo` (root cause fixed by F1).
- [x] **H2: add a LICENSE** — the project is public with a live demo and README, but has
      no license, which legally means "all rights reserved". MIT/Apache-2.0 recommended.
- [ ] **H3: add `.editorconfig`** so Go tabs / TS spaces don't churn across editors.

## 12. Documentation — 5/10

- [x] **D1: AGENTS.md is stale in at least four places**: says algo-dsp **v0.2.0**
      (go.mod: v0.5.0); says the mix is "soft-clip"ped (it's FDN reverb + limiter); the
      `window.AlgoDrum` API table omits `setDecay` and `setReverb`; the DrumMachine
      description omits decay knobs, mute, and reverb.
- [ ] **D2: README gaps** — feature list omits decay/mute; add a screenshot or GIF of
      the UI; document browser requirements (WebAssembly + Web Audio).
- [ ] **D3: document the voice architecture** (one short doc: per-voice synthesis recipe,
      parameter ranges) — invaluable once voice params become editable (E5).

## 13. PWA & deployment — 5/10

- [ ] **P1: service worker staleness** — `CACHE_VERSION` is a hardcoded `"algo-drum-v1"`
      that never changes across deploys, and `algo_drum.wasm` is un-hashed and served
      cache-first: after a deploy, returning visitors get the **old** WASM with new JS for
      one visit (stale-while-revalidate lag), and mismatches are possible. Inject a build
      hash into the SW at build time, or serve `.wasm` network-first, or move the WASM
      through Vite's hashed asset pipeline.
- [ ] **P2: COOP/COEP headers exist only in the dev server** (`vite.config.ts`) — GitHub
      Pages cannot send them. They're unnecessary today (no SharedArrayBuffer); remove
      them, or keep with a comment noting the B1 ring-buffer design will need the
      `coi-serviceworker` workaround on Pages.
- [ ] **P3: hardcoded `base: "/algo-drum/"`** breaks forks/renames — derive from an env
      var with the current value as default.
- [ ] **P4: manifest tweaks** — `orientation: "portrait"` contradicts the landscape
      layout; add a `maskable` purpose icon variant.

---

## Suggested execution order

**P0 — correctness (small, high value):** C1 (+A1/E3 state sync), C2, C3, C4, C5, C6, C7, H1, F1, H2, D1.
✔ Done 2026-07-09 — C1 fixed by creating the engine at WASM load (the AudioContext is created later at the same fixed 48 kHz rate), which also resolves C5 since the UI pushes its defaults once loaded; A1 (engine-owned state) and E3 (bulk pattern API) remain open for the AudioWorklet migration.

**P1 — foundations:** CI1 + CI2 + T1–T3 (tests before refactors), then B1/B2/B3 (AudioWorklet migration), then the §6 DOM UI rewrite (fixes U1–U6, X1–X4 structurally).
✔ CI, tests, and the audio migration landed 2026-07-09: the engine now renders in a Web Worker, an AudioWorklet consumes chunks over a direct MessageChannel (~43 ms buffer vs ~85 ms+), and the playhead follows the audible step (C8). E7 note: the lookahead limiter controls sustained level but its smoothed detector misses single-sample noise transients; the hard clamp in Render is the guaranteed brick wall for those (~13 samples per 3 s, inaudible).

**P2 — the "algo" in algo-drum:** E1, E2, G1–G6, C8, P1, remaining polish (F6, P2–P4, D2–D3, T4–T5).
