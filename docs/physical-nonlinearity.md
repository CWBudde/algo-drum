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

> This section is kept as written, because it is the argument P9/M1 acted on and
> because it remains an exact description of the Berger law itself. It is no
> longer a description of the shipped instrument: the section
> [P9/M1: the coupling channels](#p9m1-the-coupling-channels) below adds the
> second moment back, and the hypothesis raised here has since been measured.

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

## P9/M1: the coupling channels

The section above is the statement of the defect. This one is what was done about
it, and the first thing to say is that the fix does not replace the Berger law —
it **adds to** it.

### The channel form

Write \(g=|\nabla w|^2\) and choose an orthonormal set of channels \(\psi_c\) on
the head. Then

\[
\hat g_c=\langle g,\psi_c\rangle=q^{\mathsf T}D^c q,\qquad
D^c\_{ij}=\int\psi_c\,(\nabla\varphi_i\cdot\nabla\varphi_j)\,dA,\qquad
U=\frac{\tilde\beta}{4}\sum_c \hat g_c^2 .
\]

The uniform channel \(\psi_0=1/\sqrt A\) gives
\(D^0=\mathrm{diag}(\Gamma_i)/\sqrt A\) and \(\hat g_0=S/\sqrt A\), so
\(U_0=(\tilde\beta/4A)S^2\) is **exactly** the Berger potential with
\(\beta=\tilde\beta/A\). That is not an approximation to it: the mode gradients
are orthogonal analytically, by Green's identity, so the c = 0 coefficient is the
existing \(\Gamma_i\) and nothing else. The implementation therefore stores only
\(c\ge1\) and leaves the shipped `tanh`-capped law untouched, which is why the
cap stays exactly where it was needed — on the channel that detunes every mode
uniformly — and is not applied to channels that detune nothing uniformly.

This form was chosen over a bare quartic tensor for four reasons: \(U\) is a sum
of squares, so \(U\ge0\) **structurally** and passivity is not conditional on the
coefficients; it degenerates to today's law exactly; the vector discrete gradient
is the existing scalar one applied per channel (below); and \(D^c\) inherits the
selection rule as its sparsity pattern instead of needing one imposed on it.

`internal/physical/coupling.go` builds the channels from
\(\{1\}\cup\{\nabla\varphi_a\cdot\nabla\varphi_b:a,b\in P\}\), Gram–Schmidt
orthonormalised offline, and truncates each \(D^c\) to entries with at least one
index in \(P\) — so every retained quartic term has **at least two** pump
indices. That is the \(|P|\ge2\) requirement made structural rather than
documented.

### The selection rule, derived

With \(\varphi=J_m(kr)C(\theta)\) and

\[
R_1=k_ak_bJ'\_{m_a}J'\_{m_b},\qquad
R_2=m_am_bJ\_{m_a}J\_{m_b}/r^2,\qquad
D=m_a-m_b,\ \ \Sigma=m_a+m_b,
\]

the gradient products are, exactly,

\[
\begin{aligned}
\cos,\cos&:\ \tfrac12(R_1+R_2)\cos D\theta+\tfrac12(R_1-R_2)\cos\Sigma\theta\\
\sin,\sin&:\ \tfrac12(R_1+R_2)\cos D\theta-\tfrac12(R_1-R_2)\cos\Sigma\theta\\
\cos,\sin&:\ -\tfrac12(R_1+R_2)\sin D\theta+\tfrac12(R_1-R_2)\sin\Sigma\theta
\end{aligned}
\]

so a quartic coefficient vanishes unless the two gradient products share an
angular order **and an orientation family**. The second condition is the one that
removes most of the tensor, and it is **not** the naive
\(\pm m_i\pm m_j\pm m_k\pm m_l=0\) rule the four-index form suggests. There is no
radial selection rule at all. Measured on the shipped Standard bank with
\(|P|=4\): of the \(|P|\cdot N\) index pairs across 10 channels, **408**
coefficients are structurally non-zero — about 89 % of the candidate table is
removed by the rule alone.

An all-cosine pump set therefore admits **no** sine-orientation receiver, and no
receiver whose azimuthal order cannot be written as \(|m-p|\) or \(m+p\) against
a channel order. `TestCouplingSelectionRuleHoldsStructurallyAndNumerically`
asserts both without a tolerance for the first and against the retained table for
the second (34 unreachable modes carry no coefficient).

### Pump selection

\(P\) is the loudest few modes under a reference velocity-1 strike, ranked in
closed form by \(|a_i\hat H(\omega_i\tau)/\omega_i|\), where \(a_i\) is
`Mode.StrikeAccelerationPerN`, \(\tau\) is the velocity-1 contact window and
\(\hat H\) is the normalised half-sine transform `addContactPulse` lays down. It
is **displacement**, not energy, because the force goes as \(q^3\); and it is not
the frequency ordering. Measured on the shipped bank:

| rank | mode      | Hz    | amplitude |
| ---- | --------- | ----- | --------- |
| 0    | (0,1) cos | 150.1 | 6.64e-2   |
| 1    | (1,1) cos | 238.7 | 1.43e-2   |
| 2    | (2,1) cos | 320.0 | 4.75e-3   |
| 3    | (0,2) cos | 344.7 | 4.60e-3   |
| 4    | (2,2) cos | 524.9 | 3.73e-3   |
| 5    | (1,1) sin | 239.7 | 2.78e-3   |
| 6    | (1,2) cos | 437.3 | 2.27e-3   |

The (2,2) at 525 Hz outranks the (1,1) sin at 240 Hz, and the (1,1) sin outranks
the (1,2) cos at 437 Hz, so a frequency-ordered set would retain different modes.
The shipped `PumpCount: 4` takes the first four.

### Passivity, and why no Gonzalez projection

\(D^c\) is symmetric, so with \(\bar q=(q^{n+1}+q^n)/2\) and \(dq=q^{n+1}-q^n\),

\[
\hat g_c^{n+1}-\hat g_c^n=2\bar q^{\mathsf T}D^c\,dq \quad\text{(exact)},\qquad
\bar T_c=2\frac{U_c(\hat g_c^{n+1})-U_c(\hat g_c^n)}{\hat g_c^{n+1}-\hat g_c^n},
\qquad
\bar F_i=-\sum_c \bar T_c (D^c\bar q)_i
\]

gives \(\bar F\cdot dq=-\,\Delta U\) exactly. Because \(U\) is a sum of functions
of scalar quadratic forms, the **scalar** secant already _is_ the vector discrete
gradient — no Gonzalez projection is needed, and there is no \(0/0\) branch to
take on a 96-vector at rest. For \(c\ge1\) the potential is the uncapped quartic
\(U_c=(\tilde\beta/4)\hat g_c^2\), whose secant is closed form and
transcendental-free, \(\bar T_c=\tilde\beta(\hat g_c^{n+1}+\hat g_c^n)/2\).
Measured, the identity holds to a relative residual of **2.5e-15**
(`TestCouplingDiscreteGradientIsExact`); the lossless coupled system drifts by
**1.1e-11** over a second, and the lossy system never gains energy at any
velocity.

The coupling sits **inside** the fixed-point loop, since \(\bar T_c\) depends on
the endpoint, and the convergence test grew from 2 scalars to \(2+C\).

### Alias control

The bound \(r<1/(4\alpha^2)-1\) bounds a _uniform_ detune and says nothing here:
the coupling force is a product of three sampled modal signals. Because every
retained entry carries a pump index, the worst case is **not** \(3f_{\rm top}\);
it is bounded by \(f_{\max P}+2f_{\rm top}\), which is what `Validate` enforces
against `AliasFraction * SampleRateHz`. Note this is the bound for a receiver
that is _itself_ a pump, which admits two free indices; a free receiver only
reaches \(2f_{\max P}+f_{\rm top}\). At the shipped bank the conservative bound is
3 631 Hz and the table actually built reaches **2 709 Hz**, both against
21 600 Hz — an 8x margin, not binding at any shipped tier. It is implemented
anyway, because it bites if `Quality` rises, if `SampleRateHz` falls toward
8 kHz, or if `PumpMaxFrequencyHz` widens.

### Passive is not the same as stable at this timestep

The section above says \(U\ge0\) structurally and that passivity is therefore not
conditional on \(\tilde\beta\). That is true and it is not a stability claim about
the **discrete** scheme, and the two were run together for longer than they should
have been. `CoefficientNPerM` was validated in \([0,10^9]\) N/m on the strength of
the passivity argument alone, and values inside that range render NaN.

The discrete failure is a loss of contraction in the fixed point, not a loss of
passivity. Linearising one sweep about the current iterate,

\[
\delta \bar T_c=-\frac{\tilde\beta h^2}{2}\,G_{cb}\,\delta \bar T_b+O(h^3),
\qquad
G_{cb}=\sum_i \frac{(D^cq)_i\,(D^bq)_i}{M_i},
\]

since a change in the channel tensions moves the modal acceleration by
\(-(1/M_i)(D^bq)_i\), the midpoint solve turns that into a displacement change of
\(h^2/2\) times it, and the secant \(\bar T_c=\tilde\beta(\hat g^{n+1}_c+\hat
g^n_c)/2\) turns that back into a tension change. The iteration therefore
contracts while

\[
\tilde\beta\,h^2\,\rho(G)<2 ,
\]

and \(G\) is **quadratic in the modal state**. This is exactly the property the
Berger law does not have: its `tanh` cap is what makes its stability independent
of displacement magnitude, and the channels have no cap, because there is no
per-channel \(T_{\max}\) to cap them at and imposing one would break the discrete
gradient identity that buys the exact energy bookkeeping.

So no \(\tilde\beta\) ceiling derivable from the configuration alone can be
sufficient. Measured — bisecting the largest coefficient whose one-second
velocity-1 render stays finite, before the guard below existed:

| configuration           | last finite \(\tilde\beta\) |
| ----------------------- | --------------------------- |
| quality high, 48 kHz    | 6.98e8                      |
| pumps 8, 4096 coeffs    | 9.90e8                      |
| pumps 8, 96 kHz         | 3.16e9                      |
| shipped default, 48 kHz | 5.59e9                      |
| 44.1 kHz                | 5.71e9                      |
| quality draft           | 2.30e10                     |
| 96 kHz                  | 8.91e10                     |
| 192 kHz                 | 3.49e11                     |
| default at velocity 0.1 | 7.29e11                     |

The scaling is the derivation's. Halving the step raises the threshold about as
\(h^{-2}\); dropping the strike velocity tenfold raises it by a factor of 130
against the \(|q|^{-2}\) prediction of 100. Two rows sit **below** the \(10^9\)
that used to validate, which is how a validated document came to render 52754
non-finite samples with a peak of 3.0e9. There is also a band below the NaN, from
about 1.4e8, where the render is quietly wrong before it is loudly wrong.

Two things changed. `maxCouplingCoefficientNPerM` is now \(10^8\) — a seventh of
the worst measured row, below the quiet band as well as the loud one, and a factor
of 143 above the shipped 7.0e5. That margin is wider than `MaximumTensionRatio`'s
0.2 against 0.2346 on purpose: that bound is derived and exact, this one is a
bisection over a finite sweep of a quantity that is not a function of the
configuration.

And the solve defends itself, because a ceiling in `Validate` does nothing for a
`PhysicalDrum` that reaches `Render` some other way. `DoubleHead.solveNonlinearStep`
watches the fixed point's own residual: a contraction's residual falls
geometrically, so growth by more than `couplingResidualGrowth` (4) means the map is
not a contraction at this state and its iterate carries no information. The step is
then re-solved from the same pre-step state with the coupling off — nothing has
been committed at that point, so it is a clean redo rather than a rollback — which
lands on the Berger-only update whose stability is unconditional. It is
self-healing: the coupling re-engages as soon as the state decays back inside the
contraction region.

The guard sits here rather than in `internal/drum` because that hard clamp is
per-sample and the model's own state is what goes non-finite: a NaN there poisons
the FDN reverb's delay lines and the limiter's lookahead on the way to the clamp,
and the voice never recovers. It is also the only place the _reason_ is visible.
Measured across every quality tier, sample rate and velocity at coefficients from
7.0e5 to a hundred times it, the largest residual growth a converging solve
produces is **0.126** — a contraction with room to spare, not a marginal one — so
the threshold of 4 has a factor of 30 in hand. `CouplingDivergedSteps()` reports
how often it fired; on everything the validator admits it is zero, which is what
keeps the shipped voice bit-identical.

### What it does, measured

The behavioural test zeroes the strike projection of every mode outside \(P\), so
the **only** path into any other mode is the cubic force. It reads modal
amplitudes rather than the radiated spectrum, deliberately: four damped sinusoids
have Lorentzian skirts that put a −37 dB floor across 476–700 Hz, and that floor
is leakage from the pumps rather than content in the band.

| band       | best non-pump mode | uncoupled | coupled | rise    |
| ---------- | ------------------ | --------- | ------- | ------- |
| 476–700 Hz | (2,2) cos, 524.9   | −76.3 dB  | −28.4   | 47.8 dB |
| 700–1000   | (2,3) cos, 725.4   | −86.4 dB  | −34.6   | 51.8 dB |

With the cavity disabled the uncoupled figure is not small but **exactly zero** —
those modes never move at all. This is the mechanism
[`physical-excitation-gap.md`](physical-excitation-gap.md) never considered
because the model had none: a mode pumped by coupling does not depend on
\(|F(f)|\) at its own frequency, so it can be excited precisely where the
half-sine's zero comb has deleted the excitation outright.

The same effect shows up as a change to a P8 measurement. In
`TestHertzianContactReachesPastTheModalCeiling` the Hertzian contact's advantage
over the prescribed one at 800 Hz fell from **11.9 dB to 7.9 dB**, with 1500 Hz
(15.2 → 15.5) and 2500 Hz (22.9 → 22.9) unmoved. The prescribed side rose; the
Hertzian model did not lose ground. `docs/physical-contact.md` now carries both
states of that table, and its "What the mode coupling changed" section is where
this effect is laid out frequency by frequency.

### The Dahl slope

With the attack layer off and the contact duration frozen at its velocity-1 value
— so the pulse spectrum is the same shape at every dynamic — the attack centroid
above the fundamental, over the same 43 ms window P4 uses:

| velocity | uncoupled | coupled |
| -------- | --------- | ------- |
| 0.2      | 272.5 Hz  | 274.9   |
| 0.6      | 301.5     | 317.4   |
| 1.0      | 322.3     | 343.9   |

a slope of **64.7 Hz** per unit velocity uncoupled against **89.5 Hz** coupled.
Two honest caveats. This is not an isolation of the coupling: the Berger detune
raises every partial with amplitude and moves a centroid on its own, which is
where the 64.7 comes from, so the number that means something is the difference.
And Dahl's published slope is not in this repository, so the test asserts sign,
monotonicity and that the coupling steepens the slope — not a fitted match.

### Calibration status

**The P4 glide calibration is pending a refit.** On the isolated P4 fixture the
loud glide moved from **102.8 to 104.9 cents** and the test still passes inside
its \[60,140\] window. That is a much smaller move than the ratio
\(\int g^2\,dA/(S^2/A)\) would suggest, and for a specific reason: the P4 fixture
strikes the head at its centre, where only axisymmetric modes are excited, so the
orthogonal channels see very little. On the off-centre shipped strike the
coupling is a real addition to the stiffening and the coefficient has not been
refitted against a reference recording. `CoefficientNPerM` ships at
\(\beta A=7.0\times10^5\) N/m, which is the same coefficient the uniform channel
already carries — it is not a new free parameter, but it is also not a fitted
one. Refitting it, and the Berger \(\beta\) with it, is follow-up work recorded
against P9/M1 in `PLAN.md`.

### Cost

The `BenchmarkNonlinearDoubleHeadActive48k` worst case — full-velocity retrigger
before every 512-sample chunk, so the nonlinear solve never idles — at 120
oscillators, medians of five runs, zero allocations throughout:

| coefficients  | host (amd64) | `js/wasm` (node) |
| ------------- | ------------ | ---------------- |
| off           | 4.39x        | 1.40x            |
| 128           | 2.65x        | 0.79x            |
| 256 (shipped) | 2.06x        | 0.70x            |
| 408 (full)    | 1.37x        | 0.58x            |

This is the honest number and it is not good: at the shipped 256 the `js/wasm`
worst case is **below real time**. Three things soften it and none of them make it
acceptable indefinitely. It is the worst case, not the steady one — a real hit
lets the solve idle. The mean fixed-point iteration count barely moved (2.404 →
2.491 at velocity 1), so the cost is the table walk itself and not a
harder solve. And nothing here is optimised: the table is walked as three
separate index arrays with no blocking by channel or by receiver, and the whole
thing is rebuilt per iteration rather than being updated incrementally. Making
this affordable is open work, not a closed question.

The budget is also not currently buying much. Measured on the (2,2) at 524.9 Hz
under the pumps-only excitation, the 476–700 Hz level runs −29.30 dB at 64
coefficients, −28.79 at 128, −28.43 at 256 and −28.53 at the full 408 — a
0.9 dB spread across a sixfold budget, and the full table is not even the loudest
of them. So `MaxCoefficients: 256` is chosen for margin rather than because the
spectrum needs it, and 128 is the first thing to try if the cost has to come
down.

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

Both tension coefficients are validated in \([0,10^9]\) N/m³, and both render
finite at that limit — the smooth cap makes stability independent of displacement
magnitude, which is exactly the property the coupling channels lack (see
"Passive is not the same as stable at this timestep"). It is also restricted
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
