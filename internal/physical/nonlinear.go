package physical

import "math"

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

// discreteTension is the discrete gradient 2[U(S1)-U(S0)]/(S1-S0).
// Paired with q_mid, it makes the nonlinear work exactly equal the change in
// stored potential, up to the bounded nonlinear solve tolerance.
func (head nonlinearHead) discreteTension(oldStrain, newStrain float64) float64 {
	if head.coefficientNPerM3 == 0 {
		return 0
	}

	oldScaled := head.coefficientNPerM3 * oldStrain /
		head.maxTensionNPerM
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

	return head.maxTensionNPerM *
		(logCosh(newScaled) - logCosh(oldScaled)) / difference
}

func logCosh(value float64) float64 {
	absolute := math.Abs(value)
	if absolute < 12 {
		return math.Log(math.Cosh(value))
	}

	return absolute + math.Log1p(math.Exp(-2*absolute)) - math.Ln2
}
