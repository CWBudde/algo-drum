# Physical drum P3: resonant head and cavity

P3 extends the calibrated single-head model with a separately tuned resonant
head and a passive cavity. The cavity was a single lumped compliance until P9/M2
replaced it with a short modal expansion; the uniform member of that expansion
_is_ the old compliance, exactly, and `Cavity.ModeCount = 1` reproduces the
earlier model bit for bit. The real-time implementation is
`physical.DoubleHead`; `physical.SingleHead` remains available as the P2
reference and is still bit-identical when both cavity coupling and P4
nonlinear tension are disabled.

## Reduced model

Each head has its own frequency-ordered circular modal bank. The batter and
resonant heads may use different tension, surface density, stiffness, and loss
parameters. Both draw on the same quality tier, but the resonant head keeps only the modes
the cavity can reach — the only ones anything can excite — so the Standard
configuration retains 96 batter oscillators against 28 resonant ones at the
shipped six-state cavity, and against 6 when the cavity is the single lumped
compliance. See [`physical-hybrid.md`](physical-hybrid.md) and
[What `AxisymmetricOnly` now means](#what-axisymmetriconly-now-means) for why that
reduction is bit-exact rather than approximate.

## The enclosed air

The air is expanded in the rigid-walled cylinder's own modes. A rigid wall
carries no normal velocity, so the radial condition is Neumann — \(J_m'(j'_{mn})
= 0\) — and *not* the \(J_m(z_{mn}) = 0\) the clamped heads obey. Restricted to
the axially uniform family,

\[
\Psi_{mn}(r,\theta) = J_m\!\left(j'_{mn}\frac{r}{a}\right)
\{\cos m\theta,\ \sin m\theta\},
\qquad
\omega_{mn} = \frac{c\,j'_{mn}}{a}.
\]

The uniform mode is the \(m = 0\) root at \(j' = 0\), carried as \((0,0)\); it
is a mode of zero frequency and is the whole of the old lumped model. Note that
\(j'_{01} = 3.8317\) is the first _positive_ zero of \(J_0' = -J_1\) and is a
different, non-degenerate mode.

**Axially uniform only, deliberately.** An axial order \(l\) would make the
pressure vary along the shell, and the coupling coefficient would then be \(+1\)
at the batter head and \((-1)^l\) at the resonant one instead of the same at
both — a different and larger change than this. The first axial mode also sits at
\(c/2L = 858\) Hz for the shipped 0.2 m depth, above the transverse modes this
exists to add. This is a first cut, not the complete cavity.

At the shipped 12-inch radius the retained six states are

| mode          | \(j'\) | frequency |
| ------------- | ------ | --------- |
| (0,0) uniform | 0      | 0 Hz      |
| (1,1) cos/sin | 1.8412 | 659.5 Hz  |
| (2,1) cos/sin | 3.0542 | 1094.0 Hz |
| (0,1)         | 3.8317 | 1372.5 Hz |

and at the \(a = 0.1584\) m that
[`physical-excitation-gap.md`](physical-excitation-gap.md) states its hypothesis
for, the same table reads 634.5 / 1052.6 / 1320.5 Hz — the three numbers that
document compares the reference's 624.4 / 1018.4 / 1331.3 Hz partials against. A
test pins the generated table to them.

### Coupling coefficients

The coefficient between head mode \(i\) and cavity mode \(c\) is the overlap of
their shapes over the head's disc,

\[
C_{ic} = \int_A \phi_i \Psi_c \, \mathrm{d}A,
\]

and the same coefficient appears in both directions — the air is driven by
\(C_{ic}\dot q_i\) and the head is loaded by \(C_{ic}P_c\). That symmetry is
what makes the coupling passive.

The angular integral separates and gives a **selection rule**: it vanishes unless
the azimuthal orders match, and — at an unrotated principal tension axis — unless
the orientations match too. A head mode therefore couples to at most one cavity
mode per radial order rather than to all of them, which is what makes the
extension affordable at all: 44 of the 768 head/cavity pairs at the shipped basis
are non-zero. A head whose principal axis is rotated by \(\psi\) couples to both
orientations through a plane rotation through \(m\psi\), which is an isometry, so
its total coupling strength is unchanged.

Against the uniform mode, \(\Psi = 1\) and the overlap is the mode's signed swept
area,

\[
A_{0n} = 2\pi R^2\frac{J_1(z_{0n})}{z_{0n}},
\qquad
A_{mn} = 0 \text{ for } m > 0,
\]

which is the closed form the lumped model always used and which the code returns
verbatim in that case. For every other cavity mode the radial integral

\[
\int_0^R J_m\!\left(z_{mn}\frac{r}{R}\right)
J_m\!\left(j'_{m\nu}\frac{r}{R}\right) r\,\mathrm{d}r
\]

has no clean closed form — the two Bessel functions have different arguments and
different boundary conditions, so the Lommel collapse the swept area enjoys does
not happen — and is evaluated by 96-point Gauss-Legendre quadrature once per
coupled pair when the model is built. Nothing in `Render` touches it. A test
checks the quadrature against the analytic swept area at \(j' = 0\), where they
must agree; they do, to \(2\times10^{-16}\).

### What `AxisymmetricOnly` now means

`Head.AxisymmetricOnly` reads as "retain only the modes the enclosed air can
reach". With one cavity state that set is \(m = 0\) and the field means literally
what it is named; with the shipped transverse basis it widens to \(m \le 2\), and
the resonant head keeps 28 oscillators rather than 6. The reduction stays
**bit-exact** either way, because the selection rule zeroes the coupling of every
head mode whose order appears in no cavity mode and those modes provably never
leave zero. A regression test asserts sample-identical renders at both ends.

The field is widened rather than rejected. Rejecting the combination would force
the resonant head onto the full 96-slot bank to gain the same 22 modes that can
move plus 68 that provably cannot, which is not a correctness improvement but a
doubling of the audio budget spent on exact zeros.

With modal displacement \(q_i\), modal mass \(m_i\), loss rate \(d_i\), and
the product control \(g\in[0,1]\), define the effective coupling
\(\widetilde C_{ic}=gC_{ic}\). Each cavity mode carries a pressure \(P_c\) and a
conjugate state \(H_c\), and the coupled equations are

\[
\ddot q_i + 2d_i\dot q_i + \omega_i^2 q_i
= f_i/m_i - \sum_c \widetilde C_{ic} P_c/m_i,
\]

\[
\dot P_c = K_c\sum_i \widetilde C_{ic}\dot q_i + \omega_c H_c - \lambda P_c,
\qquad
\dot H_c = -\omega_c P_c,
\]

\[
K_c = s\,\frac{\rho c^2}{\Lambda_c},
\qquad
\Lambda_c = \int_V \Psi_c^2\,\mathrm{d}V,
\qquad
\Lambda_{(0,0)} = V = \pi R^2L.
\]

Here \(L\) is shell depth, \(\rho\) is air density, \(c\) is sound speed, and
`Cavity.LossPerSecond` is the pressure-amplitude loss \(\lambda\).
`Cavity.Coupling01` is \(g\); scaling drive and feedback by the same coefficient
preserves passivity. Setting it to zero or setting `Cavity.Enabled` to false is
the exact zero-coupling limit.

The \((P_c, H_c)\) pair is the second-order cavity mode written in first-order
form: eliminating \(H_c\) gives \(\ddot P_c + \omega_c^2 P_c\) driven by the
head accelerations, which is the standard modal acoustic formulation. Writing it
this way rather than as a displacement/velocity pair is what makes the uniform
mode degenerate cleanly: \(\omega_{(0,0)} = 0\), so \(H\) never leaves zero and
the first equation collapses to the single \(\dot p = K\sum\widetilde A_i\dot
q_i - \lambda p\) the lumped model had.

The Neumann normalization gives \(\Lambda_c\) in closed form,

\[
\Lambda_{mn} = L \cdot \{\pi, 2\pi\} \cdot
\frac{a^2}{2}\left(1 - \frac{m^2}{j'^2_{mn}}\right)J_m(j'_{mn})^2,
\]

positive for every admissible mode because \(j'_{mn} > m\), and equal to the
cavity volume for the uniform mode.

`Cavity.StiffnessScale` is \(s\) and multiplies every \(K_c\) alike, so the
rigid ceiling keeps its meaning. Unlike the rest of the section it is fitted
rather than derived — see [Why the air spring is
fitted](#why-the-air-spring-is-fitted).

### Why the air spring is fitted

\(\rho c^2/V\) is the bulk stiffness of a **rigid, sealed** enclosure driven by
**pistons**. A tom is none of the three: the shell flexes, the vent leaks, and a
head's axisymmetric mode shape is not a flat plate. Using the rigid value
therefore over-predicts how much the enclosed air stiffens the axisymmetric
modes, and by a large factor rather than a marginal one.

Of those three, **the vent is the small one**, and it is worth putting a number
on it rather than leaving it in a list. A vent is a Helmholtz port, so it is a
_high-pass_ leak: below \(f_H\) the air flows out and no pressure builds, and
above it the plug's inertia blocks the flow and the cavity behaves as though
sealed. With the shipped \(V = 1.459\times10^{-2}\ \mathrm{m^3}\) and a 7 mm
shell,

| vent diameter | \(f_H\) | flow diverted at the 150 Hz (0,1) |
| ------------- | ------- | --------------------------------- |
| 6 mm          | 21.8 Hz | 2.1 %                             |
| 10 mm         | 32.2 Hz | 4.6 %                             |
| 19 mm         | 50.0 Hz | 11.1 %                            |
| 25 mm         | 59.6 Hz | 15.8 %                            |

with the diverted fraction \(1/(\omega^2M_aC_a)\) falling as \(1/f^2\) above
\(f_H\), and three 10 mm vents giving \(f_H = 55.8\) Hz and about 14 %. A
typical single vent therefore softens the effective stiffness by a few per cent
at the fundamental and less at every higher mode, where \(s = 0.083\) is a
twelvefold reduction and would need roughly 92 % of the flow diverted. The vent
cannot be more than a few per cent of the discrepancy; **shell flex and the
non-piston mode shape have to carry the rest**.

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
- Every \(m>0\) mode has zero swept area, so \(s\) has no effect on them through
  the uniform mode at all — exactly. It does act on them through the transverse
  modes, which is what P9/M2 added; but those are resonators rather than springs,
  so they change the shape of the response near their own frequencies rather than
  the doublet the fit is made against.

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
\left(\dot q_i^2 + \omega_i^2q_i^2\right)

- \sum_c \frac{P_c^2 + H_c^2}{2K_c}.
  \]

After contact ends,

\[
\dot E =
-\sum_i 2d_i m_i\dot q_i^2
-\sum_c\lambda\frac{P_c^2}{K_c}\leq0.
\]

The coupling terms cancel identically: the head equation loses
\(\sum_c P_c\sum_i\widetilde C_{ic}\dot q_i\) and the cavity equation gains
exactly the same quantity, because the same coefficient appears in the drive and
in the load. The \(\omega_c\) rotation between \(P_c\) and \(H_c\) cancels
against itself for the same reason. So the argument is the lumped one with a sum
where there used to be a single term, and it holds for any number of cavity modes
and any \(s\). Cavity loss acts on \(P_c\) only, which damps the pair at
\(\lambda/2\) in amplitude and is dissipative for either sign of \(H_c\).

The audio-rate update solves the full coupled system with the implicit midpoint
rule. Substituting the head modes' own midpoint velocities into the cavity
equations leaves, for each cavity mode,

\[
P_c\left(\frac{2}{\Delta t} + \lambda + \frac{\omega_c^2\Delta t}{2}\right)

- K_c\sum_b\sum_i \frac{\widetilde C_{ic}\widetilde C_{ib}}{m_iD_i}P_b
  = \frac{2P_c^{\text{old}}}{\Delta t} + \omega_c H_c^{\text{old}}
- K_c\sum_i \widetilde C_{ic}u_i,
  \]

a \(k\times k\) Woodbury solve where the lumped model had one Sherman-Morrison
division. The matrix is \(\operatorname{diag}(K_c)\) times a symmetric positive
definite matrix — the diagonal term is strictly positive and the coupling block is
a Gram matrix with positive weights \(1/(m_iD_i)\) — so every pivot is positive
and elimination without pivoting is safe. \(k\) is capped at 8 by
`maxCavityModes` and validated at decode. At \(k = 1\) the elimination is
literally the old single division, which is what keeps a one-mode cavity
bit-exact rather than merely equivalent; a test writes out the rank-one form by
hand and compares to the last bit for four thousand samples.

Assembly stays linear in the retained mode count, because the selection rule
restricts each head mode to its own azimuthal family, and uses preallocated
structure-of-arrays storage. `Render` allocates nothing.

## Closed: what the one-mode cavity was not hiding

This section used to record an open question. P9/M2 answered it, in the negative,
and the answer is worth more than the resonances the work added.

The fitted \(s = 0.083\) is a factor of **12** below the rigid ceiling. One
candidate explanation was that the one-mode reduction itself mis-set the
compliance: a single uniform-pressure state stands in for the whole enclosed
field, and there is no obvious reason its best-fit stiffness should equal the true
bulk value. **It does.** Every cavity mode other than the uniform one has zero net
volume — that is what \(\int_A\Psi_c\,\mathrm{d}A = 0\) for \(j' \neq 0\)
says — so none of them contributes anything to the quasi-static air spring the
\((0,1)\) doublet measures. In the frequency-domain form each contributes
\(Z_c = K_c(j\Omega)^2/((j\Omega)^2 + \lambda j\Omega + \omega_c^2)\), which
tends to zero as \(\Omega\to0\). The static compliance of the full modal
expansion is \(\rho c^2/V\) exactly, and it was already exactly that with one
mode.

Measured, rather than argued: at \(s = 0.083\) the \((0,1)\) doublet is
155.3 / 178.7 Hz, a ratio of **1.1509**, and it is 1.1509 both with one cavity
state and with the shipped six. It does not move to the FFT's 2.93 Hz resolution.
**\(s\) would still have to be 0.083** to hold the doublet where measurement puts
it, and the number that would have restored a 1.16 ratio — about 0.09 — is
likewise unchanged by the transverse modes.

Together with the vent arithmetic above, that leaves shell flex and the non-piston
mode shape as the only remaining candidates for the factor of twelve. Neither has
been measured here separately, and that is now the open question rather than this
one.

## What the transverse modes actually do

Measured at the shipped default against the same configuration with
`Cavity.ModeCount = 1`:

- **\(m > 0\) head modes receive energy for the first time.** The resonant head's
  \(m > 0\) modes go from _exactly_ zero stored energy to a peak of
  \(1.4\times10^{-6}\) J on a full-velocity off-centre strike, and the transverse
  cavity pressure peaks at 8.9 Pa where it was identically zero. The batter's
  \(m > 0\) modes lose 0.08 % of their peak energy to the air they now push.
- **It has to be an off-centre strike.** A centre hit puts no energy into any
  \(m > 0\) head mode at all, and the transverse cavity modes are driven _by_
  \(m > 0\) head modes, so a centre hit leaves them silent under either setting.
  That is the selection rule seen from the excitation side, not a defect.
- **The transfer function changes materially near the transverse frequencies.**
  Sweeping the continuous reference over 100–1500 Hz, the batter-side response
  moves by up to **13.1 dB at 657.5 Hz** — the \((1,1)\) mode is at 659.5 Hz —
  and by 0.52 dB on average across the band.
- **The feature is the shell's and the air's, not the head's.** The \((1,1)\)
  cavity pressure peaks at 664.8 Hz by default, 767.5 Hz at 1.15× sound speed
  (\(\times1.155\)), 579.0 Hz at 1.15× radius (\(\times0.871\)), and 663.0 Hz at
  1.4× head tension — a 40 % tension change, five semitones on every head mode,
  moves it by 0.26 %. That is
  [PLAN's](../PLAN.md) M2 confirm criterion, and a test asserts it.
- **The microphone barely notices.** Broadband RMS over a second moves from
  0.057609 to 0.057755 and the peak from 0.8954 to 0.8932. The configured pickup
  is on the batter side and the resonant head's own radiation is deliberately not
  summed into it, so the new energy reaches the listener only as the reshaping of
  the batter response above.

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
Z_c = K_c\frac{(j\Omega)^2}{(j\Omega)^2+\lambda j\Omega+\omega_c^2},
\]

and the pressures follow from the \(k\times k\) complex system

\[
(I + ZM)P = ZS,
\qquad
M_{cb}=\sum_i \frac{\widetilde C_{ic}\widetilde C_{ib}}{D_i},
\qquad
S_c=\sum_i \widetilde C_{ic}u_i,
\]

whose \(k = 1\) case is the Sherman-Morrison form this replaced and whose
\(\omega_c = 0\) impedance is the old \(Kj\Omega/(j\Omega+\lambda)\).
`FrequencyResponse.CavityPressuresPa` returns every mode's pressure, which is how
the transverse resonance is tracked against shell radius and sound speed. A deterministic test drives
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
- analytic swept areas, and the uniform cavity mode's overlap integral
  reproducing them exactly while the 96-point quadrature agrees with them to
  2e-16;
- the cavity mode table against the analytic \(c\,j'_{mn}/2\pi a\) series at the
  radius the transverse hypothesis is stated for;
- the azimuthal selection rule, as exact zeros rather than small numbers, and the
  isometry of a rotated principal tension axis;
- a version-10 document rendering bit-identically after migration, and the
  one-mode midpoint solve reproducing the hand-written rank-one elimination to the
  last bit;
- passivity with the transverse states active — exact conservation without losses,
  monotone decrease with them — with a check that the new states are not empty;
- \(m > 0\) resonant modes moving only once transverse modes exist, and the
  transverse resonance tracking shell radius and sound speed but not head tension;
- the enclosed-air state count bounded, and the retained modes held inside the
  same anti-alias limit as the heads;
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

### What the transverse modes cost

Measured with `BenchmarkNonlinearDoubleHeadActive48k` — 512-sample chunks,
retriggered at full velocity before each one, so the nonlinear solve never idles —
on one Linux/amd64 laptop, medians of six runs:

| `Cavity.ModeCount`   | Oscillators | host  | `js/wasm` under Node |
| -------------------- | ----------- | ----- | -------------------- |
| 1 (the lumped model) | 102         | 7.28× | 2.94×                |
| 3 (uniform + (1,1))  | 114         | 5.3×  | 1.75×                |
| 6 (shipped)          | 124         | 4.05× | 1.42×                |

so the shipped basis costs **1.8× on host and 2.1× on `js/wasm`**. Scaling the
1.66× figure [`physical-hybrid.md`](physical-hybrid.md) quotes for the worst case
by the same factor puts it near **0.8× real time**, and `ModeCount = 3` near
1.0×. That is the honest headline of this change and it wants deciding rather
than absorbing.

Most of it is not the \(k\times k\) solve — profiling puts `solveCavityMidpoint`
at 8.6 % of render time. It is the resonant head, which grows from 6 oscillators
to 28 because \(m \le 2\) modes are no longer unreachable. The obvious mitigation
is a resonant-head mode budget separate from `Quality.ModeLimit`, which would let
the reachable orders be retained without spending a full 96-slot selection on
them; that is not implemented here.

Still excluded: axial cavity order, shell modes, vents, non-axisymmetric leakage,
and empirical head-to-head cross-coupling, until measurements or a
higher-fidelity reference show that they are necessary.
