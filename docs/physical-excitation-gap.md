# Where the 476–700 Hz band went

> **Pending re-measurement (2026-07-31).** Everything in this document rests on a
> reference reduction taken with a partial-level estimator that has since been
> corrected — see
> [`physical-measured-fit.md` § A correction to the partial measurement](physical-measured-fit.md#a-correction-to-the-partial-measurement-2026-07-31).
> The corrected reduction of the right channel finds 14 partials rather than 7,
> seven of them in this band, so both the size of the deficit measured here and
> the conclusions drawn from it are pending re-measurement. Nothing below is
> retracted; none of it should be quoted as current either.

[`physical-measured-fit.md`](physical-measured-fit.md) ended on one unexplained
number. The fit matched the reference's pitch, ring and envelope closely and
missed its spectral envelope by 13.6 dB against a 4 dB gate, because the
recording carries nine resolvable partials between 476 and 700 Hz and the model
produces none there. That document left three suspects: the two-head mode
series, the cavity split, or the absent shell and lug modes.

It is none of the three. **The modes are there and the force that should excite
them is not.** This is an excitation-spectrum gap, and
[`physical-tom-review.md`](physical-tom-review.md) §6 predicted it by inspection
before the fit measured it.

Everything below is measured against `testdata/physical-fit-tom.json` at 44.1 kHz
— the reference's own rate — with `internal/physical/match`. The reference
recording is of unknown provenance and is not in the repository; see the
measured-fit document for what that does and does not license.

## The gap is a hole in the spectrum, not a hole in the mode list

The ⅓-octave band levels, candidate minus reference, mean-removed within each
window so a level difference cannot show up:

| Band (Hz) | 0–20 ms | 20–100 ms | 0.1–0.4 s | 0.4–1.2 s |
| --------- | ------- | --------- | --------- | --------- |
| 200       | −5.1    | −8.5      | −0.2      | +23.7     |
| 252       | −9.5    | +6.2      | +10.5     | +27.0     |
| 400       | +3.7    | +3.3      | −1.0      | +1.4      |
| **504**   | −9.7    | −17.7     | **−22.6** | −17.5     |
| **635**   | −12.7   | −24.0     | **−30.4** | **−35.8** |
| **800**   | −7.5    | −15.2     | **−21.4** | −27.9     |
| **1008**  | −7.2    | −19.7     | **−23.9** | −15.8     |
| 1270      | −6.9    | −11.4     | −15.3     | −7.5      |
| 2016      | −1.0    | −6.1      | +6.7      | −0.8      |

Two things to read off it. The deficit is **band-limited** — it starts abruptly
between 400 and 504 Hz and is gone again by 2 kHz — and it **deepens with
time**, from about −10 dB during contact to −30 dB in the body. Below it the
model is progressively too loud in the tail, which is the same fact stated the
other way: its low modes carry the whole tail because nothing above them
survives.

The mode list is not the constraint. At High quality the batter bank puts **58
modes between 476 and 700 Hz** and the partial detector finds **none** of them:

| Quality  | Modes | Highest batter mode | Modes in 476–700 | Partials detected in 476–700 |
| -------- | ----- | ------------------- | ---------------- | ---------------------------- |
| Draft    | 48    | 467.6 Hz            | 0                | 0                            |
| Standard | 96    | 665.1 Hz            | 45               | 0                            |
| High     | 160   | 852.5 Hz            | 58               | 0                            |

Tripling the mode count moves the spectral-envelope term from 13.3 to 13.1 dB.
Fifty-eight oscillators sit in the band and are inaudible.

## What it is not

Each row changes one mechanism away from the fitted bank at High quality and
re-measures. The spectral-envelope term at the fitted bank is **13.02 dB**.

| Mechanism          | Change                                  | Spectral envelope | Partials in band |
| ------------------ | --------------------------------------- | ----------------- | ---------------- |
| Microphone height  | `Pickup.DistanceM` 0.030 → 0.010 m      | 13.02 → **12.13** | 0 → 0            |
| Near-field balance | `NearFieldScale` ×2, ×4                 | 13.4, 14.2        | 0                |
| Near field removed | `NearFieldScale` = 0 (far field only)   | 14.81             | 0                |
| Strike footprint   | `Strike.ContactRadiusM` ÷ 10            | 12.92             | 0                |
| Cavity coupling    | AIR → 1                                 | 13.02             | 0                |
| Tension asymmetry  | ASYM → 1                                | 13.12             | 0                |
| Loss-law tilt      | D.TILT → 0 (frequency-independent loss) | 16.18             | 2                |
| Damping            | DAMP → minimum                          | 12.80             | 5                |

The microphone row is worth stating explicitly, because it was the leading
hypothesis. The evanescent near-field term is `exp(−j·d/R)` in the mode's own
Bessel zero, so the microphone height applies a tilt across the mode series: at
the shipped 3 cm that is **−29.7 dB** between the fundamental and 635 Hz, which
is the right size to be the whole deficit. Flattening it to −9.9 dB by dropping
the microphone to 1 cm — twenty decibels of tilt removed — buys **0.9 dB**. The
weight was never the problem; there was nothing under it to weigh.

DAMP and D.TILT do move partials into the band, by letting everything ring long
enough to be resolved, but they wreck the decay term doing it and the spectral
envelope does not improve. They change what survives, not what was excited.

## What it is

The contact force is a smooth half-sine over the whole contact interval
(`addContactPulse`). A half-sine of duration τ has magnitude spectrum

```
|F(f)| ∝ |cos(π f τ)| / |1 − (2 f τ)²|
```

which nulls at `1.5/τ` and falls as `1/f²` after it. At the fitted bank's
τ = 8.23 ms the first null is at **182 Hz**, and everything above it is out in
the sidelobes. Its own tilt relative to the fundamental, beside the measured
deficit in the body window:

| Band (Hz)                         | 504       | 635       | 800   | 1008  |
| --------------------------------- | --------- | --------- | ----- | ----- |
| Contact-pulse tilt at τ = 8.23 ms | −28.7     | −34.2     | −47.4 | −44.3 |
| Measured band deficit             | **−22.6** | **−30.4** | −21.4 | −23.9 |

The onset of the hole is predicted within 4–6 dB by the excitation alone. Above
800 Hz the measured deficit is smaller than the pulse tilt because the
stochastic attack layer is filling in there — which is the next section.

### The measured number is real; it measures something else

5.5–8 ms is well supported, by measurements this repository already cites: Dahl,
TMH-QPSR 38(1) 1997; Wagner, KTH MSc 2006; Dahl, Grossbach & Altenmüller, Forum
Acusticum 2011 (4.5–8 ms across four professional players). The sound audit that
replaced the old 0.71 ms law with it was right to.

But that number is a **contact dwell time**, not a force-pulse duration, and the
model uses it as the latter. Wagner measured contact electrically, with a foil
switch, and separately measured the force — and they are not the same interval
(§4.1.1):

> At t ≈ t0 + 3.5 ms the stick leaves the membrane and the force on the
> drumstick ceases. After the drumstick has left the membrane, the force signal
> shows two weaker impacts starting at t = 3.75 ms and t = 5.6 ms

Those two later impacts are the wave the strike launched, reflected off the rim
and returning under the stick — Wagner timed the centre-to-rim transit at about
1.7 ms with an accelerometer at the rim. The dwell time is long because the stick
is touched _again_, not because one force pulse lasts that long (§4.2.1):

> At higher dynamic levels, it becomes obvious from the shape of the force pulse
> that the drumstick would already leave the drumhead after approximately 3.5 ms.
> Due to the arriving reflections, however, the stick remains in contact until it
> has gained enough distance from the drumhead

So the measured excitation is **three discrete impacts inside an 8 ms window**,
and `addContactPulse` replaces them with one smooth half-sine spanning the whole
window. That is wrong twice over: too long, and too smooth.

Both errors cost the same band, and neither is worth correcting alone. Taking
the main pulse's ~3.5 ms and keeping the half-sine, the same formula gives
−22.5 dB at 504 Hz and −26.4 dB at 635 Hz — about 7 dB better than the 8.23 ms
version, still not enough, and, as measured below, not safe either. The rest is
shape: the
spectral corner of an impulse is set by its **shortest feature, not its total
duration**. Three separated impacts, an asymmetric rise, and the 440 Hz stick
bending mode Wagner extracts in his appendix A are all short features; a
half-sine has none, so it discards that band and nothing downstream can put it
back.

For scale, the roll-off a prescribed pulse commits to is entirely a choice of
shape: a rectangle falls at −6 dB/octave, this half-sine at −12, and a raised
cosine — the shape Bilbao and Webb use to excite timpani FDTD — at −18.

Two further dependences follow from the same mechanism and the model has
neither. `contactSampleCount` varies contact only with hardness and velocity,
but Wagner measures it falling toward the rim (the reflection returns sooner,
and the head is locally stiffer, Fig. 4.7) and falling with head tension (faster
wave, narrower pulse, Fig. 4.10). In this model the reflection does not exist, so
neither dependence can.

### Confirming it

Three experiments, all reverted — none of this is in the tree:

| Experiment (all at High quality, fitted bank)         | 504 Hz   | 635 Hz    | Partials in band | Top partial |
| ----------------------------------------------------- | -------- | --------- | ---------------- | ----------- |
| Baseline                                              | −22.5    | −30.4     | 0                | 470.6 Hz    |
| Contact shortened to ~2 ms                            | —        | —         | 3                | 588.5 Hz    |
| Same 8.23 ms and impulse, rise time skewed to 0.49 ms | −14.3    | −20.2     | 2                | —           |
| Skewed rise **and** stick-mode ripple                 | **−9.0** | **−17.9** | 3                | —           |

Holding the measured contact duration and the prescribed impulse fixed and
changing only the force's internal shape recovers 13 dB at 504 Hz and 13 dB at
635 Hz. Nothing else tried here moves those bands at all.

It does not recover all of it, and the ripple experiment shows why the fix has
to be physical rather than cosmetic: stick-mode ripple deposits its energy at
the stick's own frequencies, so a 400 Hz ripple overshoots that band by 16 dB
while leaving 635 Hz 18 dB short. The band needs a broadband lift from a faster
rise, not narrowband injection.

The ~2 ms row was run before the literature check, as a deliberately unphysical
control — the point was only that the band responds to excitation bandwidth. It
turns out not to be unphysical at all: it is within a factor of two of Wagner's
measured 3.5 ms main pulse, and the reason it looked wrong is the same conflation
this document is about.

### Why the width cannot be fixed on its own

The obvious first move is to prescribe the pulse at Wagner's measured 3.5 ms and
leave everything else alone: it needs no new mechanism, and taking the two
widths' spectra at face value it is worth about 7 dB in the band. That was
tried, and measured, and it is not safe. At the shipped default, velocity 1:

| 60–1000 Hz peak       | τ = 5.5 ms | τ = 3.5 ms | Δ         |
| --------------------- | ---------- | ---------- | --------- |
| Nonlinearity disabled | 39.4 dB    | 53.5 dB    | **+14.1** |
| Nonlinearity enabled  | 27.6 dB    | 50.9 dB    | +23.3     |

The pulse spectra predict +3.8 dB there. The rest is **null placement**: a
half-sine of width τ nulls at 1.5/τ, so shortening the pulse drags that zero
from 273 Hz to 429 Hz — straight through the low mode cluster. It survives with
the nonlinearity disabled, so it is not a nonlinear artefact, and it is not a
level shift the output gain could absorb, because τ varies with both velocity
and hardness and the zero therefore lands on different modes at every dynamic.

This is `physical-tom-review.md` §6's third bullet — "the nulls move with the
knob… HARD re-picks which two or three modes survive rather than sweeping
dark↔bright" — measured, and worth 14 dB. Correcting the width alone does not
make the excitation right; it changes which modes a spectral zero deletes, and
it would make null placement a de facto tuning parameter. So the constants are
deliberately left as they are, with the defect recorded next to them, until the
shape is fixed in the same change.

## The seam

There is a second, structural half to this, and the search found it on its own.

The modal bank covers the bottom of the spectrum and the stochastic attack layer
covers the top. The layer's three bands sit at 0.4×, 1× and 2.5× `Attack.CentreHz`,
and its design note says the group "starts just above the top retained mode" —
true at the default 4 kHz centre, where the bands land at 1.6 / 4 / 10 kHz.

At this drum they do not meet. The fit tuned to a 118 Hz fundamental, which is
low, and at Draft the batter bank runs out at **467.6 Hz**. So the fit dragged
`ATK.T` down from its 4 kHz default to **1644 Hz**, putting the layer's lowest
band at **658 Hz** — directly into the hole — and then had to hold `ATK.L` at
**0.021**, because the same knob that lowers the useful band also raises the
1644 Hz and 4110 Hz bands the reference does not want. It spent both attack
parameters trying to cover a gap neither was built for, and could not, because
the layer is filtered noise and cannot produce resolvable partials.

So 450–1000 Hz is served by neither path: above the modes the product can afford
and below the noise layer that replaces them.

## An observation, offered as a hypothesis

The reference's three partials above the cluster — **624.4, 1018.4 and
1331.3 Hz** — are close to the transverse air-cavity series of a cylinder of this
radius. For a = 0.1584 m, `j′₁₁, j′₂₁, j′₀₁ × c/2πa` = **634, 1052 and 1320 Hz**,
and the band deficit peaks at exactly 635 Hz. The model's cavity is a lumped
compliance coupling through swept area alone, so it has no transverse modes and
cannot couple to any m > 0 head mode.

This is a coincidence of three numbers on a recording whose actual diameter is
unknown, so it is a hypothesis and not a finding. It is worth testing because a
transverse cavity mode would also explain why the reference's cluster is a
~27 Hz comb of nine lines rather than the dense membrane thicket the model puts
there — an m = 1 cavity resonance lends radiating efficiency to the m = 1 head
modes near it, which is a mechanism the lumped cavity does not have.

## What this means for P8

The exit criterion asks for modal frequency, decay **and** spectrum inside
tolerance. Frequency and decay are there. Spectrum is not, and it is now
attributable rather than open:

- it is not mode count, microphone geometry, strike footprint, cavity coupling
  or tension asymmetry — each was measured and eliminated;
- it is the contact force: the model prescribes one smooth half-sine across a
  measured _dwell_ time that Wagner shows is three separate impacts, which
  accounts for the onset of the hole to within 4–6 dB and recovers about 20 dB
  when corrected — 7 dB from the duration, 13 dB from the shape;
- and it is compounded by a seam between the modal bank and the attack layer
  that opens at low tunings.

> **Followed up 2026-07-30 — and item 1 below is now partly wrong.** The
> Hertzian contact was built, calibrated and measured; see
> [`docs/physical-contact.md`](physical-contact.md). Three corrections to what
> is written here. The gap is a **comb of exact zeros** at every `(k+½)/τ`, not
> a −30 dB tilt — 547 and 668 Hz sit inside it at −309 and −315 dB, which is why
> nothing downstream could lift them and why sliding τ made things worse. The
> Hertzian contact shallows and moves that comb but does not remove it, because
> it is still one smooth touch; it is worth 0–4 dB below 700 Hz. And it does not
> produce the re-contacts predicted below at all — the version that appeared to
> was a discretization artifact that converged away. What it is worth, by
> 12–23 dB, is the band above 800 Hz, which is the seam in the next section
> rather than the gap in this one.

The recommended work, in order:

1. **Correct the pulse width and the pulse shape together, in one change.**
   They looked separable and they are not — see below. A two-way coupled
   Hertzian contact is the principled version of both at once: the force follows
   from compression against the head's own motion, so the asymmetric rise, the
   rim reflection and the re-contacts emerge instead of being pasted on, the
   contact interval comes out rather than being prescribed, and the
   strike-position and tension dependences Wagner measured come with it for
   free. Avanzini & Rocchesso give the scaling to calibrate it against:
   τ ∝ (m/K)^(1/(α+1)) and τ ∝ v^(−(α−1)/(α+1)). This changes the shipped sound
   of the voice and needs its own calibration pass and a re-fit before the
   numbers here can be compared.
2. **Close the seam** — either extend the modal bank's reach at low tunings or
   let the attack layer's lowest band track the top retained mode rather than a
   fixed ratio of `ATK.T`.
3. **Test the transverse cavity hypothesis** before assuming it.

A longer or wider search is explicitly not on this list. The fit's best restart
was still descending and a longer run would find a better total, but every
mechanism that could put energy in this band was eliminated by measurement
above, so it would not close this.

## Reproducing

The elimination table, the band tables and the three confirmation experiments
were produced with throwaway probes against `reference/tom.wav`, which the
repository does not contain. They are not committed: no test may depend on that
file. What is committed is `testdata/physical-fit-tom.json`, the bank every
number here was measured at, and `cmd/fit-physical -report-only`, which
re-measures any bank against any recording.

## Sources

The contact-time citations the repository already carried are unchanged and
still hold; what this pass adds is the distinction between the two quantities
they measure.

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
