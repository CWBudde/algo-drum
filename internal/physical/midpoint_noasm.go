//go:build !amd64 || purego

package physical

// midpointBatter is the portable form. js/wasm — the target the shipped voice is
// built for — always lands here.
func midpointBatter(
	ratio, timeStep, inverseTimeStep, forceN float64,
	wavenumber, omegaSquared, strikeAccel, midpointDenom []float64,
	velocity, displacement, couplingAccel []float64,
	stepDenominator, midpointVelocity []float64,
) {
	midpointReferenceBatter(
		0, len(wavenumber),
		ratio, timeStep, inverseTimeStep, forceN,
		wavenumber, omegaSquared, strikeAccel, midpointDenom,
		velocity, displacement, couplingAccel,
		stepDenominator, midpointVelocity,
	)
}

// midpointResonant is the portable form.
func midpointResonant(
	ratio, timeStep, inverseTimeStep float64,
	wavenumber, omegaSquared, midpointDenom []float64,
	velocity, displacement []float64,
	stepDenominator, midpointVelocity []float64,
) {
	midpointReferenceResonant(
		0, len(wavenumber),
		ratio, timeStep, inverseTimeStep,
		wavenumber, omegaSquared, midpointDenom,
		velocity, displacement,
		stepDenominator, midpointVelocity,
	)
}
