//go:build amd64 && !purego

package physical

import "golang.org/x/sys/cpu"

// midpointUseAVX2 gates the kernel. AVX2 rather than AVX because the kernel uses
// the register form of nothing in particular — it is simply the baseline this was
// measured against, and the scalar path below is not a fallback anybody should
// mind landing on.
//
// A build with -tags purego drops this file entirely, and so does any non-amd64
// target — including js/wasm, which is what the shipped voice compiles to. The
// assembly therefore only ever runs in the offline tools (cmd/fit-physical,
// cmd/render-physical, the analysis suite), never in the browser.
var midpointUseAVX2 = cpu.X86.HasAVX2

// midpointBatterAVX2 processes n elements, n a multiple of 4. The caller handles
// the remainder; see midpointBatter.
//
//go:noescape
func midpointBatterAVX2(
	n int,
	ratio, timeStep, inverseTimeStep, forceN float64,
	wavenumber, omegaSquared, strikeAccel, midpointDenom []float64,
	velocity, displacement, couplingAccel []float64,
	stepDenominator, midpointVelocity []float64,
)

// midpointBatter runs the vector kernel over the 4-aligned prefix and the
// reference over what is left.
//
// The nil-couplingAccel case takes the scalar path rather than growing a second
// kernel: an inactive coupling is not the configuration the offline fit runs, and
// a zero array cannot stand in for the absent term without turning a -0 numerator
// into +0.
func midpointBatter(
	ratio, timeStep, inverseTimeStep, forceN float64,
	wavenumber, omegaSquared, strikeAccel, midpointDenom []float64,
	velocity, displacement, couplingAccel []float64,
	stepDenominator, midpointVelocity []float64,
) {
	vectored := 0

	if midpointUseAVX2 && couplingAccel != nil {
		if aligned := len(wavenumber) &^ 3; aligned > 0 {
			midpointBatterAVX2(
				aligned,
				ratio, timeStep, inverseTimeStep, forceN,
				wavenumber, omegaSquared, strikeAccel, midpointDenom,
				velocity, displacement, couplingAccel,
				stepDenominator, midpointVelocity,
			)

			vectored = aligned
		}
	}

	midpointReferenceBatter(
		vectored, len(wavenumber),
		ratio, timeStep, inverseTimeStep, forceN,
		wavenumber, omegaSquared, strikeAccel, midpointDenom,
		velocity, displacement, couplingAccel,
		stepDenominator, midpointVelocity,
	)
}
