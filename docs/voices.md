# Voice architecture

This is a reference for the five drum synths in `internal/drum/voices.go` — what
each one generates, which named constants shape the sound, and how a hit
travels from `Voice.Tick()` to the speakers via `internal/drum/engine.go`.

All voices implement the same interface:

```go
type Voice interface {
    Trigger(velocity float64) // velocity in [0, 1] scales the whole hit
    Tick() float64            // one sample, mono, roughly in [-1, 1]
    IsActive() bool           // false once the envelope has decayed to silence
    SetDecay(amount float64)  // amount in [0, 1], see per-voice tables below
}
```

Nothing here reads from disk or a network — every voice is procedural
(oscillator or noise generator + envelope + optional biquad filter).

Every tuning value below is already a **named constant** rather than a magic
number in the signal path — that part is done (PLAN.md **E5**). What is still
missing is any way to change them at runtime: `SetDecay` is the only per-voice
parameter on the JS API, so pitch, filter and gain constants remain
compile-time. Exposing them ("tune"/"snap" per voice) is tracked as PLAN.md
**G20**. The per-voice notes below therefore say which constant a future
control should drive and what range makes sense.

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

**Decay:** `SetDecay(amount)` scales `bassBaseDecayS` by `decayScaleMin + amount`
→ effective decay ranges **0.225 s – 0.675 s**.
**Velocity:** `env = clamp01(velocity)` — linear gain, no timbral change.
**RNG:** none; deterministic per pitch/decay/velocity, no seed needed.

A future tune parameter would most naturally replace `bassPitchFromHz` /
`bassPitchToHz` (a "pitch"/"punch" knob) or `bassPitchTCS` (a "sweep speed"
knob); `pitchSweepRate` is shared with the Tom so changing it would affect
both voices.

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

**Decay:** `SetDecay(amount)` scales _both_ `snareBaseToneS` and
`snareBaseNoiseS` by the same `decayScaleMin + amount` factor — tone and noise
stay in the same 0.5x–1.5x relationship to each other as decay changes.
**Velocity:** scales tone and noise envelopes independently at trigger
(`toneEnv = snareToneLevel * vel`, `noiseEnv = vel`) with a fixed decay rate,
so a harder hit is louder, not longer.
**RNG:** `newVoiceRng(snareSeed)`; `hpFilter.Reset()` runs on every `Trigger`
so filter state doesn't carry a click from the previous hit, but the noise
stream itself is _not_ reseeded (see
[Determinism](#determinism-and-the-golden-render-tests)).

Natural tune-parameter candidates: `snareToneHz` (body pitch), `snareHPHz`
(brightness/snap), and `snareToneLevel` (tone vs. noise balance).

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

**Decay:** `SetDecay(amount)` scales `hatBaseDecayS` by `decayScaleMin + amount`
→ effective decay ranges **0.02 s – 0.06 s**, still very short at the top of
the range — this voice is meant to stay a "closed" hat.
**Velocity:** linear gain via `env = clamp01(velocity)`.
**RNG:** `newVoiceRng(hatSeed)`; `bpFilter.Reset()` on every `Trigger`.

A tune parameter here would likely target `hatBPHz`/`hatBPQ` (brightness) or
`hatBaseDecayS` capped low enough to keep the "closed" character; a
genuinely open hat is a separate voice/track (G7), not just a wider decay
range on this one.

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

**Decay:** `SetDecay(amount)` scales `tomBaseDecayS` by `decayScaleMin +
amount` → effective decay ranges **0.175 s – 0.525 s**.
**Velocity:** linear gain via `env = clamp01(velocity)`.
**RNG:** none; deterministic like the Bass Drum.

Same shape as the Bass Drum section above: `tomPitchFromHz`/`tomPitchToHz`
are the obvious "pitch" knob, `tomPitchTCS` the "sweep speed" knob. Both toms
and the bass drum share `pitchSweepRate`, so a per-voice sweep-shape control
would need its own constant rather than reusing the shared one.

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

**Decay:** `SetDecay(amount)` scales `cymBaseDecayS` by `decayScaleMin +
amount` → effective decay ranges **0.6 s – 1.8 s**.
**Velocity:** linear gain via `env = clamp01(velocity)`.
**RNG:** `newVoiceRng(cymSeed)`; `bpFilter.Reset()` on every `Trigger`.

Tune-parameter candidates mirror the Hi-Hat: `cymBPHz`/`cymBPQ` for tone,
`cymBaseDecayS` for wash length — the Hi-Hat and Cymbal could plausibly share
one "metallic voice" tuning UI with different defaults.

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
