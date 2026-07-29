# Physical drum P6: real-instrument departures

P6 adds one controlled departure from an ideal circular drum and records why
the other candidates remain outside the audio loop. The shipped addition is
deterministic degenerate-mode splitting. Shell, hardware, bearing-edge, vent,
and measured-transfer layers still require measurements from an identified
instrument before they can alter the model.

## Non-uniform tension

An ideal circular head has equal-frequency cosine and sine members for every
azimuthal order \(m>0\). Real rim tension breaks that symmetry. Experiments and
perturbation/FEM studies report slightly separated, rotated mode pairs under
non-uniform tension:

- R. Worland, [“Normal modes of a musical drumhead under non-uniform
  tension”](https://doi.org/10.1121/1.3268605), JASA 127(1), 2010.
- R. Hashimoto, K. Yatabe, and Y. Oikawa, [“Drumhead tuning based on
  vibration mode visualization using Fourier transform
  profilometry”](https://doi.org/10.1250/ast.e23.40), Acoustical Science and
  Technology 45(2), 2024.

`Head.TensionAsymmetry` is a deliberately small phenomenological layer over
the SI membrane/plate model. If \(f_{mn}\) is the ideal frequency and \(s\) is
`SplitRatio`, the pair becomes

\[
f_{mn}^{c}=f_{mn}(1-s/2),\qquad
f_{mn}^{s}=f_{mn}(1+s/2).
\]

The pair midpoint therefore remains the frequency predicted from radius,
surface density, tension, and bending stiffness. Its shapes use
\(\theta-\theta_a\), where \(\theta_a\) is
`PrincipalAxisAngleRad`. Axisymmetric modes remain unchanged because they
have no degenerate partner. The split also feeds the existing radiation,
state-transition, nonlinear-tension, and reference-response calculations; it
is not an output-only chorus.

The validated split range is 0–2 %. Zero is an exact compatibility mode.
Physical configuration version 5 enables conservative new-config defaults of
0.4 % on the batter head and 0.3 % on the resonant head. Version-4 configs
migrate with both values at zero, so previously serialized models keep their
ideal frequencies and mode shapes.

The editor exposes:

- ASYM: 0–2 % batter-head pair separation; the resonant head receives 75 % of
  that amount;
- AXIS: −90–90° shared principal axis.

These defaults are musical reduced-model values, not a fit to a commercial
drum. Per-mode splitting depends on the symmetry and magnitude of the actual
tension perturbation, so a measured preset may replace the global reduction
with fitted per-mode residuals later.

## Shell, hardware, bearing edge, and vent evaluation

No measured shell mobility, head-to-shell transfer, or vent response is
present in the repository. Adding plausible resonators now would make them
impossible to distinguish from errors in head radiation, cavity pressure, or
microphone response. P6 therefore keeps all four candidates out of the
real-time state.

A candidate is accepted only if a documented measurement set shows a
repeatable residual after fitting the existing head/cavity model. The minimum
gate is:

1. identify the drum dimensions, heads, tuning state, support, strike,
   microphone geometry, sample rate, room, license, and raw-file checksums;
2. train on at least two hit velocities and two positions, then evaluate a
   held-out hit;
3. improve predeclared modal-frequency, modal \(T_{60}\), and log-spectrum
   tolerances without worsening the other metrics;
4. retain finite deterministic output, zero render allocations, bounded
   passive coupling, and the measured WASM modal budget.

A shell bank would need measured shell poles, damping, and signed coupling to
both heads. A bearing-edge correction belongs in per-mode frequency and decay
residuals unless measurements demonstrate a reusable compliance law. A vent
resonance needs pressure or microphone transfer data with the vent geometry
recorded. Hardware modes follow the same residual-and-held-out gate.

## Optional measured transfer calibration

A measured modal transfer function remains a valid optional observation layer,
but it is not a replacement for the physical state. The current decision is to
fit interpretable mode frequency, decay, radiation, and gain residuals first.
Add a transfer layer only when phase-coherent, time-aligned impulse responses
from more than one hit position show a stable residual that those corrections
cannot explain.

When such data exists, the layer must:

- live after the diagnostic head/cavity outputs so tests can still isolate the
  physical state;
- store source provenance and the calibration sample rate with its
  coefficients;
- be optional and bypass-identical;
- validate on held-out hits rather than only the response used for fitting;
- use a bounded, allocation-free real-time implementation.

This decision keeps P6 honest: mode splitting is justified by published
measurements, while instrument-specific coloration waits for
instrument-specific evidence.
