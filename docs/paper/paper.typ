#import "style.typ": code-path, paper

#let revision = sys.inputs.at("revision", default: "working tree")

#show: paper.with(
  title: "Matching a Physical Drum Model to a Recording",
  subtitle: "A perceptual distance in readable units, and what it establishes about a modal tom",
  author: "Christian-W. Budde",
  revision: revision,
  abstract-body: [
    Fitting a physical instrument model to a recording needs an objective that a
    person can read. This paper describes one: a recording and a render are
    reduced to the same set of perceptually named quantities --- partial
    frequencies and per-partial ring times, a time-varying fractional-octave
    envelope, an amplitude envelope, pitch glide, attack balance, and coverage in
    both directions --- and each term of the distance is reported in its own
    unit, so that a result can be read against a tolerance rather than only
    against another run. Adoption is decided per term. The reference reduction
    measures inter-channel delay and aligns before averaging, which on this
    recording decides which partials survive detection at all. The
    search is deterministic given its seed, checkpointed, and refuses to resume
    across a change in the model or the measurement.

    The instrument the method is applied to is described here in full, because
    every conclusion the fit reaches is a claim about it: two membranes coupled
    through a lumped air spring, a Berger tension nonlinearity closed by a
    discrete gradient, a three-term loss law shaped to hold constant $Q$, a
    microphone model that is a Lommel integral plus an evanescent term, and a
    stochastic layer whose releases are read off that same loss law past the
    modes the budget can afford. Which of its numbers are derived, which are
    measured and which are fitted is reported alongside them.

    Applied to that tom, the apparatus takes the distance from 32.6 at
    the shipped default to 11.6, bringing five of the nine terms below their
    thresholds --- amplitude envelope, glide, attack balance and both coverage
    shares --- and it localises what remains. What remains is a *tilt* in the
    decay structure: the model's mid-band partials ring two to five times longer
    than the reference's while its fundamental rings less than half as long, and
    the head-damping parameter, which scales all loss rates together, is fitted
    against the lower bound of its range. That is a specific, testable statement,
    and it is put to the test here: given four times the head-damping headroom
    through a build-time multiplier that leaves the shipped parameter range
    untouched, the search chooses the same physical damping to within 2%. The
    residual is therefore a question about the *shape* of the loss law rather
    than about its range. A second experiment exploits the fact that mode
    frequencies are analytic: a pre-solve seeds restarts with banks whose modes
    already lie on the reference's partials, worth 12% at identical cost once the
    seeded box is restricted to the dimensions the pre-solve has evidence about.
  ],
  status-body: [
    The fitted numbers in @results come from one complete report, produced by a
    run that was stopped by hand at 47% of its restart budget so the machine
    could be given the next experiment; the figures and every number in that
    section are read from it. @lossscale and @seeding report further runs at
    their own stated budgets, each paired against a control measured at the same
    budget.

    Those runs predate the correction to the partial-level estimator reported in
    @levels (2026-07-31), which changed the reference's partial list and every
    candidate's. Their totals and term values are therefore *not comparable to
    anything measured after that date*, and are reported here for the method they
    establish rather than as current results; @levels reports the correction and
    what it re-scopes. Under the corrected reduction the shipped bank measures 33.094
    with prescribed contact and 33.544 with Hertzian, at the Standard tier
    against the right channel. Full-budget fits on the corrected metric are in
    progress and are not reported here.
  ],
)

#set text(size: 8.8pt)
#set par(leading: 0.5em)

= The problem

Fitting a physically parameterised synthesiser to a recording is an optimisation
whose objective is the whole difficulty. A waveform difference is unusable
against a recording of a different physical instrument. A spectral distance is
usable but opaque: it produces one number, and when that number stops improving
there is nothing to say about what is still wrong.

The apparatus described here takes the opposite approach. Every term is a
quantity a listener would name, in the unit that quantity is perceived in, and
the sum exists only to order candidates --- decisions are made term by term. The
result is that a fit does not merely converge; it says what it has achieved and
what it has not, and the latter turns into statements about the model.

= The instrument <instrument>

The instrument is a double-headed tom rendered by modal synthesis: two membranes
coupled through a lumped air cavity, with a Berger tension nonlinearity, a
microphone model, and a three-band stochastic attack layer covering the transient
region modal synthesis reaches poorly. Its configuration is SI-valued and
versioned; the product exposes eighteen normalised parameters over it.

This chapter describes it in enough detail to argue with. That is not
scene-setting. Every conclusion the rest of the paper reaches is a statement
*about this model* --- that its head-damping bound is not binding, that the
residual lives in the shape of its loss law rather than in the range of any
parameter, that its mode frequencies are analytic and therefore cheap enough to
seed a search from --- and none of those can be checked, or disputed, from a
description that stops at the word "modal".

== Two membranes, an air spring, and a microphone

The signal path is short and its asymmetries are deliberate. A strike deposits
force at one point on the *batter* head, spread over a finite contact footprint.
The batter head's modes are the only ones the stick reaches. The *resonant* head
is driven solely by the enclosed air, which behaves as one lumped spring
compressed by the net volume the two heads sweep --- so the two membranes are
coupled through a single scalar and not through a field. Each head's total strain
raises its own tension, which detunes every one of its modes together. A
stochastic layer, driven by the same contact force, supplies the band above the
highest mode the budget can afford. What the listener hears is a weighted sum of
the batter head's modal accelerations and that layer, band-limited and scaled.

Two properties of that path are worth naming immediately, because they are
easy to assume otherwise. Only the batter head radiates into the output: the
resonant head is fully coupled into the dynamics but its own radiation leaves the
far side of the shell, and adding it at the same point, phase and distance would
be a fiction. And only *axisymmetric* modes drive the cavity, because the air
responds to swept volume and every other mode sweeps exactly none.

== The modal basis

Each head is a lossy tensioned circular membrane with a small bending stiffness,
fixed at the rim:

$ sigma w_(t t) + d_0 w_t - d_2 laplace w_t + D laplace^2 w - T laplace w = f $ <pde-eq>

with surface density $sigma$, tension $T$, bending stiffness $D$, and the loss
coefficients of @loss-eq. On a circle of radius $R$ the eigenfunctions are
Fourier--Bessel,

$ phi_(m n) (r, theta) = J_m (alpha_(m n) r \/ R) dot cases(cos m theta, sin m theta) $ <shape-eq>

where $alpha_(m n)$ is the $n$-th positive zero of $J_m$, so that the fixed-edge
condition $J_m (k R) = 0$ holds by construction. Substituting @shape-eq into
@pde-eq gives the dispersion relation the whole instrument's pitch follows from:

$ omega_(m n)^2 = T / sigma k_(m n)^2 + D / sigma k_(m n)^4, quad k_(m n) = alpha_(m n) \/ R $ <dispersion-eq>

*This is the reason a pre-solve is possible at all.* @dispersion-eq is closed
form: a bank's mode frequencies are read off tension, density and radius without
rendering a sample, which is what @seeding exploits at roughly a hundredth of the
cost of one evaluation. Zeros of $J_m$ are found once per process by a sign-change
scan and bisection and then memoised, orders 0 to 28 in both indices, because mode
generation is otherwise the dominant cost of a fit.

*The basis is the membrane's, and the bending stiffness rides on it as a
perturbation.* A fourth-order operator requires two boundary conditions and its
eigenfunctions are combinations of $J_m$ and $I_m$; pure Fourier--Bessel shapes
are the $D = 0$ ones, which is why a single fixed-edge condition can determine
them at all. So @dispersion-eq carries $D$ and @shape-eq does not: the
frequencies know about the stiffness, the mode shapes --- and with them the modal
masses, the strike weights and the radiation weights of @lommel-eq --- do not.
What is neglected is of size $D k^2 \/ T$, which at the shipped batter values
($D = 0.001$ N#sym.dot.c m, $T = 1250$ N/m) is 0.0149 at the highest retained
mode, $k = 136.4$ m#super[--1]. That is 0.74% in frequency there, about 12.8
cents --- larger than the 0.4% and 0.3% asymmetry splits of @split-eq, which the
model treats as audible. Removing the approximation means solving the genuinely
clamped fourth-order problem --- zero displacement *and* zero slope at the rim
--- for its own eigenfunctions, and giving up the closed form the pre-solve of
@seeding rests on.

Every $m > 0$ eigenvalue is doubly degenerate on an ideal circle, and no real head
is one. A single reduced parameter splits each pair deterministically about its
own midpoint,

$ f^(cos) = f (1 - s \/ 2), quad f^(sin) = f (1 + s \/ 2) $ <split-eq>

with the pair's mode shapes evaluated at $theta - theta_a$ about a principal
tension axis @worland2010. The shipped splits are 0.4% and 0.3%; zero is an exact
compatibility mode, which matters because it is what an older saved configuration
decodes to.

The citation carries the phenomenon and not the pattern. Worland measures
degenerate pairs *rotated and separated* under non-uniform rim tension, and a lug
pattern is not a uniform perturbation: an $N$-lug drum splits the azimuthal orders
that are multiples of $N$ strongly and leaves the others nearly degenerate. One
scalar applied to every pair alike cannot produce that. What @split-eq reproduces
is that pairs split, not which ones do.

*Selection retains a fixed count, not a fixed bandwidth.* Candidates are ordered
by frequency and admitted until a slot budget is full, an $m > 0$ mode costing two
slots for its two orientations. Since $omega prop sqrt(T)$, rendered bandwidth is
a function of tier *and tuning* together --- @tier-table is the shipped tuning,
and @ceiling-table is the same budget two octaves down, where the ceiling falls
below the band the fit cares about.

#figure(
  table(
    columns: 4,
    align: (left, right, right, right),
    table.header([Tier], [Oscillators], [Top mode], [Real time]),
    [Draft], [48 + 4], [929 Hz], [---],
    [Standard --- _shipped_], [96 + 6], [1310 Hz], [1.66$times$],
    [High], [160 + 8], [1662 Hz], [1.43$times$],
  ),
  caption: [
    What each budget buys at the shipped tuning: batter plus resonant
    oscillators, the highest retained mode, and the worst-case real-time factor.
    Doubling the count buys 0.5 of an octave, which is @density-eq seen from the
    other side.
  ],
) <tier-table>

The resonant head costs four to eight oscillators rather than a second full bank,
and that reduction is *exact rather than approximate*. Nothing can excite an
$m > 0$ resonant mode: the strike reaches only batter modes, and the cavity --- the
sole path between the heads --- couples through the swept area, which is
identically zero for every $m > 0$. Their displacement, their contribution to the
tension law and their stored energy are therefore zero for all time, so dropping
them changes the output not approximately but not at all, and a regression test
asserts bit-identical renders rather than a tolerance. The 44 oscillators it
reclaims are what pay for the batter head's wider band. The filter is applied
*after* selection, not during it: skipping those candidates inside the loop would
free their slots and admit higher axisymmetric modes instead, which is a different
instrument rather than a cheaper one.

*That exactness is inherited rather than absolute.* It is a property of the
*lumped* compliance, not of a drum: one scalar pressure driven by swept volume has
no way to reach a mode that sweeps none. A real cylindrical cavity has transverse
modes, and those couple to $m > 0$ head modes with a coefficient that is not zero.
The reduction is bit-exact given the model, and the model's exactness here is
exactly as sound as the assumption in it most likely to be wrong ---
@exclusions states that assumption as the limitation it is.

== Coupling through the cavity

The air enclosed by the shell is treated as one lumped adiabatic spring. Its
coupling coefficient is the signed net area a mode sweeps,

$ A_(0 n) = 2 pi R^2 J_1 (alpha_(0 n)) \/ alpha_(0 n), quad A_(m n) = 0 " for " m > 0 $ <swept-eq>

and the coupled system is a diagonal modal bank plus one scalar state:

$ dot.double(q)_i + 2 gamma_i dot(q)_i + omega_i^2 q_i = f_i \/ M_i - A_i p \/ M_i $ <modal-eq>
$ dot(p) + lambda p = K sum_i A_i dot(q)_i, quad K = s dot rho c^2 \/ V $ <cavity-eq>

with $V = pi R^2 L$ the shell volume and $lambda$ a pressure loss. The system is
passive by construction: with

$ E = sum_i M_i / 2 (dot(q)_i^2 + omega_i^2 q_i^2) + p^2 / (2 K) $ <energy-eq>

one has $dot(E) = -sum_i 2 gamma_i M_i dot(q)_i^2 - lambda p^2 \/ K <= 0$, so the
air can exchange energy between the heads but never manufacture it.

*The stiffness scale $s$ is fitted, and the fact that it has to be is the
interesting part.* The rigid formula $rho c^2 \/ V$ assumes a sealed, rigid shell
driven by two pistons. A real shell flexes, the vent leaks and the heads are not
pistons, and the formula correspondingly over-predicts the stiffening badly: at
$s = 1$ the axisymmetric fundamental splits into branches 1.91 apart, where
measured two-headed drums separate their two $(0,1)$ branches by 10--20%
@richardson2012 --- Fischer measured 186 Hz with one head and 215 Hz once the
resonant head was added at unchanged tuning, a ratio of 1.16 @fischer2014. The
shipped $s = 0.083$ gives 1.18. @cavity-figure is the whole effect.

#place(top, scope: "parent", float: true)[
  #figure(
    image("figures/cavity.png", width: 100%),
    caption: [
      The continuous-time radiated response at three cavity stiffnesses. The
      doublet opens with $s$ while its lower member stays penned between the two
      heads' own fundamentals, which is eigenvalue interlacing and is asserted by
      test. Above the axisymmetric family the three curves coincide exactly ---
      @swept-eq, seen rather than argued.
    ],
  ) <cavity-figure>
]

$s$ is a fraction rather than a free gain because the rigid, sealed,
piston-driven enclosure is the stiffest case that exists: 1 is a physical
ceiling, not a neutral setting, and a fitted value above it would be a statement
that the model is wrong rather than that the drum is stiff.

== The tension nonlinearity

Striking a drum harder stretches its head, which raises its tension, which raises
every mode frequency together --- and the pitch falls back as the hit decays. That
downward glide is the characteristic feature of a tom, and it is modelled by a
Berger-style reduced tension law over the modal strain measure

$ S = integral abs(nabla w)^2 dif A = sum_i Gamma_i q_i^2, quad Gamma_i = M_i k_i^2 \/ sigma $ <strain-eq>

$
  Delta T (S) &= T_max tanh(beta S \/ T_max) \
  U(S) &= T_max^2 / (2 beta) log cosh(beta S \/ T_max)
$ <berger-eq>

Near rest this is the ordinary Berger law $Delta T = beta S$ with
$U = beta S^2 \/ 4$ --- the reduction family in which the short-time tension
variation is proportional to the system's energy @marogna2010. The $tanh$ is a
smooth cap rather than a clip, so it bounds
the frequency excursion without discarding stored energy. Each mode is detuned by
$Delta omega_i^2 = Delta T k_i^2 \/ sigma$.

The cap earns its keep at the anti-alias bound. Frequencies scale as
$sqrt(1 + r)$ where $r = T_max \/ T_0$, so retaining modes up to a fraction
$nu$ of the sample rate requires

$ r < 1 \/ (4 nu^2) - 1 $ <nyquist-eq>

which at the shipped $nu = 0.45$ is 0.2346 against a shipped $r$ of 0.2 --- a 17%
margin, validated at configuration decode rather than assumed. At the shipped
coefficients the glide is 102.8 cents on the loudest hit and 3.0 on the quietest,
an audible semitone that still leaves the plateau clear: past roughly twice these
coefficients a loud hit sits on the flat of the $tanh$, which turns the glide into
a hold-then-drop and erodes the velocity dependence that makes it expressive.

Because the tension depends on the state at the end of the step and the state
depends on the tension, the update is implicit. It is closed with a *discrete
gradient* rather than by evaluating @berger-eq at either endpoint:

$ overline(Delta T) = 2 [U(S^(n+1)) - U(S^n)] \/ (S^(n+1) - S^n) $ <gradient-eq>

Paired with the midpoint displacement this makes the nonlinear work over a step
exactly equal the change in stored potential, so the lossless model conserves
@energy-eq to solver tolerance rather than drifting --- which is what lets an
energy assertion be a test rather than a hope.

== The loss law

Each mode's decay rate is the sum of three separately meaningful channels plus an
optional measured residual:

$ gamma_i = underbrace(d_0 + d_1 k_i + d_2 k_i^2, "structural") + d_"rad" alpha_i^2 + Delta_i $ <loss-eq>

from which $T_60 = ln 1000 \/ gamma_i$. They are kept apart rather than collapsed so that calibration can attribute a
change to a cause, and the $k^1$ term is the one that carries the physics. With
$omega approx c k$ on a membrane, the fraction of critical damping is

$ zeta_i = gamma_i / omega_i approx d_0 / (c k) + d_1 / c + d_2 k / c $ <zeta-eq>

so $d_1$ alone expresses *constant $Q$*: $d_0$ alone gives a $T_60$ independent of
frequency and $d_2$ alone gives $T_60 prop 1 \/ f^2$, where the measured membrane
behaviour is $T_60 prop 1 \/ f$ @fletcher1998. The shipped coefficients set
$zeta = d_1 \/ c = 0.72%$, and across the whole retained band the realised $zeta$
stays between 0.73% and 0.80% (@loss-figure). $d_0$ and $d_2$ are deliberately
small: each flattens or steepens the law at one end, and both are corrections to
a shape the $k^1$ term already has right.

#place(top, scope: "parent", float: true)[
  #figure(
    image("figures/loss.png", width: 100%),
    caption: [
      Left: the loss channels of the batter head, with the structural law
      continued past the highest resolved mode and the attack layer's three
      bands sitting on that continuation. Right: the same law as the quantity it
      is shaped to hold constant. The $(0,1)$ is the one mode that is
      deliberately off the law --- see the text --- and everything else lies
      within a few hundredths of a per cent of $d_1 \/ c$, including three
      bands that are not modes at all.
    ],
  ) <loss-figure>
]

*One mode is deliberately exempt.* A two-headed drum loses its axisymmetric
fundamental fastest of all, into the cavity and the opposite head, and the
lumped-cavity reduction does not reproduce the full magnitude of that path. The
residual is carried explicitly, as a per-mode correction $Delta_i$ on the $(0,1)$
of each head, fitted to hold its $T_60$ at 0.213 s. It shows in @zeta-eq as a
$zeta$ of 3.44% against the band's 0.74%, and in @modes-figure as the one marker
far below the line. Naming it as a correction rather than folding it into $d_0$ is
what keeps the other two coefficients readable.

#place(top, scope: "parent", float: true)[
  #figure(
    image("figures/modes.png", width: 100%),
    caption: [
      The whole instrument's mode map at the shipped tuning: 96 batter and 6
      resonant oscillators, against the constant-$Q$ law implied by $d_1 \/ c$
      alone --- a line derived from two coefficients, not fitted to the markers
      it passes through. This is the model-side counterpart of @decay-figure and
      is drawn on the same axes.
    ],
  ) <modes-figure>
]

Two consequences reach the rest of the paper. First, tuning must not be a sustain
control: because $c = sqrt(T \/ sigma)$ appears in @zeta-eq, holding $d_1$ fixed
while the tuning knob moves $T$ would drag $zeta$ from 2.2% to 0.7% across the
knob's travel and stretch a 300 Hz partial's $T_60$ from 0.166 s to 0.423 s.
Retuning therefore rescales $d_1$, $d_2$ and every correction by
$sqrt(T_"new" \/ T_"old")$, so the knob moves pitch and $T_60$ falls only in the
proportion constant $Q$ requires.

Second, and this is what @lossscale turns on: the product's damping control scales
this whole law, and a decay tilt slopes it. A residual that is neither --- a curve
that falls and rises again, which is what the reference's non-monotone ring times
demand --- is a statement about the *form* of @loss-eq, and the place an answer
would go is the per-mode correction term that already exists for exactly one mode.

== The strike

Excitation is selectable, and the two options differ in what they take as given.
The shipped `prescribed` model writes a half-sine of a measured contact duration
into the force buffer at trigger time, impulse-normalised in closed form, with the
duration running 8 ms at the quietest hit to 5.5 ms at the loudest and scaled by
stick hardness. The head never influences it.

The `hertzian` model integrates the stick as a free mass against a
Hunt--Crossley contact @hunt1975 @avanzini2001,

$ F = K [delta]_+^alpha (1 + h dot(delta)), quad delta = z - w $ <contact-eq>

where the elastic force is scaled by a factor linear in compression *rate*, so the
loss vanishes with the compression instead of stepping at impact and at
separation the way a linear dashpot does. Duration, pulse shape and re-contact
become outputs. The exponent is *measured rather than assumed*: Hertzian contact
time scales as $v^(-(alpha-1)\/(alpha+1))$, and Wagner's crescendo runs 7.5 ms at
piano to 5.9 ms at forte over a three- to fourfold velocity range @wagner2006,
which implies $alpha$ between 1.42 and 1.56. The canonical spherical $3\/2$ is
therefore what the velocity dependence says, not a convenience --- which also
means the prescribed model's velocity law is reproduced by this change rather than
discarded by it.

*The contact time is set by the head, not by the tip.* The driving-point mass of
the batter head, $1 \/ sum_i a_i^2 M_i$, is 0.31 g against a 15 g mallet, and the
same stick that rebounds off a rigid target in 0.40 ms stays in contact with the
head for 7.26 ms. Two decades of contact stiffness span a factor of 1.5 in
duration and then stop mattering, because the stick is riding the head's own
return rather than rebounding off its own compression. It is the same fact as a
stick being pressable into a roll.

*What the principled model bought is measured, and it is not the low band.* A
prescribed half-sine of duration $tau$ has magnitude spectrum
$abs(cos(pi f tau)) \/ abs(1 - (2 f tau)^2)$, which is a comb of *exact zeros* at
every $(k + 1\/2) \/ tau$ rather than a tilt: at the fitted $tau$ of 8.23 ms, 547
and 668 Hz sit at $-309$ and $-315$ dB, which is why nothing downstream can lift
them. Hertzian contact shallows that comb and moves it but does not remove it,
because it is still one smooth touch --- measured, it is worth 0--4 dB below 700
Hz and 12--23 dB above 800 Hz. Neither model reproduces what the measured
interval actually contains: Wagner's 5.5--8 ms is a *dwell* time spanning three
separate impacts --- the strike, the wave it launched returning off the rim about
1.7 ms later, and the re-loaded contact @wagner2006 --- and both models spend that
interval on a single force pulse. The Hertzian version does not produce those
re-contacts either; a build that appeared to was a discretisation artifact and it
converged away.

Whichever model is selected, the force is injected through the mode shapes times a
finite footprint factor $2 J_1 (k a_c) \/ (k a_c)$, and the strike-point state is
read back through the *same* weights. That reciprocity is not decoration: it makes
$F dot v$ exactly the power the modes receive, so contact can neither create nor
destroy energy.

== What the microphone hears

The observable is *volume acceleration*. Far-field pressure from a compact source
is proportional to it with no further frequency dependence, so the weight applied
to each mode's discrete acceleration is a pure geometric factor. For a circular
mode that factor is a Lommel integral, and because $J_m (alpha_(m n)) = 0$ by
construction it collapses with no series left over:

$ G_(m n) = 2 pi R^2 alpha_(m n) J_(m+1)(alpha_(m n)) J_m (u) / (alpha_(m n)^2 - u^2) $ <lommel-eq>

with $u = omega_(m n) R sin theta \/ c_"air"$ the acoustic trace wavenumber across
the head. Two limits carry the physics. At $u = 0$ this is *exactly* the swept
area of @swept-eq for $m = 0$ and exactly zero for $m > 0$ --- an on-axis
microphone hears the axisymmetric series and nothing else. And
$J_m (u) tilde.op (u\/2)^m \/ m!$ for small $u$, so multipole cancellation falls
out of the integral rather than being approximated by a fitted roll-off exponent.

That distinction is a near-miss worth recording. An earlier version of this model
summed modal *velocity* weighted by a radiation efficiency
$(k a \/ sqrt(1 + (k a)^2))^(m+1)$. As an amplitude ratio against velocity, that
efficiency already contains one factor of $k a$, so reusing it beside a
volume-acceleration observable counts that factor twice; and raising it to $m+1$
stands in for a cancellation whose true magnitude is $1 \/ (2^m m!)$, which at the
highest retained azimuthal order is wrong by seven orders. A fitted output gain
would have absorbed the level error and hidden both. The efficiency survives, but
only where it belongs --- apportioning radiation damping across the series in
@loss-eq.

The total weight is a *sum* of two mechanisms, not a product:

$ W_i = (G_(m n) D_m) / (1 + d\/R) + s_"nf" dot 2 pi R^2 e^(-alpha_(m n) d \/ R) Phi_i $ <weight-eq>

Here $D_m$ is the azimuthal directivity at the microphone's angle, $d$ its
distance and $Phi_i$ the mode shape beneath it. The second term is the
non-propagating near field, and a close-miked tom is mostly made of it.
@radiation-figure is why it has to exist.

#place(top, scope: "parent", float: true)[
  #figure(
    image("figures/radiation.png", width: 100%),
    caption: [
      Partial balance by azimuthal order, with and without the near-field term,
      each normalised to its own $(0,1)$. In the far field a 12-inch head below
      its modal ceiling is very nearly a monopole: the $(1,1)$ pair sits 18 and
      21 dB down and the higher orders cascade into the numerical floor. At 30
      mm the evanescent term restores them --- $(1,1)$ at $-6.8$ and $-10.1$ dB,
      $(0,2)$ at $-8.7$ --- which is what a tom microphone actually hears.
    ],
  ) <radiation-figure>
]

$s_"nf"$ is fitted, not derived: the effective area of an evanescent patch is
outside what a reduced model can compute. It matters more than any other single
number here, and it is also why the fit of @results "shifting the pickup" is a
substantive move rather than a cosmetic one --- microphone radius and angle enter
both terms of @weight-eq, so they trade the entire partial balance, not a level.
A Butterworth high-pass at 35 Hz and low-pass at 12 kHz and a scalar gain complete
the chain.

== The attack layer

Modal synthesis cannot reach a drum's real bandwidth in a browser, and the reason
is a counting argument rather than an engineering one. A membrane's mode count
below a frequency grows as its square,

$ N(f) approx (R k)^2 \/ 4, quad k = 2 pi f \/ c $ <density-eq>

so this head needs about 130 oscillators to resolve 1 kHz, 530 for 2 kHz and 3300
for 5 kHz, against a budget of 96 (@bandwidth-figure). Equivalently: bandwidth
grows only as the square root of the count, so doubling the oscillators buys about
half an octave --- which is @tier-table measured rather than derived.

#place(top, scope: "parent", float: true)[
  #figure(
    image("figures/bandwidth.png", width: 100%),
    caption: [
      Why the instrument is a hybrid. The staircase is what the budget resolves,
      the dashed curve is @density-eq --- what would have to be resolved to keep
      going --- and the shaded spans are what the stochastic layer covers
      instead. The two agree closely up to the ceiling, which is the check that
      the count is being spent on the modes a membrane actually has.
    ],
  ) <bandwidth-figure>
]

What lives above the ceiling is not individually resolvable anyway: it is a dense,
fast-decaying thicket that reads as the stick rather than as pitch, which is
precisely why the published tom analysis finds five to ten modes sufficient for
the *sustain* of a central strike @kirby2020. It is therefore modelled as
band-limited noise driven by the same contact force that drives the modes ---
which means hardness and velocity carry into it for free, with no second set of
parameters to keep consistent.

Three bands, not one, at 0.4, 1 and 2.5 times a placement frequency. The reason is
@loss-eq: $gamma$ grows with $k$, so the top of the covered range dies several
times faster than the bottom, and a single release makes the whole span ring for
as long as its slowest part --- heard as a noise burst sitting on the drum rather
than as the drum's attack. *The releases are derived, not fitted.* Each band decays
at the batter head's own structural law evaluated at that band's centre
wavenumber,

$ tau_b = "scale" / gamma_"struct" (2 pi f_b \/ c) $ <band-eq>

so the layer is an extrapolation of the mode series rather than an independent
effect beside it, and the damping controls reach it without being routed there.
At the shipped placement the three bands land at 1.6, 4 and 10 kHz with $T_60$ of
94, 37 and 15 ms --- against the flat 138 ms of the single fitted release this
replaced, which was about twice too long at the bottom of its range and seven
times too long at the top. The right panel of @loss-figure is that claim as a
picture: the three bands sit on the same constant-$Q$ line the modes do.

The noise source is a fixed-seed xorshift64\*, chosen because it is a few
instructions, has one word of state, and is exactly reproducible across
platforms --- which the language's global source is not, and which the bit-exact
render tests require.

== Integration and cost

There are two update paths, and which one runs is decided by the configuration
rather than by a quality setting. With the cavity and the nonlinearity both off,
the system is a bank of independent linear oscillators and each is advanced by the
*exact* matrix exponential of its own second-order system --- closed form in three
branches by damping regime, with no discretisation error at all.

Otherwise the coupled path runs the implicit midpoint rule. Writing
$q^(n+1) = q^n + h v_"mid"$ and $v^(n+1) = 2 v_"mid" - v^n$ and substituting into
@modal-eq gives, per mode,

$
  v_"mid" (2\/h + 2 gamma_i + omega_"eff"^2 h \/ 2) \
  = 2 v_i^n \/ h - omega_"eff"^2 q_i^n + f_i \/ M_i
$ <midpoint-eq>

where $omega_"eff"$ carries mode $i$'s current tension increase. The cavity would couple
every mode to every other, but it does so through a rank-one term, so it is
eliminated in closed form rather than solved: each mode's midpoint velocity
depends on the pressure affinely, and substituting that into the discretised
@cavity-eq leaves one scalar equation. Two passes over the modes and one division
replace what would otherwise be a dense solve.

The nonlinearity closes the loop with a fixed-point iteration on @gradient-eq,
capped at eight passes. The cap is not the cost: measured on the shipped
configuration at full velocity the mean is 2.88 iterations, and sweeping the
tension coefficient over a 32$times$ range moves it only to 3.09, because a
stiffer law both perturbs the tension more and contracts faster once the $tanh$
begins to saturate. That measurement retired a planned change --- an explicit
energy-proportional detune, proposed on the assumption that all eight passes ran
--- and it is the reason the exact energy bookkeeping was kept.

== Derived, measured, and fitted

The distinction the rest of this paper makes between a search question, a range
question and a model-structure question needs a matching distinction inside the
model. @provenance-table is it.

#place(top, scope: "parent", float: true)[
  #figure(
    table(
      columns: 3,
      align: (left, left, left),
      table.header([Quantity], [Provenance], [What fixes it]),
      [Mode frequencies], [derived], [@dispersion-eq from $T$, $sigma$, $R$],
      [Modal masses, strike weights], [derived], [orthogonality of @shape-eq],
      [Swept areas], [derived], [@swept-eq; exactly zero for $m > 0$],
      [Far-field weights], [derived], [@lommel-eq, a Lommel integral in closed form],
      [Attack-band releases], [derived], [@band-eq, the head's own @loss-eq],
      [Contact exponent $alpha = 3\/2$], [measured], [velocity dependence of contact time @wagner2006],
      [Contact duration], [measured], [5.5--8 ms on a 12-inch tom @dahl1997],
      [Damping ratio $zeta$], [measured], [constant $Q$ on membranes @fletcher1998],
      [Cavity split ratio], [measured], [10--20% on two-headed drums @richardson2012 @fischer2014],
      [Cavity stiffness scale $s$], [*fitted*], [to that measured split],
      [$(0,1)$ decay correction $Delta_i$], [*fitted*], [to a 0.213 s ring time],
      [Near-field scale $s_"nf"$], [*fitted*], [to partial balance at the shipped mic],
      [Attack level], [*fitted*], [to spectral balance against the modal layer],
      [Output gain], [*fitted*], [so a full-velocity hit peaks below clipping],
    ),
    caption: [
      Where each number comes from. The five fitted rows are the model's
      admissions: each is a quantity a reduced model cannot compute, and each is
      fitted against a *different* measurement rather than against the same one.
    ],
  ) <provenance-table>
]

The shipped values themselves are collected in @heads-table, @scalars-table and
@observation-table. All of them are SI-valued and versioned: the persisted schema is
at version 10, and every earlier version has an explicit migration. Those
migrations are not uniformly faithful, and the difference is recorded rather than
smoothed --- a zero-valued asymmetry or an absent nonlinearity reproduces the old
sound exactly, whereas the correction to the microphone model cannot, because what
it replaced was not a physical quantity and has no image under the new one.

#place(top, scope: "parent", float: true)[
  #figure(
    table(
      columns: 5,
      align: (left, left, right, right, left),
      table.header([Symbol], [Quantity], [Batter], [Resonant], [Unit]),
      [$R$], [radius], [0.1524], [0.1524], [m],
      [$sigma$], [surface density], [0.35], [0.25], [kg/m#super[2]],
      [$T$], [tension], [1250], [1040], [N/m],
      [$D$], [bending stiffness], [0.001], [0.0007], [N#sym.dot.c m],
      [$c$], [wave speed $sqrt(T\/sigma)$], [59.76], [64.50], [m/s],
      [$f_(0 1)$], [fundamental], [150.10], [161.99], [Hz],
      [$d_0$], [loss floor], [0.8], [1.0], [1/s],
      [$d_1$], [constant-$Q$ loss], [0.4303], [0.4644], [m/s],
      [$d_2$], [excess HF loss], [1.9e-5], [1.9e-5], [m#super[2]/s],
      [$d_"rad"$], [radiation loss], [1.5], [1.5], [1/s],
      [$Delta_(0 1)$], [$(0,1)$ correction], [24.6], [26.4], [1/s],
      [$s$], [asymmetry split], [0.004], [0.003], [---],
      [$beta$], [tension coefficient], [9.6e6], [6.4e6], [N/m#super[3]],
      [], [oscillators, Standard], [96], [6], [---],
    ),
    caption: [
      The two membranes at the shipped default. The resonant head is thinner and
      slacker, and carries only its axisymmetric modes --- exactly, not
      approximately.
    ],
  ) <heads-table>
]

#figure(
  table(
    columns: 3,
    align: (left, right, left),
    table.header([Quantity], [Value], [Unit]),
    table.cell(colspan: 3)[_Cavity_],
    [depth $L$], [0.20], [m],
    [volume $V$], [1.459e-2], [m#super[3]],
    [rigid $rho c^2 \/ V$], [9.707e6], [Pa/m#super[3]],
    [stiffness scale $s$], [0.083], [---],
    [pressure loss $lambda$], [5], [1/s],
    [$A_(0 1)$], [3.150e-2], [m#super[2]],
    table.cell(colspan: 3)[_Nonlinearity_],
    [tension ratio $r$], [0.2], [---],
    [anti-alias bound], [0.2346], [---],
  ),
  caption: [The air spring and the tension cap at the shipped default.],
) <scalars-table>

#figure(
  table(
    columns: 3,
    align: (left, right, left),
    table.header([Quantity], [Value], [Unit]),
    table.cell(colspan: 3)[_Strike and contact_],
    [radius, angle], [0.30, 0.2], [---, rad],
    [footprint $a_c$], [0.01], [m],
    [mallet mass], [0.015], [kg],
    [contact stiffness $K$], [1e6], [N/m#super[1.5]],
    [exponent $alpha$], [1.5], [---],
    [hysteresis $h$], [0.3], [s/m],
    table.cell(colspan: 3)[_Microphone_],
    [radius, angle], [0.65, 0.6], [---, rad],
    [distance $d$], [0.03], [m],
    [near-field scale $s_"nf"$], [1], [---],
    [high-pass, low-pass], [35, 12000], [Hz],
    table.cell(colspan: 3)[_Attack layer_],
    [level], [0.05], [---],
    [placement], [4000], [Hz],
    [band $Q$], [0.7], [---],
  ),
  caption: [Excitation, observation and the stochastic layer.],
) <observation-table>

== What the model is not <exclusions>

Stating the exclusions is part of describing the instrument, because several of
them are things a reader would otherwise assume are there.

*The resonant head does not radiate into the output.* It is fully coupled into the
dynamics through @cavity-eq and it is audible --- as the cavity branch, and as the
energy it takes out of the batter head --- but its own outward radiation leaves
the far side of the shell and is kept as a separate diagnostic. Summing it at the
batter microphone's point, phase, distance and polarity would require a
propagation and diffraction model this reduction does not have, and adding it
without one would be arithmetic rather than acoustics.

*The nonlinearity is mean-field, so it contributes pitch and nothing else.*
@berger-eq collapses the geometric nonlinearity to a single scalar $Delta T(S)$
driven by the total strain, so every mode of a head is detuned by the same
relative amount and *no mode can transfer energy to any other* --- the defining
property of the Berger and Kirchhoff--Carrier family @marogna2010. Stated
sharply: the nonlinearity here produces zero spectral content, and the only
mechanisms in the model that can deposit energy at a frequency are the contact
force's own spectrum and the stochastic attack layer. A real head struck hard
does not behave that way --- von Kármán coupling generates content at $2 f_i$, at
$f_i plus.minus f_j$ and through internal resonances, and that cascade is part of
why a hard hit is *brighter* and not merely sharper, which is measured in
@dahl1997, a source cited here already for contact time and whose brightening
this model attributes entirely to excitation and to the attack layer's velocity
scaling. The exclusion is a choice rather than a necessity: nonlinear modal
synthesis with the coupling terms retained now runs in real time @diaz2026,
though full von Kármán coupling is $O(N^3)$ in the mode count, so at 96
oscillators it would arrive needing a truncation of its own. It also bears
directly on the question @levels leaves open. A nonlinear source term is a
mechanism that deposits energy in a band the excitation comb has zeroed, because
it does not read $abs(F(f))$ in that band at all.

*There is no shell, no bearing edge, no vent and no hardware.* These are real and
audible on real drums; they are excluded because none of them can be calibrated
against anything currently available, and a free parameter with no measurement
behind it is indistinguishable from a fudge factor. There is no room and no snare.
The cavity is lumped, so it has no transverse modes of its own --- which is a
substantive limitation, and one the reference of @reference may be evidence
about. It is also the assumption that makes the resonant head's reduction to its
axisymmetric modes exact: with transverse modes present, the $m > 0$ resonant
oscillators would be driven, and dropping them would be an approximation with an
error to bound rather than a bit-exact identity.

*And the reference set is synthetic.* The committed fixture that pins this model's
behaviour is generated from the model itself, deterministically. It is a
regression reference, not an acoustic validation reference, and no measurement in
this chapter should be read as agreement with a real drum. Where a number here
does rest on a measured instrument --- the contact time, the damping ratio, the
cavity split --- it is cited to the literature it came from, and @provenance-table
is where that is visible at a glance.

= The target

The target is one recorded tom hit. Its provenance is a sample library, with no
documented microphone, room or processing chain, and no licence permitting
redistribution. It is therefore a *timbre-match target*, not an acoustic
validation reference: no automated test depends on it, and it is not part of the
repository. Keeping that distinction sharp matters, because some of what the
reduction reveals is a property of the recording rather than of any drum.

= Reduction: from a signal to named quantities

Reference and candidate pass through the same extraction, so any asymmetry
between them is a property of the signals rather than of the measurement.

*Partials* come from prominence-based peak picking on a 64k transform of the
sustain window. Prominence rather than a bare local-maximum test, because ripples
on the fundamental's skirt satisfy the latter and are not modes. Each surviving
peak carries a level, a ring time from a least-squares fit to its log envelope,
and that fit's $R^2$ --- which later lets a partial's frequency count while its
decay does not, since a beating pair or a mode buried under its neighbour has a
slope but not a meaningful one.

*Spectral envelope* is a 24-band third-octave vector computed over each of four
windows spanning the hit --- attack, early, body, tail --- so it describes not
only the timbre but the way the timbre evolves. *Amplitude envelope*, *pitch
glide* and *attack balance*, the click-to-body ratio, complete the reduction.

Deliberately absent is any sample-aligned waveform comparison. Against a
recording of a different physical drum, waveform error measures the phase
relationship between two signals that were never meant to share one: it is large
for candidates that sound identical and small for candidates that do not. Its
proper use, regression between two renders of the same model, is kept elsewhere.

= The distance

== Units and weights

Each term is reported in the unit its own perception is measured in: partial
frequency in cents, because the ear hears pitch as a ratio and 3 Hz means
something different at 118 Hz than at 1180; partial decay as
$abs(ln(T_60^"ref" \/ T_60^"cand"))$, because ringing twice as long and half as
long are the same size of mistake; levels and envelopes in dB; glide in cents.

Weights are the reciprocal of each term's "clearly wrong" threshold --- 25 cents
of pitch, 3 dB of partial balance, a factor of 1.4 in ring time, 4 dB of spectral
shape, 3 dB of envelope, 40 cents of glide, 6 dB of attack balance --- so a
just-audible error anywhere contributes about equally to the sum. Both signals
are peak-normalised, making every term gain-invariant; that is forced rather than
chosen, since the reference is normalised and its loudness carries no
information.

== Matching

Partials are matched greedily by closeness in cents rather than in index order,
so one badly placed candidate cannot cascade a misidentification through the
series, and each candidate is claimed at most once, so it cannot explain two
reference modes. The tolerance widens with mode index, since real two-headed
drums scatter about $plus.minus 20%$ around the ideal Bessel series in both
directions @richardson2012.

== Coverage, in both directions <coverage>

Six terms are computed over matched pairs, and an error averaged over matched
pairs is zero when there are no pairs. Each is therefore blended against a fixed
penalty in proportion to the share of the reference left unmatched, so that a
partial which is missing costs what a partial which is present but wrong costs.

That share must be weighted, and the weighting is the substantive choice.
Weighting by *energy* concentrates it on whichever partial is loudest --- on this
reference, one partial carries 99.4% of the total --- so a candidate holding that
partial alone reports almost nothing missing. Weighting by *count* overcorrects:
the reference contains a genuine, isolated component 41 dB down that no
two-headed drum will produce, and failing to reproduce it must not cost what
failing to reproduce the fundamental costs. Each partial is therefore worth *how
far it stands above the detection floor, in dB* --- zero at the floor, growing
with prominence, monotone in loudness like energy but compressed enough that
several missing quiet partials cannot be rounded away.

A second share scores the mirror case. Because matching iterates the reference, a
candidate partial with no reference counterpart is invisible to every partial
term and reaches the sum only through the spectral envelope, so a candidate can
cover the reference completely while its second-loudest component is a mode the
reference does not have. Both shares therefore appear, under the same weighting
and at the same weight. @partials-figure is the picture they score.

#place(top, scope: "parent", float: true)[
  #figure(
    image("figures/partials.png", width: 100%),
    caption: [
      What the two coverage terms see. Grey links join matched pairs; shading marks
      the span between the lowest and highest reference partial, within which the
      spurious share is counted. The two reference partials without counterparts
      sit at $-41$ dB, close to the detection floor, and are correspondingly cheap
      to miss.
    ],
  ) <partials-figure>
]

The spurious share is counted *only* between the lowest and highest reference
partial. Outside that span the reference's own detection is unproven --- a
recording's noise floor hides modes a model legitimately has --- so a partial out
there is charged by the spectral envelope, on evidence, and not by coverage.
Without that bound the term would fit the recording's limitations rather than the
drum.

== The gate

The sum orders candidates; it does not decide them. Adoption is gated on three
terms individually --- partial frequency within 25 cents, partial decay within
0.25 in log ratio, spectral envelope within 4 dB --- because a lower sum can be
bought in terms nobody listens for.

That is not hypothetical. In one run the candidate with the *best* spectral
envelope of any measured had the *fewest* partials, three, having found that
raising the stochastic attack layer's level and dropping its corner frequency
into the band of interest satisfies a band-energy measure. Broadband noise can
satisfy a spectral envelope; it cannot produce resolvable partials. On the total
alone that run looked like the best match to the recording.

= Preparing the reference <reference>

A stereo recording of one hit is two views of the same event, and they are
generally not simultaneous. Summing them is then a comb filter: for a delay
$tau$,

$ H(f) = 2 abs(cos(pi f tau)) $ <comb-eq>

with nulls at $(2k+1)\/2tau$ and maxima at $k\/tau$ --- a fixed $plus.minus 6$ dB
spectral shape imposed by microphone geometry alone @blauert1997.

The reduction therefore measures the inter-channel lag by cross-correlation and
aligns before averaging. It fires only when the pair is genuinely one signal
twice --- correlation at least 0.5, and clearly better than summing --- and never
on short or uncorrelated input. The measured lag and correlation are reported on
the decoded reference, so the condition cannot pass unnoticed, and regression
tests pin both directions: a delayed pair is aligned, an uncorrelated pair is left
alone.

On this recording the second channel lags the first by 69 samples at 44.1 kHz,
1.56 ms, with correlation 0.942 at that lag against 0.361 at zero. @comb-eq then
predicts nulls at 320, 959 and 1598 Hz and maxima at 639 and 1278 Hz, which is
what the naive downmix shows against a single channel, with no fitted parameter
(@comb-figure).

#figure(
  image("figures/comb.png", width: 100%),
  caption: [
    The naive mono downmix relative to the right channel alone, against
    @comb-eq. Shaded: 476--700 Hz, where the comb's first maximum falls.
  ],
) <comb-figure>

The effect on the reduction is large, because the partial detector keeps a
bounded number of peaks and the comb decides which survive:

#figure(
  table(
    columns: 3,
    align: (left, right, right),
    table.header([Reference reduced as], [Partials], [In 476--700 Hz]),
    [mono, averaged naively], [16], [9],
    [right channel alone], [7], [2],
    [mono, aligned then averaged], [*5*], [*2*],
  ),
  caption: [
    One recording, three reductions. Aligning before summing brings the mono view
    into agreement with the single channels; the naive sum agrees with neither.
    Counts are as measured before the level estimator of @levels was corrected;
    the corrected right-channel reduction finds 14 partials, 7 of them in
    476--700 Hz. The agreement the table reports is between reductions measured
    the same way, which the correction moves together.
  ],
) <channel-table>

Aligning also brings the reference's ring times into agreement with membrane
physics. Constant $Q$ requires $T_60 prop 1\/f$ @fletcher1998; the naive downmix
gives a median of 1.13 s in the 476--700 Hz band where that law predicts 0.31 s, a
factor of 3.6 that invites explanation in terms of a reverberant room. The aligned
reduction gives 0.16, 0.19, 0.60 and 0.61 s, which needs no such explanation.

= The search

The distance is minimised with the Mayfly Optimization Algorithm
@zervoudakis2020, a build-time-only dependency imported by the fitting command
and nothing else --- the shipped binary is unchanged. The search space is exactly
the bank the product exposes, plus strike velocity, with the quality tier pinned:
mode count buys fidelity with CPU and is a product decision rather than a property
of this drum.

Runs are deterministic given the seed. Progress is checkpointed with the completed
restarts and the running best point, written atomically, and the checkpoint
carries a fingerprint of the baseline cost. A resume across any change to the
model or the measurement is refused rather than silently mixed --- which has
repeatedly caught a stale checkpoint that would otherwise have produced a run
whose early generations were scored by one metric and its later ones by another.

One property of the model is worth stating here, because it determines the tier a
comparison must run at. Mode selection retains a fixed *count* --- 48, 96 or 160
by quality tier --- while membrane mode frequencies scale as $sqrt(T)$. Rendered
bandwidth is therefore a function of tier *and tuning* together, and it falls as
the drum is tuned down. For a bank at 331 N/m, roughly a quarter of the shipped
batter tension:

#figure(
  table(
    columns: 4,
    align: (left, right, right, right),
    table.header([Tier], [Modes], [Top mode], [In 476--700 Hz]),
    [Draft], [48], [467 Hz], [0],
    [Standard --- _shipped default_], [96], [664 Hz], [45],
    [High], [160], [852 Hz], [58],
  ),
  caption: [Modal ceiling of a low-tuned bank, by quality tier.],
) <ceiling-table>

Two consequences follow: an instrument truncated by count loses its top octave
when it is tuned down, and any comparison between configurations must run at the
tier that ships. Bounding mode selection by frequency instead would make the
ceiling independent of tuning.

= What the fit establishes <results>

The run reported here fits against the right channel at the shipped Standard
tier with the prescribed contact model, under the corrected reduction of
@levels: eight restarts of 150 iterations over a population of 16, seed 1, four
of the eight restarts seeded from the pre-solve of @seeding. All eight completed,
59,056 evaluations, and everything below is read from the report it wrote.

The distance falls from 33.094 at the shipped default to 11.252. More usefully,
@terms-figure shows where it fell.

#place(top, scope: "parent", float: true)[
  #figure(
    image("figures/terms.png", width: 100%),
    caption: [
      Every term as a multiple of its own threshold; the three gated terms are
      highlighted. Five terms are brought below threshold, four remain between
      1.7$times$ and 3.1$times$, and the two coverage shares confirm the candidate
      is a drum rather than either degenerate extreme.
    ],
  ) <terms-figure>
]

Five of the nine terms are brought below threshold: amplitude envelope to
$0.42times$ (1.27 dB against 3), glide $0.44times$ (17.6 cents against 40),
attack balance $0.004times$ (0.03 dB against 6), and both coverage shares ---
unmatched $0.30times$, spurious $0.55times$. The coverage pair is the useful
confirmation: the candidate accounts for most of the reference's audible partial
content while inventing moderately, so the remaining terms are being computed
over a genuine correspondence rather than over one lucky pair. The reference
reduces to fourteen partials and the candidate to sixteen; eleven pair up, and
the three reference partials left over are what the unmatched share prices
(@partials-figure).

Four terms remain above threshold --- partial frequency $2.28times$
(56.9 cents against 25), partial decay $1.66times$ (0.58 in log ratio against
0.35, and against the tighter 0.25 its own gate applies), partial level
$2.53times$ (7.58 dB against 3), spectral envelope $3.07times$ (12.28 dB against
4) --- and they are not four independent problems. @decay-figure and
@bands-figure show one.

#place(top, scope: "parent", float: true)[
  #figure(
    image("figures/decay.png", width: 100%),
    caption: [
      Ring time against frequency, with the constant-$Q$ law anchored on the
      reference fundamental. The mismatch is a shape rather than an offset: the
      model's ring times bunch between 0.6 and 1.0 s across 370--700 Hz, where
      the reference scatters over a factor of four, and it rings about
      three-quarters as long as the reference at the fundamental.
    ],
  ) <decay-figure>
]

#place(top, scope: "parent", float: true)[
  #figure(
    image("figures/bands.png", width: 100%),
    caption: [
      Spectral envelope, window by window. The attack matches within a few dB
      over most of the band; the discrepancy grows through the early, body and
      tail windows and is concentrated above about 1 kHz, where the model runs
      10--25 dB hot.
    ],
  ) <bands-figure>
]

*The remaining error is a shape of the loss law.* Ring times do not sit uniformly
above or below the reference's: at the fundamental the candidate rings 1.13 s
against the reference's 1.49 s --- too short --- while between 290 and 390 Hz it
rings 1.4 to 2.2 times too long, and across 370--700 Hz it delivers a nearly flat
0.6--1.0 s where the reference ranges from 0.26 to 1.20 s. The same fact appears
in the spectral envelope as an excess in the later windows above about 1 kHz,
energy that should have decayed and has not, with the attack window matching
within a few dB. Decay, not spectral shaping, is what the gated terms are
measuring.

The reference's own decay times are strikingly non-monotone in frequency: its
loudest partial at 213 Hz dies in 0.146 s while its 118 Hz fundamental sustains
1.49 s. No single constant-$Q$ loss law produces that ordering, which is why a
fit that gets the mid band right must get the fundamental wrong, and the reverse.
Locating the residual as a shape of the loss law rather than as a level is
exactly the discrimination the per-term units were built for.

*And the fit reports a parameter against its bound.* Head damping is fitted to
0.276 on a range whose lower bound is 0.25 --- normalised position 0.036, against
the edge. Since that parameter scales all loss rates together, the fit at that
position is asking for *less* overall loss than the bank expresses there, and it
spent its remaining freedom elsewhere, tuning the batter head to 933 N/m and
shifting the pickup. Reporting the normalised position beside the engineering
value is what makes that visible, and it turns the residual into a question that
can be put to the model directly.

= Testing a bound without widening it <lossscale>

A parameter resting on a bound invites a range change, and a range change is not
free: presets store *normalised* positions, so widening a shipped spec silently
retunes every saved patch. The question can be asked without touching the
product. A build-time `-loss-scale` multiplier applies to every head loss rate on
top of the head-damping parameter, so a run at 0.25 searches a range whose floor
is four times lower while the shipped spec is exactly as it ships. It is not a
product knob and is not intended to become one: a bank fitted at a multiplier
other than 1 describes a drum the product cannot be set to. Both the report and
the checkpoint fingerprint record the multiplier, so such a run can neither be
mistaken for a normal one nor resumed into one.

Three fits at equal budget --- 4 restarts of 60 iterations over a population of
12, seed 1 --- identical but for the multiplier:

#figure(
  table(
    columns: 6,
    align: (left, right, right, right, right, right),
    table.header(
      [Scale],
      [Baseline],
      [Best],
      [P. decay],
      [Spec. env.],
      [Damping],
    ),
    [1.0 _(control)_], [32.585], [*14.917*], [0.966], [13.27 dB], [0.475 (0.231)],
    [0.5], [26.388], [*14.924*], [1.263], [13.01 dB], [0.866 (0.448)],
    [0.25], [26.325], [*14.076*], [1.295], [12.82 dB], [2.122 (0.771)],
  ),
  caption: [
    Equal-budget sweep of the loss multiplier; head damping is given as the
    fitted value with its normalised position in brackets. Effective damping ---
    the product of the fitted value and the multiplier --- is 0.475, 0.433 and
    0.531: the same drum, reached from three different ranges.
  ],
) <lossscale-table>

Four times the headroom moves the total by 0.8, less than the spread between
restarts at this budget, and the fitted normalised position travels 0.23 → 0.45 →
0.77 to compensate for the multiplier --- which is what a parameter does when the
model can already reach the value the reference implies. At this short budget the
parameter does not rest on its bound even in the control, so the sweep alone
speaks to the range and not to that particular pin. A full-budget run at the
lowest multiplier settles it:

#figure(
  table(
    columns: 5,
    align: (left, left, right, right, right),
    table.header(
      [Scale],
      [Restarts],
      [Best],
      [Damping],
      [Effective],
    ),
    [1], [stopped at 47%], [*11.630*], [0.276 (0.036)], [0.2764],
    [0.25], [all 8 complete], [*13.023*], [1.132 (0.545)], [0.2830],
  ),
  caption: [
    The same question at full budget, damping again with its normalised position
    in brackets. Given four times the range, the search chooses the same physical
    damping to within 2%.
  ],
) <lossscale-full-table>

Both tables are measured under the estimator @levels corrects, so the totals are
superseded; what the experiment establishes survives it, because the comparison is
between runs measured identically and the conclusion is drawn from the *agreement*
between them rather than from any one value.

*The bound is not binding, and the shipped range needs no change.* The parameter
comes off it as soon as the bound stops mattering, which is what a non-binding
constraint looks like from the inside; the pinned value was a property of that
basin rather than a limit of the bank.

What the experiment establishes positively is where the residual lives. Removing
loss makes the partial-decay term *worse* --- 0.966 to 1.295 across the sweep ---
while the spectral envelope sits flat near 13 dB and never approaches its 4 dB
gate, so the envelope's excess is not made of over-long modes and damping will
not remove it. Together with the non-monotone reference decay of @results, this
places the residual in the *shape* of the loss law: head damping scales the whole
law and the decay tilt slopes it, and a curve that falls and rises again is
neither a scaling nor a slope. A per-mode decay correction, already scaled by
both knobs, is where such an answer would go. That is a model-structure question,
not a range question and not a search question, and separating the three is what
the per-term units are for.

= Seeding a restart from the reference's partials <seeding>

Mode frequencies are analytic: they are read off tension, radius and cavity
without rendering a sample, at roughly a hundredth of the cost of one full
evaluation. That makes it cheap to ask a question the fit itself cannot afford
--- how close can the model's modes get to this recording's partials at all?

#figure(
  table(
    columns: 2,
    align: (left, right),
    table.header([Measurement], [Audibility-weighted frequency error]),
    [20,000 random banks, best], [11.5 cents],
    [the same, hill-climbed], [10.5 cents],
    [the fit of @results], [59.7 cents],
    [gate], [25 cents],
  ),
  caption: [
    What the analytic pre-solve can reach, against what the fit reached and what
    the gate asks for.
  ],
) <presolve-table>

The model can place its modes on this drum, so mode placement is available to be
used as a starting point. `-seeded-restarts N` does that: a pre-solve finds $N$
diverse frequency-optimal banks, and those $N$ restarts search a box around them
while the remainder search the whole cube. The seeded restarts come first and the
unseeded ones keep their RNG seeds, so they are bit-for-bit what they were
without the flag and the comparison is paired.

*A seed is evidence about some parameters and not others, and the box has to
respect that.* A frequency-only objective has no opinion about damping, the
microphone position or the attack layer, so narrowing a restart in those
directions spends range for nothing. The dimensions the seed speaks to are
therefore found by *probing* which ones move the mode frequencies, rather than by
listing them, so the mask stays true when the parameter table changes. Here it
selects 4 of 17 free parameters --- overall size, the two head tunings and the
head asymmetry, which are the wave speed, the two tensions and the mode split ---
and the pre-solve's two seeds land at 1.0 and 1.3 cents.

#figure(
  table(
    columns: 4,
    align: (left, right, right, right),
    table.header([Restart], [Control], [Seeded, all dims], [Seeded, masked]),
    [1 --- _seeded_], [19.442], [31.707], [*16.388*],
    [2 --- _seeded_], [16.754], [19.109], [*13.132*],
    [3], [14.917], [14.917], [14.917],
    [4], [16.638], [16.638], [16.638],
    [Best], [*14.917*], [*14.917*], [*13.056*],
  ),
  caption: [
    Paired comparison at equal budget. Restarts 3 and 4 are unseeded in all three
    runs and reproduce exactly, which is what makes the first two rows readable.
  ],
) <seeding-table>

With the box restricted to those four dimensions, both seeded restarts improve
and neither regresses: a 12% better result at the same cost. The partial-frequency
term falls from 94.4 to 46.1 cents, partial decay from 0.966 to 0.766, and the
unmatched coverage share from 0.461 to 0.011 --- the seeded drum produces the
reference's partial content rather than a fraction of it. The spurious share
rises, 0.413 to 0.638, which is the honest price of the method: a bank chosen to
have a mode near every reference partial also has modes elsewhere, and
@coverage's second share is what reports it.

The pre-solve aims at the reference's detected partials, so the correction of
@levels changes the target it is given and supersedes the values in
@presolve-table and @seeding-table. The mechanism, the two defects the first
version exposed, and the paired design that makes @seeding-table readable are
unaffected, and the pre-solve is cheap enough to re-run against any reduction.

= The conditioning of a level estimate <levels>

The reduction attaches a level to each partial, and coverage, matching and the
detection floor all depend on it. That estimate was obtained exactly, and probing
it showed the exact formula to be an unusable estimator. The finding is worth
reporting for its own sake: *conditioning is a property of a measurement
independent of its correctness*, and an apparatus built to be read term by term
is exactly the kind that can be probed to expose it.

The estimate was the partial's magnitude in the sustain transform divided by the
attenuation that window applies to a partial decaying at the fitted rate. For an
isolated exponential that is exact. But the sustain window spans 0.05--0.85 s,
and the divisor it implies grows without bound as the fitted ring time shortens:

#figure(
  table(
    columns: 8,
    align: (left, right, right, right, right, right, right, right),
    table.header(
      [$T_60$ (s)],
      [2.0],
      [1.5],
      [1.0],
      [0.6],
      [0.3],
      [0.15],
      [0.073],
    ),
    [Correction (dB)],
    [12.4],
    [16.1],
    [22.9],
    [34.2],
    [54.9],
    [82.3],
    [122.0],
  ),
  caption: [
    What the window correction asks of the measurement. Across the ring times
    this recording actually contains, the divisor spans more than 100 dB.
  ],
) <correction-table>

A quantity recovered by dividing through @correction-table is largely a
restatement of the fitted decay rate: at 0.12 s a 10% error in $T_60$ moves the
reported level by 4.5 dB. On this reference the consequence was visible. At a 5 Hz
peak separation a 73 ms component --- $T_60$ 0.073 s, $R^2$ 0.865 --- was reported
as the loudest partial in the recording, which put the fundamental --- $T_60$
1.49 s, $R^2$ 0.984 --- at $-33.6$ dB. Levels are relative to the strongest, so
genuine partials then fell below the detection floor and disappeared, and the
detected set was not monotone in that floor.

Capping the correction is the obvious remedy and the measurement rejects it: the
reference's genuinely loudest partial, at 213 Hz, itself carries an 83.7 dB
correction, so a cap tight enough to exclude the runaways excludes it too.

*The estimator is instead replaced by a directly measured one.* The per-partial
decay fit is a least-squares line through the log of the partial's heterodyned
envelope, and its *intercept* --- the fitted line's value at $t=0$ --- is the same
quantity the correction was reaching for. It extrapolates back only as far as the
start of the fit window rather than through the whole taper, and it rests on a fit
over hundreds of samples rather than on a ratio of two numbers. One constraint
carries over intact and is respected: the level must not be the envelope's *peak*
inside the fit window, because a strike transient smeared through a 150 ms filter
once placed the 213 Hz partial 32 dB too loud. The fit begins after the transient,
and the intercept is an extrapolation of the fitted decay rather than a reading of
the envelope.

*A second detection window follows from the same arithmetic.* A partial ringing a
tenth of the 800 ms sustain window stands roughly 90 dB lower in its transform,
and both detection guards --- a relative floor with 20 dB of headroom, and a count
limit --- rank on that uncorrected magnitude. They bind together, which is why
loosening either alone establishes nothing: sweeping the headroom from 20 to 80 dB
left the count at 7 throughout, and so did sweeping the count limit over a factor
of sixteen; only both at once moved it. Detection therefore reads a second,
earlier window spanning 0.05--0.30 s, and *each window admits candidates relative
to its own strongest peak*, so a short-ringing partial competes against
short-lived content rather than against the fundamental's entire ring. The sustain
window is admitted first, so where both see a partial the better-resolved
frequency is kept; the early window starts after the transient, since a broadband
click offers a peak at every frequency. The shipped guard constants are unchanged.

*The order of the two fixes is the transferable part.* The tight guards were, in
effect, what had been keeping the unstable estimator from firing: they excluded
precisely the short-ringing population where its correction ran away. Opening the
aperture first would have made the measurement worse. Bound the estimator, then
widen what it is asked to see.

A regression test pins both on a synthetic two-partial signal --- amplitude 1.0
ringing 1.2 s against amplitude 0.5 ringing 0.12 s --- so the behaviour is fixed
without reference to any recording. It failed twice while being written: first
because the levels came out inverted, then because the short partial was not
detected at all.

#figure(
  table(
    columns: 3,
    align: (left, right, right),
    table.header([Right channel, reduced], [Partials], [In 476--700 Hz]),
    [before the correction], [7], [2],
    [after], [*14*], [*7*],
  ),
  caption: [
    The corrected reduction of the reference. Half of what it now reports lies in
    the band the recording's character lives in.
  ],
) <corrected-table>

@corrected-table re-scopes a standing claim. The fits of @results reproduced
little in 476--700 Hz while the reference was busiest there, and the reference
still is --- but part of that gap was the target not asking for those partials
either, because they sat below the floor the level estimate set. Whether a real
excitation deficit remains in that band is an open question on the corrected
metric. Fits at full budget are in progress; the honest statement today is that
the measurement has been repaired and the question re-opened, not that it has been
answered in either direction.

= What to watch for

Eight points generalise beyond this instrument, each learned by measurement.

*An exact formula can be an unusable estimator; probe it before trusting it.*
Correctness and conditioning are different properties, and a derivation
establishes only the first. Sweep the inputs an estimator is expected to see and
look at what its correction factor does across that range: if the correction
spans 100 dB, the output is a restatement of whichever input drives it, however
exact the algebra. That is discoverable by measurement long before it is visible
by inspection.

*Loosen a guard only after the quantity it ranks on is sound.* Guards and
estimators interact: a tight threshold can be silently protecting a downstream
computation from the region where it misbehaves, so relaxing the threshold first
makes the measurement worse and blames the wrong component. Bound the estimator,
then open the aperture --- in that order.

*Measure inter-channel lag before summing.* A stereo pair of one event is one
signal twice, and averaging imposes a fixed comb on every spectral quantity
downstream. A low zero-lag correlation between two views of one close-miked hit
is evidence that the channels are offset in time, not that there is a room on it.

*Score coverage both ways, and weight the two sides equally.* Either direction
alone admits a degenerate optimum. Weighting the invented-partial side more
heavily than the missing-partial side inverts the degeneracy rather than closing
it --- discarding the drum becomes the cheapest way to invent nothing, and a run
under such a weighting duly converged on two partials. The pressure toward
completeness and the pressure toward tidiness are one quantity seen from two
sides; the asymmetry belongs in the blend, where it is already present, and not
in the weights.

*Read a pinned parameter as a question about the range, and answer it by
measurement.* A value sitting on a bound is among the most informative things an
optimiser produces, so report the normalised position alongside the engineering
value: the difference between "fitted to 0.276" and "fitted to the bound" should
be visible without opening the bank. Then settle it by giving the search room
outside the shipped box rather than by widening the box --- a build-time
multiplier costs nothing downstream, whereas a widened spec silently retunes
every preset that stores a normalised position. Here the answer was that the
bound never bound.

*Distance to a mode is not distance to a heard partial.* A pre-solve over
analytic mode frequencies scores the distance to the nearest *mode*; the gate
scores the distance to the nearest *detected partial*. A mode can sit exactly on
a reference partial and never be heard --- wrong radiation weight, in a pickup
null, or buried under a louder neighbour --- so a 1.0-cent seed does not promise
a 1.0-cent frequency term, and the 46.1 cents the seeded fit reached is the
measure of the gap. Mode placement is necessary for that term and nowhere near
sufficient.

*Check a cheap objective for discrimination before quoting its numbers.* Reading
both heads roughly doubles the mode count, and a dense enough bank is near *any*
frequency by accident. Measured over 2000 random banks, the audibility-weighted
frequency error has median 164.8 cents, tenth percentile 56.2 cents and best 4.1
cents, so a 1.0-cent seed is a real selection from that distribution rather than
an artefact of density. An objective that cannot separate a good bank from a
random one will seed noise just as confidently.

*Gate per term, and confirm by ear.* A distance can be monotone in quality ---
ranking candidates in the order a listener would --- while being wrong about all
of them in absolute terms. Ordering is much easier to get right than calibration,
and a converged search reports the former while appearing to report the latter.

= Scope and reproducibility

The model, the feature extraction, the distance and the search are in
#code-path("internal/physical"), #code-path("internal/physical/match") and
#code-path("cmd/fit-physical"); the loss multiplier of @lossscale and the
pre-solve of @seeding are flags of that command and are recorded in the report
and in the checkpoint fingerprint, so neither can be mistaken for a default run.
Runs are deterministic given the seed and are
excluded from continuous integration, since they take minutes and require a
recording the repository does not contain. That search is derivative-free, and it
need not have been: this model is nearly analytic in its parameters --- mode
frequencies and decay rates are closed form, which is what @seeding exploits ---
so gradient-based fitting with the modal parameters constrained to stay physical
during training is available @zheleznov2026 and is not used here. The channel-alignment behaviour, the
audibility weighting and the relationship between the two coverage weights are
pinned by tests that need no recording.

@partials-figure, @terms-figure, @decay-figure and @bands-figure are drawn by
#code-path("tools/paper-figures") --- `just paper-figures` --- from a single fit
report, which carries both feature sets in full, so they are reproducible from a
committed artefact rather than from the recording, which is not redistributable.
That command reproduces the distance's own greedy partial matching rather than
approximating it, so @partials-figure shows the pairing the score was actually
computed over. @comb-figure is measured from the two channels directly and is not
regenerable without them.

The five figures of @instrument come from a second committed artefact, written by
#code-path("cmd/analyze-physical") --- `just paper-data`, then the same
`just paper-figures`. Nothing in it is rendered: the modal bank is @dispersion-eq
and @loss-eq evaluated in closed form, and the cavity curves are the
continuous-time solve of @cavity-eq, so it depends on no audio, no recording and
no seed. It is regenerated deliberately, like the images, rather than diffed in
continuous integration --- which is the reverse of the reference fixture the
model's regression tests use, and the two should not be confused. That fixture is
*generated from the model*, so nothing in @instrument is evidence of agreement
with a real drum; the measured quantities it does rest on are cited individually
in @provenance-table.

#show bibliography: set text(size: 8.1pt)
#bibliography("references.bib", style: "ieee", title: "References")
