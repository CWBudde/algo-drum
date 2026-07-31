# Fitting the physical Tom to a recording

This document records the measurement, the search, what the first fit found, and
the later refit that gave each contact model the same budget over the same bank.

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
to the unmatched reference energy, so a partial that is missing costs what a
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
from too few modes to too many. `matchPartials` iterates the *reference*, so a
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
> after, and none of them should be quoted as a result.

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

## The contact-model head-to-head (2026-07-31)

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
800 Hz and 0–4 dB below 700, and the deficit is below 700. **The excitation model
was never the binding constraint on this fit**; the mode density in that band is,
which is the P8 question and stays open.

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
