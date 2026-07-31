package physical

import (
	"math"
	"math/rand"
	"testing"
)

// TestMidpointKernelMatchesReferenceExactly is the whole justification for the
// assembly being allowed to exist.
//
// The calibration fixture and the rendered-WAV digest both compare exactly, so a
// kernel that is merely accurate is a kernel that breaks CI. Every operation in
// the update is per-lane independent, so vectorising it may not move a single
// bit — comparison is on the raw IEEE bits, not a tolerance, and -0 is therefore
// distinguished from +0 on purpose.
func TestMidpointKernelMatchesReferenceExactly(t *testing.T) {
	t.Parallel()

	rng := rand.New(rand.NewSource(7))

	// Lengths either side of the 4-wide step, so the scalar tail and the n < 4
	// early-out are covered rather than assumed.
	for _, count := range []int{0, 1, 2, 3, 4, 5, 7, 8, 9, 15, 16, 96, 97} {
		random := func() []float64 {
			out := make([]float64, count)
			for i := range out {
				out[i] = math.Exp(rng.NormFloat64()*8) * float64(1-2*rng.Intn(2))
			}

			return out
		}

		wavenumber, omegaSquared := random(), random()
		strikeAccel, midpointDenom := random(), random()
		velocity, displacement, couplingAccel := random(), random(), random()

		ratio := math.Exp(rng.NormFloat64() * 6)
		timeStep := 1.0 / 44100
		inverseTimeStep := 44100.0
		forceN := rng.NormFloat64() * 100

		wantDenominator := make([]float64, count)
		wantVelocity := make([]float64, count)
		midpointReferenceBatter(0, count,
			ratio, timeStep, inverseTimeStep, forceN,
			wavenumber, omegaSquared, strikeAccel, midpointDenom,
			velocity, displacement, couplingAccel,
			wantDenominator, wantVelocity)

		gotDenominator := make([]float64, count)
		gotVelocity := make([]float64, count)
		midpointBatter(
			ratio, timeStep, inverseTimeStep, forceN,
			wavenumber, omegaSquared, strikeAccel, midpointDenom,
			velocity, displacement, couplingAccel,
			gotDenominator, gotVelocity,
		)

		for i := range count {
			if math.Float64bits(gotDenominator[i]) != math.Float64bits(wantDenominator[i]) {
				t.Fatalf("n=%d i=%d denominator: got %x want %x",
					count, i, math.Float64bits(gotDenominator[i]),
					math.Float64bits(wantDenominator[i]))
			}

			if math.Float64bits(gotVelocity[i]) != math.Float64bits(wantVelocity[i]) {
				t.Fatalf("n=%d i=%d velocity: got %x want %x",
					count, i, math.Float64bits(gotVelocity[i]),
					math.Float64bits(wantVelocity[i]))
			}
		}
	}
}
