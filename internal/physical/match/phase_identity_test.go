package match

import (
	"math"
	"math/rand"
	"testing"
)

// TestWrappedPhaseStepMatchesAngleDifference pins the identity probeGlide rests
// on: arg(z[n] * conj(z[n-1])) is the wrapped phase step, so the cross and dot
// products replace differencing two absolute phases and correcting the wrap by
// hand.
//
// Checked against the form that was there — atan2 twice plus the +3pi/Mod/-pi
// correction — over steps well inside (-pi, pi), which is the regime a glide
// probe measures. Wraps at exactly +/-pi are excluded: the two forms land on
// opposite ends of the half-open interval there, and neither answer is more
// right than the other.
func TestWrappedPhaseStepMatchesAngleDifference(t *testing.T) {
	t.Parallel()

	rng := rand.New(rand.NewSource(11))

	worst := 0.0

	for range 20000 {
		previousPhase := (rng.Float64()*2 - 1) * math.Pi
		// Comfortably inside the wrap, and spanning the small steps a glide is
		// actually made of.
		step := (rng.Float64()*2 - 1) * 0.95 * math.Pi
		if math.Abs(step) > 0.99*math.Pi {
			continue
		}

		previousMagnitude := math.Exp(rng.NormFloat64() * 3)
		magnitude := math.Exp(rng.NormFloat64() * 3)

		previousInPhase := previousMagnitude * math.Cos(previousPhase)
		previousQuadrature := previousMagnitude * math.Sin(previousPhase)
		inPhase := magnitude * math.Cos(previousPhase+step)
		quadrature := magnitude * math.Sin(previousPhase+step)

		cross := quadrature*previousInPhase - inPhase*previousQuadrature
		dot := inPhase*previousInPhase + quadrature*previousQuadrature
		got := math.Atan2(cross, dot)

		previousAngle := math.Atan2(previousQuadrature, previousInPhase)
		angle := math.Atan2(quadrature, inPhase)
		want := math.Mod(angle-previousAngle+3*math.Pi, 2*math.Pi) - math.Pi

		worst = max(worst, math.Abs(got-want))
	}

	t.Logf("worst disagreement %.3e rad", worst)

	if worst > 1e-9 {
		t.Fatalf("wrapped phase step departs from the angle difference by %.3e rad", worst)
	}
}
