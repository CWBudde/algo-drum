# Validating the fitting objective against itself

Written **2026-08-01**. This document is the evidence record for one question that
had never been asked: **how much of what the fitting objective reports is real?**

It is deliberately narrow. The _conclusions_ drawn from these measurements live in
[`docs/physical-measured-fit.md`](physical-measured-fit.md) and in `PLAN.md` §P10;
this file holds the **method, the commands to reproduce it, and the raw numbers**,
so that the conclusions elsewhere can be checked rather than believed. Nothing
here should be restated at length elsewhere — link to it instead.

> **Which drum these numbers are from (2026-08-01).** Everything below Result 1
> was measured on `reference/tt08x08/mp/hd/`, the medium-pitch head strikes. The
> project's reference has since moved to the **low-pitch** set,
> `reference/tt08x08/lp/hd/`, chosen on the sound. Two things follow, and the
> distinction between them is the whole point of this document:
>
> - **The gate measurement has been redone on the new set** and the shipped gates
>   are the low-pitch ones. The comparison, and the finding that came out of it —
>   the glide term's floor improves by a factor of 120 on a drum whose fundamental
>   outlives the estimator's late probe — are in
>   [`reference/CREDITS.md`](../reference/CREDITS.md) and in
>   [`physical-measured-fit.md`](physical-measured-fit.md). It refutes this
>   document's standing assumption that the floor is a property of the estimator
>   alone; it is a property of estimator _and_ target.
> - **Results 5, 8 and 10 have now been re-measured** on the low-pitch set through
>   the post-N17 estimator — **Result 11**, which supersedes them and is what
>   should be quoted. **Results 2, 3 and 6 have not**, and cannot be without
>   rewriting tooling that was never committed; Result 11's last subsection says
>   which and why. Results 2 onward remain true statements about the medium-pitch
>   set with working reproduction commands. Re-running them is `PLAN.md` N16.
> - **Every result below, Result 10 included, was measured through the analysis
>   and decay windows that shipped before `PLAN.md` N17** — `analysisSeconds` 1.2
>   and `decayFitEndSeconds` 0.60. Those are now 2.0 and 1.60, and the estimator
>   they front has changed with them: a ring time must now be supported by a
>   20 dB fall inside its own window, and the decay refinement is bounded per
>   partial instead of spanning the whole window. This is a second, independent
>   reason not to quote a number here without re-running it.
>
>   **The gates are no longer among them.** `cmd/measure-objective` was re-run on
>   `reference/tt08x08/lp/hd/v*.wav` through the post-N17 estimator on
>   2026-08-01 and the gates in `DefaultWeights` are that measurement — partial
>   decay 0.6 → 0.55, glide 10 → 30, unmatched and spurious 0.3 → 0.25, the rest
>   unchanged. Read the comment on `DefaultWeights` for the four-column table; the
>   floor is now **6.32 median / 8.25 p90**, so **no total below 6.32 means
>   anything**. It was first recorded here as 6.54 / 7.86, from a run made minutes
>   before the gates were edited — right per term, wrong in the totals, which are a
>   property of the weight set. The p90 moves _up_ under the tighter gates for the
>   reason Result 7 already gave: a tighter gate is a heavier weight.
>
>   That re-run is also what found the one N17 defect nothing in N17 could see.
>   The per-partial refinement bound had no lower limit, so a fast partial's level
>   became unidentifiable, one bad fit re-based the whole level table, and the
>   glide floor went from 2.3 ¢ to 286 ¢ with four other terms following it.
>   `minimumRefinementSpanSeconds` in `internal/physical/match/decay.go` records
>   the measurement and the repair. The lesson generalises: **an estimator change
>   justified by a stability measurement has to be checked against a
>   reproducibility one**, because the two can move in opposite directions and
>   this one did.
>
>   Direction was reasoned about before it was measured, and Result 11 settles
>   both guesses: `f^-0.52` steepened to **`f^-0.70`**, as predicted; the
>   fast-versus-ESPRIT disagreement **widened** rather than narrowing, though on a
>   different drum, so the prediction is untested rather than refuted.

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
go run ./cmd/fit-physical -reference reference/tt08x08/mp/hd/v08.wav \
    -channel left  -report-only -o /tmp/target-left.json
go run ./cmd/fit-physical -reference reference/tt08x08/mp/hd/v08.wav \
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

### On a coincident pair (the strong result), medium pitch

Superseded as a gate source by the low-pitch measurement — see the note at the
top — but kept in full, because the three-way comparison in
[`reference/CREDITS.md`](../reference/CREDITS.md) is only readable against it.

`reference/tt08x08/mp/hd/v*.wav` is a **coincident** XY pair: peak inter-channel
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
`reference/tt08x08/mp/hd/v01.wav` … `v16.wav`, then evaluate the validated
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

> **This table is superseded. Read Result 7 instead.** Result 5a found that the
> estimator these gates were measured through was collapsing `v09`, `v10` and
> `v14` to one- or two-partial tables, which is what those takes contributed to
> the distribution above. The reciprocal-gate rule and two of the three findings
> stand; the numbers do not, and the re-measurement moved five of the nine gates.
> The table is kept because Result 7's "which repair did what" comparison is
> against it.

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
| **`tt08x08/mp/hd`, measured** | **1.584**   |
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

|                 | `tom.wav` (retired)                             | `tt08x08/mp/hd` (superseded)                         |
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
rather than by a single scalar. **That last sentence is withdrawn for the set the
project ships** — see Result 11e, where the low-pitch takes show a small positive
glide with no velocity trend.

Two practical constraints: the analysis window is 1.2 s and the medium-pitch files
are 1.25 s, so the higher tunings (down to 0.52 s) need the tail window shortened
before they are usable; and `v05`/`v06` are near-duplicate takes, agreeing to
seven decimal places through the analysis chain.

## Result 5 — the estimator, measured against a second one

> Superseded by **Result 11b/11c** on the low-pitch reference. 5a's defect and its
> fix are unaffected; 5b–5g's numbers are medium-pitch, and 5c's finding is not
> merely reproduced there but inverted.

Added 2026-08-01 for `PLAN.md` §N2, which required the fast estimator to be
compared partial by partial against a high-resolution one **before** anything is
refitted against either.

The second estimator is subband ESPRIT with a stabilisation sweep
(`internal/physical/match/esprit.go`), run over all sixteen medium-pitch
head-strike velocities, mono, with
`go run ./cmd/measure-tom -channel mono -high-resolution reference/tt08x08/mp/hd/v*.wav`.
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

### 5f — the decay term's confidence weighting does not work, and neither does its replacement

`partialDecayError` weighted each pair by the product of the two fits' R², on the
reasoning that a partial whose envelope is not an exponential has a meaningless
slope. The reasoning is right. R² does not implement it — §5c — and the reason it
cannot is that a slope drawn through a noise floor is perfectly straight, so the
statistic that measures straightness is blind to exactly the failure it was hired
to catch.

The obvious replacement is the dynamic range the slope was read over:
`Partial.DecayRangeDB`, how far the partial falls inside the fit window before the
fitted noise floor catches it. It was implemented and measured against the
subspace estimator on the same 153 pairs, and it does not discriminate either —
it is not even monotone:

| fitted decay range | pairs | median \|ΔT60\| |
| ------------------ | ----: | --------------: |
| 20–35 dB           |     5 |           5.0 % |
| 35–50 dB           |    33 |          93.4 % |
| 50–70 dB           |    17 |          42.1 % |
| ≥ 70 dB            |    98 |          36.9 % |

So the weighting is **gone rather than replaced**. An unmeasured confidence is
worse than no confidence, because it reads as a guard. What does the job instead
is trimming — see Result 7 — which needs no per-partial confidence estimate.

The field is still reported, because "how much evidence is there for this ring
time" is worth having in a committed table even when it does not predict this
particular disagreement.

### 5g — the Karjalainen decay model, implemented, and measured to change nothing here

Karjalainen, Antsalo, Mäkivirta, Peltonen & Välimäki (JAES 50(11):867–878, 2002)
fit an explicit exponential-plus-noise-floor model rather than truncating before
the floor arrives. It is now what `measureDecays` reports
(`decayFloorFit`): three parameters held as logarithms, Levenberg–Marquardt from
the log-linear estimate, over the whole window rather than the top 45 dB of it.

On its own model it is exact where truncation is not. A partial standing 30 dB
above its own noise floor cannot show 45 dB of decay, so the truncation never
fires and the straight line is drawn through a trace whose last third has already
flattened: the log-linear reading comes back **+930 %**, the floor model within
0.1 %. That is an ordinary configuration, not a pathological one, and it is pinned
by `TestTruncationReadsARingTimeLongAndTheFloorModelDoesNot`.

**On the licensed reference it changes nothing measurable.** Against the subspace
estimator, over the 108 pairings both estimators produce:

|                        | median \|ΔT60\| |    p75 |     p90 |
| ---------------------- | --------------: | -----: | ------: |
| truncated log-linear   |          37.8 % | 78.2 % | 181.7 % |
| exponential plus floor |          41.4 % | 79.3 % | 176.3 % |

and the refinement lands closer to the subspace estimate on **53 of 108** pairings
— a coin flip. The one thing that did move in the predicted direction is the long
bias: the fast estimator reads longer than the subspace one in 63 % of pairs
rather than 71 %.

The explanation is checkable and is not a defence of the change. These traces
mostly never reach a floor inside the fit window — the median fitted range is
86 dB — so the term the model adds is inactive on most of them, and where it is
inactive the two estimators differ only in how much of a _non_-exponential trace
they fit a straight line through. Which is §5d again, arriving from a third
direction.

It is kept because it is right where the old one is wrong, and because it removes
a threshold whose correctness depended on the signal being measured. It is not
kept because it improved this reference, and it did not.

### What 5a–5g leave open

Frequency and level are trustworthy; the decay term is not, and the reason is a
property of the drum rather than of the code — three independent measurements now
say so. What the estimator work does not do is make the ring time of one partial
of this drum a well-defined quantity.

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

## Result 7 — the gates, re-measured after the repair

Result 1's gates were taken through the pre-5a estimator, so `v09`, `v10` and
`v14` contributed one- or two-partial tables to them. This is the same
measurement after the repair, and after two changes to the objective itself: the
decay term's confidence weighting removed (§5f), and the three partial terms
aggregated by a **trimmed** RMS over the smallest 80 % of their squared errors
rather than a plain one.

The measurement is now repository code — `cmd/measure-objective` — and calls the
real `match.Distance`. Result 1's method note describes a standalone
reimplementation kept outside the tree and trustworthy only while it reproduced
`distance.go` bit-exactly; that is a standing invitation to silent drift, and the
drift would have been invisible in exactly the way 5a was. The tool also refuses a
pair whose channels are not coincident, which the old procedure relied on the
operator to check.

All sixteen files: inter-channel delay **0 samples**, correlation 0.85–0.93.

| Term              | 2026-08-01 p90 | re-measured p90 | gate now |
| ----------------- | -------------: | --------------: | -------: |
| Partial frequency |        113.0 ¢ |      **76.2 ¢** |     80 ¢ |
| Partial level     |       17.85 dB |     **6.81 dB** |     7 dB |
| Partial decay     |          1.262 |       **0.558** |      0.6 |
| Spectral envelope |        3.65 dB |         3.67 dB |     4 dB |
| Envelope          |        3.81 dB |         3.84 dB |     4 dB |
| Glide             |        310.3 ¢ |         280.1 ¢ |    290 ¢ |
| Attack balance    |        1.12 dB |         1.13 dB |   1.2 dB |
| Unmatched share   |          0.880 |       **0.250** |      0.3 |
| Spurious share    |          0.346 |       **0.245** |  0.3 (†) |

(†) Spurious keeps Unmatched's gate. Its measured floor would make it marginally
the heavier of the two, and that direction has already been refuted by a fit run
that abandoned the drum for two partials. The margin is now small enough that it
would probably not reproduce, which is not a reason to find out.

Gates are rounded **up** from the p90: a gate is what a candidate has to beat, and
rounding a measured floor down publishes a threshold below the floor.

### Which repair did what

Re-running the campaign with the trimming disabled separates the two causes
instead of asserting a split:

| Term              | 2026-08-01 | repaired only | repaired + trimmed |
| ----------------- | ---------: | ------------: | -----------------: |
| Partial frequency |    113.0 ¢ |       112.4 ¢ |         **76.2 ¢** |
| Partial level     |   17.85 dB |   **7.24 dB** |            6.81 dB |
| Partial decay     |      1.262 |     **0.608** |              0.558 |
| Unmatched share   |      0.880 |     **0.250** |              0.250 |
| Spurious share    |      0.346 |     **0.245** |              0.245 |

The estimator repair fixed level, decay, unmatched and spurious — those four were
measuring the collapsed takes and almost nothing else. The trimming fixed
frequency, which the repair did not touch at all. Two different defects; neither
substitutes for the other.

### What survives from Result 1, and what does not

- **The spectral envelope is still the only gate that was ever right** — 3.67 dB
  against a gate of 4, unmoved by either repair. Note _why_ it is unmoved: it was
  never the term the defects were in. Every conclusion drawn from it stands.
- **Attack balance is still the most reproducible term in the objective**, 0.245 dB
  at the median, better than the spectral envelope.
- **Glide is still broken**: median 50.0 ¢ against an `unreadableGlideCents` of 40,
  so more than half the pairs still cannot place two probes on a surviving
  fundamental. Repairing the decay estimator did not repair this, which is
  evidence the two are unrelated.
- **"The partial terms were never gateable" is withdrawn.** It was true of the
  collapsed measurement — nothing could beat 113 cents or a 1.26 log-ratio. It is
  not true of this one. 80 cents is still not a fine tolerance and 7 dB of partial
  balance is still a wide one, but they are thresholds a model can be held to. What
  stands is the narrower claim: the six rounds of intervention aimed at the old
  25-cent and 0.25 gates were aimed at thresholds nothing could reach.

### On totals

At these weights the objective's disagreement with itself totals **5.73 median,
6.38 p90**. That is a larger number than the 4.32/6.46 the previous gates
produced, and it is not a regression: tightening a gate raises its weight, so the
same raw disagreement scores higher. Scored under the previous weights this
distribution totals 3.74/4.29, which is the like-for-like comparison.

Which is the difficulty with totals in one paragraph — not even the sign of a
change across a re-weighting is meaningful. The readable quantity is the per-term
contribution, and there the weights' claim holds: no term contributes more than
**0.81** at its own median, against the 1.0 that means "at the gate".

**No total recorded before this change is comparable to any after it.** That was
true of the previous change too. `cmd/measure-objective` writes the floor into its
own report so a total always arrives beside the floor it should be read against.

> Superseded in its measured numbers by **Result 11d**, which confirms both halves
> of the sign-pattern check on the low-pitch set. The structural argument below —
> that the longest-ringing mode is forced to be the lowest uncorrected one — is a
> property of the model and does not depend on either reference.

## Result 8 — N3's damping defect is at a different mode than N3 says

PLAN item N3 names one instance: "the model synthesizes a mode at 186 Hz with
T60 1.81 s, the longest-ringing thing it produces", and prescribes damping "the
coupled (0,1) doublet specifically". **Those are two different modes.**

The 186 Hz mode is the **batter head's (1,1)**, at the config in
`testdata/physical-fit-tom.json` where the fitted DAMP and D.TILT scale the loss
law down by about 2.6× on its k¹ term. It is not a doublet member, it is not
axisymmetric, and it does not couple to the cavity at all through the compliance
that makes a doublet: the swept area of every m > 0 mode is exactly zero, so the
(1,1)'s only cavity path is to a transverse mode at 907 Hz, five times above it.

Its budget, at that config:

| term       | value |      share |
| ---------- | ----: | ---------: |
| `Loss1`·k  | 3.329 | **86.7 %** |
| `Loss0`    | 0.490 |     12.8 % |
| radiation  | 0.014 |      0.4 % |
| `Loss2`·k² | 0.005 |      0.1 % |
| correction | 0.000 |        0 % |

And it is not an accident of the fit. γ is monotone in k with exactly one
exception, the (0,1) correction, so **the longest-ringing mode is forced to be the
lowest-wavenumber mode the correction table does not name** — the (1,1), at every
tuning and every value of DAMP and D.TILT, both of which scale the whole law.
At shipped defaults it is the (1,1) at 238.7 Hz, T60 587 ms, against the (0,1)'s
213 ms. `TestTheLongestRingingModeIsTheLowestUncorrectedOne` pins it.

So the prescribed fix cannot reach the named defect. The (0,1) is already the most
heavily damped mode in the bank — its correction, 24.6 /s shipped, is more than
twice its structural rate — and damping it further moves the (1,1) not at all.

### The sign-pattern check N3 requires, run

N3 says: before fitting any per-mode damping vector, check the sign pattern; if it
reproduces the predicted pairwise alternation it is physics, if it is structureless
it is fitted noise. That check is now possible, because the subspace estimator
resolves the close pairs the fast one merges. Fourteen two-member pairs across the
sixteen velocities:

- **The pairwise structure is real and large.** Within a resolved pair the two
  members' ring times differ by a median factor of **1.55**, minimum 1.11, maximum
  7.25 — 0.222 s against 1.006 s at 587 Hz, 0.169 s against 1.226 s at 851 Hz. The
  members sit 3–22 Hz apart, a 1–6 % split, across which any smooth γ(k) gives a
  ratio of essentially 1.00. **No smooth loss law can express this**, which is the
  half of N3's second thread that the evidence supports.
- **The predicted sign is not there.** A cavity doublet's squeezing member is the
  upper branch and should always be the more damped one. The upper member decays
  faster in **6 of 13** pairs. That is a coin flip, and it is the answer N3's own
  test asks for: on this evidence the alternation is not the in-phase/out-of-phase
  signature.

One limitation, stated because it bounds the conclusion rather than rescuing it:
every resolved pair sits between 300 Hz and 2.7 kHz. The cavity doublet lives at
the (0,1), and no pair was resolved there — the low band shows 155, 232–235, 289
and 305 Hz components whose pairing is ambiguous. So this refutes the squeezing/
sliding hypothesis **for the pairs that were measurable**, and is silent about the
fundamental. What it establishes positively is that the model is missing pairwise
damping freedom across the whole retained band, not only at m = 0 — which is a
larger gap than N3 describes and points away from the cavity as its cause.

## Result 9 — the 1.55 is the drum, not the estimator

Result 8 left the decisive question open. A median ratio of 1.55 between the ring
times of two modes 1–6 % apart is either a real per-pair damping freedom the model
lacks, or the artefact of a subspace estimator trading energy between two
components it can barely separate. Those readings call for opposite work — one
says fit a per-pair damping split, the other says fix the measurement — and
nothing in the reference distinguishes them, because there the truth is unknown.

So it was measured against a signal where the truth is known by construction.
Pairs were synthesised at the four frequencies and the splits the resolved pairs
actually had (304, 613, 1200 and 2700 Hz; 1 %, 2 %, 4 % and 6 %), the upper member
0, 3 and 6 dB down since an off-centre strike does not excite a pair equally, over
a −60 dB noise floor, and **both members given exactly the same decay**. Any ratio
reported is manufactured.

|                     | equal damping | true ratio 1.55 |
| ------------------- | ------------- | --------------- |
| mean reported ratio | **1.001**     | 1.550           |
| worst cell          | **1.003**     | 1.551           |
| cells resolved      | 39 of 48      | 12 of 12        |

Both halves are needed and neither alone would settle it. The first says the
estimator does not invent a split: against a measured 1.55 the worst it
manufactures anywhere in the regime is 1.003. The second says it is not merely
insensitive — an estimator that always returned equal ring times would pass the
first test perfectly — and recovers a real 1.55 to better than a fifth of a per
cent at every frequency and every split.

The nine unresolved cells are all at a 1 % split, and they matter: where the
estimator cannot separate the pair it **merges**, reporting one value twice, and
does not fabricate two. That is the conservative failure, and it is what keeps the
control meaningful — an estimator that split noise into two components when it ran
out of resolution would have produced the ratio being tested for.

`TestEqualDampingIsNotSplitByTheEstimator` and
`TestARealDampingSplitIsRecoveredAtItsMeasuredSize` pin both directions.

**What this settles.** N3's first thread is answered: the pairwise damping split
is a property of the drum. The model is missing a real freedom, it spans the
retained band rather than only the m = 0 modes the cavity can reach, and — from
Result 8 — it does not carry the cavity's in-phase/out-of-phase sign. A per-pair
damping split with no predicted sign is now the shape the evidence supports, and
it is measurement-backed rather than conjectural.

It also settles N15, which was blocked on exactly this. The two members of a pair
differ in **damping**, and `ASYM` splits only **frequency**. Whatever is done about
the fast estimator's merging, `ASYM` is not the parameter that would represent
this, so fitting it against a target with the asymmetry averaged out is not worth
repairing the target for.

A third measurement, taken for another purpose, agrees. The excitation-gap sweep
([`physical-excitation-gap.md`](physical-excitation-gap.md)) drives `ASYM` from its
default to 1 and the spectral envelope moves 13.02 → **13.12** dB, against
13.02 → 16.18 for the loss-law tilt in the same table. The objective is not merely
mismeasuring `ASYM`; it barely responds to it at all.

`ASYM` is now held out of the search at its default, and a report marks it `blind`
as well as `fixed` so that its value cannot be read as a fitted result. It remains
a user knob — it is audible, and a player setting it is making no claim about a
recording. `-search-blind` puts it back for the deliberate experiment of
re-testing this, which is the only way the claim above stays revisable.

## Result 10 — the reference's ring time falls as f^-0.52, not as 1/f

> Measured through the pre-N17 windows and superseded by **Result 11a**, which
> re-reads the same set at `f^-0.70`. Kept because its window study is what
> established N17 and because the size of the correction is only legible against
> it.

Results 8 and 9 established a defect in the model's damping _distribution_ and
that its pairwise part is real. Neither says what shape the reference actually
wants, and the model's loss law has been calibrated to a stated one since P2:
"measured membrane behaviour is \(T_{60} \propto 1/f\)", i.e. constant
\(\zeta\), which is why \(d_1\) dominates and \(d_0\) is held at a small
floor ([`physical-calibration.md`](physical-calibration.md)). That premise is now
measurable against the committed reference, and it does not survive.

Measured on **`tt08x08/lp/hd`**, the set that became the reference on 2026-08-01:

```bash
go run ./cmd/measure-tom -channel mono -analysis-seconds 2.0 \
    reference/tt08x08/lp/hd/v*.wav
```

16 takes, 256 partials, at the shipped 0.05-0.6 s decay window. Third-octave
medians, with the median fit quality beside each because it is what disqualifies
one of the bands outright:

| band (Hz) |   n | median T60 (s) | median R2 |
| --------- | --: | -------------: | --------: |
| 227-286   |  34 |          0.686 |      0.92 |
| 286-360   |  17 |      _(4.688)_ |      0.77 |
| 360-454   |   8 |          0.348 |      0.86 |
| 454-571   |  19 |          0.714 |      0.95 |
| 571-720   |  11 |          0.318 |      0.95 |
| 720-907   |  15 |          0.764 |      0.94 |
| 907-1143  |  30 |          0.394 |      0.97 |
| 1143-1440 |  32 |          0.400 |      0.96 |
| 1440-1814 |  37 |          0.292 |      0.96 |
| 1814-2286 |  23 |          0.257 |      0.96 |
| 2286-2880 |  27 |          0.208 |      0.95 |

Fitting a power law to the 209 partials with R2 >= 0.90 and a ring time the file
could actually contain:

\[
T_{60} \propto f^{-0.52}.
\]

**Constant \(\zeta\) is \(f^{-1}\) and a flat \(\gamma\) is \(f^{0}\).**
The reference sits almost exactly halfway between them, in log slope. Anchored at
the 240 Hz measurement, the constant-\(\zeta\) law the model is calibrated to
predicts 64 ms at 2.6 kHz where the reference gives **208 ms**, too short by 3.3x.
The law is not wrong in kind — it is roughly twice too steep.

The mode identification is not in doubt, because the geometry is known: the
fundamental is 239.9 Hz and the \((1,1)\) is 378.8 Hz, a ratio of **1.579**
against the ideal 1.594.

### The window, checked before the conclusion, and what it found

A decay estimate can be manufactured by the window it is fitted over, so this was
checked rather than assumed. Every partial was re-measured at four other window
ends and matched to its own 0.6 s estimate by frequency, within 0.5 %:

| window end | matched pairs | median dT60 | below 1 kHz | above 1 kHz |
| ---------- | ------------: | ----------: | ----------: | ----------: |
| 0.30 s     |           148 |      15.4 % |      29.2 % |       7.5 % |
| 0.45 s     |           202 |       6.0 % |      11.9 % |       1.8 % |
| 0.90 s     |           200 |       3.1 % |      18.9 % |       0.9 % |
| 1.30 s     |           186 |       4.6 % |      25.6 % |       1.0 % |

Above 1 kHz the shipped window is settled: extending it to 1.3 s moves the
estimate by 1 %. **Below 1 kHz it is not** — 19 % at 0.9 s and 26 % at 1.3 s, and
moving _away_ from the shipped value in both directions. That is the window
failing to span the decay, and it is a property of the new reference rather than
of the estimator: this drum's fundamental rings for 0.686 s, so the 0.6 s window
does not reach \(T_{60}\) at all, where the medium-pitch set it replaced rang for
0.28 s and comfortably did.

Two numbers size that:

- **70 of 256 partials (27.3 %) are assigned a ring time longer than the 0.6 s
  window they were fitted in.**
- **11 (4.3 %) are assigned one longer than the 2.08 s file** — up to 10.4 s. All
  eleven are the same partial near 358 Hz seen across takes, at R2 0.12-0.66, and
  they are what makes the 286-360 Hz row above unusable. A ring time longer than
  the recording is not a measurement, and N2's guard does not catch it: that guard
  bounds a fit against the _envelope filter's_ fastest pole, which is the opposite
  failure.

So the low band's numbers are the ones the conclusion leans on and the ones the
window serves worst. The power-law slope is quoted over partials the file can
hold, and the direction of the residual error is known: a window that cannot
reach \(T_{60}\) truncates long decays, so the true low-frequency ring times are
if anything _longer_ than the table says, and the true slope **steeper** than
\(-0.52\) — that is, further from constant \(\gamma\) and closer to, but on
this evidence not at, constant \(\zeta\).

### What it means for N3

The lever is already in the bank and it is not a correction-table entry. `D.TILT`
scales \(d_1\) and \(d_2\) and leaves \(d_0\) alone, so the
\(d_0\):\(d_1\) balance — the whole difference between a constant-\(\gamma\)
law and a constant-\(\zeta\) one — is exactly what it moves, over a 0-3 range
whose zero is the flat law and whose 1 is the calibrated constant-Q one. Result
8's structural finding follows directly from which end of that range the model
ships at: with \(d_1\) dominant \(\gamma \propto k\), so the lowest
uncorrected mode necessarily rings longest, and the \((1,1)\) is that mode.

An \(f^{-0.52}\) target puts the answer **between the two ends, not at either**,
which is the useful part: it says the fit should land `D.TILT` well below 1 and
not at its stop, and that a bank which pins it at 0 is reporting a measurement
artefact rather than this drum. A correction entry at the \((1,1)\) would be
patching one mode of a law whose slope is the thing the reference disagrees with.

The measurement that settles it is a fit against this reference, read for where
`D.TILT` lands and whether it pins. That run is what N3 now waits on, and it
should not be started before the decay window is re-sized: 27 % of the target's
own partials are currently fitted over a window shorter than their ring time, and
the fit would be matching the model to that.

### Superseded: the same measurement on `tt08x08/mp/hd`

Made first, on the set that was the reference until 2026-08-01, and kept because
it is what prompted the window checks. There the ring time is **flat** — 0.280 s
at 352 Hz against 0.256 s at 2.9 kHz, an exponent near zero, against which
constant \(\zeta\) is wrong by 7.5x. The 0.6 s window is ample for that drum
(2.2 % on extension to 0.9 s), so the flatness is not a window artefact of the
kind found above. Two drums from one pack, one tuning apart, give exponents of
roughly 0.0 and -0.52; **neither supports \(1/f\)**, which is the claim under
test, and the disagreement between them is itself a caution against
re-calibrating a shipped law on a single recording.

## Result 11 — Results 5, 8 and 10 re-measured on the low-pitch reference

Added **2026-08-01** for `PLAN.md` N16, and it is the first pass that measures
the current target through the current estimator. One command produces all of it:

```bash
go run ./cmd/measure-tom -channel mono -high-resolution \
    -o lp-hd-hires.json reference/tt08x08/lp/hd/v*.wav
```

at the shipped defaults — analysis window 2.0 s, decay fit 0.05–1.6 s,
`minimumRefinementSpanSeconds` in force. 16 takes, 256 fast partials, 111 of them
paired against the subspace estimator. Every number below Result 10 in this
document is superseded by the corresponding number here, except where it is
explicitly about the medium-pitch set.

### 11a — the ring time falls as f^-0.70, and the window was the reason it read -0.52

| band (Hz) |   n | median T60 (s) | median R2 |
| --------- | --: | -------------: | --------: |
| 227-286   |  22 |      **1.094** |      0.97 |
| 286-360   |  15 |      _(2.449)_ |      0.97 |
| 360-454   |   5 |          0.445 |      0.86 |
| 454-571   |  18 |          0.703 |      0.96 |
| 571-720   |   4 |          0.654 |      0.97 |
| 720-907   |  15 |          0.830 |      0.95 |
| 907-1143  |  34 |          0.436 |      0.96 |
| 1143-1440 |  43 |          0.415 |      0.96 |
| 1440-1814 |  50 |          0.307 |      0.96 |
| 1814-2286 |  24 |          0.258 |      0.95 |
| 2286-2880 |  24 |          0.207 |      0.93 |

Fitting the same power law as Result 10 — the 221 partials with R2 >= 0.90 and a
ring time the file could hold — gives

\[
T_{60} \propto f^{-0.70},
\]

against the \(f^{-0.52}\) the 0.6 s window produced. **The direction predicted
in this document's header is confirmed and the size of it was not guessable**:
truncation was costing 0.18 in the exponent, a third of the distance between the
measured law and the constant-\(\zeta\) one the model is calibrated to. Anchored
at the 240 Hz band the fitted law predicts 0.207 s at 2.58 kHz where the
measurement gives 0.207 s; constant \(\zeta\) predicts 0.102 s, **too short by
2.0x** rather than Result 10's 3.3x.

So the conclusion survives in kind and shrinks in size. \(1/f\) is still not what
this drum does, but it is half as wrong as the truncating window made it look, and
the useful consequence for N3 is unchanged and now better sized: `D.TILT` should
land clearly below 1 and clearly above 0.

The fundamental is where the truncation was worst. Its ring time reads **1.076 s
mean, SD 3.1 %** across the sixteen takes, against the 0.686 s Result 10 recorded
— the 0.6 s window was cutting 36 % off the single partial the whole fit is
anchored to.

**The 286-360 Hz row is still unusable for the power law, and for the opposite
reason to before.** Twelve of 256 partials (4.7 %) are assigned a ring time longer
than the 2.08 s file, and all twelve are the same component at **357-360 Hz**, one
per take, in twelve of the sixteen takes. Result 10 recorded it at R2 0.12-0.66
and read it as a failed fit. Through the wider window and the floor model it comes
back at **R2 0.95-0.99 with T60 2.3-2.6 s**, close to but not at the \((1,1)\) at
378.8 Hz.

**A ring time longer than the file is not by itself a defect, and the first draft
of this subsection said it was.** Every one of the twelve falls **35-40 dB inside
the fit window** — a T35 to T40, roughly twice the evidence ISO 3382 asks of a
reported T60, and more than most partials in this reference have. Extrapolating a
T60 from that is the ordinary arithmetic of a decay measurement, not a fabrication;
it is how a room's own RT60 is measured. `slowestSupportedT60` already implements
the correct criterion — evidence is the **fall**, not the duration — and it admits
these twelve deliberately. Attempting to bound a ring time against the file's
length instead was tried, measured, and refuted, twice: once in N17 and once again
on 2026-08-01 when the same idea was re-proposed from this row. The second attempt
was scored through `cmd/measure-objective`, and the only term it moved was partial
decay, **worse** — p90 0.535 → 0.537.

So what is excluded, and why, is narrower than "unmeasurable". The component is
well measured; it is a _long_ one, longer than this material can bound tightly,
so the band is left out of the power-law fit where a 2.5 s value would carry the
low end. Whether the **model** should be asked to reproduce it is a separate and
still-open question — it is 12-22 dB below the fundamental, it is not at a mode
frequency the geometry predicts, and a fit will spend real parameter budget on it
(one did: see the joint-fit note in `PLAN.md` N5). That question is about what
belongs in a target, not about whether the estimator can read it.

### 11b — the fast-versus-subspace disagreement got wider, not narrower

The header predicted this would narrow, since part of it was the fast estimator
fitting past the end of its partials. **It did not.**

|                       | `mp/hd`, pre-N17 | `lp/hd`, current |
| --------------------- | ---------------: | ---------------: |
| paired partials       |              108 |          **111** |
| frequency, median     |            3.7 ¢ |        **8.9 ¢** |
| frequency, p90        |           21.5 ¢ |       **48.8 ¢** |
| ring time, median     |           41.4 % |       **62.6 %** |
| ring time, p90        |          176.3 % |      **317.7 %** |
| fast reads longer, in |             63 % |         **80 %** |

Both the estimator and the target changed, so this is not a controlled comparison
and the prediction is not cleanly refuted — it is untested, by a move that made a
controlled test impossible. What can be said is that the disagreement is not small
on the drum the fit now aims at, and that frequency agreement, while four times
worse than on the medium-pitch set, is still far inside the 120 ¢ match tolerance.

§5c re-run on this set does more than fail again — it **inverts**:

| fast R2 | median \|ΔT60\| vs the subspace estimator |   n |
| ------- | ----------------------------------------- | --: |
| ≥ 0.95  | **85 %**                                  |  70 |
| < 0.95  | **50 %**                                  |  41 |

The partials the fast estimator is most confident about are the ones it agrees
with the subspace estimator about least. §5f removed that weighting on the
evidence that it did not discriminate; on this set keeping it would have been
actively harmful.

### 11c — the fundamental is a cluster, and the subspace estimator is not ground truth there

The single most consequential disagreement is at the partial everything else is
normalised to. Matching each take's base partial to its subspace counterpart
within 15 Hz:

|                       | fast          | subspace          |
| --------------------- | ------------- | ----------------- |
| median T60            | **1.078 s**   | **0.413 s**       |
| range across 15 takes | 1.001–1.128 s | **0.174–1.589 s** |
| spread                | **±6 %**      | **9.1x**          |

Fifteen nominally identical strikes on one drum, and the high-resolution
estimator's answer for the fundamental's ring time moves by a factor of nine while
the fast one moves by six per cent. The matched subspace component is also often
not at 239.7 Hz but at 232, 243 or 246 Hz — the low band holds several components
inside 15 Hz and the subspace estimator resolves a different member of the cluster
on each take, where the fast one reads the composite.

This does not say the fast estimator is right. It says the presumption running
through Result 5 — that where the two disagree, the high-resolution one is the
measurement and the fast one the approximation — **does not hold at this drum's
fundamental**, which is where it would matter most. Result 9's control does not
cover this case either: it synthesised _two_-component cells, and what is here is
three or more inside the resolution limit.

Both readings stay open. Either the low cluster's energy really does redistribute
drastically strike to strike, or the subspace estimator is not identifying the
same component twice. The one thing that is settled is that no repair to the fast
estimator can be justified by a disagreement with a reference that is itself
unrepeatable at the mode in question.

### 11d — the pairwise damping split is confirmed on this drum, at 1.39

Result 8's pairing was done by hand on the medium-pitch set. Re-run on the
low-pitch subspace tables with a stated criterion — adjacent components 1–25 Hz
apart, split ≤ 7 %, above 200 Hz, with no third component within 25 Hz on either
side, so that only clean two-member pairs are counted:

|                        | `mp/hd`, Result 8 | `lp/hd`, current |
| ---------------------- | ----------------: | ---------------: |
| pairs                  |                14 |           **29** |
| median ring-time ratio |          **1.55** |         **1.39** |
| range                  |         1.11–7.25 |        1.04–4.86 |
| upper member faster in |    6 of 13 (46 %) |  18 of 29 (62 %) |

**Both halves of Result 8 hold on the new reference.** The split is large — a
median factor of 1.39 between members 0.1–6 % apart in frequency, across which any
smooth \(\gamma(k)\) gives essentially 1.00 — and the cavity's predicted sign is
still not there. 62 % is closer to a preference than 46 % was, but with 29 pairs
it is two pairs away from a coin flip and is not evidence of the alternation.

Result 8 was explicitly silent about the fundamental, no pair having been resolved
there. Pairs now are — 232/239 Hz at ratio 4.39, 239/243 Hz at 2.18, 304/322 Hz at
2.90 — but by 11c those are exactly the components the subspace estimator does not
resolve repeatably, so they are excluded from the summary above rather than read as
the doublet finally appearing. The conclusion is still the one Result 8 reached
from the modes between 300 Hz and 2.7 kHz.

### 11e — the glide is gone, and with it Result 4's velocity-series argument

Result 4's most valuable claimed property of the reference pack was that measured
glide rises monotonically with strike velocity — −130 ¢ at `v04`, −174 ¢ at `v08`,
−353 ¢ at `v12` — "so the Berger nonlinearity is finally constrained by a curve
rather than by a single scalar". That is a property of `tt08x08/mp/hd`. **On
`lp/hd` it is not there:**

| take  |  v01 |   v04 |   v08 | v12 |    v15 |   v16 |
| ----- | ---: | ----: | ----: | --: | -----: | ----: |
| glide | +2 ¢ | +16 ¢ | +30 ¢ | n/a | +133 ¢ | +40 ¢ |

All sixteen readings are **positive** — the pitch rises slightly rather than
falling — three (`v10`, `v11`, `v12`) are not measurable at all, the median is
**+18 ¢**, and there is no monotone trend with the file order. Fifteen of the
sixteen sit at or below `unreadableGlideCents` = 40, which is to say at or below
the level this estimator declines to interpret.

Three consequences, and the third is the one that costs something:

- The measured glide gate of **30 ¢** (`DefaultWeights`) is the same size as the
  entire spread of the measurement. The term is reproducible on this drum because
  there is almost nothing there to reproduce.
- A fit against this reference cannot be read as constraining the Berger
  nonlinearity in either direction. `BERGER` is fitted against a target whose
  glide is at the noise, so whatever it lands on is not a measurement of tension
  nonlinearity.
- **N5's velocity-series joint fit loses its stated justification.** Its argument
  was the glide curve. On the set the project actually ships that curve does not
  exist, and the remaining case for a series fit — one bank against many takes,
  contact and nonlinearity not trading against a single assumed strike — is a
  weaker and different one. It is still worth doing; it is no longer worth doing
  _for this reason_.

The medium-pitch set does have the curve, and it is the same drum one tuning
apart. Nothing here says the nonlinearity is unmeasurable — it says it is not
measurable from the recording the fit is aimed at.

### What Results 2, 3 and 6 still need

Not re-run, and the reason is recorded rather than deferred silently:

- **Results 2 and 3** rest on experiments — the four-window residual budget, the
  band-limiting sweep, the static-EQ fits at 2/3/5/7/24 free parameters — whose
  code was never committed. Re-running them means writing that tooling again.
  Their qualitative claims (the residual is time-varying; a static post-filter
  cannot reach it) are about the model rather than the target and are more likely
  to transfer than their numbers, but neither has been checked here.
- **Result 6** was never a measurement on any set: no Jacobian, Hessian or Fisher
  information exists in the repository. It is `PLAN.md` N6 and is untouched by the
  change of reference.

## What this changes

1. ~~The adoption gates must be re-derived from measured reproducibility rather
   than from aspiration~~ — **done twice**, Result 1 and then Result 7 after the
   estimator repair invalidated it. RMS aggregation is now trimmed in the three
   partial terms, which is what fixed the frequency gate.
2. The one actionable model defect is the **distribution of damping across modes**
   — but ~~specifically the under-damped coupled (0,1) doublet~~ is wrong, and
   Result 8 replaces it. The (0,1) is the most heavily damped mode in the bank.
   What is missing is **pairwise** damping freedom across the whole retained band,
   measured at a median factor of 1.55 between members of a resolved pair, and it
   does not carry the cavity's in-phase/out-of-phase signature. Result 9 closes the
   remaining escape route: on synthesised pairs with identical damping the
   estimator reports 1.001, so the 1.55 is the instrument being modelled and not
   the instrument measuring it.
3. Six rounds of intervention against the spectral envelope were aimed at a number
   whose composition had never been decomposed. Decomposing a metric before
   optimising against it is cheap; not doing so cost this project months.
4. Two of this document's own conclusions have now been withdrawn on later
   evidence — "the partial terms were never gateable" (Result 7) and the (0,1)
   doublet as N3's defect (Result 8). Both were stated with more confidence than
   the measurement behind them supported. The pattern is worth naming, because
   this path's recurring failure is not measuring too little but concluding too
   much from what was measured.
5. **A number is a property of the estimator, the target and the window
   together.** Result 11 is the third demonstration in this file: the same code on
   two tunings of one drum gives decay exponents of 0.0 and −0.70, glide floors of
   280 ¢ and 2 ¢, and a velocity-glide curve that exists on one and not the other;
   and the same code on one tuning through two windows gives −0.52 and −0.70.
   Nothing here transfers by default, including the parts of it that read like
   statements about physics.
6. The subspace estimator is measurement equipment with a measured limit of its
   own (Result 11c): at this drum's fundamental its ring time moves by 9x across
   nominally identical strikes where the fast estimator's moves by 6 %. It cannot
   be used as the reference against which the fast one is corrected there.
