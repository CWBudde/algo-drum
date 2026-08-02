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
carries no normal velocity, so the radial condition is Neumann — \(J*m'(j'*{mn})
= 0\) — and _not_ the \(J*m(z*{mn}) = 0\) the clamped heads obey. Restricted to
the axially uniform family,

\[
\Psi*{mn}(r,\theta) = J_m\!\left(j'_{mn}\frac{r}{a}\right)
\{\cos m\theta,\ \sin m\theta\},
\qquad
\omega_{mn} = \frac{c\,j'*{mn}}{a}.
\]

The uniform mode is the \(m = 0\) root at \(j' = 0\), carried as \((0,0)\); it
is a mode of zero frequency and is the whole of the old lumped model. Note that
\(j'_{01} = 3.8317\) is the first \_positive_ zero of \(J_0' = -J_1\) and is a
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

A test also pins the generated table at a second radius, \(a = 0.1584\) m, where
it reads 634.5 / 1052.6 / 1320.5 Hz; that anchor exists only so the series can be
checked by hand at two geometries.

### Coupling coefficients

The coefficient between head mode \(i\) and cavity mode \(c\) is the overlap of
their shapes over the head's disc,

\[
C\_{ic} = \int_A \phi_i \Psi_c \, \mathrm{d}A,
\]

and the same coefficient appears in both directions — the air is driven by
\(C*{ic}\dot q_i\) and the head is loaded by \(C*{ic}P_c\). That symmetry is
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
A*{0n} = 2\pi R^2\frac{J_1(z*{0n})}{z*{0n}},
\qquad
A*{mn} = 0 \text{ for } m > 0,
\]

which is the closed form the lumped model always used and which the code returns
verbatim in that case. For every other cavity mode the radial integral

\[
\int*0^R J_m\!\left(z*{mn}\frac{r}{R}\right)
J*m\!\left(j'*{m\nu}\frac{r}{R}\right) r\,\mathrm{d}r
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

With modal displacement \(q*i\), modal mass \(m_i\), loss rate \(d_i\), and
the product control \(g\in[0,1]\), define the effective coupling
\(\widetilde C*{ic}=gC\_{ic}\). Each cavity mode carries a pressure \(P_c\) and a
conjugate state \(H_c\), and the coupled equations are

\[
\ddot q*i + 2d_i\dot q_i + \omega_i^2 q_i
= f_i/m_i - \sum_c \widetilde C*{ic} P_c/m_i,
\]

\[
\dot P*c = K_c\sum_i \widetilde C*{ic}\dot q_i + \omega_c H_c - \lambda P_c,
\qquad
\dot H_c = -\omega_c P_c,
\]

\[
K*c = s\,\frac{\rho c^2}{\Lambda_c},
\qquad
\Lambda_c = \int_V \Psi_c^2\,\mathrm{d}V,
\qquad
\Lambda*{(0,0)} = V = \pi R^2L.
\]

Here \(L\) is shell depth, \(\rho\) is air density, \(c\) is sound speed, and
`Cavity.LossPerSecond` is the pressure-amplitude loss \(\lambda\).
`Cavity.Coupling01` is \(g\); scaling drive and feedback by the same coefficient
preserves passivity. Setting it to zero or setting `Cavity.Enabled` to false is
the exact zero-coupling limit.

The \((P*c, H_c)\) pair is the second-order cavity mode written in first-order
form: eliminating \(H_c\) gives \(\ddot P_c + \omega_c^2 P_c\) driven by the
head accelerations, which is the standard modal acoustic formulation. Writing it
this way rather than as a displacement/velocity pair is what makes the uniform
mode degenerate cleanly: \(\omega*{(0,0)} = 0\), so \(H\) never leaves zero and
the first equation collapses to the single \(\dot p = K\sum\widetilde A_i\dot
q_i - \lambda p\) the lumped model had.

The Neumann normalization gives \(\Lambda_c\) in closed form,

\[
\Lambda*{mn} = L \cdot \{\pi, 2\pi\} \cdot
\frac{a^2}{2}\left(1 - \frac{m^2}{j'^2*{mn}}\right)J*m(j'*{mn})^2,
\]

positive for every admissible mode because \(j'\_{mn} > m\), and equal to the
cavity volume for the uniform mode.

`Cavity.StiffnessScale` is \(s\) and multiplies every \(K_c\) alike, so the
rigid ceiling keeps its meaning. Unlike the rest of the section it is fitted
rather than derived, and it is fitted to the wrong instrument — see
[Why the air spring is scaled, and what it was scaled
to](#why-the-air-spring-is-scaled-and-what-it-was-scaled-to).

### Why the air spring is scaled, and what it was scaled to

\(\rho c^2/V\) is the bulk stiffness of a **rigid, sealed** enclosure driven by
**pistons**, which is why `Cavity.StiffnessScale` is a fraction rather than a
free gain: 1 is the physical ceiling, not a neutral default. The shipped value is
0.083, a factor of twelve below it, and it is fitted rather than derived.

Four mechanisms were offered for a gap that size — vent leakage, the one-mode
lumped reduction, shell flex, and the head's non-piston mode shape. **All four
are eliminated**, and between them they eliminate the gap rather than explain it.
PLAN.md's [N4](../PLAN.md) carries the summary; this section carries the
arithmetic.

#### The vent diverts a few per cent

A vent is a Helmholtz port, so it is a
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
at the fundamental and less at every higher mode, where a twelvefold reduction
would need roughly **92 %** of the flow diverted. The vent is not the mechanism.

#### Shell flex is arithmetically impossible

Uniform internal pressure is carried in hoop **membrane** stress, not in bending.
The wall's radial expansion is \(u = pa^2/(Eh)\), so \(\Delta V = 2\pi a L\,u\)
and the shell's compliance is

\[
C_\mathrm{shell} = \frac{2\pi a^3 L}{Eh}.
\]

With \(a = 0.1524\) m, \(L = 0.2\) m, \(h = 6\) mm maple and \(E \approx 10\) GPa
that is \(7.4\times10^{-11}\ \mathrm{m^5/N}\), against

\[
C_\mathrm{air} = \frac{V}{\rho c^2}
= \frac{1.459\times10^{-2}}{1.204 \times 343^2}
= 1.03\times10^{-7}\ \mathrm{m^5/N}.
\]

The shell is **1400× stiffer than the air it encloses**: it adds about **0.07 %**
of compliance, not a factor of twelve. A softer effective \(E\) does not rescue
the argument either, because uniform pressure does not excite ovalization — the
only compliant shell deformation — and closing the gap this way would need
\(E \approx 7\) MPa, which is rubber rather than maple.

#### The non-piston mode shape is already applied

A head's axisymmetric shape is not a flat plate, but that is a correction the
model already carries rather than one it is missing. The coupling coefficient
_is_ the signed swept area \(A_{0n} = 2\pi R^2 J_1(z_{0n})/z_{0n}\), so it
already contains the net-volume factor \(2J_1(j_{0n})/j_{0n} = 0.4318\) at
\(n = 1\), and it enters the stiffening **squared**: \(K A^2/m =
9.707\times10^6 \times (3.1503\times10^{-2})^2 / 6.883\times10^{-3} =
1.3997\times10^6\), which reproduces the analytic single-head stiffening the
review quotes. It cannot also be a factor still to be applied; earlier drafts of
this document counted it twice.

#### There is no factor of twelve — the ceiling was right and the target was wrong

The fourth candidate, the one-mode lumped reduction, is exonerated separately
below. With all four gone, what is left is the fit's anchor.

0.083 was fitted to reproduce a doublet ratio of **1.16 measured on a snare
drum**:
[Fischer, _Modal Analysis of a Snare Drum_, Illinois 2014](https://courses.physics.illinois.edu/phys406/sp2017/Student_Projects/Spring14/Matthew_Fischer_Physics_406_Final_Project_Sp14.pdf)
found 186 Hz with one head and 215 Hz after adding the resonant head at unchanged
tuning. `cavity_split_test.go` says as much in its first line, and its
\([1.10, 1.20]\) acceptance window is a snare's. A snare is a **leakier enclosure
than a tom** — vent, snare beds, throw-off slots — and recomputing Fischer's
instrument from first principles over-predicts its split by **4.3–8.7×**, which
is what a wrong anchor looks like rather than what wrong physics looks like.

Two of the tom measurements the fit should have used:

- Fletcher & Rossing §18.4 p. 608 relays Bork & Meyer's **32 cm two-headed tom**
  with the (0,1) doublet at **101 and 191 Hz — a ratio of 1.891**.
- Gärder (2005) prints single-head and two-head spectra for a 14" and a 15" tom.
  Reading the lower branch off them gives shifts of **+28.5 %** and **+37.2 %**
  when the resonant head is fitted, against the snare's +16 %. Those two
  percentages are **my own arithmetic from his figures and are not stated in the
  thesis** — the only shift he gives as a number is a pitch glide of almost 8 %
  on a hard strike to a 14" tom (§C.4.3), which is a different quantity
  altogether. Recompute from the source before committing either to code, and do
  not cite them as measurements of his.

**How Fischer's two numbers map onto the doublet is itself ambiguous**, and this
document used to assert one reading. 186 → 215 was taken here as
\(f_\mathrm{upper}/f_\mathrm{lower}\); but the interlacing argument below says
the lower branch is pinned between the two heads' uncoupled fundamentals and
cannot carry a 16 % rise, so 215 could equally be the upper branch against a
_single-head_ 186 that is neither branch. Resolving it needs three frequencies —
\(f_\mathrm{single}\), \(f_\mathrm{lower}\), \(f_\mathrm{upper}\) — which is why
PLAN's N14 asks for all three.

#### Why the retarget is nonetheless not clean

Retargeting is [P10/N4](../PLAN.md), and it is no longer blocked on a physical
capture: the licensed reference set is an **8" × 8" tom of known geometry** with
stated head gauges, so its split can be _computed_ rather than fitted. See
[`physical-objective-validation.md`](physical-objective-validation.md) for how
that reference was characterised. Four costs are already known, and none of them
is small:

- **The parameter saturates rather than lands.** \(s = 0.083\) gives 1.177 exact
  / 1.151 rendered; \(s = 1\) gives **1.841 exact / 1.830 rendered**, still 2.7 %
  _below_ Bork & Meyer's 1.891. Re-measured 2026-08-01 to settle a discrepancy
  with a since-deleted row of this document, which read 290.0 Hz and a ratio of
  1.867: `TestRigidCavityStiffnessOverpredictsTheSplit` renders **155.3 / 284.2 Hz,
  ratio 1.8302**, so 1.830 is current and 1.867 is void. Hitting the measurement needs \(s > 1\), which `Validate`
  forbids and which is above the rigid ceiling. Shipping \(s = 1\) therefore
  ships a pinned parameter — itself a finding, because something else is about
  3 % short.
- **The interleaving constraint is unmet, and it is the stronger one.** The same
  passage puts the **(1,1) at 179 Hz, _between_ the two (0,1) branches**. The
  model does not reproduce that ordering at any stiffness scale.
- **The glide gets diluted.** The cavity adds stiffness the strike does not
  modulate, so raising \(s\) shrinks the Berger shift: coupled loud-hit glide
  falls from ~52 cents at 0.083 to ~37 at \(s = 1\), against tom measurements of
  130–165. Retarget both together or neither.
- **Three tests encode the snare target.**
  `TestDefaultCavitySplitMatchesMeasuredDrums` and
  `TestRigidCavityStiffnessOverpredictsTheSplit` both invert, and
  `TestDefaultCavityLeavesNoPartialWhereTom2Belongs` guards 200–235 Hz, which is
  exactly where the upper branch sits at \(s = 0.30\)–\(0.40\). A retarget has to
  go **all the way**; the intermediate values are the worst.

Two consequences of the rank-one coupling bound what any choice of \(s\) can do:

- Only the **stiffened** branch moves appreciably. Eigenvalue interlacing keeps
  the lower branch between the two heads' uncoupled (0,1) frequencies, so the
  quantity to fit is the separation between the branches, not the absolute
  position of the audible one.
- Every \(m>0\) mode has zero swept area, so \(s\) has no effect on them through
  the uniform mode at all — exactly. It does act on them through the transverse
  modes, which is what P9/M2 added; but those are resonators rather than springs,
  so they change the shape of the response near their own frequencies rather than
  the doublet the fit is made against.

The two-head coupling loss that damps the (0,1) is a separate mechanism,
calibrated in [`physical-calibration.md`](physical-calibration.md), and is
unaffected by any of this — the lumped cavity is not where that loss comes from.

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
\(\sum*c P_c\sum_i\widetilde C*{ic}\dot q_i\) and the cavity equation gains
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

- K*c\sum_b\sum_i \frac{\widetilde C*{ic}\widetilde C\_{ib}}{m_iD_i}P_b
  = \frac{2P_c^{\text{old}}}{\Delta t} + \omega_c H_c^{\text{old}}
- K*c\sum_i \widetilde C*{ic}u_i,
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

One candidate explanation for the gap between the fitted \(s\) and the rigid
ceiling was that the one-mode reduction itself mis-set the compliance: a single
uniform-pressure state stands in for the whole enclosed
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
Whatever \(s\) is retargeted to, the transverse modes will not change the answer.

Together with the vent, shell-flex and mode-shape arithmetic above, that closes
the last of the four candidates — which is why the section above concludes there
was never a factor of twelve to explain.

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
  [PLAN's](../PLAN.md) P9/M2 confirm criterion, and a test asserts it.
- **The microphone barely notices, in broadband terms.** RMS over a second moves
  from 0.057609 to 0.057755 and the peak from 0.8954 to 0.8932. The configured
  pickup is on the batter side and the resonant head's own radiation is
  deliberately not summed into it, so the new energy reaches the listener only as
  the reshaping of the batter response above. That framing understates it: the
  next section measures the same change as a **partial** rather than as broadband
  energy, and as a partial it is not subtle.

## The same criterion, measured in the rendered output

Everything above is read off the model's internal state or off the continuous
reference solve. That is the honest place to measure a mechanism, but it is not
where M2's criterion is written: the criterion says a **partial** appears, and a
partial is a property of the signal that leaves the pickup. This section runs the
criterion there, and it is worth separating because the two can disagree — a
cavity mode can be plainly excited and still be inaudible if nothing carries it
into the microphone.

It came up because a P9/M1 run noticed something it was not looking for. Rendering
the shipped default at Quality High and velocity 1.0, 1.2 s at 44.1 kHz, and
extracting partials with `internal/physical/match` — the same path
`cmd/fit-physical -report-only` takes — the table contains a partial at
**664.38 Hz** whose **T60 is 0.745 s** against roughly 0.23 s for every head mode
in that band, with no head mode within 27 Hz of it. It rings three times as long
as anything around it and sits 13 cents from the \((1,1)\) cavity mode at
659.52 Hz.

Measurements below are that render unless stated otherwise, with the spectrum
taken over the 0.05–0.85 s sustain. Positions are read as local maxima of that
spectrum rather than from the sixteen-entry partial table, because the table is
capped at sixteen and floored at −42 dB and a partial can leave it by being
crowded out rather than by being absent — which is exactly what happens in three
of the sweeps below.

**Sound speed is the discriminator that matters**, because nothing about either
head depends on \(c\), and because radius does not discriminate at all: a head
mode goes as \(\sqrt{T}/a\) and a cavity mode as \(c/a\), so **both families move
as \(1/a\)** and the radius sweep can only ever be a consistency check.

| \(c\) (m/s) | \((1,1)\) predicted | observed | \((2,1)\) predicted | observed | observed\(\_{(2,1)}/c\) |
| ----------- | ------------------- | -------- | ------------------- | -------- | ----------------------- |
| 280         | 538.38              | 545.13   | 893.09              | 895.09   | 3.1967                  |
| 300         | 576.84              | 582.59   | 956.88              | 959.38   | 3.1979                  |
| 320         | 615.29              | 618.04   | 1020.68             | 1024.93  | 3.2029                  |
| 343         | 659.52              | 664.27   | 1094.04             | 1096.29  | 3.1962                  |
| 360         | 692.21              | 704.46   | 1148.26             | 1151.51  | 3.1986                  |
| 380         | 730.66              | 739.91   | 1212.05             | 1214.55  | 3.1962                  |
| 400         | 769.12              | 777.12   | 1275.84             | 1277.59  | 3.1940                  |

The last column is the test: if the partial is an air mode, observed\(/c\) is a
constant. It is, to **±0.09 %** over a 43 % change in sound speed. The \((1,1)\)
column is noisier (±0.6 %) for a reason worth keeping: it sits inside the head's
\(m = 1\) thicket and its peak is pulled a hertz or two toward whichever head mode
is nearest, while the \((2,1)\) at 1094 Hz has no head mode near it at all.

| radius | \(a\) (m) | \((2,1)\) predicted | observed | observed \(\times a\) |
| ------ | --------- | ------------------- | -------- | --------------------- |
| ×0.80  | 0.12192   | 1367.55             | 1366.80  | 166.64                |
| ×0.90  | 0.13716   | 1215.60             | 1216.35  | 166.85                |
| ×1.00  | 0.15240   | 1094.04             | 1096.29  | 167.08                |
| ×1.10  | 0.16764   | 994.58              | 997.58   | 167.24                |
| ×1.20  | 0.18288   | 911.70              | 915.45   | 167.42                |

observed \(\times a\) is constant to ±0.23 %, against a predicted
\(cj'\_{21}/2\pi = 166.72\).

Head tension is the kill condition, and it does not fire. The prediction does not
move at all here — nothing about the enclosed air knows the head tension — so the
whole table is one number repeated:

| batter tension | uncoupled batter (0,1) | \((1,1)\) observed | \((2,1)\) observed |
| -------------- | ---------------------- | ------------------ | ------------------ |
| ×0.64          | 120.1 Hz               | 659.52             | 1097.54            |
| ×0.81          | 135.1 Hz               | 662.77             | 1097.04            |
| ×1.00          | 150.1 Hz               | 664.27             | 1096.29            |
| ×1.21          | 165.1 Hz               | 671.27             | 1097.29            |
| ×1.44          | 180.1 Hz               | 664.02             | 1095.04            |

The \((2,1)\) moves by **2.5 Hz, 0.23 %**, while every head mode is retuned across
a range of ten semitones. The \((1,1)\) wanders by up to 12 Hz for the reason
above — the level of that partial collapses from −6.3 to −53.1 dB across this
sweep, because the drive depends on an \(m = 1\) head mode sitting near it, and a
weak peak in a dense band is read with correspondingly less confidence. Its
position is not tracking \(\sqrt{T}\) in either direction; the ×1.44 row would
have to read 792 Hz if it were.

A fourth sweep is worth having because it is free and it separates a transverse
mode from anything else in the enclosure: the family is axially uniform, so its
frequency does not depend on the shell **depth** at all. At 0.14, 0.20 and 0.28 m
the partial sits at 666.02, 664.27 and 663.02 Hz — moving 0.45 % for a depth
change of two to one, in the wrong direction for a \(c/2L\) axial mode, which
would have run 1225, 858 and 613 Hz.

### What actually creates it, and what merely reveals it

The original observation compared nonlinear mode coupling on against off, and the
partial was present only with it on. That reading was wrong, and the 2×2 says so:

|                     | lumped cavity                       | modal cavity                         |
| ------------------- | ----------------------------------- | ------------------------------------ |
| NL coupling **off** | no local maximum; −77.4 dB at 659.5 | **664.52 Hz, T60 0.738 s**, −14.6 dB |
| NL coupling **on**  | no local maximum; −70.1 dB at 659.5 | **664.27 Hz, T60 0.752 s**, −6.3 dB  |

The **modal cavity creates the partial**; the nonlinear coupling adds about 8 dB
of drive to it. What the original comparison actually saw was the detector's
floor: in the sixteen-entry table at −42 dB the partial appears in the
modal/coupling-on cell alone, so switching the coupling off looked like removing
the partial when it only pushed it under the threshold. The lesson is about the
measurement path rather than the model — a −42 dB relative floor on a
sixteen-partial table is not a test of existence.

### The second half: output through the cavity path

M2 also asks whether \(m = 1\) head modes acquire measurable **output** through
the cavity once `Pickup.NearFieldScale` is zero. The measurement needs care,
because `observe()` never sums the resonant head's radiation into the pickup at
all — it is a batter-side microphone and the resonant head radiates out of the
other end of the shell. That turns out to be an advantage: **nothing at a
resonant-only frequency can reach the output by radiating**, so if output appears
there, the batter head is being driven at that frequency, and the enclosed air is
the only thing joining the two.

With `Resonant.AxisymmetricOnly` forced off in both arms so the modal banks match
and only the cavity differs:

| frequency  | what it is                   | lumped | modal | change    |
| ---------- | ---------------------------- | ------ | ----- | --------- |
| 659.52 Hz  | cavity \((1,1)\)             | −73.0  | −36.5 | **+36.5** |
| 1094.04 Hz | cavity \((2,1)\)             | −83.7  | −59.5 | **+24.2** |
| 685.25 Hz  | resonant \(m{=}1,n{=}3\) cos | −92.0  | −68.6 | **+23.4** |
| 687.31 Hz  | resonant \(m{=}1,n{=}3\) sin | −77.7  | −64.7 | **+13.0** |
| 566.69 Hz  | resonant \(m{=}2,n{=}2\) cos | −83.3  | −83.3 | 0.0       |
| 568.39 Hz  | resonant \(m{=}2,n{=}2\) sin | −81.5  | −81.5 | 0.0       |

dB relative to the strongest sustain peak of each render, nonlinear coupling off.
The \(m = 1\) family that **straddles** the transverse mode gains 13 to 23 dB; the
\(m = 2\) family gains nothing measurable, which is consistent rather than
disappointing — the \((2,1)\) cavity mode at 1094 Hz is 24 dB weaker and its
nearest resonant \(m = 2\) modes are 90 Hz away. Read as a band rather than
bin by bin, on the shipped 48 kHz default, the 679–693 Hz band containing the
resonant \((1,3)\) pair gains **+5.6 dB**; the smaller figure is the honest one
for the shipped pickup, because the batter head's own \(m = 1\) modes keep
radiating with the near field removed and set a floor under the comparison.

That last point contradicts, mildly, what `Pickup`'s documentation claims for a
zero `NearFieldScale` — "the output is very nearly the axisymmetric modes alone".
Measured, the batter's \((1,1)\) still sits 4.8 dB below the strongest peak with
the near field removed. The far-field weight really does suppress \(m > 0\), but
not to the point where a zero scale isolates the axisymmetric family.

**Negative control.** Under the lumped cavity the only coupling coefficient is the
swept area, identically zero for every \(m > 0\) mode, so nothing can drive an
\(m = 1\) air mode and there is none to drive. The lumped column above is what
that looks like, and the level at the transverse frequency is 48 dB below the
modal case on the shipped default. `TestLumpedCavityHasNoTransversePartial` holds
that.

Tests for all of this are in
[`cavity_transverse_test.go`](../internal/physical/cavity_transverse_test.go),
alongside the internal-state versions in `cavity_modes_test.go`.

### The confirmation is a weak-coupling result

One thing sharpens the reading and one thing limits it, and the limit is the same
parameter both times: `Cavity.StiffnessScale`.

A cavity mode's frequency \(cj'\_{mn}/2\pi a\) does not contain \(s\) at all — \(s\)
scales the modal stiffness \(K_c\), which sets how hard the air pushes back, not
where it resonates. But the **coupled** mode is loaded by the heads, and that load
does scale with \(s\). Measured on the same render:

| \(s\)             | coupled \((1,1)\) partial | shift from 659.52 Hz | level                           | \(m{=}1\) band gain |
| ----------------- | ------------------------- | -------------------- | ------------------------------- | ------------------- |
| 0.083 (shipped)   | 664 Hz                    | +0.7 %               | 0.0 dB (loudest in 400–1200 Hz) | +10.3 dB            |
| 0.3               | 704 Hz                    | +6.8 %               | −20.6 dB                        | —                   |
| 1 (rigid ceiling) | 753 Hz                    | +14.2 %              | −4.1 dB                         | +5.4 dB             |

Two consequences. First, the clean confirmation above is a **weak-coupling**
result: at \(s = 0.083\) the partial sits within 1 % of the bare air-mode formula,
which is what makes "it is where \(cj'/2\pi a\) says" a sharp statement. At \(s =
1\) it is a genuinely hybridised head-and-air mode 14 % above the air formula, and
its position acquires a mild tension dependence of its own (the ratio drifts from
1.142 to 1.090 across a ×0.7–×1.4 retune). The mechanism is the same either way;
the _discrimination_ is sharp only while the coupling is weak. Second, the
tolerances in `cavity_transverse_test.go` are tied to the shipped \(s\), and the
file says so.

**And \(s\) is due to be retargeted**, for the reasons in
[§"There is no factor of twelve"](#there-is-no-factor-of-twelve--the-ceiling-was-right-and-the-target-was-wrong)
above; the sharpness of M2's confirmation is one of the costs listed there.

The M2 result does not depend on how that lands. The partial tracks \(c\) and
\(1/a\) and ignores \(T\) at \(s = 1\) as well as at \(s = 0.083\); only the size
of the coupled offset changes.

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
M*{cb}=\sum_i \frac{\widetilde C*{ic}\widetilde C*{ib}}{D_i},
\qquad
S_c=\sum_i \widetilde C*{ic}u_i,
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
- the cavity mode table against the analytic \(c\,j'\_{mn}/2\pi a\) series at the
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
- the fitted (0,1) split inside a 10–20 % band, no partial left within 10 dB of
  the fundamental where the (2,1) belongs, and the rigid stiffness overshooting
  that band. These three encode the **snare** target and are the tests P10/N4
  has to invert; they are listed here because they currently gate CI, not
  because the band they assert is a tom's;
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
to 28 because \(m \le 2\) modes are no longer unreachable.

`PhysicalDrum.ResonantModeLimit` now gives that head its own budget, so the
quality tier no longer sizes it. It was split for the structural reason rather
than this one — a 96-slot allowance chosen for the struck head has no business
sizing a head that is only ever driven through the air — and the default of 24 is
chosen against the coupling mechanism rather than against a clock; see
`DefaultResonantModeLimit` and
[`physical-hybrid.md`](physical-hybrid.md#spending-the-reclaimed-budget). It takes
the shipped Standard voice from 124 oscillators to 120 and High from 198 to 184,
which is 3.7 % and 10.7 % of `js/wasm` render time. The transverse cavity's cost
is therefore still mostly unpaid, and paying it means optimizing the mode bank,
not shrinking it further.

Still excluded: axial cavity order, shell modes, vents, non-axisymmetric leakage,
and empirical head-to-head cross-coupling, until measurements or a
higher-fidelity reference show that they are necessary.
