# Physical drum modelling research

Research date: 2026-07-29. Survey re-run 2026-08-01 — see
[The 2026-08-01 re-survey](#the-2026-08-01-re-survey-a-negative-result), whose
conclusion is negative and is the more useful half of this document.

## Decision

Build the first real-time physical voice as a **double-headed cylindrical tom**
using modal synthesis. Treat it as a new synthesis path alongside the existing
procedural voices; do not replace or silently reinterpret them.

A tom is the smallest useful model that exposes the hard parts of a real drum:

- a circular, tensioned batter head with many inharmonic modes;
- a finite-area strike at an arbitrary position;
- frequency- and mode-dependent damping and radiation;
- a resonant head coupled to the batter head through enclosed air;
- amplitude-dependent tension and the resulting downward pitch glide.

A snare is a poor first milestone. Its characteristic mechanism is distributed,
intermittent, nonlinear contact between several one-dimensional wires and the
resonant membrane, on top of the two-head/air system. Bilbao's full model uses
two 2-D membranes, a 3-D acoustic field, 1-D snares, and collision solves. That
is an excellent reference model but not a sensible first browser voice.

## What makes a real drum difficult

An ideal membrane is only the starting point. The audible instrument is a
coupled vibroacoustic system:

1. **Heads.** Tension, surface density, small bending stiffness, imperfect
   clamping at the bearing edge, non-uniform tension, and frequency-dependent
   material loss determine the modes.
2. **Excitation.** Stick or mallet mass, hardness, velocity, contact area,
   position, and finite contact time determine which modes receive energy.
3. **Air.** Internal pressure couples the heads. Exterior air both loads the
   heads and radiates sound; radiation efficiency differs strongly between
   modal shapes.
4. **Shell and hardware.** The shell, rims, lugs, and mounting split otherwise
   degenerate modes and add lossy structural paths.
5. **Nonlinearity.** A large displacement stretches a head, temporarily
   increasing its tension and modal frequencies. As the hit decays, those
   frequencies glide down.
6. **Snare contact.** Snare wires repeatedly lose and regain contact with the
   resonant head. This is a spatially distributed collision, not filtered
   noise.
7. **Observation.** A membrane displacement at one point is not the sound at a
   microphone. Radiation, listening position, shell vents, room response, and
   microphone response matter.

The model therefore needs an explicit fidelity ladder. Trying to implement all
of these mechanisms in one pass would make it impossible to tell whether a bad
sound comes from physics, discretisation, parameter estimation, radiation, or
the browser pipeline.

## Candidate numerical approaches

| Approach                               | Strengths                                                                                                       | Weaknesses                                                                                             | Role here                                        |
| -------------------------------------- | --------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------ | ------------------------------------------------ |
| Modal synthesis                        | Small real-time state; analytic circular modes; natural strike and pickup position; easy per-mode damping       | Circular/linear assumptions; nonlinear and contact coupling need reduced models                        | **Primary real-time method**                     |
| Finite differences (FDTD)              | Direct PDE implementation; local nonlinearities and collisions; energy methods give strong stability guarantees | A 3-D air grid dominates cost; numerical dispersion and CFL limits; difficult at audio rate in Go/WASM | Offline reference and later research path        |
| Digital waveguide mesh                 | Local, real-time wave propagation; established drum work                                                        | Curved boundaries and numerical dispersion require correction; 3-D coupling remains costly             | Alternative experiment, not first implementation |
| FEM/BEM                                | Flexible geometry, shell, bearing edge, and radiation                                                           | Heavy mesh/tooling; unsuitable as the browser-time integrator                                          | Offline mode/radiation data generator            |
| Measured modal/transfer-function model | Most direct path to matching one real instrument                                                                | Depends on measurements and no longer predicts a new drum from physical parameters alone               | Validation and optional calibration layer        |

The practical architecture is hybrid: physical modal dynamics in the audio
loop, with coefficients optionally calibrated from offline FDTD/FEM or
recordings.

## The 2026-08-01 re-survey: a negative result

The table above was written before anything existed. The survey was re-run on
2026-08-01, after the model had been built, calibrated and measured, to answer a
narrower and much more useful question: **does any more sophisticated formulation
target the defect that survives measurement?**

That defect is specific.
[`physical-objective-validation.md`](physical-objective-validation.md) budgets the
residual and finds that most of what the objective reports is not reproducible;
what remains is a **damping-distribution** problem — the spectrum evolves wrongly
over time, and the largest single contributor is an under-damped coupled `(0,1)`
doublet that rings far longer than the reference's. That is a question about
*which mode loses energy how fast*, not about spatial resolution, geometry
fidelity or state count.

**No surveyed method addresses it.** Recording that is worth more than the
alternatives it rejects, because each of these had been proposed at least once.

| Formulation | Would it reach the defect? | Why not |
| --- | --- | --- |
| FDTD membranes + 3-D air field (Bilbao; Torin/Hamilton/Bilbao) | No | Dead twice over. On **cost**: the CFL condition at audio rate forces a grid step of ~1.24 cm, and Bilbao & Webb needed GPGPUs for exactly this class of model — it is not a `js/wasm` audio-thread candidate at any quality tier. On **fittability**: 10⁴–10⁶ state variables, none of which is a parameter you can fit to a recording, so it moves the damping question from "17 parameters" to "a material loss law plus a mesh". |
| Digital waveguide mesh (Van Duyne & Smith; Laird) | No | Same CFL/dispersion economics as FDTD with additional boundary-fitting error on a circular rim. Damping enters through wall filters that are *less* directly parameterised per mode than the current loss law, not more. |
| Functional Transformation Method | No — it is what we already run | For a separable circular membrane the FTM's Sturm–Liouville transform **is** the Fourier–Bessel modal expansion in `internal/physical/modes.go`. Adopting it would rename the implementation, not change it. Its genuine advantage is over geometries where the modes are not analytic, which is not this one. |
| FEM/BEM | No, and it was never a real-time candidate | Useful offline for shell, bearing edge and radiation, exactly as the table above says. It supplies mode shapes and radiation efficiencies, not the head-pair damping split, and there is no measured shell mobility to fit it against — see [`physical-real-instrument-departures.md`](physical-real-instrument-departures.md). |
| A resolved 3-D interior air field instead of the lumped/modal cavity | No, and this is provable | Non-uniform cavity modes have **zero net volume**, so they do not change the quasi-static compliance the two heads see: it is exactly `ρc²/V` at one cavity state and at six. The coupling stiffness — the thing the doublet's frequency and damping both depend on — is unaffected. PLAN.md §N4 and `physical-cavity.md` record the arithmetic; the transverse modes the model *does* now carry were added for their own audible partial, not for the coupling. |
| Port-Hamiltonian formulation | No | It is a restatement of the passivity bookkeeping the model already performs: the discrete-gradient Berger update and the rank-one cavity solve both carry explicit energy arguments and tests. Real value if the model grew several interconnected nonlinear subsystems; none today. |
| Mass-interaction (CORDIS-ANIMA lineage) | No | A different discretisation of the same physics, with parameters (per-mass damping) that are further from measurable quantities than the modal loss law, not closer. |
| Differentiable / learned modal synthesis (Zheleznov et al.) | It upgrades the **search**, and search is not the ceiling | The clearest of the negatives. Two independent fitting runs from diverse seeds already agree term for term and land flat to four digits — a better optimiser cannot improve on a fit that already reaches the same optimum from anywhere. The binding constraints are the objective's reproducibility and the model's missing damping structure, both upstream of the gradient. |

Two things the survey did surface as worth having, and both are estimation rather
than simulation:

- **Modal-decay estimation done properly.** The repository fits decay by
  log-linear regression on a heterodyned envelope with a hard −45 dB truncation.
  Karjalainen et al. exists to replace exactly that, with an explicit
  exponential-plus-noise-floor model; Ege, Boutillon & David demonstrate subband
  ESPRIT resolving *twin modes* — which is the case the current
  `MinSeparationHz = 15` structurally cannot resolve — and Badeau, David &
  Richard supply the order-selection criterion. PLAN.md §P10/N2.
- **Identifiability analysis.** The converged fits show the textbook sloppy-model
  signature; Gutenkunst et al. names it and Raue et al. gives the profile-likelihood
  procedure that separates structural from practical non-identifiability. PLAN.md
  §P10/N6.

**The design consequence.** Effort belongs in the damping structure, the
estimator and the objective — not in a more faithful integrator. A model that
resolves the air in three dimensions and still puts the wrong T60 on the
fundamental sounds exactly as wrong as this one, and costs four orders of
magnitude more to run.

## Recommended reduced model

### 1. Circular head modes

For transverse displacement \(w(r,\theta,t)\), start with the lossy
membrane/plate equation

\[
\begin{aligned}
\mu w_{tt} + d_0 w_t - d_2\Delta w_t
&+ D\Delta^2w - T\Delta w = f,
\end{aligned}
\]

where \(\mu\) is surface density, \(T\) tension per unit length, \(D\) bending
stiffness, and \(d_0,d_2\) control approximately frequency-independent and
frequency-dependent loss.

For a circular head of radius \(R\) with an ideal fixed rim, expand the field in
Fourier-Bessel modes:

\[
\phi_{mn}^{c,s}(r,\theta)
= J_m(\alpha_{mn}r/R)\{\cos(m\theta),\sin(m\theta)\},
\]

where \(\alpha_{mn}\) is the \(n\)-th positive zero of \(J_m\). The approximate
linear angular frequency is

\[
\begin{aligned}
\omega_{mn}^{2}
&= \frac{T}{\mu}\left(\frac{\alpha_{mn}}{R}\right)^2 \\
&\quad + \frac{D}{\mu}\left(\frac{\alpha_{mn}}{R}\right)^4.
\end{aligned}
\]

Each retained modal coordinate is a damped second-order oscillator:

\[
\ddot q_i + 2\zeta_i\omega_i\dot q_i+\omega_i^2q_i
= b_i f_\text{strike}+c_i p_\text{cavity}.
\]

Use a stable two-pole discrete resonator or an exact state-transition update.
Precompute mode shapes, strike/pickup weights, coefficients, and normalisation;
the per-sample loop should be a flat structure-of-arrays iteration with no
allocation or transcendental functions.

Both sine and cosine members of \(m>0\) mode pairs are required. Their weights
make strike angle and pickup angle audible. A small deterministic frequency
split is a later, controlled way to represent tension/rim asymmetry.

### 2. Strike

Do not begin with an impulse added directly to every resonator. Use a finite
spatial and temporal contact force:

- a normalized Gaussian or raised-cosine footprint on the head;
- modal input weights obtained from the footprint projected onto each mode;
- first milestone: a short, band-limited force pulse parameterized by velocity
  and hardness;
- later milestone: a mallet mass plus Hertz-like unilateral contact
  \(F=K[\eta]_+^\alpha\), solved with a bounded iteration count.

This makes radius, angle, hardness, and velocity physical controls. It also
prevents an unrealistically broadband single-sample excitation.

### 3. Loss and radiation

One global exponential envelope is not adequate. Derive each resonator's pole
radius from its decay time:

\[
r_i = \exp\left(-\frac{1}{f_s\tau_i}\right).
\]

Start with a two-parameter decay law fitted over modal frequency, then allow
optional per-mode residual corrections. Keep structural loss and radiation loss
separate in the data model even if their sum sets the first implementation's
pole radius.

The output should be a weighted sum of modal velocity (and, experimentally,
acceleration) rather than raw displacement. Radiation weights depend on modal
shape: axisymmetric modes radiate differently from dipole/quadrupole-like
modes. A compact radiation filter or measured transfer function can follow the
modal sum. Keep "head pickup" and "radiated microphone" outputs available in
tests so radiation mistakes are diagnosable.

### 4. Two heads and enclosed air

Instantiate separate modal banks for batter and resonant heads. The first cavity
model is a lumped spring/damper driven by the heads' swept volume. For an ideal
circular basis, this couples the axisymmetric modes because non-axisymmetric
modes integrate to zero over the head.

This reduced model should reproduce in-phase/out-of-phase coupled modes and the
strong dependence of the lowest modes on the tuning of both heads. Add a small
empirical cross-coupling matrix only after comparison with measurements or a
higher-fidelity reference demonstrates that it is needed.

Do not use `algo-pde`'s complex Helmholtz solve as the audio-rate state update:
it computes a steady-state driven field at one frequency, not transient wave
evolution. It is useful offline to inspect cavity resonances, generate reference
transfer functions, and test reduced coupling assumptions.

### 5. Nonlinear tension modulation

After the linear coupled model is stable and calibrated, add a Berger-style
reduced tension term. Compute a scalar extension/energy measure from modal
coordinates and use it to shift the effective tension shared by the modes.
This produces the characteristic high-at-attack, downward frequency glide
without a full von Karman grid.

The update must have a discrete energy or passivity argument, a conservative
parameter bound, and an oversampled/reference test. A naive time-varying
frequency change can inject energy and become unstable.

### 6. Non-ideal shell and rim

Model these only after the heads/cavity path is credible:

- deterministic splitting of degenerate mode pairs;
- a small shell modal bank coupled weakly to both heads;
- bearing-edge compliance or loss as frequency-dependent modal corrections;
- optional vent/radiation resonance.

This preserves physical interpretability while capturing the "real object"
departures from a mathematically perfect drum.

### 7. Snare extension

Provide two progressively more faithful options:

1. a reduced contact/noise model driven by resonant-head displacement and
   velocity, sufficient for an interactive musical voice;
2. a research implementation with several 1-D strings and distributed
   unilateral collision against the resonant head, following the
   energy-conserving FDTD literature.

The reduced model must not be labelled as a full physical snare simulation.

## Repository fit

### `algo-drum`

- Keep the current `internal/drum` procedural `Voice` implementations and
  persisted parameters unchanged.
- Add a distinct physical-model package and explicit model selection. Version
  persistence before storing the selection or physical parameters.
- Prototype one physical tom voice and an audition/visualisation panel before
  routing sequencer tracks through it.
- Reuse the existing Worker → `AudioWorklet` pipeline, 512-sample rendering
  chunks, limiter, error recovery, and test gates.

### `algo-dsp`

Useful now:

- filters for radiation/microphone response and anti-alias conditioning;
- limiter/master processing already used by `algo-drum`;
- convolution/correlation for optional transfer functions and measurement
  alignment;
- logarithmic sweep and deconvolution tools for calibrating a real drum;
- analyzers for validation tooling.

Any generic modal-resonator bank should first live in `algo-drum`. Move it to
`algo-dsp` only after the drum implementation reveals a stable reusable API.

### `algo-fft`

Use for offline spectral analysis, transfer-function estimation, regression
metrics, and any convolution reached through `algo-dsp`. A modal circular head
does not need an FFT in its per-sample loop.

### `algo-pde`

The current library is a strong elliptic Poisson/Helmholtz solver, including a
complex damped acoustics example. Use it for frequency-domain reference work,
not as though it were already a transient membrane solver. A full FDTD drum
would require new time-domain wave/membrane/plate solvers, interface coupling,
absorbing boundaries, and energy/stability tests.

One repository issue must be resolved before adding it as a dependency:
`CWBudde/algo-pde` currently declares the module path
`github.com/MeKo-Tech/algo-pde`.

## Parameter and preset strategy

Use SI units in the model and expose a smaller musical control surface:

| Internal parameter                 | Initial UI control         |
| ---------------------------------- | -------------------------- |
| radius, shell depth                | diameter, depth            |
| batter/resonant tension            | batter tune, resonant tune |
| surface density, bending stiffness | head type preset           |
| \(d_0,d_2\) / modal decay fit      | damping                    |
| strike radius/angle                | hit-position control       |
| contact time/stiffness             | hardness                   |
| strike velocity                    | velocity                   |
| cavity loss/coupling               | air/coupling               |
| nonlinear coefficient              | pitch-glide amount         |
| pickup/radiation weights           | microphone position        |

Ship named, documented presets with physical dimensions and provenance. Avoid
claiming that a preset represents a specific commercial drum until it has been
measured.

## Validation

Validation must be layered so a plausible sound cannot hide incorrect physics.

1. **Analytic unit tests**
   - Bessel zeros and mode ordering;
   - modal frequencies against the formula above;
   - center/off-center strike selection rules;
   - sine/cosine degeneracy before asymmetry is enabled.
2. **Numerical invariants**
   - unforced, lossless energy remains bounded;
   - damped energy is non-increasing within a documented tolerance;
   - output is finite and deterministic;
   - render performs zero steady-state allocations.
3. **Coupling tests**
   - zero cavity coupling equals two independent heads;
   - coupled axisymmetric modes split into in-phase/out-of-phase pairs;
   - energy transferred between heads is conserved in the lossless case.
4. **Reference comparisons**
   - compare low modes and impulse responses with an offline FDTD/FEM or
     `algo-pde` frequency-domain reference where applicable;
   - compare mode frequencies, decay times, glide trajectories, and spectra to
     recorded hits at several velocities and strike radii.
5. **Browser performance**
   - benchmark native and `js/wasm` for increasing modal counts;
   - select a quality tier from measured budget rather than guessing;
   - no underrun regression in the existing production-build E2E test.

Store short derived metrics and scripts where possible. Do not commit
third-party or newly recorded audio without a clear license and provenance.

That rule is the reason the first recording ever fitted against could not be
committed, and the reason the current one can:
`reference/tt08x08/lp/hd/v01..v16.wav` is CC BY 4.0 with a stated instrument, and
[`reference/CREDITS.md`](../reference/CREDITS.md) carries the licence, the
attribution the licence requires, the instrument as described by the recordist,
what was measured here rather than claimed, and per-file checksums. Anything
added beside it is held to that sheet's shape.

## Sources

Grouped: the modelling literature the design came from, then the estimation and
identifiability literature the 2026-08-01 re-survey added. Each entry says what
it is cited **for**, so an entry that stops being used is visible as such.

### Physical modelling

- F. Avanzini and R. Marogna, ["A Modular Physically Based Approach to the
  Sound Synthesis of Membrane Percussion
  Instruments"](https://doi.org/10.1109/TASL.2009.2036903), IEEE TASLP 18(4), 2010. Modal circular membrane, reduced nonlinear tension, two-head air
  coupling, and modular string/membrane coupling.
- R. Marogna and F. Avanzini, ["Physically-Based Synthesis of Nonlinear
  Circular
  Membranes"](https://avanzini.di.unimi.it/downloads/publications/marogna_dafx09.pdf),
  DAFx-09. Parameter fitting against recordings and the audible role of air
  loading and pitch glide.
- S. Bilbao, ["Time Domain Simulation and Sound Synthesis for the Snare
  Drum"](https://doi.org/10.1121/1.3651240), JASA 131(1), 2012. Full coupled
  2-D/3-D/1-D FDTD system and its numerical limitations.
- A. Torin, B. Hamilton, and S. Bilbao, ["An Energy Conserving Finite
  Difference Scheme for the Simulation of Collisions in Snare
  Drums"](https://www.ness.music.ed.ac.uk/wp-content/uploads/2014/06/dafx14_submission_56-3.pdf),
  DAFx-14. Lossy membranes, 3-D air, mallet contact, snare strings, and
  energy-based collision updates.
- C. Alexandraki et al., ["Inferring Drumhead Damping and Tuning from Sound
  Using Finite Difference Time Domain (FDTD)
  Models"](https://doi.org/10.3390/acoustics5030047), Acoustics 5(3), 2023.
  Inverse estimation and damping/tuning validation.
- J. A. Laird, ["The Physical Modelling of Drums Using Digital
  Waveguides"](https://research-information.bris.ac.uk/en/studentTheses/the-physical-modelling-of-drums-using-digital-waveguides/),
  PhD thesis, 2001. Circular waveguide-mesh boundaries, bearing edge, and
  coupled drum construction.
- M. Nagata and A. Saito, ["Sound Synthesis of Drums Based on Modal
  Representation of Transfer
  Functions"](https://doi.org/10.1177/10775463241272937), Journal of Vibration
  and Control, 2025. Measured acoustic modal transfer functions as a practical
  calibration/validation route.
- R. Diaz, G. Constanzo and M. Sandler, ["nlm: Real-Time Non-linear Modal
  Synthesis in Max"](https://arxiv.org/abs/2603.10240), arXiv:2603.10240, 2026;
  code at https://github.com/rodrigodzf/nlm. Modal synthesis with the nonlinear
  _coupling_ terms retained rather than reduced away, running in real time inside
  Max. It is the reference that turns "keep the mode-to-mode coupling" from a
  feasibility question into a cost question — relevant because this repository's
  Berger law deliberately drops exactly those terms
  ([`physical-nonlinearity.md`](physical-nonlinearity.md) § What the mean-field
  reduction cannot do). Full von Kármán coupling is \(O(N^3)\) in the mode count,
  so at this model's 96 oscillators it would have to be truncated to the dominant
  couplings from the loudest low modes.
- R. Marogna, F. Avanzini and S. Bank, ["Energy-Based Synthesis of Tension
  Modulation in
  Membranes"](https://dafx10.iem.at/papers/MarognaAvanziniBank_DAFx10_P49.pdf),
  DAFx-10 (Graz 2010). Short-time tension variation taken as proportional to
  system energy. This is the reduction family the shipped law belongs to, and it
  is the direct source for the "explicit energy-proportional detune" idea that
  [`physical-nonlinearity.md`](physical-nonlinearity.md) § The solve cost,
  measured retires on cost grounds.
- V. Zheleznov, S. Bilbao, A. Wright and S. King, ["Stable Differentiable Modal
  Synthesis for Learning Nonlinear Dynamics"](https://arxiv.org/abs/2601.10453),
  arXiv:2601.10453. Gradient-based fitting of nonlinear modal models to
  recordings with the parameters constrained to remain physical throughout
  training — a route to calibration that the derivative-free fitter in
  `cmd/fit-physical` does not have. _Cited for:_ the differentiable-synthesis row
  of the re-survey, where it is rejected as a search upgrade rather than a model
  upgrade.
- S. Van Duyne and J. O. Smith, "Physical Modeling with the 2-D Digital Waveguide
  Mesh", ICMC 1993. _Cited for:_ the waveguide-mesh row of the re-survey — the
  origin of the method and of its dispersion and boundary-fitting problems.
- S. Bilbao and C. J. Webb, "Physical Modeling of Timpani Drums in 3D on GPGPUs",
  Journal of the Audio Engineering Society, 2013. _Cited for:_ the cost half of
  the FDTD rejection — a 3-D membrane-plus-air drum at audio rate is GPGPU work,
  which settles the question for a browser audio thread without further argument.
- S. Bank, "Direct Design of Parallel Second-Order Filters for Instrument Body
  Modeling", ICMC 2007, pp. 458–465. _Cited for:_ the body/radiation post-filter
  topology — the published, physically correct way to add a static observation
  filter after a modal sum. Recorded here because that design was tried and
  **failed the falsification test**: no static `g(f)` can fix a defect that is
  time-varying. See
  [`physical-objective-validation.md`](physical-objective-validation.md).

### Estimation, damping and identifiability

Added by the 2026-08-01 re-survey. These are what the survey concluded the path
actually needs, in place of a more sophisticated integrator.

- M. Karjalainen, P. Antsalo, A. Mäkivirta, T. Peltonen and V. Välimäki,
  "Estimation of Modal Decay Parameters from Noisy Response Measurements",
  Journal of the Audio Engineering Society **50**(11):867–878, 2002. _Cited for:_
  the replacement for the repository's log-linear decay fit — it models the
  exponential and the noise floor jointly instead of truncating at −45 dB and
  hoping.
- C. Ege, X. Boutillon and M. David, "High-resolution modal analysis", Journal of
  Sound and Vibration **325**(4–5):852–869, 2009. _Cited for:_ subband ESPRIT
  resolving the closely spaced **twin modes** of a plate — the exact case
  `match`'s `MinSeparationHz = 15` cannot resolve, and therefore the exact case
  `TensionAsymmetry` is being fitted blind against.
- R. Badeau, B. David and G. Richard, "A New Perturbation Analysis for Signal
  Enumeration in Rotational Invariance Techniques", IEEE Transactions on Signal
  Processing **54**(2), 2006. _Cited for:_ ESTER, the order-selection criterion a
  subband-ESPRIT re-estimation needs to decide how many modes are present rather
  than being told.
- R. N. Gutenkunst, J. J. Waterfall, F. P. Casey, K. S. Brown, C. R. Myers and
  J. P. Sethna, "Universally Sloppy Parameter Sensitivities in Systems Biology
  Models", PLoS Computational Biology **3**(10):e189, 2007. _Cited for:_ the name
  and the signature of what the converged fits show — a search that reaches the
  same point from diverse seeds, flat to four digits, over parameter combinations
  the data does not constrain.
- A. Raue, C. Kreutz, T. Maiwald, J. Bachmann, M. Schilling, U. Klingmüller and
  J. Timmer, "Structural and Practical Identifiability Analysis of Partially
  Observed Dynamical Models by Exploiting the Profile Likelihood",
  Bioinformatics **25**(15):1923–1929, 2009. _Cited for:_ the procedure that
  separates structurally non-identifiable parameters from merely
  under-determined ones — the follow-up to a Hessian eigenspectrum, in PLAN.md
  §P10/N6.
- G. Kirby and M. Sandler, Journal of the Acoustical Society of America
  **150**(1):202–214, 2021, [doi:10.1121/10.0005509](https://doi.org/10.1121/10.0005509).
  _Cited for:_ the closest published work to this one — measured on a **tom-tom**
  across 67 strike intensities, with a 20-listener AB test that came out at
  chance. It is both the strongest external anchor for the velocity-dependent
  behaviour the licensed pack now supplies, and the standing evidence that a
  reduced model can be indistinguishable from a recording, so the ceiling here is
  not the synthesis method.
