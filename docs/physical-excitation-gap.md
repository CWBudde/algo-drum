# The excitation-spectrum gap

This document used to be a study of a band deficit measured against
`reference/tom.wav`. That recording is being retired — unknown provenance,
unlicensed, 44.1 kHz, spaced stereo pair; see `PLAN.md` §"P10" item N8 and
`reference/CREDITS.md` for the licensed replacement. Everything that counted or
compared partials in that recording has been **deleted** rather than annotated.

What survives is what was measured against the model itself, plus the analytic
argument about the contact force, and that turns out to be the useful half.

## Where this now sits

The residual this document was chasing has since been measured a second way, and
the framing changed. The method and the raw evidence are in
[objective validation](physical-objective-validation.md); `PLAN.md` §"P10 — The
objective, the reference, and what is actually broken" carries the plan. Four
results from there bear directly on this document, and none of them is restated
here:

- The residual **budgets**, and the actionable part of it is time-varying — the
  spectrum evolves wrongly, flat at onset and diverging monotonically with time.
  That is a **damping-distribution** problem, not an excitation one.
- **A static observation or post-filter cannot fix the envelope term.** This is
  the sharpest constraint on the present document: the deficit below is a
  missing-excitation story with an observation-side alternative, and the
  falsification test forecloses the observation side outright — even a fully
  free per-band EQ, the absolute limit of any static filter, falls well short of
  the criterion.
- **Band-limiting kills the "it all lives above the top retained mode" reading.**
  Restricting to the range where the model has full modal content barely moves
  the term, so the seam described below is real but small.
- Most of the terms this document's era scored against were **not reproducible
  measurements at all** — the objective disagrees with itself on a coincident
  pair by more than the adoption gates allow. Only the spectral-envelope term
  survived as a reliable measurement.

So the excitation defect described below is real and structural, but it is not
where the remaining error lives. Read this document for what the contact force
provably cannot do, not for a ranking of work.

Throughout, **476–700 Hz** is used as a fixed band label. It was originally
picked from the retired recording's partial list; it is retained only so the
model-internal measurements below stay comparable to each other.

## The mode list is not the constraint

Model-internal. At High quality the batter bank puts 58 modes into the band:

| Quality  | Modes | Highest batter mode | Modes in 476–700 Hz |
| -------- | ----- | ------------------- | ------------------- |
| Draft    | 48    | 467.6 Hz            | 0                   |
| Standard | 96    | 665.1 Hz            | 45                  |
| High     | 160   | 852.5 Hz            | 58                  |

Fifty-eight oscillators sit in the band and are inaudible. Tripling the mode
count does not move the spectral-envelope term meaningfully. Whatever is wrong,
it is not that the model lacks oscillators there.

## What it is not

Each row changes one mechanism away from the fitted bank at High quality and
re-measures.

> The spectral-envelope column is a score against the retired recording. It is
> kept because these rows are **one-at-a-time eliminations** — each is only ever
> read against the others in the same table, and the elimination they support is
> a statement about the model, not about that recording. Do not quote any single
> value as a current fit figure.

| Mechanism          | Change                                  | Spectral envelope |
| ------------------ | --------------------------------------- | ----------------- |
| Baseline           | fitted bank, High quality               | 13.02             |
| Microphone height  | `Pickup.DistanceM` 0.030 → 0.010 m      | 12.13             |
| Near-field balance | `NearFieldScale` ×2, ×4                 | 13.4, 14.2        |
| Near field removed | `NearFieldScale` = 0 (far field only)   | 14.81             |
| Strike footprint   | `Strike.ContactRadiusM` ÷ 10            | 12.92             |
| Cavity coupling    | AIR → 1                                 | 13.02             |
| Tension asymmetry  | ASYM → 1                                | 13.12             |
| Loss-law tilt      | D.TILT → 0 (frequency-independent loss) | 16.18             |
| Damping            | DAMP → minimum                          | 12.80             |

The microphone row is worth stating explicitly, because it was the leading
hypothesis. The evanescent near-field term is `exp(−j·d/R)` in the mode's own
Bessel zero, so microphone height applies a tilt across the mode series: at the
shipped 3 cm that is **−29.7 dB** between the fundamental and 635 Hz, the right
size to be the whole deficit. Flattening it to −9.9 dB by dropping the
microphone to 1 cm — twenty decibels of tilt removed — buys **0.9 dB**. The
weight was never the problem; there was nothing under it to weigh.

DAMP and D.TILT change what survives, not what was excited: they let everything
ring long enough to be resolved and wreck the decay term doing it.

## What it is: a comb of exact zeros

The contact force is a smooth half-sine over the whole contact interval
(`addContactPulse`). A half-sine of duration τ has magnitude spectrum

```
|F(f)| ∝ |cos(π f τ)| / |1 − (2 f τ)²|
```

The numerator **vanishes at every `(k+½)/τ`**. These are analytic zeros, not a
roll-off, and at the fitted bank's τ = 8.23 ms they fall every 121.5 Hz — two of
them, 547 and 668 Hz, inside the band. Measured on the model's own render they
sit at −309 and −315 dB.

That is the mechanism, and it explains the eliminations above in one stroke: no
mode count, no microphone geometry, no loss law and no cavity coupling can
amplify an excitation of exactly zero. A tilt leaves modes quiet; a zero leaves
them unexcited.

The Hertzian contact built in [`physical-contact.md`](physical-contact.md)
turns those zeros into finite dips and moves them, but does not remove the comb,
because it is still one smooth touch and any single touch of duration τ
interferes with itself. It is worth 0–4 dB below 700 Hz. It is worth 12–23 dB
_above_ 800 Hz — which is the seam below, not this gap. It does **not** produce
the re-contacts predicted in earlier drafts of this document: the version that
appeared to was a discretization artifact that converged away.

## The measured contact time is real; it measures something else

5.5–8 ms is well supported by measurements this repository already cites: Dahl
1997; Wagner 2006; Dahl, Grossbach & Altenmüller 2011 (4.5–8 ms across four
professional players). The sound audit that replaced the old 0.71 ms law with it
was right to.

But that number is a **contact dwell time**, not a force-pulse duration, and the
model uses it as the latter. Wagner measured contact electrically, with a foil
switch, and separately measured the force — and they are not the same interval
(§4.1.1):

> At t ≈ t0 + 3.5 ms the stick leaves the membrane and the force on the
> drumstick ceases. After the drumstick has left the membrane, the force signal
> shows two weaker impacts starting at t = 3.75 ms and t = 5.6 ms

Those later impacts are the wave the strike launched, reflected off the rim and
returning under the stick — Wagner timed the centre-to-rim transit at about
1.7 ms with an accelerometer at the rim. The dwell time is long because the stick
is touched _again_, not because one force pulse lasts that long (§4.2.1):

> At higher dynamic levels, it becomes obvious from the shape of the force pulse
> that the drumstick would already leave the drumhead after approximately 3.5 ms.
> Due to the arriving reflections, however, the stick remains in contact until it
> has gained enough distance from the drumhead

So the measured excitation is **three discrete impacts inside an 8 ms window**,
and `addContactPulse` replaces them with one smooth half-sine spanning the whole
window. That is wrong twice over: too long, and too smooth.

The spectral corner of an impulse is set by its **shortest feature, not its total
duration**. Three separated impacts, an asymmetric rise, and the 440 Hz stick
bending mode Wagner extracts in his appendix A are all short features; a
half-sine has none. For scale, the roll-off a prescribed pulse commits to is
entirely a choice of shape: a rectangle falls at −6 dB/octave, this half-sine at
−12, and a raised cosine — the shape Bilbao and Webb use to excite timpani FDTD
— at −18.

Two further dependences follow from the same mechanism and the model has
neither. `contactSampleCount` varies contact only with hardness and velocity, but
Wagner measures it falling toward the rim (the reflection returns sooner, and the
head is locally stiffer, Fig. 4.7) and falling with head tension (faster wave,
narrower pulse, Fig. 4.10). In this model the reflection does not exist, so
neither dependence can.

## Why the width cannot be fixed on its own

The obvious first move is to prescribe the pulse at Wagner's measured 3.5 ms and
leave everything else alone: it needs no new mechanism, and taking the two
widths' spectra at face value it is worth about 7 dB in the band. That was tried
and measured on the model's own render, at the shipped default, velocity 1:

| 60–1000 Hz peak       | τ = 5.5 ms | τ = 3.5 ms | Δ         |
| --------------------- | ---------- | ---------- | --------- |
| Nonlinearity disabled | 39.4 dB    | 53.5 dB    | **+14.1** |
| Nonlinearity enabled  | 27.6 dB    | 50.9 dB    | +23.3     |

The pulse spectra predict +3.8 dB there. The rest is **null placement**:
shortening the pulse drags the first zero from 273 Hz to 429 Hz — straight
through the low mode cluster. It survives with the nonlinearity disabled, so it
is not a nonlinear artefact, and it is not a level shift the output gain could
absorb, because τ varies with both velocity and hardness and the zero therefore
lands on different modes at every dynamic.

This is `physical-tom-review.md` §6's third bullet — "the nulls move with the
knob… HARD re-picks which two or three modes survive rather than sweeping
dark↔bright" — measured, and worth 14 dB. Correcting the width alone does not
make the excitation right; it changes which modes a spectral zero deletes, and it
would make null placement a de facto tuning parameter. So the constants are
deliberately left as they are, with the defect recorded next to them, until the
shape is fixed in the same change.

A narrowband substitute does not work either. Stick-mode ripple deposits its
energy at the stick's own frequencies, so it overshoots wherever the stick
resonance sits while leaving the rest of the band short. The band needs a
broadband lift from a faster rise, not narrowband injection.

## The seam

There is a second, structural half to this.

The modal bank covers the bottom of the spectrum and the stochastic attack layer
covers the top. The layer's three bands sit at 0.4×, 1× and 2.5×
`Attack.CentreHz`, and its design note says the group "starts just above the top
retained mode" — true at the default 4 kHz centre, where the bands land at
1.6 / 4 / 10 kHz.

At a low tuning they do not meet. At Draft the batter bank runs out at
**467.6 Hz**, so 468 Hz to 1.6 kHz is served by neither path: above the modes the
product can afford and below the noise layer that replaces them. Lowering
`ATK.T` to cover it also lowers the two upper bands, which is the wrong trade;
and the layer is filtered noise, so it cannot produce resolvable partials in that
band whatever it is set to.

Closing the seam means either extending the modal bank's reach at low tunings, or
letting the attack layer's lowest band track the top retained mode rather than a
fixed ratio of `ATK.T`. That remains open.

## The transverse cavity

An earlier draft raised a transverse air-cavity series as a hypothesis, on the
strength of three partial frequencies read off the retired recording. That
evidence is gone, but the hypothesis was tested and it held, on model-internal
evidence needing no recording: the modal cavity landed in
`internal/physical/cavity.go` and its predicted partial tracks the rigid-cylinder
formula `c·j′_mn/(2πa)` across a 43 % sweep in `c` to ±0.09 %. See `PLAN.md`
§"P9" item M2 and [`physical-cavity.md`](physical-cavity.md). The lumped
compliance this document was written against did have no transverse modes; the
model no longer is that model.

## Reproducing

The elimination table and the null-placement table were produced with throwaway
probes; they are not committed. What is committed is
`testdata/physical-fit-tom.json`, the bank the model-internal numbers here were
measured at, and `cmd/fit-physical -report-only`, which re-measures any bank
against any recording. Note that fixture is itself orphaned by the reference
retirement (`PLAN.md` N8) and will be re-derived against the licensed set.

## Sources

- A. Wagner, _Analysis of Drumbeats — Interaction between Drummer, Drumstick and
  Instrument_, MSc thesis, KTH/TMH 2006 —
  https://www.speech.kth.se/publications/masterprojects/2006/AndreasWagner.pdf.
  §4.1.1 separates the force pulse from the electrical contact and identifies the
  rim reflection; §4.2.1 gives the dwell-versus-pulse statement quoted above;
  §4.2.2 and §4.2.3 give the strike-position and tension dependences (Figs. 4.7,
  4.10); §4.2.1 reports peak force rising from 3 N (piano) to over 100 N (forte),
  and up to 200 N; Appendix A extracts the stick's 440 Hz bending mode from the
  force signal.
- S. Dahl, "Spectral changes in the tom-tom related to striking force",
  TMH-QPSR 38(1), 1997 — the 12-inch tom endpoints the shipped law interpolates.
- S. Dahl, "Striking movements: a survey of motion analysis of percussionists",
  _Acoust. Sci. & Tech._ 32(5), 2011.
- S. Dahl, M. Grossbach & E. Altenmüller, Forum Acusticum 2011 — 4.5–8 ms across
  four professional players.
- F. Avanzini & D. Rocchesso, "Modeling Collision Sounds: Non-linear Contact
  Force", DAFx-01 — https://www.dafx.de/paper-archive/2001/papers/avanzini.pdf.
  The power-law contact model and its contact-time scaling, and the link between
  contact time and the attack's spectral centroid.
- S. Bilbao, A. Torin & V. Chatziioannou, "Numerical Modeling of Collisions in
  Musical Instruments", _Acta Acustica u. Acustica_ 101, 2015 —
  https://arxiv.org/pdf/1405.2589. Mallet–membrane force histories against
  striking velocity.

Not obtained: Sánchez & Irwin, "Drumhead contact time measurement using metallic
leaf", _JASA_ 111(5), 2002 is abstract-only — Wagner reports the same, so the
foil-switch method's primary source is unread here as well.
