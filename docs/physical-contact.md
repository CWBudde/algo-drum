# The stick, modelled

This is the record of building the two-way coupled Hertzian contact that
[`docs/physical-excitation-gap.md`](physical-excitation-gap.md) recommended, and
of measuring what it actually bought. It bought less than that document
predicted, for a reason worth having found, and it bought something else that
document did not ask for.

> **Normalisation defect, raised 2026-08-01, fixed 2026-08-02.** Every dB figure
> in this document, and in
> [`physical-nonlinearity.md`](physical-nonlinearity.md), used to be normalised
> against `contactReferenceHz` = **118 Hz** — the fundamental of
> `reference/tom.wav`, deleted on 2026-08-01 (`PLAN.md` §"P10" item N8) and never
> this bank's fundamental anyway. The 118 Hz bin read the leakage skirt of the
> model's own 150.1 Hz partial, 13.6 dB below it, so it normalised overall level
> by accident rather than referring anything to a fundamental — and once the
> recording was gone, to a number nothing could check.
>
> `contactReferenceHz` is now **derived from the configuration under test**: the
> retained (0,1) mode of `generateHeadModes(config, config.Batter)`, 150.10 Hz on
> `DefaultPhysicalDrum()`. It is not hard-coded, because the fundamental is a
> consequence of `TensionNPerM`, `RadiusM` and `SurfaceDensityKgPerM2` and B.TUNE
> moves it over 75–251 Hz.
>
> **Every table below was re-measured by running the test, not adjusted by hand.**
> The two columns of each table are separate renders, each normalised by its own
> reference bin, so the Δ column does **not** cancel the normaliser — it moved
> too. Uniformly: **every Δ in "What it does buy" rose by 5.4 dB coupled and
> 5.6 dB uncoupled**, and 800 Hz went +7.9 → **+13.3 dB** coupled and
> +11.9 → **+17.5 dB** uncoupled. The old code comment predicted +13.2 and +17.5
> from the same reasoning, so the measurement confirms it to 0.1 dB.
>
> Say it plainly rather than banking it quietly: **the re-point makes this
> document's central claim stronger, not weaker.** The Hertzian contact's reach
> past the modal ceiling was being understated by 5–6 dB at every frequency by a
> normaliser that should never have been there.
>
> One residual, measured and left in place: the derived 150.10 Hz is the
> _uncoupled batter_ (0,1) mode, while the render's fundamental sits at 154 Hz —
> the enclosed air stiffens it by **+3.9 Hz, 44 cents**
> (`TestContactReferenceIsARenderedPartial` asserts this and attributes it: the
> Berger term and the mode coupling both leave it at 154, disabling the resonant
> head moves it to 160). The bin therefore reads the fundamental's flank, 2.1 dB
> below its peak on the prescribed render and 1.3 dB on the Hertzian, so 0.8 dB
> of each Δ is that offset. Deriving the reference from the render instead would
> make it differ between the two columns, which is the defect this fixed.
>
> 118 Hz has a second life as the denominator that makes `MinSeparationHz = 15`
> an unusable resolution limit —
> [objective validation](physical-objective-validation.md) carried that too, and
> it was restated against the current reference's 240 Hz on 2026-08-02
> (`PLAN.md` N18), where the same guard is 105 cents rather than 207.
>
> **Completed 2026-08-02 (`PLAN.md` N18).** The fix above re-pointed the
> _rendered_ tables and left four literal `118`s behind in `contact_test.go`,
> which normalise the _analytic force-pulse_ spectra — the "What the comb
> actually is" table and the two figures quoted for it in the short version
> below. Those are now referred to the same derived 150.10 Hz, and the table was
> re-measured by running the test rather than adjusted. Each column moved by its
> own constant — **+7.9 dB prescribed, +5.8 dB Hertzian** — because each is
> normalised by its own bin, so the difference between the columns shrank by
> 2.1 dB and no Δ here survives the re-point unchanged either. One figure moved
> for an unrelated reason and is called out where it appears: the worst Hertzian
> notch is at **808 Hz**, not the 465 Hz this document recorded, and a notch
> frequency does not depend on the normaliser at all.

The short version:

- The 476–700 Hz gap is not a spectral **tilt**, it is a spectral **comb**. The
  prescribed half-sine has _exact analytic zeros_ every 1/τ, and two of them —
  547 and 668 Hz — sit inside the gap. That is why nothing downstream could fix
  it: no mode count, microphone, or loss law can amplify an excitation of zero.
- The Hertzian contact turns those zeros into finite dips and moves them. The
  gap's two zeros go from −301 and −307 dB to −21.7 and −19.1 dB.
- But it does **not** remove the comb, because it is still one smooth touch, and
  any single touch of duration τ interferes with itself. Its own worst dip is
  −50 dB at 808 Hz.
- It does not reproduce Wagner's separation-and-re-contact structure at all. The
  version that appeared to was **numerically wrong**, and that is the most
  important thing in this document.
- What it does do, decisively, is reach past the modal ceiling: **+13.3 dB at
  800 Hz, +20.8 dB at 1.5 kHz, +28.3 dB at 2.5 kHz** in the modal-only render.
  That is the seam, not the gap. The 800 Hz figure was +17.5 dB before the
  nonlinear mode coupling existed; the coupling raised the _prescribed_ side, for
  a reason worth reading.
- It is implemented, tested and selectable, and it is **off by default**.

## What was built

`Strike.Contact.Model` selects between two ways of producing the force the batter
head is driven by.

`ContactPrescribed` is what shipped: at trigger time a half-sine of the measured
contact duration is written into a ring buffer and played out sample by sample.
The head has no influence on it.

`ContactHertzian` integrates the stick. It is a free mass carrying a
Hunt–Crossley contact spring,

    F = K·δ^α·(1 + h·δ̇),   δ = z − w,   F ≥ 0

where `z` is the tip position, `w` the head's displacement under the tip, `K` the
tip stiffness in N/m^α, `α` Hertz's exponent and `h` the hysteresis coefficient.
Tension is not transmitted, so `δ ≤ 0` is separation. The mallet and the head
surface are both integrated across each audio sample in substeps, the head as a
free point mass driven by the same force and reseeded each sample from the true
modal state; the modal bank is then advanced by the mean force over the sample,
which is what carries the momentum.

Two details are load-bearing:

**The strike-point readback must be the strike projection.** Force is
distributed onto mode _i_ as `F·StrikeAccelerationPerN_i`; the head is read back
as `Σ StrikeAccelerationPerN_i · ModalMassKg_i · q_i`. Because those are the same
weight, `F·ẇ` is exactly the power the modes receive, and the contact cannot
manufacture energy. `TestHertzianContactCannotAddEnergy` is the guard.

**α = 3/2 is measured here, not assumed.** A Hertzian contact time scales as
`v^(−(α−1)/(α+1))`. Wagner's Fig. 4.7 crescendo runs 7.5 ms at piano to 5.9 ms at
forte; over the three- to fourfold striking velocity that spans, the implied α is
1.42–1.56. The canonical spherical value falls out of the measurement. It also
means the prescribed model's velocity law is not discarded by this change, it is
reproduced by it — where the tip has any authority, which is the next section.

## The finding: contact time is set by the head, not by the tip

This was the surprise, and everything else follows from it.

The batter head's driving-point mass under the stick is **0.31 g**, against a
15 g mallet. So the tip barely compresses; the stick pushes the head down and
rides it back up. The closed-form time for the same stick rebounding off a
_rigid_ Hertzian spring is **0.40 ms**. The coupled contact lasts **7.26 ms** —
eighteen times longer.

Sweeping the stiffness over four decades at the shipped 15 g mallet:

| K (N/m^1.5) | contact | rigid-target |
| ----------- | ------- | ------------ |
| 1e4         | 14.5 ms | 2.53 ms      |
| 1e5         | 8.7 ms  | 1.01 ms      |
| 1e6         | 7.3 ms  | 0.40 ms      |
| 3e6         | 7.1 ms  | 0.26 ms      |
| 1e8         | 6.8 ms  | 0.06 ms      |

A 900-fold stiffness range spans 1.51 in duration. The mallet mass, by contrast,
moves it almost proportionally: 3 g → 3.1 ms, 8 g → 5.5 ms, 15 g → 7.4 ms.

Three consequences.

**The duration becomes a prediction.** Nothing in the Hertzian path carries a
contact time. That it lands at 7.26 ms, inside the 5.5–8 ms Dahl 1997 and Wagner
2006 measure, is a result rather than a setting, and it is the strongest evidence
that the mechanism is right.

**HARD loses most of its authority.** The knob's stiffness law was deliberately
built to reproduce the prescribed duration law exactly — `K ∝ 2^((h−h₀)(α+1))`
gives `τ ∝ K^(−1/(α+1))` term for term — and it still does not, because that
scaling assumes a rigid target. Realized: 7.57 ms at HARD 0 against 7.39 ms at
HARD 1, where the prescribed law spans 8.93 to 4.47 ms. The measured factor of
two is not reachable through tip stiffness in this regime.

**The mallet mass stops being free.** Under the prescribed model, mass sets only
the impulse — that is, the loudness — so 15 g was never answerable to anything.
Under the Hertzian model it sets the contact time, and therefore the excitation
bandwidth, and therefore it is measurable. The measurement says the shipped value
is too heavy: reproducing the measured **velocity** law needs 4–6 g.

| mass | quiet (0.6 m/s) | loud (3 m/s) | ratio |
| ---- | --------------- | ------------ | ----- |
| 4 g  | 7.80 ms         | 5.15 ms      | 0.66  |
| 6 g  | 9.05 ms         | 5.90 ms      | 0.65  |
| 15 g | 8.07 ms         | 7.42 ms      | 0.92  |

Dahl's endpoints give 0.69. At 4–6 g the model reproduces it; at the shipped 15 g
the contact is so head-dominated that the velocity dependence nearly vanishes.
This is not fitted — the mass is the only thing changed and the ratio follows.

A few grams is also the more defensible number physically. A drumstick is a beam
struck at its tip; on a millisecond timescale only the near-tip portion
participates, so the effective impact mass is well below the stick's 45 g and
below the 15 g standing in for it.

## The near-miss: re-contacts that were not there

The first working version produced exactly what
[`docs/physical-excitation-gap.md`](physical-excitation-gap.md) predicted it
would: a first force lobe of 4.15 ms inside a 7.48 ms dwell, with **seven**
separate impacts, and 17 dB more energy at 1.5 kHz. Against Wagner's 3.5 ms lobe
inside a 5.9 ms dwell with three impacts, that is close enough to have been
written up as a confirmation.

It was an artifact. Refining the integration removes it:

| fs (kHz) | substeps | first lobe | dwell   | touches | 1500 Hz |
| -------- | -------- | ---------- | ------- | ------- | ------- |
| 44.1     | 2        | 4.15 ms    | 7.48 ms | 7       | −9.8 dB |
| 44.1     | 4        | 5.99 ms    | 7.66 ms | 4       | −21.3   |
| 44.1     | 16       | 6.96 ms    | 7.46 ms | 2       | −25.0   |
| 44.1     | 64       | 7.46 ms    | 7.46 ms | 1       | −26.4   |
| 352.8    | 64       | 7.45 ms    | 7.45 ms | 1       | −26.8   |

**This table is a record of the artifact as it was measured, and it no longer
reproduces.** Only two of its rows are covered by a shipped test, and neither
matches: `TestHertzianContactDurationIsPredictedNotPrescribed` now logs a
**7.12 ms** first lobe and dwell where the 44.1 kHz/64 row says 7.46, and
`TestHertzianContactIsSubstepConverged` logs **−27.0 dB** at 1500 Hz where the
same row says −26.4. Neither gap is the 118 Hz normalisation — it costs 5.8 dB
here and the residual is 0.6 — so the contact itself has moved since, and the
three rows that no shipped test produces cannot be re-derived without rebuilding
the sweep. What the table is being cited for is the _trend_ — chatter that
disappears as the step refines — and that is still what the converged column
says. Left as measured rather than half-corrected; re-running the sweep is its
own item.

The converged contact is a single smooth touch. The chatter came from the contact
being unresolved near δ = 0, where the spring is arbitrarily soft and a step size
that is fine at the force peak decides by rounding whether the surfaces have
parted.

What made it dangerous is that it was not implausible noise — it was a faithful
imitation of the specific phenomenon we had gone looking for, arriving in the
band we wanted it in, at roughly the right times. Three things now stand against
it recurring: `contactSubstepTarget` is set from grazing rather than stability;
separation is judged at 1% of the peak force rather than at exact zero, so a
0.01 N flicker against a 17 N peak is not a re-contact; and
`TestHertzianContactIsSubstepConverged` fails if refining the step moves the
spectrum by more than 1.5 dB.

## What the comb actually is

The half-sine's magnitude spectrum is `|cos(πfτ)| / |1 − (2fτ)²|`. The numerator
vanishes at every `(k+½)/τ`. These are analytic zeros, not a roll-off. At the
fitted bank's 8.23 ms they fall every 121.5 Hz:

| f      | prescribed | Hertzian |
| ------ | ---------- | -------- |
| 547 Hz | −301.2 dB  | −21.7 dB |
| 668 Hz | −307.1 dB  | −19.1 dB |
| 790 Hz | −330.5 dB  | −39.9 dB |

Re-derived 2026-08-02 from `TestHertzianContactShallowsAndMovesTheComb`'s own
`t.Logf` output at the derived reference; the version this replaced read
−309.1/−315.0/−338.4 against −26.4/−28.6/−36.2, of which the first column was
the old 118 Hz normalisation and the 790 Hz Hertzian figure was 9.5 dB adrift of
the test on top of that.

That is the mechanism of the gap, and it is a better explanation than the −30 dB
tilt the earlier document settled on: a tilt leaves modes quiet, a zero leaves
them unexcited. It also explains, exactly, why prescribing Wagner's shorter pulse
made things 14 dB worse — it slides the comb rather than removing it, and the
comb has to land somewhere.

The Hertzian contact does not remove the comb either. It is still one lobe, and
one lobe of duration τ interferes with itself wherever it sits. What changes is
that an asymmetric pulse's interference leaves a **finite** dip instead of a
zero: the worst below 1 kHz is −50.0 dB at 808 Hz. Better by a wide margin, and
still a hole.

That frequency is a correction rather than a re-normalisation. This document read
**−51.2 dB at 465 Hz**, and where a level depends on what it is divided by, the
_position_ of a notch does not — the 150–1000 Hz sweep reports 808 Hz both before
and after the re-point. 465 Hz was simply stale.

Removing the comb needs structure _inside_ the contact interval — which is
precisely the separation-and-re-contact Wagner measured, and precisely what this
model does not produce. Getting it will need whatever is missing from the head's
response to a strike, not a better tip.

## What it does buy

One strike at velocity 1, modal only, as shipped:

| f       | prescribed | Hertzian | Δ         |
| ------- | ---------- | -------- | --------- |
| 400 Hz  | −28.7 dB   | −14.6 dB | +14.0     |
| 504 Hz  | −27.7      | −22.2    | +5.6      |
| 635 Hz  | −31.7      | −26.4    | +5.3      |
| 800 Hz  | −40.8      | −27.6    | **+13.3** |
| 1500 Hz | −61.8      | −40.9    | **+20.8** |
| 2500 Hz | −70.8      | −42.5    | **+28.3** |
| 4000 Hz | −76.8      | −51.2    | +25.5     |

Below 700 Hz it is worth 5–14 dB, and which depends on where the two models'
combs happen to fall relative to the modes there — the spread across those three
rows is the whole of the variation. From 800 Hz up it is worth 13–28 dB, and that
is not an accident of alignment — it is the prescribed pulse's 1/f² envelope
running out.

So it addresses **the seam**, not the gap. The seam is the other finding in
`physical-excitation-gap.md`: the fit dragged `ATK.T` from 4000 Hz down to
1644 Hz and held `ATK.L` at 0.021, because the stochastic attack layer was the
only tool it had for a band the excitation never reached — and a noise layer
cannot make resolvable partials. With the Hertzian contact the excitation reaches
that band for real, which is the precondition for pushing `ATK.T` back up to
where it belongs and letting the attack layer stand in only for what is genuinely
unresolvable.

### How this was measured

Stated because the first version of this table was made by a program that was
never committed, and re-deriving it from the numbers alone turned out to be
impossible — the obvious alternative estimators disagree by tens of dB and one of
them reverses the sign of the whole table.

- One strike at velocity 1 into `DefaultPhysicalDrum()` at 44.1 kHz.
  `Strike.Contact.Model` is the only field that differs between the two columns.
- **Modal only** means `Attack.Enabled = false`, and nothing else. The cavity,
  the Berger tension term and the mode coupling are all left as shipped.
- The level at _f_ is a single-bin DFT of the **entire one-second render** at
  exactly _f_, **rectangular window**. The window is the load-bearing choice. The
  Hertzian advantage lives in the first few milliseconds of the render, so any
  taper that vanishes at sample 0 destroys the thing being measured — a Hann
  window over one second is 60 dB down at 7 ms, and the table measured through
  one comes out with the Hertzian contact _duller_ than the prescribed one
  everywhere.
- Levels are relative to `contactReferenceHz`, which since 2026-08-02 is derived
  from the configuration rather than written down: the retained (0,1) mode of
  `generateHeadModes(config, config.Batter)`, **150.10 Hz** on this bank. Before
  that it was a hard-coded 118 Hz read off a since-deleted recording; see the
  block at the top of this document for what that changed.
- The reference bin is taken from **each render separately**, because each column
  is its own render. A Δ between two columns therefore carries two
  normalisations, not none, which is why re-pointing the reference moved the Δ
  columns as well as the level columns.

### What the mode coupling changed

The Δ column above is smaller below 1 kHz than the one this document carried
before P9. The contact model did not lose ground; the prescribed side gained. The
same measurement with `Nonlinearity.Coupling.Enabled = false` — the control, and
the state the original table was measured in — is:

| f       | prescribed | Hertzian | Δ         |
| ------- | ---------- | -------- | --------- |
| 400 Hz  | −35.8 dB   | −27.5 dB | +8.3      |
| 504 Hz  | −39.7      | −34.2    | +5.4      |
| 635 Hz  | −38.7      | −26.1    | +12.5     |
| 800 Hz  | −50.2      | −32.7    | **+17.5** |
| 1500 Hz | −62.0      | −41.2    | **+20.8** |
| 2500 Hz | −71.2      | −42.6    | **+28.6** |
| 4000 Hz | −77.2      | −51.0    | +26.3     |

Under the previous 118 Hz normalisation this reproduced the pre-coupling table to
about a dB at every frequency except 635 Hz, where the original recorded −29/−25
against −25.0/−18.1 then; that row drifted with model changes between P8 and now,
and the original method was not recorded well enough to say which. That
comparison is no longer available in absolute terms — the whole table is now
referred to a different bin — but the differential is unchanged: every Δ here is
its 118 Hz value plus a flat 5.6 dB, so nothing about which rows agreed with the
pre-coupling measurement has moved.

The interesting part is _where_ the coupling helps. A mode driven by the cubic
coupling receives energy at 2f_a ± f_b and 3f_a **regardless of what |F(f)| does
at its own frequency**. The half-sine's zero comb is a statement about |F(f)| and
nothing else, so the coupling is the one mechanism in this model that can
populate a mode the comb has deleted. Reading the two tables against each other,
row by row, says exactly that:

| f       | prescribed rise | Hertzian rise | Δ moves       |
| ------- | --------------- | ------------- | ------------- |
| 400 Hz  | +7.1 dB         | +12.9 dB      | +8.3 → +14.0  |
| 504 Hz  | +12.0           | +12.0         | +5.4 → +5.6   |
| 635 Hz  | +7.0            | −0.3          | +12.5 → +5.3  |
| 800 Hz  | +9.4            | +5.1          | +17.5 → +13.3 |
| 1500 Hz | +0.2            | +0.3          | +20.8 → +20.8 |
| 2500 Hz | +0.4            | +0.1          | +28.6 → +28.3 |
| 4000 Hz | +0.4            | −0.2          | +26.3 → +25.5 |

This table moved under the re-point too, and unevenly, which is worth being
precise about. Its "Δ moves" column is read straight off the two tables above and
so carries their full 5.4/5.6 dB shift. Its two **rise** columns are differences
between the coupled and uncoupled renders of the _same_ contact model, and those
two renders' reference bins agree to about half a dB, so the rises shifted by at
most 0.3 dB — nothing here changes which rows rose and which did not. The two
halves of the table move differently because they are differences taken in
different directions across four separately normalised renders; there is no
normalisation that cancels out of both.

Below 1 kHz the rises run 5–13 dB — every row but the 635 Hz Hertzian one, which
sits at −0.3 and did under the old normalisation too; above 1 kHz neither column
moves by half a dB.
The coupling reaches the band the comb deleted and stops dead where the comb
stops, because the pumps are chosen from modes below `PumpMaxFrequencyHz` =
700 Hz and a cubic force from those reaches roughly 3× that and no further.

So the two mechanisms are not substitutes. The coupling repairs the comb's
_zeros_; the contact model repairs the envelope's _tail_. The gap needs the
first, the seam needs the second, and the fact that this table moved below 1 kHz
and did not move at all above it is the cleanest evidence of that split in the
model.

## Why it is off by default

Switching `Strike.Contact.Model` changes the shipped sound, and not only in the
band above. The Hertzian contact delivers 1.9× the prescribed impulse, because
the stick rebounds — physically right, and a level change. `Pickup.OutputGain`,
`Nonlinearity.*TensionCoefficient` and `Attack.LevelRelative` were all fitted
against the prescribed excitation and would all need to move. The fitted preset
in `testdata/physical-fit-tom.json` would need re-deriving, and by the argument
above so would `Strike.MalletMassKg`.

That is a calibration pass, not a switch, so the switch is left where it is with
the measurements recorded beside it. `DefaultContact()` is nonetheless calibrated
for the drum as shipped — K = 1e6 N/m^1.5 predicts 7.3 ms against the 15 g mallet
— so flipping the model without touching anything else gives the best available
single-change result rather than a broken one.

### Whether the pass is worth starting — open

It was answered once, by fitting both contact models head-to-head against
`reference/tom.wav`. Those totals are **deleted**: that recording is gone
(`PLAN.md` §"P10" item N8), the objective that scored them is now known not to
resolve most of what it reported (see
[objective validation](physical-objective-validation.md)), and the run predated
two corrections to the measurement itself. Nothing is carried
forward from it, in either direction — there is currently **no fitted evidence**
on whether the Hertzian contact pays for its calibration pass.

`DefaultContact().Model` therefore stays `ContactPrescribed` on the grounds in
the section above — flipping it is a calibration pass, not a switch — and the
question reopens with the joint refit across the licensed sixteen-velocity
reference (`PLAN.md` N5). The mallet-mass finding higher up this page is
unaffected: it is measured against the model and against Dahl's velocity ratio,
not against any recording.

## Reproducing

Everything above is in `internal/physical/contact_test.go`, which is in
`just test`. No render here ever depended on `reference/tom.wav` — the tests run
against `DefaultPhysicalDrum` at 44.1 kHz — so deleting the recording broke
nothing here. The **normalisation** was the exception, via a hard-coded
`contactReferenceHz` = 118 Hz read off that recording; it is now derived from the
configuration instead, and this document carries no number traceable to the
deleted file. See the block at the top for what that cost and what it bought.

Both tables in "What it does buy" are asserted row by row, in both coupling
states, by `TestHertzianContactReachesPastTheModalCeiling`, to ±1.5 dB, and the
bin they are referred to is itself checked against the render's fundamental by
`TestContactReferenceIsARenderedPartial`. Its doc comment carries the method as
well, so the two cannot drift apart silently the way the first version of that
table did — that one was measured by an
uncommitted program, and when it was re-derived the method had to be recovered
from the surviving test helper rather than from the document.

The sweeps that are not committed — stiffness, mass, hysteresis, hit radius,
quality, and the substep/sample-rate convergence grid — were throwaway probes
over `NewDoubleHead` + `Tick().ContactForceN`, which is the whole interface they
need.

## Sources

- **Dahl, S. (1997).** _Spectral changes in the tom-tom related to striking
  force._ STL-QPSR 38(1). Contact-time endpoints for a 12-inch tom.
- **Wagner, A. (2006).** _Analysis of drumbeats — interaction between drummer,
  drumstick and instrument._ MSc, KTH. §4.1.1 and §4.2.1 for the separation at
  ~3.5 ms and the re-contacts at 3.75 and 5.6 ms; Fig. 4.7 for the crescendo
  contact times with and without re-contacts counted.
- **Avanzini, F. & Rocchesso, D. (2001).** _Modeling collision sounds:
  non-linear contact force._ DAFx-01. The Hunt–Crossley form and the
  `τ ∝ (m/K)^(1/(α+1))`, `τ ∝ v^(−(α−1)/(α+1))` scalings used to calibrate and to
  read α off Wagner's crescendo.
- **Hunt, K. H. & Crossley, F. R. E. (1975).** _Coefficient of restitution
  interpreted as damping in vibroimpact._ J. Appl. Mech. 42(2). The hysteresis
  term, and the validity limit that bounds `HysteresisSPerM`.
