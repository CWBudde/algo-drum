# Voice architecture

This is a reference for the five drum synths in `internal/drum/voices.go` — what
each one generates, which named constants shape the sound, and how a hit
travels from `Voice.Tick()` to the speakers via `internal/drum/engine.go`.

All voices implement the same interface:

```go
type Voice interface {
    Trigger(velocity float64)            // velocity in [0, 1] scales the whole hit
    Tick() float64                       // one sample, mono, roughly in [-1, 1]
    IsActive() bool                      // false once the envelope has decayed to silence
    SetDecay(amount float64)             // amount in [0, 1], trims the base decay 0.5x-1.5x
    SetParam(index int, value01 float64) // one synthesis parameter, normalized
    Param(index int) float64
    ParamSpecs() []ParamSpec
}
```

Nothing here reads from disk or a network — every voice is procedural
(oscillator or noise generator + envelope + optional biquad filter).

## Runtime parameters

Every tuning value below is a **named constant** rather than a magic number in
the signal path (PLAN.md **E5**), and each one is now also reachable at runtime
(PLAN.md **G20**, which closes **D11**). The constants did not go away — they
are the `Shipped` field of the parameter specs in `internal/drum/params.go`,
which is what pins the default sound.

A parameter is addressed by `(track, index)` and set from a normalized `[0, 1]`
position via `setVoiceParam` on the JS API. `internal/drum/params.go` maps that
position onto engineering units:

- **exp**: `min · (max/min)^v` — every frequency and every time, because the ear
  hears ratios rather than differences.
- **lin**: `min + (max − min) · v` — levels and mixes, so 0 really is silence.

`ParamSpec.Map` snaps to `Shipped` within half a persistence byte step
(`|v − Default| < 1/510`). Persistence stores each scalar as one byte, so a
default of `0.4648` would otherwise come back as `0.4667` and retune a 200 Hz
body to 205 Hz on every reload. The dead zone is ±0.2 %, sub-pixel on the knob's
150 px sweep, and reads as a detent at the default position. The TypeScript
mirror (`web/src/engine/voiceParams.ts`) applies the same snap, and both sides
pin the same triples in tests.

**Decay has two controls.** The strip's `DEC` knob (`setDecay`, unchanged) is a
trim; the modal's `TIME` parameter is the base:

```
effective decay = base decay parameter × (decayScaleMin + strip knob)
                = base × (0.5 … 1.5)
```

**Not exposed on purpose:** `envSilence` — raising it stops a voice ever
deactivating, leaving it stuck and burning CPU — and `decayScaleMin`, which
defines what the persisted `setDecay` byte means, so changing it would silently
reinterpret every share link already in the wild.

Filter-backed voices (Snare highpass, Hi-Hat and Cymbal bandpass) recompute
their coefficients in place when a tone parameter changes. Only
`Section.Coefficients` is reassigned; the delay line is left alone, so a hit
already ringing keeps ringing rather than clicking. `clampDesignHz` keeps the
frequency inside `(0, Nyquist)`, because `design.Bandpass`/`Highpass` return
all-zero coefficients — a silent voice — at or above Nyquist.

The per-voice tables below list each parameter's index, ID, curve and range
alongside the constant it defaults to.

## Shared building blocks

A few constants and helpers in `voices.go` are shared across voices:

| Name             | Value  | Meaning                                                                                                                                           |
| ---------------- | ------ | ------------------------------------------------------------------------------------------------------------------------------------------------- |
| `envSilence`     | `1e-4` | Envelope level below which a voice deactivates (`IsActive()` → false).                                                                            |
| `decayScaleMin`  | `0.5`  | `SetDecay(amount)` scales the voice's base decay time by `decayScaleMin + amount`, i.e. **0.5x–1.5x** of the base decay for `amount` in `[0, 1]`. |
| `pitchSweepRate` | `5.0`  | Exponential rate of the pitch-sweep decay used by Bass Drum and Tom; higher settles onto the target pitch faster.                                 |

Two helper functions do the actual math:

- `decayCoef(sr, decayS)` converts a decay time in seconds into a per-sample
  one-pole multiplier (`env *= coef` each tick), floored at 5 ms so `SetDecay`
  can never fully freeze a voice.
- `newVoiceRng(seed)` returns a `math/rand/v2` PCG generator seeded with
  `rand.NewPCG(seed, seed)`. Each noise-based voice (Snare, Hi-Hat, Cymbal)
  keeps **one** `*rand.Rand` for its whole lifetime, created once in the
  constructor and never reseeded on `Trigger` — successive hits draw further
  along the same deterministic stream. See
  [Determinism](#determinism-and-the-golden-render-tests) below.

`clamp01` clamps any float to `[0, 1]`; used everywhere a velocity, decay
amount, or reverb amount comes in from the JS API.

---

## Bass Drum (track 0)

**Recipe:** a sine oscillator whose pitch sweeps down exponentially from an
attack frequency to a resting frequency, shaped by a single decay envelope.
No filter, no noise — the pitch sweep itself supplies the "thump".

```go
freq := bassPitchToHz + (bassPitchFromHz-bassPitchToHz)*math.Exp(-t*pitchSweepRate)
sample := math.Sin(phase) * env
```

| Constant          | Value      | What it controls                                                                                    |
| ----------------- | ---------- | --------------------------------------------------------------------------------------------------- |
| `bassPitchFromHz` | `200.0 Hz` | Pitch at the instant of trigger (the "click").                                                      |
| `bassPitchToHz`   | `50.0 Hz`  | Pitch the sweep settles on (the "body").                                                            |
| `bassPitchTCS`    | `0.06 s`   | Time constant of the pitch sweep — how fast it drops from `bassPitchFromHz` toward `bassPitchToHz`. |
| `bassBaseDecayS`  | `0.45 s`   | Base amplitude-envelope decay before `SetDecay` scaling.                                            |

**Decay:** `SetDecay(amount)` scales the `bass.decay` parameter by
`decayScaleMin + amount` → **0.225 s – 0.675 s** with that parameter at its
default, or **0.025 s – 3.0 s** across its full range.
**Velocity:** `env = clamp01(velocity)` — linear gain, no timbral change.
**RNG:** none; deterministic per pitch/decay/velocity, no seed needed.

**Parameters** (`bassSpecs`, `internal/drum/params.go`):

| idx | ID               | Label | Curve | Range       | Default constant      |
| --- | ---------------- | ----- | ----- | ----------- | --------------------- |
| 0   | `bass.pitchFrom` | ATK   | exp   | 60–800 Hz   | `bassPitchFromHz` 200 |
| 1   | `bass.pitchTo`   | TUNE  | exp   | 25–120 Hz   | `bassPitchToHz` 50    |
| 2   | `bass.sweepTime` | SWP   | exp   | 0.005–0.5 s | `bassPitchTCS` 0.06   |
| 3   | `bass.sweepRate` | SNAP  | exp   | 1–20        | `pitchSweepRate` 5.0  |
| 4   | `bass.decay`     | TIME  | exp   | 0.05–2.0 s  | `bassBaseDecayS` 0.45 |

`sweepRate` is a per-voice field initialised from the shared `pitchSweepRate`
constant, so the Bass Drum and the Tom can now be swept independently. Nothing
stops `pitchFrom` being dragged below `pitchTo`; the sweep simply rises instead
of falling.

---

## Snare (track 1)

**Recipe:** a tuned sine "body" tone plus highpass-filtered white noise
("snap"), mixed together. Each layer has its own envelope and decay time so
the tone can die out independently from the noise.

```go
tone := math.Sin(phase) * toneEnv       // snareToneHz
noise := hpFilter.ProcessSample((rng.Float64()*2 - 1) * noiseEnv)
return tone + noise
```

| Constant          | Value       | What it controls                                                                                                     |
| ----------------- | ----------- | -------------------------------------------------------------------------------------------------------------------- |
| `snareToneHz`     | `200.0 Hz`  | Frequency of the body oscillator.                                                                                    |
| `snareToneLevel`  | `0.7`       | Tone level relative to the noise layer at trigger time (tone starts at `0.7 * velocity`, noise at `1.0 * velocity`). |
| `snareBaseToneS`  | `0.12 s`    | Base decay of the tone layer.                                                                                        |
| `snareBaseNoiseS` | `0.18 s`    | Base decay of the noise layer (outlasts the tone).                                                                   |
| `snareHPHz`       | `2000.0 Hz` | Highpass cutoff applied to the noise layer (`design.Highpass`).                                                      |
| `snareHPQ`        | `0.7`       | Q of that highpass (`biquad.Section` via `design.Highpass`).                                                         |
| `snareSeed`       | `42`        | Fixed PCG seed for the noise generator.                                                                              |

**Decay:** `SetDecay(amount)` scales _both_ the `snare.toneDecay` and
`snare.noiseDecay` parameters by the same `decayScaleMin + amount` factor, so
the strip knob keeps their relationship intact; the two parameters themselves
set it (**0.01 s – 1.5 s** and **0.01 s – 2.25 s** including the trim).
**Velocity:** scales tone and noise envelopes independently at trigger
(`toneEnv = snareToneLevel * vel`, `noiseEnv = vel`) with a fixed decay rate,
so a harder hit is louder, not longer.
**RNG:** `newVoiceRng(snareSeed)`; `hpFilter.Reset()` runs on every `Trigger`
so filter state doesn't carry a click from the previous hit, but the noise
stream itself is _not_ reseeded (see
[Determinism](#determinism-and-the-golden-render-tests)).

**Parameters** (`snareSpecs`):

| idx | ID                 | Label | Curve | Range       | Default constant       |
| --- | ------------------ | ----- | ----- | ----------- | ---------------------- |
| 0   | `snare.toneHz`     | BODY  | exp   | 100–500 Hz  | `snareToneHz` 200      |
| 1   | `snare.toneLevel`  | MIX   | lin   | 0–1         | `snareToneLevel` 0.7   |
| 2   | `snare.toneDecay`  | B.DEC | exp   | 0.02–1.0 s  | `snareBaseToneS` 0.12  |
| 3   | `snare.noiseDecay` | S.DEC | exp   | 0.02–1.5 s  | `snareBaseNoiseS` 0.18 |
| 4   | `snare.hpHz`       | SNAP  | exp   | 200–8000 Hz | `snareHPHz` 2000       |
| 5   | `snare.hpQ`        | RES   | exp   | 0.3–4       | `snareHPQ` 0.7         |

The two decay times are separate parameters, so the tone/noise balance is no
longer fixed — but the strip's `DEC` trim still scales both together, keeping
the documented relationship as it is swept. `MIX` at 0 silences the body and
leaves the snap.

---

## Hi-Hat (track 2)

**Recipe:** white noise through a bandpass filter tuned high, with a fast
envelope and a fixed make-up gain to compensate for the bandpass's
attenuation. This is the closed-hat only; an open-hat track is deferred (see
`PLAN.md` **G7**).

```go
sample := bpFilter.ProcessSample((rng.Float64()*2 - 1) * env)
return sample * hatGain
```

| Constant        | Value        | What it controls                                                                                |
| --------------- | ------------ | ----------------------------------------------------------------------------------------------- |
| `hatBPHz`       | `10000.0 Hz` | Center frequency of the metallic bandpass (`design.Bandpass`).                                  |
| `hatBPQ`        | `2.0`        | Q of that bandpass — narrower Q rings more.                                                     |
| `hatBaseDecayS` | `0.04 s`     | Base envelope decay — short, giving the closed-hat "tick" (vs. the Cymbal's long decay).        |
| `hatGain`       | `1.5`        | Make-up gain applied after the bandpass, to bring the filtered noise back up to a usable level. |
| `hatSeed`       | `123`        | Fixed PCG seed for the noise generator.                                                         |

**Decay:** `SetDecay(amount)` scales the `hat.decay` parameter by
`decayScaleMin + amount` → **0.02 s – 0.06 s** with that parameter at its
default, which keeps the "closed" character; the parameter itself widens this
to **0.0025 s – 0.6 s**.
**Velocity:** linear gain via `env = clamp01(velocity)`.
**RNG:** `newVoiceRng(hatSeed)`; `bpFilter.Reset()` on every `Trigger`.

**Parameters** (`hatSpecs`):

| idx | ID          | Label | Curve | Range         | Default constant     |
| --- | ----------- | ----- | ----- | ------------- | -------------------- |
| 0   | `hat.bpHz`  | TONE  | exp   | 2000–16000 Hz | `hatBPHz` 10000      |
| 1   | `hat.bpQ`   | RES   | exp   | 0.5–8         | `hatBPQ` 2.0         |
| 2   | `hat.decay` | TIME  | exp   | 0.005–0.4 s   | `hatBaseDecayS` 0.04 |
| 3   | `hat.gain`  | LVL   | lin   | 0–2.5         | `hatGain` 1.5        |

`hat.decay` can now be pushed past the "closed" character it was designed for;
a genuinely open hat is still a separate voice/track (**G7**), not just a longer
envelope on this one.

---

## Tom (track 3)

**Recipe:** the same pitch-sweep sine architecture as the Bass Drum, tuned
to a higher pair of frequencies, with its own gain stage.

```go
freq := tomPitchToHz + (tomPitchFromHz-tomPitchToHz)*math.Exp(-t*pitchSweepRate)
sample := math.Sin(phase) * env
return sample * tomGain
```

| Constant         | Value      | What it controls                                                                                      |
| ---------------- | ---------- | ----------------------------------------------------------------------------------------------------- |
| `tomPitchFromHz` | `120.0 Hz` | Pitch at trigger.                                                                                     |
| `tomPitchToHz`   | `60.0 Hz`  | Pitch the sweep settles on.                                                                           |
| `tomPitchTCS`    | `0.1 s`    | Time constant of the pitch sweep (slower than the Bass Drum's `0.06 s`, giving a more audible glide). |
| `tomBaseDecayS`  | `0.35 s`   | Base amplitude-envelope decay before `SetDecay` scaling.                                              |
| `tomGain`        | `0.9`      | Output gain applied after the envelope.                                                               |

**Decay:** `SetDecay(amount)` scales the `tom.decay` parameter by
`decayScaleMin + amount` → **0.175 s – 0.525 s** at its default, or
**0.025 s – 3.0 s** across its full range.
**Velocity:** linear gain via `env = clamp01(velocity)`.
**RNG:** none; deterministic like the Bass Drum.

**Parameters** (`tomSpecs`):

| idx | ID              | Label | Curve | Range       | Default constant     |
| --- | --------------- | ----- | ----- | ----------- | -------------------- |
| 0   | `tom.pitchFrom` | ATK   | exp   | 60–600 Hz   | `tomPitchFromHz` 120 |
| 1   | `tom.pitchTo`   | TUNE  | exp   | 30–300 Hz   | `tomPitchToHz` 60    |
| 2   | `tom.sweepTime` | SWP   | exp   | 0.005–0.5 s | `tomPitchTCS` 0.1    |
| 3   | `tom.sweepRate` | SNAP  | exp   | 1–20        | `pitchSweepRate` 5.0 |
| 4   | `tom.decay`     | TIME  | exp   | 0.05–2.0 s  | `tomBaseDecayS` 0.35 |
| 5   | `tom.gain`      | LVL   | lin   | 0–2         | `tomGain` 0.9        |

Same shape as the Bass Drum, plus its own output level.

---

## Cymbal (track 4)

**Recipe:** structurally identical to the Hi-Hat — filtered white noise with
a make-up gain — but tuned to a lower bandpass center and a much longer
decay, giving a wash instead of a tick.

```go
sample := bpFilter.ProcessSample((rng.Float64()*2 - 1) * env)
return sample * cymGain
```

| Constant        | Value       | What it controls                                                                             |
| --------------- | ----------- | -------------------------------------------------------------------------------------------- |
| `cymBPHz`       | `7000.0 Hz` | Center frequency of the bandpass (`design.Bandpass`) — lower than the Hi-Hat's `10000.0 Hz`. |
| `cymBPQ`        | `1.2`       | Q of that bandpass — wider than the Hi-Hat's `2.0`, giving a broader, washier band.          |
| `cymBaseDecayS` | `1.2 s`     | Base envelope decay — long, giving the cymbal its sustained tail.                            |
| `cymGain`       | `1.2`       | Make-up gain applied after the bandpass.                                                     |
| `cymSeed`       | `999`       | Fixed PCG seed for the noise generator.                                                      |

**Decay:** `SetDecay(amount)` scales the `cym.decay` parameter by
`decayScaleMin + amount` → **0.6 s – 1.8 s** at its default, or
**0.05 s – 6.0 s** across its full range.
**Velocity:** linear gain via `env = clamp01(velocity)`.
**RNG:** `newVoiceRng(cymSeed)`; `bpFilter.Reset()` on every `Trigger`.

**Parameters** (`cymSpecs`):

| idx | ID          | Label | Curve | Range         | Default constant    |
| --- | ----------- | ----- | ----- | ------------- | ------------------- |
| 0   | `cym.bpHz`  | TONE  | exp   | 1000–14000 Hz | `cymBPHz` 7000      |
| 1   | `cym.bpQ`   | RES   | exp   | 0.3–6         | `cymBPQ` 1.2        |
| 2   | `cym.decay` | TIME  | exp   | 0.1–4.0 s     | `cymBaseDecayS` 1.2 |
| 3   | `cym.gain`  | LVL   | lin   | 0–2           | `cymGain` 1.2       |

Structurally the same table as the Hi-Hat with different ranges, which is why
both voices render from the same descriptor-driven editor UI. At the top of its
range a cymbal takes ~55 s to fall below `envSilence`; that is inaudible long
before, but it is why the parameter-extremes test uses a longer cap than the
30 s one `tickUntilInactive` applies elsewhere.

---

## Signal flow: the master chain

Once triggered, a voice's `Tick()` output travels through `Engine.Render`
(`internal/drum/engine.go`) once per sample, in this order:

1. **Per-voice `Tick()`.** Each of the 5 voices produces one sample (0 if
   inactive).
2. **Volume smoothing.** Each track's live gain (`liveVol[t]`) ramps toward
   its target (`volumes[t]`, set by `SetVolume`) with a one-pole filter:
   `liveVol[t] += (volumes[t] - liveVol[t]) * volCoef`, where `volCoef` is
   derived from `volSmoothTauS` (`0.008 s`, ~8 ms) — fast enough to feel
   instant on a knob, slow enough to avoid zipper noise on live changes.
3. **Mix + headroom.** The 5 volume-scaled voice outputs are summed, then
   scaled by `mixHeadroom` (`0.5`) so that simultaneous hits on all tracks
   don't slam the limiter — the limiter is meant to catch rare worst cases,
   not do steady-state gain reduction.
4. **FDN reverb (conditional).** If `reverbAmount > 0`, the sample passes
   through `Engine.reverb` (`reverb.FDNReverb` from algo-dsp). `SetReverb(amount)`
   maps the UI's `[0, 1]` knob to `wet = amount * 0.45` (so reverb is never
   more than 45% wet) and `RT60 = 0.3 + amount * 3.7` seconds (0.3 s dry-ish
   room up to a 4 s tail at full reverb).
5. **Lookahead limiter.** `Engine.limiter` (`dynamics.LookaheadLimiter` from
   algo-dsp), threshold fixed at `-1.0 dB` via `SetThreshold`. This is the
   real gain-reduction stage for sustained loud passages (dense patterns,
   long reverb tails) — its smoothed detector still under-reacts to
   single-sample transients.
6. **Hard clamp.** A final `if out > 1 / out < -1` clamp to `[-1, 1]` is the
   brick wall for whatever the limiter's detector missed; the browser's
   output stage would clip anyway, so this just guarantees the contract of
   `Render` producing values in range.

```
voice.Tick() ×5 → × liveVol[t] (smoothed) → Σ → × mixHeadroom → FDN reverb → limiter → clamp[-1,1] → buf[i]
```

## Determinism and the golden render tests

Every noise source in `voices.go` is a `math/rand/v2` PCG generator created
with a **fixed** seed — `snareSeed = 42`, `hatSeed = 123`, `cymSeed = 999` —
via `newVoiceRng(seed)`. The seed is fixed at construction time and the
generator is never reseeded afterward; `Trigger` resets filter state
(`hpFilter.Reset()` / `bpFilter.Reset()`) but not the RNG, so a voice's noise
stream just keeps advancing from hit to hit.

That means: given the same sample rate, the same sequence of API calls
(`SetCell`, `SetTempo`, `SetReverb`, …), and the same number of rendered
samples, `Engine.Render` produces **bit-identical output** every time —
there is no wall-clock seeding, no `time.Now()`, nothing non-reproducible in
the signal path. `internal/drum/engine_test.go`'s `TestRenderDeterministic`
builds two independent engines with an identical setup and asserts every
rendered sample matches exactly; this is only possible because of the fixed
PCG seeds here. Any future change that reseeds a voice's RNG per-trigger (to
add hit-to-hit variation, say) would need to either accept that it breaks
bit-exact reproducibility, or thread a seed derived from something
deterministic (e.g. step index) rather than real time.

**Auditioning breaks the "same link, same audio" property.** `TriggerVoice`
(the voice editor's AUDITION button) fires a voice outside the sequencer, and
for the noise voices that advances the same PCG stream a later rendered hit
draws from. The result is inaudible — white noise is white either way — and no
test is affected, because none of them audition. But it does mean a share link
only reproduces sample-for-sample until the first audition. If bit-exactness
ever has to survive that (an offline WAV export, PLAN.md **G12**), the escape
hatch is to marshal each noise voice's PCG state around the audition trigger;
that allocates, but only on a UI gesture, never inside `Render`.

Turning the tuning constants into runtime parameters did not change any of
this: `TestVoiceParamDefaultsAreShippedConstants` and
`TestDefaultVoiceParamsPreserveRender` assert that a fresh voice (and a fresh
engine) renders bit-identically to one whose every parameter was explicitly
written to its default, which is what the byte-step snap in `ParamSpec.Map`
buys.
