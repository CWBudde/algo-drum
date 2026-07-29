# Physical drum P4: nonlinear hit behaviour

P4 adds amplitude-dependent tension to both heads of `physical.DoubleHead`.
The model is still modal and allocation-free at audio rate, but loud hits now
start sharper and brighter, then glide down as their stored energy decays.
`physical.SingleHead` remains the linear P2 reference.

## Reduced Berger law

For one head, define the modal strain measure

\[
S = \int_A |\nabla w|^2\,dA
= \sum_i \Gamma_i q_i^2,
\qquad
\Gamma_i = \frac{m_i}{\mu}k_i^2.
\]

Here \(q_i\), \(m_i\), and \(k_i\) are modal displacement, mass, and
wavenumber, while \(\mu\) is head surface density. The Berger approximation
adds one spatially uniform tension shared by every mode. The unbounded
small-strain law is \(\Delta T=\beta S\), where \(\beta\) has units N/m³.

The real-time model uses the smooth bounded extension

\[
\Delta T(S)=T\_{\max}\tanh\left(\frac{\beta S}{T\_{\max}}\right),
\qquad
T\_{\max}=rT_0.
\]

It has the stored potential

\[
U\_\mathrm{B}(S)=
\frac{T\_{\max}^2}{2\beta}\,
\log\cosh\left(\frac{\beta S}{T\_{\max}}\right).
\]

Near rest, \(U\_\mathrm{B}=\beta S^2/4+O(S^4)\) and
\(\Delta T=\beta S+O(S^3)\), so this is the usual quartic Berger potential.
At large displacement it approaches the configured tension cap without a
hard clip or an energy discontinuity. Mode \(i\) receives the additional
stiffness

\[
\Delta\omega_i^2=\frac{\Delta T}{\mu}k_i^2.
\]

`Nonlinearity.BatterTensionCoefficientNPerM3` and its resonant-head
counterpart set \(\beta\). `MaximumTensionRatio` sets \(r\). The conservative
default uses \(3.0\times10^5\) N/m³ on the batter head,
\(2.0\times10^5\) N/m³ on the resonant head, and \(r=0.2\). These coefficients
are reduced-model calibration values, not claims about a particular film's
Young modulus.

## Discrete passivity

The update retains the P3 implicit-midpoint cavity solve and evaluates the
nonlinear force with the discrete tension

\[
\overline{\Delta T}
=

2\frac{U\_\mathrm{B}(S^{n+1})-U\_\mathrm{B}(S^n)}
{S^{n+1}-S^n}.
\]

Its limiting value is used when the two strain measures are nearly equal.
Because

\[
S^{n+1}-S^n
=2h\sum_i\Gamma_i\bar q_i\bar v_i,
\]

the work of the nonlinear modal force is exactly the change in
\(U\_\mathrm{B}\). After the finite strike pulse ends, the complete discrete
energy therefore satisfies, up to the nonlinear solve tolerance,

\[
E^{n+1}-E^n =
-h\sum_i 2d_i m_i\bar v_i^2
-h\lambda\frac{\bar p^2}{K}\leq0.
\]

The lossless update conserves linear head energy, nonlinear potential, and
cavity energy together. A fixed-point solve couples the two scalar head
tensions to the rank-one cavity pressure. It stops at a relative tension
tolerance of \(2\times10^{-12}\) and is bounded to eight iterations, so audio
work cannot become unbounded.

`DoubleHeadOutput` exposes each head's tension increase, linear head energy,
nonlinear potential, total energies, and the iteration count used by the last
sample. With nonlinearity disabled, the linear zero-cavity path still uses the
original exact state transition.

## Bounds and alias control

Both tension coefficients are validated in \([0,10^9]\) N/m³. The smooth cap
makes stability independent of displacement magnitude. It is also restricted
by each enabled head's modal frequency limit \(\alpha f_s\):

\[
r < \frac{1}{4\alpha^2}-1.
\]

The membrane contribution can therefore rise by at most \(1+r\), keeping
every retained mode below \(f_s/2\). This is conservative because bending
stiffness does not scale with the tension increase. A test checks the bound
against every generated batter and resonant mode, while a raw-radiation FFT
checks that the top one percent below Nyquist remains negligible.

## Verification

The deterministic P4 suite covers:

- velocity-dependent first-mode glide: with the isolated default batter
  fixture, velocity 0.2 measures about 105.3 to 104.0 Hz, while velocity 1.0
  measures about 113.5 to 104.0 Hz;
- attack-spectrum change: its normalized raw-radiation centroid rises from
  about 475 Hz to 517 Hz between those velocities;
- a 48 kHz trajectory against the same nonlinear system oversampled at
  192 kHz (0.08 percent maximum displacement error in the test fixture);
- lossless nonlinear energy conservation for both the isolated head and the
  coupled two-head/cavity system;
- monotonic dissipation with configured losses, finite output at the maximum
  allowed coefficient, modal Nyquist headroom, and zero render allocations.

The Standard 96-mode model was also benchmarked with a full-velocity retrigger
before every 512-sample chunk, keeping the nonlinear solve continuously
active. On the 2026-07-29 development machine it measured 103 ksamples/s
(2.14 times real time) on Linux/amd64 and 71 ksamples/s (1.48 times real time)
on `js/wasm` under Node, with zero allocations.

## Contact-model decision

P4 retains the normalized raised-sine force pulse. The repository's current
reference set is a deterministic linear synthesis fixture, not a measured
mallet/head force or recording, so it cannot establish that a power-law,
bounded-iteration contact model is more accurate. The reduced tension model
already creates measurable velocity-dependent attack and glide while the
existing pulse remains finite, normalized, deterministic, and inexpensive.

Adding a second strong nonlinearity without a contact-force or recorded-hit
target would add cost and parameters without evidence of benefit. A future
contact model should enter only with force/displacement or multi-velocity
recordings and should use an energy-based collision update. This follows the
evidence gate in the [physical-model research notes](physical-model-research.md)
and leaves the contact change open for a calibrated later phase.
