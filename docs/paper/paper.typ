#import "style.typ": code-path, correction, paper

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
    through a short modal expansion of the enclosed air, a Berger tension
    nonlinearity closed by a discrete gradient and extended by the mode-to-mode
    channels Berger projects away, a three-term loss law shaped to hold constant
    $Q$, a microphone model that is a Lommel integral plus an evanescent term,
    and a stochastic layer whose releases are read off that same loss law past
    the modes the budget can afford. Which of its numbers are derived, which are
    measured and which are fitted is reported alongside them.

    Applied to that tom, the apparatus takes the distance from 33.1 at
    the shipped default to 11.3, bringing five of the nine terms below their
    thresholds --- amplitude envelope, glide, attack balance and both coverage
    shares --- and it localises what remains. What remains is a *shape* the loss
    law cannot make: the model delivers a nearly flat 0.6--1.0 s ring across
    370--700 Hz where the reference scatters over a factor of four, and rings
    about three-quarters as long as the reference at its fundamental. The
    head-damping parameter, which scales all loss rates together, was fitted
    against the lower bound of its range in an earlier run. That is a specific,
    testable statement, and it is put to the test here: given four times the
    head-damping headroom through a build-time multiplier that leaves the shipped
    parameter range untouched, the search chooses the same physical damping to
    within 2%, so the residual is a question about the shape of the loss law
    rather than about its range. The same per-term reading separates two contact
    models fitted at identical budget --- 11.252 against 11.535, winning on
    different terms, by a margin @glidefault then puts back in question --- and a
    second experiment exploits the fact that mode
    frequencies are analytic: a pre-solve seeds restarts with banks whose modes
    already lie on the reference's partials, worth 12% at identical cost against
    a target it reaches to a cent, and worth nothing against one it reaches only
    to thirty-six --- a distinction the pre-solve itself reports before the search
    begins. A final chapter refits the prescribed model under the corrected glide
    estimator and reaches 10.382 from a 32.442 baseline; a second, independent
    search agrees with it on every term the objective can read, which places the
    residual at the model's ceiling on this recording rather than at the search's.
  ],
  status-body: [
    The fitted numbers in @results come from two complete reports, one per contact
    model, each a full-budget run of eight restarts under the corrected reduction
    of @levels; the figures and every number in that section are read from the
    better of the two. @lossscale and @seeding report earlier runs at their own
    stated budgets, each paired against a control measured at the same budget.

    Those earlier runs predate the correction to the partial-level estimator
    reported in @levels (2026-07-31), which changed the reference's partial list
    and every candidate's. Their totals and term values are therefore *not
    comparable to anything measured after that date*, and are reported here for
    the method they establish rather than as current results; @levels reports the
    correction and what it re-scopes. Under the corrected reduction the shipped
    bank measures 33.094 with prescribed contact and 33.544 with Hertzian, at the
    Standard tier against the right channel.

    *Every fit up to @glidefault, including those two, predates a second
    correction: the glide estimator was faulty in both of its probes*
    (@glidefault, 2026-07-31). That term sat inside the objective the search
    minimised, so the fitted banks are conditioned on it and not merely mis-scored
    by it. @results and @contact-table are reported as obtained rather than
    re-run, and @glidefault gives the fault, its size, the corrected reference
    figure and what specifically it puts back in question.

    @refit is the one run made *after* that correction (2026-08-01) and the only
    section whose total is current: the prescribed contact refitted from scratch,
    32.442 #sym.arrow 10.382. It also re-measures four archived banks under the
    corrected objective so that they can be read on one axis. The Hertzian
    contact has not been refitted, so nothing in this paper establishes the
    contact-model ordering.
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
coupled through a modal expansion of the enclosed air, with a Berger tension
nonlinearity and its mode-coupling extension, a microphone model, and a
three-band stochastic attack layer covering the transient region modal synthesis
reaches poorly. Its configuration is SI-valued and versioned; the product exposes
eighteen normalised parameters over it.

This chapter describes it in enough detail to argue with. That is not
scene-setting. Every conclusion the rest of the paper reaches is a statement
*about this model* --- that its head-damping bound is not binding, that the
residual lives in the shape of its loss law rather than in the range of any
parameter, that its mode frequencies are analytic and therefore cheap enough to
seed a search from --- and none of those can be checked, or disputed, from a
description that stops at the word "modal".

== Two membranes, an air cavity, and a microphone

The signal path is short and its asymmetries are deliberate. A strike deposits
force at one point on the *batter* head, spread over a finite contact footprint.
The batter head's modes are the only ones the stick reaches. The *resonant* head
is driven solely by the enclosed air, which is expanded in the rigid-walled
cylinder's own modes --- six pressure states at the shipped default, of which the
uniform one is the lumped spring compressed by the net volume the two heads
sweep. Each head's total strain raises its own tension, which detunes every one
of its modes together, and the part of that quartic potential the tension law
projects away is restored as a small set of mode-to-mode channels, so a loud hit
also deposits energy at frequencies nothing struck. A stochastic layer, driven by
the same contact force, supplies the band above the highest mode the budget can
afford. What the listener hears is a weighted sum of the batter head's modal
accelerations and that layer, band-limited and scaled.

Two properties of that path are worth naming immediately, because they are
easy to assume otherwise. Only the batter head radiates into the output: the
resonant head is fully coupled into the dynamics but its own radiation leaves the
far side of the shell, and adding it at the same point, phase and distance would
be a fiction. And the cavity reaches only the azimuthal orders its own basis
carries --- $m = 0$ alone when the air is one lumped state, because the uniform
pressure responds to swept volume and every other mode sweeps exactly none, and
$m <= 2$ at the shipped basis, where the transverse air modes have shapes to
overlap against.

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
    columns: 5,
    align: (left, right, right, right, right),
    table.header([Tier], [Batter], [Resonant], [Top mode], [Real time]),
    [Draft], [48], [20], [929 Hz], [---],
    [Standard --- _shipped_], [96], [24], [1310 Hz], [0.70$times$],
    [High], [160], [24], [1662 Hz], [---],
  ),
  caption: [
    What each budget buys at the shipped tuning: the two heads' oscillator
    counts, the highest mode the head that radiates retains, and the worst-case
    real-time factor on `js/wasm`. Doubling the batter count buys 0.5 of an
    octave, which is @density-eq seen from the other side. The resonant head no
    longer scales with the tier --- it has its own budget, sized against the
    cavity rather than against a quality setting --- so at Draft its own ceiling,
    1001 Hz, is the higher of the two. The one measured real-time figure is the
    Standard worst case with the coupling table at its shipped size; the same
    configuration measures 1.40$times$ with the coupling off and High
    1.15$times$, and @cost is where that is discussed rather than tabulated.
  ],
) <tier-table>

The resonant head costs a couple of dozen oscillators rather than a second full
bank, and the *selection* part of that reduction is exact rather than
approximate. Nothing can excite a resonant mode the air cannot reach: the strike
reaches only batter modes, and the cavity --- the sole path between the heads ---
couples through an overlap integral whose azimuthal factor is identically zero
unless the head mode's order matches a cavity mode's. Their displacement, their
contribution to the tension law and their stored energy are therefore zero for
all time, so dropping them changes the output not approximately but not at all,
and a regression test asserts bit-identical renders rather than a tolerance, at
both ends of the cavity basis. The filter is applied *after* selection, not
during it: skipping those candidates inside the loop would free their slots and
admit higher reachable modes instead, which is a different instrument rather than
a cheaper one.

*What the reduction retains changed when the cavity did, and the exactness did
not.* With one lumped pressure state the reachable set is $m = 0$ and the filter
is literally "axisymmetric only", leaving 6 of Standard's 96 slots. With the
shipped six-state cavity it is ${0, 1, 2}$ and the same selection leaves 28. The
statement is therefore "the orders the air can reach" rather than "axisymmetric",
and it is bit-exact at both ends for the same reason --- the selection rule
zeroes the coupling of every unreachable order, and those modes provably never
leave zero.

*The second budget is not exact, and is stated separately for that reason.* A
head that is only ever driven through the air has no business being sized by a
quality tier chosen for the head the stick hits, and sharing one number was
accidental --- harmless while the reduction left half a dozen modes, and not
harmless once a cavity setting started quadrupling the resonant bank. The
resonant head therefore has its own limit, 24, and that truncation from 28 *is* a
change to the instrument. It is a small one: broadband level moves by less than
0.001 dB and the small-signal transfer function by 0.019 dB RMS to 4 kHz. The
value is chosen against the mechanism rather than against a clock --- 24 is the
smallest bank whose modes straddle both transverse cavity resonances, with the
$(1,1)$ air mode at 660 Hz between resonant partials at 472 and 685 Hz and the
$(2,1)$ at 1094 Hz between 1001 and 1213 Hz, the 23rd and 24th oscillators.
Below a budget of 12 the first straddle is missing and the coupling feature of
@cavity-figure reads 3 dB wrong.

== Coupling through the cavity

The air enclosed by the shell is expanded in the rigid-walled cylinder's own
modes. A rigid wall carries no normal velocity, so the radial condition is
*Neumann* --- and not the Dirichlet condition the clamped heads obey, which is a
distinction worth stating because the two families look alike and are not:

$ Psi_(m n) (r, theta) = J_m (j'_(m n) r \/ a) dot cases(cos m theta, sin m theta), quad omega_(m n) = c j'_(m n) \/ a $ <cavitymode-eq>

where $j'_(m n)$ is the $n$-th zero of $J_m'$. The uniform mode is the $m = 0$
member at $j' = 0$: a mode of zero frequency, and the whole of the lumped spring
this replaced. Note that $j'_(0 1) = 3.8317$, the first *positive* zero of
$J_0' = -J_1$, is a different and non-degenerate mode.

*Axial order is excluded deliberately, not overlooked.* A pressure varying along
the shell would make the two heads see different pressures --- the coupling
coefficient picks up a factor that is $+1$ at the batter head and $(-1)^l$ at the
resonant one instead of the same at both, which is a larger change than this one
--- and the first axial mode sits at $c \/ 2L = 858$ Hz on the shipped 0.2 m
depth, above the transverse modes this exists to add. Axially uniform is a first
cut and is described as one.

#figure(
  table(
    columns: 4,
    align: (left, right, right, right),
    table.header([Cavity mode], [$j'$], [at 0.1524 m], [at 0.1584 m]),
    [$(0,0)$ uniform], [0], [0 Hz], [0 Hz],
    [$(1,1)$ cos/sin], [1.8412], [659.5 Hz], [634.5 Hz],
    [$(2,1)$ cos/sin], [3.0542], [1094.0 Hz], [1052.6 Hz],
    [$(0,1)$], [3.8317], [1372.5 Hz], [1320.5 Hz],
  ),
  caption: [
    The six shipped air states, at the shipped 12-inch radius and at the 0.1584 m
    the excitation-gap analysis states its hypothesis for. The second column is
    where that analysis compares the reference's 624.4, 1018.4 and 1331.3 Hz
    partials; a test pins the generated table to it. The series is
    $c j' \/ 2 pi a$ and nothing else, so it is set by the shell and the air and
    not by the tuning.
  ],
) <cavitymode-table>

Each head mode couples to each air mode through the overlap of their shapes over
the head's disc, and the same coefficient appears in both directions --- the air
is driven by $C_(i c) dot(q)_i$ and the head is loaded by $C_(i c) P_c$:

$ C_(i c) = integral_A phi_i Psi_c dif A $ <overlap-eq>

The angular integral separates and gives a *selection rule*: the coefficient
vanishes unless the azimuthal orders match, and at an unrotated principal tension
axis unless the orientations match too, a rotated axis entering as a plane
rotation through $m psi$ and therefore as an isometry that leaves the total
coupling strength alone. A head mode reaches at most one air mode per radial
order, which is what makes the extension affordable: 44 of the 768 head/air pairs
at the shipped basis are non-zero. Against the uniform mode $Psi = 1$ and
@overlap-eq collapses to the signed swept area,

$ A_(0 n) = 2 pi R^2 J_1 (alpha_(0 n)) \/ alpha_(0 n), quad A_(m n) = 0 " for " m > 0 $ <swept-eq>

which is the closed form the lumped model always used and which the code returns
verbatim in that case. For every other air mode the radial integral has no clean
closed form --- the two Bessel functions carry different arguments and different
boundary conditions, so the Lommel collapse does not happen --- and is evaluated
by 96-point Gauss--Legendre quadrature once per coupled pair at construction.
Nothing in the render touches it, and a test checks the quadrature against the
analytic swept area where the two must agree, to $2 times 10^(-16)$.

The coupled system is a diagonal modal bank plus a $(P_c, H_c)$ pair per air
mode:

$ dot.double(q)_i + 2 gamma_i dot(q)_i + omega_i^2 q_i = f_i \/ M_i - sum_c C_(i c) P_c \/ M_i $ <modal-eq>
$
  dot(P)_c = K_c sum_i C_(i c) dot(q)_i + omega_c H_c - lambda P_c, quad dot(H)_c = -omega_c P_c
$ <cavity-eq>
$ K_c = s rho c^2 \/ Lambda_c, quad Lambda_c = integral_V Psi_c^2 dif V $ <stiffness-eq>

with $Lambda_(0 0) = V = pi R^2 L$ the shell volume and $lambda$ a pressure loss.
Writing each mode in that first-order pair rather than as a
displacement/velocity one is what makes the uniform member degenerate cleanly:
$omega_(0 0) = 0$, so $H$ never leaves zero and @cavity-eq collapses to the single
$dot(p) + lambda p = K sum_i A_i dot(q)_i$ the lumped model had. The system is
passive by construction: with

$ E = sum_i M_i / 2 (dot(q)_i^2 + omega_i^2 q_i^2) + sum_c (P_c^2 + H_c^2) / (2 K_c) $ <energy-eq>

one has $dot(E) = -sum_i 2 gamma_i M_i dot(q)_i^2 - lambda sum_c P_c^2 \/ K_c <= 0$.
That is *proved rather than assumed*, and the proof is the reason @overlap-eq is
written with one symbol: the head equation loses $sum_c P_c sum_i C_(i c)
dot(q)_i$ and the cavity equation gains exactly that quantity, because the same
coefficient stands in the drive and in the load, so the coupling terms cancel
identically for any number of air modes and any $s$. The $omega_c$ rotation
cancels against itself for the same reason. The air can exchange energy between
the heads but never manufacture it.

*The stiffness scale $s$ is fitted, and the fact that it has to be is the
interesting part.* The rigid formula $rho c^2 \/ V$ assumes a sealed, rigid shell
driven by two pistons. A real shell flexes and the heads are not pistons, and the
formula correspondingly over-predicts the stiffening badly: at $s = 1$ the
axisymmetric fundamental splits into branches 1.87 apart, where measured
two-headed drums separate their two $(0,1)$ branches by 10--20% @richardson2012
--- Fischer measured 186 Hz with one head and 215 Hz once the resonant head was
added at unchanged tuning, a ratio of 1.16 @fischer2014. The shipped $s = 0.083$
gives 1.18. @cavity-figure is the whole effect.

#place(top, scope: "parent", float: true)[
  #figure(
    image("figures/cavity.png", width: 100%),
    caption: [
      The continuous-time radiated response at three cavity stiffnesses. The
      doublet opens with $s$ while its lower member stays penned between the two
      heads' own fundamentals, which is eigenvalue interlacing and is asserted by
      test. Above the axisymmetric family the curves no longer coincide, and that
      is the transverse air modes: at the $(1,1)$ of @cavitymode-table the
      shipped cavity moves the response 12.3 dB against no air spring at all,
      where the lumped cavity moved it 0.04 dB. @swept-eq is now the statement
      about *one* member of the expansion rather than about the cavity.
    ],
  ) <cavity-figure>
]

*A vent was on the list of excuses for $s$, and it does not belong there.* A vent
is a Helmholtz port and therefore a *high-pass* leak: below $f_H$ the air flows
out and no pressure builds, above it the plug's inertia blocks the flow and the
cavity behaves as sealed. On this shell $f_H$ runs 21.8 Hz for a 6 mm vent to
59.6 Hz for a 25 mm one, all far below the 150 Hz fundamental, so the diverted
fraction there --- falling as $1 \/ f^2$ above $f_H$ --- is 2.1% to 15.8%, and
less at every higher mode. Explaining a twelvefold reduction needs about 92%. The
honest list is therefore shell flex and the non-piston mode shape, with the vent
worth a few per cent at most; neither of the two has been measured separately
here, and that is the open question the fitted $s$ now stands for.

$s$ is a fraction rather than a free gain because the rigid, sealed,
piston-driven enclosure is the stiffest case that exists: 1 is a physical
ceiling, not a neutral setting, and a fitted value above it would be a statement
that the model is wrong rather than that the drum is stiff. It multiplies every
$K_c$ alike, so the ceiling keeps its meaning once the air carries more than one
state.

*The most useful result of widening the cavity is a negative one.* The
hypothesis the work was done on was that the lumped reduction mis-set the
compliance, and that this was why the fitted $s$ sits a factor of twelve below
the rigid ceiling: one uniform pressure state stands in for a whole field, and
there is no obvious reason its best-fit stiffness should be the true bulk value.
It is. Every non-uniform air mode has exactly zero net volume ---
$integral_A Psi_c dif A = 0$ for $j' != 0$ --- so its impedance
$K_c (j Omega)^2 \/ ((j Omega)^2 + lambda j Omega + omega_c^2)$ vanishes as
$Omega -> 0$ and it contributes nothing to the quasi-static air spring the
doublet measures. The static compliance of the full expansion is *exactly*
$rho c^2 \/ V$, and it always was. Measured rather than argued: the doublet is
155.3 / 178.7 Hz, a ratio of 1.1509, and it is 1.1509 with one air state and with
the shipped six, unmoved to the transform's 2.93 Hz resolution. $s$ would still
have to be 0.083. The transverse modes are resonators, not springs; they reshape
the response near their own frequencies and leave the quantity the fit is made
against alone.

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
variation is proportional to the system's energy @marogna2010.

The approximation here is worth locating precisely, because it is not in
@strain-eq. That diagonal form is exact: the mode shapes are Dirichlet Laplacian
eigenfunctions, so $integral nabla phi_i dot nabla phi_j dif A = k_i^2 integral
phi_i phi_j dif A$ and the cross terms vanish identically. Writing
$g = abs(nabla w)^2$, all of Berger's error lives in the *second* moment: the
quartic membrane potential goes as $integral g^2 dif A$, while @berger-eq uses
$(integral g dif A)^2 \/ A$, the projection of $g$ onto the constant function.
Being a projection rather than a truncated series, it satisfies
$U_"Berger" <= U_"exact"$ by Cauchy--Schwarz. @coupling is that second moment
put back.

The $tanh$ is a
smooth cap rather than a clip, so it bounds
the frequency excursion without discarding stored energy. Each mode is detuned by
$Delta omega_i^2 = Delta T k_i^2 \/ sigma$.

The cap earns its keep at the anti-alias bound. Frequencies scale as
$sqrt(1 + r)$ where $r = T_max \/ T_0$, so retaining modes up to a fraction
$nu$ of the sample rate requires

$ r < 1 \/ (4 nu^2) - 1 $ <nyquist-eq>

which at the shipped $nu = 0.45$ is 0.2346 against a shipped $r$ of 0.2 --- a 17%
margin, validated at configuration decode rather than assumed. @nyquist-eq bounds
a *uniform* detune and nothing else, which is why the coupling of @coupling needs
its own bound. At the shipped coefficients the glide is 104.9 cents on the
loudest hit and 3.0 on the quietest, an audible semitone that still leaves the
plateau clear: past roughly twice these coefficients a loud hit sits on the flat
of the $tanh$, which turns the glide into a hold-then-drop and erodes the
velocity dependence that makes it expressive.

Because the tension depends on the state at the end of the step and the state
depends on the tension, the update is implicit. It is closed with a *discrete
gradient* rather than by evaluating @berger-eq at either endpoint:

$ overline(Delta T) = 2 [U(S^(n+1)) - U(S^n)] \/ (S^(n+1) - S^n) $ <gradient-eq>

Paired with the midpoint displacement this makes the nonlinear work over a step
exactly equal the change in stored potential, so the lossless model conserves
@energy-eq to solver tolerance rather than drifting --- which is what lets an
energy assertion be a test rather than a hope.

== The coupling channels <coupling>

@berger-eq is *exactly* the rank-one projection of the quartic potential onto the
constant channel. That is a stronger statement than "an approximation", and it is
what makes the missing part addable rather than replaceable. Choose an
orthonormal set $psi_c$ on the head and write

$
  hat(g)_c = chevron.l g, psi_c chevron.r = q^top D^c q, quad D^c_(i j) = integral psi_c (nabla phi_i dot nabla phi_j) dif A
$ <channel-eq>
$ U = tilde(beta) / 4 sum_c hat(g)_c^2 $ <quartic-eq>

The uniform channel $psi_0 = 1 \/ sqrt(A)$ gives $D^0 = "diag"(Gamma_i) \/ sqrt(A)$
and $hat(g)_0 = S \/ sqrt(A)$, so its term *is* the Berger potential with
$beta = tilde(beta) \/ A$ --- not an approximation to it, because the mode
gradients are orthogonal analytically by Green's identity. The table therefore
stores only $c >= 1$ and adds to the shipped capped law rather than replacing it,
which is also why the $tanh$ stays exactly where it was needed, on the channel
that detunes every mode uniformly, and is not applied to channels that detune
nothing uniformly.

*The force is cubic and odd, and that fixes what it can reach.* An even potential
gives an odd force, so the combinations generated are $3 f_a$,
$2 f_a plus.minus f_b$ and $f_a plus.minus f_b plus.minus f_c$ --- no second
harmonic and no simple sum or difference tone, which would need a *quadratic*
term in the potential that a shell or a curved plate has and a flat tensioned
head does not. The consequence is a design requirement rather than a preference:
the lowest combination consumes three slots, so a single pump reaches only $f_a$
and $3 f_a$, and $3 f_(0 1) approx 450$ Hz falls *below* the 476--700 Hz band the
excitation gap of @levels sits in. At least two pumps are therefore mandatory,
and a configuration asking for fewer is rejected outright with that reason.

The channels are built from ${1} union {nabla phi_a dot nabla phi_b : a, b in P}$,
Gram--Schmidt orthonormalised offline, with each $D^c$ truncated to entries
carrying at least one index in the pump set $P$ --- so the $abs(P) >= 2$
requirement is structural rather than documented. The selection rule falls out of
the angular algebra: the gradient product of two Fourier--Bessel modes is exactly
two harmonics, at $abs(m_a - m_b)$ and $m_a + m_b$, so a quartic coefficient
vanishes unless two gradient products share an angular order *and* an orientation
family. That second condition is the one that removes most of the tensor, it is
*not* the naive $plus.minus m_i plus.minus m_j plus.minus m_k plus.minus m_l = 0$
the four-index form suggests, and there is no radial rule at all. On the shipped
bank with $abs(P) = 4$ it leaves 408 structurally non-zero coefficients across 10
channels, removing about 89% of the candidates, and 34 modes provably carry none.
The shipped budget retains 256 of the 408.

*The pump set is chosen by displacement, not by frequency and not by energy.*
Since the force goes as $q^3$, the right weight is peak modal displacement under
a reference velocity-1 strike, available in closed form from the strike weight
and the contact pulse's transform. The measured ranking is not frequency-ordered
and that matters: the $(2,2)$ at 525 Hz outranks the $(1,1)$ sine at 240 Hz,
which outranks the $(1,2)$ at 437 Hz, so a frequency-ordered set of four would
retain different modes. The shipped four are the $(0,1)$ at 150.1 Hz, the $(1,1)$
cosine at 238.7, the $(2,1)$ cosine at 320.0 and the $(0,2)$ at 344.7.

*The energy exactness survives the extension, and for free.* Because $U$ is a sum
of functions of *scalar* quadratic forms, the scalar secant of @gradient-eq
already is the vector discrete gradient, applied per channel: no Gonzalez
projection, and no $0\/0$ branch to take on a 96-vector at rest. Measured, the
identity holds to a relative residual of $2.5 times 10^(-15)$ and the lossless
coupled system drifts by $1.1 times 10^(-11)$ over a second.

The anti-alias bound has to be restated, because a force that multiplies three
sampled signals reaches the sum of three modal frequencies and @nyquist-eq says
nothing about that. Since every retained entry carries a pump index the worst
case is not $3 f_"top"$; a receiver that is itself a pump admits two free indices
and is bounded by

$ f_(max P) + 2 f_"top" $ <alias-eq>

while a free receiver reaches only $2 f_(max P) + f_"top"$. At the shipped bank
the conservative bound is 3631 Hz and the table actually built reaches 2709 Hz,
both against 21.6 kHz --- an eightfold margin, not binding at any shipped tier.
It is implemented anyway, because it bites if the tier rises, if the sample rate
falls toward 8 kHz, or if the pump band widens.

*What it does is measured on modal amplitudes rather than on the spectrum, and
the distinction is the finding.* With every non-pump mode's strike projection
zeroed, so that the cubic force is the *only* path into any other mode, modes
that receive no excitation at all rise by roughly 50 dB: the $(2,2)$ cosine at
524.9 Hz from $-76.3$ to $-28.4$ dB, and the $(2,3)$ cosine at 725.4 Hz from
$-86.4$ to $-34.6$. Reading that off the radiated spectrum would have flattered
it, because four damped sinusoids put a Lorentzian leakage floor at $-37$ dB
across the band --- leakage from the pumps rather than content in it. With the
cavity disabled the uncoupled figures are not small but *exactly zero*: those
modes never move.

That is the mechanism @levels leaves open. A mode pumped by the coupling does not
read $abs(F(f))$ at its own frequency at all, so it can be excited precisely
where the half-sine's zero comb has deleted the excitation outright --- which is
the one thing the excitation-gap analysis never considered, because the model had
none to consider.

*One independent corroboration, and one calibration that is now pending.* At the
shipped configuration $tilde(beta) = beta A = 7.0 times 10^5$ N/m, against a
material $E h \/ (2(1 - nu^2))$ of $6.5 times 10^5$ N/m for a mylar head of the
implied thickness --- agreement to about 8%, which is the strongest independent
corroboration any coefficient in this model has. It is worth two caveats: it
reuses the same unmeasured surface density and a datasheet modulus, so it is not
fully independent, and the coefficient is fitted either way. Against that, the
glide moved only 102.8 to 104.9 cents, far less than the ratio between the two
moments predicts, and the reason is the fixture rather than the model: the
calibration hit is at the head's *centre*, where only axisymmetric modes are
excited and the orthogonal channels see almost nothing. The shipped strike is
off-centre, so the calibration is recorded as pending a refit rather than as
confirmed.

*And the local quartic is a bracket, not the exact operator.* Full von Kármán
condenses the in-plane displacement through an Airy stress function, giving a
quartic with an inverse-biharmonic kernel. Berger is that family's *uniform*
limit, with the in-plane stress spatially constant, and $integral g^2 dif A$ its
*local* limit, with the stress following the local slope and no elastic
redistribution at all; the truth sits between. What is claimed here is therefore
the coupling's *structure* --- which frequencies can be generated, and by which
mode sets --- and not its magnitude.

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
reduced cavity does not reproduce the full magnitude of that path --- and
widening it to six states does not either, since every state but the uniform one
has zero net volume and so carries nothing at this frequency. The
residual is carried explicitly, as a per-mode correction $Delta_i$ on the $(0,1)$
of each head, fitted to hold its $T_60$ at 0.213 s. It shows in @zeta-eq as a
$zeta$ of 3.44% against the band's 0.74%, and in @modes-figure as the one marker
far below the line. Naming it as a correction rather than folding it into $d_0$ is
what keeps the other two coefficients readable.

#place(top, scope: "parent", float: true)[
  #figure(
    image("figures/modes.png", width: 100%),
    caption: [
      The whole instrument's mode map at the shipped tuning: 96 batter and 24
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
Hz and 12--23 dB above 800 Hz.

*Part of that advantage has since been taken from the other end.* With the
coupling of @coupling active the Hertzian model's lead at 800 Hz falls from 11.9
to 7.9 dB, with 1500 and 2500 Hz unmoved, and it falls because the *prescribed*
side rose about 4 dB rather than because the Hertzian side lost anything. The
cubic force fills part of the band the half-sine's zero comb deleted, which is
the excitation gap being addressed by a source instead of by a touch.

Neither model reproduces what the measured
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
      $(0,2)$ at $-8.7$ --- which is what a tom microphone actually hears. Both
      heads' modes are plotted, so the second $m = 0$ marker near 162 Hz and the
      $m <= 2$ resonant series are weights the resonant head *would* radiate with
      and which this model deliberately does not sum in.
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
      Why the instrument is a hybrid. The staircase is what the budget resolves
      across both heads, the dashed curve is @density-eq for one membrane ---
      what would have to be resolved to keep going --- and the shaded spans are
      what the stochastic layer covers instead. The two track each other in slope
      up to the ceiling, which is the check that the count is being spent on the
      modes a membrane actually has; the staircase's excess above the curve is
      the second head's 24 oscillators.
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

== Integration and cost <cost>

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

where $omega_"eff"$ carries mode $i$'s current tension increase. The cavity would
couple every mode to every other, but it does so through $k$ rank-one terms, so
it is eliminated rather than solved: each mode's midpoint velocity depends on the
pressures affinely, and substituting that into the discretised @cavity-eq leaves
a $k times k$ system where the lumped model left one scalar equation. That system
is $"diag"(K_c)$ times a symmetric positive definite matrix --- the diagonal term
is strictly positive and the coupling block is a Gram matrix with positive
weights --- so every pivot is positive and elimination without pivoting is safe.
$k$ is capped at eight and validated at decode. At $k = 1$ the elimination is
literally the old single division, which is what keeps a one-mode cavity
bit-exact rather than merely equivalent; a test writes the rank-one form out by
hand and compares to the last bit over four thousand samples.

The nonlinearity closes the loop with a fixed-point iteration on @gradient-eq,
capped at eight passes, with the coupling of @coupling inside that loop since its
per-channel secants depend on the endpoint too. The cap is not the cost: measured
on the shipped configuration at full velocity the mean is 2.88 iterations, and
sweeping the tension coefficient over a 32$times$ range moves it only to 3.09,
because a stiffer law both perturbs the tension more and contracts faster once
the $tanh$ begins to saturate. That measurement retired a planned change --- an
explicit energy-proportional detune, proposed on the assumption that all eight
passes ran --- and it is the reason the exact energy bookkeeping was kept.

*The shipped configuration is below real time in the browser, and that is stated
plainly rather than buried.* The `js/wasm` worst case --- a full-velocity
retrigger before every 512-sample chunk, so the solve never idles --- measures
0.70$times$ real time at the shipped 256 coefficients, against 1.40$times$ with
the coupling off and 1.66$times$ before this chapter's two mechanisms existed.
Three things soften it and none of them settle it. It is the worst case and not
the steady one, since a real hit lets the solve idle at 3.9$times$. The cost is
not a harder solve: the mean iteration count moves only 2.40 to 2.49 at velocity
1, so what is being paid for is the table walk itself. And nothing here is
optimised --- the table is walked over a run partition that blocks by channel and
by the row of each retained pair, so within a run that mode's displacement and
inverse mass hoist out of the inner loop and its force accumulates in a register,
but the receiver side is unblocked and still scatters into the acceleration
array, and the table is rebuilt per iteration rather than updated incrementally. The budget is also not currently buying much: across a sixfold
range of retained coefficients the level in the band the coupling exists to reach
moves 0.9 dB, and the full table is not even the loudest of them, so 256 is
chosen for margin and 128 is the first thing to try. Making this affordable is
open work, deliberately deferred, and not a closed question.

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
      [Cavity mode frequencies], [derived], [@cavitymode-eq, zeros of $J_m'$ over the shell radius],
      [Head/air coefficients $C_(i c)$], [derived], [@overlap-eq; closed form at $j' = 0$, quadrature elsewhere],
      [Quartic coefficients $D^c$], [derived], [@channel-eq over the retained shapes],
      [Far-field weights], [derived], [@lommel-eq, a Lommel integral in closed form],
      [Attack-band releases], [derived], [@band-eq, the head's own @loss-eq],
      [Coupling coefficient $tilde(beta)$], [derived, then corroborated], [$beta A$ from the fitted Berger $beta$; within 8% of $E h \/ (2(1-nu^2))$],
      [Contact exponent $alpha = 3\/2$], [measured], [velocity dependence of contact time @wagner2006],
      [Contact duration], [measured], [5.5--8 ms on a 12-inch tom @dahl1997],
      [Damping ratio $zeta$], [measured], [constant $Q$ on membranes @fletcher1998],
      [Cavity split ratio], [measured], [10--20% on two-headed drums @richardson2012 @fischer2014],
      [Cavity stiffness scale $s$], [*fitted*], [to that measured split],
      [$(0,1)$ decay correction $Delta_i$], [*fitted*], [to a 0.213 s ring time],
      [Near-field scale $s_"nf"$], [*fitted*], [to partial balance at the shipped mic],
      [Attack level], [*fitted*], [to spectral balance against the modal layer],
      [Output gain], [*fitted*], [so a full-velocity hit peaks below clipping],
      [Berger $beta$], [*fitted*], [to a 100-cent glide, and *pending a refit* --- see @coupling],
      [Cavity pressure loss $lambda$], [*unjustified*], [nothing; 5 1/s is a placeholder no measurement stands behind],
    ),
    caption: [
      Where each number comes from. The six fitted rows are the model's
      admissions: each is a quantity a reduced model cannot compute, and each is
      fitted against a *different* measurement rather than against the same one.
      The coupling coefficient is the one row of its own kind --- it is not a new
      free parameter but the fitted Berger one spent on the channels Berger
      projects away, and it then lands within 8% of a material value it was not
      fitted to. The last row is of a fourth kind and was absent from this table
      until it was noticed to be: the cavity's pressure loss is neither derived,
      measured nor fitted, and a taxonomy that simply omitted it was making the
      model look better argued than it is.
    ],
  ) <provenance-table>
]

The shipped values themselves are collected in @heads-table, @scalars-table and
@observation-table. All of them are SI-valued and versioned: the persisted schema is
at version 11, and every earlier version has an explicit migration. Those
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
      [], [oscillators, Standard], [96], [24], [---],
    ),
    caption: [
      The two membranes at the shipped default. The resonant head is thinner and
      slacker, and carries only the modes the enclosed air can reach --- a
      selection that is exact rather than approximate, followed by a truncation
      to its own budget that is not.
    ],
  ) <heads-table>
]

#place(top, scope: "parent", float: true)[
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
      [air states], [6], [---],
      [resonant mode limit], [24], [---],
      table.cell(colspan: 3)[_Nonlinearity_],
      [tension ratio $r$], [0.2], [---],
      [anti-alias bound], [0.2346], [---],
      table.cell(colspan: 3)[_Coupling_],
      [$tilde(beta)$], [7.0e5], [N/m],
      [pumps $abs(P)$], [4], [---],
      [pump band], [700], [Hz],
      [coefficients, of 408], [256], [---],
      [worst force frequency], [2709], [Hz],
    ),
    caption: [
      The air, the tension cap and the coupling channels at the shipped default.
    ],
  ) <scalars-table>
]

#place(top, scope: "parent", float: true)[
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
]

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

*The mode coupling is truncated to one generation from a few pumps.* @berger-eq
alone is mean-field --- every mode detuned by the same relative amount, *no mode
able to transfer energy to any other*, which is the defining property of the
Berger and Kirchhoff--Carrier family @marogna2010 and which would mean the
nonlinearity contributed pitch and no spectral content whatever. @coupling is
that exclusion lifted, and what remains excluded is the size of the retained
tensor rather than the mechanism. Full von Kármán coupling is $O(N^4)$ in
coefficients and $O(N^3)$ to evaluate, so at 96 oscillators it arrives needing a
truncation of its own, and the shipped one is severe: every retained term carries
a pump index, the pump set is four modes below 700 Hz on the *batter* head only,
and 256 of 408 structurally non-zero coefficients survive the budget. So there is
one generation of transfer and no cascade --- energy reaching a receiver cannot
be re-radiated by it into a third mode --- no parametric sidebands from the
time-varying tension, and no coupling on the resonant head at all. The parity is
not a truncation but a property: the force is cubic and odd, so $2 f_i$ and
$f_i plus.minus f_j$ are absent because they need a *quadratic* term in the
potential, which a shell or a curved plate has and a flat tensioned head does
not. That a hard hit is *brighter* and not merely sharper is measured in
@dahl1997, a source cited here already for contact time, and again in @kirby2021,
which tracks a tom-tom's modes across 67 strike intensities and resynthesises
that evolution well enough that twenty listeners scored exactly chance in an AB
test against the recordings --- so what a strike does to the modes, and not only
to the level, is what an intensity-dependent drum sound is made of. This model
produces part of that brightening as a source term and attributes the rest to
excitation and to the attack layer's velocity scaling. Nonlinear modal synthesis with the
coupling terms retained wholesale runs in real time @diaz2026; this one, at the
shipped truncation, does not yet --- see @cost.

*There is no shell, no bearing edge, no vent and no hardware.* These are real and
audible on real drums; they are excluded because none of them can be calibrated
against anything currently available, and a free parameter with no measurement
behind it is indistinguishable from a fudge factor. There is no room and no
snare. The cavity now has transverse modes but no *axial* ones, so a pressure
that varies along the shell --- and with it the sign difference between what the
two heads see --- is still absent, and so is any shell mode the air could couple
to. The vent's absence is now quantified rather than asserted: it is a Helmholtz
high-pass worth a few per cent of the head's volume velocity at the fundamental
and less above, which is why it is excluded and also why excluding it does not
explain the fitted stiffness scale.

*And the reference set is synthetic.* The committed fixture that pins this model's
behaviour is generated from the model itself, deterministically. It is a
regression reference, not an acoustic validation reference, and no measurement in
this chapter should be read as agreement with a real drum. Where a number here
does rest on a measured instrument --- the contact time, the damping ratio, the
cavity split --- it is cited to the literature it came from, and @provenance-table
is where that is visible at a glance.

*Two rows of that table lean on measurements that do not appear to exist, and
both are recorded here as OPEN rather than guessed at.* The first is the contact
duration. Every source behind it is a *stick*: Dahl's 5.5--8 ms endpoints
@dahl1997 and Wagner's crescendo @wagner2006 are both struck with sticks, and no
measured felt-mallet contact time on any drum was found. The shipped mallet is
therefore a stick's contact law given a different mass and hardness, and the
longer, softer contact a felt head should give is uncalibrated --- which matters
because @contact-eq turns duration directly into the excitation spectrum. The
second is the loss split. @loss-eq keeps radiation apart from the structural
channels precisely so that calibration can attribute a change to a cause, and
@heads-table assigns $d_"rad" = 1.5$ 1/s, but no numeric
radiation-versus-internal damping split has been published for *any* drum. Only
the total the two sum to is answerable to the constant-$Q$ measurement
@fletcher1998 the law is shaped against; the division between them is a modelling
choice wearing the notation of a measured one. Neither gap is filled by a plausible
number here, because a number with nothing behind it would be indistinguishable in
@provenance-table from one of the measured rows.

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
      spurious share is counted. Eleven of the reference's fourteen partials find a
      counterpart. Of the three that do not, two sit near the detection floor and
      are correspondingly cheap to miss, while the third stands 26 dB below the
      strongest and carries most of the unmatched share on its own --- which is
      the audibility weighting doing exactly what it is for.
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

#correction[
  *Read with @glidefault.* The glide term of the distance minimised below was
  later found to be faulty, and both the glide rows in this chapter and the
  search that produced the banks are conditioned on it. The numbers here are
  reported as they were obtained; @glidefault gives the fault, its size, and
  what of this chapter it puts back in question.
]

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
content while inventing little of its own, so the remaining terms are computed
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

*The same apparatus settles a design question.* The prescribed contact model and
the Hertzian alternative were fitted separately at identical budget,
seed and reduction --- the only difference being the flag --- so the comparison
is between two excitations over the same bank rather than between one excitation
and a default that happens to suit it.

#figure(
  table(
    columns: 4,
    align: (left, right, right, right),
    table.header(
      [Term],
      [Threshold],
      [Prescribed],
      [Hertzian],
    ),
    [Partial frequency], [25 cents], [56.9], [*47.2*],
    [Partial decay], [0.25], [*0.581*], [0.728],
    [Spectral envelope], [4 dB], [12.28], [12.30],
    [Partial level], [3 dB], [*7.58*], [9.18],
    [Amplitude envelope], [3 dB], [1.27], [*1.18*],
    [Glide (see @glidefault)], [40 cents], [17.6], [*0.12*],
    [Attack balance], [6 dB], [*0.03*], [1.31],
    [Unmatched share], [0.5], [0.151], [*0.047*],
    [Spurious share], [0.5], [*0.275*], [0.359],
    [*Total*], [], [*11.252*], [11.535],
    [Baseline], [], [33.094], [33.544],
  ),
  caption: [
    Two contact models at identical budget, seed and reduction. Bold marks the
    better of the pair on each term. The prescribed model wins the sum, and the
    two win on different terms --- which is the reading the per-term units exist
    to make available.
  ],
) <contact-table>

The prescribed model takes the sum, 11.252 against 11.535, and the restart
spreads say how firmly. Its eight restarts run 11.252 to 15.461 and the
Hertzian's 11.535 to 18.904; the Hertzian best would rank second among the
prescribed restarts while its remaining seven fall outside the prescribed range's
better half. That is a *consistent* ordering rather than a decisive one, and it
is worth stating in those terms: the prescribed excitation earns the default it
already holds, and nothing here claims it is the better physics.

What the per-term reading adds is that the two do not merely differ in size. The
Hertzian fit is better placed in frequency, has essentially no glide error and
leaves less of the reference uncovered; the prescribed fit has the better decay,
partial balance, attack balance and tidiness. A single number would report one
winner and conceal that the two are good at different things --- which is the
information a next iteration of the excitation model would want.

*A normalised position beside an engineering value turns a fitted parameter into
a question.* Head damping here is fitted to 0.709 at normalised position 0.376,
comfortably inside its range. An earlier run under the superseded reduction
fitted the same parameter to 0.276 at position 0.036 --- against the edge --- and
it was the position rather than the value that made that legible. @lossscale puts
that question to the model directly.

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
    [converged pre-solve, corrected target], [35.9--37.0 cents],
    [gate], [25 cents],
  ),
  caption: [
    What the analytic pre-solve reaches, against what the gate asks for. The
    first two rows are measured against the reduction of the reference as it
    stood before the correction of @levels; the third against the corrected
    reduction, whose fourteen partials include seven inside a 224 Hz span. The
    pre-solve is the same in both cases --- it is the target that changed.
  ],
) <presolve-table>

Read that table as a property of the *searchable space* rather than of any
search: it reports how close the modes of the best bank the pre-solve can
construct come to the partials it is asked to hit, before a single sample is
rendered. Against the earlier reduction the answer was around a cent, well inside
the gate. Against the corrected one it is around 36 cents, outside it. Obtaining
that answer costs roughly one full evaluation, which makes it a cheap thing to
know in advance and, as below, a decision procedure rather than only a diagnostic.

`-seeded-restarts N` puts it to use: a pre-solve finds $N$ diverse
frequency-optimal banks, and those $N$ restarts search a box around them while
the remainder search the whole cube. The seeded restarts come first and the
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
and, against the reduction as it then stood, the pre-solve's two seeds land at
1.0 and 1.3 cents.

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

*A seed helps in proportion to how good it is, and the pre-solve says how good it
is before the search starts.* @seeding-table is a 1-cent seed, and it buys 12%.
The corrected reduction of @levels gives the same pre-solve a harder target, its
seeds floor at 36 cents, and the two full-budget fits of @results --- eight
restarts each, four of them seeded --- were both *won by an unseeded restart*:
11.252 against a best seeded 11.741 with the prescribed contact, 11.535 against
13.574 with the Hertzian. A box is range spent to buy a head start. At one cent
the head start is a real selection from the distribution below and the trade is
worth making; at thirty-six the seed is barely a selection, and the box only
removes room. The flag therefore stays off by default, and the number that
decides whether to raise it is printed by the pre-solve for the price of one
evaluation.

That is the durable part of this section. @seeding-table and the cent figures in
it are measured against the earlier reduction and do not transfer, but the
mechanism, the two defects the first version exposed, the paired design that
makes the table readable, and the rule above all survive the change of target
--- and the rule is only visible *because* the method was tried against two
targets of very different difficulty.

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

@corrected-table re-scopes a standing claim, and the fits of @results then
narrow it. Fits under the earlier reduction reproduced little in 476--700 Hz
while the reference was busiest there, and the reference still is --- but part of
that gap was the target not asking for those partials either, because they sat
below the floor the level estimate set. On the corrected metric both full-budget
fits place *five of their sixteen partials* in that band against the reference's
seven, so the model does populate it. Whether the remaining difference in density
is an excitation deficit or a mode-density one is open, and the corrected
reduction is what makes it answerable: the target now asks for the partials the
question is about.

One quantity to keep in view while answering it: the analytic pre-solve of
@seeding floors at 36 cents against this target, so the frequency gate's
reachability within the shipped bank is itself an open question. The gate was
calibrated against a reduction with half as many partials, and a per-partial cent
tolerance does not ask the same thing of a model when seven partials must be
placed inside a 224 Hz span. Re-deriving it against the corrected reduction is
the natural next step, and it is a question the apparatus can now be pointed at
directly.

= A glide read off a dead partial <glidefault>

@levels is a measurement defect found by probing, corrected, and then folded into
the fits it changed. This chapter is the same genre of finding with the opposite
disposition: the defect was found *after* the fits of @results were run, and it is
reported here as a correction *to* them rather than absorbed into them. Nothing
above this chapter has been restated. The paper is a record of what was done, and
a run described in one chapter and silently re-scored in another is not one.

The glide term reduces the whole pitch bend to a single number: the frequency of
one tracked partial at an early probe against its frequency at a late one, in
cents. Two faults, independent of each other, were in that estimator throughout
the work reported above.

*The late probe fired after the tracked partial was dead.* The probe sat at a
fixed 0.400 s. The model's $(0,1)$ has a ring time near 0.21 s, so by then it is
roughly 105 dB below where the early probe found it, and what the second reading
returns is not that partial at all but whatever leaks through the probe's
passband from a neighbour --- reported, in cents, as the offset to that
neighbour. On ordinary renders this produced readings such as $-717$ cents. At a
weight of one part in forty cents that is a contribution of 18 to totals of order
32: a term nominally worth 0.44 in @contact-table could, on a different
candidate, be worth more than half the distance.

*The glide was read off the loudest partial rather than the fundamental.* The
loudest partial was chosen deliberately, and for a reason that still holds --- the
lowest peak in the band may be a shell resonance or a room mode forty decibels
down, and the bend belongs to the mode carrying the note. But on this reference
the loudest partial is at 212.74 Hz with a 0.162 s ring time, while the
fundamental at 118.05 Hz is 7.7 dB quieter and rings for 1.53 s. The measurement
was being taken on whichever mode peaked highest rather than on the one that
still existed to be measured.

Fault two is the one with reach, because it moved *the target*. The reference's
glide is not what the fits of @results were asked to match:

#figure(
  table(
    columns: 3,
    align: (left, right, right),
    table.header([Reference reduction], [as measured then], [corrected]),
    [Mono], [89.3 cents], [*54.1 cents*],
    [Right channel, as used in @results], [120.4 cents], [58.9 cents],
  ),
  caption: [
    The reference's downward glide before and after the correction. The earlier
    figures were read from a mode that had decayed into the noise floor by
    0.15 s; the corrected ones track the fundamental, whose trajectory over the
    same span is monotone.
  ],
) <glide-table>

The estimator now walks the late probe back from its nominal position to the
last point at which the tracked partial is still within 20 dB of its early level,
refuses any early-to-late span shorter than 50 ms outright, and tracks the lowest
partial standing within 20 dB of the loudest --- which keeps the guard against the
forty-decibel room mode while preferring the fundamental, on a drum the
longest-lived thing in the recording. It also returns a *measured* flag beside the
value, so that a take whose fundamental cannot support two probes is reported as
unreadable rather than as zero cents. That last part is the piece worth
generalising, and it is the eleventh point of @watch.

== What this corrects, and what it does not

The correction reaches further into @results than a rescored row. The glide term
was inside the objective the search minimised, so both fits in @contact-table are
*conditioned* on it: it is not that nine terms were measured correctly and one
was not, but that the optimiser was steered, at every evaluation, by a term that
could be worth 18 on a candidate whose fundamental died early. The fitted banks
of @results are outputs of the faulty objective and are described as such.

Two specific consequences are worth naming rather than leaving to the reader.

*The contact-model ordering is not established by @contact-table.* The margin is
0.283 and the prescribed fit's glide row contributes 0.44 against the Hertzian's
0.00, both measured against a target of 120.4 cents that should have read 58.9.
The ordering may well survive --- it is a consistent one across restarts --- but it
is no longer supported by the numbers printed. @refit re-derives the prescribed
side under the corrected objective; the Hertzian side has not been re-run, so the
ordering itself is still outstanding.

*The shipped Berger $beta$ is untouched.* It was fitted to the model's own glide
on an isolated batter fixture --- 96.9 cents between velocity 0.2 and velocity
1.0, 104.9 at the shipped coefficients --- measured directly from mode
frequencies and never through this estimator or against this recording. The
*pending refit* recorded against it in @provenance-table is the coupling question of
@coupling and is unrelated to anything here.

What is worth reading against it instead is the corrected target itself. Three
independent measurements put a loud hit on a real tom between 130 and 165 cents
--- Fletcher and Rossing's §18.4 relays Bork and Meyer at 160 cents on a 32 cm
tom and Rose at about 8% on a 33 cm one @fletcher1998 @bork1983, and Gärder
measures almost 8% on a 14-inch drum @garder2005 --- against this
reference's corrected 54.1. Those do not conflict; the natural reading is that
the reference is simply not a loud hit. But the reference is what the objective is
written against, so a nonlinearity fitted through it is fitted to match *this
recording* and should be read as that rather than as a physical coefficient.

== A parameter the recording cannot see

One further result falls out of the correction, and it is the kind that is easier
to state than to obtain. Swept over the cavity stiffness scale $s$ from the
shipped value to a rigid shell, the objective total used to range 32.83 to 53.15
--- a spread of 20.3, which reads as strong discrimination. Under the corrected
estimator the same sweep ranges 32.98 to 34.10, a spread of *1.1*.

Almost all of the apparent discrimination was the glide artefact: at the strongly
coupled end the tracked partial died early and the probe read a neighbour, and
the size of that misreading varied with the coupling. What remains says that *this
recording has essentially nothing to say about $s$*, which is a substantive
finding given that $s$ is the one cavity parameter the model fits rather than
derives (@provenance-table) and that its value is under independent question from the
literature. A parameter that cannot be identified from the target is a parameter
whose value has to be settled by a measurement made for the purpose --- Fischer's
doublet protocol on a tom --- and not by a better fit.

= The refit under the corrected objective <refit>

@glidefault leaves a debt: the banks of @results are outputs of an objective that
has since been corrected, and re-deriving them under the corrected one was
recorded there as outstanding. This chapter pays part of it. The prescribed
contact model was refitted from scratch, at a budget larger than @results ran at,
against the corrected estimator. The Hertzian alternative was *not* refitted, so
@contact-table is not superseded here --- it remains a superseded result, and the
contact-model ordering remains undetermined.

== Putting two objectives on one axis

An archived report's stored total was produced by an objective that no longer
exists, but the report records its own best point exactly, so the bank can be
re-measured: pin every free parameter to its recorded normalised position, pin
the strike velocity, and ask for a report without a search. Quality tier, contact
model and channel must be reproduced from the report's own header, or the answer
is about a different drum.

#figure(
  table(
    columns: (auto, auto, auto, auto),
    align: (left, right, right, right),
    table.header(
      [Archived run],
      [Then],
      [Today],
      [Glide (cents)],
    ),
    [Control, post-fix], [10.577], [*10.577*], [1.9 #sym.arrow 1.9],
    [Hertzian, @contact-table], [11.535], [15.146], [0.1 #sym.arrow 59.6],
    [Hertzian, earlier], [11.630], [15.650], [--- #sym.arrow 49.7],
    [Prescribed, @results], [11.252], [*18.991*], [17.6 #sym.arrow 107.1],
  ),
  caption: [
    Four archived banks re-measured under the corrected objective, each at its
    own tier, contact model, channel and velocity, against their totals as
    reported at the time. The first row is the recipe validating itself: it is
    the one archived run made after the correction, and re-scoring reproduces its
    reported total to the digit, term for term.
  ],
) <rescore-table>

Two readings of that table are wrong and worth closing off. It is *not* a measure
of what the glide fault alone cost, because a re-score renders the bank through
today's synthesis as well as measuring it with today's metric, and the model
gained its mode-to-mode coupling channels (@coupling) on by default in the same
interval --- the shipped baseline moved 33.094 #sym.arrow 32.442 on the same bank
and channel for that reason alone. And it is *not* a ranking of the searches that
produced those banks, because each was optimised against a different objective
from the one marking it. What it does say is that the run which won under the old
objective is the worst of the four under the corrected one, and the glide column
says why: a near-zero error against a fabricated target is a bank tuned onto the
artefact.

== The run

Twelve restarts of 150 iterations over a population of 16, four of the twelve
seeded from the pre-solve of @seeding, seed 1, right channel, Standard tier,
prescribed contact. All twelve completed; 88,584 evaluations. *The distance falls
from 32.442 at the shipped default to 10.382.*

#place(top, scope: "parent", float: true)[
  #figure(
    image("figures/terms-refit.png", width: 100%),
    caption: [
      The refit, every term as a multiple of its own threshold, drawn as
      @terms-figure is. Glide and attack balance have no visible fitted bar
      because they fall off the bottom of the axis --- 0.03 cents against a
      40-cent threshold and 0.02 dB against 6. The three gated terms are
      highlighted and all three are still missed.
    ],
  ) <terms-refit-figure>
]

Five terms again fall below threshold and the same four remain above, but the
comparison that matters is not with @results, whose total is not on this axis.
It is with the one archived run measured the same way --- the control row of
@rescore-table, an interrupted eight-restart run at a different seed.

#figure(
  table(
    columns: 3,
    align: (left, right, right),
    table.header([Term], [This run], [Control]),
    [Partial frequency], [48.936 cents], [48.997 cents],
    [Partial level], [9.081 dB], [9.172 dB],
    [Partial decay], [0.573], [0.602],
    [Spectral envelope], [11.068 dB], [11.034 dB],
    [Amplitude envelope], [0.724 dB], [0.746 dB],
    [Unmatched share], [0.047], [0.047],
    [Spurious share], [0.327], [0.330],
    [Glide], [*0.029 cents*], [*1.892 cents*],
    [Attack balance], [*0.019 dB*], [*0.188 dB*],
    [*Total*], [*10.382*], [10.577],
  ),
  caption: [
    Two independent searches under the same objective --- twelve restarts against
    eight, different seeds, one complete and one interrupted. Every term the
    objective can read about the drum agrees; the whole of the 0.195 difference
    is the two bold rows, which are the cheapest terms in the sum to polish once
    the rest has settled.
  ],
) <convergence-table>

*That agreement, and not the 1.8% better total, is the result.* Two searches
arriving at the same point from different starts is a statement about the model's
ceiling on this recording rather than about either search: the remaining 10.4 is
not search effort left on the table. The winning restart's convergence agrees from
the inside --- its last three iterations read 10.3829, 10.3823, 10.3821, flat to
four digits, where the run of @results was still visibly descending when its
budget ran out. A better total on this reference now needs a different model, not
a longer run.

== What the ceiling is made of

All three gated terms are missed, for the third round running: partial frequency
48.9 cents against 25, partial decay 0.573 against 0.25, spectral envelope
11.07 dB against 4. The spectral envelope is now on its *sixth* independent
intervention --- contact model, head-damping range (@lossscale), seeding
(@seeding), the level correction (@levels), the glide correction (@glidefault),
and a converged full-budget refit --- and it has sat between 11 and 13.6 dB
throughout. A term that does not move for six unrelated changes is as likely to be
ill-posed for this model as it is to be a hard target, and nothing measured here
distinguishes the two.

#place(top, scope: "parent", float: true)[
  #figure(
    image("figures/decay-refit.png", width: 100%),
    caption: [
      Ring times of the refit against the reference, drawn as @decay-figure is.
      The residual is the same shape and it has not softened: across 200--750 Hz
      the model delivers 0.5--1.3 s at all but one of its partials, where the
      reference scatters from 0.15 to 1.2 s, and at the fundamental it now rings 0.83 s
      against the reference's 1.49 --- shorter than the run of @results managed
      there. Its longest-ringing partial, 1.81 s at 186 Hz, has no counterpart in
      the reference at all.
    ],
  ) <decay-refit-figure>
]

@decay-refit-figure is @decay-figure again, which is the point: the correction
moved the objective and the fitted bank, and left the residual where @lossscale
located it. The partial-level term reads the same fact from the other side. The
model's loudest partial is its fundamental at 118 Hz, while the reference's is at
213 Hz --- a partial the model does place, to within 43 cents, but sets 4.5 dB
below its own loudest and rings for 0.25 s where the reference gives it 0.15. A
loss law that cannot make the reference's
non-monotone ring times also cannot make its partial balance, because on this
recording the two are the same statement.

Coverage is where the refit has genuinely closed. Eleven of the fourteen reference
partials find a counterpart within 5%, the fundamental to within 6 cents,
which is what an unmatched share of 0.047 means concretely; what is missing is the
1598 Hz component and the two quiet, long-ringing partials at 140 and 530 Hz. The
0.327 spurious share is charging for five invented modes. The 476--700 Hz band ---
the one the reduction of @reference turns on --- holds five fitted partials against
the reference's seven, exactly what the earlier winner managed. Its density is
still not reproduced.

== Two notes on the seeding

The four seeded restarts converged at 26.777, 26.777, 26.777 and 26.793 cents of
frequency error, which is the best seed error yet measured against a corrected
target --- against the 35.9--37.0 cents of @presolve-table, and within 7% of the
25-cent gate. The reading offered in @seeding, that no bank the product can
express places its modes much better than the middle thirties on this target, is
therefore *too strong as stated* and is narrowed rather than withdrawn. Whether
the gate is reachable at all remains open.

The second note is a caution about the instrument that produced the first. Four
seeds from four different starts agreeing to three decimal places is not what four
diverse optima look like. Either the pre-solve's diversity constraint is not
producing diverse banks or this is one sharp optimum every start falls into, and
the report does not distinguish them --- which is worth settling before
`-seeded-restarts` is used to argue anything. Seeding lost again on the paired
reading: best 10.970 seeded against 10.382 unseeded, mean 13.05 against 11.68, and
the winning restart was unseeded for the third run running.

= What to watch for <watch>

Eleven points generalise beyond this instrument, each learned by measurement.

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
a 1.0-cent frequency term, and the tens of cents the seeded fit actually reached
is the measure of the gap. Mode placement is necessary for that term and nowhere
near sufficient.

*Let a cheap pre-solve decide whether its own output is worth using.* Seeding a
restart trades range for a head start, and the trade is only worth making when
the head start is large. The useful part is that the same computation which
produces the seed also reports how good it is, in the units the gate is written
in and before any expensive evaluation --- so the flag can be raised or left down
on evidence rather than on hope. Here the identical mechanism gained 12% against
a target it could reach to a cent and lost against one it could only reach to
thirty-six, with the winning restart unseeded in both full-budget fits. Prefer a
warm start whose quality you can read in advance, and treat "seeding helps" as a
statement about a target rather than about a method.

*A term that will not move under any intervention is a question about the term.*
Where the other eight terms of this distance respond to the contact model, to the
damping range, to seeding, to two corrected reductions and to a converged
full-budget refit, the spectral envelope has sat between 11 and 13.6 dB through
all six --- the largest single contributor in both fits of @contact-table, and
second only to partial level in @refit. That pattern is worth reading as evidence about the measurement
as much as about the model: this term is mean-removed band shape out to 12.5 kHz,
and above roughly 3 kHz the model offers only its stochastic attack layer --- no
shell, no bearing edge, no hardware. A threshold applied over a
span the model cannot structurally populate asks for something no parameter
reaches, and the remedy is then to restrict the band or re-derive the threshold
rather than to keep fitting against it. Whether that is the case here is not
settled and is being measured; the transferable point is that immobility under
independent interventions is a signal, and a per-term apparatus is what lets it
be seen.

*Check a cheap objective for discrimination before quoting its numbers.* Reading
both heads roughly doubles the mode count, and a dense enough bank is near *any*
frequency by accident. Measured over 2000 random banks, the audibility-weighted
frequency error has median 164.8 cents, tenth percentile 56.2 cents and best 4.1
cents, so a 1.0-cent seed is a real selection from that distribution rather than
an artefact of density. An objective that cannot separate a good bank from a
random one will seed noise just as confidently.

*An estimator must be able to say that it did not measure.* A probe placed at a
fixed time on a partial that decays is a probe that eventually reads something
else, and it reports that reading in the same units, on the same scale, with the
same air of authority as a good one --- so the failure is invisible exactly where
it is worst. Two guards close it, and both are cheap: place the second probe
relative to the tracked quantity's own decay rather than at a fixed offset, and
return a *measured* flag beside the value so that "did not bend" and "could not be
read" are different answers. Without the flag the two collapse onto zero, and a
zero flows into every sum downstream as though it were evidence. @glidefault is
what that cost here.

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

@terms-refit-figure and @decay-refit-figure are the same two renderers over the
report of @refit --- `just paper-figures-refit` --- written under a distinct
suffix rather than over the originals. A figure carries no record of the run that
drew it, so redrawing a chapter's figures from a run that chapter does not
describe is a silent way to make a paper wrong; each set is therefore named for
its own run and neither recipe can overwrite the other's output.

The five figures of @instrument come from a second committed artefact, written by
#code-path("cmd/analyze-physical") --- `just paper-data`, then the same
`just paper-figures`. Nothing in it is rendered: the modal bank is @dispersion-eq
and @loss-eq evaluated in closed form, and the cavity curves are the
continuous-time solve of @cavity-eq linearized at rest, so it depends on no
audio, no recording and no seed --- and, being linearized, it shows the cavity of
@cavity-figure and not the coupling of @coupling, whose effect exists only at
amplitude and is reported here from time-domain measurement instead. It is regenerated deliberately, like the images, rather than diffed in
continuous integration --- which is the reverse of the reference fixture the
model's regression tests use, and the two should not be confused. That fixture is
*generated from the model*, so nothing in @instrument is evidence of agreement
with a real drum; the measured quantities it does rest on are cited individually
in @provenance-table.

#show bibliography: set text(size: 8.1pt)
#bibliography("references.bib", style: "ieee", title: "References")
