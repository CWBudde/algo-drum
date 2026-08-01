# Validating the fitting objective against itself

Written **2026-08-01**. This document is the evidence record for one question that
had never been asked: **how much of what the fitting objective reports is real?**

It is deliberately narrow. The _conclusions_ drawn from these measurements live in
[`docs/physical-measured-fit.md`](physical-measured-fit.md) and in `PLAN.md` §P10;
this file holds the **method, the commands to reproduce it, and the raw numbers**,
so that the conclusions elsewhere can be checked rather than believed. Nothing
here should be restated at length elsewhere — link to it instead.

## Why this was needed

Every judgement on the physical path had been made by
`internal/physical/match`'s nine-term distance: three adoption gates (partial
frequency 25 cents, partial decay 0.25, spectral envelope 4 dB), six rounds of
intervention against them, and a standing conclusion that the model had hit a
ceiling. The objective's own **reproducibility had never been measured**. There
was no bootstrap, no jackknife, no repeat-measurement figure, and no error bar on
any term anywhere in the repository.

## The method

A stereo recording of one drum hit contains **two observations of the same
acoustic event**. Reducing each channel independently and scoring one against the
other gives a direct empirical floor: no model can be expected to match a
recording more closely than the recording matches itself.

`cmd/fit-physical -report-only` runs `match.Extract` on the chosen reduction,
writes the reduced feature vector into the report's `target` field, scores the
shipped defaults and stops — seconds, not minutes. So the whole experiment is two
extractions and one distance evaluation:

```bash
go run ./cmd/fit-physical -reference reference/tt08x08-mp-hd-v08.wav \
    -channel left  -report-only -o /tmp/target-left.json
go run ./cmd/fit-physical -reference reference/tt08x08-mp-hd-v08.wav \
    -channel right -report-only -o /tmp/target-right.json
# then evaluate match.Distance(target_right, target_left)
```

The distance was evaluated by a standalone reimplementation of `match.Distance`,
**validated bit-exactly** before use: recomputing the shipped fit's terms from its
own report reproduces all ten to within 10⁻¹⁵. That validation must be repeated if
`distance.go` changes, or the numbers below stop meaning anything.

Two details of the real matcher that a naive reimplementation gets wrong, and
which materially change the result:

- matching is **greedy over all pairs sorted by cents, without replacement** — a
  candidate partial cannot match two reference partials;
- the match tolerance **widens with reference index**, `120 × (1 + 0.15 i)` cents,
  so high partials are matched far more permissively than low ones.

An earlier hand-rolled matcher that allowed double assignment reported 29.3 cents
where the real objective reports 58.5. Use the validated path.

## Result 1 — the objective disagrees with itself

### On a coincident pair (the strong result)

`reference/tt08x08-mp-hd-*.wav` is a **coincident** XY pair: peak inter-channel
correlation falls at **exactly 0 samples of lag**, at 0.87–0.97. Two microphones
at the same point in space. There is no room-path or arrival-time explanation
available for any disagreement between them.

Full distribution over **32 scorings** — sixteen velocities, each scored in both
directions, since the objective is not symmetric:

| Term              |    min |     median |     p75 |         p90 |     max |
| ----------------- | -----: | ---------: | ------: | ----------: | ------: |
| Partial frequency | 15.333 | **71.785** | 102.824 | **113.034** | 122.093 |
| Partial level     |  0.000 |      9.076 |  14.174 |  **17.849** |  23.577 |
| Partial decay     |  0.419 |  **0.618** |   0.778 |   **1.262** |   3.877 |
| Spectral envelope |  2.522 |  **3.163** |   3.411 |   **3.653** |   3.843 |
| Envelope          |  1.544 |      3.127 |   3.395 |   **3.812** |   3.842 |
| Glide             |  0.000 |     40.000 | 136.992 | **310.348** | 345.434 |
| Attack balance    |  0.151 |  **0.258** |   0.630 |   **1.116** |   1.725 |
| Unmatched share   |  0.000 |      0.252 |   0.326 |   **0.880** |   1.000 |
| Spurious share    |  0.000 |      0.166 |   0.293 |   **0.346** |   1.000 |
| **Total**         |  8.584 | **12.893** |  16.998 |      19.080 |  25.254 |

To reproduce: run the two `-report-only` extractions above for each of
`reference/tt08x08-mp-hd-v01.wav` … `v16.wav`, then evaluate the validated
`match.Distance` reimplementation both ways round on each pair and take the order
statistics per term. The reimplementation is a scratch file rather than repository
code, deliberately — it is only trustworthy while it reproduces `distance.go`
bit-exactly, and pinning a second copy in the tree would invite the two to drift.

Reading it:

- **The 25-cent and 0.25 gates were never achievable by any model.** Nothing can
  beat 113 cents or a 1.26 log-ratio on this evidence.
- **The spectral envelope is the only gate that was right** — 3.65 dB at p90
  against a gate of 4, and the tightest spread in the table (2.52–3.84).
- **Attack balance is the most reproducible term in the whole objective** — median
  0.258 dB, better than the spectral envelope — and it carried the _smallest_
  weight, `1/6`. This was not previously noticed and is a real finding.
- **Glide's median is exactly 40.0**, which is `unreadableGlideCents`: in over half
  the pairs one channel's fundamental did not survive far enough to place two
  probes on. The term is broken, not merely noisy. Note this is _per-hit_ glide;
  glide-**across velocities** (−130 ¢ at v04 → −353 ¢ at v12) is a different and
  perfectly sound measurement, and is what P10/N5 uses.

### What this was used for

`match.DefaultWeights()` was re-derived from this table on 2026-08-01: **every gate
is the p90 above, rounded, and every weight is 1/gate.** The rule that a term at
its gate contributes exactly 1.0 was already the intent; it is now also true, which
it had not been for `PartialDecay` (weight `1/0.35` against a documented gate of
0.25).

| Weights  | Floor: min |     median |    p90 |    max |
| -------- | ---------: | ---------: | -----: | -----: |
| old      |      8.584 | **12.893** | 19.080 | 25.254 |
| measured |      3.180 |  **4.320** |  6.457 |  7.490 |

Under the old weights the best fit ever recorded (10.38) scored **below the
objective's own noise floor (12.89)**. Under the measured weights no single term
contributes more than 0.79 at its median, which is the commensurability the weights
always claimed. **Totals recorded before this change are not comparable to totals
after it.**

One deliberate departure from the rule: `Spurious`' measured floor of 0.346 would
give it 2.6× `Unmatched`'s weight, and that asymmetry has already been refuted by a
fit run — it abandoned the drum for two partials, because outweighing the blend's
pressure toward completeness makes emptiness the cheapest bank on offer. Both terms
therefore keep `Unmatched`'s gate. The reasoning is on `DefaultWeights` and pinned,
as far as it can be, by `TestSpuriousDoesNotOutweighCompleteness`.

Re-derive all of this if `features.go` changes: the floor is a property of the
estimator, not of the drum.

> **`features.go` has since changed, and this table is stale.** Result 5a found
> that the estimator these gates were measured through was collapsing `v09`,
> `v10` and `v14` to one- or two-partial tables, which is what those takes
> contributed to the distribution above. The reciprocal-gate rule and the three
> findings stand; the numbers must be re-measured. `PLAN.md` §N2 carries it as
> the first thing to do.

### On the retired spaced pair, for comparison

`reference/tom.wav` (44.1 kHz, channels 1.56 ms apart, correlating 0.36 at zero
lag — a _spaced_ pair, so summing it combs):

| Term              | Left vs right | Best fitted model | Gate |
| ----------------- | ------------- | ----------------- | ---- |
| Partial frequency | 58.5 ¢        | 48.9 ¢            | 25   |
| Partial level     | 14.67 dB      | 9.08 dB           | —    |
| Partial decay     | 0.844         | 0.573             | 0.25 |
| Envelope          | 1.42 dB       | 0.72 dB           | —    |
| Glide             | 9.60 ¢        | 0.03 ¢            | —    |
| Spectral envelope | 3.44 dB       | **11.07 dB**      | 4    |
| Spurious share    | 0.121         | **0.327**         | —    |
| **Total**         | **11.76**     | **10.38**         | —    |

The fitted model scores **better than one channel scores against the other**. That
is not paradoxical — the model is fitted to that specific microphone, whereas the
left-vs-right figure bounds what a _microphone-position-independent_ model could
do — but it does establish that the partial-based terms are dominated by
position-dependent detail. Pushing them harder fits the room, not the drum.

### Why the terms are unreliable: outliers, not noise

The estimator is bimodal rather than noisy. Across matched partials the _median_
disagreement is ~1 cent and ~0.1 in log-T60 — excellent — with a handful of
catastrophic mis-assignments that RMS aggregation converts into the headline
number. The clearest single case, from the retired recording: the 139.6 Hz partial
reads **−38.2 dB with T60 1.55 s** on one channel and **0.0 dB — the loudest
partial — with T60 0.128 s** on the other.

Two structural causes, both in `internal/physical/match/features.go`:

- **`MinSeparationHz = 15`** is **207 cents at 118 Hz**. The extractor structurally
  cannot resolve the mode pairs that `TensionAsymmetry`/`ASYM` exists to model — a
  2 % split at 213 Hz is 4.3 Hz — so `ASYM` is fitted against a target with
  asymmetry averaged out of it. Merged pairs also **beat**, and the log-linear
  decay fit over a beating envelope terminates at the first null and returns a
  slope with a _high_ R², so `FitQuality` does not protect against this and may
  reward it.
- The decay estimator is plain log-linear regression with hard truncation at
  −45 dB. Karjalainen et al. (JAES 50(11):867–878, 2002) exists to replace exactly
  that, with an explicit exponential-plus-noise-floor model.

## Result 2 — the residual, budgeted

Against the retired recording the fit reached 11.07 dB spectral envelope. Three
independent routes agree on how it decomposes:

| Component                                       | Size        | Reachable?           |
| ----------------------------------------------- | ----------- | -------------------- |
| Measurement floor (room, mic, estimator)        | ~3.2–3.4 dB | No                   |
| **Time-varying — the spectrum evolves wrongly** | **~7.8 dB** | **Yes — the target** |
| Static spectral shape                           | ~1.7–3.3 dB | Partly               |

Per time window: attack **6.46** / early **9.99** / body **14.36** / tail
**13.47** dB. Flat at onset, diverging monotonically with time. This is a
**damping-distribution** defect, not a spectral-shape one — the spectral-envelope
and partial-decay terms are one defect seen twice.

The concrete instance: the model synthesizes a partial at **186.0 Hz with
T60 1.81 s — the longest-ringing thing it produces** — with no counterpart in the
recording (which has nothing between 139.6 and 212.8 Hz). It is simultaneously the
top contributor to the envelope term, most of the 0.327 spurious share, and part
of the frequency error. Together with four others (269.7, 341.9, 428.3, 742.0 Hz)
it makes the spurious term worth **0.654 of the 10.38 total**.

## Result 3 — falsification tests, all negative

Each hypothesis below was proposed, tested and refuted. They are recorded so the
work is not repeated.

### The envelope term is not a band-coverage artifact

The standing hypothesis was that the 11.07 dB is unreachable because the term runs
to 12.5 kHz while the model has only the stochastic attack layer above the top
retained mode. Measured, by re-centring and band-limiting:

| Band                                    | Spectral envelope |
| --------------------------------------- | ----------------- |
| 50 Hz – 10.2 kHz (as shipped, 24 bands) | 11.07 dB          |
| ≤ 3.2 kHz                               | 9.36              |
| ≤ 2.0 kHz                               | **9.11**          |
| ≤ 1.6 kHz                               | 9.18              |

Band-limiting to where the model has full modal content buys **2 dB of 11**. Even
restricted to 50 Hz–2 kHz the split is attack 4.69 / early 4.08 (at the gate) vs
body 12.51 / **tail 15.16**. The hypothesis is real but small, and cannot rescue
the term.

A related claim also fails: the top band's upper edge is 11.4 kHz, below the
12 kHz `Pickup.LowpassHz`, so the configured 12.5 kHz maximum never generates a
band the pickup filter zeroes.

### A static body/radiation post-filter cannot fix it

The topology is physically correct — membranes → shell/edge → radiation → room →
mic really are linear at drum SPLs — and the design literature is solid (Bank,
ICMC 2007, gives ~13 pole pairs for ±1 dB fractional-octave accuracy). The
falsification criterion was: five free parameters should take 11.07 → 3–4 dB.

Fitting a static `g(f)`, mean-removed, across all four windows:

| Free parameters                 | Spectral envelope |
| ------------------------------- | ----------------- |
| 2 (tilt)                        | 10.09 dB          |
| 3                               | 9.96              |
| **5**                           | **9.34**          |
| 7                               | 8.40              |
| **24 (fully free per-band EQ)** | **7.81**          |

It fails at five and it fails at twenty-four — and 24 free bands against 24 bands
is the absolute limit of any static filter. **The 7.81 dB floor is the
time-varying component**, which is where Result 2's budget comes from.

This is worth recording precisely because it forecloses the dishonest version: a
26–40-coefficient filter against ~24 bands would drive the term to zero **by
construction** and teach nothing. Any future observation-model work must start at
~5 physically-parameterised terms and report the full-band number alongside.

### Exterior air loading is not the missing mechanism

The argument for it was strong: the model's (1,1)/(0,1) ratio is structurally
pinned at 1.588 with no parameter able to move it, the retired recording showed
1.802, tom measurements in the literature run 1.77–1.85 (Bork & Meyer 1.772,
Sørensen 1.85), and an added-mass calculation predicts 1.783.

The licensed reference settles it, because its **geometry is known**: fixing the
fundamental predicts every other mode with no free parameter.

| Source                        | (1,1)/(0,1) |
| ----------------------------- | ----------- |
| Ideal circular membrane       | 1.594       |
| **`tt08x08-mp-hd`, measured** | **1.584**   |
| Air-loaded prediction         | ~1.78       |

An **11-cent** match to the ideal model, ~950 cents from the air-loaded
prediction. The 1.802 seen in the retired recording belongs to that drum's
two-head tuning — a mechanism the model already has — not to missing physics. This
**confirms** the original rejection of air loading in
[`physical-tom-review.md`](physical-tom-review.md) rather than overturning it, and
on far better evidence than the scatter argument it originally rested on.

### No more sophisticated formulation is warranted

A survey of the alternatives — FDTD and 3D air fields, waveguide meshes, the
Functional Transformation Method, FEM/BEM, port-Hamiltonian formulations,
mass-interaction, and differentiable/learned synthesis — found none that targets
the defect that survives measurement. Detail and citations in
[`physical-model-research.md`](physical-model-research.md). The short version:
FDTD is dead on cost and on fittability; FTM _is_ this model's modal expansion for
a separable circular membrane; a 3D air field changes nothing about the coupling
stiffness because non-uniform cavity modes have zero net volume; and
differentiable modal synthesis upgrades the **search**, which is not the ceiling —
two independent runs already agree term for term.

## Result 4 — the two references, characterised

|                 | `tom.wav` (retired)                             | `tt08x08-mp-hd-*` (current)                          |
| --------------- | ----------------------------------------------- | ---------------------------------------------------- |
| Provenance      | unknown                                         | Freesound `quartertone`, stated                      |
| Licence         | none                                            | **CC BY 4.0**                                        |
| Committed       | no, and may not be                              | **yes**                                              |
| Instrument      | unknown                                         | 8" × 8" tom, Remo coated Ambassador / clear Diplomat |
| Sample rate     | 44.1 kHz                                        | **48 kHz**, 24-bit — no resampling                   |
| Stereo geometry | **spaced**, 1.56 ms apart, r = 0.36 at zero lag | **coincident**, 0 lag, r = 0.87–0.97                 |
| Mono summing    | **combs the target**                            | safe                                                 |
| Takes           | one hit                                         | 16 velocities × 7 tunings × 3 strike types           |

Provenance and checksums are in [`reference/CREDITS.md`](../reference/CREDITS.md).

The velocity series is the most valuable property: measured glide rises
monotonically with strike velocity (**−130 ¢** at v04, **−174 ¢** at v08,
**−353 ¢** at v12), so the Berger nonlinearity is finally constrained by a curve
rather than by a single scalar.

Two practical constraints: the analysis window is 1.2 s and the medium-pitch files
are 1.25 s, so the higher tunings (down to 0.52 s) need the tail window shortened
before they are usable; and `v05`/`v06` are near-duplicate takes, agreeing to
seven decimal places through the analysis chain.

## Result 5 — the estimator, measured against a second one

Added 2026-08-01 for `PLAN.md` §N2, which required the fast estimator to be
compared partial by partial against a high-resolution one **before** anything is
refitted against either.

The second estimator is subband ESPRIT with a stabilisation sweep
(`internal/physical/match/esprit.go`), run over all sixteen medium-pitch
head-strike velocities, mono, with
`go run ./cmd/measure-tom -channel mono -high-resolution reference/tt08x08-mp-hd-v*.wav`.
It is measurement equipment: nothing in a fit calls it, and `Distance` does not
know it exists. Its accuracy is pinned on synthetic signals of known content in
`esprit_test.go` — 0.1 Hz on frequency, 2 % on ring time.

### 5a — a defect that made three of sixteen takes unusable

**Found, fixed, and it is worse than either defect §N2 named.**

`measureDecays` reports a partial's level as its decay fit extrapolated back to
the strike. The fit itself was admitted on a **sample count** — sixteen — with no
lower bound on the time it spans. On `v10`, a candidate at 2349.6 Hz was fitted
over **6.1 ms** of trace at **−4034 dB/s** and extrapolated back 50 ms to an
intercept of **+137 dB**, against the loudest genuine partial's −6.6 dB. Levels
are relative to the strongest partial, so all sixty-three others fell below the
−42 dB floor and were discarded.

The tool then reported the drum's fundamental as 2349.6 Hz with a T60 of 15 ms.

| take  | partials before | after |
| ----- | --------------- | ----- |
| `v09` | 2               | 16    |
| `v10` | **1**           | 16    |
| `v14` | 1               | 16    |
| `v02` | 5               | 16    |

Any fit scored against those takes was scoring a target of one partial, with
Unmatched, Spurious and all three partial terms computed from it. Nothing warned.

The fix is one condition: a fitted ring time faster than the envelope filter's
own fastest pole cannot be a measurement, because no input produces such an
output. See `fastestObservableT60`. The bound is the _faster_ of the cascade's
two pole pairs, deliberately — the slower one (287 ms at the 10 Hz cutoff floor)
would have discarded this drum's own fundamental, and the argument for using it
is wrong for a reason that was measured rather than reasoned about; see 5d.

### 5b — the merge defect is real, and it is at the modes that matter

Across the sixteen takes, **24 fast partials stand for two or more subspace
components**, recurring at the same frequencies: 304 Hz, 351 Hz, 586–613 Hz,
851 Hz. Spreads run 1.5–22 Hz, most inside the 15 Hz separation guard. Examples:

| fast partial | components                       | fast T60 | R²   |
| ------------ | -------------------------------- | -------- | ---- |
| 304.6 Hz     | 301.6 / 0.256 s, 306.2 / 0.213 s | 0.317 s  | 0.88 |
| 606.7 Hz     | 598.9 / 0.170 s, 606.9 / 2.155 s | 0.864 s  | 0.98 |
| 851.3 Hz     | 851.1 / 1.010 s, 860.8 / 0.755 s | 0.889 s  | 0.94 |

So `TensionAsymmetry` is indeed being fitted against a target with the asymmetry
averaged out of it, as §N2 predicted.

### 5c — `FitQuality` does not do the job it is used for

`partialDecayError` weights each matched pair by the product of the two fits' R²,
on the stated belief that a beating or buried partial scores low there and is
weighted down. Measured across all 164 paired partials:

| fast R² | median \|ΔT60\| vs the subspace estimator | n   |
| ------- | ----------------------------------------- | --- |
| ≥ 0.95  | 39 %                                      | 98  |
| < 0.95  | 44 %                                      | 66  |

R² does not separate the partials the two estimators agree about from the ones
they do not. The confidence weighting is not protecting the decay term.

### 5d — the ring-time disagreement is not an estimator defect

The two estimators agree closely on **frequency** — median 3.7 ¢, p90 21.5 ¢
across 164 pairs, against a 115 ¢ gate — and, once the difference in normalising
partial is removed, on **level**. They disagree on **ring time**: median **+27 %**
with the fast one longer, p10 −34 %, p90 +149 %, longer in **71 %** of pairs.

The obvious explanation was that the fast estimator is filter-limited: the
envelope filter's cutoff is half the neighbour spacing, so two modes 18 Hz apart
are both read through a 10 Hz low-pass whose slower pole rings for 287 ms —
longer than the partials do.

**That explanation is wrong.** `TestCloseNeighboursDoNotBiasEitherRingTime` puts
exactly that configuration through both estimators with the answer known, and
both land within 3 %. The fit is dominated by the signal term, which stands well
above the filter transient until the −45 dB truncation has already ended it.

What is left is that the reference's partials are **not single decaying
exponentials**, which the two methods summarise differently and neither
incorrectly, plus the merged pairs of 5b. Neither is repaired by fixing an
estimator. §N2 says so rather than claiming a defect the measurement does not
support.

### 5e — ESTER's criterion is unusable on this signal

§N2 named ESTER (Badeau, David & Richard 2006) for order selection. It is
implemented, and it is reported rather than used, because its argmax is wrong by
a wide margin. On the 960–1358 Hz band of `v08` the criterion runs

```
33.7  32.0  20.9  9.3  9.9  15.7  16.7  25.5  21.5  25.4  19.8  11.2   (dB)
```

whose argmax is **order 1**, for a band in which the fast estimator alone finds
four partials. It is not near-unimodal with a wrong peak; it is not unimodal.
ESTER assumes a white noise subspace, and a struck drum's — attack layer, room,
residual glide — is not.

The order is chosen instead by a **stabilisation sweep**, the standard answer to
this problem in experimental modal analysis: every order is fitted, and a
component is kept only if it appears at several of them within 20 ¢ and 40 % of
ring time. ESTER's choice is carried in the report as `esterOrder` so the
disagreement is in the committed table rather than in this paragraph alone.

### What 5a–5e leave open

Frequency and level are trustworthy; the decay term is not, and the reason is a
property of the drum rather than of the code. The gates measured in Result 1 were
taken with the pre-5a estimator, so the takes where it collapsed contributed a
one-partial table to that measurement. **They have not been re-measured.**

## Result 6 — identifiability was never measured

The converged fits show the textbook sloppy-model signature (Gutenkunst et al.,
PLoS Comput. Biol. 3(10):e189, 2007): a search that reaches the same point from
diverse seeds, flat to four digits. There is **no Jacobian, Hessian or
Fisher-information code in the repository**, so how many of the ~17 free
parameters one hit actually constrains is unknown.

One flat direction is provable without running anything. Each mode's observed
amplitude goes as Φ(r_strike)·Φ(r_mic), which is **symmetric under exchange of
strike and pickup**, and for the axisymmetric modes the angles enter only through
their difference. So `(HIT.R, MIC.R)` is identifiable at best up to a swap and
`(HIT.A, MIC.A)` only through Δθ — four parameters carrying at least two exactly
flat directions, by construction.

A central-difference Hessian at the optimum costs ~600 evaluations against the
88 584 a full fit already spends. `PLAN.md` §P10/N6 carries this.

## What this changes

1. The adoption gates must be re-derived from measured reproducibility rather than
   from aspiration, and the partial-frequency, partial-decay and glide terms
   down-weighted or dropped until the estimator is repaired. RMS aggregation
   should be replaced with a robust statistic — every one of these terms is
   outlier-dominated.
2. The one actionable model defect is the **distribution of damping across modes**,
   and specifically the under-damped coupled (0,1) doublet.
3. Six rounds of intervention against the spectral envelope were aimed at a number
   whose composition had never been decomposed. Decomposing a metric before
   optimising against it is cheap; not doing so cost this project months.
