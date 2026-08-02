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
// a stored velocity. The test is hoisted into two loop bodies so the branch is
// per call, not per element.
//
// # Bounds checks
//
// Both loops range over equal-length reslices rather than walking an index from
// first to last. The arithmetic is unchanged; only the bound moves. `last` is an
// opaque parameter, so the prover could not relate it to any slice length and
// emitted a check on every indexed load and store, per element. Reslicing once
// gives it a length it can carry into the loop:
//
//	GOOS=js GOARCH=wasm go build -gcflags='-d=ssa/check_bce/debug=1' ./internal/physical/
//
// reported 16 IsInBounds inside these two loop bodies before this change and
// none after — what remains is 16 IsSliceInBounds on the reslices themselves,
// which are paid once per call instead of once per mode. That matters here
// specifically because midpoint_noasm.go is what the shipped js/wasm build
// always runs.
func midpointReferenceBatter(
	first, last int,
	ratio, timeStep, inverseTimeStep, forceN float64,
	wavenumber, omegaSquared, strikeAccel, midpointDenom []float64,
	velocity, displacement, couplingAccel []float64,
	stepDenominator, midpointVelocity []float64,
) {
	count := last - first
	if count <= 0 {
		return
	}

	wavenumber = wavenumber[first:last]
	omegaSquared = omegaSquared[first : first+count]
	strikeAccel = strikeAccel[first : first+count]
	midpointDenom = midpointDenom[first : first+count]
	velocity = velocity[first : first+count]
	displacement = displacement[first : first+count]
	stepDenominator = stepDenominator[first : first+count]
	midpointVelocity = midpointVelocity[first : first+count]

	if couplingAccel == nil {
		for index, wave := range wavenumber {
			nonlinear := ratio * wave * wave
			angularSquared := omegaSquared[index] + nonlinear
			denominator := midpointDenom[index] + 0.5*nonlinear*timeStep

			numerator := 2*velocity[index]*inverseTimeStep -
				angularSquared*displacement[index] +
				forceN*strikeAccel[index]

			stepDenominator[index] = denominator
			midpointVelocity[index] = numerator / denominator
		}

		return
	}

	couplingAccel = couplingAccel[first : first+count]

	for index, wave := range wavenumber {
		nonlinear := ratio * wave * wave
		angularSquared := omegaSquared[index] + nonlinear
		denominator := midpointDenom[index] + 0.5*nonlinear*timeStep

		numerator := 2*velocity[index]*inverseTimeStep -
			angularSquared*displacement[index] +
			forceN*strikeAccel[index]
		numerator += couplingAccel[index]

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
	count := last - first
	if count <= 0 {
		return
	}

	wavenumber = wavenumber[first:last]
	omegaSquared = omegaSquared[first : first+count]
	midpointDenom = midpointDenom[first : first+count]
	velocity = velocity[first : first+count]
	displacement = displacement[first : first+count]
	stepDenominator = stepDenominator[first : first+count]
	midpointVelocity = midpointVelocity[first : first+count]

	for index, wave := range wavenumber {
		nonlinear := ratio * wave * wave
		angularSquared := omegaSquared[index] + nonlinear
		denominator := midpointDenom[index] + 0.5*nonlinear*timeStep

		numerator := 2*velocity[index]*inverseTimeStep -
			angularSquared*displacement[index]

		stepDenominator[index] = denominator
		midpointVelocity[index] = numerator / denominator
	}
}
