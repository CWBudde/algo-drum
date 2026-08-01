# algo-drum — Repository Review & Improvement Plan

Reviewed: 2026-07-09 · Re-reviewed: **2026-07-26** at `81dae31` · Hardening pass: **2026-07-26**.

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
| 3   | Go engine / DSP           | **7/10** | —   | Idiomatic, tested, allocation-free render; gain staging still lets transients hit the clamp |
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

**Overall: 7.2/10** (was 6.6 at re-review, 4.0 at first review). The hardening pass closed
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

Still measured open: an ordinary 3 s pattern hard-clips **16 samples** at ±1.0, so the
clamp rather than the limiter is enforcing the ceiling (**E7**).

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

## 3. Go engine / DSP — 7/10

The strongest area: idiomatic, well-commented, magic numbers lifted into documented
constants (`voices.go:22-79`), `math/rand/v2` with fixed per-voice seeds, an
allocation-free `Render` with a test asserting it, and coverage of clamping, swing
bar-length, velocity and humanize bounds. Gain staging is the outstanding problem.

- [ ] **E6: stop vs. pause semantics.** Still open and unchanged. `SetRunning(false)`
      (`engine.go:155`) resets `currentStep`/`stepSamples` and clears pending triggers;
      there is no `Pause()` in Go, no `pause()` in `wasmEngine.ts`, and
      `DrumMachine.tsx:249` is a binary toggle. Voices and reverb _do_ ring on after stop.
- [ ] **E7 (re-opened): the limiter still isn't limiting.** Previously marked closed.
      Output is now _guaranteed_ bounded by the hard clamp (`engine.go:410`), but that
      clamp — not the limiter — is doing the work: verified **16 samples clipped in 3 s**
      on an ordinary bass/snare/hat pattern at reverb 0.3, with the pre-limiter mix
      peaking around 1.8 despite `mixHeadroom = 0.5` (`engine.go:21`) and a −1 dBFS
      threshold (`engine.go:124`). `TestRenderOutputBoundedAndFinite` only asserts the
      clamp works, so it can never fail. Fix the gain staging (`hatGain 1.5`,
      `cymGain 1.2`) or the limiter usage, and add a test that asserts _no clipping_.
- [ ] **E8: reverb bypass is a discontinuity.** `engine.go:402` skips `ProcessSample`
      entirely at `reverbAmount == 0`, truncating the tail with a click and later dumping
      stale delay-line contents back out. `SetWet(0)` (`:288`) already mutes correctly.
- [ ] **E9: `firePending` is O(32) per sample** (`engine.go:360`) ≈ 1.5 M iterations/s,
      almost always all-inactive. An active count or free-list head makes it near-free.
- [ ] **E10: `Pattern()` allocates per call and is not rare.** `engine.go:253` allocates a
      fresh slice and `cmd/wasm/main.go:108` a `Float32Array` plus 80 `SetIndex` calls.
      The "called rarely (state sync)" comment (`main.go:106`) is stale — the pattern echo
      calls it on **every cell click** (`audioWorker.ts:188`). Reuse a persistent buffer.
- [ ] **E11: `SetPattern` is asymmetric** — a short slice leaves untouched cells alone
      (`engine.go:241`) while `getPattern` always returns 80, so partial set + get is a
      merge, not a replace.
- [ ] **E12: `Voice.IsActive()` is dead outside tests** (`voices.go:17`). Either use it to
      skip `Tick()` on inactive voices — a real saving — or drop it from the interface.
- [ ] **E13: `stepLen` truncation has no error accumulator** (`engine.go:148`), dropping
      up to ~0.5 samples/step at non-integer BPM. Irrelevant standalone; matters the
      moment anything external syncs to it.

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

**This section is the forward backlog.** The phases that built the model are
closed; what they established lives in `docs/physical-*.md`, and each phase below
points at the document that holds its evidence. Numbers are not repeated here —
if a figure appears in both places, the document is the one to trust.

Research record and primary sources:
[`docs/physical-model-research.md`](docs/physical-model-research.md).

### Where it stands

| Phase                                           | What it settled                                                                                                                                                                                                                                               | Record                                                                                                       |
| ----------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------ |
| **P0–P2** contracts, modal bank, loss/radiation | Parameter structs, quality tiers, allocation-free `Render`, Fourier–Bessel modes, exact state transitions, the frequency-dependent loss law and the radiation weight.                                                                                         | [`physical-calibration.md`](docs/physical-calibration.md)                                                    |
| **P3** resonant head + cavity                   | Passive rank-one pressure coupling, validated against an offline frequency-domain solve.                                                                                                                                                                      | [`physical-cavity.md`](docs/physical-cavity.md)                                                              |
| **P4** nonlinear hit behaviour                  | Bounded Berger potential, discrete-gradient passivity, anti-alias bound, measured glide.                                                                                                                                                                      | [`physical-nonlinearity.md`](docs/physical-nonlinearity.md)                                                  |
| **P5** product integration                      | Generated control bank, A/B against the procedural path, versioned persistence and share links.                                                                                                                                                               | [`physical-product-integration.md`](docs/physical-product-integration.md)                                    |
| **P6** real-instrument departures               | Deterministic degenerate splitting and a rotated tension axis. Shell/edge/vent corrections were **refused** for want of a measurement, and the acceptance gate written down.                                                                                  | [`physical-real-instrument-departures.md`](docs/physical-real-instrument-departures.md)                      |
| **P8** sound correction (S1–S10)                | The voice stopped sounding like a ringing sine: the `d₁k` loss term and constant Q, the damped (0,1), the fitted cavity split, the corrected radiated sum, the audible glide, the hybrid attack layer, and the retune that made tuning stop changing sustain. | [`physical-tom-review.md`](docs/physical-tom-review.md), [`physical-hybrid.md`](docs/physical-hybrid.md)     |
| **P9** model-structure gaps (M1–M7)             | Nonlinear mode-to-mode coupling (M1) and a modal cavity (M2) — the latter being **the first externally checkable prediction this model made and had come true**, a rigid-cylinder air mode confirmed against sound speed, radius, depth and head tension.     | [`physical-nonlinearity.md`](docs/physical-nonlinearity.md), [`physical-cavity.md`](docs/physical-cavity.md) |
| **P10** the objective itself                    | The instrument used to judge P8 and P9 **cannot resolve most of what it reports**. Gates re-derived from measured reproducibility; the reference replaced with a licensed one of known geometry.                                                              | [`physical-objective-validation.md`](docs/physical-objective-validation.md)                                  |

Two things from that history are worth restating because they constrain
everything after them:

- **Six rounds of intervention concluded "the model is the ceiling" from evidence
  that could not support it.** Every partial-based conclusion drawn before
  2026-08-01 is withdrawn rather than adjusted. Decomposing a metric before
  optimising against it is cheap; not doing so cost this project months.
- **Four separate defects were found in tests that passed _because_ of the defect
  they were meant to guard.** When a physical correction lands and a test fails,
  the test is the first suspect, not the last.

### Open work (P10)

Ordered. N2 gates the measurement-dependent items; N9–N13 are independent of any
recording and can be taken at any time.

- [x] **N1: derive the adoption gates from measured reproducibility.** _Done
      2026-08-01._ Each gate is now the p90 of the objective's disagreement with
      itself over the sixteen velocities of the coincident reference pair, scored
      both ways round, and each weight is `1/gate` — a rule `AdoptionGates()` and
      `TestWeightsAreReciprocalGates` now make structural rather than aspirational.

      Three findings came out of it. The **spectral envelope was the only gate that
          was ever right** (measured floor 3.65 dB against a gate of 4). The **partial
          terms were never gateable** — nothing can beat 113 cents or a 1.26 log-ratio
          — so their gates are now honest and, as quality statements, useless. And
          **attack balance was the most reproducible term in the objective while
          carrying the smallest weight**, `1/6` against a 0.26 dB median; it is now
          `1/1.2`.

          The arithmetic that matters: at the old weights the objective's
          self-disagreement totalled **12.89** while the best fit ever recorded scored
          **10.38** — below its own noise floor. At the measured weights the floor is
          **4.32 median / 6.46 at p90** and no term contributes more than 0.79 at its
          median. **No total recorded before this change is comparable to any total
          after it.**

          Not done here, and left to N2 because it needs the repaired estimators: RMS
          aggregation is still used where the terms are outlier- rather than
          noise-dominated.

          **Superseded, and the numbers below are stale.** N2 found that the estimator
          these gates were measured through was silently collapsing three of the
          sixteen takes to a one-partial table, and re-measured them: five of the nine
          moved, all tighter. The reciprocal-gate rule and two of the three findings
          stand; "the partial terms were never gateable" is withdrawn, and the numbers
          are superseded by N2's. This item is left in place because N2's separation of
          the two causes is measured against it.

- [x] **N2: repair the partial and decay estimators.** _Done 2026-08-01._ Evidence
      and every number below:
      [`physical-objective-validation.md`](docs/physical-objective-validation.md)
      §Result 5 and §Result 7.

      Subband ESPRIT with a stabilisation sweep exists beside the fast estimator
          (`internal/physical/match/esprit.go`, its dense complex linear algebra in
          `linalg.go`, exposed by `measure-tom -high-resolution`). It is measurement
          equipment — no fit calls it and `Distance` does not know it exists. The
          partial-by-partial comparison this item gated everything on was run across
          all sixteen velocities, and produced:

  - **A defect worse than either this item named, now fixed.** The decay fit was
    admitted on a sample count with no bound on the time it spans, so a 6.1 ms
    fragment on `v10` was fitted at −4034 dB/s and extrapolated back to a level of
    **+137 dB**, putting every genuine partial below the relative floor. Three of
    sixteen takes were reduced to one or two partials and the fundamental was
    reported as 2349.6 Hz. All sixteen now yield sixteen. The guard is that no
    fitted decay may be faster than the envelope filter's own fastest pole.
  - **The gates re-measured, by repository code.** `cmd/measure-objective` now runs
    the campaign through the real `match.Distance`, rather than the standalone
    reimplementation Result 1 relied on, and refuses a non-coincident pair instead
    of trusting the operator. Five of the nine gates moved, every one of them
    **tighter**: frequency 113 → 80 ¢, level 17.85 → 7 dB, decay 1.262 → 0.6,
    unmatched 0.880 → 0.3, spurious 0.346 → 0.3. The target got harder, as this
    item required.
  - **The two causes separated by measurement.** Re-running with the trimming
    disabled shows the estimator repair fixed level, decay, unmatched and
    spurious — those four were measuring the collapsed takes and nothing else —
    and the trimming fixed frequency, which the repair did not touch. Two
    defects, neither substituting for the other.
  - **RMS replaced by a trimmed RMS** in the three partial terms: the smallest
    80 % of squared errors, nothing discarded below five pairs. This is aimed at
    the estimator's tail, not at letting a model off — Unmatched, Spurious and
    SpectralEnvelope are untrimmed, so anything a candidate fails to produce or
    invents is still charged in full.
  - **The decay term's confidence weighting is gone rather than replaced.** R²
    does not discriminate (median |ΔT60| 40 % at R² ≥ 0.95, 55 % below). Its
    replacement candidate, `DecayRangeDB`, was implemented, measured on the same
    153 pairs, and does not discriminate either — it is not even monotone. An
    unmeasured confidence is worse than none, because it reads as a guard. The
    field is still reported; the term weights by nothing.
  - **Karjalainen et al. (2002) implemented** — `decayFloorFit`, the explicit
    exponential-plus-noise-floor model, Levenberg-Marquardt from the log-linear
    estimate over the whole window. It is exact where truncation reads **+930 %**
    long, on an ordinary configuration. On the licensed reference it changes
    nothing measurable: 53 of 108 pairings move closer to the subspace estimate,
    which is a coin flip. It is kept because it is right where the old one is
    wrong and removes a threshold whose correctness depended on the signal — not
    because it improved this reference, and it did not.
  - **ESTER is unusable here** — its criterion is not unimodal on this signal and
    its argmax picks order 1 for a band holding four partials. Implemented,
    reported, not used.

    Two predictions this item made were **refuted by the measurements it asked
    for**, and both are load-bearing for what comes next:

  - The ring-time disagreement between the two estimators (median +27 %, fast
    longer in 63 % of pairs) is **not an estimator defect**. Both recover a known
    answer to within 3 % in the exact close-neighbour configuration the drum
    imposes, and the Karjalainen model — the last remaining candidate repair —
    moved it by nothing. What remains is that the reference's partials are not
    single exponentials, which is a property of the drum. Three independent
    measurements now say so. **Do not spend another round trying to fix it in
    code.**
  - "The partial terms were never gateable" was true of the collapsed
    measurement and is **withdrawn**. 80 cents and 7 dB are wide, but they are
    thresholds a model can be held to.

    Left deliberately undone: **the merged pairs are not resolved in the fast
    estimator**, and `ASYM` was still being fitted against a target with the
    asymmetry averaged out (it no longer is — N15 pinned it). `MinSeparationHz` is not the whole of it — an FFT peak picker
    over an 800 ms Hann window cannot separate 4 Hz at 213 Hz whatever the guard
    is set to — so this is not a threshold change but a second estimator in the
    fit loop, at seconds per candidate against milliseconds. It was carried to
    **N15**, which closed it by pinning `ASYM`; the merging itself is still
    unfixed, and is now the only part of this that remains open.

- [ ] **N3: fix the damping distribution — the one real model defect.**
      _Re-scoped 2026-08-01; the item as previously written named the wrong mode
      and prescribed a fix that cannot reach it._ Evidence:
      [`physical-objective-validation.md`](docs/physical-objective-validation.md)
      §Result 8.

      What was wrong with it. The instance — "a mode at 186 Hz with T60 1.81 s,
          the longest-ringing thing the model produces" — is the **batter head's
          (1,1)**, seen at the fitted config where DAMP and D.TILT scale the k¹ term
          down by ~2.6×. It is not a doublet member and not axisymmetric: every m > 0
          mode has a swept area of exactly zero, so it has no path to the cavity
          compliance at all, only to a transverse mode at 907 Hz. The prescribed fix,
          "damp the coupled (0,1) doublet specifically", therefore moves it by
          nothing — and the (0,1) is already the most heavily damped mode in the bank,
          its correction more than twice its structural rate.

          Nor is 186 Hz an accident of the fit. γ is monotone in k with exactly one
          exception, the (0,1) correction, so **the longest-ringing mode is forced to
          be the lowest-wavenumber mode the correction table does not name** — the
          (1,1), at every tuning and every value of DAMP and D.TILT, both of which
          scale the whole law. `TestTheLongestRingingModeIsTheLowestUncorrectedOne`
          pins this so the next round cannot re-derive it from scratch.

          **The sign-pattern check this item demanded has been run**, using the
          subspace estimator to resolve the pairs the fast one merges. Fourteen
          two-member pairs:

  - **The pairwise structure is real and large.** Within a resolved pair the ring
    times differ by a median factor of **1.55** (min 1.11, max 7.25) across a
    1–6 % frequency split, where any smooth γ(k) gives 1.00. No smooth loss law
    can express this. That half of the conjecture is confirmed, and it is the
    strongest single piece of evidence this path has for what the model lacks.
  - **The predicted sign is absent.** A cavity doublet's squeezing member is the
    upper branch and should always be the more damped one; the upper member
    decays faster in **6 of 13** pairs. On this evidence the alternation is not
    the in-phase/out-of-phase signature, so **do not implement squeezing/sliding
    damping as the fix** — that is what the check was for.
  - Bounding the conclusion: every resolved pair sits between 300 Hz and 2.7 kHz.
    No pair was resolved at the (0,1), so this is silent about the fundamental
    specifically. What it establishes is that the missing freedom spans the whole
    retained band rather than only m = 0, which is a **larger** gap than this item
    described and points away from the cavity as its cause.
  - **Re-measured on the low-pitch reference, 2026-08-01** (§Result 11d): 29 clean
    pairs, median ratio **1.39** (1.04–4.86), upper member faster in 18 of 29.
    Both halves survive the change of drum — the split is large, the sign is not
    there. Pairs are now resolved near the fundamental as well, but §Result 11c
    disqualifies the subspace estimator there, so they are excluded rather than
    read as the doublet appearing.

    So the threads are now:

  - ~~**Find what the pairwise splitting actually is.**~~ **Answered 2026-08-01;
    it is the drum.** Evidence: [`physical-objective-validation.md`](docs/physical-objective-validation.md)
    §Result 9. Pairs were synthesised at the frequencies and splits the resolved
    pairs actually had, with the upper member 0/3/6 dB down, and **both members
    given identical damping**: the estimator reports a mean ratio of **1.001**,
    worst 1.003, against the measured 1.55. Given a true 1.55 it returns
    1.549–1.551 across every cell, so it is not merely insensitive. Where the
    split is too narrow it **merges** rather than splitting — the conservative
    failure, and the one that keeps the control valid. Both directions are pinned
    by `TestEqualDampingIsNotSplitByTheEstimator` and
    `TestARealDampingSplitIsRecoveredAtItsMeasuredSize`.

    So the model is missing a real per-pair damping freedom. Real heads split
    degenerate pairs through thickness and tension inhomogeneity (Worland, JASA
    2010), which splits **frequency** — `ASYM` models that — but the measurement
    says the two members also **decay** differently, which `ASYM` does not touch.
    A per-pair damping split with no predicted sign is what the evidence supports,
    and it is now measurement-backed rather than conjectural. What remains open is
    the mechanism, not the phenomenon.

  - **The (1,1) is a separate problem** and the reachable lever for it is the
    shape of γ(k), not the cavity. Fitting a smooth power law to the reference's
    own T60s leaves 0.677 and the model already achieves 0.573, so a smooth law
    is not it either. A second correction-table entry is the cheap experiment and
    is honest only if it is labelled as fitted.

    **Re-aimed 2026-08-01, before that experiment was run.** Evidence:
    [`physical-objective-validation.md`](docs/physical-objective-validation.md)
    §Result 10, and **re-measured post-N17 as §Result 11a — use that one**. The
    committed reference's ring time was measured across all sixteen takes and it
    falls as **T60 ∝ f^-0.70** (f^-0.52 through the old truncating window), between
    the f^-1 the loss law is calibrated to and the f^0 a d0-dominant law gives.
    Anchored at 240 Hz, constant ζ predicts 102 ms at 2.6 kHz against a measured
    207 ms: the law is not wrong in kind, it is about twice too steep.
    The medium-pitch set that was the reference until this date gives an exponent
    near **zero** on the same measurement, so two tunings of one drum disagree
    and neither supports 1/f. That disagreement is a caution, not a result: do
    not re-calibrate a shipped law on one recording.

    So Result 8's structure — "the lowest uncorrected mode is forced to ring
    longest" — is a consequence of the law's **slope**, and the slope is already
    a product knob: `D.TILT` scales d₁ and d₂ and leaves d₀ alone, over a 0–3
    range whose zero is the flat law and whose 1 is the calibrated constant-Q
    one. An f^-0.70 target puts the answer between the ends rather than at
    either, which is what makes it useful: the fit should land `D.TILT` well
    below 1 and not at its stop, and a bank that pins it at 0 is reporting an
    artefact rather than this drum. A correction entry at the (1,1) would be
    patching one mode of a law whose slope is the thing the reference disagrees
    with. **Run the fit before the entry** — the first ever made against this
    reference — and read it for where `D.TILT` lands.

  - **N17 gated that fit and is now done** (2026-08-01): the windows are 2.0 s /
    1.60 s and a partial can no longer be credited with a ring time its window
    did not show. Its consequence is also discharged — `measure-objective` was
    re-run through the repaired estimator and the gates re-edited from it (see
    N17), so a total from this objective can now be read against a measured floor
    of 6.54 / 7.86. Result 10's −0.52 has been re-measured too, and the guess was
    right in direction and larger than expected in size: **−0.70**, Result 11a.

    **Nothing further gates the fit.** N3's remaining content is: run
    `just fit-physical-series reference/tt08x08/lp/hd`, read where `D.TILT`
    lands, and only then decide about a correction entry.

  - **The instrument for that experiment exists**, so it is not what the next
    round is spent on: `cmd/fit-physical -mode-correction m,n=perSecond` adds an
    entry to both heads' correction tables, replacing rather than appending — the
    configuration rejects two rates for one mode. It is deliberately shaped like
    `-loss-scale`: not a knob, recorded in the report, and part of the checkpoint
    fingerprint. It is the quieter of the two fingerprint hazards, because unlike
    `-search-blind` it does not change the width of a position vector — a resume
    across it would read every stored point correctly and score it against a
    different drum, and nothing in the report would look wrong. The rate given is
    the **effective** one, applied after DAMP and D.TILT, which do scale the
    table's own entries; a value measured this way has to be divided by that
    product before it could become a default.
  - **Before fitting any per-mode damping vector**, the standing rule still
    applies and has now been shown to earn its keep: check the structure first.
    It cost one afternoon and it refuted the mechanism this item was going to
    spend a round implementing.

- [x] **N15: decide what to do about `ASYM` and the merged pairs.** _Split out of
      N2 and closed the same day, 2026-08-01._ The fast estimator merges 15–24 of ~160 matched partials
      into single peaks, recurring at 304, 351, 586–613 and 851 Hz, so `ASYM` is
      fitted against a target with the asymmetry averaged out of it. This is not a
      guard setting: an FFT peak picker over an 800 ms Hann window cannot separate
      4 Hz at 213 Hz at any value of `MinSeparationHz`.

      Three options, and the choice is a real one:

  - Put the subspace estimator in the fit loop. Correct, and costs seconds per
    candidate against milliseconds — a fit is ~90 000 evaluations.
  - Run it on the reference only. **Rejected as written**: reference and candidate
    would then be measured by different instruments, and the partial terms would
    be scoring the difference between two estimators as if it were the model's
    error. N2 measured that difference at a median 43 % on ring time.
  - Pin `ASYM` and stop fitting it. Cheapest, and honest — a parameter the
    objective cannot see should not be reported as fitted. It stays a user knob.

    **Unblocked 2026-08-01, and the answer is the third option.** N3's first
    thread resolved the way that decides this: the two members of a resolved
    pair differ in **damping** (a real 1.55, Result 9), and `ASYM` splits only
    **frequency**. So `ASYM` is not the parameter that would represent what was
    measured, and repairing the target it is fitted against would not make it
    the right one. Pin it, stop fitting it, keep it as a user knob — and record
    in the same change that the objective cannot see it, so that a later round
    does not re-add it to the search as an oversight.

    Putting the subspace estimator in the fit loop remains the correct fix for
    the merging itself, and remains unaffordable at seconds per candidate
    against ~90 000 evaluations. That is a separate question from `ASYM`, and it
    should not be reopened on `ASYM`'s account.

    **Implemented.** `cmd/fit-physical` now holds a named list of parameters the
    objective is measured to be blind to (`blindParameters`) out of the search,
    at their defaults; `ASYM` is its only member, and the list carries both
    measurements rather than the conclusion alone. A report marks such a
    parameter `blind` as well as `fixed`, so a reader can tell a value the
    caller pinned from one that carries no information about the reference.
    `-search-blind` puts them back for the deliberate experiment of re-testing
    the claim, and is part of the checkpoint fingerprint because it changes the
    width of the search space. Five tests pin it, including that the list has
    exactly one member — so a later addition has to be a deliberate edit with a
    measurement behind it.

- [x] **N17: re-size the analysis and decay windows for the new reference.**
      _Opened and closed 2026-08-01._ **Done:** `analysisSeconds` 1.2 → 2.0,
      `decayFitEndSeconds` 0.60 → 1.60, the `tail` spectral window 1.2 → 2.0 so it
      still ends where the analysis does, and `cmd/fit-physical -duration` now
      *defaults from* `match.DefaultOptions().AnalysisSeconds` and refuses a value
      below it, because the two being separate literals is how they drifted 0.4 s
      apart in the first place.

      The window end was measured rather than chosen. Re-extracting all sixteen
      takes at five window ends and matching partial-to-partial by frequency
      within 0.5 %, the median |log2 T60 ratio| below 1 kHz between successive
      ends is **18.9 % → 8.1 % → 4.1 % → 1.4 %** at 0.60, 0.90, 1.20, 1.60,
      1.90 s. 1.60 is where it converges; above 1 kHz it was settled at 1 %
      throughout, confirming this was a low-band problem specifically.

      Three things came out of it that were not in the item as written:

  - **The guard's criterion is the *fall*, not the duration.** "No fitted T60 may
    exceed the analysed span" — the rule this item asked for — is wrong, and
    measuring it is what showed that: twelve of this reference's partials
    legitimately ring longer than the 2.08 s file, and they are fine, because
    they fell 37 dB while the window was open. What shipped instead is
    `slowestSupportedT60`: a partial must fall at least 20 dB inside its fit
    window, so **T60 may not exceed three times the window**. 20 dB is ISO 3382's
    own floor — it defines T20 and T30 precisely because a 60 dB fall is rarely
    observed, and sanctions nothing shorter. The guard is *decisive* at the old
    window (max T60 10.39 s → 1.51 s, the ~358 Hz runaway gone) and *inert* at
    the new one (nothing rejected, output bit-identical to the pre-guard run) —
    which is the right shape: the guard states the standard, the window is what
    lets the reference meet it.
  - **A long window is not free, and the fix is per partial.** The refinement
    (`decayFloorFit`) was fitted over the whole window. Its model is one
    exponential plus a **stationary** floor, and past the point a partial has
    fallen below its own floor its band holds a neighbour's *decaying* skirt,
    which the model can only absorb by bending the first exponential towards it.
    Two partials 27.7 Hz apart, the lower ringing 0.21 s and the longer-lived
    upper one 0.64 s: at a 0.60 s window the lower reads T60 0.240 s at its true
    level; at 1.60 s it reads **0.443 s at −21.5 dB**. The level is the worse
    error — it is the fitted line extrapolated back to the strike. The refinement
    is now bounded per partial at twice the span the partial stood above its own
    floor: one span of decay to fit, one of floor to identify the floor, nothing
    beyond. Without this, widening the window would have been a net regression.
  - **It repaired half of a defect N2 documents, by accident.** The fast
    estimator's merged degenerate split was biased 5–6 % in ring time because it
    was fitted over a beating envelope; the beat's later lobes now fall outside
    the bound, and the bias is 1.2 %. The defect N2 is actually about is
    untouched — the pair is still merged and the 30 %-shorter member still lost
    entirely — so `TestFFTEstimatorMergesADegenerateSplit` records the change
    rather than celebrating it. An accurate ring time for the survivor makes it
    look more trustworthy than it is.

    Fixture fallout worth knowing, since it will recur the next time a window
    moves: a *noiseless* synthetic exponential is a degenerate input to a
    floor-fitting estimator — the floor parameter has nothing to identify it, so
    the fit degrades the further the window runs. Synthetic hits are now
    `testHitSeconds` long (derived from `AnalysisSeconds`, plus margin for the
    zero-phase filter's edge transient, which rings at *both* ends) and carry a
    `testNoiseFloorDB` = −120 dB floor relative to peak. Relative, not absolute,
    or `TestExtractIsGainInvariant` measures the fixture's noise instead of the
    extractor.

    **The gates are re-measured and re-edited, and doing it found the one defect
    the N17 work could not see.** `cmd/measure-objective` re-run on the sixteen
    lp/hd takes through the post-N17 estimator came back *worse than pre-N17 on
    every term that reads the partial table*: glide 2.3 ¢ → 286 ¢, level 6.76 →
    11.23 dB, unmatched 0.280 → 0.434, and partial decay 0.589 → 0.745 — the term
    the widening was for. Only spectral envelope and attack balance were
    unmoved, and they are exactly the two terms that do not read that table.

    The cause was the per-partial refinement bound, not the widening. The bound
    had no lower limit, so a fast partial got a refinement window two of its own
    spans wide — 105 points for a 26 ms partial — and the three-parameter fit
    could then trade a steep slope against a low floor for free. On v16's right
    channel a 2511.8 Hz component came back at T60 45 ms with an intercept of
    +31.4 dB against its own observed peak of −41.2 dB, became the loudest thing
    in the table, and pushed a sixteen-partial table down to two, both above
    2 kHz, on a drum whose fundamental is 240 Hz. Across the set, both channels
    named the ~240 Hz fundamental as loudest on 15 of 16 takes before N17 and on
    7 of 16 after it. `measureGlide` picks its partial off that table, which is
    the whole path from a decay-window bound to a 120× glide regression.

    `minimumRefinementSpanSeconds` = 0.8 s is the repair, and the constant's
    comment carries the sweep it was chosen from. Post-repair every term is at or
    better than pre-N17 except glide, which recovers to 23.4 ¢ and probably
    cannot return to 2.3 ¢ — that number belonged to a level table that no longer
    exists. Gates now: decay 0.6 → **0.55**, glide 10 → **30**, unmatched and
    spurious 0.3 → **0.25**; frequency 70, level 7, spectrum 3.5, envelope 1.5
    and attack 0.9 unchanged. The floor is **6.54 median / 7.86 p90**.

    **The transferable lesson, and the reason this is written out at length:** an
    estimator change justified by a *stability* measurement must be checked
    against a *reproducibility* one before it ships. N17's window study measured
    how much a ring time moves when the window moves, and by that measure the
    bound was an improvement. The two-microphone floor measures something the
    stability study structurally cannot — whether the estimator says the same
    thing about the same event twice — and by that measure the bound was a
    regression large enough to break a different term entirely.

    **Next:** N16's Results 2–10 re-measured against these gates, then N3's fit.
    Results 5, 8 and 10 are now done — objective-validation Result 11.

<details><summary>The item as originally written</summary>

- [ ] **N17: re-size the analysis and decay windows for the new reference.**
      _Opened 2026-08-01, when the reference became `tt08x08/lp/hd`._ Evidence:
      [`physical-objective-validation.md`](docs/physical-objective-validation.md)
      §Result 10. `match.Options` ships `analysisSeconds` 1.2 and
      `decayFitEndSeconds` 0.6. Those were sized against the medium-pitch set —
      1.25 s files whose fundamental rang for 0.28 s — and the set that replaced
      it is 2.08 s files whose fundamental rings for 0.686 s.

      Measured on the new reference, at the shipped window:

  - **70 of 256 partials (27.3 %) are assigned a ring time longer than the 0.6 s
    window they were fitted in**, and the estimate below 1 kHz moves by 19 % when
    the window is extended to 0.9 s and 26 % at 1.3 s — in the same direction,
    so this is the window failing to span the decay rather than noise. Above
    1 kHz it is settled at 1 %, so this is a low-band problem specifically, which
    is exactly the band N3 is about.
  - **11 partials (4.3 %) are assigned a ring time longer than the 2.08 s file**,
    up to 10.4 s, all of them the same ~358 Hz component across takes at R² 0.12–
    0.66. A ring time longer than the recording is not a measurement. N2's guard
    does not catch it: that one bounds a fit against the envelope filter's
    fastest pole, which is the opposite failure. **A second guard is wanted — no
    fitted T60 may exceed the analysed span** — and it should report rather than
    clamp, because a partial that cannot be measured is not the same thing as one
    that decays slowly.

    Three things follow, and the order matters:

  - The windows are not free parameters to be tuned until a number improves.
    Widening `analysisSeconds` toward the 2.08 s the files hold is bounded by
    what the *model* is rendered for (`-duration`, 1.2 s) — both sides must move
    together or the candidate is scored over a window it was never rendered
    across.
  - **Every measured gate is a property of the estimator, and these gates were
    measured through the current windows.** `cmd/measure-objective` has to be
    re-run on the new reference after any window change, exactly as N1 and N2
    each re-ran it. The lp/hd gates now in `DefaultWeights` were measured at the
    shipped windows and are correct for them and nothing else.
  - **N3's fit waits on this**, and Result 10's own slope carries the same
    caveat: truncation shortens long decays, so the true exponent is if anything
    steeper than −0.52.

</details>

- [ ] **N4: recompute the cavity stiffness against a known-geometry drum.**
      `Cavity.StiffnessScale` ships at 0.083, a factor of twelve below its physical
      ceiling of 1 — and there is **no factor of twelve to explain**: all four
      candidate mechanisms are eliminated in
      [`physical-cavity.md`](docs/physical-cavity.md), and 0.083 was fitted to a
      doublet ratio of 1.16 **measured on a snare**, which is a leakier enclosure
      than a tom. The ceiling was right and the target was wrong.

      The licensed 8" × 8" reference unblocks this: known geometry, stated head
          gauges, so the split is computable rather than fitted. Four costs are known
          and none is small — the parameter **saturates rather than lands** (s = 1 gives
          1.841 exact / 1.830 rendered, re-measured 2026-08-01, still 2.7 % below Bork &
          Meyer's 1.891, so shipping it ships a pinned parameter); the **interleaving
          constraint is unmet and is the stronger one**; the **glide gets diluted**, so
          retarget it and the Berger coefficients together or neither; and M2's
          confirmation is a weak-coupling result that degrades as s rises. **Retarget
          all the way or not at all** — the intermediate values are the worst, and three
          tests encode the snare target and must be re-quoted with it.

- [ ] **N5: refit jointly across all sixteen velocities.** `tt08x08/lp/hd`, mono —
      safe here because the pair is coincident. Pin what is now known (diameter,
      depth, both surface densities from the stated Remo gauges) and fit the rest
      against the whole velocity curve rather than one hit.

      **The Berger justification below is withdrawn for this set, 2026-08-01.**
      Objective-validation Result 11e: on `lp/hd` the glide is +18 ¢ median, all
      sixteen takes positive, three unmeasurable, fifteen of sixteen at or below
      the 40 ¢ readability threshold, and with no trend against velocity. The
      curve quoted below is `mp/hd`'s. What is left of the case for a joint fit is
      the narrower one — one bank against sixteen takes so that contact stiffness
      and nonlinearity cannot trade against a single assumed strike, and sixteen
      independent measurements of the file-order claim — which is worth having but
      is not an identification signal for `BERGER`. Do not read whatever `BERGER`
      lands on as a measurement of tension nonlinearity.

      This is what finally constrains the Berger nonlinearity, including
          `CoefficientNPerM` — which ships at the same coefficient the uniform channel
          already carries, so it is not a new free parameter but not a fitted one
          either. The measured glide rises monotonically with strike velocity (−130 ¢ at
          v04, −174 at v08, −353 at v12), which is the cleanest identification signal on
          the path. Expect little from the mode-coupling term itself: its measured reach
          into the target band is ~2 dB and already at its useful maximum.

          This also closes P6's one unmet clause — a documented preset fitted *with
          provenance*, which was impossible while the recording had none.

          Practical constraint: the analysis window is 1.2 s and the MP files are
          1.25 s; the higher tunings are shorter and need the tail window shortened
          before they can be used. Superseded for the current reference by N17,
          which sizes both windows against the 2.08 s low-pitch files.

      **The joint fit exists as of 2026-08-01; what remains of this item is
      running it and reading the result.** `cmd/fit-physical -reference` is
      repeatable, and every take given is scored by one shared parameter bank.
      The search space grows by one dimension per take — `len(free) + N`, one
      strike velocity each — the objective is the **mean** of the per-take
      distances, and the report carries a `takes[]` entry per file with that
      take's velocity, terms and features beside the one bank they were all
      fitted from. `just fit-physical-series <directory>` is the recipe; it reads
      the geometry off the path exactly as `fit-physical` does and names the
      output after the series. One model serves every take inside an evaluation
      — `Reset` between strikes rather than a fresh `NewDoubleHead`, which is
      bit-exact against a fresh one and has to be, since the checkpoint
      fingerprint carries the baseline cost. Cost: sixteen renders and sixteen
      extractions per candidate, so roughly sixteen times a single-file run.

      The aggregate is the plain mean, deliberately. A trimmed or median
      aggregate across takes would discard whichever hits the model fits worst,
      which is exactly the evidence this item exists to use; the trimming that
      *is* justified happens one level down inside `Distance`, over partials,
      on a measured argument. `TestJointAggregateIsTheMeanOfTheTakes` pins it.

      **The velocity labelling is measured, not assumed** — added because the
      order may be wrong. The takes are named v01…v16 in what the pack calls
      increasing strike order and were played by hand, so that ordering is a
      claim rather than data. Nothing in the fit reads it: each take carries its
      own free velocity, no take is constrained to be struck harder than the one
      before it, and the takes never see each other.
      `TestJointCostIgnoresTheOrderTheTakesWereGivenIn` pins that as a bit-exact
      invariance under reversing the list. The fitted velocities are therefore an
      *independent* read on the labelling, and the summary prints them against
      the file order and counts the steps where the two disagree. It reports a
      count and does not re-order anything: a genuinely non-monotone series and a
      mislabelled one are indistinguishable from here, and renaming files to
      improve a number is the opposite of a measurement.

      **The numbers below are stated against the file order and inherit its
      uncertainty.** The +0.64 dB/step attack-balance trend and the soft-half /
      hard-half glide split both bin the takes by index. That the trend is
      R² = 0.78 rather than 0.98 is consistent with a labelling that is roughly
      but not exactly right. The first joint fit's velocities are what settles
      it, and re-deriving both against the fitted order — rather than the file
      order — is part of reading its result.

      **Geometry is pinned as of 2026-08-01, and no longer part of this item.**
      `cmd/fit-physical` gained `-set`, which freezes a parameter at a value in its
      own unit rather than at a normalized position — `drum.ParamSpec.Unmap` is the
      inverse of the curve `Map` applies, so the caller states 0.2032 m and the
      model receives 0.2032 m. `just fit-physical` reads `<diameter>x<depth>` off
      the reference path and passes both, so pointing it at another pack moves the
      geometry with it and a recording outside `reference/<WxH>/` leaves them free
      and says so. This matters more than it sounds: `SIZE` and `DEPTH` default to
      0.3048 m and 0.20 m, so every fit against this 8" × 8" reference up to now
      was free to answer it with a 12" head on a 20 cm shell — and did, without
      anything in the report marking it as odd. Two of eighteen parameters are now
      constants the recording cannot argue with. The head gauges are still fitted;
      turning the stated Remo ply thicknesses into surface densities is the
      remaining half of this paragraph.

      **What the sixteen takes determine, measured 2026-08-01** — with the base
      rule repaired (see below), on `tt08x08/lp/hd`, `cmd/measure-tom -channel
      mono`:

  - **Batter tuning is nailed.** f0 = **239.66 Hz, total spread 11.6 cents across
    all sixteen hits**. With the diameter now pinned, that is a direct read on
    `B.TUNE` and should be seeded or pinned rather than searched — a search that
    lands 50 cents off is landing outside the instrument's own repeatability.
  - **The contact model has a clean velocity signature.** Attack balance runs
    −11.6 dB at v01 to −4.1 dB at v16, **+0.64 dB per step at R² = 0.78**, a 10 dB
    swing. That is the observable for `HARD`, `ATK.L`, `ATK.T` and the strike
    velocity map, and it is monotone enough to fit against directly. It is also
    the term whose reproducibility floor is tightest (0.81 dB p90), so the swing
    is twelve times the noise.
  - **The Berger nonlinearity is visible in the glide series.** Per-hit glide runs
    a median **13.7 ¢ over the soft half against 37.3 ¢ over the hard half**, a
    factor of 2.7. This is the identification signal the item is built on, and it
    survives on this reference where the previous one could not measure glide at
    all. Three of sixteen takes still return no reading.
  - **The higher mode ratios do not survive the take-to-take comparison.** Only
    three components appear in twelve or more of the sixteen takes: f/f0 1.000,
    1.068 and 1.495. The fast estimator retains a different set of partials on
    each hit, so there is no stable ratio table to pin `R.TUNE` or the cavity
    coupling against — the fundamental is the only frequency the series agrees
    on. Repairing that is N17 and N2, and it is the blocker on pre-deriving
    anything beyond `B.TUNE` from frequencies.

      **A measurement defect found and fixed on the way.** `cmd/measure-tom`
      defaulted `-base-window-db` to 30 while `match` picks the fundamental for its
      own glide term at 20, with a written argument for 20 that this violated. At
      30, `v04` and `v05` handed the note to peaks 24.6 and 26.0 dB down — 199.5
      and 159.7 Hz against the 239 Hz the other fourteen agree on — and every
      base-keyed quantity followed. The repeatability summary read **SD 182 cents,
      spread 709** for a drum stable to **SD 2.9 cents**. Two misidentifications,
      reported as a property of the instrument. The default is now 20 and the two
      tools ask the same question the same way; anything quoting the old spread
      figure is quoting an artefact.

- [ ] **N6: measure identifiability before trusting any fitted bank.** The
      converged fits show the textbook sloppy-model signature (Gutenkunst et al.,
      PLoS Comput. Biol. 3(10):e189, 2007) and there is no Jacobian, Hessian or
      Fisher-information code in the repository. A central-difference Hessian at the
      optimum is ~600 evaluations against the 88 584 a fit already spends; its
      eigenspectrum says how many parameter combinations the data constrains. Follow
      with profile likelihood (Raue et al., Bioinformatics 25(15), 2009) to separate
      structurally from practically non-identifiable.

      **One flat direction is provable without running anything**: each mode's
          observed amplitude goes as Φ(r_strike)·Φ(r_mic), symmetric under exchange, and
          for axisymmetric modes the angles enter only as a difference. So (HIT.R,
          MIC.R) is identifiable at best up to a swap and (HIT.A, MIC.A) only through
          Δθ — four parameters carrying at least two exactly flat directions, by
          construction.

- [ ] **N7: rewrite the paper against the new reference.** _Deferred
      deliberately._ `docs/paper/` describes a fit to a recording of unknown
      provenance under an objective now known not to resolve most of what it
      reports. Do **not** patch numbers into it. The rewrite must:
  - remove every result derived from the old recording rather than annotating it,
    including the figures drawn from those runs;
  - state the reference's licence and provenance, which it can now do;
  - report the reproducibility measurement as a **first-class result** — it is the
    strongest methodological finding on the path and belongs in the method chapter,
    not a footnote;
  - carry the recorded negative results, which are worth more than the fits they
    replace;
  - re-derive `<comb-eq>` and the channel table against a coincident pair, where
    the comb argument no longer applies.

    Blocked on N2 and N5: nothing to report until the objective is trustworthy and
    a fit against the licensed reference exists.

- [x] **N8: retire `reference/tom.wav`.** _Done 2026-08-01 — the file is deleted.
      Not done the way this item asked, and the difference matters._ Unknown
      provenance, unlicensed, 44.1 kHz, spaced pair; it could not be committed and
      no test ever depended on it, so the build, `just test` and `just ci` were
      unaffected by its removal. Every fit and render measured against it —
      `fits/` and `renders/` in full, 24 reports with their checkpoints, logs and
      audio — was deleted with it, as was the stale committed `paper-figures`
      binary. Every remaining mention in code, docs and the justfile now reads as
      a dated historical note rather than as a live path.

      This item wanted the deletion to land in the same change that re-derived
      what depended on it. It did not, so those things are **orphaned now** and
      are tracked where the re-derivation lives:

  - `testdata/physical-fit-tom.json` and the **Measured tom** preset in
    `web/src/algo/physicalTomPresets.ts` — a bank fitted to a recording nobody
    can obtain, that missed all three adoption gates even when it was current →
    N5;
  - the figures and totals in `docs/paper/` → N7;
  - `contactReferenceHz` = 118 Hz, this recording's fundamental rather than the
    model's 150.08 Hz, still normalising every dB figure in
    [`physical-contact.md`](docs/physical-contact.md) →
    **N8a** below.

- [ ] **N8a: re-point `contactReferenceHz` at the model's own fundamental.** It is
      118 Hz, the deleted recording's fundamental, so the normalising bin reads
      the leakage skirt of the model's 150.08 Hz partial and works as an overall
      level normaliser by accident. Moving it to 150.08 Hz rewrites every dB
      table in [`physical-contact.md`](docs/physical-contact.md) and
      [`physical-nonlinearity.md`](docs/physical-nonlinearity.md) and the tests
      that assert them. Independent of N5: nothing here needs a recording.

- [ ] **N9: make the nonlinear mode coupling affordable on `js/wasm`.** The
      retrigger worst case at 120 oscillators went 1.40× → **0.70× real time**
      (4.39× → 2.06× on host), zero allocations throughout. The fixed-point
      iteration count barely moved (2.404 → 2.491), so this is the table walk
      itself: three separate index arrays, no blocking by channel or receiver,
      rebuilt per iteration rather than updated. 128 coefficients buys 0.79× and
      costs 0.4 dB in the target band, which is the first thing to try. Independent
      of any recording.

- [ ] **N10: jitter mode frequencies per trigger** by a fraction of a percent so
      repeated hits are not identical (Cook, PhISEM, ICMC 1996). The static
      degenerate split from P6's `TensionAsymmetry` is a different mechanism and
      stays. Blocked on repeated hits at one dynamic: the licensed pack's sixteen
      velocities are single takes, so this needs a repeated-hit capture or a
      different pack. Confound to watch — a frequency spread that tracks take peak
      level is the Berger nonlinearity, not jitter.

- [ ] **N11: assert the damping shape across the _modal_ octave bands.** P8's one
      unmet exit clause. `damping_test.go` asserts constant ζ, near-proportional
      decay rate and the (0,1) as the fastest-decaying mode of the low band, and
      `TestAttackBandsDecayAtTheirOwnRate` does the same one attack band at a time —
      but nothing states it across the modal bands, where a uniformly damped bank
      would still pass today's tests. State it quantitatively: "the bands' envelopes
      differ" is nearly guaranteed by construction, since the attack layer decays in
      tens of milliseconds and the modal band in hundreds.

- [ ] **N12: two model-internal soundness gaps.** Both found while fixing a
      validated config that could render NaN.
  - `Validate()` accepts a head with **zero total loss**, structural and radiation.
    Finite, but the modes never decay and `IsActive()` stays true forever — a hung
    voice in the modal bank, where the clamp in `attack.go` cannot reach it.
  - `attack.levelRelative = 1000` and `pickup.outputGain = 100` let the physical
    voice hand ~1e4 to the master chain. Legitimate and clamped downstream, but the
    ceilings look chosen for headroom rather than measured.

- [ ] **N13: citation debt.** `docs/paper/references.bib` is missing three keys the
      prose already leans on: Kirby & Sandler, JASA **150**(1):202–214 (2021),
      doi:10.1121/10.0005509 — measured on a **tom-tom** at 67 strike intensities
      with a 20-listener AB test at chance — plus `bork1983` and `garder2005`. Bork
      is **unpublished** and citable only via Fletcher & Rossing, which the entry
      must say. Two literature gaps also remain genuinely open and should be
      recorded as such rather than guessed: **felt-mallet contact time**, and a
      numeric **radiation-versus-internal damping split** for any drum.

- [ ] **N14: the doublet pair, by physical capture.** The one measurement the
      sample pack cannot supply. Ten centre hits with the resonant head removed, ten
      with it refitted, batter tuning untouched; Fischer's protocol on a tom.
      Capture **three** frequencies, not two — `f_single`, `f_lower`, `f_upper` —
      because [`physical-cavity.md`](docs/physical-cavity.md) reads Fischer's
      186 → 215 as `f_upper/f_lower` while its own interlacing argument says the
      lower branch is pinned and cannot carry the shift. `T = σc²` must be applied to
      the **uncoupled** fundamental or the air spring is baked into the tension.
      Postponed by the user 2026-08-01 for noise reasons; N4 now supplies most of
      what this was for, so it is no longer blocking.

- [ ] **N16: re-measure the objective-validation results on the reference the fit
      now aims at.** The reference moved from `tt08x08/mp/hd` to `tt08x08/lp/hd` on
      2026-08-01, chosen on the sound.
      [`physical-objective-validation.md`](docs/physical-objective-validation.md)
      Result 1 has been redone on the new set and the shipped gates come from it;
      **Results 2 onward have not**. They remain true statements about the
      medium-pitch drum, with working reproduction commands, and each needs
      re-running before it can be quoted as a property of the current target.

      This is not bookkeeping. The one result already redone refuted a standing
      assumption of that document — that the reproducibility floor is a property of
      the estimator rather than of the estimator and the recording together. Same
      code, different drum, and the glide floor moved from 280.1 ¢ to 2.3 ¢, because
      the medium-pitch fundamental dies before the glide estimator's late probe and
      the low-pitch one does not. Anything in Results 2–10 that was read as a
      statement about the estimator is now suspect in the same way, and the two
      candidates worth doing first are the residual budget and Result 10's decay-shape
      finding, which
      [`physical-calibration.md`](docs/physical-calibration.md) currently cites as
      the reason to doubt the constant-\(\zeta\) loss law.

      **Results 5, 8 and 10 are now done**, on 2026-08-01, as objective-validation
      **Result 11** — one `cmd/measure-tom -channel mono -high-resolution` run over
      `reference/tt08x08/lp/hd/v*.wav` at the post-N17 defaults. What it changes:

      - The decay exponent is **f^-0.70**, not the f^-0.52 the truncating window
        gave. Constant \(\zeta\) is now too steep by 2.0x rather than 3.3x. The
        fundamental reads 1.076 s (SD 3.1 %) against 0.686 s — the old window was
        cutting 36 % off the partial the whole fit is anchored to. `D.TILT` should
        land clearly below 1 and clearly above 0, which is what N3 waits to read.
      - Result 8 holds on this set: **29 clean pairs, median ring-time ratio 1.39**
        between members 0.1–6 % apart, and the cavity's predicted sign still absent
        (upper member faster in 18 of 29). The missing pairwise damping freedom is
        confirmed on the drum the fit aims at.
      - Result 5's fast-versus-subspace disagreement **widened** (median 41 % →
        63 %), against the prediction that it would narrow — but the drum changed
        too, so the prediction is untested rather than refuted. §5c did not merely
        fail again, it **inverted**: the partials the fast estimator is most
        confident about are the ones it agrees with the subspace one about least.
      - New, and it bounds the estimator work N2 opens: at this drum's fundamental
        the **subspace estimator is not ground truth**. Its ring time moves by 9x
        across nominally identical strikes where the fast one moves by 6 %, because
        the low band is a cluster and it resolves a different member each take.
      - New, and it costs something: **the glide is gone on this set** — all
        sixteen takes positive, median +18 ¢, three unmeasurable, no velocity
        trend, fifteen of sixteen at or below the 40 ¢ readability threshold.
        Result 4's velocity-glide curve is a property of `mp/hd`. A fit against
        `lp/hd` does not constrain `BERGER` in either direction, and **N5's joint
        velocity-series fit loses its stated justification** — still worth doing,
        no longer worth doing for that reason.
      - Two partials-per-take facts worth carrying: 4.7 % of partials are assigned
        a ring time longer than the 2.08 s file, and **all of them are the same
        357–360 Hz component** in twelve of sixteen takes, now at R² 0.95–0.99. It
        is not a failed fit; it is something in that room that outlives the take.
        The 286–360 Hz band stays excluded, on a stated ground rather than on poor
        fit quality.

      **Results 2, 3 and 6 remain open**, and 2 and 3 cannot be re-run without
      rewriting tooling that was never committed (the four-window residual budget,
      the band-limiting sweep, the static-EQ fits). Result 6 is N6 and was never
      measured on any set.

      **Result 10's original write-up follows**, kept because its window study is
      what established N17. Re-measured on
      `lp/hd` the ring time falls as f^-0.52; on `mp/hd` the same code gave an
      exponent near **zero**. The conclusion that survives the move is only the
      negative one — neither tuning supports the 1/f the loss law is calibrated to —
      and the positive number is a property of the drum and the tuning together, as
      the glide floor was. It also exposed a defect the medium-pitch set hid, now
      **N17**: 27 % of this reference's partials are fitted over a window shorter
      than their own ring time. Redo the rest of Results 2–10 *after* N17, not
      before, or they are measured through a window that is about to move.

      A second consequence to watch when the refit happens: the glide gate is now
      **30 cents** rather than 290, so a term that could previously not influence a
      total can now dominate one. Result 11e is the caution against reading much
      into it either way — on this set the whole glide measurement spans about one
      gate. No fit has been run under these weights.

**Exit.** The objective's gates are traceable to measured reproducibility under
repaired estimators; a fit exists against the licensed reference with at least one
test depending on a measured number; the long-ringing spurious mode is either
fixed with a measured improvement or closed by the measurement that rejected the
fix; and the paper reports that state rather than the previous one.

### Closed on evidence — do not re-open

Each was proposed, tested and refuted. Recorded so the work is not repeated;
detail and citations in
[`physical-objective-validation.md`](docs/physical-objective-validation.md) and
[`physical-model-research.md`](docs/physical-model-research.md).

- **Exterior air loading to harmonicise the mode ratios.** Rejected in P8, argued
  back in P9 on the model's (1,1)/(0,1) being pinned at 1.588 against a recording's
  1.802, and re-closed 2026-08-01 on far better evidence: the licensed drum's
  geometry is **known**, so fixing the fundamental predicts every other mode with no
  free parameter, and the measured ratio is **1.584 against 1.594 ideal** — an
  11-cent match, ~950 cents from the air-loaded ~1.78. The 1.802 belongs to that
  drum's two-head tuning, a mechanism the model already has.
- **A tom vent (Helmholtz) resonance.** A first-principles estimate puts a 12"×9"
  tom near 30 Hz, far below the (0,1), and no measurement of one was found.
- **The spectral-envelope residual as a band-coverage artifact.** Band-limiting to
  50 Hz–2 kHz, where the model has full modal content, buys **2 dB of 11**.
- **A fitted static body/radiation post-filter.** The topology is physically
  correct (Bank, ICMC 2007) and the falsification test still fails: 9.34 dB at five
  free parameters against a 3–4 dB criterion, and **7.81 dB with a fully free
  per-band EQ** — the absolute limit of any static filter. Worth keeping because it
  forecloses the dishonest version, where a 26–40-coefficient filter against 24
  bands drives the term to zero by construction and teaches nothing.
- **A more sophisticated formulation.** A survey of FDTD and 3-D air fields,
  waveguide meshes, the Functional Transformation Method, FEM/BEM, port-Hamiltonian
  formulations, mass-interaction and differentiable/learned synthesis found **none
  that targets the defect surviving measurement**. FDTD is dead on cost and on
  fittability; FTM _is_ this model's modal expansion for a separable circular
  membrane; a 3-D air field changes nothing about the coupling stiffness, because
  non-uniform cavity modes have zero net volume; and differentiable modal synthesis
  upgrades the **search**, which is not the ceiling — two independent runs already
  agree term for term.
- **A per-mode damping vector fitted freely.** Not refuted but pre-empted: see N3's
  sign-pattern check. A structureless result is fitted noise, and it will score
  well.

### Gated and deferred

- **P7 — snare research extension.** Gated behind a tom that sounds like a tom.
  Starting it now would build on an uncalibrated foundation and make it impossible
  to attribute a bad snare sound to its own model rather than the inherited one.
  When it starts: a clearly labelled reduced snare-contact model driven by the
  resonant head first; 1-D snare strings with distributed unilateral membrane
  contact and an energy-conserving update as a separate prototype; and any full
  2-D-heads-plus-3-D-air FDTD kept as a quality/reference path until measured WASM
  performance proves it can meet the real-time contract. The reduced musical model
  and the high-fidelity research model must have separate names, tests and
  performance expectations.
- **Extract a generic modal bank to `algo-dsp`** — only after this implementation
  has proven the API.
- **Use `algo-pde` for offline frequency-domain reference work**, not for
  audio-rate evolution. Resolve its GitHub/module-path mismatch before taking a
  direct dependency.

### Physical-path success criteria

1. Existing procedural renders and old persisted/share states are bit-for-bit
   unchanged when the physical path is not selected.
2. Physical parameters have units, bounds, documented provenance, and one
   generated Go/TypeScript description.
3. Analytic mode tests, energy/passivity tests, reference comparisons, fuzzing,
   and zero-allocation rendering all gate CI.
4. The shipped quality tier meets the measured 48 kHz WASM budget with headroom
   in the production Worker/AudioWorklet pipeline.
5. Claims distinguish analytic prediction, reduced physical approximation,
   empirical calibration, and full numerical simulation.
6. **It sounds like the instrument.** Per-mode decay, the radiated tonal balance,
   and the attack transient are each checked against a cited measurement rather
   than only against the model's own analytic targets, and no compensating output
   gain, EQ or envelope stands between the physics and the mix. P8 exists because
   criteria 1–5 were all met while criterion 6 was not.
7. **The instrument that judges criterion 6 is itself measured.** P10 exists
   because criterion 6 was assessed for months by a metric whose reproducibility
   had never been established. Any gate quoted anywhere on this path must be
   traceable to a measured floor.

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
