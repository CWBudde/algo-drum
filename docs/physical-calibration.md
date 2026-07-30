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

The reference set uses \(\zeta = 1.1\,\%\), so \(d_1 = 0.4554\) m/s on the
batter (\(c = 41.40\) m/s) and \(0.4919\) m/s on the resonant head
(\(c = 44.72\) m/s), with \(d_0\) reduced to a small floor. The retained band
then holds \(\zeta\) between 1.12 % and 1.24 %.

The batter's \((0,1)\) carries a \(\Delta = 24.6\) /s correction and the
resonant head's \(26.4\) /s, putting both near \(\zeta = 5\,\%\). This is the
one mode whose loss is not a membrane property: the axisymmetric fundamental is
the shape that compresses the cavity and drives the opposite head, so it sheds
energy into the coupled system far faster than its neighbours. Measured
two-headed drums show it as the shortest partial in the low band, not the
longest.

The default 12-inch head's lowest mode is 104.00 Hz with a 0.21 s analytic
amplitude \(T_{60}\); its highest retained mode, at 915 Hz once the reclaimed
resonant-head budget went to the batter head, decays in about 0.13 s. These are
model targets, not claims about a commercial drum. The physical
configuration schema is version 8. Version-1 configurations are migrated by
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
the rigid value.

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
waveform correlation, and log-spectrum RMSE in dB for regression or for a
future locally recorded input set.

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
an acoustic validation recording and must not be described as one. A measured
set can be added later without changing the report or comparison formats,
provided its drum dimensions, head/tuning state, mallet, microphone, geometry,
gain chain, sample rate, room, license, and raw-file checksums are recorded.

`just check-physical-reference` regenerates and diffs the fixture; it is part of
`just ci`.

## Dependency compatibility

Direct analysis uses `algo-fft` v0.7.3, its latest tag at implementation time.
The latest `algo-dsp` tag remains v0.5.1 and calls the removed `NewPlanT` API,
so P2 temporarily pins the published compatibility commit
`8ea972cf5f07` (`v0.0.0-20260729115219-8ea972cf5f07`). Replace the
pseudo-version with the next compatible `algo-dsp` release tag when one is
available.
