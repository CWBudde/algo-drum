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

`Quality.ModeLimit()` is effectively the batter head's budget: the resonant head
runs the same selection and then keeps only the modes the enclosed air can reach.
The tiers doubled once the second full bank stopped being computed and discarded.

| Tier     | Batter | Resonant, lumped cavity | Resonant, shipped six-state cavity | Top mode |
| -------- | ------ | ----------------------- | ---------------------------------- | -------- |
| Draft    | 48     | 4                       | 20                                 | 929 Hz   |
| Standard | 96     | 6                       | 28                                 | 1310 Hz  |
| High     | 160    | 8                       | 38                                 | 1662 Hz  |

The resonant column grew when P9/M2 gave the transverse cavity modes a coupling
path to the \(m = 1\) and \(m = 2\) families; the reduction is still exact, and
its cost is measured in
[`physical-cavity.md`](physical-cavity.md#what-the-transverse-modes-cost).

Those are the frequencies at the retuned 1250 N/m default; before it they were
646, 915 and 1166 Hz. Raising the tuning raises the whole mode series, so the same
budget reaches 1.44 times higher — which is where the room to move the attack
layer above the modal band came from.

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
separate justification (PLAN.md S7).

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

The layer is one band with one decay. A real high band has its own spectral tilt
that changes with strike position and evolves during the decay, and none of that
is modelled. It is a reduced model of unresolved modes, not a claim about them,
and the next thing it needs is measured spectra to fit against rather than more
parameters.
