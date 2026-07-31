# The stick, modelled

This is the record of building the two-way coupled Hertzian contact that
[`docs/physical-excitation-gap.md`](physical-excitation-gap.md) recommended, and
of measuring what it actually bought. It bought less than that document
predicted, for a reason worth having found, and it bought something else that
document did not ask for.

The short version:

- The 476–700 Hz gap is not a spectral **tilt**, it is a spectral **comb**. The
  prescribed half-sine has _exact analytic zeros_ every 1/τ, and two of them —
  547 and 668 Hz — sit inside the gap. That is why nothing downstream could fix
  it: no mode count, microphone, or loss law can amplify an excitation of zero.
- The Hertzian contact turns those zeros into finite dips and moves them. The
  gap's two zeros go from −309 and −315 dB to −26.4 and −28.6 dB.
- But it does **not** remove the comb, because it is still one smooth touch, and
  any single touch of duration τ interferes with itself. Its own worst dip is
  −51 dB at 465 Hz.
- It does not reproduce Wagner's separation-and-re-contact structure at all. The
  version that appeared to was **numerically wrong**, and that is the most
  important thing in this document.
- What it does do, decisively, is reach past the modal ceiling: **+11.8 dB at
  800 Hz, +15.1 dB at 1.5 kHz, +22.9 dB at 2.5 kHz** in the modal-only render.
  That is the seam, not the gap.
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
| 547 Hz | −309.1 dB  | −26.4 dB |
| 668 Hz | −315.0 dB  | −28.6 dB |
| 790 Hz | −338.4 dB  | −36.2 dB |

That is the mechanism of the gap, and it is a better explanation than the −30 dB
tilt the earlier document settled on: a tilt leaves modes quiet, a zero leaves
them unexcited. It also explains, exactly, why prescribing Wagner's shorter pulse
made things 14 dB worse — it slides the comb rather than removing it, and the
comb has to land somewhere.

The Hertzian contact does not remove the comb either. It is still one lobe, and
one lobe of duration τ interferes with itself wherever it sits. What changes is
that an asymmetric pulse's interference leaves a **finite** dip instead of a
zero: the worst below 1 kHz is −51.2 dB at 465 Hz. Better by a wide margin, and
still a hole.

Removing the comb needs structure _inside_ the contact interval — which is
precisely the separation-and-re-contact Wagner measured, and precisely what this
model does not produce. Getting it will need whatever is missing from the head's
response to a strike, not a better tip.

## What it does buy

Modal-only (attack layer disabled), one strike, referred to the fundamental:

| f       | prescribed | Hertzian | Δ         |
| ------- | ---------- | -------- | --------- |
| 400 Hz  | −22 dB     | −19 dB   | +3        |
| 504 Hz  | −25        | −25      | 0         |
| 635 Hz  | −29        | −25      | +4        |
| 800 Hz  | −36        | −25      | **+11.8** |
| 1500 Hz | −48        | −34      | **+15.1** |
| 2500 Hz | −58        | −35      | **+22.9** |
| 4000 Hz | −64        | −43      | +21       |

Below 700 Hz it is worth 0–4 dB. Above 800 Hz it is worth 12–23 dB.

So it addresses **the seam**, not the gap. The seam is the other finding in
`physical-excitation-gap.md`: the fit dragged `ATK.T` from 4000 Hz down to
1644 Hz and held `ATK.L` at 0.021, because the stochastic attack layer was the
only tool it had for a band the excitation never reached — and a noise layer
cannot make resolvable partials. With the Hertzian contact the excitation reaches
that band for real, which is the precondition for pushing `ATK.T` back up to
where it belongs and letting the attack layer stand in only for what is genuinely
unresolvable.

> **Confirmed by fitting, 2026-07-31.** Refitting the whole bank under each
> contact model at 150 iterations leaves `ATK.T` at **3426 Hz** under Hertzian
> against **1261 Hz** under prescribed, with `ATK.L` at its lowest of any run.
> The search stops leaning on the noise layer, exactly as predicted here — and
> the fit still finds **0 partials** in 476–700 Hz against the reference's 9, so
> the seam and the gap really are two different things. See
> [`physical-measured-fit.md`](physical-measured-fit.md).

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

### Whether the pass is worth starting — measured 2026-07-31

It is not, on fit quality. Both models were given the same bank and the same
budget against `reference/tom.wav`, 8 restarts × 150 iterations:

| Contact        | Best total | Partial decay | Spectral envelope | 476–700 Hz |
| -------------- | ---------- | ------------- | ----------------- | ---------- |
| Prescribed     | **5.901**  | 0.179 ✅      | 12.3 dB ❌        | 0 of 9     |
| Hertzian, 15 g | 7.450      | 0.493 ❌      | 14.5 dB ❌        | 0 of 9     |
| Hertzian, 5 g  | 6.548      | 0.188 ✅      | 11.9 dB ❌        | 0 of 9     |

The prescribed excitation wins on total and on the decay gate, and neither model
comes within 3× of the spectral-envelope gate. The restart distributions overlap
heavily, so read this as "the Hertzian contact does not pay for its calibration
pass", not as "the prescribed contact is better physics" — the measurements
higher up this page say plainly that it is not. What the fit shows is that the
band this model fails to populate is one the contact model cannot reach either.

The 15 g mallet is confirmed as too heavy for this model: at the measured 5 g the
Hertzian total improves 7.450 → 6.548. That is a finding about
`DefaultPhysicalDrum()`, not about the contact switch, and it is recorded rather
than applied for the same reason the switch is — mallet mass moves the impulse,
so it belongs to the same calibration pass.

`DefaultContact().Model` therefore stays `ContactPrescribed`, now on fitted
evidence rather than on an open question.

## Reproducing

Everything above is in `internal/physical/contact_test.go`, which is in
`just test`. Nothing here depends on `reference/tom.wav`; the frequencies quoted
come from the measured fit and the tests run against `DefaultPhysicalDrum` at
44.1 kHz so they line up with it.

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
