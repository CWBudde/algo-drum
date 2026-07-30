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
- [ ] Fit documented presets from measurement while preserving the underlying
      SI parameters and provenance.
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

Exit: a measured tom can be matched within documented tolerances for modal
frequency, decay, and spectrum across more than one hit.

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
attributable to a measurement. Rows 1, 2 and 4 are closed by S1, S2 and S3 below;
rows 3 and 5 are open, and the numbers in them still describe the shipped voice.

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
- [ ] **S4: fix the radiated sum.** Weight volume **acceleration** by
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
- [ ] **S5: move the default strike radius** from 0.12 to ≈ 0.3 of the radius.
- [ ] **S6: make the glide audible and cheap.** Raise the tension coefficient
      toward the 157-cent headroom `MaximumTensionRatio = 0.2` already permits,
      and replace the 8-iteration fixed-point solve with an energy-proportional
      single-factor detune (Avanzini et al., _JASA_ 131(1) 2012 — the short-time
      average tension variation is approximately proportional to system energy).
      Every published tom analysis treats the downward glide as _the_
      characteristic feature, so this is worth keeping; it is the 6× cost for
      38 cents that is not.
- [ ] **S7: jitter mode frequencies per trigger** by a fraction of a percent so
      repeated hits are not identical (Cook, PhISEM, ICMC 1996). The static
      degenerate split from P6's `TensionAsymmetry` is a different mechanism and
      stays.

Then the architecture:

- [ ] **S8 (high): go hybrid.** Pure modal synthesis cannot cover a drum's
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

Execution order: **S1 + S2 first** — they are cheap, independent, and should be
audible immediately — then S4, S3, S5, then S6/S7, then S8. S1 and S2 landed
together on 2026-07-30, and S3 followed the same day ahead of S4, which is
harmless: they touch different code and S3 does not depend on the radiated sum.
**S4 is next.**

Exit: per-mode T60 within a documented tolerance of the measured ζ structure,
the (0,1) the fastest-decaying mode rather than the slowest, audible content
above 1 kHz, no compensating output gain, and a hit whose octave-band envelopes
decay at visibly different rates. The regression suite must assert the damping
_shape_, not only its scale — today's tests would pass a uniformly damped bank.

Progress against that exit: the first two clauses hold, and
`internal/physical/damping_test.go` now asserts the shape — constant ζ across
the series, decay rate near-proportional to frequency, and the (0,1) the
fastest-decaying mode of the low band. Three clauses remain, each owned by a
later item: bandwidth above 1 kHz (S8), removing `physicalTomOutputGain` (S4),
and the octave-band envelope check, which is only meaningful once there is
content in more than two bands to compare.

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
