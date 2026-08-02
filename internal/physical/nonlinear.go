package physical

import "math"

// nonlinearSolveIterations is a cap, not a cost. The fixed-point iteration
// exits as soon as the tension stops moving, and measured on the shipped
// configuration at full velocity that is a mean of 2.88 iterations — and it
// stays there: sweeping the tension coefficient over a 32x range moves the mean
// only from 2.88 to 3.09, because a stiffer law both perturbs the tension more
// and contracts faster once tanh starts to saturate.
//
// This is worth stating because it retires a planned change. P8 proposed
// replacing this solve with an explicit energy-proportional detune to buy back
// "6x the voice", on the assumption that all eight iterations ran. They do not,
// the real figure is about three, and it does not grow when the glide is made
// audible — so the discrete-gradient solve keeps its exact energy bookkeeping
// and nothing is traded away for a saving that was never there.
const (
	nonlinearSolveIterations = 8
	nonlinearSolveTolerance  = 2e-12
)

// nonlinearHead is one bounded Berger tension law. For the modal strain
// measure S = integral(|grad w|²)dA, the added tension and stored potential are
//
//	DeltaT(S) = Tmax tanh(beta S / Tmax)
//	U(S)      = Tmax²/(2 beta) log(cosh(beta S / Tmax)).
//
// Near rest this is the ordinary Berger law DeltaT = beta S and
// U = beta S²/4. The smooth cap keeps every retained mode below Nyquist under
// the validated MaximumTensionRatio without clipping stored energy.
//
// Worth being precise about what is approximated here, because it is not the
// strain. S = sum_i Gamma_i q_i² with Gamma_i = M_i k_i²/sigma is *exact*: the
// mode shapes are Dirichlet Laplacian eigenfunctions, so integral(grad phi_i .
// grad phi_j)dA = k_i² integral(phi_i phi_j)dA vanishes off the diagonal and
// the cross terms are identically zero. Writing g = |grad w|², the whole of
// Berger's error lives in the *second* moment: the quartic membrane potential
// goes as integral(g²)dA, while Berger uses (integral(g)dA)²/A, which is the
// projection of g onto the constant function. That is a projection and not a
// series truncation, which is why U_Berger <= U_exact always, by Cauchy-Schwarz.
//
// Everything from here to the end of this comment describes what *this type*
// does, and it is still exactly true of it — but it is no longer the whole
// model. P9/M1 added the second moment back as a set of orthogonal channels in
// coupling.go, keeping this law untouched as the uniform channel it reproduces
// exactly. The limitation below is therefore the limitation of the Berger
// reduction, not of the shipped instrument.
//
// This reduction is mean-field, and that is a stated limitation rather than an
// implementation detail. Collapsing the geometric nonlinearity onto one scalar
// DeltaT over total strain detunes every mode by the same *relative* amount and
// leaves the modal equations diagonal, so no mode can transfer energy to any
// other — the defining property of the Berger / Kirchhoff-Carrier family. The
// consequence is that this nonlinearity contributes pitch and nothing else: it
// generates no spectral content at all. The only things in the whole model that
// can put energy at a given frequency are the contact force's spectrum and the
// stochastic attack layer, and a real head struck hard does more than that.
//
// What it does is set by the parity of the potential. The membrane's geometric
// nonlinearity is quartic and therefore *even* in the modal amplitudes, so the
// force it produces is cubic and *odd*, and an odd force generates only odd
// combinations: 3f_a, 2f_a ± f_b, f_a ± f_b ± f_c, and the internal resonances
// those admit where the ratios come close to rational. It generates no second
// harmonic and no simple sum or difference tone; 2f_a and f_a ± f_b would need
// a quadratic term in the potential, which arises for shells, curved plates or
// a static preload asymmetry and not for a flat tensioned head. Because the
// lowest combination needs three frequency slots, one pump mode reaches only
// f_a and 3f_a — at least two simultaneously excited modes are needed for
// anything else. That cascade is part of why a hard hit is brighter and not
// merely sharper (Dahl, TMH-QPSR 38(1) 1997). See docs/physical-nonlinearity.md
// § "What the mean-field reduction cannot do".
type nonlinearHead struct {
	coefficientNPerM3 float64
	maxTensionNPerM   float64
	strainMeasureM2   float64
}

func newNonlinearHead(
	enabled bool,
	coefficientNPerM3 float64,
	maximumTensionRatio float64,
	staticTensionNPerM float64,
) nonlinearHead {
	if !enabled || coefficientNPerM3 == 0 {
		return nonlinearHead{}
	}

	return nonlinearHead{
		coefficientNPerM3: coefficientNPerM3,
		maxTensionNPerM:   maximumTensionRatio * staticTensionNPerM,
	}
}

func (head nonlinearHead) tensionAt(strainMeasureM2 float64) float64 {
	if head.coefficientNPerM3 == 0 {
		return 0
	}

	scaledStrain := head.coefficientNPerM3 * strainMeasureM2 /
		head.maxTensionNPerM

	return head.maxTensionNPerM * math.Tanh(scaledStrain)
}

func (head nonlinearHead) potentialEnergy(strainMeasureM2 float64) float64 {
	if head.coefficientNPerM3 == 0 {
		return 0
	}

	scaledStrain := head.coefficientNPerM3 * strainMeasureM2 /
		head.maxTensionNPerM

	return 0.5 * head.maxTensionNPerM * head.maxTensionNPerM /
		head.coefficientNPerM3 * logCosh(scaledStrain)
}

// tensionReference is the left endpoint of the discrete gradient for one head
// over one solve: the pre-step strain, already scaled, plus a memo of its
// logCosh.
//
// It exists because that endpoint is *fixed* for the whole fixed-point
// iteration. solveNonlinearStep reads d.<head>Nonlinear.strainMeasureM2 as the
// old strain on every iteration, and that field is written in exactly three
// places — Reset, and the two commits in tickCoupled and observe — all of which
// run outside the solve. The diverged re-solve re-enters from the same
// unchanged pre-step state, which is what makes it a clean redo rather than a
// rollback, so the endpoint is invariant across that second pass too.
//
// Recomputing logCosh(oldScaled) per iteration was therefore two libm calls
// (Log and Cosh) spent on a value that could not have moved. Counted on
// BenchmarkNonlinearDoubleHeadActive48k, the solve runs 3.96 iterations per head
// per sample and 99.8 % of them reach the branch below that needs it, so the
// memo removes about three quarters of the old-side calls. In that profile the
// logCosh reached through discreteTension falls from 5.7 % to 3.9 % of samples.
//
// Be careful what that is worth: it is 1.8 points of a *profile*, not of the
// clock. The same benchmark retires only 0.58 % fewer instructions (41.826 G ->
// 41.583 G, pinned to one P-core, +-0.03 %) for 1.3 % fewer cycles, because
// Log and Exp are short in instructions and long in dependency chains, which is
// exactly what a sampling profiler over-attributes. BenchmarkCost in
// cmd/fit-physical does not move at all (-0.02 %, inside run-to-run spread): a
// fit evaluation is dominated by feature extraction, not by this solve.
// Wall-clock benchstat called all three of these runs insignificant.
//
// The memo is lazy rather than hoisted into the caller because the *quiet* path
// does not want it at all: on BenchmarkDoubleHeadRender48k, where the tail has
// decayed, the solve runs 1.35 iterations per head per sample and 91 % of the
// calls take the small-difference branch and never touch logCosh. Computing it
// up front would have added a libm pair per head per sample there in exchange
// for nothing; as written that benchmark is unchanged (-0.09 %, noise).
//
// It is a value local to solveNonlinearStep rather than a field beside
// strainMeasureM2 so that a stale memo is not a state that can be represented:
// the reference is built from the strain it belongs to and dies with the solve.
type tensionReference struct {
	scaled      float64
	logCosh     float64
	haveLogCosh bool
}

// tensionReferenceAt builds the fixed endpoint the solve's discrete gradients
// are all taken from.
func (head nonlinearHead) tensionReferenceAt(strainMeasureM2 float64) tensionReference {
	if head.coefficientNPerM3 == 0 {
		return tensionReference{}
	}

	return tensionReference{
		scaled: head.coefficientNPerM3 * strainMeasureM2 /
			head.maxTensionNPerM,
	}
}

// discreteTension is the discrete gradient 2[U(S1)-U(S0)]/(S1-S0).
// Paired with q_mid, it makes the nonlinear work exactly equal the change in
// stored potential, up to the bounded nonlinear solve tolerance.
//
// oldEnd carries S0 and is updated in place with its memoized logCosh; see
// tensionReference for why that is sound and what it saves.
func (head nonlinearHead) discreteTension(
	oldEnd *tensionReference,
	newStrain float64,
) float64 {
	if head.coefficientNPerM3 == 0 {
		return 0
	}

	oldScaled := oldEnd.scaled
	newScaled := head.coefficientNPerM3 * newStrain /
		head.maxTensionNPerM

	difference := newScaled - oldScaled
	if math.Abs(difference) < 1e-5 {
		midpoint := 0.5 * (oldScaled + newScaled)
		tanhMidpoint := math.Tanh(midpoint)
		// The centered secant expansion avoids cancellation close to rest.
		return head.maxTensionNPerM * (tanhMidpoint -
			difference*difference*tanhMidpoint*
				(1-tanhMidpoint*tanhMidpoint)/12)
	}

	if !oldEnd.haveLogCosh {
		oldEnd.logCosh = logCosh(oldScaled)
		oldEnd.haveLogCosh = true
	}

	return head.maxTensionNPerM *
		(logCosh(newScaled) - oldEnd.logCosh) / difference
}

func logCosh(value float64) float64 {
	absolute := math.Abs(value)
	if absolute < 12 {
		return math.Log(math.Cosh(value))
	}

	return absolute + math.Log1p(math.Exp(-2*absolute)) - math.Ln2
}
