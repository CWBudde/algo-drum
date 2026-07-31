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

Research completed: **2026-07-29**. See
[`docs/physical-model-research.md`](docs/physical-model-research.md) for the
model comparison, equations, repository fit, validation strategy, and primary
sources.

### Scope and architecture decision

- [x] Add a **parallel, explicitly selected physical model**. Preserve the
      existing procedural voices, their parameter meanings, and old share links.
- [x] Target a **double-headed tom first**. It covers circular head modes,
      strike position/contact, frequency-dependent loss, radiation, enclosed-air
      coupling, and nonlinear tension without making snare collision a
      prerequisite.
- [x] Keep the physical core independent of sequencer/UI state. Use SI units
      internally and a flat, precomputed, allocation-free modal state.
- [ ] Prototype in `algo-drum`; extract a generic modal bank to `algo-dsp` only
      after its API has been proven by this implementation.
- [ ] Use `algo-pde` for offline frequency-domain/reference calculations, not
      for audio-rate time evolution. Resolve its GitHub/module-path mismatch
      before taking a direct dependency.

### P0 — Baseline and contracts

- [x] Define `PhysicalDrum`, `Head`, `Mode`, `Strike`, `Cavity`, and `Pickup`
      parameter structs, units, valid ranges, defaults, and versioned
      serialization.
- [x] Define quality tiers by retained frequency/mode count; benchmark both
      native and `GOOS=js GOARCH=wasm` before fixing the shipped tier.
- [x] Add a benchmark harness that reports samples/second, real-time factor,
      allocations, and active modes at 48 kHz/512-sample chunks.
- [x] Establish the integration contract: explicit model selection, deterministic
      reset/trigger, finite output, zero allocations in `Render`, and no changes
      to existing procedural output when physical mode is not selected.

Exit: an empty/silent physical backend is selectable in tests and the measured
WASM budget is recorded.

Initial P1 microbenchmark baseline (2026-07-29, 48 modes, 48 kHz/512-sample
chunks, zero allocations): 3.49–3.96 Msamples/s (72.7–82.6× real time) on
Linux/amd64 and 1.71 Msamples/s (35.6×) on `js/wasm` under Node. The Standard
tier remains a prototype default until the production Worker/AudioWorklet path
is measured.

### P1 — Linear single-head modal prototype

- [x] Generate circular Fourier-Bessel modes, including both orientations of
      each \(m>0\) pair; test zeros, ordering, normalization, and analytic modal
      frequencies.
- [x] Implement stable damped two-pole/exact-state modal updates with
      precomputed coefficients and structure-of-arrays storage.
- [x] Project a finite-area, band-limited strike onto the modes. Expose velocity,
      hardness, strike radius, and strike angle.
- [x] Add separate diagnostic outputs for head displacement/velocity and the
      radiated pickup sum.
- [x] Validate center/off-center selection rules, determinism, finite output,
      bounded lossless energy, monotonic damped energy, and zero steady-state
      allocations.

Exit: an auditionable single circular head whose pitch, mode mix, and decay move
predictably with physical parameters and strike position.

### P2 — Loss, radiation, and calibration

- [x] Replace the provisional uniform decay with a two-parameter
      frequency-dependent modal decay law; retain room for measured per-mode
      corrections.
- [x] Add mode-dependent radiation weights and a compact radiation/microphone
      filter using `algo-dsp`.
- [x] Add offline analysis tooling for modal peaks, decay times, pitch-glide
      tracks, spectra, and waveform/spectrum regression metrics using
      `algo-fft`/`algo-dsp`.
- [x] Define an openly licensed or locally measured reference set: multiple
      velocities, strike radii, and microphone positions, with provenance and
      recording conditions.

Exit: low modal frequencies and decay times match analytic/reference targets;
the radiated output is clearly distinguished from a raw point pickup.

### P3 — Resonant head and cavity

- [x] Add an independently tuned resonant-head modal bank.
- [x] Implement a passive lumped cavity spring/damper driven by swept head
      volume; couple the ideal axisymmetric modes first.
- [x] Test the zero-coupling limit, in-phase/out-of-phase modal splitting, and
      lossless energy exchange/conservation.
- [x] Compare the reduced transfer function against an offline
      frequency-domain reference; add only evidenced cross-coupling terms.
- [x] Expose batter tuning, resonant tuning, shell depth, and air/coupling as
      physical parameters with safe update semantics.

Exit: changing either head or shell depth causes explainable coupled-mode
changes, and a batter hit audibly excites the resonant head.

Completed 2026-07-29. The passive rank-one pressure coupling, update semantics,
validation equations, transfer-function reference, and native/WASM benchmark
results are documented in
[`docs/physical-cavity.md`](docs/physical-cavity.md).

### P4 — Nonlinear hit behaviour

- [x] Implement a Berger-style reduced tension modulation driven by modal
      displacement/strain energy.
- [x] Give the update a discrete energy/passivity argument and conservative
      parameter bounds; add an oversampled or high-precision reference test.
- [x] Verify velocity-dependent attack spectrum and downward modal-frequency
      glides without runaway energy or aliasing.
- [x] Evaluate the provisional force pulse against measured contact durations.
      Its original 0.71 ms default was wrong by nearly an order of magnitude;
      replace it with a bounded, velocity- and hardness-dependent 5.5–8 ms
      half-sine contact while retaining the allocation-free real-time contract.

Exit: louder hits produce a controlled, reference-comparable pitch glide and
attack change while all fuzz/finite/energy tests remain green.

Completed 2026-07-29. The bounded Berger potential, discrete-gradient
passivity argument, anti-alias parameter bound, oversampled reference,
velocity/glide measurements, active native/WASM benchmarks, and the corrected
measurement-bounded contact pulse are documented in
[`docs/physical-nonlinearity.md`](docs/physical-nonlinearity.md).

### P5 — Product integration

- [x] Add a Physical Drum lab/editor with model selection, head dimensions and
      tuning, damping, hit position/hardness, cavity coupling, nonlinear amount,
      pickup position, quality tier, audition, and reset.
- [x] Decide after profiling whether the first physical tom replaces the Tom
      track when selected or appears as a separate experimental instrument;
      never make this an implicit preset change.
- [x] Extend the generated parameter metadata rather than hand-maintaining a
      second Go/TypeScript parameter table.
- [x] Version persistence and URL sharing; old states must decode to the
      unchanged procedural engine.
- [x] Extend Worker/WASM command validation, recovery, E2E coverage, and
      accessibility for the new controls.

Exit: users can A/B the procedural and physical paths in the production browser
build without audio-pipeline or persistence regressions.

Completed 2026-07-29. The generated control bank, bounded physical mappings,
passive cavity-coupling control, independent A/B state, version-4
persistence/share migration, Worker/WASM validation, accessibility behavior,
and production-browser coverage are documented in
[`docs/physical-product-integration.md`](docs/physical-product-integration.md).

### P6 — Real-instrument departures

- [x] Add deterministic degenerate-mode splitting/non-uniform tension.
- [x] Evaluate a small shell/hardware modal bank and bearing-edge/vent
      corrections, accepting each only when measurements justify it. None were
      accepted without an instrument-specific measurement residual; the
      acceptance gate is documented in
      [`docs/physical-real-instrument-departures.md`](docs/physical-real-instrument-departures.md).
- [~] Fit documented presets from measurement while preserving the underlying
      SI parameters and provenance. The machinery exists and has produced one
      fit; the *provenance* half of the item does not, and cannot with the
      recording available. See below.
- [x] Consider measured modal transfer functions as an optional calibration
      layer, not a replacement for the physical state. The documented decision
      is to fit interpretable modal residuals first and require phase-coherent,
      held-out evidence before adding a real-time transfer layer.

Implemented 2026-07-29: physical-config v5 adds bounded, deterministic
cosine/sine pair splitting and a rotated principal tension axis, with exact
zero-asymmetry migration for v4 configs. The generated physical Tom bank adds
append-only ASYM/AXIS controls and app-state v5 preserves v4 links. The
measurement-fit preset and the phase exit criterion remain open because the
repository contains no identified real-tom recording set.

Sound re-audit 2026-07-29: corrected the default hit from peripheral to central,
replaced the erroneous sub-millisecond contact, and removed an unevidenced
equal-phase sum of opposite-side head radiation that cancelled the coupled
fundamental. App-state v6 migrates only the former shipped HIT.R detent. The
mechanical, observation, persistence, and regression audit is recorded in
[`docs/physical-sound-audit.md`](docs/physical-sound-audit.md).

**Superseded in part by P8 (2026-07-30).** That audit's contact-duration finding
holds — 4.5–8 ms is well supported by two independent measurement studies — but
it drew the wrong conclusion from it. A smooth half-sine force over the whole
contact interval is 40 dB down at the top of the retained mode bank, and the
`physicalTomOutputGain = 4` added to recover the level is compensating for a
missing signal path (a separate stochastic attack layer), not for a mis-scaled
pulse. The audit also moved the default hit to `Radius01 = 0.12`, effectively the
geometric centre, which spreads strike coupling over 116 dB; the sources describe
central playing as a _region_. See
[`docs/physical-tom-review.md`](docs/physical-tom-review.md).

**Measurement fitting added 2026-07-30.** `internal/physical/match` reduces a
recorded or rendered hit to one feature vector and scores two of them on eight
perceptual terms; `cmd/fit-physical` searches the exposed parameter bank against
a recording with the Mayfly algorithm. Both are offline only — the shipped WASM
binary is unchanged. Recorded in
[`docs/physical-measured-fit.md`](docs/physical-measured-fit.md), with the
fitted bank in `testdata/physical-fit-tom.json` and a **Measured tom** preset in
the voice editor.

The item stays open, for two reasons rather than one. The recording is of
unknown provenance, so the SI-parameter and provenance half of this item is
untouched — there is nothing to preserve. And the fit itself does not meet its
own adoption gate: modal frequency (21.5 ¢) and decay (0.179 log-ratio) pass
comfortably, spectrum (13.6 dB against a 4 dB gate) does not, so
`DefaultPhysicalDrum()` is deliberately unchanged.

Exit: a measured tom can be matched within documented tolerances for modal
frequency, decay, and spectrum across more than one hit.

**Progress against this exit criterion, and the one thing blocking it.**
Frequency and decay are inside tolerance on one hit, and nothing is pinned at a
parameter bound — the range is adequate, which was genuinely in doubt. Spectrum
is not, and the reason is specific and reproducible: the reference carries nine
resolvable partials between 476 and 700 Hz and the model produces none there, at
any quality tier (Draft 13.3 dB, Standard 13.1, High 13.1 — mode count is not
the constraint). Also still outstanding: "across more than one hit" — this is a
single strike from a single file.

**That band is now explained**, in
[`docs/physical-excitation-gap.md`](docs/physical-excitation-gap.md). It is not
the two-head mode series, the cavity split or the absent shell modes. The modes
are present — 58 of them lie in the band at High quality and none is audible —
and the excitation is not. `addContactPulse` prescribes a smooth half-sine over
the whole contact interval, whose spectrum nulls at 1.5/τ and falls as 1/f²
after it; at the fitted τ = 8.23 ms that is −28.7 dB at 504 Hz and −34.2 dB at
635 Hz, against a measured band deficit of −22.6 and −30.4 dB. Microphone
height, near-field balance, strike footprint, cavity coupling and tension
asymmetry were each measured and eliminated — twenty decibels of microphone tilt
buys 0.9 dB.

An independent literature check sharpened this into a second, separable error.
The 5.5–8 ms the shipped law prescribes is well supported, but it is a **contact
dwell time, not a force-pulse width**, and the model spends it as the latter.
Wagner (KTH 2006 §4.1.1, §4.2.1) measured contact electrically and force
separately: the stick "would already leave the drumhead after approximately
3.5 ms", and the dwell runs on only because the wave reflected from the rim
returns under it, producing two further weaker impacts at 3.75 ms and 5.6 ms. So
the measured excitation is three discrete impacts inside an 8 ms window and
`addContactPulse` replaces them with one smooth half-sine across the whole of it
— too long *and* too smooth. Prescribing the main pulse at 3.5 ms is worth ~7 dB
in the band; holding duration and impulse fixed and skewing the rise to 0.49 ms
is worth a further ~13 dB. The shape half is exactly what
[`docs/physical-tom-review.md`](docs/physical-tom-review.md) §6 predicted by
inspection.

**The width and the shape are not separable.** Prescribing the pulse at
Wagner's 3.5 ms and leaving the shape alone looked like the cheap half and was
tried: it raises the default drum's 60–1000 Hz peak by 14.1 dB with the
nonlinearity disabled, because a half-sine nulls at 1.5/τ and shortening the
pulse drags that zero from 273 Hz to 429 Hz through the low mode cluster. That
is the review's "the nulls move with the knob" bullet, measured. It is not a
level shift the output gain can absorb — τ varies with velocity and hardness, so
the zero lands on different modes at every dynamic. The constants are therefore
left as they are with the defect recorded beside them, and the correction has to
be one change rather than two.

**The Hertzian contact was built and measured (2026-07-30).**
`Strike.Contact.Model` now selects it; it is off by default;
[`docs/physical-contact.md`](docs/physical-contact.md) is the record. It did not
close the gap, and it corrected two things written above.

The gap is a **comb**, not a tilt. The half-sine's spectrum
`|cos(πfτ)|/|1−(2fτ)²|` has exact analytic zeros every `(k+½)/τ`, and at the
fitted 8.23 ms two of them — 547 and 668 Hz — fall inside the gap at −309 and
−315 dB. A tilt leaves modes quiet; a zero leaves them unexcited. That is the
complete explanation of why mode count, microphone geometry, cavity coupling and
the loss law were each eliminated, and of why shortening τ cost 14 dB: it slides
the comb, and the comb must land somewhere.

The Hertzian contact turns those zeros into −26 and −29 dB and moves them, but it
does **not** remove the comb — it is still one smooth touch, and one touch of
duration τ interferes with itself wherever it sits, leaving a −51 dB dip of its
own at 465 Hz. Below 700 Hz it is worth 0–4 dB. Above 800 Hz it is worth
**+11.8 dB at 800 Hz, +15.1 at 1.5 kHz, +22.9 at 2.5 kHz** in the modal-only
render, which is the *seam*, not the gap.

It also does not reproduce Wagner's separation and re-contacts. The version that
appeared to — 4.15 ms lobe inside a 7.48 ms dwell, seven impacts — was a
discretization artifact and converged away under substep refinement; the
converged contact is one smooth touch. That near-miss is written up in full
because it imitated the exact phenomenon being looked for.

What it did establish, and this is the useful part: **contact time here is set by
the head, not by the tip.** The head's driving-point mass under the stick is
0.31 g against a 15 g mallet, so a 900-fold stiffness range spans only 1.51 in
duration while mass moves it almost proportionally. Three consequences. The
7.26 ms it predicts is a genuine prediction, and it lands inside Dahl's and
Wagner's 5.5–8 ms. HARD loses most of its authority, since tip stiffness cannot
reach the measured factor of two. And `Strike.MalletMassKg` stops being a free
loudness knob and becomes measurable — the measurement says 4–6 g, not the
shipped 15 g, because that is what reproduces the measured velocity law
(0.65–0.66 against Dahl's 0.69, where 15 g gives 0.92).

Closing P8 therefore still needs: structure *inside* the contact interval, which
is the one thing neither model supplies and the only thing that removes a comb;
the seam between the modal bank's top and the attack layer's lowest band closed
at low tunings — for which the Hertzian contact is now the precondition, since it
puts real excitation where `ATK.T` was dragged down to fake it; and the
transverse-cavity hypothesis tested. Adopting the Hertzian contact is a
calibration pass in its own right: it delivers 1.9× the impulse, and
`Pickup.OutputGain`, the nonlinear tension coefficients, `Attack.LevelRelative`,
`Strike.MalletMassKg` and the fitted preset would all have to move with it.

**Measured 2026-07-31, and the pass is not worth starting yet.** Refitting the
whole bank under each contact model at 150 iterations — 8 restarts, 59 056
evaluations each — puts prescribed at **5.901** against Hertzian's 7.450, or
6.548 once the mallet is dropped to the measured 5 g. The seam does close exactly
as predicted: `ATK.T` sits at 3426 Hz under Hertzian against 1261 Hz under
prescribed, so the search stops using the noise layer to fake a band the
excitation never reached. The gap does not: **all three fits find 0 partials in
476–700 Hz** where the reference has 9, and none comes within 3× of the 4 dB
spectral-envelope gate. So the excitation model was never the binding constraint
on this fit, and P8's remaining question is mode density in that band. The 5 g
mallet is separately confirmed as the better value, and is recorded rather than
applied because it belongs to the same calibration pass. Two by-products worth
keeping: the 150-iteration prescribed fit is the best total ever measured here
(5.901, against the documented 6.364) and still fails the gate, confirming that
search effort does not move the band; and the 5 g run scores the *best* spectral
envelope of any fit while finding the *fewest* partials, by parking `ATK.T` at
626 Hz inside the gap with `ATK.L` at 3.7× — the spectral-envelope term is
gameable by the noise layer, which is why the gate is per-term. See
[`docs/physical-measured-fit.md`](docs/physical-measured-fit.md).

### P7 — Snare research extension

Gated behind **P8**. A snare adds mechanisms on top of the two-head/air system,
so starting it before the tom sounds like a tom would build on an uncalibrated
foundation and make it impossible to attribute a bad snare sound to its own
model rather than to the inherited one.

- [ ] First add a clearly labelled reduced snare-contact model driven by the
      resonant head.
- [ ] Separately prototype 1-D snare strings with distributed unilateral
      membrane contact and an energy-conserving update.
- [ ] Keep any full 2-D heads + 3-D air FDTD implementation as a quality/reference
      path until measured WASM performance proves it can meet the real-time
      contract.

Exit: the reduced musical model and the high-fidelity research model have
separate names, tests, and performance expectations.

### P8 — Sound correction (open, highest priority on this path)

Measured diagnosis **2026-07-30**, with an independent literature check. Full
evidence, citations and the list of conclusions this pass reversed are in
[`docs/physical-tom-review.md`](docs/physical-tom-review.md).

The mechanics are correct — mode labels, eigenmodes, exact state transitions, the
passive cavity solve, the discrete-gradient Berger update and the energy
bookkeeping all check out. The model sounds wrong because of four calibration
errors, one architectural mismatch, and a compute budget spent on the least
audible mechanism. Measured against `DefaultPhysicalDrum()` at 48 kHz, velocity
0.8:

| Defect                                 | Measured                                     | Target                                      |
| -------------------------------------- | -------------------------------------------- | ------------------------------------------- |
| Damping ~4–11× too weak and flat vs. f | γ = 3.1 /s every mode; T60 1.8–2.3 s         | γ = 11–41 /s; T60 ∝ 1/f                     |
| The fundamental rings **longest**      | (0,1) T60 = 2213 ms                          | (0,1) T60 ≈ 209 ms — the **shortest**       |
| No usable bandwidth                    | highest retained mode **646 Hz**             | audible content to several kHz              |
| Cavity coupling ~5× too strong         | (0,1) doublet 107.7/219.4 Hz, ratio **2.04** | ratio ≈ **1.16**                            |
| Pitch glide inaudible but expensive    | 38 cents, costs 6× the voice                 | audible glide over a few tenths of a second |

Damping is the dominant error and the reason the voice reads as a low ringing
sine rather than a drum. The two-parameter law `γ = d0 + d2·k²` has no k¹ term,
so it **cannot express constant Q** — and the measured structure for a
two-headed drum is roughly constant ζ ≈ 1.1 % above the fundamental, with
ζ ≈ 5.07 % on the (0,1) caused by the two-head coupling itself. Raising `d2`
alone gives T60 ∝ 1/f², which is the wrong shape.

The table is the 2026-07-30 diagnosis, kept as written so the corrections stay
attributable to a measurement. All five rows are now closed, by S1 through S6 and
S8, though rows 3 and 5 were closed differently from how they are written: the
bandwidth came from a hybrid noise layer rather than from more modes, and the
glide's "6× the voice" cost premise turned out not to exist. The numbers in the
table describe the voice as diagnosed, not as it now stands.

Cheap, high-impact, no architecture change:

- [x] **S1 (high): add a `d1·k` term to the modal loss law** and calibrate to
      constant Q. For ζ = 1.1 % and this head's wave speed c = 41.40 m/s,
      `d1 = ζ·c ≈ 0.455 m/s`. Raise the `DAMP` ceiling well above its present
      12 /s — the fundamental alone needs ≈ 33 /s — and expose a damping _tilt_
      rather than only a uniform scale. Today `DAMP` and the strip `DEC` both
      scale every loss term by the same factor, so nothing can change the shape.
      **Done 2026-07-30.** `Head.Loss1MPerSecond` makes the law
      `γ = d0 + d1·k + d2·k²`; the reference set uses `d1 = 0.4554` (batter) and
      `0.4919` (resonant), with `d0` cut from 3/4 to 0.8/1.0 — it had to be,
      since `d0` is the term that flattens ζ toward low frequencies. ζ now holds
      1.12–1.24 % across the retained band. Config schema v6, with v5 migrating
      at `d1 = 0` so old configurations keep their flat damping exactly. `DAMP`
      turned out not to need rescaling, only honest labelling: it was a
      0.75–12 "/s" range that `reconfigure` immediately divided by 3, so its
      real effect was always a 0.25–4× multiplier and the spec now says so —
      same 16× span, identical mapping for every persisted position, no
      migration. The absolute ceiling rose anyway because the law beneath it
      changed. The new **`D.TILT`** control (0–3, default 1) scales the
      frequency-dependent terms against `d0`, so the decay _shape_ is
      adjustable: 0 reproduces the old flat behaviour, 1 is constant Q.
- [x] **S2 (high): damp the (0,1) modes specifically.** ζ ≈ 5 % → γ ≈ 33 /s at
      104 Hz, sourced to two-head coupling. `Head.ModeDecayCorrections` already
      exists for exactly this. S1 + S2 are a few lines and are most of the
      difference between "boing" and "thump".
      **Done 2026-07-30.** Default corrections of +24.6 /s (batter (0,1)) and
      +26.4 /s (resonant), landing both on ζ ≈ 5 %: T60 211 ms and 196 ms
      against the 2213 ms it shipped with. The corrections scale with `DAMP`,
      `DEC` and `D.TILT` like every other loss, or those controls would have no
      authority over the one mode whose length is most audible.
      Measured across the nine-condition reference suite, RT60 went from a
      uniform 2.06–2.17 s to 0.27–0.55 s, and per-mode T60 now falls with
      frequency (211 ms at 104 Hz, 151 ms at 645 Hz) instead of sitting flat at
      ≈ 2.2 s. Peak level dropped from 1.20 to 0.78 at velocity 1, so the voice
      no longer arrives at the master limiter already clipping;
      `physicalTomOutputGain = 4` is left for S4 to delete.
      One existing test asserted the defect: `TestDefaultBatterSideSoundIs`
      `FundamentalLed` required the (0,1) to be the strongest partial over
      1.4 s, which a correctly damped fundamental cannot be. It now checks that
      the fundamental leads the _attack_ and that the sustain peak moves above
      it — the pitch envelope a real tom has.
- [x] **S3: refit the cavity split** to a 10–20 % (0,1) separation instead of
      deriving it from ρc²/V. The rigid-cavity formula over-predicts ~5× because
      the shell is compliant, the vent leaks and the heads are not pistons;
      verified by isolating the air spring (batter only, centre hit:
      104.00 → 191.89 Hz, against an analytic 215.1 Hz). This also removes a
      spurious −9 dB partial at 219 Hz sitting where (2,1) should be.
      **Done 2026-07-30.** New `Cavity.StiffnessScale` multiplies the rigid
      ρc²/V, shipped at **0.04**. On a central strike the (0,1) doublet moves from
      108.4/219.7 Hz (ratio 2.03) to 105.5/123.0 Hz (ratio **1.17**), inside the
      measured 10–20 % band, and the 219.7 Hz partial — which sat 3 dB below the
      fundamental, on top of the (2,1) at 221.8 Hz — is gone. The field is a
      fraction, not a free gain: the rigid, sealed, piston-driven enclosure is
      the stiffest case there is, so 1 is the physical ceiling. Config schema v7;
      this is the **only** migration in the chain whose compatibility value is
      not the zero value, since an absent `stiffnessScale` decodes to 0, which is
      the uncoupled limit rather than the old sound, so `migrateV6Config` writes
      1 explicitly. No app-state bump: `StiffnessScale` is calibration, not a
      control, and AIR (`Cavity.Coupling01`) is unchanged — its top of travel is
      now the calibrated split instead of the rigid one.
      One correction to the diagnosis this item was written from: the _audible_
      fundamental cannot rise 16 %. Eigenvalue interlacing pins the lower branch
      between the two heads' uncoupled (0,1) frequencies, 104.0 and 112.3 Hz, for
      any rank-one air coupling; only the stiffened branch carries the shift, so
      the branch **separation** is the fittable quantity. Level is barely
      affected — the voice peak at velocity 1 went 0.78 → 0.90, since the peak
      belongs to the attack transient, not to the fundamental.
      A pre-existing test defect surfaced here.
      `TestDoubleHeadReferenceTransferMatchesTimeDomain` compared the nonlinear
      time-domain model against `ReferenceFrequencyResponse`, which is linearized
      at rest, so its residual was tension modulation rather than the coupled
      solve it claims to check: 0.31 % at the chosen 137 Hz but **27 %** at
      300 Hz. It now runs with nonlinearity off, where the two agree to 0.034 %,
      and the tolerance is tightened from 5e-3 to 1e-3.
- [x] **S4: fix the radiated sum.** Weight volume **acceleration** by
      `SweptAreaM2` for the axisymmetric modes — already computed, currently
      unused in the output — and use a directivity factor for m > 0. Remove
      `PickupShape` from the radiated path; a far-field radiation efficiency and
      a near-field point mode shape are different objects, and multiplying them
      nulls modes arbitrarily as the microphone angle moves. Keep the mode shape
      for the diagnostic contact pickup and for strike-position weighting, where
      it is correct. Summing velocity also adds a spurious −6 dB/octave tilt.
      Then delete `physicalTomOutputGain`.
      The `(ka/√(1+ka²))^(m+1)` rolloff itself is **not** the problem and should
      be kept: m ≠ 0 modes have null net volume velocity, and a real snare's
      (1,1) is measurably not a strongly radiated mode.
      **Done 2026-07-30**, with two of this item's own premises corrected by
      measurement before any code was written.
      **The −6 dB/octave claim is wrong.** `(ka/√(1+(ka)²))^(m+1)` is ≈ ka at
      m = 0, so weighting velocity by it already carries the acceleration tilt.
      Multiplying by ω as well while keeping the exponent would have added ≈ +10 dB
      of unjustified tilt across the band, and the refitted output gain would have
      hidden it. `TestAxisymmetricRadiationWeightIsFrequencyIndependent` now
      asserts the m = 0 weight contains no ω at all; the "6 dB per octave against
      the old sum" test this item implied is a tautology — new/old is jω per mode
      by construction — and is deliberately not written.
      **The naive generalization of the swept area drops the multipole factor.**
      Extending `2πR²J₁(z)/z` to `2πR²J_{m+1}(z)/z` and letting the rolloff carry
      the m > 0 normalization loses `1/(2^m m!)`, which is 1.03e7 at m = 8:
      measured on the shipped basis it raises the (10,1) edge mode **401×**, to a
      quarter of the fundamental, where multipole theory puts it seven orders
      down. It would have measured as "brighter" and sounded like a plate. The
      implemented weight is the exact Rayleigh/Lommel closed form
      `2πR²·z·J_{m+1}(z)·J_m(u)/(z²−u²)` with `u = ka·sinθ`, which reduces
      *identically* to the swept area on axis and gets `1/(2^m m!)` from `J_m(u)`'s
      own small-argument behaviour. No arbitrary length scale, no free exponent.
      Dropping `PickupShape` was right, and its consequence was larger than this
      item assumed: far-field physics leaves every m > 0 mode ≥ 23 dB down even
      with the mic against the head — correct for a distant mic, since a 12-inch
      head below 600 Hz really is nearly a monopole, and wrong for the close one a
      tom is recorded with at d/a ≈ ⅓. So the weight became a **sum** of the
      far-field term and an explicitly fitted evanescent near-field term
      (`Pickup.NearFieldScale`, decaying as `exp(-z·d/R)` with the mode's own
      structural wavenumber, shaped by `PickupShape` — which is the right object
      there and only there). The (1,1) goes from −21.1 dB to −7.1 dB relative to
      the fundamental. Both terms multiply the same acceleration, so the weight is
      still one precomputed scalar per mode and the per-sample cost is unchanged.
      Mic geometry was refitted with it — 0.65 of the radius, 30 mm up, a real
      close mic — giving (0,1) 0 dB, (1,1) −7.1/−10.4, (0,2) −8.5, (2,1)
      −9.3/−17.5, down to −34.5 dB at the top of the band. **MIC.R is now the
      model's strongest timbral control** rather than an inert knob, which is why
      the plan's proposal to repurpose it was dropped.
      `physicalTomOutputGain` is **deleted**; `Pickup.OutputGain` is fitted so a
      velocity-1 hit peaks at 0.90 on its own, and a test in `internal/drum`
      keeps it that way. Config schema v8. This is the one migration in the chain
      that cannot promise the old sound — the old product is not a physical
      quantity and is not recoverable from a scale factor — so the correction
      applies to old and new alike; what migrates is the mixture, and zero
      near-field is its exact absence. Also fixed: `migrateV6Config` assigned
      `ConfigVersion` rather than 7, so once the version moved a v6 document would
      have skipped every later migration silently, with its own test still
      passing.
      Two tests turned out to be passing *because* of the defect, the third and
      fourth instances of that pattern here. `TestDefaultBatterSideSoundIsFunda`
      `mentalLedInTheAttack` required the fundamental to lead the default
      configuration's attack, which is a property of *where the drum is hit*, not
      of the model: measured across radius and window, a centre hit is
      fundamental-led everywhere and a 0.30 hit is (1,1)-led from 43 ms on. It is
      now scoped to the centre hit, with a companion test that the fundamental
      stays within 12 dB of the strongest partial on the default. And the
      nonlinear attack centroid was measured full-band, where the fundamental's
      *level* dominates: it moved 112.373 → 112.377 Hz, measuring nothing. Above
      the fundamental the mechanism is unambiguous, 243.8 → 310.1 Hz.
- [x] **S5: move the default strike radius** from 0.12 to ≈ 0.3 of the radius.
      **Done 2026-07-30**, at 0.30. Measured against radius, the fundamental sits
      2.07 dB below the strongest partial at 0.12, 7.23 at 0.22, 9.78 at 0.30 and
      11.22 at 0.36 — monotone, with no sweet spot, so this trades low-end weight
      for the (1,1) family rather than optimizing anything, and HIT.R exposes the
      trade. The (1,1) already leads at 0.12 once S4 lands, so S5 deepens an
      effect S4 introduced rather than creating one. App-state format v11 migrates
      the exact 0.12 detent in **both** physical banks — the v6 rule touched only
      Tom 1, which was right then because that bank did not exist before v9 — and
      the two detent rules stay separately gated so a blob deliberately dragged
      back to 0.45 is preserved. Two latent decoder bugs were fixed alongside:
      the byte-length table had no branch for the immediately preceding version,
      which would have made `decodeState` return null for every blob in every
      user's localStorage and every share link in the wild, and the bank-width
      ternary would have read the previous version's bank one slot short and
      desynchronized every offset after it. Neither was covered; both are now.
- [x] **S6: make the glide audible and cheap.** Raise the tension coefficient
      toward the 157-cent headroom `MaximumTensionRatio = 0.2` already permits,
      and replace the 8-iteration fixed-point solve with an energy-proportional
      single-factor detune (Avanzini et al., _JASA_ 131(1) 2012 — the short-time
      average tension variation is approximately proportional to system energy).
      Every published tom analysis treats the downward glide as _the_
      characteristic feature, so this is worth keeping; it is the 6× cost for
      38 cents that is not.
      **Done 2026-07-30 — half of it, because the other half was unnecessary.**
      The glide is 102.8 cents at full velocity, up from 37.9, from raising both
      tension coefficients fourfold. Measured: ×1 37.9 cents, ×2 65.7, ×4 102.8,
      ×8 135.7, ×16 152.0 against a 157-cent cap, with the quiet-hit glide at 1.5,
      2.3, 3.0, 7.5 and 14.3 — so ×4 is an audible semitone that keeps the
      velocity dependence, while past ×8 the loud hit sits on the `tanh` plateau
      and the glide flattens into a hold-then-drop.
      **The single-factor detune was not implemented, because the 6× premise is
      false.** The solve early-exits: mean **2.88** iterations at full velocity,
      not 8, and sweeping the coefficient over a 32× range moves it only to 3.09,
      since a stiffer law both perturbs the tension more and contracts faster once
      `tanh` saturates. There was no cost to buy back, so the discrete-gradient
      solve keeps its exact energy bookkeeping and nothing was traded for nothing.
      Also worth recording: the guard this item would have relied on is not one.
      `TestNonlinearFrequencyBoundKeepsRetainedModesBelowNyquist` computes its
      bound from `MaximumTensionRatio` and the wavenumbers alone, so the
      coefficient does not appear in it and it cannot fail however far it is
      raised. What actually binds is the 4× oversampled trajectory comparison
      (0.0773 % → 0.0833 % against a 1.5 % ceiling) and the velocity-dependence
      clause. The glide now has an assertion in cents, since the ratio test passes
      at 38 cents and 38 cents is not audible as a bend.
- [ ] **S7: jitter mode frequencies per trigger** by a fraction of a percent so
      repeated hits are not identical (Cook, PhISEM, ICMC 1996). The static
      degenerate split from P6's `TensionAsymmetry` is a different mechanism and
      stays.

Then the architecture:

- [x] **S8 (high): go hybrid.** Pure modal synthesis cannot cover a drum's
      bandwidth in a browser. Mode count for a membrane grows as f²:
      N(f) ≈ (a·k)²/4, so this head needs ~130 oscillators for 1 kHz, ~530 for
      2 kHz and **~3300 for 5 kHz**, against a shipped budget of 48. Keep modal
      synthesis for the low, individually resolved band and add a **separate
      1–8 kHz stochastic attack layer** driven by the contact force, with its own
      fast decay. This is what the published tom-analysis work does — Kirby &
      Sandler (DAFx-20) found 5–10 key modes sufficient for the _sustain_ of a
      central strike precisely because the attack is modelled separately — so it
      is cheaper than what the voice runs today, not more expensive. Fund it from
      S6 and from reducing the resonant head to the axisymmetric modes that
      actually couple to the cavity; its 48 oscillators are currently computed
      and then discarded from the output.
      **Done 2026-07-30**, and the funding claim was stronger than written: 44 of
      the resonant head's 48 oscillators are not merely discarded but provably
      never excited — the strike force reaches only batter modes, and the cavity,
      the sole path between the heads, couples through a swept area that is exactly
      zero for every m > 0 mode. So `Head.AxisymmetricOnly` is **bit-exact**, which
      a test asserts by comparing two renders sample for sample (with `==`, not the
      bit patterns: the only difference is fewer additions of exact zero, and
      `x + 0` maps `-0` to `+0`). The filter runs *after* mode selection, not
      during it — skipping candidates inside the budget loop would free their slots
      and the loop would refill them with higher-order axisymmetric modes that *do*
      drive the cavity, which is a different instrument, not a cheaper one.
      That paid for `Quality.ModeLimit()` becoming the batter budget at 48/96/160,
      taking Standard from 646 Hz to 915 Hz. Bandwidth grows as √N, so this is 0.6
      of an octave, not one — the honest figure, against this item's own arithmetic.
      Measured on js/wasm under Node, worst case with the nonlinear solve never
      idling: **1.66× real time at 102 oscillators**, against the 1.48× the old 96
      cost, with zero allocations. A wider modal band and an added noise layer for
      slightly less than before. Two simultaneous physical toms remain below real
      time (≈ 0.8×), as they already were (0.74×) — improved, not fixed, and Draft
      exists for it.
      The attack layer is one bandpass of noise driven by the **contact force**, so
      hardness and velocity carry into it with no second trigger; a one-pole
      release (20 ms, following DAMP and DEC) so the burst outlasts contact; and
      xorshift64* seeded from a constant and rewound by `Reset`, because much of
      the suite compares renders exactly. Level fitted by spectral balance in the
      43 ms attack window, relative to the strongest low partial: 1–2 kHz goes
      **−66.9 → −32.3 dB** and 2–5 kHz **−83.9 → −27.0 dB**. The first figures are
      the defect — with modal synthesis alone there is nothing up there at all.
      Its envelope enters `IsActive`, or the voice would cut the burst off. New
      `ATK.L` and `ATK.T` at indices 16 and 17; app-state v12. See
      [`docs/physical-hybrid.md`](docs/physical-hybrid.md).
      One bookkeeping fix found on the way: `TestPhysicalTomParamIDsAreStable`
      pinned only 15 of 16 indices, so `D.TILT` sat outside the guard that exists
      to stop a slot's meaning changing under links already in the wild. It now
      requires every index to be pinned.

Deliberately **not** doing:

- [ ] ~~Add exterior air loading to harmonicise the mode ratios.~~ Rejected on
      evidence. Real two-headed drums scatter ±20 % around the ideal Bessel
      series _in both directions_, so no fixed ratio set is right; the practical
      target is batter (1,1) ≈ 1.5× the (0,1) (Richardson, Toulson & Nunn,
      _JASA_ 131(1) 2012), and this model's coupled value is already
      165.4/107.7 = **1.54**. Timpani-style harmonicisation is a kettle/air-load
      effect at a much larger diameter.
- [ ] ~~Model a tom vent (Helmholtz) resonance.~~ A first-principles estimate
      puts a 12"×9" tom near 30 Hz, far below the (0,1). No measurement of one
      was found.

- [x] **S9: the tuning knob was also a sustain knob.** Reported from listening —
      "the drum only sounds good for rather high B.TUNE values" — and the
      measurement found two causes at once.

      **The default pitch was a floor tom.** At the shipped 600 N/m the 12-inch
      batter head's fundamental is 104.00 Hz; the whole range topped out at
      158.85 Hz, so the drum only began to read as a rack tom against the stop.
      Now 1250 N/m and 150.10 Hz, with the range 300–3500 N/m (75–251 Hz) so the
      usable pitch is mid-travel. Every mode moved with it, which incidentally
      raised the top retained mode from 915 Hz to 1310 Hz at Standard.

      **And ζ drifted with the knob.** `Loss1MPerSecond` *is* ζc, but it was
      stored as an absolute constant, so it only meant ζ = 1.1 % at the default
      tension: 2.20 % at the bottom of the range, 0.72 % at the top. A 300 Hz
      partial's T60 therefore ran 0.166 s → 0.423 s across the travel, and
      turning the drum up made it ring half again as long as well as higher. New
      `physical.RetuneTension` scales `Loss1`, `Loss2` and the mode decay
      corrections by √(T_new/T_old) — each of them is proportional to c — and
      `physicalTom.reconfigure` goes through it instead of assigning the tension
      field. `TestRetuningHoldsConstantQ` pins ζ across the range and
      `TestRetuningMovesPitchAndNotMuchElse` states it as a player would: 4× the
      tension is 2× the pitch and ½ the T60, so the ring length in *cycles* does
      not change.

      ζ is now 0.72 %, not 1.1 %, because 0.72 % is what the old coefficients
      happened to produce at the tuning that was reported as sounding right. The
      (0,1) correction stays at its absolute rate, holding that mode's T60 at
      213 ms — the same anchor — which puts it near ζ = 3.4 % rather than the 5 %
      the 104 Hz default produced from the same number.

      Three fitted values had to follow the retuning, none of them optional:
      `Cavity.StiffnessScale` 0.04 → 0.083 (a stiffer head is less moved by the
      same air spring, and 0.04 × 1250/600 = 0.083 is both the argument and the
      measurement); the Berger coefficients ×2 again, to 9.6e6/6.4e6, because the
      same strike is a smaller *relative* tension excursion on a stiffer head —
      the fourfold raise that gave 102.8 cents at 600 N/m gave 20.7 at 1250, and
      the glide is back to 96.9; and `Pickup.OutputGain` 0.0033 → 0.0048 for a
      velocity-1 peak of 0.895.

- [x] **S10: the attack layer sounded like noise, not like a stick.** Reported
      from listening about ATK.L and ATK.T. Two defects, both measurable against
      the model's own loss law.

      **The release was far too long.** `Attack.DecaySeconds` was a fitted 20 ms
      one-pole, which is a 138 ms T60. The head's loss law, extrapolated into the
      band the layer stands for, gives 149 ms at 1 kHz, 75 ms at 2 kHz, 30 ms at
      5 kHz and 18 ms at 8 kHz — so the layer rang about twice too long at the
      bottom of its range and **seven times** too long at the top. Broadband noise
      held that far past the strike does not fuse into the attack; it is heard as
      a separate source.

      **And one rate covered the whole span.** Constant Q means the absolute decay
      rate rises with frequency, so 8 kHz should die several times faster than
      1 kHz. A single release cannot express that.

      The layer is now three bands at 0.4, 1 and 2.5 × `Attack.CentreHz`, each
      with its own envelope whose rate is *derived* from the batter head's loss
      law at that band's centre — 94 ms T60 at 1.6 kHz, 37 ms at 4 kHz, 15 ms at
      10 kHz. `DecaySeconds` became a dimensionless `DecayScale`, defaulting to 1,
      which exists only because the law is being read past where it was fitted.
      `DAMP`, `DEC` and `D.TILT` now reach the layer for free because they are
      applied to the head first — and `D.TILT` genuinely applies, where a single
      band had no shape to tilt.

      `Attack.CentreHz` moved 3 kHz → 4 kHz, which is a separate defect: at 3 kHz
      the lowest band sat at 1.2 kHz, **below** the 1310 Hz top retained mode.
      Noise where the model already has resolved modes is both a double count and
      the wrong texture, since that region is heard as pitch.
      `TestAttackLayerStartsAboveTheModalBand` keeps them apart.

      Three summed envelopes are about three times as loud as one, so `ATK.L` was
      refitted to 0.05 on a narrowed 0–0.15 range. That is the one range change,
      so app-state format version 13 doubles a stored position and moves an
      untouched v12 detent to the new default — the same two-rule shape
      `migrateStrikeRadius` uses, applied to both Tom banks. Physical config
      schema 8 → 9.

Execution order: **S1 + S2 first** — they are cheap, independent, and should be
audible immediately — then S4, S3, S5, then S6/S7, then S8. S1 and S2 landed
together on 2026-07-30, S3 followed the same day ahead of S4 — harmless, they
touch different code — and S4, S5, S6 and S8 landed together on 2026-07-30 in
that order. S9 and S10 were added the same day from listening reports and landed
in that order — S9 first, because the attack layer's derived decay rates read the
retuned head's loss law. **S7 is the only item left**, and it is the smallest.

Exit: per-mode T60 within a documented tolerance of the measured ζ structure,
the (0,1) the fastest-decaying mode rather than the slowest, audible content
above 1 kHz, no compensating output gain, and a hit whose octave-band envelopes
decay at visibly different rates. The regression suite must assert the damping
_shape_, not only its scale — today's tests would pass a uniformly damped bank.

Progress against that exit: four of the five clauses now hold.
`internal/physical/damping_test.go` asserts the damping shape — constant ζ across
the series, decay rate near-proportional to frequency, and the (0,1) the
fastest-decaying mode of the low band. S4 deleted `physicalTomOutputGain`, with a
test in `internal/drum` that fails if a compensating gain comes back. S8 supplied
the bandwidth, and S10's refit restated it against the retuned head and the
three-band layer: 1–2 kHz moves from −45.5 to −31.6 dB and 2–5 kHz from −77.7 to
−31.1 dB relative to the strongest low partial, asserted in both directions so the
layer cannot be quietly removed.

The octave-band envelope clause is the one still open. It is now *possible* —
before S8 there was content in only two bands to compare — and it should be
written rather than deferred a fourth time. Note it needs stating carefully: the
attack layer decays in tens of milliseconds and the modal band in hundreds, so
"the bands' envelopes differ" is nearly guaranteed by construction. The assertion
worth having is quantitative and against the *modal* bands, where a uniformly
damped bank would still pass today's tests.

S10 does add part of it: `TestAttackBandsDecayAtTheirOwnRate` asserts each attack
band matches the loss law at its own centre and decays strictly faster than the
band below, which is the same shape property one octave-band at a time. What is
missing is the modal half.

S3 adds a second shape assertion the criterion did not ask for but needs, in
`internal/physical/cavity_split_test.go`: the (0,1) split inside the measured
10–20 % band, nothing left within 10 dB of the fundamental where the (2,1)
belongs, and the rigid stiffness still overshooting that band — the last so the
fitted scale cannot be quietly deleted as a redundant coefficient.

One qualification the exit criterion needs: the (0,1) is the fastest-decaying
mode _of the low band_, not of the whole bank. Constant Q means modes far above
it legitimately decay faster in absolute rate — the (4,2) at 480 Hz reaches
34 /s against the fundamental's 33 /s — so the assertion is scoped to modes
below 3× the fundamental. Requiring it bank-wide would forbid constant Q.

### P9 — Model-structure gaps

Opened **2026-07-31** from a review of the shipped model against
[`docs/paper/paper.typ`](docs/paper/paper.typ) and against recent literature.
P8 is a calibration phase: every one of its items moves a number the model
already has. These four are different — each is a mechanism the model does not
have, or a piece of evidence the path does not have, and each changes either the
shipped sound or the method rather than a coefficient. None of them is a
prerequisite for closing P8, and none of them is optional if the path's own
success criterion 6 is to mean anything.

- [ ] **M1: the nonlinearity contributes pitch and no spectral content.** The
      Berger law collapses the geometric nonlinearity to a **single scalar over
      total strain**: `nonlinearHead.tensionAt` takes one strain measure
      `S = Σ Γᵢqᵢ²` and returns one tension, and every mode is then detuned by
      the same relative amount, `Δωᵢ² = ΔT·kᵢ²/σ`. No mode can transfer energy
      to any other, at any amplitude. So the only two mechanisms in the model
      that can put energy _at_ a frequency are the contact force's spectrum and
      the stochastic attack layer, and P8's entire spectral investigation is
      constrained by that without ever saying so.
      Real heads struck hard do not behave this way. The membrane's geometric
      nonlinearity comes from a **quartic** potential, `U ∝ ∫(|∇w|²)²dA`, which
      is _even_ in the modal amplitudes, so its force is **cubic and odd** and
      generates only **odd** combinations — `3fₐ`, `2fₐ ± f_b`, `fₐ ± f_b ± f_c`
      — plus the internal resonances those admit between near-commensurate
      modes. It generates **no `2fᵢ` and no `fᵢ ± fⱼ`**: those need a _quadratic_
      potential term, which a shell, a curved plate or a static preload
      asymmetry has and a flat tensioned head does not. Dahl (TMH-QPSR 38(1),
      1997) measures the resulting brightening _with striking force_ — which
      this model currently attributes entirely to the contact pulse shortening
      with velocity and to the attack layer's force-driven envelopes.
      **Design requirement that falls out of the parity: `|P| ≥ 2`.** The lowest
      combination consumes three frequency slots, so a single pump mode reaches
      only `fₐ` and `3fₐ`. Any truncated coupling set `P` tested against a target
      band must therefore contain at least two simultaneously loud modes; a
      self-term-only truncation is guaranteed to return zero for reasons that
      have nothing to do with the coupling's strength, and would falsify nothing.
      **Why this is now actionable rather than aspirational.** Diaz, Constanzo &
      Sandler, "nlm: Real-Time Non-linear Modal Synthesis in Max",
      arXiv:2603.10240 (2026), https://arxiv.org/abs/2603.10240, code at
      https://github.com/rodrigodzf/nlm — coupled nonlinear modal oscillators for
      strings, membranes and plates, with energy-conserving integration, running
      in real time as Max externals. That is the architecture this repository
      already has: a precomputed modal bank, an implicit energy-conserving step,
      and a fixed-point closure that measures 2.88 mean iterations. Feasibility
      is not the open question.
      **Truncation is.** The cubic von Kármán term carries a coupling tensor over
      mode _quadruples_, `Γᵢⱼₖₗ`; even factored, evaluating it is cubic in the
      retained modes per sample, and at 96 batter oscillators that is not
      affordable against a shipped worst case of **1.66× real time on js/wasm**
      (and ≈0.8× for two simultaneous toms, i.e. already unaffordable). So the
      item is not "add von Kármán"; it is **measure how few couplings carry the
      audible effect** — the terms among the three to five loudest low modes
      first, never self-terms alone — with the cost re-measured on `js/wasm`
      under the same worst case (retrigger before every chunk, nonlinear solve
      never idling) and the same zero-allocation contract.
      **And what such a change claims is structure, not magnitude.** The local
      quartic `∫(|∇w|²)²dA` is not exact von Kármán either: full von Kármán
      condenses the in-plane displacement through an Airy stress function, giving
      a quartic with an inverse-biharmonic kernel. Berger is that family's
      _uniform_ limit (in-plane stress spatially constant) and the local quartic
      its _local_ limit (stress follows the local slope, no elastic
      redistribution); the truth is bracketed between them. So the measurable
      claim is which frequencies can be generated and by which mode sets. The
      coefficient is fitted either way, which is also why the falsification below
      is run at the passivity-bounded _maximum_ rather than at a fitted value.
      **Test P8's band first, because it is the cheapest falsification.**
      [`docs/physical-excitation-gap.md`](docs/physical-excitation-gap.md)
      eliminates mode count, microphone geometry, strike footprint, cavity
      coupling and tension asymmetry and lands on the contact force. It never
      considers a nonlinear _source_ term, because the model has none. This
      matters specifically: the excitation deficit is a **comb of exact zeros**
      at every `(k+½)/τ`, and a mode pumped by coupling from the loud (0,1) does
      not depend on `|F(f)|` at its own frequency, so it can be excited where the
      comb has deleted the excitation outright. Hypothesis: cubic coupling among
      the loud low modes deposits energy at `2fₐ ± f_b` inside the gap. Note the
      (0,1) cannot do this alone — at `f₀₁ ≈ 150 Hz` its only self-term is
      `3f₀₁ ≈ 450 Hz`, _below_ the 476–700 Hz band — so the retained set must
      pair it with at least one more loud mode for the experiment to mean
      anything.
      Falsified if, with coupling at its passivity-bounded maximum, the partial
      count in 476–700 Hz measured by `cmd/fit-physical -report-only` against the
      fitted bank stays at 0 — the number every P8 experiment has returned.
      Second measurement, independent of any recording: the attack centroid's
      slope against striking velocity, compared with Dahl's measured slope, with
      the contact pulse and the attack layer both disabled so the slope can only
      come from coupling.
      Two constraints on any implementation. It must keep the discrete-gradient
      energy argument — the coupling has to be the gradient of a single potential
      or the passivity result in
      [`docs/physical-nonlinearity.md`](docs/physical-nonlinearity.md) stops
      holding — and it must respect the anti-alias bound `r < 1/(4ν²) − 1`, which
      currently bounds a _uniform_ detune and says nothing about energy moved to a
      combination frequency above Nyquist — and a cubic term reaches `3f`, so the
      worst case is three times the highest retained mode, not twice.

- [ ] **M2: the cavity has no transverse modes, and that may be visible in the
      reference.** `Cavity` is one lumped compliance with one scalar pressure
      state, so head modes couple to it only through swept area
      `A₀ₙ = 2πR²J₁(z₀ₙ)/z₀ₙ` — which is **identically zero for every m > 0
      mode**. `Head.AxisymmetricOnly` is bit-exact for exactly that reason, and a
      test asserts it sample for sample. That is a property of the model, not of
      a drum: in a real shell the m = 1 and m = 2 transverse air modes exist and
      do couple to the head modes of matching angular order.
      The hypothesis is already recorded in
      [`docs/physical-excitation-gap.md`](docs/physical-excitation-gap.md) §"An
      observation, offered as a hypothesis". The reference's partials at
      **624.4 / 1018.4 / 1331.3 Hz** sit close to the transverse cylinder series
      `j′₁₁, j′₂₁, j′₀₁ × c/2πa` = **634 / 1052 / 1320 Hz** at a = 0.1584 m, and
      the measured ⅓-octave band deficit peaks at **635 Hz**. A transverse cavity
      mode would give m > 0 head modes the coupling path they currently lack, and
      would lend radiating efficiency to the m = 1 modes near it — which is a
      mechanism for why the reference's cluster reads as a sparse ~27 Hz comb
      rather than the dense membrane thicket the model puts there.
      **This is three numbers agreeing on a recording of unknown provenance,
      whose diameter is unknown, so the item is a test and not an
      implementation.** Two further cautions, both from the sources: the
      excitation-gap document is flagged pending re-measurement, because the
      corrected partial estimator finds 14 partials in the right channel rather
      than 7 and changes the size of the deficit every number there rests on; and
      the transverse series depends on the shell radius and on `c`, neither of
      which is known for that recording, so a fit that "confirms" it by choosing a
      radius has confirmed nothing.
      **The cheap version.** Replace the single scalar pressure state with a
      handful of cavity modes (the axisymmetric one the model already has, plus
      m = 1 and m = 2), each with its own overlap coefficient against the head
      modes of matching angular order. The rank-one Sherman–Morrison elimination
      becomes a k × k Woodbury solve, which for k ≤ 4 is still two passes and a
      tiny dense solve rather than the dense system the paper says the coupling
      would otherwise be — so the cost claim is checkable before the physics is.
      Confirmed if a partial appears near the predicted transverse frequency
      whose position moves with **shell radius and sound speed and not with head
      tension**, and if m = 1 head modes acquire measurable output through the
      cavity path with the near-field pickup term removed. Killed if the added
      partials track head tension (then they are head modes, and the coupling
      coefficients are wrong) or if `Head.AxisymmetricOnly` remains bit-exact
      after the change (then nothing was actually coupled).
      **The related suspicion, worth recording with it.**
      `Cavity.StiffnessScale` is fitted at **0.083**, a factor of 12 below the
      rigid ceiling of 1. [`docs/physical-cavity.md`](docs/physical-cavity.md)
      attributes that to shell flex, vent leakage and non-piston mode shapes, and
      those are all real — but a factor of 12 is a lot to hang on them, and part
      of it may be the one-mode reduction mis-setting the compliance. If
      separating the transverse modes moves the fitted scale materially toward
      the ceiling at an unchanged split ratio, that is evidence for the reduction
      being the cause; if it does not move, the shell-and-leak explanation stands
      and should be restated as measured rather than assumed.

- [ ] **M3 (do this one first): measure one real tom.** This is the weakest
      thing on the whole path and the cheapest to fix. Every number in the
      physical model has been checked **against the model**. The committed
      fixture `testdata/physical-reference-v2.json` is generated from the model
      itself, deterministically — the paper says so in "What the model is not":
      it is a regression reference, not an acoustic validation reference. The one
      recording is of unknown provenance, unlicensed, not in the repository, and
      **no test depends on it or may**.
      [`docs/physical-tom-review.md`](docs/physical-tom-review.md) §"Where the
      literature is genuinely thin" states the resulting position plainly: no
      published modal table for a mounted 12–14" tom, no measured felt-mallet
      contact time, no numeric radiation-vs-internal damping split for any drum,
      no published overall T60 for a tom hit. The anchors actually in use are a
      snare (Rossing), a student tom report (Sørensen) and a student snare
      project (Fischer) — and the cavity split ratio, one of only four _measured_
      rows in the paper's provenance table, comes from the snare.
      A phone microphone plus a contact mic or a cheap accelerometer produces a
      modal table and a T60 curve in an afternoon. What to capture:
      - modal frequencies and their ratios to the (0,1), for the model's ±20 %
        scatter claim and for `TensionAsymmetry`;
      - per-mode T60, which is the ζ-versus-frequency structure S1/S2 were
        calibrated to from the literature and never checked on an instrument;
      - the coupled (0,1) doublet **with and without the resonant head at
        unchanged batter tuning** — Fischer's protocol, applied to a tom instead
        of a snare, which is the direct measurement of the cavity split ratio
        that `Cavity.StiffnessScale` is fitted to;
      - a strike-position series (centre, 0.30 R, near the rim), since S5 showed
        HIT.R trades low-end weight monotonically and S4 made MIC.R the strongest
        timbral control — both currently unfalsifiable;
      - repeated hits at one dynamic, which is the only thing that can size S7's
        per-trigger jitter;
      - contact time across dynamics if a piezo on the stick is feasible, against
        Wagner's 3.5 ms pulse inside an 8 ms dwell.
      Record provenance with it and commit the **derived tables** — drum make and
      size, heads, tuning, mic distance and angle, room, rig, and a licence — so
      a test may finally depend on a measurement. The audio itself need not be
      committed for the tables to be usable.
      What it buys: it converts fitted rows of the paper's provenance table into
      measured ones, and it converts measured-from-a-different-instrument rows
      into instrument-matched ones. **M1 and M2 are worth materially less without
      it**, because a model-structure change with no external reference can only
      be judged by ear — which is how S9 and S10 arrived, and both of those
      turned out to be right for reasons the ear could not have supplied.

- [ ] **M4: the fitting method is derivative-free over a differentiable model.**
      The paper's "The search" describes a Mayfly swarm over an expensive render;
      "Seeding a restart from the reference's partials" then observes that mode
      frequencies are **analytic** — read off tension, radius and cavity without
      rendering a sample, at roughly a hundredth of the cost of one evaluation —
      and uses that observation to seed 2 of 8 restarts, for a measured 12 %
      better result at equal budget.
      That observation is stronger than the use it is put to. The modal bank is a
      sum of exponentially damped sinusoids in its own parameters: ∂f/∂T and
      ∂f/∂σ follow from `f ∝ √(T/σ)`, and ∂γ/∂d₀, ∂γ/∂d₁, ∂γ/∂d₂ are linear by
      construction of the loss law. A large part of the search space has closed-
      form derivatives that the search does not use at all.
      References. Zheleznov, Bilbao, Wright & King, "Stable Differentiable Modal
      Synthesis for Learning Nonlinear Dynamics", arXiv:2601.10453 (JAES, DAFx
      special issue) — gradient-based learning of _nonlinear_ modal dynamics with
      the physical parameters kept accessible and constrained to stay physical
      through training, which is exactly the failure mode a naive autodiff fit
      hits. One correction to how it is usually cited here: it is demonstrated on
      **synthetic** data from a nonlinear string, not on real recordings, so it
      establishes the technique and not the result. Also Lee, Choi, Kim et al.,
      "Differentiable Modal Synthesis for Physical Modeling of Planar String
      Sound and Motion", NeurIPS 2024.
      **Which of the two this proposes to change: the offline fitting tool, not
      the runtime.** The engine is Go and compiles to WASM; the differentiable-
      synthesis ecosystem is PyTorch/JAX. Two honest options, and the item is to
      choose between them rather than to assume the second:
      - _Gradients in Go._ Extend the analytic pre-solve from frequencies to
        decay rates and per-mode levels and gradient-descend that sub-problem,
        leaving Mayfly for the parameters that genuinely need a render (contact,
        attack layer, microphone). No second model, no drift, and it reuses
        machinery that already exists and is already paired-tested.
      - _A second implementation._ A PyTorch/JAX mirror of the modal bank would
        buy full end-to-end gradients and cost a duplicate model that must be
        kept numerically equal to the Go one across every config-schema
        migration. Given that the schema is at version 10 with an explicit
        migration for each, drift is the main risk and a cross-check test against
        the Go render is the minimum price of entry.
      One obstacle that belongs to either option: the objective in
      `internal/physical/match` is **not differentiable end to end** — partial
      detection, peak picking and the coverage shares are all discrete. A
      gradient method needs a differentiable surrogate (multi-resolution STFT is
      the usual choice), which is a _different_ distance from the one the paper
      defines and gates on, so the surrogate would have to be shown to correlate
      with the gated terms before any result from it counts.
      Related, and the reason this item touches the attack layer too:
      [`docs/physical-excitation-gap.md`](docs/physical-excitation-gap.md)
      records the seam as a fit pathology — the search dragged `ATK.T` from its
      4 kHz default down to **1644 Hz** and pinned `ATK.L` at **0.021**, spending
      both attack parameters on a band neither was built for (1261 Hz in the
      later prescribed-contact fit; 3426 Hz under Hertzian, where the seam
      closes). Shier, Caspe, Robertson, Sandler, Saitis & McPherson,
      "Differentiable Modelling of Percussive Audio with Transient and Spectral
      Synthesis", https://arxiv.org/pdf/2309.06649, train transient and spectral
      encoders **jointly** rather than layering one over the other's ceiling,
      which is the same seam approached from the fitting side rather than the
      model side.

Exit: one measured tom exists in the repository as a licensed, provenance-
carrying derived table that at least one test depends on; M1 and M2 have each
been either implemented with a measured improvement against that table and a
re-measured `js/wasm` cost, or closed by the measurement that rejected them,
with the rejecting measurement written down; and M4 is resolved as a documented
decision between the two options above with a cost attached — code is not
required for it to exit, a decision is.

Ordering: **M3, then M2, then M1, then M4.** M3 first because it is the only
item that makes the others falsifiable — M2's whole hypothesis rests on three
numbers from a recording whose diameter is unknown, and M1's brightening claim
needs a measured centroid-versus-force slope to be checked against. M2 second
because it is one experiment, already specified, that would explain two open
anomalies at once (the 635 Hz deficit and the factor-of-12 stiffness scale) and
because a rank-4 Woodbury solve is a smaller change than any coupling scheme.
M1 third: it is the real physics gap and it is now demonstrably real-time, but
it is also the one item that can move the shipped sound in ways no current test
would catch, so it wants an external reference in place first. M4 last because
it changes the method rather than the instrument — a better search cannot find
what the model cannot produce, which is the same argument
[`docs/physical-excitation-gap.md`](docs/physical-excitation-gap.md) uses to
rule out a longer run.

One qualification on that order, from reading the sources rather than from
assuming it: M2 is partly testable _before_ M3, because its confirm/kill
criteria above (partials that move with radius and not tension;
`AxisymmetricOnly` ceasing to be bit-exact) are internal to the model and need
no recording at all. Only the last step — checking the predicted transverse
frequencies against a real shell — needs M3. So M2's implementation can start in
parallel; only its verdict has to wait.

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
6. **It sounds like the instrument.** Per-mode decay, the radiated tonal
   balance, and the attack transient are each checked against a cited
   measurement rather than only against the model's own analytic targets, and no
   compensating output gain, EQ or envelope stands between the physics and the
   mix. P8 exists because criteria 1–5 were all met while criterion 6 was not:
   every internal invariant held, and the voice still did not sound like a tom.

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
