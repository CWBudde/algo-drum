# algo-drum — Repository Review & Improvement Plan

Reviewed: 2026-07-09 · Re-reviewed: **2026-07-26** at `81dae31` · Hardening pass: **2026-07-26** · Go engine pass: **2026-08-06**.

> The 2026-07-09 review drove three implementation passes; its items are closed and
> removed (`git log` is the record). The 07-26 re-review below replaced it, and a
> hardening pass has since closed 33 of its items — those stay listed but ticked, with
> their original diagnosis intact, so the fix has a rationale attached. Unchecked items
> are open.

## Scorecard

Δ is the change from the 07-26 re-review baseline, i.e. what the hardening pass moved.

| #   | Category                  |  Score   | Δ   | Verdict                                                                                     |
| --- | ------------------------- | :------: | --- | ------------------------------------------------------------------------------------------- |
| 1   | Correctness & robustness  | **8/10** | +2  | Non-finite input rejected, swing fixed at every step count, bridge load recoverable         |
| 2   | Audio pipeline & WASM     | **7/10** | —   | Right architecture, ~43 ms latency; the pull protocol still has no recovery path            |
| 3   | Go engine / DSP           | **9/10** | +2  | True peak limiting, drift-free timing, explicit pause, continuous reverb, lean hot paths    |
| 4   | Architecture & state      | **7/10** | +1  | Command layer fully typed and echoes validated; nine parameters still UI-owned              |
| 5   | Frontend code quality     | **7/10** | —   | Dead bridge plumbing gone; the 502-line component and missing CSS tokens remain             |
| 6   | UI / UX                   | **6/10** | —   | Good knob and playhead mechanics; the grid is still unusable below ~725 px                  |
| 7   | Accessibility             | **6/10** | —   | Real buttons, real sliders, AA text contrast; 80 flat tab stops and no AT playhead          |
| 8   | Testing                   | **8/10** | +1  | Fuzzed engine + a fake-Worker bridge suite; components still untestable (no DOM env)        |
| 9   | CI/CD & tooling           | **9/10** | +2  | Lints the WASM target, deploy gated on CI, tools pinned, caches and artifacts in place      |
| 10  | Repo hygiene              | **8/10** | —   | Clean tree and dependabot now watching; the deps themselves are still three majors behind   |
| 11  | Documentation             | **9/10** | +2  | Every verified-stale claim corrected; the toolchain and test suite are finally documented   |
| 12  | PWA & deployment          | **6/10** | —   | Stamp verified and deploy no longer races CI; the bundle is still never precached           |
| 13  | Feature depth vs the name | **6/10** | —   | The "algo" arrived — but every algorithmic control is global and one-shot                   |

**Overall: 7.4/10** (was 6.6 at re-review, 4.0 at first review). The hardening pass closed
the correctness and tooling gaps; what is left is mostly reach — mobile layout, offline,
accessibility depth, and the per-step algorithms the name promises.

### Verified after the hardening pass

Every defect below was reproduced before the fix and re-checked after, with independent
probes rather than the implementation's own tests:

| Probe                                         | Before                         | After                            |
| --------------------------------------------- | ------------------------------ | -------------------------------- |
| `SetCell(0,0,NaN)` → `Render`                 | 1024/1024 samples NaN          | **no NaN; state unchanged**      |
| `SetTempo(NaN)`                               | `stepLen[0] = INT64_MIN`       | **rejected, step lengths sane**  |
| `SetSwing(0.5)` + `SetStepCount(7)`           | 45000 vs 42000 (+7.14 %)       | **exact at counts 1–16**         |
| `NewEngine(0 / NaN / ±Inf)`                   | nil-deref panic                | **no panic, degrades to bypass** |
| `golangci-lint run ./...` (WASM target)       | 3 issues in `cmd/wasm/main.go` | **0 issues**                     |
| `go test ./internal/...`                      | 34 tests, 98.8 %               | **44 tests + fuzz, 98.0 %**      |
| `bun run test`                                | 67 passing                     | **80 passing**                   |
| `bun run test:e2e` (real production build)    | 2 passing                      | **2 passing**                    |
| `treefmt --fail-on-change` · `tsc` · `eslint` | clean                          | **clean (0 warnings)**           |

The Go engine pass replaced the attack-smoothed compressor with a true lookahead peak
limiter; both an isolated 2.0 impulse and a dense all-track render stay below −1 dBFS
without invoking the final safety clamp (**E7**). REVERB became a wet/dry mix rather than
a send, so the knob no longer raises the master level: pre-limiter peak across the sweep
went from 1.802 → 2.202 (rising) to 1.802 → 1.400 (falling) (**E14**).

---

## 1. Correctness & robustness — 8/10

The sequencer core is genuinely correct — step _k_ occupies exactly `stepLen[k]` samples
with no off-by-one, the probability gate short-circuits at `prob==1`, and pending
humanize triggers are cleared on stop. Every guard used to be a `<`/`>` comparison, which
is false for NaN — that whole class is now closed by a single `validFloat` boundary, and
the JS bridge validates argument types before converting. What remains is lifecycle:
teardown and the stop-time playhead race.

- [x] **C10 (high): NaN defeats every clamp.** `clamp01` (`internal/drum/voices.go:81`)
      and the manual clamps in `SetTempo` (`engine.go:183`) / `SetSwing` (`engine.go:196`)
      use `<`/`>`, which are false for NaN. Verified: `SetCell(NaN)` → 100 % NaN output;
      `SetTempo(NaN)` → `stepLen[0] = INT64_MIN`, advancing a step per sample. The final
      clamp (`engine.go:410`) is also `>`/`<`, so nothing sanitises it and the Web Audio
      graph is dead until reload. Add an `IsNaN` rejection at the API boundary.
- [x] **C11 (high): swing is wrong for odd step counts.** `recomputeStepLengths`
      (`engine.go:142`) assigns long/short by absolute step-index parity, but
      `SetStepCount` (`engine.go:212`) allows any end index. Verified: a 7-step loop at
      swing 0.5 runs **7.14 % slow** with two long steps back-to-back across the wrap.
      `SetStepCount` also leaves `stepSamples` untouched when it wraps `currentStep`, so
      the in-flight step plays out with another step's length.
- [x] **C12: nil-deref on an invalid sample rate.** `engine.go:115` calls `rev.SetWet(0)`
      unconditionally after `reverb.NewFDNReverb`, which returns `(nil, err)` for
      `sr<=0`/NaN/Inf; the argument is evaluated before `logErr` runs. Same shape at
      `engine.go:124` for the limiter. `cmd/wasm/main.go:30` passes `args[0].Float()`
      straight through, so `AlgoDrum.init(0)` kills the runtime.
- [x] **C13: non-numeric JS args panic the engine.** `.Float()`/`.Int()`/`.Bool()` panic
      on a wrong-typed `js.Value` (`cmd/wasm/main.go:43,52,59,67,75,125,133,141,149,157,168`);
      `setPattern` calls `arr.Get("length").Int()` (`:90`) and panics on any non-array. A
      panic here takes the whole engine down with nothing reported to the app.
- [x] **C14 (high): the pattern mirror mis-reconciles after a retry.**
      `wasmEngine.ts:91-96` calls `patternMirror.reset()` on teardown but never clears
      `pendingCommands` (`:56`), so edits queued against the dead worker are flushed at
      `:141` with `inFlight` back at 0 — every intermediate echo is then published as
      authoritative (`patternMirror.ts:34`), reverting newer edits. This is precisely the
      scenario the mirror exists to prevent.
- [x] **C15: `loadWasm()` has no in-flight guard.** `wasmEngine.ts:86` checks only
      `wasmReady`. Under StrictMode the App effect (`App.tsx:31`) invokes it twice: call
      #1 binds its `ready` handler to worker W1, call #2 terminates W1 — P1 **never
      settles**. Double-clicking Retry does the same.
- [x] **C16 (high): no `worker.onerror` / `onmessageerror`.** `wasmEngine.ts:104` wires
      only `onmessage`, and `audioWorker.ts:176` dispatches `AlgoDrum[name](...)` with no
      `try`/`catch` and no callable check. A post-load throw leaves the UI showing a
      "ready" machine that makes no sound, with no timeout and no Retry path.
- [x] **C17: playhead flashes backwards on Stop.** `stop()` fires `notifyStep(-1)`
      (`wasmEngine.ts:187`) while ~2048 already-rendered samples carrying pre-stop step
      numbers drain; `worklet.js:63` reports them and re-lights the LED for ~43 ms.
- [x] **C18: no teardown.** Nothing calls `audioCtx.close()`, `workletNode.disconnect()`
      or `worker.terminate()` outside the retry path.
- [x] **C19: unbounded allocation from a JS arg.** `cmd/wasm/main.go:168` accepts any
      positive `n`; `ensureRenderBuffers` allocates `n` floats + an `n*4` ArrayBuffer.

### To raise this score — correctness

The defects above are symptoms of one root cause: **validation is re-implemented
per setter**, so each new parameter is a fresh chance to forget a case. Fix the
structure, not just the ten instances.

- [x] **C20: one validated-parameter boundary.** Replace the hand-rolled clamps in every
      setter with a single `validFloat(name, v, lo, hi)` helper that rejects NaN/Inf and
      clamps in one place. C10, C12 and the C4-era
      clamps all collapse into it, and the next parameter inherits the policy for free.
- [x] **C21: fuzz the public API.** A Go fuzz target that drives arbitrary setter
      sequences then asserts `Render` output is finite and within ±1.0 would have found
      C10 on its own. Cheap to write against `internal/drum` (pure Go, no WASM needed)
      and it runs in the existing `go test` job via `-fuzztime`.
- [x] **C22: define the out-of-range contract.** `SetCell`/`SetVolume` silently no-op on
      a bad track/step index (`engine.go:231`, `:265`), so a UI bug looks like a dead
      pad. Decide — and document — whether these clamp, error to JS, or panic in dev
      builds; today the answer differs per method.
- [x] **C23: an engine self-check.** A `Validate()` (or a build-tagged assertion in
      `Render`) verifying the invariants the tests already assume — `stepLen` positive
      and finite, `currentStep < stepCount`, no active pending trigger past its deadline
      — turns silent corruption like C11 into a loud failure.

## 2. Audio pipeline & WASM bridge — 7/10

The architecture is right and the 07-09 migration holds up: engine in a dedicated Worker,
AudioWorklet pulling over a direct `MessageChannel`, ~43 ms latency, `js.CopyBytesToJS`
instead of per-sample `SetIndex`, reused render buffers, an `instantiateStreaming`
fallback, and a genuinely nice `REQUIRED_METHODS` runtime assertion against a stale
`.wasm`. The weakness is that the pull protocol is a bare credit counter.

- [ ] **B7 (high): a dropped `need` deadlocks audio permanently.** `audioWorker.ts:150`
      returns early and silently when `!engineReady`, while `worklet.js:45` only requests
      more when `queued + pendingRequests*512 < 2048`. Four lost requests pin
      `pendingRequests` at 4, the condition never becomes true again, and audio stops
      forever with no error. Same outcome if the render call throws — there is no
      `try`/`finally` decrement and no timeout.
- [ ] **B8: underruns are invisible.** `worklet.js:83` fills the tail with silence (the
      comment is honest) but there is no counter, no message to the main thread and no
      adaptive queue growth, so a Go GC pause is an unreported audible dropout.
- [ ] **B9: per-chunk garbage.** `audioWorker.ts:157` allocates a fresh 2 KB
      `Float32Array` per chunk (~94/s) to have something transferable and never recycles
      the transferred buffer back. A return pool — or a SharedArrayBuffer ring, which
      would delete the credit protocol entirely — is the obvious next step.
- [ ] **B10: rendering never stops.** Stop only sets `running=false` (`engine.go:155`);
      the worklet keeps pulling and the engine keeps running all five voices + reverb +
      limiter forever. Nothing suspends the AudioContext. Continuous idle CPU/battery.
- [ ] **B11: stale comment** — `worklet.js:12` claims `CHUNK_SAMPLES` "must match the
      worker's render chunk size"; the worker renders whatever `samples` says.

## 3. Go engine / DSP — 9/10

The strongest area: idiomatic, well-commented, magic numbers lifted into documented
constants (`voices.go:22-79`), `math/rand/v2` with fixed per-voice seeds, an
allocation-free `Render` with a test asserting it, and coverage of clamping, swing
bar-length, velocity and humanize bounds. The engine pass closed every item below.

- [x] **E6: stop vs. pause semantics.** `SetRunning(false)`
      (`engine.go:155`) resets `currentStep`/`stepSamples` and clears pending triggers;
      there is no `Pause()` in Go, no `pause()` in `wasmEngine.ts`, and
      `DrumMachine.tsx:249` is a binary toggle. Voices and reverb _do_ ring on after stop.
      Closed with an explicit stopped/playing/paused transport: pause freezes fixed-point
      sequencer time and delayed hits while tails ring, Play resumes, and a separate Stop
      resets to step 0.
- [x] **E7 (re-opened): the limiter still isn't limiting.** Previously marked closed.
      Output is now _guaranteed_ bounded by the hard clamp (`engine.go:410`), but that
      clamp — not the limiter — is doing the work: verified **16 samples clipped in 3 s**
      on an ordinary bass/snare/hat pattern at reverb 0.3, with the pre-limiter mix
      peaking around 1.8 despite `mixHeadroom = 0.5` (`engine.go:21`) and a −1 dBFS
      threshold (`engine.go:124`). `TestRenderOutputBoundedAndFinite` only asserts the
      clamp works, so it can never fail. Fix the gain staging (`hatGain 1.5`,
      `cymGain 1.2`) or the limiter usage, and add a test that asserts _no clipping_.
      Closed with an allocation-free monotonic-queue lookahead limiter whose instantaneous
      gain reduction mathematically bounds isolated transients as well as sustained peaks;
      tests assert the semantic hard-clamp counter stays at zero.
      Closed on the limiter half only, deliberately. The gain staging this item also names
      is real and stays: measured pre-limiter, a **solo hi-hat peaks at +1.7 dBFS** and a
      solo snare at −0.5 dBFS, so an ordinary three-voice pattern runs +5.1 dBFS and the
      worst case +10.4 dBFS into a −1 dBFS ceiling. The limiter therefore does routine
      transient reduction, not rare worst-case catching, and `mixHeadroom`'s comment was
      corrected to say so. It was left alone because `hatGain`/`cymGain` are user-facing
      parameter _defaults_ (hat ranges to 2.5), so no static scalar can bound the mix —
      lowering them would only rebalance the kit while leaving the worst case unbounded.
- [x] **E8: reverb bypass is a discontinuity.** `engine.go:402` skips `ProcessSample`
      entirely at `reverbAmount == 0`, truncating the tail with a click and later dumping
      stale delay-line contents back out. `SetWet(0)` (`:288`) already mutes correctly.
      The FDN now advances at every wet setting, while its wet amount follows a short
      one-pole ramp; zero wet remains bit-exactly transparent and cannot preserve a stale
      tail to resurrect later.
- [x] **E14: REVERB was a send, not a mix.** `FDNReverb.ProcessSample` returns
      `input*dry + tail*wet` and its dry gain defaults to 1, but `Engine` only ever set
      the wet gain — so turning REVERB up raised the master level and pushed the limiter
      harder. Measured on an ordinary bass/snare/hat pattern, pre-limiter peak: 1.802 dry,
      **1.845 at reverb 0.3, 2.202 at reverb 1.0**. `Render` now sets `dry = 1 - wet`
      alongside the wet gain, so the same sweep reads 1.802 → **1.602** → **1.400** —
      monotonically down instead of up, with `hardClipCount` at 0 throughout.
- [x] **E9: `firePending` is O(32) per sample** (`engine.go:360`) ≈ 1.5 M iterations/s,
      almost always all-inactive. An active count or free-list head makes it near-free.
      A 32-bit active-slot mask makes the empty case O(1) and visits only live slots,
      preserving lowest-slot scheduling and deterministic simultaneous-trigger order.
- [x] **E10: `Pattern()` allocates per call and is not rare.** `engine.go:253` allocates a
      fresh slice and `cmd/wasm/main.go:108` a `Float32Array` plus 80 `SetIndex` calls.
      The "called rarely (state sync)" comment (`main.go:106`) is stale — the pattern echo
      calls it on **every cell click** (`audioWorker.ts:188`). Reuse a persistent buffer.
      `CopyPattern` writes into caller-owned `[112]float32` storage; the WASM bridge bulk-
      copies it into one persistent JS-owned typed array. Allocation tests cover both the
      snapshot and setter paths.
- [x] **E11: `SetPattern` is asymmetric** — a short slice leaves untouched cells alone
      (`engine.go:241`) while `getPattern` always returns 80, so partial set + get is a
      merge, not a replace.
      The setter now accepts exactly one complete 112-cell snapshot and applies it
      atomically; wrong lengths or any non-finite entry reject the whole replacement.
- [x] **E12: `Voice.IsActive()` is dead outside tests** (`voices.go:17`). Either use it to
      skip `Tick()` on inactive voices — a real saving — or drop it from the interface.
      Dropped from the production interface; concrete voice lifecycle methods remain for
      the test-only `activeVoice` contract without adding a second virtual call in Render.
- [x] **E13: `stepLen` truncation has no error accumulator** (`engine.go:148`), dropping
      up to ~0.5 samples/step at non-integer BPM. Irrelevant standalone; matters the
      moment anything external syncs to it.
      Step durations and elapsed phase now use Q32.32 samples. Fractional residual crosses
      every step and loop boundary, swing pairs sum exactly, odd loops keep their final
      unpaired base step, and long-run tests pin the cumulative error below one sample.

## 4. Architecture & state management — 7/10

`PatternMirror` is small, isolated, unit-tested, and the `WorkerCommand`/`WorkerResponse`
discriminated unions with the `AssertNever` guard are the best code in the repo, and the
command layer is now typed end to end with echoes validated against the expected size. But
"single source of truth" still holds for **one of ten** pieces of state, so two
contradictory state disciplines sit side by side.

- [ ] **A4 (high): only the pattern is engine-owned.** Tempo, swing, step count, reverb,
      probability, humanize, volumes, decays and mute are still UI-owned and fired
      one-way with no echo, no clamp feedback and no reconciliation — so the engine's
      clamping (e.g. tempo 30–300) is invisible to the UI. Either extend the echo
      protocol to a full state snapshot or document the split deliberately.
- [x] **A5: the typed command layer has a hole at its constructor.**
      `wasmEngine.ts:67` is `command(name: string, ...args: unknown[])` with an
      `as WorkerCommand` cast, so `command("setTemp", 120)` compiles despite
      `WorkerCommand` declaring `name: keyof AlgoDrumApi`. Typing the parameter is a
      one-word fix; per-method arg tuples close it fully.
- [x] **A6: echo length is never validated.** `patternMirror.ts:35` rejects only
      `length === 0`, and `flatToVisual` pads with `?? 0`, so a version-skewed engine
      returning a short array silently wipes tracks.
- [ ] **A7: `playing` has no owner** — set optimistically (`DrumMachine.tsx:253`), faked
      locally (`wasmEngine.ts:189`), with no `onRunning`. A dead worker leaves the UI
      showing "playing".
- [ ] **A8: persistence mixes two coordinate systems.** `buildState`
      (`DrumMachine.tsx:170`) converts the pattern to engine-major but writes
      `volumes`/`decays`/`muted` in **visual** order, documented only as "length 5"
      (`persistence.ts:33`). Reordering `TRACKS` silently corrupts every saved blob and
      shared link with no version bump to catch it.
- [ ] **A9: the share format encodes knob positions, not semantics.** Tempo is stored
      normalized (`persistence.ts:91`), so changing `BPM_MIN`/`BPM_MAX` reinterprets every
      existing v1 link; `FORMAT_VERSION` guards byte layout only.
- [ ] **A10: `getShareUrl` mutates history as a side effect of a getter**
      (`persistence.ts:198` calls `history.replaceState`).
- [ ] **A11: a load error unmounts the whole machine.** `App.tsx:57` renders
      `DrumMachine` only when `status !== "error"`, so Retry discards all UI state and
      re-reads persistence, losing up to 300 ms of debounced edits.
- [x] **A12: `startAudio` has no error handling** (`wasmEngine.ts:146`); a rejecting
      `addModule` propagates into `handlePlayStop` (`DrumMachine.tsx:249`, no catch) with
      nothing shown to the user.

### To raise this score — architecture

A4 and A8 are the same bug wearing two hats: **there is no shared definition of "the
state"**, so the engine, the echo protocol, the React tree and the persistence blob each
describe it differently. One type fixes the category rather than the instances.

- [ ] **A13: a single `EngineState` shape.** Define the full parameter set once and have
      the engine snapshot echo it, the mount-time seed push it, and persistence serialise
      it — replacing today's three independent orderings. Removes A4's split ownership and
      A8's engine-major/visual-order mismatch by construction, and gives A9 a place to
      store tempo as BPM rather than a knob position.
- [ ] **A14: collapse the eight push-effects into one state → command mapping.**
      `DrumMachine.tsx:113-167` is eight `useEffect`s that each mirror one value to the
      engine. A reducer whose actions map to commands makes the set exhaustive (a new
      parameter cannot be forgotten), makes A7's `playing` an ordinary reducer field, and
      is the precondition for F7's component split.
- [ ] **A15: version the worker protocol, not just its method list.** `REQUIRED_METHODS`
      checks that method _names_ exist; it cannot detect changed semantics or argument
      order. Send a `protocolVersion` in the `ready` message and refuse to run on
      mismatch — the failure mode today is a silently wrong-sounding engine.
- [ ] **A16: give the transport a single owner.** `playing` (UI), `currentStep` (worklet)
      and `running` (Go) are three views of one state machine with no arbiter. Model it
      explicitly — `stopped | starting | playing` owned by the engine and echoed like the
      pattern — which subsumes A7 and C17's stop-drain suppression (fixed pointwise in
      `wasmEngine.ts`, but only because the bridge knows -1 means "stopped").

## 5. Frontend code quality — 7/10

`bunx tsc --noEmit` and `eslint .` both pass with zero warnings, there is no `any`, the
two casts are defensible, and the pure algo modules are well tested. The weaknesses are
structural rather than sloppy.

- [ ] **F7: `DrumMachine.tsx` is a 502-line god component** — persistence restore, eight
      parameter-push effects, engine seeding, mirror subscription, tap tempo, the global
      keybinding, grid, track strips, transport and share plumbing, with ~210 lines of
      unbroken JSX. Seams: a `useEngineSync` hook for `:113-167`, and
      `<StepGrid>` / `<TrackStrip>` / `<Transport>` for `:311-398` and `:409-499`.
- [ ] **F8: pure logic trapped in the component.** `cycleVelocity`, `velocityName`,
      `visualToFlat`, `flatToVisual`, `snapVelocity`, `visualPatternsEqual`
      (`DrumMachine.tsx:27-69`) are pure, untested, and belong beside `algo/pattern.ts`.
- [ ] **F9: duplicated constants** — `DrumMachine.tsx:17-22` redefines `COLS`, `ROWS`,
      `VEL_NORMAL`, `VEL_ACCENT`, which already exist in `algo/pattern.ts:8-17` where they
      define the persistence byte format.
- [x] **F10: ~45 lines of dead code.** Verified zero callers for `getPattern()`
      (`wasmEngine.ts:223`), `nextRequestId`/`patternResolvers` (`:73`),
      `settlePendingPatternRequests` (`:78`), the worker's `"getPattern"` case
      (`audioWorker.ts:197`) and the `"pattern"` response variant (`:94`).
      `currentStep()` (`:256`) is unused too — the UI uses `onStep`.
- [ ] **F11: the whole machine re-renders per playhead tick.** `currentStep`
      (`DrumMachine.tsx:108`) lives in the top component, so ~8×/s React re-renders 80
      buttons, 16 step numbers, 10 SVG knobs and `AlgoPanel`. Nothing is memoized, and
      `getShareUrl` (`:203`) changes identity on every edit.
- [ ] **F12: `AlgoPanel` leaks a timer** — `copiedTimer` (`AlgoPanel.tsx:32`) is never
      cleared on unmount; the component has no `useEffect` at all.
- [ ] **F13: `Knob` listener churn** — the non-passive wheel listener re-registers on
      every render (`Knob.tsx:112`, deps `[value, onChange]`), and pointer capture
      (`:76`) is never released.
- [ ] **F14: no CSS token layer.** Three custom properties exist (`DrumMachine.css:5-8`)
      while the palette repeats as raw hex across 870 lines; the accent colour lives in
      four places (`DrumMachine.css:5`, `DrumMachine.tsx:24`, `Knob.tsx:52`, and a fifth
      mismatched background in `index.html` vs `App.css:13`).
- [ ] **F15: tsconfig strictness gaps** — `noUncheckedIndexedAccess` is **off** despite
      pervasive raw indexing (`pattern[row][col]`, `bytes[offset++]`); that is the single
      highest-value flag here. Also missing `exactOptionalPropertyTypes`,
      `verbatimModuleSyntax`, `isolatedModules`. `include: ["src"]` means
      `vite.config.ts` / `vitest.config.ts` / `playwright.config.ts` are never typechecked.
- [ ] **F16: ESLint coverage gaps** — no `eslint-plugin-jsx-a11y` despite heavily
      hand-rolled ARIA; `recommendedTypeChecked` rather than `strictTypeChecked`; and
      `react-hooks/exhaustive-deps` is `warn` with no `--max-warnings 0`, so a deps
      regression would not fail CI.

## 6. UI / UX — 6/10

The rewrite delivered real DOM, real focus rings, a well-built knob (pointer capture,
shift-fine, wheel, double-click reset, drag readout, `touch-action: none`), tap tempo,
share, and `prefers-reduced-motion` in all four stylesheets. The playhead is a cheap
per-cell attribute toggle — the right mechanism. Layout and feedback are what hurt.

- [ ] **U7 (high): the grid does not fit a phone.** `DrumMachine.css:76` and `:399` keep
      `repeat(16, minmax(0, 1fr))` at **every** width; verified no container queries and
      no overflow wrapper anywhere. At a 390 px viewport ~120 px is left for 16 cells →
      **~7.5 px each**. The app's primary interaction surface is unusable below ~725 px.
      Needs an overflow-scroll wrapper, an 8+8 split, or a page-1/page-2 toggle.
- [ ] **U8: breakpoint discontinuity** — 620 px yields ~21.8 px cells, 621 px yields
      **~17.5 px**. `DrumMachine.css:393` should be `min-width`-driven.
- [ ] **U9: the playhead is near-invisible** — `rgba(200,140,40,0.07)` tint is **1.09:1**
      (`DrumMachine.css:214`) and the marker line **1.95:1** (`:222`). On a sparse track
      it cannot be followed at all. Same for the bar-group aids (1.12:1, 1.09:1).
- [ ] **U10: no value readout outside dragging.** `Knob.tsx:150` renders `.knob-readout`
      only while `dragging`, so keyboard and wheel changes to SWING/STEPS/PROB/HUMAN/
      REVERB/volumes/decays show **no number**. Only tempo embeds its value in the label.
      Partly addressed by G20: every knob inside the voice editor has a persistent
      `.dm-voice-value` readout under it. The main panel still does not.
- [ ] **U11: the wheel handler hijacks page scroll unconditionally** (`Knob.tsx:112`
      always `preventDefault()`s, focused or not) — scrolling with the cursor over a knob
      silently changes the value.
- [ ] **U12: one keyboard shortcut.** `DrumMachine.tsx:261` handles Space and nothing
      else — no arrow navigation, no clear/mutate/preset keys, no `1`–`5` track mute.
- [ ] **U13: no drag-paint on the grid** (`DrumMachine.tsx:337` is a per-cell `onClick`),
      and clearing an accent takes two taps. Standard sequencer affordances are absent.
- [ ] **U14: stale/awkward panel state** — the preset name persists after hand-edits
      (`AlgoPanel.tsx:103`), and `.dm-algo-num` (`:162`) clamps mid-typing under the cursor.
- [ ] **U15: bare loading state** — `App.tsx:55` is a plain `<p>`; the engine LED carries
      "Engine ready" only in a `title` on an `aria-hidden` span. The error/retry panels,
      by contrast, are good.

### To raise this score — UI/UX

Beyond the defects above, the machine is missing the everyday affordances a step
sequencer is judged by: editing is strictly one cell at a time, and the interface never
tells you what it can do.

- [ ] **U16: per-row operations.** There is no clear-row, shift-row-left/right,
      duplicate-row, or copy-bar-to-bar anywhere — rotation exists only inside the
      Euclidean fill (`AlgoPanel.tsx:64`). These are a handful of pure array functions
      next to `algo/mutate.ts` plus a row context menu, and they change how the machine
      feels to use far more than their cost suggests.
- [ ] **U17: accent is conveyed by LED brightness alone.** Normal and accent hits differ
      only in glow intensity, which is hard to scan at a glance and disappears entirely
      under X12's forced-colors gap. Differentiate by size or shape as well.
- [ ] **U18: no shortcut discovery.** Space is the only binding and nothing advertises it.
      A `?` overlay listing the keys — worth doing together with U12, since shortcuts
      nobody can find are shortcuts nobody uses.
- [ ] **U19: no first-run guidance.** A new visitor gets an empty grid and a disabled Play
      button with no hint that presets exist. Seed a default pattern on first load (no
      saved state, no URL hash) so the machine makes a sound within one click.
- [ ] **U20: no count-in or metronome**, which makes tap tempo and humanize hard to judge
      by ear.
- [ ] **U21: the panel does not scale as a unit.** Sizes are fixed px throughout
      (`Knob size={42}`, 9–11 px labels), so there is no way to make the whole machine
      bigger on a large display or smaller on a cramped one. A single `--dm-scale`
      custom property driving `rem`-based sizing would also give U7 a lever to pull.

## 7. Accessibility — 6/10

The fundamentals are done properly and the contrast pass was real: **every text colour
checked passes AA** (5.0:1 on step numbers up to 12.2:1 in inputs; the one sub-4.5 value
is 21 px/600 large text where 3:1 applies). Cells are real `<button>`s with `aria-pressed`,
the knob is a textbook `role="slider"` with `aria-valuetext` and full Arrow/Page/Home/End
support, focus-visible rings exist everywhere, both failure panels use `role="alert"`.
The gaps are structural.

- [ ] **X5 (high): 80 flat tab stops, no grid semantics.** `DrumMachine.tsx:322` emits 80
      tabbable buttons with the _primary_ control (Play, `:410`) last in the DOM — no
      `role="grid"`/`gridcell`, no roving `tabindex`, no arrow-key movement. Reaching the
      transport by keyboard costs ~95 Tab presses.
- [ ] **X6: a tri-state control forced into a boolean toggle.** `DrumMachine.tsx:335` sets
      `aria-pressed={velocity > 0}` and encodes off/on/accent in the label, so accent and
      normal announce the same role state and the accessible _name_ mutates on every
      activation.
- [ ] **X7: the playhead is invisible to AT.** `data-playhead` (`:332`, `:353`) is purely
      presentational — no `aria-current`, no live region. Combined with U9, low-vision
      users have no playhead at all.
- [ ] **X8: no live regions for state changes.** Space toggles playback with **zero
      announcement** (the Play label flips but it isn't focused), and PRESET / MUTATE /
      CLEAR / FILL rewrite up to 80 cells silently. Only the SHARE toast has
      `role="status"` (`AlgoPanel.tsx:214`). The loading→ready transition isn't announced
      either, while Play is `disabled` and so cannot explain why.
- [ ] **X9: target size (SC 2.5.8) fails below ~725 px**, and `.dm-tap` fails at every
      width — `font-size: 9px` + `padding: 3px 10px` ≈ **17 px** high
      (`DrumMachine.css:314`). `.dm-mute` and the algo controls sit at 26 px: over the
      24 px floor, well under the 44 px touch guidance, with no `@media (pointer: coarse)`.
- [ ] **X10: non-text contrast (SC 1.4.11) failures** — playhead marker 1.95:1, playhead
      tint 1.09:1, bar divider 1.12:1, bar tint 1.09:1, all below the 3:1 required of
      meaningful UI indicators.
- [ ] **X11: "beyond step count" is conveyed by opacity alone** (`DrumMachine.css:155`),
      so a screen-reader user editing step 13 of an 8-step pattern gets no signal that it
      will never fire. `aria-disabled` or a name suffix would fix it.
- [ ] **X12: no forced-colors / prefers-contrast support** anywhere in `src/`. Every state
      is a background, box-shadow or gradient — all stripped in Windows High Contrast,
      leaving 80 identical empty buttons.
- [ ] **X13: dead accessible-name plumbing** — `DrumMachine.tsx:316` assigns
      `id={dm-track-…}` to each track label and nothing ever references it.

### To raise this score — accessibility

The ARIA here was written carefully by hand and is largely correct, but nothing
_verifies_ it, so the next refactor can quietly undo it. Add enforcement, then close the
remaining "can a screen-reader user actually operate this?" gaps.

- [ ] **X14: enforce accessibility automatically.** Nothing checks any of this today —
      verified no `axe-core`, `eslint-plugin-jsx-a11y`, `jsdom` or Testing Library in
      `web/package.json`. Add `eslint-plugin-jsx-a11y` (static, free) and an `axe-core`
      scan in the Playwright run (runtime, catches contrast and name/role/value). This is
      the item that keeps X5–X13 fixed once they are fixed.
- [ ] **X15: a text alternative for the pattern.** The grid is 80 controls with no
      summary; an sr-only, `aria-live="polite"` description per track ("Bass drum: steps
      1, 5, 9, 13") makes the pattern comprehensible without 80 Tab presses, and doubles
      as the announcement channel X8 needs for bulk edits.
- [ ] **X16: focus management for bulk actions.** After PRESET / CLEAR / MUTATE / FILL the
      grid is rewritten under the user's feet; focus should stay put and the change be
      announced, rather than the current silent swap.
- [ ] **X17: a skip link to the transport.** Directly addresses X5's ~95-Tab journey and
      is a few lines, independent of the roving-tabindex work.
- [ ] **X18: state a target and test against it.** No conformance level is claimed
      anywhere. Commit to WCAG 2.2 AA, list the known exceptions, and record one manual
      screen-reader pass (NVDA or VoiceOver) — automation catches perhaps half of what
      X6/X7 are about.

## 8. Testing — 8/10

The Go engine is genuinely well tested: 44 test functions plus a fuzz target, **98.0 %**
statement coverage,
asserting real invariants (swing preserves bar length, clamping on every setter, output
bounded and finite, humanize bounds, allocation-free render via `AllocsPerRun`). The
frontend's tests are up to **80 passing**, now including a fake-Worker suite for the
main-thread bridge. The remaining island is the UI itself.

- [ ] **T7: `cmd/wasm/main.go` is invisible to the test runner.** Its `js && wasm` build
      tag means `go test ./...` never even compiles it, so the arg marshalling,
      clamping, `unsafe` buffer reuse and pre-init gating — the entire API surface the
      frontend depends on — are 0 % covered. This is where C13 and C19 live.
- [ ] **T8: no golden render test.** `engine_test.go:608` builds two engines in the same
      process and compares them, which proves determinism but not stability — any DSP
      change passes silently. Commit a reference checksum/RMS-per-step-window instead.
- [ ] **T9: the bridge is untested** — `wasmEngine.ts` (258 lines) and `audioWorker.ts`
      (206 lines): worker spawn, MessageChannel wiring, command dispatch, chunking, step
      tagging, echo-after-edit and the new API-compat checks have no unit test. C14–C16
      all live here.
- [ ] **T10: no component tests at all**, and currently impossible: `vitest.config.ts:9`
      sets `environment: "node"` with no jsdom/happy-dom or Testing Library in
      `package.json`. Adding them would let X5–X8 be regression-tested.
- [ ] **T11: e2e is two tests wide.** The assertions are real (aria-pressed flip, name
      cycling off→on→accent, playhead appear/clear, Space transport) but nothing covers
      preset load, MUTATE, Euclid fill, SHARE/URL restore, localStorage, knob drag, mute,
      STEPS/PROB/HUMAN or reverb.
- [ ] **T12: no coverage measurement or threshold in CI** for either language, and no
      `coverage` block in `vitest.config.ts`.

## 9. CI/CD & tooling — 9/10

`ci.yml` gates PRs on all four axes — Go test + WASM build + tidy + lint, treefmt with
every formatter genuinely installed (checksum-verified download), typecheck + eslint +
vitest + vite build, and a real Playwright run against the production build. `golangci-lint
config verify` is clean under v2.12.2, tools are pinned, caches and failure artifacts are
in place, and the deploy now runs only after a green CI on the exact commit it builds. The
remaining gap is payload size: the WASM is reported but never budgeted.

- [x] **CI5 (high): CI never lints `cmd/wasm`.** `ci.yml:33` runs
      `golangci-lint-action@v8` with the host GOOS, where `./cmd/...` resolves to
      `no go files to analyze` (verified). `justfile:23` correctly sets
      `GOOS=js GOARCH=wasm` — which currently reports **3 real issues** in
      `cmd/wasm/main.go` (2 × varnamelen, 1 × wsl_v5) that CI cannot see. Set the env on
      the action.
- [x] **CI6: `deploy.yml` does not depend on CI.** No `needs:`, and its own steps are
      only `bun run build` — so a push to `main` deploys in parallel with the test run
      and can publish a build whose unit or e2e tests are red.
- [x] **CI7: `--allow-missing-formatter` is still passed in CI** (`ci.yml:86`) despite the
      comment at `:39` claiming all formatters are installed. If any install step
      degrades — e.g. the `bun pm bin -g` path drift at `:70` for prettier, which owns
      _every_ `.ts/.tsx/.md/.yml/.json/.css` file — the check passes green having
      formatted nothing. Drop the flag so the claim is enforceable.
- [x] **CI8: unpinned tool versions** — `golangci-lint: latest` (`ci.yml:35`) and
      `bun-version: latest` in both workflows make builds non-reproducible and let an
      upstream release break CI with no repo change.
- [x] **CI9: no `concurrency` group on `ci.yml`**, so rapid pushes run redundant full
      matrices; and no bun-store cache in `ci.yml` (`deploy.yml:49` has one) nor a
      `~/.cache/ms-playwright` cache, so Chromium is re-downloaded every run.
- [x] **CI10: e2e artifacts are discarded.** `playwright.config.ts:25` produces traces on
      retry and nothing uploads them; add `actions/upload-artifact` with `if: failure()`.
- [ ] **CI11: WASM size is reported but never budgeted** (`deploy.yml:35`) — 4,188,961
      bytes today, and only post-merge, never on a PR. No `wasm-opt -Oz` pass exists
      (binaryen isn't installed anywhere); it typically takes another 10–20 % off.
- [x] **CI12: `just ci` and the CI workflow have diverged** — the recipe is
      `check-formatted lint check-tidy web-typecheck` with **no tests**, so a green local
      `just ci` says less than it appears to.

## 10. Repo hygiene — 8/10

`git status` is clean, nothing stray is tracked, `LICENSE` (MIT) is present and excluded
from formatting, and `.gitignore` is thorough enough to document _why_ `/wasm` is ignored.
No dead files; `docs/voices.md` is linked from the README.

- [ ] **H4: the `.trunk` decision lives on one machine.** `/.trunk` is excluded via
      `.git/info/exclude`, which isn't shared — a teammate running `trunk init` (or an IDE
      extension) would see it as untracked and could commit a second tool stack. The local
      config pins `go@1.21.0` against a `go 1.25.0` module and duplicates
      gofmt/prettier/shellcheck/shfmt, which `treefmt.toml:45-84` already owns. Decide:
      commit the ignore rule, or commit `.trunk/trunk.yaml` deliberately.
- [x] **H5: dependencies are three majors behind** — `vite` 7.3.1 → 8.1.5,
      `@vitejs/plugin-react` 4.7.0 → 6.0.5, `typescript` 5.9.3 → 7.0.2, plus minor drift
      in `@playwright/test`, `eslint`, `typescript-eslint`, `react`/`react-dom`. TypeScript
      is now installed twice on purpose: 7.0.2 under the `@typescript/native` alias for
      `tsc`, and 6.0.3 as `typescript` for `typescript-eslint`, which still needs the TS 6
      compiler API (see the TypeScript entry in `AGENTS.md`). Collapse that once
      typescript-eslint supports TS 7.
- [x] **H6: no `dependabot.yml` or Renovate config**, so nothing keeps the above or
      `go.mod` current.
- [ ] **H7: `.editorconfig` is advisory only** — no `editorconfig-checker` in CI, and
      `justfile` recipe bodies already contradict its global `indent_size = 2`.

## 11. Documentation — 9/10

The load-bearing content is now accurate: all 15 `api.Set(...)` registrations were checked
against the API table — every documented method exists, none is missing, and every
documented clamp is real (tempo 30–300, swing 0–0.5, steps 1–16, ~8 ms volume ramp,
humanize ≤15 ms / ±20 %). The track table, reverse UI order, signal-flow numbers (512-sample
chunks, ~2048 buffered, 48 kHz) and dependency versions all match the code. Every stale
claim the re-review found has been corrected, and the toolchain — the `justfile` and all
three test suites — is documented for the first time.

- [x] **D4: `AGENTS.md:7` is stale and self-contradicting** — "exposes a global
      `window.AlgoDrum` API". There is no `window.AlgoDrum` on the main thread, as
      `AGENTS.md:47` and `:81` themselves say.
- [x] **D5: the architecture list has fallen ~8 files behind** (`AGENTS.md:46-63`) —
      missing `main.tsx`, `ErrorBoundary.tsx`, `knobMath.ts`, `sw.js`, `site.webmanifest`,
      `e2e/smoke.spec.ts`, the five `*.test.ts` files, `docs/voices.md` and `PLAN.md`.
- [x] **D6: the toolchain is undocumented.** `AGENTS.md:9-42` never mentions the
      `justfile` — the actual dev interface — nor any test command (`go test ./...`,
      `bun run test`, `bun run test:e2e`, `bun run lint`). An agent reading it would not
      know the test suite exists.
- [x] **D7: Key Dependencies omits React 19.2.7** — the primary runtime dependency —
      along with TypeScript 5.9, Vitest 4, Playwright 1.61, ESLint 10, treefmt,
      golangci-lint (`AGENTS.md:101-105`).
- [x] **D8: `README.md:27` is false** — "the step grid and knobs are focusable and respond
      to arrow keys". Knobs do; grid cells are plain buttons with no arrow handling (X5).
- [x] **D9: two overstated README claims** — "Installable / offline-capable" (`:28`) is
      contradicted by P5, and `:74` gives the dev URL as `localhost:5173` when `base`
      puts the app at `/algo-drum/`.
- [x] **D10: `.claude/skills/verify/SKILL.md:36` says "steps 1–8"** — the grid is 16.
- [x] **D11: `docs/voices.md:20` treats E5 as future work** while this plan marks it done;
      voice params are indeed still hardcoded and unexposed. Reconcile the two.
      Resolved by G20 actually exposing them: the doc now describes the shipped
      parameter API and the constants it defaults to.

## 12. PWA & deployment — 6/10

The P1–P4 fixes are real: the cache version is genuinely stamped at build time
(`deploy.yml:68` seds in the 12-char SHA), so a returning visitor gets a byte-different SW
→ install → `skipWaiting` → old caches dropped → `clients.claim()`; combined with
network-first navigations and network-first `.wasm`, the stale-WASM bug is fixed. `base`
is env-configurable, COOP/COEP were removed with a correct explanatory comment, and
`start_url`/`scope` are relative. Offline is the gap.

- [ ] **P5 (high): the app bundle is never precached, so offline does not work.**
      `sw.js:16-29` lists no `assets/index-*.js|css`. The SW registers on `load`
      (`main.tsx:12`), so the first visit's bundle is fetched before the SW controls the
      page and never cached: offline reload → cached `index.html` → bundle request →
      miss → blank app. Worse, the activate handler deletes the previous SHA-named cache,
      so **every deploy resets offline capability** until another online visit. Needs a
      build-time asset manifest, or generating `sw.js` from the Vite build.
- [ ] **P6: the maskable icon is the same edge-to-edge artwork** as the `any` icon
      (`site.webmanifest:29`); Android's adaptive mask crops to the central 80 % circle,
      so the braces clip and the transparent corners are filled by the launcher. Needs a
      separate ~40 %-inset icon on an opaque background.
- [x] **P7: the cache-version `sed` is unverified** (`deploy.yml:70`) — a silent no-op if
      `sw.js:6` is ever renamed or reworded. Add
      `grep -q "algo-drum-${GITHUB_SHA::12}" web/dist/sw.js`.
- [ ] **P8: SW cache writes are not awaited** — background `caches.put` is fired with
      `void` instead of `event.waitUntil` (`sw.js:94`, `:107`), so termination can lose
      entries; and in the SWR branch `networkFetch` is created eagerly with its rejection
      unhandled, giving an unhandled rejection per asset on every offline load.
- [ ] **P9: `respondWith(undefined)` is reachable** — `sw.js:82` matches `index.html` from
      a deliberately fail-soft precache (`:46`), producing a hard network error rather
      than falling through.
- [ ] **P10: manifest gaps** — no `id` (stable PWA identity), no explicit `orientation`
      (P4 removed the wrong `portrait` without adding `any`), no `screenshots` /
      `display_override`.
- [x] **P11: `deploy.yml:17` uses `cancel-in-progress: true`** on the pages concurrency
      group, which can cancel an in-flight _deployment_; GitHub's guidance for deploy
      workflows is `false`.

### To raise this score — PWA

P5, P7, P8 and P9 are all consequences of hand-maintaining a service worker whose
precache list has to be kept in sync with a hashed build by hand. That is the thing to
change.

- [ ] **P12: generate the service worker from the build.** Adopting `vite-plugin-pwa`
      (Workbox) produces the precache manifest from the actual emitted assets, which
      closes P5 (bundle never precached), P7 (unverified `sed`) and P8 (unawaited cache
      writes) structurally, and gives P10's manifest fields a single source. Keep the
      hand-written routing rules for `.wasm` if the network-first behaviour is wanted.
- [ ] **P13: no update-available UX.** Registration is fire-and-forget (`main.tsx:11-17`)
      — no `updatefound` or `waiting` handling — while the SW calls `skipWaiting()` and
      `clients.claim()`. A new version therefore takes over _mid-session_ and can serve
      the new bundle to an already-running page. Either prompt ("new version — reload") or
      drop `skipWaiting` and activate on next load.
- [ ] **P14: route the WASM through Vite's hashed asset pipeline.** A content-hashed
      `algo_drum.wasm` can be cached immutably (cache-first, forever) instead of the
      current network-first revalidation of a 4.2 MB unhashed file on every visit — it is
      both the largest asset and the one currently costing a round-trip.
- [ ] **P15: verify the PWA claims in CI.** Nothing checks installability or offline
      behaviour, which is how P5 shipped while the README advertised it. A Lighthouse
      PWA/performance budget on the deploy build, or a Playwright test that loads the app
      offline after one online visit, makes the claim testable.

## 13. Feature depth vs. the name — 6/10

The "algo" arrived and it is real work: a correct Bjorklund with rotation
(`euclid.ts:11`), a musically-biased random walk that protects the downbeat and never
empties the pattern (`mutate.ts:71`), a probability gate and humanize in the Go engine
(`engine.go:307`), six presets, a compact versioned share format, and tap tempo — all
pure, all unit-tested. What holds the score is that **every algorithmic control is global
and one-shot**: one probability knob for all 80 cells, one humanize, one length, one
pattern. It is an excellent generative-_assist_ step sequencer, not yet an algorithmic
drum machine.

- [ ] **G7: accent row / open hi-hat / master volume.** Partly superseded — accent shipped
      as a per-cell 3-state cycle, which is better than a separate row. Still open: the
      6th voice (open hat + choke group; `TrackCount = 5`, `engine.go:12`) and master
      volume (only a fixed `mixHeadroom` exists, `engine.go:21`).
- [ ] **G8 (high): probability and humanize are global, not per-step.** `engine.go:76`
      holds one scalar applied to every hit on every track. The original G2 asked for
      _per-step_ probability — this is the single biggest gap against the product name.
      Humanize is also strictly _late_ (`engine.go:328`), so the groove drags as it rises.
- [ ] **G9: no per-track length / polymeter.** One `stepCount` (`engine.go:71`) wraps all
      voices together (`:385`). Table stakes for algorithmic drums.
- [ ] **G10: no pattern banks, song or chain mode.** Exactly one pattern exists in the
      engine (`engine.go:63`) and the UI — no A/B, copy, queueing or chaining.
- [ ] **G11: no undo/redo.** MUTATE, CLEAR, preset load and Euclid FILL all destroy the
      current pattern irreversibly.
- [ ] **G12: no export or sync** — no offline WAV render, no MIDI export, no MIDI clock or
      input. Notable for an app whose engine is already a deterministic `Render(buf)`.
- [ ] **G13: Euclid is shallow** — `n` is forced to the global step count
      (`AlgoPanel.tsx:64`), fills at `VEL_NORMAL` only (`:68`), overwrites in one shot,
      and the k/rotation settings are neither persisted nor shared.
- [ ] **G14: the Tom track appears in zero presets** (verified across all six in
      `presets.ts:25-75`) — one of five voices is invisible to anyone exploring presets.
- [ ] **G15: swing is global and fixed-shape** (`engine.go:142`) — no per-track swing, no
      swing-8 vs swing-16 choice.

### To raise this score — feature depth

G8–G10 are the structural gaps. These are the features that would make the name earned
rather than aspirational, roughly in order of impact per unit of work — and all of them
build on an engine that already stores continuous per-cell velocity and renders
deterministically.

- [ ] **G16: conditional trigs.** Per-step conditions — every 2nd/3rd/4th pass, first-loop
      only, fill-only, not-if-previous-fired — are the single highest-value algorithmic
      feature per line of code, and the engine already has the per-step data structure to
      hang them on. This is what makes a pattern evolve without the user touching it.
- [ ] **G17: ratcheting / sub-step retrigger.** A per-step repeat count (2–4 hits inside
      one step, optionally with a velocity ramp) reuses the existing pending-trigger list
      (`engine.go:360`) and is the other classic generative gesture.
- [ ] **G18: expose continuous velocity.** The engine accepts any 0–1 value per cell, but
      the UI quantises to exactly three (`cycleVelocity`, `DrumMachine.tsx:27`). Let a
      drag or modifier set velocity freely — depth already paid for in the engine and
      currently thrown away at the UI layer.
- [ ] **G19: a density/seed generator.** One control pair — density plus a visible seed —
      that fills a track reproducibly would tie euclid, mutate and probability into a
      coherent generative story, and make shared links reproduce a _generator_ rather
      than a frozen snapshot.
- [x] **G20: expose per-voice tuning.** Done: 25 parameters across the five voices
      (`internal/drum/params.go`), reachable via `setVoiceParam(track, index, 0–1)`
      plus `triggerVoice` for auditioning, edited in a per-strip modal
      (`VoiceEditor.tsx`) driven by a generated descriptor table, and persisted in
      the v2 share/localStorage blob. Closes D11.
- [ ] **G21: a demo that plays itself.** The landing experience is a silent empty grid
      (see U19). Given presets, mutate and a share format already exist, an autoplaying
      demo pattern is nearly free and is what communicates "algorithmic" in five seconds.
- [ ] **G22: a second sound set for every voice.** Six of seven voices offer exactly one
      synthesis recipe; only the two Toms can be switched (`SetTomModel`, `engine.go:514`).
      Tracked in full as the waveguide voice-model path below (W1–W6), which generalizes
      that per-voice selector to all seven tracks.

---

## Suggested execution order

Where a structural item subsumes several defects, it is listed instead of them — P12
closes P5/P7-partial/P8, A13 closes A4/A8, and so on.

**P3 — hardening.** ✔ Done 2026-07-26: C20 (one validated boundary, closing C10/C12), C11
(odd-step swing), C13/C19 (JS arg validation), C14–C16 (bridge lifecycle), A5/A6/A12,
F10, C21 (fuzz), CI5–CI12, P7/P11, H6, D4–D11.
Still open from this phase: **E7** (re-opened — gain staging still lets an ordinary
pattern hard-clip 16 samples per 3 s; fix it and add a no-clipping test that can actually
fail), **B7** (a dropped `need` message deadlocks audio permanently), **T7/T9**
(`cmd/wasm` is still never compiled by `go test`; `audioWorker.ts` still has no direct
test). C17/C18 closed 2026-07-26 (stop-drain suppression + `dispose()`).

**P4 — reach:** U7/U8 (mobile grid, with U21's scale lever), X14 (a11y enforcement first,
so the rest stays fixed), X5/X17 (roving tabindex + skip link), X7/X8/X15 (playhead and
live regions), X9 (targets), P12 (generated SW → closes P5/P8) and P13 (update UX),
T10 (a DOM test environment — the precondition for testing any of the above).

**P5 — depth:** A13/A14/A16 (one state shape and one owner → closes A4, A7, A8), then
G8 (per-step probability), G16 (conditional trigs), G9 (per-track length), G10 (pattern
banks), G11 (undo), G18 (continuous velocity — already in the engine), F7 (split
`DrumMachine.tsx` once its state model settles).

**Quick wins, any time:** G14 (Tom absent from every preset), G21 + U19 (a default
pattern so the app makes a sound on first click), U18 (`?` shortcut overlay), CI11 (a
WASM size budget), H5 (take dependabot's first PRs).
---

## Physical drum synthesis path

A double-headed physical tom, selectable per Tom track, running beside the
procedural voices rather than replacing them. Modal banks per head, a lumped +
modal cavity between them, a Berger tension nonlinearity, a Rayleigh/Lommel
radiation weight and a stochastic attack layer above the modal ceiling.

**Its backlog is not here.** On 2026-08-06 the model, its fitting objective, its
offline tooling, its reference recordings and its fourteen design documents were
extracted to [github.com/cwbudde/algo-tom](https://github.com/cwbudde/algo-tom),
which algo-drum consumes as a module dependency — and the ~1,860 lines of
P-numbered and N-numbered backlog went with them, to
[algo-tom's PLAN.md](https://github.com/cwbudde/algo-tom/blob/main/PLAN.md).
Nothing about the model's physics, its objective, its gates or its measurement
discipline is decided in this repository any more.

What stays this repository's business is the product surface, and it is small
enough to state in full: the adapter (`internal/drum/physical_tom.go` and
`params.go`'s binding to `tomparams`), the digest that guards the shipped sound
(`TestPhysicalTomRenderIsBitExact`), the per-Tom model radio and eighteen-knob
editor, and the persistence and share-link versions that carry them. Those are
tracked in the numbered sections above, not here.

One item did stay behind and is worth naming, because algo-tom's backlog
describes it but cannot fix it: `web/src/algo/physicalTomPresets.ts` is a preset
bank fitted to a recording that no longer exists (algo-tom P10/N8). It ships from
here, so replacing it is this repository's item.

---

## Waveguide voice-model path

Origin: **PR #1** (`codex/add-sounds-combo-box-and-second-sound-set`, still open)
proposed a whole-kit `SetKit` switch plus five voices — `PMKick`, `PMSnare`,
`PMHat`, `PMTom`, `PMCymbal` — over one shared `waveguideDrum`. It can no longer
be applied: it predates `Trigger(velocity)`, `Reset()`, the parameter bank,
`math/rand/v2`, tracks 5–6, and the double-headed model that supersedes its
`PMTom` outright. Its DSP core is still worth having, so it is re-implemented
here as a **third per-voice model selection** on the mechanism `TomModel`
already established, rather than as a kit-level switch.

Naming: PR #1 called this "Physical Model". It is a damped comb loop with a
two-tap dispersion allpass — Karplus–Strong flavoured — not a physical model in
the sense of `internal/physical`. It ships as **Waveguide** so the two are not
conflated in the UI or in the code.

Disposition: the path exists to find out whether the recipe is good enough to
keep. If it is not, the answer is to delete it, not to grow it — the
double-headed model is the route to better sound (P8), and the per-voice
selector this path builds is what a second physical voice would land on anyway.

### Scope and architecture decision

- [ ] Generalize `TomModel` to `VoiceModel` over all seven tracks. Procedural
      stays the default everywhere; waveguide is offered on every track;
      physical stays Tom-only, enforced by one `modelAvailable(track, model)`
      predicate that `SetVoiceModel`, `validate.go` and the editor all read.
- [ ] Keep the persisted wire codes as they are: procedural `0`, physical `1`,
      waveguide `2`. Declaring waveguide `1` for a nicer reading order would
      silently reinterpret every existing v3–v10 physical-Tom link as waveguide.
- [ ] Give the waveguide bank its own capacity, accessor and persistence block,
      as `physicalTomSpecs` does. It must not widen `maxVoiceParams = 6`
      (`params.go:18`), which would shift procedural byte offsets.
- [ ] Build waveguide voices lazily on first selection. Seven delay lines that
      most users never select are the same argument that makes `physicalToms`
      lazy today (`engine.go:489`).

### W1 — Engine: per-voice model selection

- [ ] Replace `tomModels`/`proceduralToms`/`physicalToms` (`engine.go:101`) with
      `voiceModels`, `proceduralVoices`, `waveguideVoices`, `physicalToms`.
- [ ] `SetTomModel` → `SetVoiceModel(track, model)`, keeping today's discipline:
      reject an unavailable pair, lazily construct, `Reset()` both the outgoing
      and incoming voice so no dormant tail resumes, re-apply `e.decays[track]`,
      then swap. Extend `validate.go:124` to the generalized invariant.
- [ ] Rename the WASM method to `setVoiceModel(track, model)` and the bridge to
      match. No consumer outside this repo — the name is not a compatibility
      surface, unlike the persisted codes.

Exit: every track reports and honours its selected model, physical is rejected
on the five non-Tom tracks, and procedural renders stay bit-for-bit unchanged.

### W2 — The waveguide core

- [ ] New `internal/drum/waveguide.go` (+ `waveguide_test.go`); `voices.go` is
      already 733 lines. Port PR #1's `waveguideDrum` — damped comb loop, two-tap
      dispersion allpass, noise burst over the first third of the delay line.
- [ ] Bring it up to the four contracts it predates: `Trigger(velocity)` scaling
      the excitation burst rather than the output, so a soft hit carries less
      energy instead of being merely quieter; `Reset()` clearing line, envelope
      and index; `newVoiceRng` PCG seeding (`voices.go:140`) in place of the v1
      `rand.NewSource`; and a `paramBank`, with retuning preserving the tail.

Exit: each voice activates on trigger, deactivates at `envSilence`, renders
bit-exactly across runs, and leaves no tail after `Reset()`.

### W3 — Parameter tables

- [ ] Seven tables in `params.go` with PR #1's constants as their `Shipped`
      values, so the PR's sound is the default knob position. Core four on every
      voice — `PITCH`, `DAMP`, `DISP`, `DECAY` — plus: Bass `SUB`/`SUB.F`; Snare
      `NOISE`/`N.DEC`/`HP`/`HP.Q`; Hi-Hat `BP`/`BP.Q`/`N.DEC`/`RES`; Tom and
      Tom 2 `GAIN`; Cymbal `PITCH.B`/`BP`/`BP.Q`/`SHIM`/`S.DEC`; Percussion
      `RATIO`/`CLICK`. Cymbal is widest at nine, setting `maxWaveguideParams`.
- [ ] Invent the Percussion recipe: PR #1 predates track 6, so it has no
      counterpart. Same core, short and bright with a click, rather than leaving
      one track without the alternative.
- [ ] Extend `cmd/gen-voiceparams` to emit the waveguide mirror and capacity;
      CI already diffs the generated file.

Exit: `just gen-params` is a no-op on a clean tree and every waveguide knob
round-trips through `Map` at its documented precision.

### W4 — Frontend

- [ ] `web/src/engine/tomModel.ts` → `voiceModel.ts`: `VoiceModel` gains
      `"waveguide"`, plus a `modelsForTrack` table mirroring the engine rule —
      the editor must not offer physical on the Hi-Hat.
- [ ] Generalize the `VoiceEditor.tsx:157` fieldset from two hard-coded radios to
      one per available model, legend `"<Voice> synthesis model"`, with a
      waveguide entry in the existing help tooltip; `showProceduralParams`
      becomes a three-way spec/value selection.
- [ ] Collapse `tomModel`/`tom2Model` into `voiceModels` (length 7) and add
      `waveguideParams` in `DrumMachine.tsx`. The `TOM_TRACK`/`TOM2_TRACK`
      ternary chains at `:478` and `:644` become plain track indexing — a net
      simplification, and it pays down part of F-series component bulk.

Exit: every strip's editor offers its available models, switching is audible
without a reload, and no track offers a model the engine would reject.

### W5 — Persistence (v11)

- [ ] Append only, so nothing before it moves: widen the two existing Tom model
      bytes from `{0,1}` to `{0,1,2}` at their current offsets, then append five
      model bytes for tracks 0, 1, 2, 4, 6 and
      `TRACK_COUNT × WAVEGUIDE_PARAM_CAPACITY` waveguide positions, engine-major.
- [ ] Keep the Toms' codes in their existing bytes rather than duplicating them
      into the new block — one source of truth per track.
- [ ] A v10-or-earlier blob must decode exactly as today, `waveguideParams`
      absent and the call site supplying generated defaults, as
      `physicalTomParams` does. Add the v10 → v11 migration test alongside the
      existing v6 one.

Exit: v11 round-trips, every earlier version still decodes at its original
length, and a share link carries the model selection.

### W6 — Judgement: keep or delete

- [ ] Render and measure each waveguide voice — per-voice spectra and decay
      envelopes — rather than accepting that it builds and plays. "Is it good
      enough" is the entire point of the path, and it is the one question the
      unit tests cannot answer.
- [ ] Decide per voice, not per kit. The recipe may well earn its place on the
      Hi-Hat and Cymbal, where a comb loop is close to how the real thing
      behaves, and lose on the Bass, where the procedural sweep is stronger.
- [ ] If a voice does not earn it, delete that voice and its table. Leaving a
      weak alternative in the selector costs a UI choice, a persistence block and
      a maintenance surface for nothing.

Exit: a written verdict per voice, and PR #1 closed either way — merged in
spirit or declined with the measurement that settled it.

### Waveguide-path success criteria

1. Procedural renders and every v1–v10 persisted state are bit-for-bit unchanged
   when no waveguide voice is selected.
2. One selector mechanism serves all three models; no track can select a model
   the engine does not implement for it, at any layer.
3. The waveguide bank cannot shift a procedural or physical byte offset.
4. `Render` stays allocation-free with waveguide voices selected on all seven
   tracks, inside the WASM budget the physical path already measures against.
5. The name matches the mechanism: nothing calls this a physical model.
6. Every kept voice was kept on a measurement, and every dropped one was dropped
   on one.
