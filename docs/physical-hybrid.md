# Physical drum P8: the hybrid voice

P8 corrects the microphone model and then changes the shape of the synthesizer:
the resolved low band stays modal, and everything above it becomes a stochastic
layer driven by the same contact force. This document covers the architectural
half. The corrected radiated sum is in
[`physical-calibration.md`](physical-calibration.md); the loss law and the cavity
fit are in that file and in [`physical-cavity.md`](physical-cavity.md).

## Why modal synthesis alone cannot do it

A membrane's mode count grows as the square of frequency,
\(N(f) \approx (ak)^2/4\), so the default 12-inch head needs roughly 130
oscillators to reach 1 kHz, 530 for 2 kHz and **3300 for 5 kHz**. The voice
shipped with 48 per head and a top retained mode at 646 Hz — below the fourth
partial of a snare drum's fundamental, and with nothing at all where a stick
sounds like a stick.

Two consequences follow, and they point in the same direction:

- Buying bandwidth modally is a losing trade. Bandwidth grows only as
  \(\sqrt{N}\), so doubling the oscillator count buys 0.6 of an octave.
- Above roughly 1 kHz the modes are not individually resolvable anyway. They are
  dense, they decay fast, and they are heard as a broadband transient, not as
  pitch. Kirby and Sandler (DAFx-20) found 5 to 10 modes sufficient for the
  _sustain_ of a central strike precisely because the attack belongs to a
  separate object.

So the high band is modelled as filtered noise. This is cheaper than what the
voice ran before, not more expensive.

## Reclaiming the resonant head

`Head.AxisymmetricOnly` retains only the m = 0 modes, and on the resonant head
it is **bit-exact** rather than an approximation. Nothing can excite an m > 0
resonant mode:

- the strike force is added only to batter modes;
- the cavity is the only path between the heads, and it couples through the
  swept area, which is exactly zero for every m > 0 mode.

Their displacement, their contribution to the Berger strain measure and their
stored energy are therefore exactly zero for all time. Removing them changes the
rendered output not approximately but not at all, which
`TestAxisymmetricResonantHeadIsBitExact` asserts by comparing two renders sample
for sample. The resonant head goes from 48 oscillators to **4**.

Two details worth keeping in mind:

- The comparison uses `==`, not the bit patterns. The only arithmetic difference
  is that several accumulators receive fewer additions of exact zero, and
  \(x + 0\) equals \(x\) for every \(x\) — but it maps \(-0\) to \(+0\), so a
  fully silent stretch can legitimately differ in the sign of its zero.
- The filter runs **after** mode selection, not during it. `generateHeadModes`
  fills a slot budget, so skipping m > 0 candidates inside the selection loop
  would free their slots and the loop would refill them with higher-order
  axisymmetric modes — modes that do drive the cavity. That is a different
  instrument, not a cheaper one.

## Spending the reclaimed budget

`Quality.ModeLimit()` is the batter head's budget, and only the batter head's: the
resonant head runs the same selection, keeps only the modes the enclosed air can
reach, and is then truncated to its own budget, `ResonantModeLimit`. The tiers
doubled once the second full bank stopped being computed and discarded.

| Tier     | Batter | Resonant, lumped cavity | Resonant, shipped six-state cavity | Top batter mode |
| -------- | ------ | ----------------------- | ---------------------------------- | --------------- |
| Draft    | 48     | 4                       | 20                                 | 929 Hz          |
| Standard | 96     | 6                       | 24                                 | 1310 Hz         |
| High     | 160    | 8                       | 24                                 | 1662 Hz         |

The top-mode column is at the retuned 1250 N/m default; before it those were 646,
915 and 1166 Hz. Raising the tuning raises the whole mode series, so the same
budget reaches 1.44 times higher — which is where the room to move the attack
layer above the modal band came from.

The resonant column grew when P9/M2 gave the transverse cavity modes a coupling
path to the \(m = 1\) and \(m = 2\) families — the reachable set went from
\(\{0\}\) to \(\{0,1,2\}\) and the same 96-slot selection went from leaving 6
modes to leaving 28. The reachability reduction is still exact; what is new is
that the second budget then truncates it, which is not.

`ResonantModeLimit` exists because one number was sizing two banks that are
excited by different things. The batter head is struck and heard, so its budget
buys bandwidth and belongs on a quality tier. The resonant head is only ever
driven through the air, so the span worth covering is the cavity's, which is a
property of the shell. Sharing the tier's number was accidental rather than
designed, and it showed once the cavity setting started deciding the resonant
head's size.

The default of 24 is the smallest budget that straddles both transverse cavity
resonances: the cavity's \((1,1)\) pair at 660 Hz sits between the resonant
\((1,2)\) at 472 Hz and \((1,3)\) at 685 Hz, and its \((2,1)\) pair at 1094 Hz
between \((2,4)\) at 1001 Hz and \((2,5)\) at 1213 Hz, the 23rd and 24th
oscillator of the reachable bank. Below 12 the \((1,3)\) straddle is missing and
the P9/M2 mechanism itself is 3 dB wrong; the value is measured against the
mechanism in `DefaultResonantModeLimit`'s note.

Truncating from 28 to 24 is a real change to the instrument, unlike the
reachability filter, and a small one: broadband RMS and peak at the shipped pickup
both move by less than 0.001 dB, the small-signal transfer function moves by
0.019 dB RMS over 20 Hz–4 kHz with a worst point of 1.08 dB at 1376 Hz, and the
13.1 dB modal-versus-lumped feature at 657.5 Hz that P9/M2 exists for reads
13.16 dB instead of 13.13 dB.

A lumped cavity leaves 4, 6 and 8 axisymmetric modes at the three tiers, all far
below 24, so the budget never binds on a migrated v1–v10 document and those still
render bit-identically — verified sample for sample against a `git worktree` at
the pre-P9/M2 commit, at all three tiers.

Measured on `js/wasm` under Node with 512-sample chunks, retriggering at full
velocity before every chunk so the nonlinear solve never idles:

| Configuration           | Oscillators | x real time |
| ----------------------- | ----------- | ----------- |
| before P8 (48 + 48)     | 96          | 1.48        |
| Standard, worst case    | 102         | **1.66**    |
| Standard, decaying tail | 102         | 3.90        |
| High, worst case        | 168         | 1.43        |

Standard now covers twice the mode count of the old default, plus a high band the
old default did not have at all, for slightly _less_ cost than before. Render
still allocates nothing.

Re-measured after the attack layer became three bands, since that tripled its
per-sample filtering: 1.70, 1.81 and 2.19 times real time over three runs at 102
oscillators, still with zero allocations. The 1.66 above is inside that spread and
is kept as the conservative figure. Two extra biquads and two extra one-pole
envelopes are a rounding error beside a hundred oscillators and a nonlinear solve,
which is what the measurement says as well as what the arithmetic predicts.

Those four rows predate the modal cavity and are all `ModeCount = 1`. On a second,
busier machine, so absolute figures are not comparable with the table above but
before/after pairs measured back to back on it are, with medians of five
interleaved runs of `BenchmarkNonlinearDoubleHeadActive48k`:

| Configuration              | Oscillators | host  | `js/wasm` |
| -------------------------- | ----------- | ----- | --------- |
| Standard, lumped cavity    | 102         | —     | 2.56      |
| Standard, before the split | 124         | 4.638 | 1.443     |
| Standard, with the split   | 120         | 4.750 | 1.496     |
| High, before the split     | 198         | —     | 1.035     |
| High, with the split       | 184         | —     | 1.146     |

So the split is worth 3.7 % at Standard and 10.7 % at High on `js/wasm`, which is
a real improvement and nowhere near a fix: the shipped six-state cavity still
costs 42 % of the lumped model's headroom, and scaling the 1.66 figure above by
the measured ratio puts the Standard worst case near 0.97× real time against 0.94×
before. The budget split was made for the structural reason, not this one, and the
remaining cost is the transverse cavity's own — it wants attacking as an
optimization rather than by shrinking the instrument further. Render still
allocates nothing.

> **Superseded by measurement, 2026-07-31.** The 0.97× above is a projection, and
> the nonlinear mode coupling landed before anyone collected on it. Measured
> directly at 120 oscillators with the coupling enabled at its shipped 256
> coefficients, the Standard retrigger worst case is **0.70× real time** on
> `js/wasm` (2.06× on host), against 1.40× with the coupling off. The
> fixed-point iteration count barely moved — 2.404 to 2.491 — so the cost is the
> coupling's table walk, not the physics, and it is the subject of PLAN N9.
> The two-voice figure below is stale by the same factor. Standard is currently
> below real time on `js/wasm`; Draft is the shipped answer until N9 lands.

Do not use the decaying-tail row for this comparison. That benchmark is bimodal:
once the modes decay into subnormal range the arithmetic falls off the fast path
and the figure swings by more than the effect being measured. The retrigger worst
case is the number to trust.

The honest caveat: both Tom tracks can select the physical model, and two
simultaneous physical voices at Standard sit at about 0.8 times real time in the
worst case. That was already true before this work — 0.74 — so this is an
improvement rather than a fix, and Draft exists for it.

## The attack layer

`Attack` is three bands of noise, driven by the contact force and each decaying at
its own rate.

### Why three, and not one

It shipped as one band with one fitted 20 ms release, and it sounded like a hiss
laid over the drum rather than like the drum's own attack. Two things were wrong
with it, and both are measurable.

**The release was far too long.** A one-pole 20 ms release is a 138 ms \(T_{60}\).
The head's own loss law, extrapolated into the band the layer stands for, gives:

| Frequency | \(\gamma\) | \(\tau\) | \(T_{60}\) |
| --------- | ---------- | -------- | ---------- |
| 1 kHz     | 46.3 /s    | 21.6 ms  | 149 ms     |
| 2 kHz     | 92.1 /s    | 10.9 ms  | 75 ms      |
| 5 kHz     | 232.3 /s   | 4.3 ms   | 30 ms      |
| 8 kHz     | 376.2 /s   | 2.7 ms   | 18 ms      |

So the layer rang about twice too long at the bottom of its range and seven times
too long at the top. Broadband noise held that far past the strike does not fuse
into the attack; it is heard as a separate source, which is exactly the complaint.

**And it was one rate for the whole span.** Constant \(Q\) means the absolute
decay rate rises with frequency, so the top of this band genuinely dies several
times faster than its bottom. A single release cannot express that, and the
version that had one made 8 kHz sustain as long as 1 kHz — something no membrane
does.

### How it works now

- **Excitation** is the contact force sample the modes see, so mallet hardness
  and velocity carry into the layer for free: a harder stick has a shorter
  contact and so a tighter, brighter burst. There is no second trigger and no
  second set of dynamics to keep consistent with the first.
- **Bands** sit at 0.4, 1 and 2.5 times `Attack.CentreHz`, each a bandpass with
  `Attack.QualityFactor` defaulting to 0.7 — about an octave wide, so the three
  tile the group without a gap. Broad on purpose: this stands in for a dense
  thicket of unresolved modes, so a resonant peak would be a worse lie than a
  gentle hump.
- **Envelopes** are one per band, a one-pole release charged by the contact force,
  so each burst outlasts the 5.5–8 ms contact the way a struck head's high band
  does. Their rates are **derived, not fitted**: each is the batter head's own
  structural loss law evaluated at that band's centre wavenumber, so the layer is
  an extrapolation of the mode series rather than an effect bolted beside it. At
  the default that gives 94 ms \(T_{60}\) at 1.6 kHz, 37 ms at 4 kHz and 15 ms at
  10 kHz.

  `Attack.DecayScale` is a dimensionless multiplier on all three, defaulting to 1.
  It exists only because the loss law is being read past the range it was fitted
  in.

  `DAMP`, the strip `DEC` and `D.TILT` now all reach the layer for free, because
  they are applied to the head before the rates are read off it. `D.TILT` in
  particular _does_ apply now: three bands at different rates are a shape to tilt,
  where the single band it replaced was not.

- **Placement** starts above the modal band. `Attack.CentreHz` defaults to 4 kHz,
  putting the lowest band at 1.6 kHz against a top retained mode of 1310 Hz. At
  the 3 kHz it shipped with, that band landed at 1.2 kHz — _inside_ the modal
  band, which is both a double count and the wrong texture, because that region is
  heard as pitch rather than as hiss. `TestAttackLayerStartsAboveTheModalBand`
  keeps the two halves of the voice from describing the same frequencies.

- **Level** is fitted, like the near-field scale, and measured in the 43 ms attack
  window against the strongest low partial:

  | `LevelRelative` | 1–2 kHz | 2–5 kHz | 5–10 kHz | voice peak |
  | --------------- | ------- | ------- | -------- | ---------- |
  | 0               | −45.5   | −77.7   | −94.5    | 0.435      |
  | 0.02            | −37.9   | −39.1   | −40.6    | 0.503      |
  | **0.05**        | −31.6   | −31.1   | −32.7    | 0.616      |
  | 0.1             | −26.1   | −25.0   | −26.6    | 0.803      |

  The first row is the defect this layer exists to fix: with modal synthesis alone
  there is nothing above 2 kHz within 75 dB of the fundamental. Three summed
  envelopes are about three times as loud as one for the same number, so the
  refitted default is 0.05 where the single-band version used 0.1.

### Determinism

The noise source is xorshift64\*, one word of state, seeded from a constant and
rewound by `Reset`. This is not a detail: much of the suite compares renders
exactly, and a global random source would break those comparisons in a way that
only appears sometimes. `TestAttackLayerIsDeterministic` pins it, including that
`Reset` rewinds the sequence, so a second hit on one model matches a first hit on
a new one.

The consequence is that repeated hits within a render are identical to each
other. That is deliberate. Per-trigger variation is a separate mechanism with a
separate justification (PLAN.md N10).

### Termination

The layer's envelope enters `IsActive`. It has to: the voice stops calling `Tick`
the moment `IsActive` reports false, and a layer whose release outlasts the modal
energy threshold would otherwise be cut off mid-burst.

## Controls

Two additions to the physical bank, appended at indices 16 and 17:

| Control | Field                  | Range        | Default |
| ------- | ---------------------- | ------------ | ------- |
| ATK.L   | `Attack.LevelRelative` | 0–0.15       | 0.05    |
| ATK.T   | `Attack.CentreHz`      | 500 Hz–8 kHz | 4 kHz   |

App-state format version 12 widened both physical banks for them. Version 13
changes no bytes at all: `ATK.L`'s range narrowed from 0–0.3 to 0–0.15 with the
three-band refit, so a stored position has to double to keep meaning the same
level, and a position still sitting on v12's detent moves to the new default
instead — the same two-rule shape `migrateStrikeRadius` uses, and Tom 2's bank
gets it too. Versions 10 and 11 keep their own 16-slot width, so links written by
either still decode.

## Validation

The P8 suite covers:

- bit-exact equality between the full and axisymmetric-only resonant head, and
  separately that no m > 0 resonant mode ever leaves zero while the axisymmetric
  ones are excited — so a failure says which half broke;
- audible content in 1–2 kHz and 2–5 kHz with the layer on, and its absence
  below 60 dB with the layer off, so the layer cannot be quietly removed;
- each band's release matching the loss law at its own centre, and each band
  decaying strictly faster than the one below it, so the layer cannot go back to
  ringing at one flat rate;
- the lowest band sitting above the top retained mode;
- bit-identical renders from two fresh models and across `Reset`;
- the layer scaling with velocity, and keeping the voice active while it rings;
- exact silence when disabled or at zero level, with the radiated sum falling
  back to the batter term alone;
- product level within [0.70, 0.95] at velocity 1 with no compensating gain in
  the voice — the assertion that keeps `physicalTomOutputGain` deleted;
- zero render allocations, unchanged.

## What is deliberately still missing

The three bands have fixed centres and fixed relative levels. A real high band
has its own spectral tilt that changes with strike position and evolves during
the decay, and none of that is modelled. The layer is a reduced model of
unresolved modes, not a claim about them, and what it needs next is measured
spectra to fit against rather than more parameters — which the licensed
reference set now supplies.

## The high band is not where the model's deficiency lives

It is tempting to read the sections above as saying the voice's remaining error
is above the top retained mode, in the region only this layer covers. **It is
not**, and the point has been measured rather than argued: band-limiting the
spectral-envelope error to 50 Hz–2 kHz, where the model has full modal content,
moves it only from **11.07 to 9.11 dB**. The band-coverage hypothesis is real and
worth about 2 dB of 11; the rest is inside the modal band.

So widening the attack layer, or moving it, is not the lever. The actionable
defect is a damping-distribution problem in the modal band — see
[`physical-objective-validation.md`](physical-objective-validation.md) for the
residual budget and the falsification tests behind that conclusion, and
[P10/N3](../PLAN.md) for the work it implies.
