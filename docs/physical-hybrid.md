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
runs the same selection and then keeps its handful of axisymmetric modes. The
tiers doubled once the second full bank stopped being computed and discarded.

| Tier     | Batter | Resonant | Top mode |
| -------- | ------ | -------- | -------- |
| Draft    | 48     | 4        | 646 Hz   |
| Standard | 96     | 6        | 915 Hz   |
| High     | 160    | 8        | 1166 Hz  |

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

The honest caveat: both Tom tracks can select the physical model, and two
simultaneous physical voices at Standard sit at about 0.8 times real time in the
worst case. That was already true before this work — 0.74 — so this is an
improvement rather than a fix, and Draft exists for it.

## The attack layer

`Attack` is a single band of noise, driven by the contact force and decaying on
its own.

- **Excitation** is the contact force sample the modes see, so mallet hardness
  and velocity carry into the layer for free: a harder stick has a shorter
  contact and so a tighter, brighter burst. There is no second trigger and no
  second set of dynamics to keep consistent with the first.
- **Envelope** is a one-pole release charged by that force, so the burst outlasts
  the 5.5–8 ms contact the way a struck head's high band does.
  `Attack.DecaySeconds` defaults to 20 ms and follows `DAMP` and the strip `DEC`,
  for the same reason the (0,1) decay correction does: a damping control that
  cannot shorten the stick has no authority over the part of the sound listeners
  notice first. `D.TILT` deliberately does not apply — one band has no shape to
  tilt.
- **Band** is one bandpass at `Attack.CentreHz` with `Attack.QualityFactor`
  defaulting to 0.7, which spans roughly 1–8 kHz. Broad on purpose: this stands
  in for a dense thicket of unresolved modes, so a resonant peak would be a worse
  lie than a gentle hump.
- **Level** is fitted, like the near-field scale, and measured in the 43 ms
  attack window against the strongest low partial:

  | `LevelRelative` | 1–2 kHz | 2–5 kHz | 5–10 kHz |
  | --------------- | ------- | ------- | -------- |
  | 0               | −66.9   | −83.9   | −99.8    |
  | 0.05            | −38.3   | −33.0   | −33.9    |
  | **0.1**         | −32.3   | −27.0   | −27.9    |
  | 0.2             | −26.3   | −20.9   | −21.9    |

  The first row is the defect this layer exists to fix: with modal synthesis
  alone there is nothing above 1 kHz within 60 dB of the fundamental. 0.1 is a
  stick that is clearly present and still well under the low band; 0.2 reads as a
  click.

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

| Control | Field                  | Range        |
| ------- | ---------------------- | ------------ |
| ATK.L   | `Attack.LevelRelative` | 0–0.3        |
| ATK.T   | `Attack.CentreHz`      | 500 Hz–8 kHz |

App-state format version 12 widens both physical banks for them. Version 11 —
which changed no bytes, only the meaning of the stored strike radius — and
version 10 keep their own 16-slot width, so links written by either still decode.

## Validation

The P8 suite covers:

- bit-exact equality between the full and axisymmetric-only resonant head, and
  separately that no m > 0 resonant mode ever leaves zero while the axisymmetric
  ones are excited — so a failure says which half broke;
- audible content in 1–2 kHz and 2–5 kHz with the layer on, and its absence
  below 60 dB with the layer off, so the layer cannot be quietly removed;
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
