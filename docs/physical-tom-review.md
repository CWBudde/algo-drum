# Physical tom review — measured diagnosis and literature check

Review date: 2026-07-30
Scope: `internal/physical/*`, `internal/drum/physical_tom.go`, `docs/physical-*.md`

Every number attributed to the model was measured from `physical.DefaultPhysicalDrum()`
at 48 kHz, velocity 0.8. Every number attributed to a real drum carries a
citation.

The diagnostic programs and figures are **not committed** — they were throwaway
tooling. Each measurement below states the configuration it came from, so any of
them can be reproduced against `internal/physical` directly; the mode table is a
loop over `DoubleHead.BatterMode`, the timings are `Render` over a 48000-sample
buffer, and the spectra are an FFT of `Tick().Radiated`. `cmd/render-physical`
already renders an auditionable WAV. Figure references (`NN-*.png`) name the
plot each conclusion came from, for the record.

## Verdict

The implementation is careful and correct on its own terms: Bessel eigenmodes
with the right labels, exact damped state transitions, a passive rank-one
cavity solve, a discrete-gradient Berger update, honest energy bookkeeping.
There is no bug in the mechanics.

It sounds wrong because of **four calibration errors and one architectural
mismatch**, and because the compute budget is spent on the parts that are least
audible. Ranked by audible return:

| #   | Defect                                          | Measured                                       | Should be                                   |
| --- | ----------------------------------------------- | ---------------------------------------------- | ------------------------------------------- |
| 1   | Damping ~10× too weak, and flat vs. frequency   | γ = 3.1 /s for every mode; T60 1.8–2.3 s       | γ = 11–41 /s, T60 ∝ 1/f                     |
| 2   | The fundamental is the **longest**-ringing mode | (0,1) T60 = 2213 ms                            | (0,1) T60 ≈ 209 ms — the _shortest_         |
| 3   | No usable bandwidth                             | highest mode **646 Hz**                        | audible content to several kHz              |
| 4   | Cavity coupling ~5× too strong                  | (0,1) doublet 107.7 / 219.4 Hz, ratio **2.04** | ratio ≈ **1.16**                            |
| 5   | Pitch glide inaudible but expensive             | 38 cents, costs 6× the voice                   | "a few tenths of a second" of audible glide |

The table is the 2026-07-30 diagnosis, left as written so the corrections stay
attributable to a measurement. Every row has since been addressed — see the
implementation notes below — though rows 3 and 5 were addressed differently from
how they are described here: the bandwidth came from a hybrid noise layer rather
than from more modes, and the glide's cost premise turned out not to exist. The
numbers in the table describe the voice as diagnosed, not as it now stands.

Two things I initially flagged turned out to be **fine**, and the literature is
what corrected me — details in "What I got wrong" below.

One thing this review **missed entirely**, found later the same day by listening
rather than by measurement: the whole drum was tuned about a fifth too low
(104 Hz on a 12-inch head), and the tuning control silently changed the decay as
well as the pitch, because the ζc coefficient was stored as an absolute constant.
Every frequency in this document is therefore from a drum tuned to 600 N/m,
where the default is now 1250. See PLAN.md items S9 and S10, and
[`physical-calibration.md`](physical-calibration.md#tuning-and-constant-q).

## The measurements

### 1 & 2. Damping is the biggest error (11, 06, 05)

The loss law is `γ(k) = d0 + d2·k²` with `d0 = 3 /s`, `d2 = 2e-5 m²/s`. Over
the retained wavenumber range the `d2·k²` term contributes 0.005 /s at the
fundamental and 0.22 /s at the top — i.e. nothing. Measured against the only
published modal damping data I could find for a two-headed drum
(Rossing et al.'s snare batter head, reproduced in
[Skrodzka, Hojan & Proksza, _Archives of Acoustics_ 31(3) 2006, Table 1](https://acoustics.ippt.pan.pl/index.php/aa/article/download/674/592),
converted via T60 = ln(1000)/(ζ·ω) and mapped onto this model's frequencies):

| Mode  | f (Hz) | model γ | model T60 | model implied ζ | measured ζ | target T60 |
| ----- | ------ | ------- | --------- | --------------- | ---------- | ---------- |
| (0,1) | 104.0  | 3.12 /s | 2213 ms   | 0.0048          | **0.0507** | **209 ms** |
| (1,1) | 165.4  | 3.06 /s | 2258 ms   | 0.0029          | 0.0110     | 604 ms     |
| (2,1) | 221.8  | 3.05 /s | 2261 ms   | 0.0022          | 0.0107     | 463 ms     |
| (0,2) | 238.9  | 3.49 /s | 1980 ms   | 0.0023          | 0.0116     | 397 ms     |
| (3,1) | 275.7  | 3.06 /s | 2255 ms   | 0.0018          | 0.0077     | 518 ms     |
| (2,4) | 646.1  | 3.86 /s | 1790 ms   | 0.0010          | ~0.010     | 170 ms     |

Three separate findings in that table:

- **Everything is ~4–11× under-damped.**
- **The implied ζ _falls_ with frequency** (0.0048 → 0.0010), so Q _rises_ with
  frequency — backwards. Skrodzka et al. found the opposite structure: ζ ≈ 1.1 %
  roughly **constant** above the fundamental, i.e. T60 ∝ 1/f (figure 11).
- **The fundamental should be the fastest-decaying mode, not the slowest.**
  Skrodzka et al. measured ζ = 5.07 % on the (0,1) and state the cause
  explicitly: "This mode is relatively highly damped (5.07 %) **due to the
  coupling between the two heads**", the coupling "being a decreasing function
  of frequency". The model has it inverted: a 104 Hz sine ringing for 2.2 s
  under everything else. **That is the "boing".**

There is also a structural problem with the basis. Constant Q means γ ∝ ω ∝ k
for a membrane. `d0 + d2·k²` has **no k¹ term**, so it cannot express constant
Q at all — and raising `d2` alone (which is what I first recommended) gives
T60 ∝ 1/f², too steep at the top and still wrong at the bottom (figure 11,
dotted curve). The law needs a `d1·k` term; for ζ = 1.1 % and this head's wave
speed c = 41.40 m/s, `d1 = ζ·c = 0.455 m/s`.

Note too that no exposed control can reach these values: `DAMP` maxes at 12 /s,
below the 33 /s the fundamental alone needs, and both `DAMP` and the strip
`DEC` scale all terms uniformly so they never change the tilt.

### 3. No usable bandwidth (04, 08)

`Quality.ModeLimit()` is 24/48/96 oscillators filled in frequency order, so the
batter bank spans **104.0 → 646.1 Hz**. The `FrequencyLimitFraction·SampleRate`
guard (21.6 kHz) never binds. Above 646 Hz the output is empty (figure 04).

This is the method, not a setting. For a membrane the mode count grows as f²:
N(f) ≈ (a·k)²/4, so this head needs ~130 modes for 1 kHz, ~530 for 2 kHz,
**~3300 for 5 kHz** (figure 08). `docs/physical-model-research.md` chose modal
synthesis for its "small real-time state"; that premise is exactly what fails
for a 2-D radiator.

The literature's resolution is not "use more modes":
[Kirby & Sandler, DAFx-20](https://dafx2020.mdw.ac.at/proceedings/papers/DAFx2020_paper_66.pdf)
analysed 416 close-miked tom samples and found "as few as **5–10 key modes**
could be sufficient to replicate convincing central strikes" — because they
model the bright attack (**~1–8 kHz**) as a _separate stochastic/near-field
component_, not as modal excitation. That is the architecture this project
needs, and it is cheaper than what it currently runs.

### 4. Cavity coupling is ~5× too strong — and I had this backwards

Isolating the air spring by disabling the resonant head and striking dead
centre:

```
batter (0,1): f = 104.004 Hz, modal mass 6.883e-3 kg, swept area 3.150e-2 m²
cavity: V = 1.459e-2 m³, K = ρc²/V = 9.707e6 Pa/m³
analytic single-head stiffening: Δω² = 1.400e6 → 215.1 Hz (+106.8 %)
measured, batter only, cavity off → 104.00 Hz
measured, batter only, cavity on  → 191.89 Hz
```

So the code faithfully implements the rigid-cavity lumped air spring. With both
heads the (0,1) splits into the expected parallel/antiparallel pair
([Rossing, Bork, Zhao & Fystrom, _JASA_ 92(1) 1992](https://pubs.aip.org/asa/jasa/article/92/1/84/959717/Acoustics-of-snare-drumsAcoustics-of-snare-drums):
"mode pairs in which the heads move parallel or antiparallel"), measured here as:

```
two-head, cavity off → 104.0 (0 dB)  238.8 (−23.1)  375.0 (−22.2)  512.3 (−14.6)
two-head, cavity on  → 107.7 (0 dB)  219.4 ( −9.0)  383.4 (−19.7)  514.5 ( −9.4)
```

The doublet is **107.7 / 219.4 Hz, a ratio of 2.04**. The measured value for a
real drum is far smaller: [Fischer, _Modal Analysis of a Snare Drum_, Illinois 2014](https://courses.physics.illinois.edu/phys406/sp2017/Student_Projects/Spring14/Matthew_Fischer_Physics_406_Final_Project_Sp14.pdf)
measured the (0,1) at **186 Hz** with one head and **215 Hz** after adding the
resonant head at unchanged tuning — "this increase is due only to the coupling
between heads" — a ratio of **1.16**. The rigid-cavity formula over-predicts by
roughly 5× because the shell is compliant, the vent leaks, and the heads are
not pistons.

Consequences in the model: the audible fundamental barely moves (104.0 → 107.7)
where a real drum's rises ~16 %, and the stiffened branch lands at 219.4 Hz at
−9 dB — a loud spurious partial sitting on top of where (2,1) should be. My
first reading of the +3 Hz shift was that the coupling was too _weak_; isolating
it shows the opposite. **Fit the split to 10–20 % rather than deriving it from
ρc²/V.**

### 5. The glide is inaudible and costs 6× the voice

Measured on an i7-1255U, one voice, 48 kHz, native:

| Configuration                   | Oscillators | × realtime |
| ------------------------------- | ----------- | ---------- |
| default (cavity + nonlinearity) | 96          | **8.7**    |
| nonlinearity off                | 96          | 10.0       |
| nonlinearity off, cavity off    | 96          | 22.5       |
| batter only, uncoupled          | 48          | 54.4       |
| quality "high"                  | 192         | 6.9        |

`solveMidpoint` runs `nonlinearSolveIterations = 8` fixed-point passes over
every mode per sample, plus a second pass in `observe()` — 6× the uncoupled
path. What it buys, per the project's own audit, is a 106.3 → 104.0 Hz glide:
**38 cents**. `MaximumTensionRatio = 0.2` would permit up to 157 cents, so the
cap is not the limit; the coefficient is.

This matters more than I first thought. Every source that studied real toms
treats the downward glide as _the_ characteristic feature — Kirby & Sandler
build their whole model around it, and
[Avanzini & Marogna, _IEEE TASLP_ 18(4) 2010](https://avanzini.di.unimi.it/downloads/publications/avanzini_taslp10.pdf)
report glide durations of "a few tenths of seconds" that "closely resemble
those observed in a real membrane". So the answer is not to cut the
nonlinearity — it is to make it audible _and_ cheap. Avanzini's companion paper
([_JASA_ 131(1) 2012](https://pubmed.ncbi.nlm.nih.gov/22280712/)) shows the
short-time average tension variation is approximately proportional to system
energy, which gives a single-factor detune per sample instead of an 8-iteration
solve over 96 modes.

> **Half implemented 2026-07-30** as PLAN.md item S6. The glide is now 102.8
> cents at full velocity, up from 37.9, by raising the tension coefficients
> fourfold — measured, and stopped short of the `tanh` plateau where the glide
> would flatten into a hold-then-drop.
>
> The single-factor detune was **not** implemented, because its premise does not
> hold: the solve early-exits at a mean of **2.88** iterations, not 8, and
> sweeping the coefficient over a 32-fold range moves that only to 3.09. There is
> no 6× cost to buy back, so replacing the discrete-gradient solve would have
> given up its exact energy bookkeeping for nothing.

Also worth noting: Avanzini & Marogna warn that "using an **insufficient number
of modes** produces an **unnaturally slowly decaying tension**" — so the mode
truncation is corrupting the glide as well as the brightness.

Meanwhile the resonant head's 48 oscillators are computed and then discarded
from the output (`output.RawRadiated = output.BatterRawRadiated`).

> **Implemented 2026-07-30** as PLAN.md item S8, and stronger than stated here:
> 44 of those 48 are not merely discarded but provably never excited, so removing
> them is bit-exact. They paid for doubling the batter head's budget — 646 Hz to
> 915 Hz — and for the stochastic attack layer that covers 1–8 kHz. See
> [`physical-hybrid.md`](physical-hybrid.md).

### 6. Excitation: right duration, wrong force shape, missing second path (01, 07)

I initially called the 5.5–8 ms contact wrong. **It is well supported.**
[Wagner, KTH MSc 2006](https://www.speech.kth.se/publications/masterprojects/2006/AndreasWagner.pdf)
measured 7.5 ms piano and ~4.7 ms forte at the centre, 3.5 ms at the rim;
[Dahl, Grossbach & Altenmüller, Forum Acusticum 2011](http://www.sofiadahl.net/pdf/DahletalForumAcusticum2011.pdf)
measured 4.5–8 ms across four professional players. The audit's citation holds
and its correction of the old 0.71 ms pulse was right.

The errors are elsewhere:

- **The force _shape_.** `addContactPulse` prescribes a smooth half-sine over
  the entire contact interval. A smooth pulse of duration τ is −10 dB by
  1.02/τ and nulls at 1.5/τ, so a 6 ms half-sine is 17 dB down at the (1,1)
  mode and 40 dB down at the top of the bank (figure 01). The output does not
  peak until **21 ms** after the strike — no transient at all (figure 07).
  Wagner's measured force histories are not half-sines: they are asymmetric,
  perturbed mid-contact by the wave reflected off the bearing edge, and carry
  stick-mode ripple (stick modes at 400 Hz, 1 kHz, 1.7 kHz; the 400 Hz mode is
  +6 dB at the rim).
- **The nulls move with the knob.** A half-sine's spectral zeros land inside
  the mode bank and shift with hardness, so HARD re-picks which two or three
  modes survive rather than sweeping dark↔bright. That is why it feels arbitrary.
- **There is no separate attack path.** Kirby & Sandler's 1–8 kHz stochastic
  attack component is exactly what is missing, and it is why
  `physicalTomOutputGain = 4` was needed: the gain is compensating for an
  absent signal path, not for a mis-scaled pulse.

### 7. Radiation: two of my three criticisms survive

```go
RadiationWeight = PickupShape × (ka/√(1+ka²))^(m+1) × 1/(1+d/a)
RawRadiated    += RadiationWeight × velocity
```

- **The `(ka)^(m+1)` rolloff is defensible** — I was too harsh. For an
  acoustically small drum, far-field pressure follows net volume velocity, and
  m ≠ 0 modes "produce **null net displacement of the surrounding air**"
  (Avanzini & Marogna, citing Fletcher & Rossing §4.5); Skrodzka et al.
  measured that on a real snare the (1,1) "is **not** a strongly radiated
  mode". So suppressing m > 0 at low ka and letting it rise with frequency is
  the right _direction_. The exponent is uncalibrated, but it is not backwards.
- **Multiplying by `PickupShape` is wrong for the radiated path.** A far-field
  radiation efficiency and a near-field point mode shape are different objects.
  `PickupShape` changes sign and crosses zero with microphone angle, so modes
  get arbitrarily nulled — (5,1) sine lands at 2.3e-4 and (6,1) cosine at
  −7.4e-4 purely because of where the "microphone" sits. Keep the mode shape
  for the diagnostic contact pickup and for _strike_-position weighting, where
  it is correct; drop it from the radiated sum.
- **Summing velocity adds a spurious −6 dB/octave tilt.** Far-field pressure
  follows volume _acceleration_.

> **Implemented 2026-07-30** as PLAN.md item S4, with two of the three claims
> above corrected by measurement first.
>
> The **−6 dB/octave claim is wrong.** `(ka/√(1+(ka)²))^(m+1)` is ≈ ka at m = 0,
> so weighting velocity by it already carries the acceleration tilt. Multiplying
> by ω as well while keeping the exponent would have added about +10 dB of
> unjustified tilt across the retained band, which the refitted output gain would
> have concealed. In the acceleration domain the m = 0 weight must be
> frequency-independent, and a test now asserts exactly that.
>
> The **generalization of the swept area is not the naive one.** Extending
> 2πR²J₁(z)/z to 2πR²J\_{m+1}(z)/z and letting the existing rolloff carry the
> m > 0 normalization drops the multipole factor 1/(2^m·m!) — 1.03e7 at m = 8.
> Measured on the shipped basis it would have raised the (10,1) edge mode
> **401×**, to a quarter of the fundamental's weight, where multipole theory puts
> it seven orders down. The implemented weight is the exact Rayleigh/Lommel form
> 2πR²·z·J\_{m+1}(z)·J_m(u)/(z²−u²), which reduces identically to the swept area
> on axis and supplies 1/(2^m·m!) from J_m(u)'s own small-argument behaviour.
>
> Dropping `PickupShape` from the far-field weight was right, and the consequence
> is larger than expected: it leaves every m > 0 mode at least 23 dB down even
> with the microphone against the head. That is correct far-field physics for a
> head this size, and not what a close microphone hears, so the model gained an
> explicitly fitted evanescent near-field term alongside the far-field one — a
> sum, not the product the two used to form. See
> [`physical-calibration.md`](physical-calibration.md).

Correct weights, computed from the mode shapes: for the axisymmetric modes the
net-volume factor 2J₁(j₀ₙ)/j₀ₙ is 0.4318, −0.1233, 0.0627 for n = 1,2,3 — the
model already carries this as `SweptAreaM2`, it just isn't used in the output.

### 8. Strike position (02)

`Strike.Radius01 = 0.12` is nearly the geometric centre. Strike coupling then
spans **116 dB** across the retained modes: the axisymmetric (0,1) and (0,2)
take everything, (5,1) is 60 dB down, (7,1) 95 dB down. The audit chose 0.12
from Dahl's "central playing is normal", which is about the _region_, not the
geometric centre — Wagner's own centre measurements are at a nominal centre but
real playing scatters. Combined with defect 2, the output's loudest partial is
the (0,1) by 13 dB: a boom, not a pitched drum.

> **Implemented 2026-07-30** as PLAN.md item S5: the default moved to 0.30.
> Measured against strike radius, with the corrected microphone model, the
> fundamental sits 2.07 dB below the strongest partial at 0.12, 7.23 at 0.22,
> 9.78 at 0.30 and 11.22 at 0.36 — monotone, with no sweet spot. So the choice
> trades low-end weight for the (1,1) family rather than optimizing anything, and
> HIT.R exposes it. Note that the (1,1) already leads at 0.12 once the radiated
> sum is correct, so this deepens an effect S4 introduced rather than creating
> one.

## What I got wrong, and what the literature settled

- **Mode labelling.** I suspected the ratio↔label pairing (2.297 ↔ (0,2) vs
  (3,1)). Measured from the code: `(0,2) 2.2974`, `(3,1) 2.6511`. **The code is
  correct.** The mislabelling was in my research prompt.
- **Mode ratios / air loading.** I expected the missing exterior air loading to
  be a major defect. It is not, at this diameter. **Drop air loading from the
  priority list** — and that verdict, challenged in 2026-08 and re-tested, now
  rests on much better evidence than the argument I gave for it.

  The argument I gave was scatter: that real two-headed drums land ±20 % either
  side of the ideal Bessel series, so no fixed ratio set is right. That figure
  does not survive. It came from putting single-headed and coupled two-headed
  measurements in one column. Separated, uncoupled or single-headed instruments
  run **1.85–2.16** (Sørensen 1.85; Fletcher & Rossing Table 18.7's 12-inch tom
  2.16) and coupled two-headed ones run **1.52–1.56** (Gärder 2005) — two tight
  clusters, not one wide one. The model's 1.54 sits in the coupled cluster, where
  it belongs. Scatter of ±20 % was never measured on one class of drum, and an
  argument from it proves nothing either way.

  What settles it is a drum of **known geometry**. The licensed 8" × 8"
  reference states its diameter, so fixing the fundamental predicts every other
  mode with no free parameter at all. Measured (1,1)/(0,1) is **1.584** against
  the ideal membrane's **1.594** — an 11-cent match, and roughly **950 cents**
  from the ~1.78 an air-loaded head would give. There is no room in that for an
  added-mass term. Detail in
  [`physical-objective-validation.md`](physical-objective-validation.md) §"Exterior
  air loading is not the missing mechanism".

  [Richardson, Toulson & Nunn, _JASA_ 131(1) 2012](https://pubmed.ncbi.nlm.nih.gov/22280713/)'s
  practical target — tune the batter (1,1) to ≈**1.5×** the (0,1) — is unchanged,
  and this model's coupled 1.54 still meets it.

- **Cavity coupling** — I read the +3 Hz shift as under-coupling; isolating it
  shows ~5× over-coupling (defect 4).
- **Damping fix** — "raise d2 100×" was the wrong basis; constant Q needs a k¹
  term (defect 1).
- **Contact time** — well supported; the shape and the missing attack path are
  the real problems (defect 6).

The prototype in figures 09/10 also boosts m > 0 radiation, which the
literature says is backwards. It sounds better anyway because bandwidth and
damping dominate — **do not copy its radiation weighting.**

## Recommended plan

**Cheap and high-impact, do these first — no architecture change:**

1. **Rewrite the loss law** as `γ = d0 + d1·k + d2·k²` and calibrate to
   constant Q: `d1 ≈ 0.455 m/s` for ζ ≈ 1.1 %. Raise the `DAMP` ceiling well
   above 12 /s and add a damping _tilt_ control.
   **Implemented 2026-07-30** as PLAN.md item S1, at ζ = 1.1 % with `d0` cut to
   a small floor; ζ now holds 1.12–1.24 % across the retained band.
2. **Add the two-head coupling loss to the (0,1) modes specifically** —
   ζ ≈ 5 % → γ ≈ 33 /s at 104 Hz. `ModeDecayCorrections` already exists for
   exactly this. This single change is most of the difference between "boing"
   and "thump".
   **Partly done, and the remainder is now the highest-value fix on the path.**
   PLAN.md item S2 landed the correction on the batter head's own (0,1) on
   2026-07-30: that mode's T60 went from 2213 ms to 211 ms and suite RT60 from
   ~2.1 s to 0.27–0.55 s. What this item asked for and did not get is the loss on
   the **coupled doublet** — the pair the cavity creates, whose squeezing member
   pumps the air and should be heavily damped while its sliding partner is not.
   The model still has no such splitting, and the consequence is measurable: it
   synthesizes a **186 Hz mode with a T60 of 1.81 s**, the longest-ringing thing
   it produces and absent from the reference. That single mode is the top
   contributor to the spectral-envelope term, most of the spurious-partial share,
   and part of the frequency error — i.e. to the one metric that survived the
   objective's own reproducibility check. It is PLAN.md §P10/N3, and it is the
   single highest-value model fix outstanding. See
   [`physical-objective-validation.md`](physical-objective-validation.md) for the
   residual budget that identifies it, and note its warning: check the sign
   pattern of any fitted per-mode damping vector before believing it, because
   in-phase/out-of-phase splitting predicts pairwise alternation and a
   structureless vector is fitted noise.
3. **Refit the cavity split** to a 10–20 % (0,1) separation instead of ρc²/V,
   which will also remove the spurious −9 dB partial at 219 Hz.
   **Implemented 2026-07-30** as PLAN.md item S3. `Cavity.StiffnessScale = 0.04`
   moves the doublet from 108.4/219.7 Hz (ratio 2.03) to 105.5/123.0 Hz (ratio
   1.17), and the partial at 219.7 Hz is gone. One correction to the diagnosis
   above: the audible **lower** branch cannot rise 16 %, because eigenvalue
   interlacing pins it between the two heads' uncoupled (0,1) frequencies of
   104.0 and 112.3 Hz. It is the stiffened branch that carries the coupling
   shift, and the separation between the branches is what a fit can target.
4. **Fix the radiated sum**: volume acceleration weighted by `SweptAreaM2` for
   m = 0 and a directivity factor for m > 0; remove `PickupShape` from the
   radiated path. Then delete `physicalTomOutputGain`.
5. **Move the default strike radius** to ~0.3 of the radius.
6. **Make the glide audible and cheap**: raise the tension coefficient toward
   the 157-cent headroom the existing cap already allows, and replace the
   8-iteration solve with Avanzini's energy-proportional single-factor detune.
7. **Jitter mode frequencies per event** by a fraction of a percent
   ([Cook, PhISEM, ICMC 1996](https://quod.lib.umich.edu/i/icmc/bbp2372.1996.071/1/--physically-informed-sonic-modeling-phism-percussive):
   "the implemented resonance frequencies are randomly varied with each
   collision event") so repeated hits are not identical. `TensionAsymmetry`
   already handles the static degenerate split.

**Then the architecture:**

8. **Go hybrid.** Keep modal synthesis for the low, individually resolved band
   — Kirby & Sandler's 5–10 key modes up to a few hundred Hz, or the current
   48 if it is free — and add a **separate 1–8 kHz stochastic attack layer**
   driven by the contact force, with its own fast decay. That is where a tom's
   brightness actually comes from, it is what the published tom-analysis work
   does, and it costs a fraction of the ~3300 oscillators the equivalent modal
   bandwidth would need. Fund it from items 6 and from reducing the resonant
   head to the few axisymmetric modes that actually couple to the cavity.

Items 1, 2 and 4 alone should be audible immediately and are a few lines each.

## Evidence

A standalone prototype changing only mode budget, damping, contact time,
strike position and radiation weighting — same Bessel modes, same tension, no
new mechanisms — gives the blue curves in figures 09 and 10: spectrum out to
2.5 kHz instead of a cliff at 650 Hz, transient peaking at 1.5 ms instead of a
swell at 21 ms. It needed **699 oscillators** for that bandwidth, at ~3–4×
realtime native for a single uncoupled head — which is the whole argument for
item 8.

A/B: `current-model.wav` vs `prototype-fixed.wav`.

## Where the literature is genuinely thin

Two of the gaps listed here in 2026-07-30 have since closed and are struck from
the list. **A modal table for a real tom** now exists in the repository: the
licensed 8" × 8" reference supplies frequencies, ratios and per-mode decays for a
drum of stated diameter, depth and head models — though its per-mode T60s carry
the estimator uncertainty measured in
[`physical-objective-validation.md`](physical-objective-validation.md) and must
not be quoted finely. And **overall T60 for a tom** is published after all:
Fletcher & Rossing §18.4, Table 18.8 gives 0.8–1.05 s for a mounted 32 cm tom,
with the same passage showing the mount itself moving the fundamental's decay
5.5 s → 0.6 s with support-arm length. The 3–4 s figures that made this model's
0.27–0.55 s look wrong are wire-suspended artefacts.

Still thin: no measured felt-mallet contact time. No numeric
radiation-vs-internal damping split for any drum. No measurement of a tom vent
resonance — a first-principles estimate
puts a 12"×9" tom's Helmholtz frequency near 30 Hz, far below the (0,1), so
**do not model a vent resonance**. The ζ→T60 conversions, the departure
percentages and the doublet ratios in this document are my own arithmetic from
cited inputs; recompute before committing them to code.

## Figure index (uncommitted)

| File                                       | Shows                                                |
| ------------------------------------------ | ---------------------------------------------------- |
| `01-excitation-spectrum.png`               | contact-pulse spectra vs. the mode bank's span       |
| `02-mode-map.png`                          | flat per-mode T60; 116 dB spread in per-mode weights |
| `03-mode-ratios.png`                       | model vs. two measured drums                         |
| `04-output-spectrum.png`                   | the 646 Hz cliff                                     |
| `05-decay-envelope.png`                    | 2.2 s overall decay                                  |
| `06-band-decay.png`                        | all octave bands decaying in parallel                |
| `07-waveform.png`                          | attack peaking 21 ms after the strike                |
| `08-mode-density-wall.png`                 | Weyl mode count vs. the quality budgets              |
| `09-prototype-comparison.png`              | tonal balance, current vs. prototype                 |
| `10-attack-comparison.png`                 | attack shape, current vs. prototype                  |
| `11-damping-law.png`                       | shipped law vs. measured ζ vs. constant Q            |
| `current-model.wav`, `prototype-fixed.wav` | listen                                               |
