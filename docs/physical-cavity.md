# Physical drum P3: resonant head and cavity

P3 extends the calibrated single-head model with a separately tuned resonant
head and a passive lumped cavity. The real-time implementation is
`physical.DoubleHead`; `physical.SingleHead` remains available as the P2
reference and is still bit-identical when both cavity coupling and P4
nonlinear tension are disabled.

## Reduced model

Each head has its own frequency-ordered circular modal bank. The batter and
resonant heads may use different tension, surface density, stiffness, and loss
parameters. Both draw on the same quality tier, but the resonant head keeps only
its axisymmetric modes — the only ones anything can excite through the cavity — so
the Standard configuration retains 96 batter oscillators against 6 resonant ones.
See [`physical-hybrid.md`](physical-hybrid.md) for why that reduction is
bit-exact rather than approximate.

For an axisymmetric mode with Bessel zero \(z\_{0n}\), the signed swept area is

\[
A\_{0n} = 2\pi R^2\frac{J_1(z\_{0n})}{z\_{0n}}.
\]

The angular integral of every \(m>0\) mode is zero, so those modes have exactly
zero cavity coupling. No empirical cross-coupling terms are present.

That zero is exact, and it is exact _because of the lumped reduction_ rather than
because a drum behaves that way. The cavity here is a single compliance whose
only observable is total swept volume, and an \(m>0\) shape sweeps none of it. A
real cylindrical cavity also has transverse modes — the \(j'\_{mn}\) series —
which carry azimuthal structure of their own and couple to \(m>0\) head modes
with a coefficient that is not zero. Nothing below weakens the claim; it states
what the claim rests on. See [Open: what the one-mode cavity may be
hiding](#open-what-the-one-mode-cavity-may-be-hiding).

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
K=s\,\frac{\rho c^2}{V},
\qquad
V=\pi R^2L.
\]

Here \(L\) is shell depth, \(\rho\) is air density, \(c\) is sound speed, and
`Cavity.LossPerSecond` is the pressure-amplitude loss \(\lambda\).
`Cavity.Coupling01` is \(g\); scaling pressure drive and feedback by the same
coefficient preserves passivity. Setting it to zero or setting
`Cavity.Enabled` to false is the exact zero-coupling limit.

`Cavity.StiffnessScale` is \(s\), and unlike the rest of the section it is
fitted rather than derived — see [Why the air spring is
fitted](#why-the-air-spring-is-fitted).

### Why the air spring is fitted

\(\rho c^2/V\) is the bulk stiffness of a **rigid, sealed** enclosure driven by
**pistons**. A tom is none of the three: the shell flexes, the vent leaks, and a
head's axisymmetric mode shape is not a flat plate. Using the rigid value
therefore over-predicts how much the enclosed air stiffens the axisymmetric
modes, and by a large factor rather than a marginal one.

Measured on the default 12-inch configuration with a central strike, which
excites only the modes that have swept area:

| \(s\)           | (0,1) branches   | ratio    |
| --------------- | ---------------- | -------- |
| 1 (rigid)       | 155.3 / 290.0 Hz | **1.87** |
| 0.083 (shipped) | 155.3 / 178.7 Hz | **1.15** |

The shipped value was 0.04 before the tuning default moved from 600 N/m to
1250 N/m. It had to be refitted, and roughly in proportion: a stiffer head is
less influenced by the same air spring, so holding a given _relative_ split
requires the air stiffness to rise with the head's. \(0.04 \times 1250/600 =
0.083\) is where the measurement landed as well as where that argument puts it.

A measured two-headed drum separates its two (0,1) branches by 10–20 %:
[Fischer, _Modal Analysis of a Snare Drum_, Illinois 2014](https://courses.physics.illinois.edu/phys406/sp2017/Student_Projects/Spring14/Matthew_Fischer_Physics_406_Final_Project_Sp14.pdf)
found 186 Hz with one head and 215 Hz after adding the resonant head at
unchanged tuning — "this increase is due only to the coupling between heads" — a
ratio of 1.16. The rigid value also puts its stiffened branch on top of the (2,1)
family, so it does not merely mistune the doublet: it masks a mode.

Two consequences of the rank-one coupling are worth stating, because they bound
what any choice of \(s\) can do:

- Only the **stiffened** branch moves appreciably. Eigenvalue interlacing keeps
  the lower branch between the two heads' uncoupled (0,1) frequencies — 104.0
  and 112.3 Hz here — so it cannot rise 16 % no matter how stiff the air is. The
  quantity to fit is the separation between the branches, not the absolute
  position of the audible one.
- Every \(m>0\) mode has zero swept area, so \(s\) has no effect on them at all —
  again, exactly, and again only under the lumped compliance.

`Cavity.StiffnessScale` is a fraction rather than a free gain because the rigid,
sealed, piston-driven enclosure is the stiffest case that exists; 1 is the
physical ceiling, not a neutral default. The two-head coupling loss that damps
the (0,1) is a separate mechanism, calibrated from measured decay rates in
[`physical-calibration.md`](physical-calibration.md), and is unaffected by this
refit — the model's lumped cavity is not where that loss comes from.

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

## Open: what the one-mode cavity may be hiding

The fitted \(s=0.083\) is a factor of **12** below the rigid ceiling, and the
section above attributes the whole of that to shell flex, vent leakage and the
non-flat mode shape. That is a lot to hang on three effects none of which has
been measured here separately. Part of it may instead be the one-mode reduction
itself mis-setting the compliance: a single uniform-pressure state has to stand
in for the whole enclosed field, and there is no reason its best-fit stiffness
should equal the true bulk value even in a drum where the shell were rigid and
sealed. The fit measures whatever makes the (0,1) split come out right; it does
not decide which mechanism owes the difference.

Two threads to pull, and they may be the same thread.
[`physical-excitation-gap.md`](physical-excitation-gap.md#an-observation-offered-as-a-hypothesis)
records a testable transverse-cavity hypothesis — the reference's 624.4 / 1018.4
/ 1331.3 Hz partials against \(j'\_{11}, j'\_{21}, j'\_{01}\times c/2\pi a\) =
634 / 1052 / 1320 Hz, with the measured band deficit peaking at 635 Hz — offered
there as a hypothesis and not as a finding. It is the same missing structure seen
from the other side: transverse modes would both couple to the \(m>0\) heads that
the lumped model decouples exactly, and change what the axisymmetric compliance
should have been. Neither has been tested. The claims in this document stand as
written under the reduction they are stated for.

## Output and parameter updates

`DoubleHeadOutput` keeps batter and resonant point motion and radiation
separate. The configured pickup is on the batter side, so `RawRadiated` and
the microphone-filtered output use the batter contribution. The resonant head
still changes that signal through the passive cavity coupling and remains
available as `ResonantRawRadiated` for diagnostics.

The earlier implementation added both outward-radiation signals directly as
if the heads occupied the same point with the same orientation, distance,
delay, and polarity. With a realistic stick pulse this nearly cancelled the
108 Hz coupled fundamental. Direct resonant-head radiation must remain
separate until a propagation/diffraction transfer around the shell is
available. The output also reports swept volume, cavity pressure, head energy,
cavity energy, and total energy for validation.

The microphone signal is therefore the batter contribution plus the stochastic
attack layer described in [`physical-hybrid.md`](physical-hybrid.md), and
specifically not the resonant head's own radiation.

All P3 parameters remain SI-valued fields of `PhysicalDrum`:

- batter and resonant tuning use each head's `TensionNPerM`,
  `SurfaceDensityKgPerM2`, and `BendingStiffnessNM`;
- shell geometry uses `Cavity.DepthM`;
- air stiffness uses `Cavity.AirDensityKgPerM3` and
  `Cavity.SoundSpeedMPerS`;
- `Cavity.Coupling01` continuously controls passive head/air coupling,
  `Cavity.Enabled` can bypass it, and `Cavity.LossPerSecond` controls pressure
  loss;
- `Cavity.StiffnessScale` sets how far below the rigid-enclosure limit the air
  spring sits. It multiplies the same stiffness the energy term divides by, so
  the passivity and energy-conservation results above hold for any value of it.

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
magnitude with this independent continuous-time solution. It renders with P4
nonlinearity disabled, because this reference is linearized at rest and leaving
the tension modulation on compares two different models: the residual then
depends on drive amplitude and reaches 27 % at 300 Hz, where linear against
linear agrees to 0.034 percent at 137 Hz.

## Validation and measured cost

The P3 suite covers:

- exact single-head output in the zero-coupling limit;
- independently generated resonant modes and audible resonant-head excitation;
- analytic swept areas and zero coupling for every non-axisymmetric mode;
- the fitted (0,1) split inside the measured 10–20 % band, no partial left within
  10 dB of the fundamental where the (2,1) belongs, and the rigid stiffness still
  overshooting that band — so the fit cannot be dropped without a failure;
- earlier in-phase than out-of-phase zero crossings for identically tuned
  heads;
- lossless total-energy conservation and non-trivial energy exchange;
- monotonic energy loss with head and cavity damping;
- continuous frequency-response agreement;
- rejected-update atomicity and zero allocations in `Render`.

Before P4 nonlinear tension was enabled, the Standard linear two-head model
(96 total oscillators, 48 kHz, 512-sample chunks) measured 292 ksamples/s or
6.09 times real time on Linux/amd64, and 244 ksamples/s or 5.08 times real time
on `js/wasm` under Node, with zero render allocations. Both figures predate the
nonlinear solve and are not the cost of the shipped voice; current measurements,
which do include it, are in
[`physical-nonlinearity.md`](physical-nonlinearity.md) and
[`physical-hybrid.md`](physical-hybrid.md).

The P4 equations, energy extension, active-solve benchmarks, and validation
results are documented in
[`physical-nonlinearity.md`](physical-nonlinearity.md).

The cavity is intentionally a single uniform-pressure state. Shell modes,
vents, non-axisymmetric leakage, and empirical head-to-head cross-coupling
remain excluded until measurements or a higher-fidelity reference show that
they are necessary.
