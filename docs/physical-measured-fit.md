# Fitting the physical Tom to a recording

This document records the fitting method — what is measured, how it is scored,
and how the search is run — together with what that objective can and cannot
resolve, and the two model errors that survive the measurement.

It does not report a current fit. Every fitted bank in the repository was
measured against `reference/tom.wav`, a recording of unknown provenance deleted
from the working tree on 2026-08-01, and against an objective whose partial terms have since been
shown not to reproduce. Both are covered by
[PLAN.md § P10](../PLAN.md#open-work-p10),
which is the authority for the plan; this page is the method and the evidence
behind it and deliberately does not restate P10 at length.

The evidence behind that second claim — how the objective was scored against
itself, the reproducibility tables, the residual budget and the falsification
tests — lives in
[`physical-objective-validation.md`](physical-objective-validation.md). Read it
before quoting any partial term from any run, past or future.

## The reference

The committed reference is the licensed set `reference/tt08x08/lp/hd/v01..v16.wav`
— an 8" × 8" tom, Remo coated Ambassador batter and clear Diplomat resonant
head, low tuning, head strikes at sixteen velocities, 48 kHz 24-bit stereo,
CC BY 4.0 (Freesound user `quartertone`, pack "Tomtom 08x08inch-multisampled").
`reference/CREDITS.md` is committed and is the authority on licence, attribution,
what was verified and what was not, and the measured properties of the files.

Two properties of it matter for everything below:

- **The stereo pair is coincident.** Peak inter-channel correlation falls at
  0 samples of lag on thirteen of the sixteen and 1 sample on the other three, at
  0.944–0.990 across the set. One sample at 48 kHz is 21 µs, so summing to mono
  is safe — the first comb null sits at 24 kHz — and, more importantly, the two
  channels are two observations of the same acoustic event, which is what makes
  the reproducibility measurement below possible.
- **The instrument is stated.** Shell diameter and depth are known, so `SIZE` and
  `DEPTH` are constants rather than fitted parameters, and the head gauges fix
  both surface densities. Strike position and angle and microphone position and
  angle remain fitted.

`reference/tom.wav` — 44.1 kHz, a **spaced** pair 1.56 ms apart correlating 0.36
at zero lag, no stated instrument, no licence, never committable — was superseded
and then **deleted on 2026-08-01** (P10's N8). It was a timbre-match target and
never an acoustic validation recording; no test depended on it, so nothing in
the suite changed when it went. Every dimension of it quoted in this document is
a measurement recorded while it existed and cannot now be re-checked.

## What is measured

`internal/physical/match` reduces a hit — recorded or rendered — to the same
feature vector, and `Distance` scores two of them. Everything is built on
`algo-dsp` and `algo-fft`; nothing here is a hand-rolled transform. Both sides
are peak-normalized at the onset and every term is **gain invariant**, which is
forced on us: the reference is normalized, so its loudness carries no
information.

| Feature            | How                                                                                                                                                                                                                               | Reused from                              |
| ------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------- |
| Onset              | impulse-start detection                                                                                                                                                                                                           | `measure/ir`                             |
| Partials           | Hann-windowed 64k transform of two windows — the sustain, and an earlier 0.05–0.30 s window — topographic peak prominence, log-domain parabolic interpolation; each window admits candidates relative to its _own_ strongest peak | `dsp/window`, `algo-fft`, `dsp/spectrum` |
| Per-partial decay  | heterodyne to baseband, zero-phase Butterworth low-pass whose cutoff is set from the measured spacing to the nearest neighbour, log-linear fit with R²                                                                            | `dsp/filter/design`, `dsp/filter/biquad` |
| Per-partial level  | the intercept of that same decay fit — the fitted line's value at t = 0, read off the partial's heterodyned envelope                                                                                                              | —                                        |
| Glide              | residual phase slope of the **fundamental** — the lowest partial within 20 dB of the loudest — at 30 ms against the latest probe that partial still supports, refused outright if it supports none                                | as above                                 |
| Spectral shape     | ⅓-octave band levels, mean-removed, in four windows (attack / early / body / tail)                                                                                                                                                | `stats/frequency`                        |
| Amplitude envelope | frame RMS in dB, peak-referred                                                                                                                                                                                                    | `stats/time`                             |
| Attack balance     | 1–8 kHz against 100–500 Hz in the first 20 ms                                                                                                                                                                                     | —                                        |
| Decay metrics      | RT60, EDT, T20, T30, C50, C80 (reported, not fitted)                                                                                                                                                                              | `measure/ir`                             |

### The distance

Nine terms, each in its own perceptual unit so it can be read against a tolerance
rather than only against another run. The weight on each is the reciprocal of that
term's **adoption gate**, so a candidate scoring at its gate contributes exactly
1.0 and the terms are commensurable. The gates and the weights are therefore one
set of numbers, not two — `AdoptionGates()` is `DefaultWeights()` inverted, and
`TestWeightsAreReciprocalGates` keeps them that way.

Since **2026-08-01** the gates are measured rather than asserted. Each is the 90th
percentile of the objective's disagreement with itself over the sixteen velocities
of the coincident reference pair, scored channel-against-channel in both
directions — the floor below which a candidate is indistinguishable from a second
microphone at the same point:

The measurement has been made three times: twice on the medium-pitch set, before
and after the estimator repair of P10/N2, and once on the low-pitch set that
replaced it as the reference on 2026-08-01. The current gates are the last
column.

| Term              | Asserted | `mp/hd` defective | `mp/hd` repaired | `lp/hd` p90 | Gate now |
| ----------------- | -------- | ----------------- | ---------------- | ----------- | -------- |
| Partial frequency | 25 ¢     | 113.0 ¢           | 76.2 ¢           | 65.0 ¢      | 70 ¢     |
| Partial level     | 3 dB     | 17.85 dB          | 6.81 dB          | 6.76 dB     | 7 dB     |
| Partial decay     | 0.25\*   | 1.262             | 0.558            | 0.589       | 0.6      |
| Spectral envelope | 4 dB     | 3.65 dB           | 3.67 dB          | 3.24 dB     | 3.5 dB   |
| Envelope          | 3 dB     | 3.81 dB           | 3.84 dB          | 1.38 dB     | 1.5 dB   |
| Glide             | 40 ¢     | 310.3 ¢           | 280.1 ¢          | **2.3 ¢**   | 10 ¢     |
| Attack balance    | 6 dB     | 1.12 dB           | 1.13 dB          | 0.81 dB     | 0.9 dB   |
| Unmatched share   | 0.5      | 0.880             | 0.250            | 0.280       | 0.3      |
| Spurious share    | 0.5      | 0.346             | 0.245            | 0.293       | 0.3      |

\* the old decay _weight_ was `1/0.35` against a gate documented as 0.25 — the one
place the reciprocal rule was silently broken, so every decay contribution ever
reported was scaled by a threshold nobody had adopted.

What the three columns say, in order:

- **The spectral envelope is the term that was always right.** 3.65 → 3.67 → 3.24
  dB: neither the estimator defect nor the change of drum moved it. Every
  conclusion drawn from that term stands.
- **The estimator repair fixed level, decay and coverage**, which had been
  measuring three collapsed takes and nothing else, and the trimming fixed
  frequency. Neither substitutes for the other; both are P10/N2.
- **The change of drum fixed glide**, by a factor of 120, with the estimator
  untouched. The glide estimator needs the fundamental to survive to its late
  probe; the medium-pitch fundamental does not and the low-pitch one does. The
  standing claim that "glide is broken" was a statement about the target, and is
  withdrawn. What survives is narrower and still worth carrying: the estimator
  fails **silently** when the fundamental dies early, so glide is trustworthy
  only on a reference where it has been checked.
- **"The partial terms were never gateable" is withdrawn.** 65 cents is a wide
  tolerance, not an unusable one. The six rounds of intervention aimed at the old
  25-cent and 0.25 gates were nonetheless aimed at thresholds nothing could
  reach; that much stands.

Read totals against the floor, never alone: on this reference the objective's
disagreement with itself totals **5.92 at the median and 6.68 at p90** under
these weights, and every term's p90 lands at or under its own gate. Two things
moved between the last gate set and this one — the weights and the drum — so no
total recorded before 2026-08-01 is comparable to any recorded after it, in
either direction.

The **glide gate of 10 cents deserves separate warning**. A candidate whose bend
is 30 cents wrong used to contribute 0.10 to the total and now contributes 3.0.
Glide has gone from a term the objective could not see to one of the two or three
most able to dominate a total, and no fit has yet been run under it.

`Spurious` used to be a deliberate departure from the reciprocal rule, because on
`mp/hd` its floor came out just under `Unmatched`'s and an asymmetry in that
direction had already been refuted by a fit run (see below). On `lp/hd` the two
land the other way round and round to the same 0.3, so the equality is now what
the measurement says rather than an override of it.

Partials are identified greedily by closeness in cents, each candidate claimed at
most once, with a tolerance that widens with mode index. Real two-headed drums
scatter ±20 % around the ideal Bessel series in both directions (Richardson,
Toulson & Nunn, _JASA_ 131(1) 2012); insisting the tenth partial land as tightly
as the first would make the series unmatchable rather than the fit precise.

**Deliberately not used:** any sample-aligned waveform comparison.
`analysis.CompareSignals`' NRMSE and correlation stay where they are, for
regression between two renders of the same model. Against a recording of a
different physical drum they measure the phase relationship between two signals
that were never meant to share one — large for candidates that sound identical,
small for candidates that do not.

### The two coverage terms, and why both exist

Three degeneracies were found by measurement, each of which the search reached
within minutes, and the shape of the distance is what closes them.

- **An error averaged over matched pairs is zero when there are no pairs.** A
  candidate producing one partial in the wrong place therefore scored better on
  three of the nine terms than any real drum can. Each partial term is now
  blended against a fixed penalty in proportion to the unmatched share of the
  reference, so a missing partial costs what a present-but-wrong one costs.
  `TestSilenceIsNeverCheaperThanADrum` pins it.
- **The unmatched share must not be weighted by energy.** Energy is dominated by
  whichever partial is loudest, so a candidate reproducing that one partial and
  nothing else reported an unmatched share near zero and the blend did nothing.
  Each partial is now worth **how far it stands, in dB, above the detection
  floor** — zero at the floor, growing with prominence; monotone in loudness like
  energy but compressed. `TestOneLoudPartialIsNotADrum` states the defect
  directly, and `TestAudibilityFloorTracksTheDetectionFloor` fails if the scoring
  floor and `Options.PartialFloorDB` drift apart.
- **Invented partials were free**, which just moves the degenerate optimum from
  too few modes to too many: `matchPartials` iterates the _reference_, so a
  candidate partial with no counterpart reached the total only through the
  spectral envelope. `Spurious` is the mirror image — the share of the
  candidate's partial audibility, under the same weighting, that no reference
  partial claimed.

`Spurious` carries the **same** weight as `Unmatched`, and setting it larger cost
a run: the search abandoned the drum for two partials, having found that
inventing nothing is easiest when there is nothing. The blend already supplies
the asymmetry toward completeness; supplying it a second time through the weight
inverts the degeneracy instead of closing it.
`TestSpuriousDoesNotOutweighCompleteness` pins the inequality — and says
explicitly that it cannot pin the behaviour, because the failure lives in the
composition of the model with the metric.

`Spurious` is counted **only between the lowest and highest reference partial**.
Outside that span the reference's own detection is unproven, so a partial out
there is charged by the spectral envelope, on evidence, and not by this.

## What the objective can and cannot measure

This is the strongest methodological result on the path, and it invalidates most
of what was previously concluded from these terms.

The licensed pair is coincident: two microphones at one point capturing one
event. Reducing each channel separately through the same feature extraction and
scoring one against the other therefore measures the **objective's disagreement
with itself**, with no room path available to explain it. That was done across all
sixteen velocities, and the result is that **the partial-frequency,
partial-decay, partial-level and glide terms are not reproducible measurements** —
their self-disagreement exceeded the adoption gates they were scored against, so
the 25 ¢ and 0.25 gates were never achievable by any model. The spectral envelope
is the only one of the nine terms that reproduces, and its self-disagreement is the
floor any envelope figure must be read against.

That distribution is now what the gates are set from; see the table above.

The method, the exact commands and the full tables for both references are in
[`physical-objective-validation.md`](physical-objective-validation.md). Read them
before quoting any partial term from any run. Rounds of intervention concluded
"the model is the ceiling" from evidence that cannot support it; those conclusions
are withdrawn rather than adjusted.

Part of this is estimator resolution rather than the objective's shape:
`MinSeparationHz` structurally cannot resolve the split mode pairs that
`TensionAsymmetry` exists to model, and merged pairs beat, which corrupts the
log-linear decay fit while keeping R² high. Both are measured in
[`physical-objective-validation.md`](physical-objective-validation.md) and are
P10's N2.

## The two genuine model errors

Only two of the nine terms carry model error that survives the measurement above:
the **spectral envelope**, at 11.07 dB against a floor around 3.2–3.4 dB, and the
**spurious share**, at 0.327 against a 0.121 floor. Both were measured against the
retired recording, so the totals around them are gone; these two are quoted
because the envelope term reproduces and because the defect behind them is
identified.

**They are the same defect seen twice.** The model synthesizes a mode at 186 Hz
with a T60 of **1.81 s** — the longest-ringing thing it produces — that the
recording does not contain. It is simultaneously the top contributor to the
envelope term, most of the spurious share, and part of the frequency error.

**It is a damping-distribution defect, not a spectral-shape one.** Decomposing the
envelope term by window gives attack 6.46 / early 9.99 / body 14.36 / tail
13.47 dB: flat at onset, diverging monotonically with time. The residual budgets
into a measurement floor, a large time-varying part that is the actual target, and
a small static-shape part — three independent routes agree, and the numbers are in
[`physical-objective-validation.md`](physical-objective-validation.md#result-2--the-residual-budgeted).

The missing freedom is **structured, not free-per-mode**. No smooth loss law can
reach the decay gate: fitting a smooth power law to the reference's own T60s
leaves **0.677**, and the model already achieves **0.573**. What a smooth γ(k)
cannot express is in-phase/out-of-phase head-pair splitting, which the model has
only for m = 0. P10's N3 is where that is addressed, and it says to check the sign
pattern before fitting any per-mode damping vector: alternation is physics,
structurelessness is fitted noise.

## Recorded negative results

Three fixes for the envelope term were proposed, tested and refuted on
2026-08-01. They are named here so the work is not repeated; the measurements are
in
[`physical-objective-validation.md`](physical-objective-validation.md#result-3--falsification-tests-all-negative).

- **The envelope term is _not_ a band-coverage artifact.** This document
  previously suggested the term might be ill-posed because it runs to 12.5 kHz
  where the model has only the stochastic attack layer, and proposed band-limiting
  it. Band-limiting to 50 Hz–2 kHz, where the model has full modal content, buys
  about 2 dB of the 11. The hypothesis is real and far too small to rescue the
  term, and the suggestion is withdrawn.
- **A fitted static body/radiation post-filter cannot fix it.** The published
  design (Bank, ICMC 2007) is sound and the topology physically correct, but the
  falsification test fails: fitting a static `g(f)` across all four windows against
  a criterion of 3–4 dB at five parameters falls well short, and even a fully free
  per-band EQ — the absolute limit of any static filter — does not reach it.
- **Exterior air loading is not the missing mechanism.** The licensed 8" × 8" tom
  has known geometry, so the ideal model predicts every mode from the fundamental
  with no free parameter, and the measured (1,1)/(0,1) ratio matches that
  prediction to about eleven cents while sitting roughly 950 cents from the
  air-loaded one. The ratio the old recording showed is that drum's two-head
  tuning, a mechanism the model already represents.

## The search

`cmd/fit-physical` minimizes the distance with the Mayfly Optimization Algorithm
([`github.com/cwbudde/mayfly`](https://github.com/cwbudde/mayfly)), a
build-time-only dependency imported by that command and nothing else — the
shipped WASM binary is unchanged.

The search space is **exactly the bank the product exposes**:
`drum.PhysicalTomSpecs()`, seventeen free normalized parameters (QUAL is pinned —
it buys mode count with CPU, which is a product decision, not a property of the
drum), plus strike velocity as an eighteenth dimension. All bounded to [0, 1],
which is also the only shape mayfly's scalar bounds can express. Anything the
search finds can therefore be typed into the app or shared as a link; a fit that
needed a hidden parameter would not be a preset.

Candidates are rendered at the **reference's own sample rate**, so no resampler
enters the measurement path on either side. The mapping from knob positions to SI
configuration is `drum.PhysicalTomConfig`, the same function the voice uses —
extracted and exported for this, because the constant-ζ retune rule, the
DAMP/DEC/D.TILT composition and the resonant head's reduced asymmetry are
calibration decisions with their own evidence, and a fitter that reimplemented
them would be measuring a different instrument than the one that ships.

Concurrency is between restarts rather than inside one, since mayfly calls the
objective sequentially. Multi-start suits this landscape anyway: every knob's
mapping has a detent at its default, so a single swarm can settle into one.

A reference with more than one channel and no `-channel` on the command line is
**refused before the search starts**, naming the file, its channel count and the
three choices. A full budget was once spent on a target nobody intended, leaving
its only trace in a `"channel"` field of a report nobody read. Only an absent flag
is refused; `-channel mono` typed out is a decision and is honoured — and on the
coincident licensed pair it is the right one, where on the retired spaced pair
it notched the spectrum.

`-progress N` reports every N objective evaluations from inside the objective
itself — the one place every restart passes through, since mayfly offers no
per-iteration hook — with the running best, elapsed time and a projected finish.
The first full run printed nothing at all for forty minutes without it, which is
indistinguishable from a wedged process.

`BenchmarkCost` and its siblings break one evaluation down, because the search's
whole cost is that one number: `DoubleHead.Render` ~69 %, `match.Extract` ~35 %,
`physical.NewDoubleHead` ~5 % of the render, `drum.PhysicalTomConfig` ~2 µs. A
CPU profile puts 56 % of the render in `solveMidpoint` and 18 % in `observe`,
both per-sample and both inherent to the model. Worth recording for its own sake:
1.2 s of audio takes ~0.5 s to synthesize at Draft quality, so one physical Tom
costs roughly 0.4× a core in real time — a product number, not just a fitting one.

### Two defects found in mayfly

Both are fixed upstream and released as
[v0.2.0](https://github.com/CWBudde/mayfly/releases/tag/v0.2.0); `go.mod` pins
[v0.2.1](https://github.com/CWBudde/mayfly/releases/tag/v0.2.1), which adds only
CI, lint and documentation fixes on top.

- **`Result.BestSolution` was not a solution.** It holds the best cost after each
  iteration — a convergence curve. The name was the whole defect: the field is a
  `[]float64` exactly like a position vector, so reading it as one compiles, runs
  and yields nonsense. Here it panicked on an index past the end of an 18-element
  bank, which was luck; a search space wider than the iteration count would have
  silently fitted a drum to noise. It is now `ConvergenceCurve`, a deliberately
  breaking rename.
- **`NC` was unvalidated.** It drives three index expressions that do not
  bounds-check, and the shipped default of `NC = 20` panicked for every `-pop`
  under 10 — precisely what someone shrinking the swarm to go faster would write.
  `Optimize` now rejects all three cases with an error naming the constraint. Each
  restart still runs under a `recover`, no longer for a known trigger but because
  the objective runs third-party numerics on adversarially-chosen parameters.

## Corrections to the estimators

Three defects were found by probing the reference rather than by reading the
code. All are fixed and all are in force; they are recorded because the way they
were found generalizes.

### A correction to the partial measurement (2026-07-31)

**The partial level estimator was ill-conditioned.** `measureDecays` computed a
partial's strike level as its magnitude in the sustain transform divided by the
attenuation that window applies to a partial decaying at the fitted rate. That is
exact for an isolated exponential and unusable as an estimator: for the default
sustain window the divisor is enormous and depends violently on the fitted decay,
running from 12.4 dB at a T60 of 2.0 s to 122 dB at 0.073 s. The reported level
was largely a restatement of the fitted decay rate. Capping the correction was
considered and rejected with a measurement — the loudest partial of the old
reference itself carried an 83.7 dB correction, so any cap tight enough to
exclude the runaways excluded it too.

The fix is to take the level from the decay fit's own **intercept**: the same
quantity measured directly, extrapolating back only as far as
`DecayFitStartSeconds` rather than through the whole Hann taper, and by least
squares over hundreds of samples rather than as a ratio of two numbers. The level
must **not** be the envelope's peak inside the fit window — a strike transient
smeared through a 150 ms filter once put a partial 32 dB too loud.

**Detection was blind to short-ringing partials.** The sustain transform spans
800 ms, so a partial ringing a tenth of that stands roughly 90 dB lower in it, and
both detection guards rank on that uncorrected magnitude and bind _together_ —
loosening either alone changed nothing. `detectPartials` now also reads an earlier
window (`EarlyDetectionStartSecs` 0.05 → `EarlyDetectionEndSecs` 0.30), and **each
window admits candidates relative to its own strongest peak**, so a short partial
competes against short-lived content rather than against the fundamental's whole
ring. The sustain window is admitted first, so where both see a partial the
better-resolved frequency is kept. The early window starts after the strike
transient, because a broadband click would otherwise offer a peak at every
frequency.

**The ordering mattered.** The tight guards were, accidentally, what kept the
unstable estimator from firing: they excluded precisely the short-ringing
population where the correction ran away. Fixing detection first would have made
the measurement worse. Bound the estimator, then open the aperture.
`TestShortPartialsDoNotOutrankLongOnes` pins both on a synthetic two-partial
signal and depends on no recording.

The general lesson is worth as much as the fix: **the conditioning of a
measurement is a result in its own right.** An exact formula can still be an
unusable estimator, and the way to find out is to probe it — sweep the parameter,
look at what the correction factor does across the range it is asked to span, and
check whether the answer is a measurement or a restatement of one of its inputs.

### A correction to the glide term (2026-08-01)

The glide is the one observable `NLIN` moves and nothing else does, so it is the
term that decides the nonlinearity's calibration. It was being read off the wrong
partial, at a time by which that partial no longer existed, through a filter wide
enough to admit its neighbour.

**It tracked the loudest partial, not the fundamental.** `Extract` took the bend
from whichever partial peaked highest, which on a tom can be a mode that is in the
noise floor long before the late probe fires — so what was read was the noise the
loud mode decayed into. Neither obvious alternative is right either: the lowest
partial outright may be a shell resonance or a room mode 40 dB down.
`glidePartial` now takes the lowest partial standing within
`GlidePartialWindowDB` (20 dB) of the loudest.

**The late probe was nailed to a fixed time.** An instantaneous-frequency reading
is a reading only while the tracked partial still dominates its own passband; once
it is gone, the phase slope reports the neighbours' offset from the carrier. That
was the normal case, and it hit candidates as well as the reference: as cavity
coupling separates the doublet the reported "glide" tracked the neighbour, making
the term a function of the cavity rather than of `NLIN`. `GlideLateSeconds` is
therefore no longer where the late probe sits but the _latest_ it may sit. The
probe is walked back to the last point at which the partial is still within
`GlideFloorDB` (20 dB) of its early level, and the reading is **refused** if that
point is not at least `GlideMinSpanSeconds` (50 ms) after the early probe. Both
probes must also land inside the filter's own passband. A short honest span beats
a long dishonest one: the bend is an exponential settling with a time constant of
tens of milliseconds.

**Refusal has to be scoreable**, and `Features.GlideMeasured` is what makes it so.
A reference with no reading **zeroes** the term — a fabricated zero would silently
assert that the reference does not bend. A candidate with no reading against a
reference that has one pays `unreadableGlideCents` = 40, exactly one "clearly
wrong" glide: nonzero, so that an unmeasurable candidate cannot outscore one that
is merely wrong; and no larger, because a fundamental that dies early is already
charged by the decay and envelope terms.

**The probe filter admitted the neighbour.** `glideCutoffHz` sets the baseband
width from the measured spacing to the nearest partial — wide enough to follow a
bend of a semitone, narrow enough to exclude the neighbour, which a coupled pair
tens of Hz apart otherwise fails, reading their beat as a bend.

## The fits against the retired recording

Historical note only. Four full-budget fits and several sweeps were run against
`reference/tom.wav` under three successive objectives. Their totals are not
comparable to each other, are not comparable to anything measured against the
licensed reference, and are not quoted here. The two figures worth carrying
forward are in [the two genuine model errors](#the-two-genuine-model-errors)
above; everything else about those runs is superseded either by the change of
reference or by the reproducibility measurement.

Two method results from that era do survive, because they are about the search
rather than about the recording:

- **Search effort is not the ceiling.** Two independent runs — different restart
  counts, different seeds, one complete and one interrupted — converged to the
  same point and agreed term by term on everything the objective can see, with
  the best restart's last three iterations flat to four digits.
- **`-seeded-restarts` stays off by default.** The winning restart was unseeded in
  three consecutive paired comparisons. The mechanism is measurable in advance:
  the analytic pre-solve reports its own frequency error before any rendering
  happens, and a box around a seed is worth its cost only in proportion to how
  good the seed is. Seeding won when the pre-solve reached ~1 ¢ and lost once the
  same pre-solve floored in the twenties and thirties.

`testdata/physical-fit-tom.json` and the **Measured tom** preset in
`web/src/algo/physicalTomPresets.ts` both derive from the last of those runs.
They record what was found; `DefaultPhysicalDrum()` and `DefaultContact()` are
**unchanged**, as they have been through every round, because a candidate that
misses every gated term does not earn a claim about what the model should sound
like out of the box. Both are due for re-derivation against the licensed
reference (P10's N5 and N8).

## Reproducing

The run is deterministic given the seed; `TestSearchIsDeterministic` keeps it that
way. It is not part of `just ci`: it takes minutes, and the licensed reference is
committed but the analysis is not cheap.

```bash
just fit-physical <reference.wav>                   # passes a checkpoint for you
go run ./cmd/fit-physical -reference <ref> -channel mono -report-only
```

### Listening to what it found

`-wav <path>` renders the fitted bank alongside the JSON report, at
`-wav-duration` (3 s by default) rather than the 1.2 s the search itself renders
— long enough to hear a tail, where the fitting duration is chosen to be just long
enough to measure one. It renders from the candidate's own recorded config, so
what lands on disk is the drum the report describes even when the report was
resumed from a checkpoint. The export is peak-normalized, like the reference, so
it is for listening and not for judging level; the true peak is printed.

This matters because **the distance is a proxy and the recording is the target**.
Every number here is a summary of nine terms chosen by hand, four of which do not
reproduce, and a fit that wins on them can still be wrong in a way the terms do
not represent. A/B against the reference before believing any total.

### Re-scoring an archived report

An archived report's stored total was produced by an objective that may no longer
exist, but the report records its own best point exactly, so the bank can be
re-measured by pinning it and asking for a report — one `-fix` per free parameter,
taking the `normalized` value from `best.params` (not the SI `value`, since `-fix`
is expressed in knob positions), with parameters marked `"fixed": true` left
alone.

Four things must line up or the answer is about a different drum:

- **`-velocity <best.velocity01>`.** Strike velocity is the eighteenth search
  dimension but is not a bank parameter, so `-fix` cannot reach it and
  `-report-only` would otherwise measure every re-scored bank at VEL = 1.
  Velocity moves the level, envelope and attack-balance terms together, and the
  distortion is large enough to change the ranking.
- **`-quality` and `-contact` as the run used them**, both recorded in the
  report's `search` block. A Standard-tier bank re-scored at Draft is a different
  instrument.
- **`-channel` as the run used it**, from `reference.channel`.
- **Any flag the search itself was run under.** `-loss-scale`, `-mallet-g` and
  `-mode-correction`
  describe a drum the product cannot be set to; dropping them re-measures the bank
  on an instrument the run was never fitted to, so such runs have no comparable
  total and must not be tabulated with ordinary ones.

A re-score renders the archived bank through today's synthesis as well as
measuring it with today's metric, so the answer is "what this parameter set is
worth today" and not "what one correction cost that run", which cannot be
recovered. And a re-scored ranking is not a re-fitted ranking: each bank was
optimized against a different objective and is only being marked under this one.

### Stopping one, and picking it up again

A fit takes the better part of an hour. `-checkpoint FILE` makes that cheap, and
`just fit-physical` passes one by default.

A true continuation is not available: mayfly runs its whole loop inside
`Optimize`, offers no per-iteration hook and cannot be handed a starting
population, so a stopped swarm's velocities and personal bests are gone for good.
Two things are saved instead.

**Finished restarts.** Multi-start is the outer loop, so a restart that finished
is finished; a resumed run skips it and replays its position through this build's
measurement code rather than trusting a stored number. A restart an interrupt cut
short is recorded but marked incomplete, and re-run.

**The best point, continuously.** Every restart runs concurrently, so they all
finish at roughly the same moment — interrupt a run half way and typically _none_
is complete, so restart-level checkpointing alone would save nothing. The best
position any restart has reached is therefore recorded from inside the objective
every 250 evaluations, which bounds what a stop can destroy to a quarter of a
percent of the run.

`SIGINT`/`SIGTERM` asks the search to wind up rather than killing the process: the
objective starts returning `+Inf` without rendering, so mayfly keeps the incumbent
it had and every restart still reports the best it genuinely found. A stopped run
writes a normal report marked `"interrupted": true` — which also qualifies its
evaluation count, since the tail of that count is refusals rather than measured
candidates. A second signal kills it the usual way.

Resuming onto a checkpoint from a different run is refused, naming the field that
changed. The guard that matters is not the flags but **`baselineCost`** — the
shipped bank's distance from the reference, measured end to end through the same
synthesis and feature extraction the search uses, and computed on every run
anyway. Any edit that moves a rendered sample or a measured feature moves it too,
and that is exactly the case a resume must refuse: a best-of taken across two
different models is not a fit, and nothing downstream would reveal the mix. A
performance change that really is bit-exact leaves it untouched and resumes
cleanly, so the guard doubles as a test of that claim.
