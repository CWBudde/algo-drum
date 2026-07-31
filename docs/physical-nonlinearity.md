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

It is worth being exact about which step here is the approximation, because it
is **not** the strain. The diagonal form \(S=\sum_i\Gamma_i q_i^2\) is exact: the
mode shapes are Dirichlet Laplacian eigenfunctions, so
\(\int\nabla\varphi_i\cdot\nabla\varphi_j\,dA
= k_i^2\int\varphi_i\varphi_j\,dA\) vanishes off the diagonal and the cross
terms are identically zero. All of Berger's error lives in the _second_ moment.
Writing \(g=|\nabla w|^2\), the quartic membrane potential goes as
\(\int g^2\,dA\), while Berger uses \((\int g\,dA)^2/A\) — the projection of
\(g\) onto the constant function. That it is a projection rather than a series
truncation is why \(U_\mathrm{B}\le U_\mathrm{exact}\) always, by
Cauchy–Schwarz.

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

## What the mean-field reduction cannot do

The Berger law collapses the geometric nonlinearity onto a single scalar
\(\Delta T(S)\) over the total strain measure. Every mode is therefore detuned by
the same _relative_ amount, the modal equations stay diagonal, and **no mode can
transfer energy to any other**. That is not a shortcut in this implementation;
it is the defining property of the Berger / Kirchhoff–Carrier reduction family,
and it is what makes the law cheap enough to run at audio rate with exact energy
bookkeeping.

The consequence is worth stating flatly, because nothing else in the design
record says it: **this nonlinearity contributes pitch and nothing else.** It
produces zero spectral content. A loud hit under it is a sharper copy of a quiet
hit with a glide on it, and the only two mechanisms in the whole model that can
deposit energy at a given frequency remain the contact force's spectrum and the
stochastic attack layer of [`physical-hybrid.md`](physical-hybrid.md).

A real head struck hard does more, and what it does is fixed by the parity of the
potential. The membrane's geometric nonlinearity comes from a **quartic**
potential, \(U\propto\int(|\nabla w|^2)^2\,dA\). A quartic potential is _even_ in
the modal amplitudes, so the force it produces is **cubic and odd**, and an odd
force generates only **odd** combinations:

\[
3f_a,\qquad 2f_a\pm f_b,\qquad f_a\pm f_b\pm f_c,
\]

together with the internal resonances those admit where the ratios come close to
rational, which pour energy up the mode series over the length of the tail. It
generates **no second harmonic and no simple sum or difference tone**: \(2f_i\)
and \(f_i\pm f_j\) require a _quadratic_ term in the potential, which arises for
shells, curved plates, or a structure carrying a static preload asymmetry — none
of which a flat tensioned head is. That cascade is part of why a hard hit is
_brighter_ and not merely sharper, which is exactly what Dahl 1997 measures on a
12-inch tom and which
[`physical-sound-audit.md`](physical-sound-audit.md) already cites for the
contact law. The reduced law reproduces the sharper and none of the brighter.

The parity has a direct consequence for how many modes have to be driven. The
lowest combination consumes three frequency slots, so a **single** pump mode
reaches only \(f_a\) and \(3f_a\) and nothing between them; anything else needs
at least **two** simultaneously excited modes, through \(2f_a\pm f_b\). A cubic
coupling is not a mechanism that one loud fundamental can exploit on its own.

### A hypothesis this raises for the excitation gap

[`physical-excitation-gap.md`](physical-excitation-gap.md) eliminates mode count,
microphone geometry, strike footprint, cavity coupling and tension asymmetry by
measurement, and lands on the contact force. It never considers a nonlinear
source term, and the reason is structural: **the model has none to consider.**

That leaves an untested mechanism. A cubic modal coupling deposits energy at
frequencies chosen by the mode triples, not by \(|F(f)|\) — so it does not care
that the half-sine's zero comb has driven 547 and 668 Hz to −309 dB, because it
is not driven through the excitation at all. On the face of it that is the one
known mechanism that could put resolvable content into a band the excitation has
exactly zeroed.

The parity constrains what would have to be retained for it to reach that band,
and this is a design requirement rather than a tuning preference. The (0,1) alone
cannot do it: at \(f_{01}\approx150\) Hz its only self-term is \(3f_{01}\approx
450\) Hz, which falls _below_ 476–700 Hz. Filling the gap needs at least two
distinct pump modes, through \(2f_a\pm f_b\) — so any truncation tested against
this band must retain couplings among a set of at least two simultaneously loud
low modes, and a truncation down to self-terms would be guaranteed to return
zero for reasons that have nothing to do with the coupling's strength.

This is offered as a hypothesis and not as a finding, in the same register as
that document's transverse-cavity observation. It has not been measured here:
neither the coupling coefficients for this bank nor the level the cascade would
reach at realistic strike energies has been computed, and it may well turn out
to be 30 dB below what the band needs. It is worth testing because it is cheap to
falsify — the coefficients are a known integral over the retained shapes — and
because, unlike everything already eliminated, it is a source rather than a
weighting.

The cost, if it survives testing, is a known quantity rather than an open
question: nonlinear modal synthesis with the coupling terms retained runs in real
time today (Diaz, Constanzo & Sandler 2026, in
[`physical-model-research.md`](physical-model-research.md)). Full von Kármán
coupling is \(O(N^3)\) in the mode count, so at 96 oscillators it would have to be
truncated to the dominant couplings from the loudest low modes rather than
retained wholesale.

One caution on what such a change would and would not buy. The local quartic
\(\int(|\nabla w|^2)^2\,dA\) is not exact von Kármán either. Full von Kármán
condenses the in-plane displacement through an Airy stress function, giving a
quartic with an inverse-biharmonic kernel; Berger is that family's _uniform_
limit, with the in-plane stress spatially constant, and the local quartic is its
_local_ limit, with the in-plane stress following the local slope and no elastic
redistribution at all. The truth sits between those two brackets. What adopting
the local quartic would claim is therefore the **structure** of the coupling —
which frequencies can be generated, and by which mode sets — and not its
magnitude. The coefficient is fitted either way.

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
  fixture, velocity 0.2 measures about 150.19 to 150.09 Hz, while velocity 1.0
  measures about 158.73 to 150.09 Hz — a 96.9-cent glide.

  The coefficients have now been raised twice, and the second time was forced by
  the retuning rather than chosen. A stiffer head sees a smaller _relative_
  tension excursion from the same strike, so the fourfold raise that produced
  102.8 cents at 600 N/m produced only 20.7 at 1250. Restoring an audible glide
  took another factor of two on top of that, to \(9.6\times10^6\) N/m³ on the
  batter. The cost is accuracy against the oversampled reference, which went from
  0.083 % to 0.290 % of a 1.5 % ceiling — still an order of magnitude clear, but
  no longer negligibly so, and this is the term that limits how much further the
  coefficients can go;

- attack-spectrum change: the centroid _above the fundamental_ rises from about
  360 Hz to 367 Hz between those velocities. It is measured above the fundamental
  because tension modulation shifts every partial upward rather than moving
  energy into the top of the spectrum, and a full-band centroid is dominated by
  the fundamental's level instead.

  It is also measured over a 43 ms window rather than the 171 ms it first used.
  A Hann window over 171 ms suppresses exactly the interval where the tension is
  raised and emphasises the settled middle, so once the glide grew large the
  measured centroid moved _down_ with velocity — 371.2 Hz quiet against 363.8
  loud. That was a property of the window, not of the model, and the test was
  measuring the window;

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
