package physical

// The elementwise half of the implicit-midpoint solve, extracted so it can have
// an assembly implementation.
//
// Why this block and not another: perf puts solveMidpoint at 55% of Render's
// retired instructions, and the machine runs it at an IPC of 4.9 out of a
// 6-wide core with 0.04% branch misses and 0.25% L1 misses. There is no stall to
// recover — the only lever left is issuing fewer, wider instructions. These two
// loops are the one part of the solve shaped for that: pure elementwise
// arithmetic over contiguous float64, no gather, no scatter, no reduction and no
// branch. The cavity fill and the coupling table are not, and are left scalar.
//
// # Bit-exactness
//
// Every operation below is per-lane independent, so a vector implementation that
// keeps the same operation order produces identical results — IEEE 754 requires
// mul, add, sub and div to be correctly rounded, and doing four at once changes
// nothing about any one of them.
//
// Two things would break that and are therefore forbidden in the kernels:
//
//   - FMA. Go's amd64 compiler does not contract x*y+z into a fused multiply-add,
//     so the scalar path rounds the product before adding. VFMADD would not, and
//     the results would differ in the last bits.
//   - Reassociation. 0.5*nl*timeStep is (0.5*nl)*timeStep, and 2*v*inverseTimeStep
//     is (2*v)*inverseTimeStep. Folding the constants — nl*(0.5*timeStep) — is
//     equal in exact arithmetic and almost always equal in floating point, but not
//     at the bottom of the subnormal range. The extra multiply is cheaper than the
//     doubt.
//
// midpointReferenceBatter and midpointReferenceResonant below are the definition
// of "the same operation order". They are what the generic build runs, what the
// assembly is checked against, and what the arithmetic in double_head.go used to
// be inline.

// midpointReferenceBatter is the batter head's update: struck, and the only head
// the quartic coupling reaches.
//
// couplingAccel is nil when the coupling is inactive, which is not the same as
// zero — adding a zero would turn a -0 numerator into +0 and change the sign of
// a stored velocity. The branch is per call, not per element.
func midpointReferenceBatter(
	first, last int,
	ratio, timeStep, inverseTimeStep, forceN float64,
	wavenumber, omegaSquared, strikeAccel, midpointDenom []float64,
	velocity, displacement, couplingAccel []float64,
	stepDenominator, midpointVelocity []float64,
) {
	for index := first; index < last; index++ {
		wave := wavenumber[index]
		nonlinear := ratio * wave * wave
		angularSquared := omegaSquared[index] + nonlinear
		denominator := midpointDenom[index] + 0.5*nonlinear*timeStep

		numerator := 2*velocity[index]*inverseTimeStep -
			angularSquared*displacement[index] +
			forceN*strikeAccel[index]
		if couplingAccel != nil {
			numerator += couplingAccel[index]
		}

		stepDenominator[index] = denominator
		midpointVelocity[index] = numerator / denominator
	}
}

// midpointReferenceResonant is the same recurrence without either source term:
// the resonant head is never struck and the quartic table is batter-only.
func midpointReferenceResonant(
	first, last int,
	ratio, timeStep, inverseTimeStep float64,
	wavenumber, omegaSquared, midpointDenom []float64,
	velocity, displacement []float64,
	stepDenominator, midpointVelocity []float64,
) {
	for index := first; index < last; index++ {
		wave := wavenumber[index]
		nonlinear := ratio * wave * wave
		angularSquared := omegaSquared[index] + nonlinear
		denominator := midpointDenom[index] + 0.5*nonlinear*timeStep

		numerator := 2*velocity[index]*inverseTimeStep -
			angularSquared*displacement[index]

		stepDenominator[index] = denominator
		midpointVelocity[index] = numerator / denominator
	}
}
