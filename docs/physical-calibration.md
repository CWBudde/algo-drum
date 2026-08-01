# Physical drum P2 calibration

This document records the implemented loss/radiation model, the offline
analysis workflow, and the provenance and limits of the repository's first
physical-drum reference set.

## Modal loss

For mode \(i\), with membrane wavenumber \(k_i\), the amplitude decay rate is

\[
\gamma_i =
d_0 + d_1 k_i + d_2 k_i^2 +
d_\mathrm{rad} R_i^2 +
\Delta_i.
\]

- \(d_0\), \(d_1\) and \(d_2\) are the three structural-loss parameters stored
  on `Head`.
- \(R_i\) is the mode's radiation amplitude described below.
- \(d_\mathrm{rad}\) keeps radiation loss distinct from structural loss.
- \(\Delta_i\) is an optional correction keyed by azimuthal and radial mode
  order. It is zero unless a measured fit supplies a residual.

`Mode` exposes all five terms separately as well as their sum. A negative total
is rejected. The exact state transition uses \(\gamma_i\) directly, and the
analytic amplitude-decay target is

\[
T_{60,i} = \frac{\ln(1000)}{\gamma_i}.
\]

### Why the \(k^1\) term exists

\(d_1\) carries constant \(Q\), and nothing else in the law can. On a membrane
\(\omega \approx ck\), so the fraction of critical damping is

\[
\zeta_i = \frac{\gamma_i}{\omega_i}
\approx \frac{d_0}{ck_i} + \frac{d_1}{c} + \frac{d_2 k_i}{c},
\]

whose only frequency-independent contribution is the middle term: choosing
\(d_1 = \zeta c\) fixes \(\zeta\) across the whole mode series, while \(d_0\)
alone gives \(T_{60}\) independent of frequency and \(d_2\) alone gives
\(T_{60} \propto 1/f^2\). Measured membrane behaviour is \(T_{60} \propto 1/f\).

The reference set uses \(\zeta = 0.72\,\%\), so \(d_1 = 0.4303\) m/s on the
batter (\(c = 59.76\) m/s) and \(0.4644\) m/s on the resonant head
(\(c = 64.50\) m/s), with \(d_0\) reduced to a small floor. The retained band
then holds \(\zeta\) between 0.73 % and 0.80 %.

0.72 %, not the 1.1 % this was first calibrated at, and the reason is in
[the tuning section](#tuning-and-constant-q) below: \(d_1\) used to be an
absolute constant, so \(\zeta\) drifted as the tuning knob moved, and 0.72 % is
what it happened to reach at the tuning that was reported as sounding right.

The batter's \((0,1)\) carries a \(\Delta = 24.6\) /s correction and the
resonant head's \(26.4\) /s, putting both near \(\zeta = 3.4\,\%\) at the
default tuning — a \(T_{60}\) of 213 ms, which is the number the correction is
actually anchored to. This is the
one mode whose loss is not a membrane property: the axisymmetric fundamental is
the shape that compresses the cavity and drives the opposite head, so it sheds
energy into the coupled system far faster than its neighbours. Measured
two-headed drums show it as the shortest partial in the low band, not the
longest.

The default 12-inch head's lowest mode is 150.10 Hz with a 0.21 s analytic
amplitude \(T_{60}\); its highest retained mode, at 1310 Hz once the reclaimed
resonant-head budget went to the batter head, decays in about 0.11 s. These are
model targets, not claims about a commercial drum.

### What a smooth loss law cannot reach

The law is smooth in \(k\) — \(d_0 + d_1k + d_2k^2\), plus a radiation term that
is itself a smooth function of the mode's own \(ka\). That is a structural limit,
and it has now been measured rather than assumed. Fitting the best smooth power
law to a real tom's own measured \(T_{60}\)s leaves **0.677** in log-decay error,
while the fitted model already reaches **0.573**. So no choice of \(d_0\),
\(d_1\), \(d_2\) and \(d_\mathrm{rad}\) can reach the 0.25 decay gate: the model
is already past the ceiling a smooth \(\gamma(k)\) imposes, and adding terms to
the polynomial is not the way forward.

The missing freedom is **structured, not free-per-mode**. A two-headed drum
splits each head-pair mode into an in-phase and an out-of-phase member: the
squeezing member pumps the cavity and is heavily damped, the sliding member is
not. The model has that mechanism, but only for \(m = 0\), and only as the
\(\Delta\) correction above. Extending it is
[P10/N3](../PLAN.md), which also states the check that keeps the extension
honest — a fitted per-mode damping vector is physics if its sign pattern
reproduces the predicted pairwise alternation, and fitted noise if it is
structureless.

The 0.25 gate is itself not a reachable target, for a reason that has nothing to
do with the loss law: the objective disagrees with itself by more than that when
the same hit is scored through two coincident microphones. See
[`physical-objective-validation.md`](physical-objective-validation.md).

## Tuning and constant Q

`RetuneTension` sets a head's tension and rescales \(d_1\), \(d_2\) and the
mode decay corrections by \(\sqrt{T_\mathrm{new}/T_\mathrm{old}}\), because
each of them is proportional to the wave speed:

- \(d_1\) *is* \(\zeta c\), by the argument above;
- \(d_2\) is the same statement one order out in \(k\);
- the \((0,1)\) correction stands for a coupling loss, which is a fraction of
  that mode's own \(\omega\).

\(d_0\) is a frequency-independent floor and does not move.
`RadiationLossPerSecond` also stays put, because its frequency dependence lives
in the radiation amplitude it multiplies rather than in the coefficient.

Without this the tuning control is secretly a sustain control. Measured on the
coefficients the model shipped with, tuning the batter head across `B.TUNE`'s
range gave:

| \(T\) (N/m) | \(c\) (m/s) | \(\zeta\) | \((0,1)\) | \(T_{60}\) of a 300 Hz partial |
| ------------- | ------------- | ------------ | ----------- | -------------------------------- |
| 150           | 20.70         | 2.20 %       | 52.0 Hz     | 0.166 s                          |
| 600 (shipped) | 41.40         | 1.10 %       | 104.0 Hz    | 0.313 s                          |
| 1400          | 63.25         | 0.72 %       | 158.9 Hz    | 0.423 s                          |

Turning the drum up therefore made it ring half again as long as well as higher,
and the report that it "only sounds good at high `B.TUNE`" was in part a report
about that. `TestRetuningHoldsConstantQ` now pins \(\zeta\) across the whole
range, and `TestRetuningMovesPitchAndNotMuchElse` states the same property the
way a player would: four times the tension is twice the pitch and half the
\(T_{60}\), so the number of cycles the drum rings for does not change.

The default tension moved with it, from 600 N/m to 1250. At 600 the 12-inch
batter head's fundamental is 104 Hz, which is a floor tom rather than a rack
tom; the drum only began to read correctly near the old ceiling of 1400. The
range is now 300–3500 N/m, or 75–251 Hz, so the usable pitch sits mid-travel
instead of against the stop.

That value is corroborated twice from outside this repository, which matters
because tension is otherwise the parameter with the least external anchoring.
Fletcher & Rossing Fig. 18.11 gives **351 N/m** for a 33 cm tom head "at the low
end of the normal playing range", and an independent derivation for a 13-inch
10-mil head gives **420 N/m**. Both are consistent with a shipped 1250 N/m at a
tuning well above the low end — a factor of three in tension is a fifth in pitch.
Neither is a measurement of this drum, so they bound the default rather than set
it.

## Configuration schema

The physical configuration schema is version 9. Version-1 configurations are migrated by
filling the P2 radiation and microphone defaults; version-2 linear double-head
configurations migrate with P4 nonlinearity disabled so their sound is
unchanged; version 3 migrates with full cavity coupling, preserving its
previous equations; version 4 migrates with zero tension asymmetry, preserving
its ideal circular-head modes exactly; version 5 migrates with \(d_1 = 0\) and
its decay corrections untouched, which reproduces its flat damping — including
its 2.21 s fundamental — exactly; version 6 migrates with the cavity stiffness
scale set to 1, the rigid-enclosure value it derived. That last one is the only
migration in the chain whose compatibility value is not the zero value, so it has
to be written explicitly: an absent `stiffnessScale` decodes to 0, which is the
uncoupled limit rather than the old sound. See
[`physical-cavity.md`](physical-cavity.md) for why new configurations do not use
the rigid value. Version 7 migrates onto the corrected radiated sum with the
near-field mixture at zero, which is the exact absence of that term rather than
a broken value, and onto the recalibrated output gain. Version 8 migrates onto
the multi-band attack layer; its `decaySeconds` is dropped rather than mapped,
because an absolute release has no image in a set of rates read off the loss law.

Version 8 documents keep their own tuning. Tension and the loss coefficients
quoted against it are the document's measured content, so migrating them would
retune a saved drum; only new configurations start from the retuned default.
`TestDecodeConfigMigratesVersionEightKeepsItsTuning` pins that.

## Radiation and microphone response

The observable is **volume acceleration**, because far-field pressure from a
compact source is \(p = \rho\ddot V/4\pi r\) with no further frequency
dependence. The head-point diagnostics remain the modal sum weighted by the
circular mode shape at the pickup projection; the microphone signal does not use
that mode shape at all — a near-field point shape and a far-field efficiency are
different objects, and multiplying them nulls modes arbitrarily as the microphone
moves.

### The far-field term

The Rayleigh integral of one circular mode against the observation direction is a
Lommel integral, and because \(J_m(z_{mn}) = 0\) by construction it collapses to
a closed form with nothing left over:

\[
G_{mn} =
2\pi R^2 \, z \, J_{m+1}(z) \, \frac{J_m(u)}{z^2 - u^2},
\qquad
u = \frac{\omega_{mn} R}{c}\sin\theta .
\]

Two properties carry the physics:

- at \(u = 0\) it is *exactly* the swept area \(2\pi R^2 J_1(z)/z\) for m = 0 and
  exactly zero for m > 0, so an on-axis microphone hears the axisymmetric modes
  and nothing else, which is the correct and measurable result;
- \(J_m(u) \sim (u/2)^m/m!\) for small \(u\), so multipole cancellation comes out
  of the integral instead of being approximated by a rolloff exponent.

The observation angle comes from the microphone geometry already in the
configuration, \(\sin\theta = r_\mathrm{mic}/\sqrt{r_\mathrm{mic}^2 + d^2}\).
Because a membrane carries waves far slower than air does, \(u/z\) is bounded by
\(c_\mathrm{membrane}/c_\mathrm{air}\) — about 0.12 here — so the denominator
cannot vanish for any real head. The coincidence limit \(\pi R^2 J_{m+1}(z)^2\)
is implemented anyway rather than dividing by something unchecked.

The compact approximation is good to \(ka\) of about 3, which covers the retained
band.

### The near-field term, and why it is needed

The far-field term alone is not what a tom microphone hears. Measured on the
default configuration, multipole cancellation leaves every m > 0 mode at least
23 dB down even with the microphone against the head — correct for a distant
microphone, because a 12-inch head below 600 Hz really is nearly a monopole, and
wrong for the close one a tom is actually recorded with, at \(d/a\) of about a
third.

What a close microphone picks up there is the part of the field that never
radiates. For a structural wave slower than sound that part is evanescent,
decaying as \(e^{-k d}\) with the mode's own structural wavenumber, so higher
modes fade out of it faster than low ones; its shape at the microphone is the
mode shape. So the weight is a **sum of two terms**, not a product:

\[
W_i = G_{mn}\,D_m(\phi)\,\frac{1}{1+d/a}
\;+\;
s_\mathrm{nf}\,2\pi R^2 e^{-z_{mn} d/R}\,\Phi_i .
\]

\(D_m(\phi) = \cos m(\phi-\phi_0)\) or \(\sin\) is the far-field azimuthal
pattern, which depends on the microphone's angle and never on its radius — the
polar dependence lives in \(u\). \(\Phi_i\) is `PickupShape`, which is the right
object here and only here. `Pickup.NearFieldScale` is \(s_\mathrm{nf}\); it is
fitted, because the effective area of an evanescent patch is not something this
reduced model can compute. The exponential carries this term's whole distance
law, so the geometric spreading factor deliberately does not appear in it.

Both terms multiply the same per-mode acceleration, so the weight stays one
precomputed scalar and the per-sample cost is unchanged.

The fitted result at the shipped close-microphone geometry — 0.65 of the radius,
30 mm up — is a partial structure of (0,1) 0 dB, (1,1) −7.1 and −10.4, (0,2)
−8.5, (2,1) −9.3 and −17.5, falling to −34.5 dB at the top of the retained band.

### What the efficiency factor is still for

\[
R_i =
\left(\frac{ka}{\sqrt{1+(ka)^2}}\right)^{m_i+1}
\]

remains, but only to apportion **radiation damping** across the mode series. It
is deliberately not the output weight. As an amplitude ratio against modal
velocity it already contains one factor of \(ka\), so using it beside the
volume-acceleration observable would count that factor twice — about +10 dB of
unjustified tilt across the retained band, which a refitted output gain would
have concealed. And raising it to \(m+1\) stands in for a multipole cancellation
whose true magnitude is \(1/(2^m m!)\), which is off by seven orders at the
highest retained azimuthal order.


The raw radiation sum passes through Butterworth high-pass and low-pass
biquads from `algo-dsp`. Defaults are 35 Hz and 12 kHz. `Output` exposes the
point displacement/velocity, raw radiation sum, filtered microphone output,
and mechanical energy independently so tests can localize errors.

## Offline analysis

Run a single analysis:

```bash
go run ./cmd/analyze-physical -o report.json
```

Useful flags include `-duration`, `-velocity`, `-strike-radius`,
`-pickup-radius`, `-pickup-angle`, `-pickup-distance`, `-fft-size`,
`-pitch-frame`, and `-pitch-hop`.

The report contains:

- analytic modal frequencies, separated decay terms, \(T_{60}\), and radiation
  weights;
- time-domain peak, RMS, energy, and crest factor from `algo-dsp`;
- Schroeder/IR decay metrics from `algo-dsp`;
- a Hann-windowed real FFT from `algo-fft`, spectral descriptors from
  `algo-dsp`, and interpolated modal peaks;
- a frame-wise track constrained around the lowest analytic mode. This
  analyzer intentionally renders the linear P2 `SingleHead`, so its track
  should remain essentially stationary. P4 glide verification renders
  `DoubleHead` directly; see
  [`physical-nonlinearity.md`](physical-nonlinearity.md).

`analysis.CompareSignals` supplies gain-fitted normalized waveform RMSE,
waveform correlation, and log-spectrum RMSE in dB, for regression and for
comparison against the recorded reference set below.

## Reference-set provenance

[`testdata/physical-reference-v2.json`](../testdata/physical-reference-v2.json)
is a deterministic synthetic calibration set generated by:

```bash
just gen-physical-reference
```

It is repository-authored and distributed under the repository's MIT license.
The committed file stores derived metrics, not audio.

Recording/simulation conditions:

- 48 kHz, mono, 1.5 seconds per hit;
- default 12-inch batter-head parameters;
- three normalized velocities: 0.35, 0.80, and 1.00;
- velocity-dependent stick contact from the measured 8 ms quiet endpoint to
  the 5.5 ms loud endpoint, scaled by configured hardness;
- three strike radii: center (0), offset (0.45), and near edge (0.70);
- three microphone projections/distances: center/0.10 m,
  radius 0.32/0.30 m, and radius 0.65/1.00 m;
- no room, background noise, normalization, cavity coupling, resonant head, or
  nonlinear tension modulation;
- zero P6 tension asymmetry: this version-2 fixture retains ideal circular P2
  modes, while P6 splitting has separate analytic regression tests.

Version 2 replaces the earlier synthetic fixture because version 1 encoded the
incorrect 0.25–8 ms hardness-only contact law. At the shipped hardness its
normal hit was just 0.71 ms, far outside the measured tom-stick range.

This set catches deterministic changes and anchors analytic targets. It is not
an acoustic validation recording and must not be described as one.

`just check-physical-reference` regenerates and diffs the fixture; it is part of
`just ci`.

## The recorded reference set

The synthetic fixture above is a regression anchor. Acoustic validation uses
`reference/tt08x08-mp-hd-v01`–`v16.wav`, and that set is the first one on this
path whose provenance is known:

- **Licence** CC BY 4.0, with the attribution
  [`reference/CREDITS.md`](../reference/CREDITS.md) states verbatim. It must be
  carried by any distribution of audio derived from it.
- **Instrument** an 8" × 8" tom — diameter and depth are therefore constants
  rather than fitted parameters — with a Remo coated Ambassador batter and a
  Remo clear Diplomat resonant head, so both surface densities follow from the
  manufacturer's gauges instead of being fitted.
- **Capture** 48 kHz, a **coincident** XY pair, sixteen velocities of the same
  head strike at one tuning.

Everything the two properties above buy is why P10 is possible at all: the known
geometry lets the cavity split be computed rather than fitted
([`physical-cavity.md`](physical-cavity.md)), and the coincident pair lets the
fitting objective be scored against itself. What that measurement found — the
objective's self-disagreement, the residual budget, and four refuted hypotheses —
is in [`physical-objective-validation.md`](physical-objective-validation.md).

`reference/tom.wav`, which earlier work on this path was fitted against, is
44.1 kHz, a spaced pair, of unknown provenance and unlicensed. It is being
retired ([P10/N8](../PLAN.md)) and no number derived from it survives in this
document.

## Dependency compatibility

Direct analysis uses `algo-fft` v0.7.3, its latest tag at implementation time.
The latest `algo-dsp` tag remains v0.5.1 and calls the removed `NewPlanT` API,
so P2 temporarily pins the published compatibility commit
`8ea972cf5f07` (`v0.0.0-20260729115219-8ea972cf5f07`). Replace the
pseudo-version with the next compatible `algo-dsp` release tag when one is
available.
