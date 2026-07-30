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
  fixture, velocity 0.2 measures about 104.2 to 104.0 Hz, while velocity 1.0
  measures about 110.3 to 104.0 Hz — a 102.8-cent glide, up from 37.9 cents
  before the tension coefficients were raised fourfold;
- attack-spectrum change: the centroid _above the fundamental_ rises from about
  244 Hz to 310 Hz between those velocities. It is measured above the fundamental
  because tension modulation shifts every partial upward rather than moving
  energy into the top of the spectrum, and a full-band centroid is dominated by
  the fundamental's level instead — with the corrected microphone model it moves
  only from 112.373 to 112.377 Hz, which measures nothing;
- a 48 kHz trajectory against the same nonlinear system oversampled at
  192 kHz (0.08 percent maximum displacement error in the test fixture);
- lossless nonlinear energy conservation for both the isolated head and the
  coupled two-head/cavity system;
- monotonic dissipation with configured losses, finite output at the maximum
  allowed coefficient, modal Nyquist headroom, and zero render allocations.

The Standard model was also benchmarked with a full-velocity retrigger before
every 512-sample chunk, keeping the nonlinear solve continuously active. On the
2026-07-29 development machine, at 96 total oscillators, it measured
103 ksamples/s (2.14 times real time) on Linux/amd64 and 71 ksamples/s
(1.48 times real time) on `js/wasm` under Node, with zero allocations. After P8
the same worst case measures 79.5 ksamples/s (1.66 times real time) on `js/wasm`
at 102 oscillators — a wider modal band and an added noise layer for slightly
less cost. See [`physical-hybrid.md`](physical-hybrid.md).

## The solve cost, measured

`nonlinearSolveIterations = 8` is a cap, not a cost. The fixed-point iteration
exits as soon as the tension stops moving, and at full velocity on the shipped
configuration that is a mean of **2.88** iterations. It also does not grow when
the glide is made audible: sweeping the tension coefficient over a 32-fold range
moves the mean only from 2.88 to 3.09, because a stiffer law both perturbs the
tension more and contracts faster once `tanh` begins to saturate.

This retires a planned change. P8 proposed replacing this solve with an explicit
energy-proportional detune (Avanzini et al., _JASA_ 131(1) 2012) to buy back
"6 times the voice", on the assumption that all eight iterations ran. They do
not, the real figure is about three, and it does not move — so the
discrete-gradient solve keeps its exact energy bookkeeping and nothing was traded
away for a saving that was never there.

The real limit on the tension coefficient is accuracy, not cost, and it is not
the Nyquist-bound test: that computes its bound from `MaximumTensionRatio` and
the mode wavenumbers alone, so the coefficient does not appear in it and it
cannot fail however far the coefficient is raised. What binds is the 4x
oversampled trajectory comparison, whose error grows from 0.0773 % to 0.0833 % at
the shipped fourfold coefficient against a 1.5 % ceiling, and the requirement
that the glide stay velocity-dependent: past about eight times the original
coefficient the loud hit sits on the `tanh` plateau, which flattens the glide
into a hold-then-drop and erodes exactly the expressiveness it exists for.

## Contact-model correction

The original P4 decision retained a hardness-only raised-sine force pulse
without checking its shipped duration. That was an error: HARD 0.7 produced
0.71 ms of contact and heavily excited the 300–650 Hz modes. Measurements on a
12-inch tom report approximately 8 ms for a quiet central hit and 5.5 ms for a
loud one:

- S. Dahl, [“Spectral changes in the tom-tom related to striking
  force”](https://www.speech.kth.se/qpsr/1997/1997_38_1_059-065.pdf),
  TMH-QPSR 38(1), 1997.

The corrected prescribed-force model interpolates between those measured
velocity endpoints. HARD scales the duration around the shipped stick
hardness, while the normalized pulse keeps total prescribed impulse fixed.
Coefficients are written directly into preallocated pending-force storage, so
Trigger and Render remain allocation-free. Louder strikes now shorten contact
and brighten the attack as observed, instead of velocity changing amplitude
alone.

This is still not a unilateral stick/head collision solve. A future
mass-contact model should enter only with force/displacement or multi-velocity
recordings and should use an energy-based update. The corrected prescribed
pulse is intentionally bounded and measurement-anchored rather than presented
as a complete contact simulation.
