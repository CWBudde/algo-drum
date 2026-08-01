# Measuring a real tom: what is already answered, and what is not

This document used to specify six numbered captures to be made on a real drum,
because nothing in `internal/physical` had ever been checked against a
measurement of a known instrument. Most of that is now supplied by a licensed
sample pack, and the document has been cut down to what the pack cannot answer.

The pack is `reference/tt08x08/lp/hd/v01..v16.wav`: an **8" × 8"** tom, Remo
coated Ambassador batter and clear Diplomat resonant head, 48 kHz 24-bit, sixteen
velocities, a **coincident** XY pair, CC BY 4.0, committed to the repository with
its provenance sheet in [`reference/CREDITS.md`](../reference/CREDITS.md). That
file is authoritative on licence, instrument and what was and was not verified.
The plan items that consume it are PLAN.md's physical-path backlog, N1–N14, and
[`physical-objective-validation.md`](physical-objective-validation.md) is the
method and evidence behind them — read it before trusting any number this
document's tooling produces.

Everything below is written so that each capture ends in a **number that changes
a named field**. If a measurement cannot be traced to a field, it is not in this
document.

---

## What the pack already answers

The pack is a full multisample of one drum with **known diameter, depth and head
models**, so several things that used to require an afternoon with a drum are now
a matter of running the analysis:

| Was a capture here                  | Now supplied by                                                                                                           |
| ----------------------------------- | ------------------------------------------------------------------------------------------------------------------------- |
| Modal frequencies and their ratios  | any one take; the geometry is stated, so the whole Bessel series is predicted from the fundamental with no free parameter |
| Per-mode T60 and the ζ(f) structure | the same takes, through `internal/physical/match`'s decay fitter                                                          |
| A strike-**velocity** series        | the sixteen velocities, single takes, soft to hard                                                                        |
| A tuning series                     | seven tunings, `vlp` through `vhp` (only `mp` is committed; the rest are in the pack)                                     |
| Head surface density                | the stated Remo gauges — 10 mil batter, 7.5 mil resonant — rather than a fit                                              |

Two consequences worth stating plainly. `SIZE` and `DEPTH` become constants
rather than fitted parameters. And the mode series is now a known-geometry
measurement rather than a scatter argument, which is what settled the air-loading
question — see
[`physical-objective-validation.md`](physical-objective-validation.md) §"Exterior
air loading is not the missing mechanism".

**A practical constraint on using it.** `match.DefaultOptions().AnalysisSeconds`
is 1.2 s and the medium-pitch files are 1.250 s, which fits with almost nothing
to spare. The higher tunings are shorter — down to 0.52 s — so the tail window
has to be shortened before those files are usable at all. Do that before
concluding anything about decay from them.

**What the pack does not supply, and is not scheduled here.** One strike position
per tuning, so no strike-position or microphone-position series. Single takes per
velocity, so no repeated-hit spread at one dynamic — which is what PLAN item S7
would need to size per-trigger jitter. No contact-time measurement; the
repository's contact model already predicts 7.4 ms for its 15 g mallet, inside
Wagner's 4.7–7.5 ms and Dahl, Grossbach & Altenmüller's 4.5–8 ms, so that
measurement had the least headroom of anything here even before the pack existed.

---

## The one outstanding physical measurement: the `(0,1)` doublet

This is Fischer's protocol applied to a tom instead of a snare, and it is the
only capture in this document that requires physical access to an instrument.
PLAN.md carries it as **N14**. It was postponed by the user on 2026-08-01 for
noise reasons and is **no longer blocking**: N4 recomputes the cavity stiffness
against the licensed drum's known geometry, which supplies most of what this was
for. It remains the cleanest evidence available, and it is worth doing if a quiet
room and a drum ever coincide.

**What the model needs.** Three frequencies, not two:

- `f_single` — the batter `(0,1)` with the resonant head **removed**;
- `f_lower`, `f_upper` — the two branches the `(0,1)` splits into once the
  resonant head is on, at **unchanged batter tuning**.

The physics
([`physical-cavity.md`](physical-cavity.md#why-the-air-spring-is-fitted)): the
enclosed air couples the heads, and the axisymmetric fundamental becomes a
parallel/antiparallel pair. Eigenvalue interlacing pins `f_lower` **between the
two heads' uncoupled `(0,1)` frequencies**, so the audible branch cannot rise by
16 % no matter how stiff the air is — it is `f_upper` that carries the coupling,
and the **separation** is what a fit can target.

Capturing all three is the point. Fischer's published 186 → 215 Hz, ratio 1.16,
is a before-and-after of _one_ number, and this repository has read it as
`f_upper/f_lower` while its own interlacing argument says the lower branch is
pinned and cannot carry that shift. Three frequencies disambiguate the two
readings; two do not.

**Lands in.** `physical.Cavity.StiffnessScale`. Also `Cavity.DepthM`,
`AirDensityKgPerM3` and `SoundSpeedMPerS`, which are geometry and air and should
simply be set to the drum's and the room's.

### The captures, in order

1. two-head take, ten centre hits
2. remove the resonant hoop and head, **counting rod turns** so it goes back
3. single-head take, ten centre hits
4. refit the resonant head to the same rod positions
5. re-record the two-head take as a **bracket**

If the batter fundamental in step 5 differs from step 1 by more than about
5 cents, the drum drifted while you worked and the split ratio is worth nothing.
That check is the whole reason for step 5. If the drum's resonant and batter
hoops share a lug body — most do — removing the resonant head changes the lug
loading on its own, so the bracket is not optional.

**Strike dead centre**, not at 0.30 R: the doublet is an axisymmetric
phenomenon, a centre hit excites `m = 0` and very little else, and every `m > 0`
mode not excited is a partial that cannot be mistaken for the stiffened branch.
Same microphone position for both takes — move nothing but the head.

### Rig

**Microphone.** Record at the model's own default pickup geometry —
`Pickup.Radius01 = 0.65`, `AngleRad = 0.6 rad`, `DistanceM = 0.03` — so the
measured partial levels and the model's `Pickup` term describe the same
observation point. Prefer an **omnidirectional** measurement microphone: a
cardioid at 3 cm has tens of dB of proximity boost below 200 Hz, right where the
fundamental lives, and the model has no proximity term, so a fit would absorb the
microphone's low end into `NearFieldScale` and `OutputGain` and call it the drum.
If a cardioid is all there is, write the model down and treat the _levels_ as
suspect while the _frequencies_ remain fine.

**Gain staging.** 48 kHz (`LoadReference` does not resample, and it matches both
the engine and the pack), 24-bit, peaks between −12 and −6 dBFS. Never let a
sample touch full scale: a clipped attack manufactures partials at every
frequency. `cmd/measure-tom` counts clipped samples and refuses to be quiet about
them.

**Room.** Drum at least 1 m from every wall, out of corners, in the most damped
room available. A 3.4 m room dimension puts axial modes near 50, 100 and 150 Hz
and a tom's fundamental sits at 75–250 Hz, so a room mode can land on top of the
measurement. Two defences: the microphone is at 3 cm, where the direct field
dominates; and the **move-the-drum control** — re-record one take with the drum
moved half a metre and rotated. Anything below 300 Hz that moves with the drum's
position is the room's.

**Strikes.** One stick, one player, one dynamic, at least **2 s** between hits so
no tail overlaps the next onset — more than twice the 1.2 s analysis window.
Record 2 s of silence at the head of each file so the noise floor is measurable.

**Do not damp the other head** with tape, gel or a wallet. Every one of those
adds mass and loss to the head being measured, which is the same as changing σ
and the loss law — the two things the capture exists to determine. Fischer's
protocol removes the head; so does this one.

### Analysis

```bash
go run ./cmd/measure-tom -doublet -o doublet.json single-head.wav two-head.wav
```

The first file is the one with the resonant head **off**. The tool reports
`f_single`, `f_lower`, `f_upper`, the split ratio and both ratios against
`f_single`, plus every candidate partial above the lower branch so the choice of
`f_upper` is visible rather than asserted. The search window is
`-doublet-max-ratio`, default 1.45 — below the ≈1.5 where the `(1,1)` family
sits. If the stiffened branch is above the window, raise the flag and say so in
the notes; do not let the default hide the answer.

### What a bad measurement looks like

- **The batter tuning moved.** The bracket take catches it. More than ~5 cents
  and the whole thing is void.
- **An off-centre strike** on either take. It excites the `(1,1)` family at
  ~1.5× the fundamental and the tool will happily nominate one of those as the
  stiffened branch. The `(1,1)` should be at least 10 dB down.
- **Hits closer than ~2 s**, which is the most common way to get a plausible,
  wrong T60.
- **A noise floor less than ~45 dB below the peak**, which makes the decay fit
  end in the room rather than in the drum and read long.
- **Confusing the vent.** A tom's vent is a Helmholtz port near 30 Hz on a
  12"×9" shell, far below the fundamental. Taping it over is a legitimate
  control — it should move the split ratio by **less than about 5 %** — and if it
  moves it a lot, that is a finding, because the repository's arithmetic says it
  cannot. Do not tape it for the main takes.

---

## The tension question

`internal/physical/config.go` records that `TensionNPerM` was chosen
**pitch-first**: at the previous 600 N/m the 12-inch batter head's fundamental
was 104 Hz, "which is a floor tom"; 1250 puts it at 150.08 Hz, mid-travel of the
300–3500 N/m range. No tension was ever measured, and the doublet capture is
what would supply one.

For an ideal circular membrane,

```
f(0,1) = α01 · c / (2πR),    α01 = 2.404826
c      = 2πR · f(0,1) / α01
T      = σ · c²
σ      = gauge · ρ_PET,      ρ_PET ≈ 1390 kg/m³
```

`ρ_PET` is the bulk density of polyester film (Mylar/Hostaphan datasheets give
1.38–1.40 g/cm³); heads are quoted in **mil**, so a 10 mil head is 0.254 mm and
`σ = 0.353 kg/m²`. The model's shipped batter `SurfaceDensityKgPerM2 = 0.35`
**is** a 10 mil single-ply head to within a percent, so the surface density,
unlike the tension, was evidently chosen physically. The licensed drum's stated
Remo gauges make σ a known quantity for it too.

Worked example, the shipped default, checkable against the repository:
`R = 0.1524 m`, `f(0,1) = 150.08 Hz` gives `c = 59.76 m/s` and
`T = 0.35 · 59.76² = 1250 N/m` — exactly the shipped value and exactly the `c`
quoted in
[`physical-calibration.md`](physical-calibration.md#why-the-k1-term-exists), so
the formula chain verifies before it is pointed at a drum.

**Three cautions, in decreasing order of importance.**

1. **Use the single-head fundamental**, `f_single`. The coupled `f_lower` is
   shifted by the cavity, and feeding it into this formula bakes the air spring
   into the tension.
2. **The model's `T` is an effective tension.** A measured `T` and a fitted `T`
   are not required to agree, and the gap is itself informative. It is no longer
   the estimate of missing exterior air loading it was once described as: on the
   licensed drum, whose diameter is known, the measured mode series matches the
   **ideal** membrane to 11 cents and sits ~950 cents from an air-loaded
   prediction, so air loading is not what the gap measures.
3. **`R` is the vibrating radius**, not the head's nominal size. Measure the
   bearing-edge diameter and use half of it. Frequency goes as `1/R`, so a 3 mm
   error at `R = 152 mm` is 2 % in `f` and 4 % in `T`.

---

## The acceptance criterion

This is the part of the original document that survives unchanged in intent, and
it is what any measurement here is for.

**Commit the derived tables, plus a provenance sheet, under `testdata/`, and make
at least one test depend on a measured number.** The audio need not be committed
for a derived table to be usable — but the provenance must be, and
[`reference/CREDITS.md`](../reference/CREDITS.md) is the worked example of what
that sheet looks like: licence first, then the instrument as stated by the
source, then what was measured here rather than claimed, then per-file SHA-256s.

What a provenance sheet has to carry:

**The drum** — make, model, nominal size and depth; shell material and ply;
**measured bearing-edge diameter** and shell depth; vent count and diameter; how
it was mounted, because a mount is a boundary condition and moves the
fundamental's decay by an order of magnitude on its own (F&R §18.4: 5.5 s → 0.6 s
with support-arm length).

**The heads** — model and gauge in mil, and age; the tuning procedure; lug count
and rod positions or turn counts; for the doublet, an explicit statement that the
batter rods were not touched.

**The rig** — microphone make, model, polar pattern, distance, angle, height and
its radius on the head as a fraction of `R`; interface and gain; sample rate and
bit depth; any filtering in the chain (a preamp high-pass destroys the
fundamental's level and must be off); the stick or mallet, tip material and total
mass in grams (`Strike.MalletMassKg` ships at 0.015).

**The room** — approximate dimensions, treatment, drum position, and the result
of the move-the-drum control; ambient noise floor in dBFS relative to the take's
peak.

**The session** — date, temperature and relative humidity
(`Cavity.SoundSpeedMPerS = 343` is a 20 °C number and moves ~0.6 m/s per °C,
which matters for the transverse cavity modes); who played, and the dynamic.

**The licence**, in the first line. Without it the tables are unusable no matter
how good the measurement is.

**Checksums** — SHA-256 per file, so a rediscovered file can be verified as the
one a table came from.
[`physical-calibration.md`](physical-calibration.md#reference-set-provenance)
lists this requirement for any measured set added beside the synthetic fixture;
this is that list, made operational.

---

## Deriving the tables: `cmd/measure-tom`

```bash
# One or more takes: partial table, ratios, T60, ζ, plus repeatability
go run ./cmd/measure-tom -o tom-modes.json take-*.wav

# Fischer's doublet: single-head file first, two-head file second
go run ./cmd/measure-tom -doublet -o tom-doublet.json single.wav double.wav

# Everything the flags can change
go run ./cmd/measure-tom -h
```

It reuses `internal/physical/match` — the same WAV loader, onset finder, partial
detector and decay fitter that `cmd/fit-physical` measures a candidate with — so
a table produced here and a fit run later are measuring with one instrument.
There is deliberately no second FFT and no second peak picker in the tree.

That shared instrument is also the reason to read
[`physical-objective-validation.md`](physical-objective-validation.md) before
trusting a table from it: scored against itself on two coincident channels of the
same hit, its partial-frequency and partial-decay estimates do not reproduce, and
only the spectral-envelope term does. The doublet is still worth measuring — it
is a large frequency difference, not a fine one — but a per-mode T60 table from
this tool carries that uncertainty and must not be quoted to three figures.

Health lines come first in the output on purpose: peak amplitude, clipped sample
count, DC offset, pre-onset noise floor, and whether the analysis window ran off
the end of the file. Read those before the partial table; most of the failure
modes above show up there.

### Check the tool against the model before pointing it at a drum

The shipped voice is a signal whose answers are already written down, so run this
first — it takes ten seconds and verifies the loader, the onset finder, the
partial detector and the decay fit at once:

```bash
go run ./cmd/render-physical -o model-render.wav -duration 2s
go run ./cmd/measure-tom model-render.wav
```

Each printed line should match a number documented elsewhere in `docs/`: the
coupled `(0,1)`, the stiffened branch and their ratio against
`Cavity.StiffnessScale` in [`physical-cavity.md`](physical-cavity.md), ζ and the
base T60 against
[`physical-calibration.md`](physical-calibration.md#why-the-k1-term-exists), and
the coupled `(1,1)/(0,1)`. Re-derive the expected values from the current
configuration rather than from a table here: N4 retargets the cavity stiffness
and N5 refits against the licensed reference, and both move these lines.

Note what the same table says about the **ratio base**: the fundamental comes out
well below the strongest partial on the model's own output, because the attack
layer dominates the top of the band. That is why `-base-window-db` defaults to 30
and why `-base-hz` exists. On a recording, look at the table before believing the
ratios.
