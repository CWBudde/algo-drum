# Fitting the physical Tom to a recording

This document records the measurement, the search, what the first fit found, the
refits that gave each contact model the same budget over the same bank, and the
correction to the measurement itself that superseded everything before it.

It does **not** close P6's _"fit documented presets from measurement"_ item, and
it does not meet P8's exit criterion. It builds the machinery both of those need
and reports honestly how far one recording gets: on the corrected measurement all
three gate terms are missed, and the shipped default is left alone as a result.
The gap it leaves is specific enough to be the next piece of work.

The current result is
[the corrected head-to-head](#the-head-to-head-re-run-on-the-corrected-measurement-2026-07-31);
every fit number recorded before
[the measurement correction](#a-correction-to-the-partial-measurement-2026-07-31)
carries a superseded banner and should not be quoted.

## Provenance, and its limits

The target is a tom recording of **unknown provenance** — a sample-library
file, with no record of the drum's dimensions, head, tuning, mallet,
microphone, geometry, gain chain or room, and no license the repository could
redistribute under.

So, plainly: **this is a timbre-match target, not an acoustic validation
recording.** P6 gates a measured calibration on recorded provenance, and none of
it is available here. Nothing in the test suite depends on the file,
`reference/` is gitignored, and no claim in this document should be read as
"the model was validated against a real drum". What it says is narrower and
still worth having: given one recorded tom, here is how close the shipped
parameter range can get to it, and where it runs out of room.

Measured properties of the file itself, which is all the provenance there is:

|                     |                                           |
| ------------------- | ----------------------------------------- |
| Format              | 44.1 kHz, 16-bit stereo, 4.49 s           |
| Level               | peak normalized (1.000 left, 0.762 right) |
| Channel correlation | 0.36 — there is a room on it              |
| Onset               | sample 0; the file is trimmed to the hit  |

## What is measured

`internal/physical/match` reduces a hit — recorded or rendered — to the same
feature vector, and `Distance` scores two of them. Everything is built on
`algo-dsp` and `algo-fft`; nothing here is a hand-rolled transform.

Both sides are peak-normalized at the onset and every term is **gain
invariant**, which is forced on us: the reference is normalized, so its
loudness carries no information at all.

| Feature            | How                                                                                                                                                                                                                               | Reused from                              |
| ------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------- |
| Onset              | impulse-start detection                                                                                                                                                                                                           | `measure/ir`                             |
| Partials           | Hann-windowed 64k transform of two windows — the sustain, and an earlier 0.05–0.30 s window — topographic peak prominence, log-domain parabolic interpolation; each window admits candidates relative to its _own_ strongest peak | `dsp/window`, `algo-fft`, `dsp/spectrum` |
| Per-partial decay  | heterodyne to baseband, zero-phase Butterworth low-pass whose cutoff is set from the measured spacing to the nearest neighbour, log-linear fit with R²                                                                            | `dsp/filter/design`, `dsp/filter/biquad` |
| Per-partial level  | the intercept of that same decay fit — the fitted line's value at t = 0, read off the partial's heterodyned envelope                                                                                                              | —                                        |
| Glide              | residual phase slope of the loudest partial at 30 ms against 400 ms                                                                                                                                                               | as above                                 |
| Spectral shape     | ⅓-octave band levels, mean-removed, in four windows (attack / early / body / tail)                                                                                                                                                | `stats/frequency`                        |
| Amplitude envelope | frame RMS in dB, peak-referred                                                                                                                                                                                                    | `stats/time`                             |
| Attack balance     | 1–8 kHz against 100–500 Hz in the first 20 ms                                                                                                                                                                                     | —                                        |
| Decay metrics      | RT60, EDT, T20, T30, C50, C80 (reported, not fitted)                                                                                                                                                                              | `measure/ir`                             |

### The distance

Nine terms, each in its own perceptual unit so it can be read against a
tolerance rather than only against another run. The weight on each is the
reciprocal of that term's "clearly wrong" threshold — 25 cents of pitch, 3 dB
of partial balance, a factor of 1.4 in ring time, 4 dB of spectral shape, 3 dB
of envelope, 40 cents of glide, 6 dB of attack balance, and the two partial-
coverage shares below — so a just-audible
error anywhere contributes about the same amount, and the sum means something.

Partials are identified greedily by closeness in cents, each candidate claimed
at most once, with a tolerance that widens with mode index. Real two-headed
drums scatter ±20 % around the ideal Bessel series in both directions
(Richardson, Toulson & Nunn, _JASA_ 131(1) 2012); insisting the tenth partial
land as tightly as the first would make the series unmatchable rather than the
fit precise.

**Deliberately not used:** any sample-aligned waveform comparison.
`analysis.CompareSignals`' NRMSE and correlation stay where they are, for
regression between two renders of the same model. Against a room recording of a
different physical drum they measure the phase relationship between two signals
that were never meant to share one — large for candidates that sound identical,
small for candidates that do not.

### Four measurement problems worth recording

All were found by measurement, not by inspection, and all changed the design.
The last two were found only after listening to a fitted bank that the distance
called good and that plainly was not — the metric ranked candidates correctly
while being wrong about all of them in absolute terms.

**A phantom partial, and why the floor is not the fix.** The reference has a
genuine, isolated component at 87 Hz — 22 dB of topographic prominence, so not
a shoulder — that is 39.5 dB below the fundamental. A bare local-maximum test
also accepted ripples on the fundamental's skirt, which prominence now rejects.
But the 87 Hz peak is real, and no model of a two-headed drum will produce it.
A level floor tight enough to exclude it also discards the 500–700 Hz cluster
that carries the drum's character. The fix is instead in the distance: the
`Unmatched` term is weighted by how loud each partial is, not by count, so
failing to reproduce a partial costs roughly what that partial was worth. It was
first weighted by **energy**; see below for why that was too strong a form of
the same idea.

**A degenerate optimum, and the blend that closes it.** An error averaged over
matched pairs is zero when there are no pairs. A candidate that produces one
partial in the wrong place therefore scored _better_ on three of the nine
terms than any real drum can — and the search found it immediately, reaching
11.2 against the shipped default's 39.2 from a render that sounded like
nothing. Each partial term is now blended against a fixed penalty in proportion
to the unmatched share of the reference, so a partial that is missing costs what a
partial that is present but wrong costs.
`TestSilenceIsNeverCheaperThanADrum` pins it.

**Energy weighting reopened the same degenerate optimum.** The blend above only
works if the unmatched share is an honest measure of what is missing, and
weighted by energy it is not. Energy is dominated by whichever partial is
loudest, and on this reference the 212.78 Hz partial carries **99.4 %** of it.
A candidate reproducing that one partial and nothing else therefore reported an
unmatched share of 0.006 — so the blend did nothing, and the three partial
terms averaged over the single pair that matched and returned excellent numbers
for a drum with one mode in it. Six of the nine terms were scoring one partial.
Repricing the two stopped fit runs under the corrected weighting moved them from
4.517 and 7.827 to **13.143** and **15.102**, and their partial frequency error
from 9.7 ¢ to 88.1 ¢.

Plain counting would overcorrect, because of the 87 Hz phantom above: missing a
component 39 dB down must not cost what missing the fundamental costs. Each
partial is now worth **how far it stands, in dB, above the detection floor** —
zero at the floor, growing with prominence. Monotone in loudness like energy,
but compressed enough that six missing quiet partials cost 54 % rather than
0.6 %. `TestOneLoudPartialIsNotADrum` states the defect directly, and
`TestAudibilityFloorTracksTheDetectionFloor` fails if the scoring floor and
`Options.PartialFloorDB` ever drift apart.

**And the mirror of it: invented partials were free.** Making missing partials
expensive without charging for invented ones just moves the degenerate optimum
from too few modes to too many. `matchPartials` iterates the _reference_, so a
candidate partial with no reference counterpart is invisible to every partial
term and reaches the total only through the spectral envelope. The first run
under the corrected weighting demonstrated it within six minutes: the candidate
covered all seven reference partials, reported an unmatched share of **0.000**,
and made its second-loudest component an invented 182 Hz mode 15 dB down.

The `Spurious` term is the mirror image — the share of the candidate's partial
audibility, under the same dB-above-floor weighting, that no reference partial
claimed.

**It carries the same weight as `Unmatched`, and getting that wrong cost a run.**
It was first set larger, at 1/0.2, on the reasoning that nothing else in the sum
absorbs an invented partial while a missing one is charged twice — once directly
and once through the blend. A fit refuted that within fourteen minutes: it
abandoned the drum for **two partials** and a spurious share of 0.000, having
found that inventing nothing is easiest when there is nothing. Measured on that
run's own candidates, the dense and sparse extremes came out within 0.12 of each
other, so the search simply drifted between them.

The two pressures are the same quantity seen from opposite sides. The blend
already supplies the asymmetry toward completeness that the argument above wanted;
supplying it a second time through the weight inverts the degeneracy instead of
closing it. `TestSpuriousDoesNotOutweighCompleteness` pins the inequality — and
says explicitly that it cannot pin the behaviour, because the failure lives in the
composition of the model with the metric. Dropping a partial from a synthetic
candidate leaves its invented partials in place, so the total rises monotonically
at either weight; in the search, coverage and invention move together, since both
are consequences of the same tuning.

It is counted **only between the lowest and highest reference partial**. Above
and below that span the reference's own detection is unproven — a room
recording's noise floor hides modes a model legitimately has — so a partial out
there is charged by the spectral envelope, on evidence, and not by this. Without
that bound the term would fit the recording's limitations rather than the drum.
It is also not blended into the partial terms: those pairs really did match, and
an invented mode elsewhere does not make a matched one less matched.

## The search

`cmd/fit-physical` minimizes the distance with the Mayfly Optimization
Algorithm ([`github.com/cwbudde/mayfly`](https://github.com/cwbudde/mayfly)), a
build-time-only dependency imported by that command and nothing else — the
shipped WASM binary is unchanged.

The search space is **exactly the bank the product exposes**:
`drum.PhysicalTomSpecs()`, seventeen free normalized parameters (QUAL is pinned
— it buys mode count with CPU, which is a product decision, not a property of
this drum), plus strike velocity as an eighteenth dimension. All bounded to
[0, 1], which is also the only shape mayfly's scalar bounds can express.
Anything the search finds can therefore be typed into the app or shared as a
link; a fit that needed a hidden parameter would not be a preset.

Candidates are rendered at the **reference's own sample rate**, so no resampler
enters the measurement path on either side. The mapping from knob positions to
SI configuration is `drum.PhysicalTomConfig`, the same function the voice uses
— extracted and exported for this, because the constant-ζ retune rule, the
DAMP/DEC/D.TILT composition and the resonant head's reduced asymmetry are
calibration decisions with their own evidence, and a fitter that reimplemented
them would be measuring a different instrument than the one that ships.

Concurrency is between restarts rather than inside one, since mayfly calls the
objective sequentially. Multi-start suits this landscape anyway: every knob's
mapping has a detent at its default, so a single swarm can settle into one.

```bash
just fit-physical                       # the default reference and settings
go run ./cmd/fit-physical -reference reference/tom.wav -report-only
go run ./cmd/fit-physical -reference reference/tom.wav \
    -restarts 11 -iterations 150 -pop 20 -o fit-report.json
```

Driving mayfly from outside its own examples surfaced two defects. Both are
fixed upstream and released as
[v0.2.0](https://github.com/CWBudde/mayfly/releases/tag/v0.2.0); `go.mod` pins
[v0.2.1](https://github.com/CWBudde/mayfly/releases/tag/v0.2.1), which adds only
CI, lint and documentation fixes on top.

- **`Result.BestSolution` is not a solution.** It holds the best cost after each
  iteration — a convergence curve of `MaxIterations` entries. The name is the
  whole defect: the field is a `[]float64` exactly like a position vector, so
  reading it as one compiles, runs, and yields nonsense. Here it panicked on an
  index past the end of an 18-element bank, which was luck; a search space wider
  than the iteration count would have silently fitted a drum to noise. The
  position is `Result.GlobalBest.Position`. It is now `ConvergenceCurve` — a
  deliberately breaking rename rather than an alias, since keeping the old
  spelling would have left the trap in place.
- **`NC` was unvalidated.** It drives three index expressions that do not
  bounds-check. Mating reads `males[k]` and `females[k]` for `k < NC/2`, so any
  population below half the offspring count panics inside the library — and the
  shipped default of `NC = 20` does that for every `-pop` under 10, which is
  precisely what someone shrinking the swarm to go faster would write. Mutants
  are then drawn from the offspring with `rng.Intn`, which panics on the empty
  slice `NC < 2` leaves behind. `Optimize` now rejects all three cases with an
  error naming the constraint. `NC` is still set from the population here, since
  a returned error is not what this caller wants either, and each restart still
  runs under a `recover` — no longer for a known trigger, but because the
  objective runs third-party numerics on adversarially-chosen parameters and one
  restart dying should not take the other seven with it.

### Watching a search that takes an hour

The first full run printed its reference and baseline lines and then nothing at
all for forty minutes, because the only progress report was per finished
restart. That is indistinguishable from a wedged process, and it hid the fact
that the run was over-provisioned by a factor of eight.

`-progress N` now reports every N objective evaluations from inside the
objective itself — the one place every restart passes through, since mayfly
offers no per-iteration hook — with the running best, the elapsed time and a
projected finish. It is what turned "this might take all day" into a measured
23 minutes, and it is why the iteration count below is a considered number
rather than a guess.

`BenchmarkCost` and its siblings in `cmd/fit-physical` break one evaluation
down, because the search's whole cost is that one number:

| Stage                    | Share of one evaluation |
| ------------------------ | ----------------------- |
| `DoubleHead.Render`      | ~69%                    |
| `match.Extract`          | ~35%                    |
| `physical.NewDoubleHead` | ~5% (of the render)     |
| `drum.PhysicalTomConfig` | ~2 µs, i.e. nothing     |

Rendering dominates, and a CPU profile puts 56% of it in `solveMidpoint` and
18% in `observe` — both per-sample, both inherent to the model rather than a
defect. Worth recording for its own sake: 1.2 s of audio takes ~0.5 s to
synthesize at **Draft** quality, so one physical Tom costs roughly 0.4× a core
in real time. That is a product number, not just a fitting one, and `observe`
accumulates several diagnostic energies per mode per sample that the audio path
never reads. Neither is touched here — changing the synthesis hot path is not
something to do in the same breath as fitting it.

## Results

Eight restarts of 80 iterations at population 16, seed 1, Draft quality,
31 616 evaluations, 41 minutes on twelve cores. The fitted bank is in
`testdata/physical-fit-tom.json` and is selectable in the app as **Measured
tom**.

> **Superseded.** Every number in this table and the next was measured through
> the combed mono downmix and under the energy-weighted `Unmatched` term, both
> since corrected. They are kept because the reasoning around them is still
> instructive and because the corrections were found by asking why these numbers
> disagreed with listening — but they are not comparable to anything measured
> after, and none of them should be quoted as a result. The current fit is
> [the corrected head-to-head](#the-head-to-head-re-run-on-the-corrected-measurement-2026-07-31).

| Term                  | Shipped default | Fitted    | Adoption gate |
| --------------------- | --------------- | --------- | ------------- |
| Partial frequency     | 119.4 ¢         | 21.5 ¢    | ≤ 25 ¢ ✅     |
| Partial decay         | 1.106           | 0.179     | ≤ 0.25 ✅     |
| **Spectral envelope** | 20.6 dB         | 13.6 dB   | ≤ 4 dB ❌     |
| Partial level         | 12.4 dB         | 2.0 dB    | —             |
| Amplitude envelope    | 37.3 dB         | 1.1 dB    | —             |
| Glide                 | 60.9 ¢          | 18.2 ¢    | —             |
| Attack balance        | 1.9 dB          | 0.3 dB    | —             |
| Unmatched energy      | 0.980           | 0.027     | —             |
| **Total**             | **33.455**      | **6.364** | —             |

**Nothing is pinned at a bound.** That refutes the prediction this work started
from. The reference's fundamental rings 1.53 s against the shipped model's
0.21 s, and since DAMP only reaches 0.25× the expectation was that it would
bottom out and the gap would survive as a finding about the
`ModeDecayCorrections` anchor. Instead the fit reaches **1.52 s** with DAMP at
1.26 — _more_ damping than default — by dropping batter tension from 1250 to
334 N/m. `RetuneTension` holds ζ constant, so lower tension lowers ω and
lengthens T60 at unchanged damping. The ring time was never out of range; it was
coupled to a parameter nobody had thought to move. The range is fine, and the
earlier reasoning was wrong.

### What it does not reach, and why that blocks adoption

The gate says a fit is worth adopting only inside ~25 ¢, ~0.25 log-ratio and
~4 dB. Two of three pass comfortably. The spectral envelope misses by more than
three times its threshold, and the partial lists say why:

|                | Reference | Fitted |
| -------------- | --------- | ------ |
| Partials found | 16        | 5      |
| 476–700 Hz     | 9         | 0      |

> **Re-scoped 2026-07-31.** The reference column here was measured with the
> superseded partial-level estimator and the single-window detector; see
> [the correction below](#a-correction-to-the-partial-measurement-2026-07-31).
> The reference is still busiest in this band — the corrected reduction of the
> right channel puts seven of its fourteen partials between 476 and 700 Hz — but
> the size of the deficit measured here is not a number that survives, and part
> of the gap was the target not asking for those partials either. Whether a real
> excitation gap remains in this band is an **open question** on the corrected
> metric, not a settled finding.

The recording's character lives in a dense cluster of nine partials between 476
and 700 Hz. The fitted drum has none there. It matches the fundamental
(118.06 → 118.68 Hz, T60 1.53 → 1.52 s), the envelope and the glide almost
exactly, and is empty where the reference is busiest.

The obvious suspect was the Draft mode budget, since QUAL is pinned during the
search. Re-measuring the fitted bank at every tier rules it out:

| Quality  | Modes | Spectral envelope |
| -------- | ----- | ----------------- |
| Draft    | 48    | 13.3 dB           |
| Standard | 96    | 13.1 dB           |
| High     | 160   | 13.1 dB           |

Two hundred cents of extra mode count buys 0.2 dB. The sparsity is structural,
not a budget: within the range the product exposes, this model does not put nine
resolvable partials in that band. Whether that is the two-head mode series
itself, the cavity split, or the absence of shell and lug modes is the next
question, and it is a **P8 finding rather than a fitting failure**.

> **Answered 2026-07-30**, and it is none of those three. The modes are there —
> 58 of them lie in the band at High quality — and the force that should excite
> them is not: a smooth half-sine over the measured 5.5–8 ms contact is 34 dB
> down at 635 Hz, which is the size of the measured deficit. Microphone
> geometry, strike footprint, cavity coupling, tension asymmetry and mode count
> were each measured and eliminated. See
> [`physical-excitation-gap.md`](physical-excitation-gap.md).
>
> **Reopened 2026-07-31.** That answer was reasoning about a deficit whose size
> came from the superseded measurement. It is pending re-measurement, in either
> direction.

So the fit ships as a preset and a fixture, and **the shipped default is
unchanged**. Moving `DefaultPhysicalDrum()` was the third deliverable of this
work, and it is deliberately not done: a default is a claim about what the model
should sound like out of the box, and a candidate that is empty across the band
carrying the target's character does not earn that on a total-cost improvement
alone. The preset makes the fit auditionable and the fixture makes it
reproducible, which is what the evidence supports.

### Reading the restarts

| Restart | 1     | 2     | 3     | 4     | 5     | 6         | 7     | 8      |
| ------- | ----- | ----- | ----- | ----- | ----- | --------- | ----- | ------ |
| Total   | 8.821 | 6.598 | 8.044 | 7.110 | 7.904 | **6.364** | 7.050 | 10.634 |

The spread from 6.36 to 10.63 says the landscape is multi-modal and that
multi-start is doing real work — the median restart is 20% worse than the best.
The best restart's convergence curve was **still descending at the last
iteration**, so this is a good point rather than a converged one. A longer run
would find a better total; on the evidence above it would not close the 476–700
Hz gap, because that gap does not move with search effort.

## The contact-model head-to-head (2026-07-31)

> **Superseded** by
> [the re-run on the corrected measurement](#the-head-to-head-re-run-on-the-corrected-measurement-2026-07-31),
> which asks the same question with the same budget and answers it the same way.
> Every number in this section was measured with the partial-level estimator
> corrected [below](#a-correction-to-the-partial-measurement-2026-07-31), which
> changed both the reference's partial list and every candidate's. The reasoning is
> kept — the seam-closing argument and the 5 g run's exposure of the
> spectral-envelope term stand on their own — but the totals, the term values and
> the partial counts are not comparable to anything measured after the correction,
> and none of them should be quoted as a result.

`ContactHertzian` closes the excitation gap measured in
[`physical-excitation-gap.md`](physical-excitation-gap.md), so the question was
whether it also closes the fit. It had to be asked _after_ fitting: at the
shipped bank the Hertzian model already scores better (31.087 against 33.455),
and a fixed-default comparison cannot separate a better excitation from
parameters that happen to suit it.

Three runs, each 8 restarts × 150 iterations at population 16, seed 1, Draft,
**59 056 evaluations** apiece, all restarts complete. The iteration budget is
nearly double the run above, which the previous section's closing prediction
makes a test in its own right. Both baselines reproduced to the digit, so these
are comparable to the 6.364 fit and to each other.

| Term                  | Fitted (80 it) | Prescribed | Hertzian   | Hertzian, 5 g | Gate   |
| --------------------- | -------------- | ---------- | ---------- | ------------- | ------ |
| Partial frequency     | 21.5 ¢         | 20.6 ¢ ✅  | 19.6 ¢ ✅  | 27.2 ¢ ❌     | ≤ 25 ¢ |
| Partial decay         | 0.179          | 0.179 ✅   | 0.493 ❌   | 0.188 ✅      | ≤ 0.25 |
| **Spectral envelope** | 13.6 dB        | 12.3 dB ❌ | 14.5 dB ❌ | 11.9 dB ❌    | ≤ 4 dB |
| Partial level         | 2.0 dB         | 2.0 dB     | 2.0 dB     | 2.0 dB        | —      |
| Amplitude envelope    | 1.1 dB         | 1.0 dB     | 2.8 dB     | 3.1 dB        | —      |
| Glide                 | 18.2 ¢         | 18.0 ¢     | 0.1 ¢      | 0.0 ¢         | —      |
| Attack balance        | 0.3 dB         | 0.0 dB     | 0.0 dB     | 1.1 dB        | —      |
| Unmatched energy      | 0.027          | 0.027      | 0.027      | 0.027         | —      |
| **Total**             | **6.364**      | **5.901**  | **7.450**  | **6.548**     | —      |
| Baseline              | 33.455         | 33.455     | 31.087     | 37.868        | —      |

That unmatched row — 0.027 in all four columns, to the digit, across three
different contact models and two mallet masses — is the defect above, sitting in
plain sight and read at the time as a shared floor rather than as a signal. It
is what an energy-weighted share reports when a candidate reproduces the one
partial carrying 99.4 % of the reference's energy and nothing else, which is
what all four of these did. The lesson is worth more than the table: a term that
does not move between candidates is not a constant of the problem, it is a term
that has stopped measuring.

**The prediction above held.** Nearly doubling the search budget moved the
prescribed fit 6.364 → 5.901 and left the band untouched, which is what that
paragraph said would happen and the reason it is worth having written down.

### The seam closes, and it was not the gap

`ATK.T` is the diagnostic, because the previous fit dragged it from 4000 Hz to
1644 Hz — the stochastic attack layer standing in for a band the excitation never
reached:

|         | Prescribed | Hertzian    | Hertzian, 5 g |
| ------- | ---------- | ----------- | ------------- |
| `ATK.T` | 1261 Hz    | **3426 Hz** | 626 Hz        |
| `ATK.L` | 0.021      | 0.012       | **0.078**     |

At the shipped 15 g mallet the Hertzian fit leaves `ATK.T` near its 4000 Hz
default and drops `ATK.L` to the lowest value of the three. The excitation
reaches the high band for real, so the search stops abusing the noise layer to
cover it. That is the seam closing, visible as a parameter difference exactly
where [`physical-contact.md`](physical-contact.md) predicted it.

It does not close the gap:

|                | Reference | Prescribed | Hertzian | Hertzian, 5 g |
| -------------- | --------- | ---------- | -------- | ------------- |
| Partials found | 16        | 5          | 7        | 3             |
| **476–700 Hz** | **9**     | **0**      | **0**    | **0**         |

Hertzian finds two more partials overall and still none in the band carrying
more than half the reference's character. The contact model buys 12–23 dB above
800 Hz and 0–4 dB below 700, and the deficit is below 700.

> **Re-scoped 2026-07-31.** The conclusion drawn here — that the excitation model
> was never the binding constraint and mode density in the band is — was drawn
> from a reference partial list the correction below has changed, on both sides of
> the comparison. It is an open question again.

### The 5 g mallet, and what it exposes about the metric

`Strike.MalletMassKg` is the Hertzian model's strongest lever — contact time here
is set by the head's 0.31 g driving-point mass, not the tip — and it is not in
the product bank, so the search cannot reach it. The measured velocity law says
4–6 g against the shipped 15 g, and at 5 g the Hertzian total improves markedly,
7.450 → 6.548. Taken alone that is a finding about `DefaultPhysicalDrum()`'s
mallet rather than about the fit.

Taken with the parameters it is something more useful. The 5 g run has the **best
spectral envelope of any fit ever measured here** (11.9 dB) and the **fewest
partials** (3). It gets there by parking the attack layer in the gap: `ATK.T` at
626 Hz, inside the 476–700 Hz band, with `ATK.L` at 0.078 — 3.7× the other two
runs. Broadband noise satisfies a band-energy metric; it cannot make resolvable
partials, and the partial-frequency term duly fails the gate at 27.2 ¢.

So the spectral-envelope term is **gameable by the noise layer**, and the partial
terms are what expose it. This is why the gate is per-term. A single-number
comparison would have ranked the 5 g Hertzian run as the closest match to the
recording, when it is the one that resembles it least in structure.

### Verdict

`DefaultContact().Model` **stays `ContactPrescribed`**, and no fitted bank is
adopted. The prescribed 150-iteration fit is the best total ever measured against
this reference (5.901, beating 6.364) and passes two of three gate terms, but it
misses the spectral envelope by 3× and is empty across the band carrying the
target's character — the same argument that left the shipped default alone the
first time, applied to the same evidence. `testdata/physical-fit-tom.json` and
the **Measured tom** preset are therefore unchanged.

The restart spreads say how much weight the ranking carries:

| Run           | Best  | Median | Worst  |
| ------------- | ----- | ------ | ------ |
| Prescribed    | 5.901 | 7.838  | 10.166 |
| Hertzian      | 7.450 | 8.279  | 11.153 |
| Hertzian, 5 g | 6.548 | 7.703  | 9.588  |

They overlap heavily — the Hertzian best would rank 6th of 8 among the prescribed
restarts — so this is a consistent ordering rather than a decisive one. It is
enough to say the Hertzian contact does not earn a calibration pass on fit
quality. It is not enough to say the prescribed excitation is better physics, and
the excitation-gap measurements say it is not.

## Is the head-damping range the constraint? (2026-07-31)

> **Superseded.** Every total, term value and baseline below was measured with the
> partial-level estimator corrected
> [further down](#a-correction-to-the-partial-measurement-2026-07-31). The
> question this section asks and the way it answers it — put a bound to the test
> with a build-time multiplier rather than by widening the shipped spec — are
> unaffected, and so is the qualitative finding that the search reaches the same
> physical damping from three different ranges. The numbers are not comparable to
> anything measured after the correction. The conclusion is independently
> confirmed by
> [the corrected head-to-head](#the-head-to-head-re-run-on-the-corrected-measurement-2026-07-31):
> `DAMP` fits well clear of its lower bound in both of those runs.

The run under the nine-term distance (`fit-v4-hertzian`, Standard, Hertzian,
stopped by hand at 47 % of its restart budget, **11.630** from a 32.585
baseline) fitted `DAMP` to **0.276 against a lower bound of 0.25** — normalized
position 0.036, effectively pinned. A parameter resting on its bound is a
statement about the bound, not about the drum, and the only way to tell which is
to look past it.

Widening the shipped spec to find out is not available: presets store
_normalized_ positions, so moving `expSpec("physicalTom.damping", …, 0.25, 4, …)`
would silently retune every saved patch. `-loss-scale` answers the question
without touching the product — it multiplies every head loss rate on top of
`DAMP`, so a run at 0.25 searches a range whose floor is four times lower. It is
not a knob and never will be: a bank fitted at `-loss-scale ≠ 1` describes a drum
the product cannot be set to. Both the report and the checkpoint fingerprint
record it, so such a run cannot be mistaken for a normal one or resumed into one.

Three fits, equal budget (4 restarts × 60 iterations, population 12, seed 1),
identical but for the multiplier:

| Loss scale    | Baseline | **Best**   | Partial decay | Spectral env. | `DAMP` (norm) | Effective |
| ------------- | -------- | ---------- | ------------- | ------------- | ------------- | --------- |
| 1.0 (control) | 32.585   | **14.917** | 0.966         | 13.27 dB      | 0.475 (0.231) | 0.475     |
| 0.5           | 26.388   | **14.924** | 1.263         | 13.01 dB      | 0.866 (0.448) | 0.433     |
| 0.25          | 26.325   | **14.076** | 1.295         | 12.82 dB      | 2.122 (0.771) | 0.531     |

**The range is not the constraint.** Four times the headroom moves the total by
0.8 — less than the spread between restarts at this budget — and the search
converges on the same physical damping regardless of the range it is given:
effective scales of 0.43–0.53 across all three. `DAMP`'s normalized position
travels 0.23 → 0.45 → 0.77 to compensate for the multiplier, which is exactly
what a parameter does when the model can already reach the value the reference
implies.

**And the two terms do not fall together**, which was the hypothesis. Removing
loss makes partial decay _worse_ (0.966 → 1.295) while the spectral envelope sits
flat at ~13 dB and never approaches its 4 dB gate. The envelope's excess is not
made of over-long modes, so it will not be fixed by damping at all.

At this short budget `DAMP` does not pin even in the control — it fits 0.475, not
the 0.276 the long run found — so the sweep alone could not speak to that
particular pin. A full-budget run at `-loss-scale 0.25` settles it:

| Run                 | Restarts        | Best       | `DAMP` (norm)    | Effective damping |
| ------------------- | --------------- | ---------- | ---------------- | ----------------- |
| v4, loss scale 1    | stopped at 47 % | **11.630** | 0.276 (0.036) 📌 | 0.2764            |
| v5, loss scale 0.25 | all 8 complete  | **13.023** | 1.132 (0.545)    | 0.2830            |

Given four times the range the search chose the same physical damping to within
2 %, and did worse overall than v4 managed in fewer than half the evaluations.
`DAMP` came off the bound as soon as the bound stopped mattering, which is what a
non-binding constraint looks like from the inside. **The pin was that basin's
coincidence, not a limit**, and the head-damping range needs no change.

### What this points at instead

The reference's decay is **non-monotone in frequency**, and no single loss law is:
its loudest partial at 212.8 Hz dies in 0.146 s while the fundamental at 118.1 Hz,
12.1 dB quieter, sustains 1.490 s. (Both ring times survive the correction below
unchanged; the level difference is quoted from the corrected measurement, which
puts it at 12.1 dB rather than the 25 dB the superseded estimator reported.) The
fitted candidate has the opposite tilt —
0.597 s at the fundamental, and 0.71–1.47 s across 240–500 Hz where the reference
spends 0.26–0.55 s.

`DAMP` scales the whole loss law and `D.TILT` slopes it. Neither can bend it, so
this is a model-structure question rather than a range or a search question.
`Head.ModeDecayCorrections` — already scaled by both knobs, already used for the
measured (0,1) correction — is where a per-mode answer would go. That touches the
shipped instrument's sound, so it is not a change this document makes.

## Seeding a restart from the reference's partials (2026-07-31)

> **Superseded, and the recommendation is reversed.** The reference partial list
> the pre-solve aims at, and every fit total in the comparison table, were measured
> with the partial-level estimator corrected
> [below](#a-correction-to-the-partial-measurement-2026-07-31). The mechanism, the
> two defects it exposed, and the paired-comparison design are unaffected; the cent
> figures and totals are not comparable to anything measured after the correction,
> and the pre-solve targets a different set of partials now. On that corrected
> target seeding **lost** — see
> [the corrected head-to-head](#the-head-to-head-re-run-on-the-corrected-measurement-2026-07-31)
> — and `-seeded-restarts` stays off by default. The 12 % win below is a result
> about a seed that was worth 1 ¢, not about seeding as such.

Mode frequencies are analytic — `physical.GenerateDrumModes` reads them off the
tension, radius and cavity without rendering a sample — at about a hundredth of a
full evaluation. That makes it cheap to ask a question the fit cannot: **how
close can the model's modes get to this recording's partials at all?**

| Measurement                 | Audibility-weighted frequency error |
| --------------------------- | ----------------------------------- |
| 20 000 random banks, best   | 11.5 ¢                              |
| the same, hill-climbed      | 10.5 ¢                              |
| the fit (`fit-v4-hertzian`) | 59.7 ¢                              |
| gate                        | 25 ¢                                |

So the model can place its modes on this drum. Read that as a statement about
mode placement and not about the gate: these figures count the distance to the
nearest mode, while the gate counts the distance to the nearest partial the
candidate is actually heard to produce. The two are not the same number, and the
gap between them is the subject of the cautions below.

> **Retired 2026-07-31.** The 11.5 ¢ and 10.5 ¢ figures — and with them the reading
> that "the modes are reachable and the search was not reaching them" — were
> measured against the pre-correction 7-partial target. Against the corrected
> 14-partial target the same pre-solve floors at **35.9–37.0 ¢**, so the conclusion
> does not survive: within the searchable space no bank places its modes on this
> target much better than that. See
> [the corrected head-to-head](#the-head-to-head-re-run-on-the-corrected-measurement-2026-07-31).

`-seeded-restarts N` acts on that: a pre-solve finds N diverse frequency-optimal
banks, and those N restarts search a box around them while the rest search the
whole cube. Seeded restarts come first and the unseeded ones keep their RNG
seeds, so they are bit-for-bit what they were and the comparison is paired.

**The first version made things worse**, and the two defects it exposed are worth
more than the feature. One in the pre-solve, one in the reasoning:

- **`GenerateModes` returns the batter head alone.** It is written for the
  single-head reference and says so, but the pre-solve was using it to reason
  about a double-headed drum, so it never saw half the partials. Nine times the
  resonant tension moved no mode frequency by a single cent. `GenerateDrumModes`
  is the accessor that returns both.
- **The box narrowed every parameter, not the ones the seed knew about.** A
  frequency-only objective has no opinion about damping, the microphone position
  or the attack layer, so confining a restart to a neighbourhood of _those_ threw
  away range for nothing. `frequencyRelevant` now probes which dimensions move
  the modes and boxes only those — probed rather than listed, so it stays true
  when the parameter table changes.

With both corrected, the pre-solve boxes 4 of 17 free parameters — `SIZE`,
`B.TUNE`, `R.TUNE`, `ASYM`, which is the wave speed, the two tensions and the
mode split, discovered by probing rather than written down — and its seeds land
at 1.0 and 1.3 ¢. The comparison is paired: same budget, same RNG seeds, so the
unseeded restarts are bit-for-bit identical between the runs.

| Restart | Control    | Seeded, all dimensions | Seeded, masked |
| ------- | ---------- | ---------------------- | -------------- |
| 1       | 19.442     | 31.707 🌱              | **16.388** 🌱  |
| 2       | 16.754     | 19.109 🌱              | **13.132** 🌱  |
| 3       | 14.917     | 14.917                 | 14.917         |
| 4       | 16.638     | 16.638                 | 16.638         |
| Best    | **14.917** | **14.917**             | **13.056**     |

Both seeded restarts improved and neither regressed, for a 12 % better result at
the same cost — **against a target the pre-solve could reach to 1 ¢**, which the
corrected target is not, and which is why the flag is off by default. The
frequency term fell from 94.4 ¢ to 46.1 ¢, partial decay from
0.966 to 0.766, and the unmatched share from 0.461 to 0.011 — the seeded drum
produces the reference's partials rather than a handful of them. The spurious
share rose, 0.413 to 0.638, which is the honest cost: a bank chosen to have a
mode near every reference partial also has modes elsewhere.

**Two cautions on reading any of this.**

The pre-solve scores the distance to the nearest **mode**; the gate scores the
distance to the nearest detected **partial**. A mode can sit exactly on a
reference partial and never be heard — wrong radiation weight, in a pickup null,
or buried under a louder neighbour — so a 1.0 ¢ seed does not promise a 1.0 ¢
frequency term, and the fitted 46.1 ¢ is the proof. Mode placement is necessary
for that term and nowhere near sufficient.

And the objective has to be checked for discrimination before its numbers mean
anything, because reading both heads roughly doubles the mode count and a dense
enough bank is near _any_ frequency by accident. Measured over 2000 random banks:
median 164.8 ¢, p10 56.2 ¢, best 4.1 ¢. A 1.0 ¢ seed is a real selection from
that distribution, not a free lunch.

## A correction to the partial measurement (2026-07-31)

Two defects in `internal/physical/match/features.go`, both found by probing the
reference rather than by reading the code, and both now fixed. Together they
supersede every fit result measured before this date.

### The partial level estimator was ill-conditioned

`measureDecays` computed a partial's strike level as its magnitude in the sustain
transform divided by the attenuation that window applies to a partial decaying at
the fitted rate (`sustainWindow.decayAttenuation`). That is exact for an isolated
exponential. It is unusable as an estimator, because for the default sustain
window (0.05–0.85 s) the divisor is enormous and depends violently on the fitted
decay:

| T60     | correction gain |
| ------- | --------------- |
| 2.0 s   | 12.4 dB         |
| 1.5 s   | 16.1 dB         |
| 1.0 s   | 22.9 dB         |
| 0.6 s   | 34.2 dB         |
| 0.3 s   | 54.9 dB         |
| 0.15 s  | 82.3 dB         |
| 0.11 s  | 97.5 dB         |
| 0.073 s | 122.0 dB        |

So the reported level was largely a restatement of the fitted decay rate. A 10 %
error in T60 at 0.12 s moves the level by 4.5 dB.

The consequence on the reference is concrete. At a 5 Hz peak separation, a **73 ms**
component (T60 0.073, R² 0.865) was reported as the **loudest partial in the
recording**, which pushed the fundamental (T60 1.49 s, R² 0.984) to −33.6 dB.
Levels are all relative to the strongest, so genuine partials then fell below
`PartialFloorDB` and vanished. That is also why the detected set was non-monotone
in the level floor.

**Capping the correction was considered and rejected with a measurement.** The
reference's loudest partial (212.8 Hz) itself carries an 83.7 dB correction, so
any cap tight enough to exclude the runaways excludes it too.

**The fix** is to take the level from the decay fit's own intercept — the fitted
line's value at t = 0, read off the partial's heterodyned envelope. It is the same
quantity, measured directly: it extrapolates back only as far as
`DecayFitStartSeconds` instead of through the whole Hann taper, and it is a
least-squares fit over hundreds of samples rather than a ratio of two numbers.

The pre-existing warning in that code still holds and is respected: the level must
**not** be the envelope's _peak_ inside the fit window, because a strike transient
smeared through a 150 ms filter once put the 212 Hz partial 32 dB too loud. The
fit starts after the transient, and the intercept is an extrapolation of the
fitted decay, not a reading of the peak.

### Detection was blind to short-ringing partials

The sustain transform spans 800 ms, so a partial ringing a tenth of that stands
roughly 90 dB lower in it. Both detection guards — a relative floor with 20 dB
headroom, and a count limit of `MaxPartials × 2` — rank on that uncorrected
magnitude, and they bind _together_. On the reference, loosening either alone
changes nothing: sweeping headroom 20 → 80 dB at surplus 2 gives 7 partials
throughout, and sweeping surplus 2 → 32 at headroom 20 also gives 7 throughout.
Only both loosened (60 dB / 8) reached 9.

`detectPartials` now also reads a second, earlier window
(`EarlyDetectionStartSecs` 0.05 → `EarlyDetectionEndSecs` 0.30, both new
`Options` fields), and **each window admits candidates relative to its own
strongest peak**, so a short partial competes against short-lived content rather
than against the fundamental's whole ring. The sustain window is admitted first,
so where both windows see a partial the better-resolved frequency is kept. The
early window starts after the strike transient, because a broadband click would
otherwise offer a peak at every frequency. The shipped guard constants are
unchanged (20 dB, ×2).

### The ordering mattered

The tight guards were, accidentally, what kept the unstable estimator from firing:
they excluded precisely the short-ringing population where the correction ran
away. Fixing detection first would have made the measurement worse. Bound the
estimator, then open the aperture.

`TestShortPartialsDoNotOutrankLongOnes` (`internal/physical/match/level_test.go`)
pins both on a synthetic two-partial signal — a loud partial at amplitude 1.0
ringing T60 1.2 s and a quiet one at 0.5 ringing 0.12 s — and depends on no
recording. It failed twice while being developed: first because the levels were
inverted, then because the short partial was not detected at all.

### The corrected target

The reference (`-channel right`) now reduces to **14 partials**, against 7 before:

| Hz     | dB    | T60 s | R²   |
| ------ | ----- | ----- | ---- |
| 118.1  | −12.1 | 1.490 | 0.98 |
| 139.6  | −38.2 | 1.552 | 0.77 |
| 212.8  | 0.0   | 0.146 | 0.94 |
| 259.1  | −30.9 | 0.260 | 0.75 |
| 296.7  | −26.8 | 0.554 | 0.98 |
| 380.5  | −36.5 | 0.468 | 0.41 |
| 476.6  | −36.7 | 1.137 | 0.79 |
| 502.6  | −26.6 | 0.257 | 0.92 |
| 530.1  | −40.9 | 1.198 | 0.96 |
| 546.9  | −35.7 | 0.843 | 0.94 |
| 624.6  | −39.4 | 0.599 | 0.94 |
| 675.8  | −29.9 | 0.629 | 0.93 |
| 696.9  | −26.6 | 0.344 | 0.91 |
| 1598.0 | −38.9 | 0.283 | 0.93 |

**Seven of those fourteen lie between 476 and 700 Hz** — the band this document
calls the recording's character. The previous measurement found two there.

New baselines at the shipped bank, `-channel right`, Standard quality:
**33.094** prescribed, **33.544** Hertzian.

### What this supersedes

Every fit result measured before this date, without exception. That includes the
contact-model head-to-head, the `-loss-scale` damping sweep and the seeding
comparison, each of which now carries a note. The reasoning in those sections is
kept because it is about method and still holds; the numbers are not comparable
to anything measured after.

The standing claim that the fit is _empty_ across 476–700 Hz while the reference
is busiest there is **re-scoped, not resolved**. The reference is still busiest
there — seven of fourteen partials — but part of the measured gap was the target
not asking for those partials either, because they were below the floor. Whether
a real excitation gap remains is an open question.

Both full-budget fits on the corrected metric have since completed; see
[the head-to-head below](#the-head-to-head-re-run-on-the-corrected-measurement-2026-07-31).
They narrow the claim without closing it: both winning banks put **5 of their 16
partials** in 476–700 Hz against the reference's 7, so the band is no longer
empty, but neither reproduces its density.

The general lesson is worth as much as the fix: **the conditioning of a
measurement is a result in its own right.** An exact formula can still be an
unusable estimator, and the way to find out is to probe it — sweep the parameter,
look at what the correction factor does across the range it is asked to span, and
check whether the answer is a measurement or a restatement of one of its inputs.

## The head-to-head, re-run on the corrected measurement (2026-07-31)

Both contact models were refitted from scratch on the corrected partial
measurement above. These are the first fit numbers on this page that are not
superseded, and they replace the [earlier
head-to-head](#the-contact-model-head-to-head-2026-07-31) outright.

Two runs, identical but for `-contact`: `reference/tom.wav`, `-channel right`,
Standard quality, 8 restarts × 150 iterations at population 16, seed 1,
`-seeded-restarts 4`, **59 056 evaluations** each, all restarts complete and
neither interrupted. Reports are `fit-final-prescribed.json` and
`fit-final-hertzian.json`; both are gitignored, so the numbers below are quoted
rather than linked.

**Prescribed wins, 11.252 against 11.535**, from baselines of **33.094** and
**33.544** at the shipped bank.

Each term is given with its threshold and with its contribution to the total
(value × weight; a term sitting exactly at its threshold contributes 1.0), which
is what makes the two columns comparable term by term rather than only in the
sum:

| Term                         | Threshold | Prescribed | contrib  | Hertzian | contrib  |
| ---------------------------- | --------- | ---------- | -------- | -------- | -------- |
| Partial frequency (gate)     | 25 ¢      | 56.9 ¢     | 2.28     | 47.2 ¢   | 1.89     |
| Partial decay (gate)         | 0.25      | 0.581      | 1.66     | 0.728    | 2.08     |
| **Spectral envelope** (gate) | 4 dB      | 12.28 dB   | **3.07** | 12.30 dB | **3.08** |
| Partial level                | 3 dB      | 7.58 dB    | 2.53     | 9.18 dB  | 3.06     |
| Amplitude envelope           | 3 dB      | 1.27 dB    | 0.42     | 1.18 dB  | 0.40     |
| Glide                        | 40 ¢      | 17.6 ¢     | 0.44     | 0.12 ¢   | 0.00     |
| Attack balance               | 6 dB      | 0.03 dB    | 0.00     | 1.31 dB  | 0.22     |
| Unmatched share              | 0.5       | 0.151      | 0.30     | 0.047    | 0.10     |
| Spurious share               | 0.5       | 0.275      | 0.55     | 0.359    | 0.72     |
| **Total**                    | —         | **11.252** | —        | 11.535   | —        |
| Baseline                     | —         | 33.094     | —        | 33.544   | —        |

**The two baselines moved in opposite directions across the correction** —
prescribed 33.236 → 33.094, Hertzian 32.585 → 33.544. That is the plainest
available statement that the correction changed *what is being measured* rather
than applying an offset to it, and it is why no baseline or total from either
side of the correction can be compared with one from the other.

### Verdict: unchanged, on better evidence

`DefaultContact().Model` **stays `ContactPrescribed`**.

The margin is small — 11.252 against 11.535 — and the restart spreads say how
much weight it carries. Best first:

| Run        | Restart totals                                                          |
| ---------- | ----------------------------------------------------------------------- |
| Prescribed | **11.252**, 11.741, 12.038, 12.389, 13.220, 14.189, 14.924, 15.461       |
| Hertzian   | **11.535**, 13.141, 13.169, 13.574, 14.446, 17.355, 17.770, 18.904       |

The spreads overlap at the top and separate below it: Hertzian's best would rank
**second** among the prescribed restarts, while its remaining seven all fall
outside the prescribed range's better half. So this is a **consistent ordering
rather than a decisive one** — the same reading the earlier head-to-head arrived
at, now on a measurement that survives.

The two also win on **different terms**, which is worth more than the margin:

- **Hertzian is better** on partial frequency (47.2 ¢ against 56.9 ¢), glide
  (0.12 ¢ against 17.6 ¢, i.e. essentially none) and unmatched share (0.047
  against 0.151).
- **Prescribed is better** on partial decay (0.581 against 0.728), partial level
  (7.58 dB against 9.18 dB), attack balance (0.03 dB against 1.31 dB) and
  spurious share (0.275 against 0.359).

Prescribed wins the sum because it wins the terms that carry weight here, not
because it is better everywhere. Nothing in this says the prescribed excitation
is better physics; the excitation-gap measurements still say it is not.

### Seeding lost, and this supersedes the 12 % win

Restarts 1–4 are seeded and 5–8 are not, so each run is its own paired
comparison:

|            | Seeded best | Unseeded best | Seeded mean | Unseeded mean |
| ---------- | ----------- | ------------- | ----------- | ------------- |
| Prescribed | 11.741      | **11.252**    | 13.20       | 13.10         |
| Hertzian   | 13.574      | **11.535**    | 15.79       | 14.19         |

**The winning restart in both runs was unseeded.** This supersedes the 12 %
improvement recorded in [the seeding
section](#seeding-a-restart-from-the-references-partials-2026-07-31).
`-seeded-restarts` stays **off by default**, and the recommendation there is
corrected in place; the method and the masking result are unaffected.

The mechanism is visible in the reports rather than inferred. On the corrected
target the pre-solve's four converged seeds land at **35.9, 37.0, 37.0 and
37.0 cents**. On the old target the same pre-solve landed at **1.0–1.5 cents**.

That is the whole story, and it generalizes: **a box around a seed is worth its
cost in proportion to how good the seed is.** A 1 ¢ seed is a strong selection
from the random distribution and buys a restart a head start worth more than the
range it gives up. A 36 ¢ seed against a 25 ¢ gate is barely a selection at all,
so boxing four dimensions around it only removes range. The pre-solve reports its
own error before any rendering happens, so this is decidable in advance rather
than after an hour of search — which makes it a usable rule and not just a
result.

### The seed error is itself a result about the model's reach

Four converged analytic pre-solves flooring at 35.9–37.0 ¢ against a 25 ¢ gate is
a statement about the searchable space, not about the search: **no bank the
product can express places its modes on this target much better than that.**

This **retires** the claim recorded in the seeding section that "the model can
place its modes on this drum" — 11.5 ¢ over 20 000 random banks, hill-climbed to
10.5 ¢, read at the time as evidence that the modes were reachable and the search
was not reaching them. Those figures were measured against the pre-correction
7-partial target. The corrected target has **14 partials, seven of them between
476 and 700 Hz** — a seven-partial cluster inside a 224 Hz span — and the analytic
pre-solve cannot get near it.

Two consequences, both open:

- Whether the 25 ¢ frequency gate is reachable **at all** by tuning within this
  bank is now an open question, where before it was thought settled in the
  affirmative.
- The gate itself may need re-deriving. It was calibrated against a target with
  half as many partials, and a per-partial cent tolerance does not mean the same
  thing when seven partials must be placed inside a 224 Hz span.

### What moved and what did not

`testdata/physical-fit-tom.json` and the **Measured tom** preset in
`web/src/algo/physicalTomPresets.ts` have been re-derived from the prescribed
winner, because they are what makes the fit auditionable and reproducible and
they should describe the best fit measured under the current metric.

`DefaultPhysicalDrum()` and `DefaultContact()` are **unchanged**, because the fit
**misses all three gate terms** — partial frequency at 2.28×, partial decay at
1.66×, spectral envelope at 3.07× their thresholds. The fixture and the preset
record what was found; the defaults are a claim about what the model should sound
like out of the box, and a candidate that fails every gated term does not earn
that on a total-cost improvement alone. This is the same rule applied the same
way as in the two earlier rounds.

The partial counts show the fit is a drum rather than either degenerate extreme,
and also where it still is not the target: both winning banks produce **16
partials, 5 of them between 476 and 700 Hz**, against the reference's 7 in that
band. The band is no longer *empty* — that was the pre-correction finding — but
neither bank reproduces the density there.

Worth recording separately: `DAMP` fits to **0.709 (normalized 0.376)** in the
prescribed winner and 0.524 (0.267) in the Hertzian one. Neither is near the
0.25 lower bound, which is consistent with
[the loss-scale experiment's](#is-the-head-damping-range-the-constraint-2026-07-31)
conclusion that the earlier pin was a property of that basin and not of the range.

### The spectral envelope has not moved for anything

12.28 dB prescribed, 12.30 dB Hertzian. To two decimal places the term does not
distinguish the two contact models, and it is the **largest single contributor in
both runs** at just over 3× its threshold.

It has now survived every intervention tried against it: contact model, head
damping range (`-loss-scale`, four times the headroom, flat near 13 dB), seeding,
and a corrected target that changed every other term. A quantity that does not
move under four independent interventions is behaving less like a hard problem
than like a badly posed one.

**An open suggestion, not a conclusion:** the term may be ill-posed for this
model rather than the model failing it. It is mean-removed third-octave band
shape out to 12.5 kHz, and above about 3 kHz this model has nothing but the
stochastic attack layer — no shell, no bearing edge, no lug or hardware
resonances, no room. A 4 dB threshold over that full span may be asking for
content the model structurally cannot produce, in which case the honest fix is to
band-limit the term or to re-derive its threshold, not to keep fitting against it.
Settling that means measuring what the term's value would be over a restricted
band, which has not been done. Until it is, this is a question and the 4 dB gate
stands.

## Reproducing

The run is deterministic given the seed; `TestSearchIsDeterministic` keeps it
that way. It is not part of `just ci`: it takes minutes, and it needs a
reference recording the repository does not contain.

### Listening to what it found

`-wav <path>` renders the fitted bank alongside the JSON report, at
`-wav-duration` (3 s by default) rather than the 1.2 s the search itself renders
— long enough to hear a tail, where the fitting duration is chosen to be just
long enough to measure one. It renders from the candidate's own recorded config,
so what lands on disk is the drum the report describes even when the report was
resumed from a checkpoint. The export is peak-normalized, like the reference, so
it is for listening and not for judging level; the true peak is printed.

This matters because **the distance is a proxy and the recording is the target**.
Every number in this document is a summary of nine terms that were chosen by
hand, and a fit that wins on them can still be wrong in a way the terms do not
represent — the 5 g run above is exactly that, and it is audible long before it
is arguable. A/B against `reference/tom.wav` before believing any total.

### Stopping one, and picking it up again

A fit takes the better part of an hour, which is long enough that something
else will want the machine. `-checkpoint FILE` makes that cheap, and
`just fit-physical` passes one by default.

A true continuation is not available: mayfly runs its whole loop inside
`Optimize`, offers no per-iteration hook and cannot be handed a starting
population, so a stopped swarm's velocities and personal bests are gone for
good. Two things can be saved instead, and between them they cover both ways a
run ends.

**Finished restarts.** Multi-start is the outer loop, so a restart that
finished is finished; a resumed run skips it and replays its position through
this build's measurement code rather than trusting a stored number. A restart an
interrupt cut short is recorded but marked incomplete, and re-run.

**The best point, continuously.** This is the one that matters, and the first
design missed it. Every restart runs concurrently, so they all finish at roughly
the same moment — interrupt a run half way and typically _none_ of them is
complete, and restart-level checkpointing alone would save nothing at all. The
best position any restart has reached is therefore recorded from inside the
objective, every 250 evaluations, which bounds what a stop can destroy to a
quarter of a percent of the run.

`SIGINT`/`SIGTERM` asks the search to wind up rather than killing the process:
the objective starts returning `+Inf` without rendering, so mayfly keeps the
incumbent it already had and every restart still reports the best it genuinely
found. A stopped run therefore writes a normal report, marked
`"interrupted": true` — which also qualifies its evaluation count, since the
tail of that count is refusals rather than measured candidates. A second signal
kills it the usual way.

Resuming onto a checkpoint from a different run is refused, naming the field
that changed. The guard that matters is not the flags but **`baselineCost`** —
the shipped bank's distance from the reference, measured end to end through the
same synthesis and feature extraction the search uses, and computed on every run
anyway. Any edit that moves a rendered sample or a measured feature moves it
too, and that is exactly the case a resume must refuse: a best-of taken across
two different models is not a fit, and nothing downstream would reveal the mix.
A performance change that really is bit-exact leaves it untouched and resumes
cleanly, so the guard doubles as a test of that claim.
