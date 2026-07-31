# Measuring one real tom

This is the capture protocol for PLAN.md item **M3**. Its purpose is narrow and
worth stating first: **every number in `internal/physical` has so far been
checked against the model, or against a published measurement of a different
instrument.** The committed fixture `testdata/physical-reference-v2.json` is
generated from the model itself and is a regression reference, not an acoustic
one. The one recording ever fitted against is of unknown provenance, unknown
diameter, unlicensed, and not in this repository — so no test depends on it and
none may.

[`physical-tom-review.md`](physical-tom-review.md#where-the-literature-is-genuinely-thin)
states what the literature does not supply: no published modal table for a
mounted 12–14" tom, no measured felt-mallet contact time, no radiation-versus-
internal damping split for any drum, no published overall T60 for a tom hit. The
anchors actually in use are Rossing's **snare**, Sørensen's student **tom**
report and Fischer's student **snare** project. One afternoon with the drum
you already own replaces several of those with your instrument.

Everything below is written so that each capture ends in a **number that changes
a named field**. If a measurement cannot be traced to a field, it is not in this
document.

The derived tables are what gets committed. The audio does not have to be — see
[What to write down besides audio](#what-to-write-down-besides-audio).

---

## The minimum viable subset

If you have thirty minutes, do these three captures and nothing else. Together
they produce every table in this document except contact time and the strike
series.

| #   | Capture                                                      | Buys                                                                                            |
| --- | ------------------------------------------------------------ | ----------------------------------------------------------------------------------------------- |
| 1   | 10 centre hits, **resonant head removed**, batter untouched  | the uncoupled `(0,1)` → a **measured** `Batter.TensionNPerM`; half of the doublet               |
| 2   | 10 centre hits, **resonant head refitted**, batter untouched | the other half → a **measured** `Cavity.StiffnessScale`                                         |
| 3   | 10 hits at 0.30 R, one dynamic, resonant head on             | the modal table, the ratios, per-mode T60 → ζ(f), the `(1,1)` split, and the per-trigger jitter |

**Capture 1+2 is the single most valuable thing here**, and it is Fischer's
protocol applied to a tom. `Cavity.StiffnessScale` is fitted at **0.083**, a
factor of twelve below the rigid ceiling of 1, and the only measurement it is
anchored to is a **snare** — Fischer's 186 Hz with one head, 215 Hz with two, a
ratio of 1.16 ([`physical-cavity.md`](physical-cavity.md#why-the-air-spring-is-fitted)).
The rigid formula ρc²/V would put the shipped drum's doublet at a ratio of
1.87; the fit puts it at 1.15. That factor of twelve is currently explained by
"shell flex and the non-piston mode shape", which is an argument rather than a
measurement, and this pair of captures is the direct test of it.

Capture 3 is second because it produces four independent tables from one take,
and because everything in it is currently taken from the literature for a
different drum.

Do them in this order — 1 and 2 need the resonant head to move, and you want the
batter tuning to be the same in both:

1. two-head take (this is capture 2)
2. remove the resonant hoop and head, **counting rod turns** so it goes back
3. single-head take (capture 1)
4. refit the resonant head to the same rod positions
5. re-record capture 2 as a **bracket**

If the batter fundamental in step 5 differs from step 1's two-head take by more
than about 5 cents, the drum drifted while you worked and the split ratio you
derived is worth nothing. That check is the whole reason for step 5.

---

## Rig, common to every capture

**Microphone.** The model's own default pickup geometry is
`Pickup.Radius01 = 0.65`, `AngleRad = 0.6 rad`, `DistanceM = 0.03` — 3 cm above
the head, two-thirds of the way out toward the rim. **Record at that geometry**,
because then the measured partial levels and the model's `Pickup` term describe
the same observation point and `NearFieldScale` is being fitted to something
rather than to nothing. If you can only manage one distance, manage that one.

Prefer an **omnidirectional** measurement microphone. A cardioid at 3 cm has
tens of dB of proximity boost below 200 Hz — right where the fundamental lives —
and the model has no proximity term at all, so the fit will absorb your
microphone's low end into `Pickup.NearFieldScale` and `OutputGain` and call it
the drum. If a cardioid is all you have, use it, write the model down, and treat
the _partial levels_ as suspect while the _frequencies and T60s_ remain fine.
Frequency and decay do not care about the microphone's magnitude response.

**Gain staging.** 48 kHz (the model's default rate; `LoadReference` does not
resample, so matching it removes one variable), 24-bit, peaks between −12 and
−6 dBFS. Never let a single sample touch full scale: a clipped attack puts
broadband energy at every frequency and manufactures partials that are not
there. `cmd/measure-tom` counts clipped samples and refuses to be quiet about
them.

**Room.** Drum at least 1 m from every wall, out of corners, in the most damped
room you have. A 3.4 m room dimension puts axial modes at roughly 50, 100 and
150 Hz, and a tom's fundamental sits at 75–250 Hz — so a room mode can land on
top of the thing you are trying to measure and add or subtract several dB.
Two defences: the microphone is at 3 cm, where the direct field is enormous
relative to the reverberant one; and the **move-the-drum control** —
re-record one take with the drum moved half a metre and rotated. Frequencies and
levels below 300 Hz that move with the drum's position in the room are the
room's, not the drum's.

**Strikes.** One stick, one player, one dynamic per take, and let the head come
to rest between hits — at least **2 seconds**, more than twice the 1.2 s
`match.DefaultOptions().AnalysisSeconds`. A hit that lands on the tail of the
previous one measures the sum of two decays and reads as a longer T60 than the
drum has. Record silence for 2 s at the head of each file so there is pre-onset
material to measure the noise floor from.

**Damping the other head.** Do not use tape, gel or a wallet to "isolate" a
head. Every one of those adds mass and loss to the head you are measuring, which
is the same as changing σ and the loss law — the two things the capture exists
to determine. Fischer's protocol removes the head; so does this one.

---

## 1. Modal frequencies and their ratios

**What the model needs.** The mode series, and each partial's ratio to the
`(0,1)`. Ideal circular-membrane theory puts those ratios at the Bessel-zero
ratios `α(m,n)/α(0,1)`:

| mode  | Bessel zero | ratio to (0,1) |
| ----- | ----------- | -------------- |
| (0,1) | 2.4048      | 1.000          |
| (1,1) | 3.8317      | 1.593          |
| (2,1) | 5.1356      | 2.136          |
| (0,2) | 5.5201      | 2.295          |
| (3,1) | 6.3802      | 2.653          |

Real two-headed drums scatter **±20 % around that series in both directions** —
Rossing's snare gives `(1,1)/(0,1) = 1.25`, Sørensen's tom gives 1.85
([`physical-tom-review.md`](physical-tom-review.md#what-i-got-wrong-and-what-the-literature-settled)).
The model's coupled value is 1.54, against Richardson, Toulson & Nunn's
practical target of ≈1.5. **Your drum's number is currently unknown and is the
only one that can falsify any of this.**

**Lands in.** `physical.Head.RadiusM` and `physical.Head.TensionNPerM` set the
whole series together (see [The tension question](#the-tension-question)); the
observed spread of your ratios against the table above is the check on the ±20 %
claim; and the _splitting of each `(m,n)` pair into two peaks a few Hz apart_ is
a direct read of `physical.TensionAsymmetry.SplitRatio`, which ships at 0.004
(0.4 %) on the batter head and has never been measured.

**Capture.** Capture 3 of the minimum subset. Stick tip, struck at 0.30 of the
radius — the model's `Strike.Radius01` default, chosen because a centre hit
excites the axisymmetric family and almost nothing else. Microphone at the
default geometry above. Ten hits at one comfortable mezzo-forte, 2 s apart, in
one file or ten; `cmd/measure-tom` takes either.

**Analysis.** `go run ./cmd/measure-tom -o modes.json take3.wav`. The partial
table it prints is frequency, level relative to the strongest partial, ratio to
the base partial, T60 and ζ. The detector is `internal/physical/match`'s, which
reads two windows — a long one for the modes that ring and a short one for the
modes that do not — precisely so a loud short partial is not discarded for being
90 dB down in an 800 ms transform.

To see the `(1,1)` pair split you need resolution finer than the split itself.
At 0.4 % of a 240 Hz mode that is 1 Hz, so the default 65536-point transform at
48 kHz (0.73 Hz per bin, plus parabolic interpolation) is adequate but not
generous; if the two peaks merge, that is an upper bound on the split, not a
measurement of zero. Report it as `< 1 Hz`.

**What a bad measurement looks like.**

- **Clipping.** Manufactures partials everywhere. Non-negotiable; re-record.
- **A partial at 50 or 100 Hz (60/120 Hz)** that does not move when the drum
  moves and does not decay: that is mains hum, not a mode.
- **Partials below 300 Hz that shift when you move the drum**: room modes.
- **A "mode" 30–40 dB down sitting right on the shoulder of the fundamental**:
  spectral leakage. The detector's `PeakProminenceDB = 6` guard exists because
  exactly that produced a phantom 87 Hz mode on the old reference.
- **Ratios that drift across takes** by more than a few cents: the drum is
  detuning as you play, or the stand is resonating. Re-tune, re-take.

---

## 2. Per-mode T60, and the ζ-versus-frequency structure

**What the model needs.** The decay rate of each resolved partial, converted to
the fraction of critical damping `ζ = γ/ω` with `γ = ln(1000)/T60`. This is the
single most consequential shape in the model:
[`physical-calibration.md`](physical-calibration.md#why-the-k1-term-exists)
builds the whole loss law `γ(k) = d0 + d1·k + d2·k²` around the claim that ζ is
roughly **constant** above the fundamental — which is Skrodzka, Hojan & Proksza's
measurement of a **snare batter head**, ζ ≈ 1.1 %, and has never been checked on
a tom.

**Lands in.**

- `Head.Loss1MPerSecond` is literally `ζ·c`. Ships at 0.4303 m/s on the batter,
  which is ζ = 0.72 % at that head's `c = 59.76 m/s`. Your measured ζ, averaged
  over the partials above the fundamental, times your measured `c`, is the
  replacement.
- `Head.Loss0PerSecond` (0.8 /s) is the frequency-independent floor and
  `Head.Loss2M2PerSecond` (1.9e-5) the excess high-frequency term. Fit them only
  if the measured ζ(f) genuinely tilts; a flat ζ says leave them small.
- `Head.ModeDecayCorrections` is the per-mode residual. The batter's `(0,1)`
  carries `DecayRatePerSecond = 24.6` — the largest single hand-set number in
  the loss model, standing for the energy the fundamental loses into the cavity
  and the opposite head. Compute yours as
  `Δ = ln(1000)/T60_measured − (d0 + d1·k + d2·k²)` at `k = α(0,1)/R`.
  Skrodzka's snare puts the `(0,1)` at ζ = 5.07 % against ~1.1 % for its
  neighbours; the model at 3.4 % and a 213 ms T60. **If your tom's fundamental
  is not the shortest partial in the low band, that is a genuine finding and the
  correction is wrong.**

**Capture.** The same file as measurement 1. Nothing extra to record.

**Analysis.** The `t60Seconds`, `dampingRatioPercent` and `fitQuality` columns.
The fit is log-linear on the heterodyned envelope of each partial over
`DecayFitStartSeconds = 0.05` to `DecayFitEndSeconds = 0.60`, stopping at
`DecayFitFloorDB = -45`. `fitQuality` is R²; anything below about 0.9 means the
envelope is not a single exponential — a beating pair of nearly-degenerate
modes, or a partial that fell into the noise floor before the window closed.
Use those frequencies, distrust those T60s.

**What a bad measurement looks like.**

- **Hits closer than ~2 s.** The previous hit's tail is still there and every
  T60 comes out long. This is the most common way to get a plausible, wrong
  number.
- **A noise floor less than ~45 dB below the peak.** Then the fit window ends
  in the room, not in the drum, and it reads long. `cmd/measure-tom` reports the
  pre-onset floor for exactly this reason; if it is above −45 dBFS relative to
  peak, shorten `-decay-end` or get a quieter room.
- **The room's own reverberation.** `match.DefaultOptions().AnalysisSeconds` is
  1.2 s because past that a close recording is mostly room. If your RT60 is
  comparable to the drum's, the long partials are unmeasurable in that room at
  that distance and only the short ones are trustworthy.
- **A hand still on the head, or the drum still in a stand that rattles.**
  Shows up as a low `fitQuality` on everything at once.

---

## 3. The `(0,1)` doublet, with and without the resonant head

This is Fischer's protocol, applied to a tom instead of a snare, and it is the
direct measurement of the quantity `Cavity.StiffnessScale` is fitted to.

**What the model needs.** Three frequencies:

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

Note that Fischer's published 186 → 215 Hz, ratio 1.16, is a
before-and-after of _one_ number, and the repository has used it as a target for
`f_upper/f_lower`. Measuring all three frequencies is what finally
disambiguates those two readings, which is a second reason this capture is worth
more than any other here.

**Lands in.** `physical.Cavity.StiffnessScale` (0.083, fitted). Also
`Cavity.DepthM`, `AirDensityKgPerM3` and `SoundSpeedMPerS`, which are geometry
and air and should simply be set to your drum's and your room's. If your
measured `f_upper/f_lower` comes out near the rigid-formula prediction, the
factor of twelve is an artefact of the model and not a property of shells; if it
lands in Fischer's 10–20 % band, the shipped fit is confirmed **on a tom** and
one of the paper's fitted rows becomes a measured one.

**Capture.** Captures 1 and 2 of the minimum subset. **Strike dead centre**, not
at 0.30 R: the doublet is an axisymmetric phenomenon, a centre hit excites `m=0`
and very little else, and every `m>0` mode you avoid exciting is a partial that
cannot be confused for the stiffened branch. Same microphone position for both
takes — move nothing but the head.

The batter's tuning rods must not be touched between the two takes. Mark them.
If your drum's resonant hoop and batter hoop share a lug body (most do), simply
removing the resonant head slightly changes the lug loading, so the bracket take
in step 5 above is not optional.

**Analysis.**

```bash
go run ./cmd/measure-tom -doublet -o doublet.json single-head.wav two-head.wav
```

The first file is the one with the resonant head **off**. The tool reports
`f_single`, `f_lower`, `f_upper`, the split ratio `f_upper/f_lower`, and both
ratios against `f_single`, plus every candidate partial above the lower branch
so the choice of `f_upper` is visible and checkable rather than asserted. The
search window is `-doublet-max-ratio`, default 1.45: below the ≈1.5 where the
`(1,1)` family sits and comfortably above the measured 10–20 % band. If your
drum's stiffened branch is above the window — which is what the rigid formula
predicts — raise the flag and say so in the notes; do not let the default hide
the answer.

**What a bad measurement looks like.**

- **The batter tuning moved.** The bracket take catches it. More than ~5 cents
  and the whole thing is void.
- **An off-centre strike** on either take. It excites the `(1,1)` family at
  ~1.5× the fundamental, and the tool will happily nominate one of those as the
  stiffened branch. Strike centre; the `(1,1)` should be at least 10 dB down.
- **Two different microphone positions.** The two takes are compared by
  frequency, so this is survivable, but the level comparison is then
  meaningless.
- **Confusing the vent.** A tom's vent is a Helmholtz port tuning near 30 Hz on
  a 12"×9" shell, far below the fundamental, and diverts a few percent of the
  flow at 150 Hz. Taping it over is a legitimate control — it should move the
  split ratio by **less than about 5 %** — but if it moves it a lot, that is a
  finding worth recording, because the repository's arithmetic says it cannot.
  Do not tape it over for the main takes.

---

## 4. A strike-position series

**What the model needs.** How the partial balance moves as the strike travels
from centre to rim. Item S5 measured, on the model, that the fundamental sits
2.07 dB below the strongest partial at 0.12 R, 7.23 at 0.22, 9.78 at 0.30 and
11.22 at 0.36 — **monotone, with no sweet spot**. That is a strong, falsifiable
claim about a real drum, and nothing has ever tested it.

**Lands in.** `physical.Strike.Radius01` (0.30). Also, less directly, the
excitation model: if the measured series is _not_ monotone, the mode-shape
weighting `StrikeAccelerationPerN` is missing something.

**Capture.** Five hits each at approximately 0, 0.15, 0.30, 0.45 and 0.70 of the
radius, along one radial line, at one dynamic, microphone unmoved. Mark the
positions on the head with removable tape at the edge so they are repeatable and
so the number in your notes is real. Same file or five files.

**Analysis.** `go run ./cmd/measure-tom -o strike.json hit-*.wav` and read the
`levelDB` column of the base partial across the takes. The levels are relative
to each take's own strongest partial, so this is a **balance** series and is
immune to your gain staging — which is the point.

**A companion worth ten minutes: the microphone series.** S4 made
`Pickup.Radius01` the strongest timbral control in the model, and it is equally
unfalsified. Fix the strike at 0.30 R and move the microphone instead: 0, 0.35,
0.65 and 0.9 of the radius at the same 3 cm height, then 0.65 R at 3, 10 and
30 cm. The far-field term and the evanescent near-field term have _different_
distance laws — geometric spreading versus `exp(-z·d/R)` — so a distance series
is the only thing that can separate them, and `Pickup.NearFieldScale` is
currently a pure fit.

**What a bad measurement looks like.** The player's strike position drifting
(mark the head), the dynamic drifting between positions (the balance is not
level-invariant if the head goes nonlinear — see the glide), and the microphone
being nudged.

---

## 5. Repeated hits at one dynamic

**What the model needs.** How much a real drum varies hit to hit at nominally
constant input: the spread in fundamental frequency, in partial balance, and in
decay.

**Lands in.** Nothing yet — and that is the point. PLAN item S7 proposes
per-trigger jitter of the mode frequencies, after Cook's PhISEM ("the
implemented resonance frequencies are randomly varied with each collision
event"), and there is **no config field for it today**. This measurement is what
would size one; without it, any jitter amount would be invented. Note that
`TensionAsymmetry` already handles the _static_ degenerate split, so what is
being sized here is only the per-event part.

Two of the spread's causes are already in the model and must be subtracted
before anything is attributed to jitter: velocity varies hit to hit, and the
Berger nonlinearity makes pitch depend on velocity (102.8 cents of glide at full
velocity, 3.0 at low — `Nonlinearity.BatterTensionCoefficientNPerM3`). So a
frequency spread that **correlates with the take's peak level** is the
nonlinearity working correctly, not jitter.

**Capture.** Capture 3 again — ten hits at one dynamic is both the modal table
and this. Twenty is better if the player can hold a dynamic.

**Analysis.** With more than one file and without `-doublet`, `cmd/measure-tom`
emits a `repeatability` block: mean base frequency, standard deviation and
peak-to-peak spread in cents, the same for the base partial's T60, and the
spread of the attack balance. Plot the per-take base frequency against the
per-take peak amplitude before concluding anything.

**What a bad measurement looks like.** A player who cannot hold a dynamic (the
spread is then the player), a drum that detunes over the take (the spread has a
trend — check first five against last five), and a stand that moves.

---

## 6. Contact time across dynamics

**Read this section's caveat before buying anything.** Wagner measured 7.5 ms at
piano and ~4.7 ms at forte at the centre and 3.5 ms at the rim, with a
force-instrumented stick; Dahl, Grossbach & Altenmüller measured 4.5–8 ms across
four professional players. The repository's contact model already predicts
7.4 ms for its 15 g mallet, inside that range, **and predicted rather than
prescribed**. So this measurement has the least headroom of anything here.

**A piezo taped to a stick does not measure contact time.** It measures the
stick's bending response to the impulse, whose duration is set by the stick's
own modes (Wagner reports stick modes at 400 Hz, 1 kHz and 1.7 kHz). What it
gives cleanly is a **trigger** and hit-to-hit **timing**, which is useful for
measurement 5 and not for this one.

**What can be measured cheaply, and what it lands in.** The tip's own
parameters, by taking the head out of the loop:

- `Strike.Contact.StiffnessNPerMAlpha` (1e6 N/m^α) and
  `Strike.Contact.Exponent` (1.5).

Strike a **massive rigid surface** — a concrete block, a thick steel plate —
with the piezo stick, dropping the tip from measured heights so the impact speed
is known, `v = sqrt(2gh)`. Against an immovable surface the contact duration is
set by the tip alone, and Hertzian contact gives
`τ ∝ v^(-(α-1)/(α+1))`. The slope of `log τ` against `log v` therefore yields α
directly, and the absolute τ at a known v and stick mass yields the stiffness.

This is worth doing precisely because the repository already notes that the
on-head contact time is **head-dominated** — over four decades of tip stiffness
the predicted contact runs 14.5 ms down to 7.1 ms and then stops moving, and
1e6 was chosen for sitting on that plateau. The rigid-surface experiment is the
one configuration in which the tip stiffness is _not_ masked, which makes it the
only cheap way to check that choice at all.

**What a bad measurement looks like.** A surface that is not rigid (a table
top rings, and you measure the table); a piezo whose own resonance is inside the
band of interest — check by tapping it directly; and inferring `v` from anything
other than a measured drop height.

**My recommendation: do this last, or not at all.** It constrains two parameters
that the model itself says the sound is insensitive to, and the three captures
of the minimum subset each constrain something the sound is _very_ sensitive to.

---

## The tension question

`internal/physical/config.go` documents why `TensionNPerM` ships at 1250 with
unusual candour: it was chosen **pitch-first**. At the previous 600 N/m the
12-inch batter head's fundamental was 104 Hz, "which is a floor tom"; 1250 puts
it at 150.08 Hz and puts the usable pitch mid-travel of the 300–3500 N/m range
instead of against the stop. No tension was ever measured.

Your drum can supply a real number. For an ideal circular membrane,

```
f(0,1) = α01 · c / (2πR),    α01 = 2.404826
c      = 2πR · f(0,1) / α01
T      = σ · c²
σ      = gauge · ρ_PET,      ρ_PET ≈ 1390 kg/m³
```

`ρ_PET` is the bulk density of polyester film (Mylar/Hostaphan datasheets give
1.38–1.40 g/cm³); drum heads are quoted in **mil** (thousandths of an inch), so
a 10 mil head is 0.254 mm and `σ = 0.353 kg/m²`. That is worth noticing: the
model's shipped batter `SurfaceDensityKgPerM2 = 0.35` **is** a 10 mil single-ply
head, and the resonant head's 0.25 is a 7 mil one, to within a percent. The
surface density, unlike the tension, was evidently chosen physically.

Worked example, the shipped default, which you can check against the repository:
`R = 0.1524 m`, `f(0,1) = 150.08 Hz` gives `c = 2π·0.1524·150.08/2.404826 =
59.76 m/s`, and `T = 0.35 · 59.76² = 1250 N/m`. That is exactly the shipped
value and exactly the `c` quoted in
[`physical-calibration.md`](physical-calibration.md#why-the-k1-term-exists) — so
the formula chain is verified before you point it at your own drum.

**Three cautions, in decreasing order of importance.**

1. **Use the single-head fundamental**, `f_single` from capture 1. The coupled
   `f_lower` is shifted by the cavity, and feeding it into this formula bakes
   the air spring into your tension.
2. **The model's `T` is an effective tension.** It absorbs the exterior air
   loading, which the model does not have and which lowers a real head's
   frequencies. So a measured `T` and a fitted `T` are **not required to
   agree**, and the gap is itself the interesting number — it is an estimate of
   how much the missing air loading is worth. The review already concluded air
   loading is not a priority at this diameter, on the grounds that real drums
   scatter ±20 % around the ideal series anyway; a measured gap would turn that
   judgement into a number.
3. **`R` is the vibrating radius, not the head's nominal size.** A "12-inch"
   head seats on a bearing edge somewhat inside its nominal diameter. Measure
   the bearing-edge diameter with a tape and use half of it. The frequency goes
   as `1/R`, so a 3 mm error at `R = 152 mm` is 2 % in `f` and 4 % in `T`.

Report both numbers in your notes: the tension your drum actually carries, and
the tension the fit wants. Do not silently replace one with the other.

---

## What to write down besides audio

M3 requires that the **derived tables** be committable so that a test may
finally depend on a measurement. The audio need not be. What makes the tables
usable a year later is the provenance sheet — record all of it, in the same
directory as the JSON:

**The drum**

- make, model, nominal size and depth (e.g. "Yamaha Stage Custom 12"×8"")
- shell material, ply count and thickness; bearing-edge profile if known
- **measured bearing-edge diameter** (see caution 3 above) and shell depth
- number and diameter of vents
- how it was mounted: tom arm, snare stand, on a cushion — a mount is a boundary
  condition

**The heads**

- batter and resonant model and gauge in mil, and their age
- the tuning procedure: which reference, tuned by ear or by gauge, in what
  order, cross-pattern or sequential
- lug count, and rod positions marked or turn counts recorded, so the batter
  tuning is provably unchanged between the doublet takes
- for the doublet: an explicit statement that the batter rods were not touched

**The rig**

- microphone make and model, polar pattern, distance, angle, height, and its
  radius on the head as a fraction of `R`
- preamp/interface, gain setting, sample rate, bit depth
- any filtering in the chain — a high-pass on the preamp will destroy the
  fundamental's level and must be off
- the stick or mallet: make, tip material and shape, total mass in grams
  (`Strike.MalletMassKg` ships at 0.015)

**The room**

- approximate dimensions, floor and wall treatment, drum position relative to
  walls, and the result of the move-the-drum control
- ambient noise floor in dBFS relative to the take's peak (the tool reports it)

**The session**

- date, temperature and relative humidity — `Cavity.SoundSpeedMPerS = 343`
  is a 20 °C number and moves about 0.6 m/s per °C, which matters for the
  transverse cavity modes
- who played, and the dynamic they were holding

**The licence.** State it explicitly, in the directory, in the first line of the
provenance file. The repository is MIT and the derived tables should be too;
whether the raw audio is published is a separate decision and the tables must
not depend on it. Without a licence line the tables are as unusable as the
current reference recording, which is exactly the problem M3 exists to fix.

**Checksums.** If the audio is kept out of the repository, record the SHA-256 of
each WAV in the provenance file, so a future reader can verify that a rediscovered
file is the one the table came from.
[`physical-calibration.md`](physical-calibration.md#reference-set-provenance)
already lists this requirement for any measured set added beside the synthetic
fixture; this is that list, made operational.

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

It prints a human-readable table to stdout and writes JSON with `-o`. The JSON
is what gets committed; it carries the full `match.Options` used, so a table can
be reproduced from itself.

Health lines come first in the output on purpose: peak amplitude, clipped sample
count, DC offset, pre-onset noise floor, and whether the analysis window ran off
the end of the file. Read those before reading the partial table. Most of the
failure modes above show up there.

### Check the tool against the model before you point it at the drum

The shipped voice is a signal whose answers are already written down, so run
this first — it takes ten seconds and it verifies the whole chain:

```bash
go run ./cmd/render-physical -o tom.wav -duration 2s
go run ./cmd/measure-tom tom.wav
```

| what it prints            | where the number is written down                                                               |
| ------------------------- | ---------------------------------------------------------------------------------------------- |
| base 153.74 Hz            | the coupled `(0,1)`, from a 150.08 Hz uncoupled head                                           |
| second partial 178.59 Hz  | the stiffened branch                                                                           |
| ratio 1.162               | `Cavity.StiffnessScale = 0.083` is fitted to give 1.16 (`config.go`)                           |
| ζ = 3.36 % on the base    | "near ζ = 3.4 %" ([`physical-calibration.md`](physical-calibration.md#why-the-k1-term-exists)) |
| T60 = 0.213 s on the base | the 0.21 s the `(0,1)` decay correction is anchored to                                         |
| 237.80/153.74 = 1.547     | the model's coupled `(1,1)/(0,1)`, documented as 1.54                                          |

If those six lines come out, the loader, the onset finder, the partial detector,
the decay fit and every ratio in this document are working. The render has no
lead-in, so it warns about the missing pre-roll — which is itself a useful
demonstration of what that warning looks like.

Note what the same table says about the **ratio base**: the fundamental is 26.7
dB below the strongest partial on the model's own output, because the attack
layer dominates the top of the band. That is why `-base-window-db` defaults to
30 and why `-base-hz` exists. On your recording, look at the table before
believing the ratios.
