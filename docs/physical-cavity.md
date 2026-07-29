# Physical drum P3: resonant head and cavity

P3 extends the calibrated single-head model with a separately tuned resonant
head and a passive lumped cavity. The real-time implementation is
`physical.DoubleHead`; `physical.SingleHead` remains available as the P2
reference and is still bit-identical when both cavity coupling and P4
nonlinear tension are disabled.

## Reduced model

Each head has its own frequency-ordered circular modal bank. The batter and
resonant heads may use different tension, surface density, stiffness, and loss
parameters. Both use the same quality tier, so the Standard configuration
retains 48 oscillators per head.

For an axisymmetric mode with Bessel zero \(z\_{0n}\), the signed swept area is

\[
A\_{0n} = 2\pi R^2\frac{J_1(z\_{0n})}{z\_{0n}}.
\]

The angular integral of every \(m>0\) mode is zero, so those modes have exactly
zero cavity coupling. No empirical cross-coupling terms are present.

With modal displacement \(q_i\), modal mass \(m_i\), loss rate \(d_i\), cavity
pressure \(p\), and the product control \(g\in[0,1]\), define the effective
swept area \(\widetilde A_i=gA_i\). The coupled equations are

\[
\ddot q_i + 2d_i\dot q_i + \omega_i^2 q_i
= f_i/m_i - \widetilde A_i p/m_i,
\]

\[
\dot p = K\sum_i \widetilde A_i\dot q_i - \lambda p,
\qquad
K=\frac{\rho c^2}{V},
\qquad
V=\pi R^2L.
\]

Here \(L\) is shell depth, \(\rho\) is air density, \(c\) is sound speed, and
`Cavity.LossPerSecond` is the pressure-amplitude loss \(\lambda\).
`Cavity.Coupling01` is \(g\); scaling pressure drive and feedback by the same
coefficient preserves passivity. Setting it to zero or setting
`Cavity.Enabled` to false is the exact zero-coupling limit.

The stored mechanical energy is

\[
E =
\sum_i \frac{m_i}{2}
\left(\dot q_i^2 + \omega_i^2q_i^2\right) + \frac{p^2}{2K}.
\]

After contact ends,

\[
\dot E =
-\sum_i 2d_i m_i\dot q_i^2
-\lambda\frac{p^2}{K}\leq0.
\]

The audio-rate update solves the full rank-one coupled system with the implicit
midpoint rule. For this linear quadratic system it conserves the lossless
energy to roundoff and makes the configured losses passive. The solve is
linear in retained mode count and uses preallocated structure-of-arrays
storage.

## Output and parameter updates

`DoubleHeadOutput` keeps batter and resonant point motion and radiation
separate, then exposes their combined raw and microphone-filtered radiation.
It also reports swept volume, cavity pressure, head energy, cavity energy, and
total energy for validation.

All P3 parameters remain SI-valued fields of `PhysicalDrum`:

- batter and resonant tuning use each head's `TensionNPerM`,
  `SurfaceDensityKgPerM2`, and `BendingStiffnessNM`;
- shell geometry uses `Cavity.DepthM`;
- air stiffness uses `Cavity.AirDensityKgPerM3` and
  `Cavity.SoundSpeedMPerS`;
- `Cavity.Coupling01` continuously controls passive head/air coupling,
  `Cavity.Enabled` can bypass it, and `Cavity.LossPerSecond` controls pressure
  loss.

Call `DoubleHead.Config`, edit the returned copy, and pass it to
`DoubleHead.Reconfigure` from the model's owner goroutine. Reconfiguration
validates and fully constructs a replacement before installing it. A rejected
update leaves the sounding model untouched; a successful update resets the
dynamic state, so coefficients never change halfway through a tail.

## Frequency-domain reference

`DoubleHead.ReferenceFrequencyResponse` is an allocation-tolerant offline
check of the continuous linearized-at-rest reduced model. Nonlinear
amplitude-dependent responses require a time-domain render. At angular
frequency \(\Omega\),

\[
D_i = m_i(\omega_i^2-\Omega^2+j\,2d_i\Omega),
\qquad
Z_c = K\frac{j\Omega}{j\Omega+\lambda},
\]

and the modal system is the diagonal-plus-rank-one solve

\[
(\operatorname{diag}(D_i)+Z_cAA^\mathsf{T})q=f.
\]

The implementation uses the Sherman-Morrison form. A deterministic test drives
the real-time model with a sinusoid and compares its steady-state raw-radiation
magnitude with this independent continuous-time solution. The current
difference at 137 Hz is below 0.5 percent.

## Validation and measured cost

The P3 suite covers:

- exact single-head output in the zero-coupling limit;
- independently generated resonant modes and audible resonant-head excitation;
- analytic swept areas and zero coupling for every non-axisymmetric mode;
- earlier in-phase than out-of-phase zero crossings for identically tuned
  heads;
- lossless total-energy conservation and non-trivial energy exchange;
- monotonic energy loss with head and cavity damping;
- continuous frequency-response agreement;
- rejected-update atomicity and zero allocations in `Render`.

Before P4 nonlinear tension was enabled, the Standard linear two-head model
(96 total oscillators, 48 kHz, 512-sample chunks) measured 292 ksamples/s or
6.09 times real time on Linux/amd64, and 244 ksamples/s or 5.08 times real time
on `js/wasm` under Node, with zero render allocations.

The P4 equations, energy extension, active-solve benchmarks, and validation
results are documented in
[`physical-nonlinearity.md`](physical-nonlinearity.md).

The cavity is intentionally a single uniform-pressure state. Shell modes,
vents, non-axisymmetric leakage, and empirical head-to-head cross-coupling
remain excluded until measurements or a higher-fidelity reference show that
they are necessary.
