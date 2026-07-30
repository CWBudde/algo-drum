# Fitting the physical Tom to a recording

This document records the measurement, the search, and what the first fit
found.

It does **not** close P6's _"fit documented presets from measurement"_ item, and
it does not meet P8's exit criterion. It builds the machinery both of those need
and reports honestly how far one recording gets: modal frequency and decay land
inside tolerance, spectrum does not, and the shipped default is left alone as a
result. The gap it leaves is specific enough to be the next piece of work.

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

| Feature            | How                                                                                                                                                    | Reused from                              |
| ------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------ | ---------------------------------------- |
| Onset              | impulse-start detection                                                                                                                                | `measure/ir`                             |
| Partials           | Hann-windowed 64k transform of the sustain, topographic peak prominence, log-domain parabolic interpolation                                            | `dsp/window`, `algo-fft`, `dsp/spectrum` |
| Per-partial decay  | heterodyne to baseband, zero-phase Butterworth low-pass whose cutoff is set from the measured spacing to the nearest neighbour, log-linear fit with R² | `dsp/filter/design`, `dsp/filter/biquad` |
| Per-partial level  | the sustain transform's magnitude, divided by the attenuation that window applies to a partial decaying at the fitted rate                             | —                                        |
| Glide              | residual phase slope of the loudest partial at 30 ms against 400 ms                                                                                    | as above                                 |
| Spectral shape     | ⅓-octave band levels, mean-removed, in four windows (attack / early / body / tail)                                                                     | `stats/frequency`                        |
| Amplitude envelope | frame RMS in dB, peak-referred                                                                                                                         | `stats/time`                             |
| Attack balance     | 1–8 kHz against 100–500 Hz in the first 20 ms                                                                                                          | —                                        |
| Decay metrics      | RT60, EDT, T20, T30, C50, C80 (reported, not fitted)                                                                                                   | `measure/ir`                             |

### The distance

Eight terms, each in its own perceptual unit so it can be read against a
tolerance rather than only against another run. The weight on each is the
reciprocal of that term's "clearly wrong" threshold — 25 cents of pitch, 3 dB
of partial balance, a factor of 1.4 in ring time, 4 dB of spectral shape, 3 dB
of envelope, 40 cents of glide, 6 dB of attack balance — so a just-audible
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

### Two measurement problems worth recording

Both were found by measurement, not by inspection, and both changed the design.

**A phantom partial, and why the floor is not the fix.** The reference has a
genuine, isolated component at 87 Hz — 22 dB of topographic prominence, so not
a shoulder — that is 39.5 dB below the fundamental. A bare local-maximum test
also accepted ripples on the fundamental's skirt, which prominence now rejects.
But the 87 Hz peak is real, and no model of a two-headed drum will produce it.
A level floor tight enough to exclude it also discards the 500–700 Hz cluster
that carries the drum's character. The fix is instead in the distance: the
`Unmatched` term is weighted by **energy**, not by count, so failing to
reproduce a partial costs exactly what that partial was worth.

**A degenerate optimum, and the blend that closes it.** An error averaged over
matched pairs is zero when there are no pairs. A candidate that produces one
partial in the wrong place therefore scored _better_ on three of the eight
terms than any real drum can — and the search found it immediately, reaching
11.2 against the shipped default's 39.2 from a render that sounded like
nothing. Each partial term is now blended against a fixed penalty in proportion
to the unmatched reference energy, so a partial that is missing costs what a
partial that is present but wrong costs.
`TestSilenceIsNeverCheaperThanADrum` pins it.

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

## Reproducing

The run is deterministic given the seed; `TestSearchIsDeterministic` keeps it
that way. It is not part of `just ci`: it takes minutes, and it needs a
reference recording the repository does not contain.

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
