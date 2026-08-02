package physical

import "math"

// The implicit-midpoint solve: the fixed point DoubleHead.tickCoupled advances a
// coupled sample with, and the elimination of the enclosed air inside it.
//
// Split out of double_head.go for file length alone. Nothing here is reachable
// except through tickCoupled, and the elementwise halves of the head update live
// one level further down in midpoint.go so they can have an assembly
// implementation; see the bit-exactness note there.

// solveNonlinearStep runs the implicit-midpoint fixed point for one sample and
// returns the two head strain measures it converged on, plus whether the coupled
// iteration had to be abandoned.
//
// useCoupling is not the same question as d.couplingActive: tickCoupled calls
// this a second time with it false when the coupled pass diverged. Everything
// written here is scratch — d.displacement, d.velocity and the cavity state are
// committed by the caller — so a second call from the same pre-step state is a
// clean redo rather than a rollback.
func (d *DoubleHead) solveNonlinearStep(
	forceN float64,
	useCoupling bool,
) (float64, float64, bool) {
	batterTension := d.batterNonlinear.tensionAt(
		d.batterNonlinear.strainMeasureM2,
	)
	resonantTension := d.resonantNonlinear.tensionAt(
		d.resonantNonlinear.strainMeasureM2,
	)
	batterStrain := d.batterNonlinear.strainMeasureM2
	resonantStrain := d.resonantNonlinear.strainMeasureM2

	// The left endpoint of every discrete gradient below is this pre-step strain,
	// which no iteration touches — nothing in this function or anything it calls
	// writes strainMeasureM2. Built once here so its logCosh is paid for once per
	// solve instead of once per iteration; see tensionReference.
	batterEnd := d.batterNonlinear.tensionReferenceAt(batterStrain)
	resonantEnd := d.resonantNonlinear.tensionReferenceAt(resonantStrain)

	iterationCount := 1
	if d.config.Nonlinearity.Enabled {
		iterationCount = nonlinearSolveIterations
	}

	if useCoupling {
		d.beginCouplingStep()
	} else {
		// The coupled path's force term is still summed into the midpoint
		// numerator — the branch there is on couplingActive, which has not
		// changed — so it is zeroed rather than skipped.
		clear(d.couplingAccel)
		clear(d.channelTension)
	}

	iterationsUsed := 0
	previousResidual := math.Inf(1)
	diverged := false

	for range iterationCount {
		iterationsUsed++

		if useCoupling {
			d.accumulateCouplingForces()
		}

		batterStrain, resonantStrain = d.solveMidpoint(forceN, batterTension, resonantTension)
		nextBatterTension := d.batterNonlinear.discreteTension(
			&batterEnd,
			batterStrain,
		)

		nextResonantTension := d.resonantNonlinear.discreteTension(
			&resonantEnd,
			resonantStrain,
		)
		converged := tensionConverged(
			batterTension,
			nextBatterTension,
			d.batterNonlinear.maxTensionNPerM,
		) && tensionConverged(
			resonantTension,
			nextResonantTension,
			d.resonantNonlinear.maxTensionNPerM,
		)

		// The channel tensions depend on the endpoint exactly as the head
		// tensions do, so the coupling has to sit inside the fixed point rather
		// than beside it, and the convergence test grows from two scalars to
		// 2 + C.
		if useCoupling {
			residual := d.advanceChannelTensions()
			tolerance := nonlinearSolveTolerance * d.channelTensionScale

			// The growth test is deliberately not applied once the residual has
			// reached the tolerance band: down there it is a difference of two
			// nearly equal tensions, its ratio to the previous one is float noise,
			// and a converged channel set would trip a divergence check built on
			// it. A NaN residual is caught on its own, since every comparison
			// against it is false and it is the last thing that happens before the
			// state itself goes non-finite.
			switch {
			case math.IsNaN(residual),
				previousResidual > tolerance &&
					residual > couplingResidualGrowth*previousResidual:
				diverged = true
			case residual > tolerance:
				converged = false
			}

			previousResidual = residual

			if diverged {
				break
			}
		}

		if converged {
			break
		}

		batterTension = nextBatterTension
		resonantTension = nextResonantTension
	}

	if d.config.Nonlinearity.Enabled {
		d.nonlinearSolveIterations = iterationsUsed
	} else {
		d.nonlinearSolveIterations = 0
	}

	return batterStrain, resonantStrain, diverged
}

// beginCouplingStep seeds the fixed point: the endpoint guess is the current
// state, the midpoint displacement is the current displacement, and the channel
// tensions follow from those two exactly as the head tensions do.
func (d *DoubleHead) beginCouplingStep() {
	copy(d.channelTrial, d.channelValue)
	copy(d.couplingBar, d.displacement[:len(d.couplingBar)])

	for index, value := range d.channelValue {
		d.channelTension[index] = d.coupling.coefficientNPerM * value
	}
}

// accumulateCouplingForces forms the modal acceleration
//
//	a_i = -(1/M_i) sum_c T_c (D^c q_bar)_i
//
// from the current iterate. Paired with the secant T_c, its work over the step
// is exactly minus the change in the channel potentials — the same discrete
// gradient identity the scalar Berger law already satisfies, and for the same
// reason: U is a sum of functions of scalar quadratic forms, so the scalar
// secant *is* the vector discrete gradient. No Gonzalez projection, and no 0/0
// branch at rest on a 96-vector.
func (d *DoubleHead) accumulateCouplingForces() {
	// Hoisted deliberately, and measured rather than assumed: the compiler cannot
	// prove that storing through d.couplingAccel leaves d's own fields alone, so
	// with these read as d.field it reloaded every slice header from the struct on
	// each iteration. perf annotate put ~20% of this function's instructions in
	// those reloads alone. In locals the base pointers stay in registers.
	accel := d.couplingAccel
	bar := d.couplingBar
	inverseMass := d.couplingInverseMass
	tensions := d.channelTension
	columns := d.coupling.entryColumn
	values := d.coupling.entryValue
	runs := d.coupling.runs

	clear(accel)

	for index := range runs {
		run := &runs[index]

		tension := tensions[run.channel]
		if tension == 0 {
			continue
		}

		// Constant across the run, which is the point of iterating over runs.
		row := run.row
		barRow := bar[row]
		rowTotal := 0.0

		for slot := run.first; slot < run.last; slot++ {
			column := columns[slot]
			scaled := tension * values[slot]

			rowTotal += scaled * bar[column]
			if row != column {
				accel[column] -= scaled * barRow * inverseMass[column]
			}
		}

		accel[row] -= rowTotal * inverseMass[row]
	}
}

// advanceChannelTensions recomputes T_c from the endpoint the last solve
// produced and returns the largest channel tension correction it made — the
// fixed point's residual in N/m. Below nonlinearSolveTolerance*channelTensionScale
// the iteration has converged; growing from one iteration to the next it is
// diverging. Returning the residual rather than a converged flag is what lets
// the caller tell those two apart.
func (d *DoubleHead) advanceChannelTensions() float64 {
	residual := 0.0

	for channel, trial := range d.channelTrial {
		tension := d.coupling.coefficientNPerM *
			0.5 * (d.channelValue[channel] + trial)
		residual = max(residual, math.Abs(tension-d.channelTension[channel]))

		d.channelTension[channel] = tension
	}

	return residual
}

// channelValuesAt evaluates g_c = q^T D^c q for every retained channel at the
// given batter displacements.
func (d *DoubleHead) channelValuesAt(displacement, dst []float64) {
	// Accumulated rather than assigned, because a channel spans several runs —
	// and cleared first, so a channel with no entries still lands on zero.
	clear(dst)

	// Hoisted for the same reason as in accumulateCouplingForces.
	columns := d.coupling.entryColumn
	values := d.coupling.entryDoubledValue
	runs := d.coupling.runs

	for index := range runs {
		run := &runs[index]

		total := 0.0
		for slot := run.first; slot < run.last; slot++ {
			total += values[slot] * displacement[columns[slot]]
		}

		dst[run.channel] += displacement[run.row] * total
	}
}

func (d *DoubleHead) solveMidpoint(
	forceN, batterTensionNPerM, resonantTensionNPerM float64,
) (float64, float64) {
	timeStep := 1 / d.config.SampleRateHz
	inverseTimeStep := d.config.SampleRateHz

	// One division for each head rather than one per mode per nonlinear
	// iteration. T/sigma is the same quotient for every mode of a head, so at 120
	// modes and about two iterations a sample this was ~240 divisions a sample
	// spent recomputing two numbers. Same quotient, so the bank is unchanged bit
	// for bit.
	batterRatio := batterTensionNPerM / d.config.Batter.SurfaceDensityKgPerM2
	resonantRatio := resonantTensionNPerM / d.config.Resonant.SurfaceDensityKgPerM2

	cavityCount := len(d.cavityModes)

	// Hoisted into locals for the reason perf annotate exposed in
	// accumulateCouplingForces: read as d.field inside these loops, the compiler
	// reloads each slice header — and couplingActive — from the struct on every
	// mode, because a store through any of them might have touched d itself.
	modes := d.modes
	velocity := d.velocity
	displacement := d.displacement
	midpointVelocities := d.midpointVelocity
	midpointDenom := d.midpointDenom
	stepDenominator := d.stepDenominator
	couplingAccel := d.couplingAccel
	couplingActive := d.couplingActive
	batterModeCount := d.batterModeCount

	clear(d.cavityDrive)
	clear(d.cavityMatrix)

	// The two heads are separate loops rather than one loop with a predicate on
	// index, because every predicate in here was loop-invariant: which head, and
	// therefore which density, which tension, whether the strike force applies and
	// whether the quartic coupling reaches it. Splitting evaluates each of them
	// once instead of 120 times a pass, and leaves two straight elementwise loops.
	// The two heads are separate calls rather than one loop with a predicate on
	// index, because every predicate here was loop-invariant: which head, and
	// therefore which density, which tension, whether the strike force applies and
	// whether the quartic coupling reaches it.
	//
	// The bodies live in midpoint.go so they can have a vector implementation; see
	// the bit-exactness note there for why the operation order is not negotiable.
	accel := couplingAccel
	if !couplingActive {
		accel = nil
	}

	midpointBatter(
		batterRatio, timeStep, inverseTimeStep, forceN,
		d.modeWavenumberPerM[:batterModeCount],
		d.modeOmegaSquared[:batterModeCount],
		d.modeStrikeAccelPerN[:batterModeCount],
		midpointDenom[:batterModeCount],
		velocity[:batterModeCount],
		displacement[:batterModeCount],
		accel,
		stepDenominator[:batterModeCount],
		midpointVelocities[:batterModeCount],
	)

	// The resonant head is never struck and the quartic table is batter-only, so
	// this is the same recurrence without either source term.
	midpointResonant(
		resonantRatio, timeStep, inverseTimeStep,
		d.modeWavenumberPerM[batterModeCount:],
		d.modeOmegaSquared[batterModeCount:],
		midpointDenom[batterModeCount:],
		velocity[batterModeCount:],
		displacement[batterModeCount:],
		stepDenominator[batterModeCount:],
		midpointVelocities[batterModeCount:],
	)

	// Both accumulations are restricted to this mode's own azimuthal family, so
	// the k x k feedback matrix is filled block by block and the loop is linear in
	// the retained mode count exactly as the rank-one form was.
	//
	// Its own pass now: the modes it visits are the minority that couple to the
	// air at all, and hosting it inside the update loops meant every mode paid the
	// two index loads that decide whether it does. The accumulation order over
	// modes is the one the fused version had, which is what keeps the matrix
	// identical rather than merely equivalent.
	//
	// The four per-mode arrays are cut to one common length so the walk over the
	// bank costs no bounds check per mode, and cavityDrive and cavityMatrix are
	// taken into locals because the stores through them may alias d.
	modeArrayCount := len(modes)
	fillCouplingRange := d.couplingRange[:modeArrayCount]
	fillStepDenominator := stepDenominator[:modeArrayCount]
	fillMidpointVelocity := midpointVelocities[:modeArrayCount]
	cavityDrive := d.cavityDrive
	cavityMatrix := d.cavityMatrix

	for index := range modes {
		couplingRange := fillCouplingRange[index]

		first := int(couplingRange.first)

		last := first + int(couplingRange.count)
		if first == last {
			continue
		}

		// Sliced once, so the O(count^2) inner loop below indexes short local
		// slices instead of re-deriving offsets into the bank-wide arrays.
		areas := d.couplingAreaM2[first:last]
		gains := d.couplingGain[first:last]
		cavities := d.couplingCavity[first:last]

		modalDenominator := modes[index].ModalMassKg * fillStepDenominator[index]
		for slot := range gains {
			gains[slot] = areas[slot] / modalDenominator
		}

		uncoupledMidpointVelocity := fillMidpointVelocity[index]

		for slot, area := range areas {
			cavity := int(cavities[slot])
			row := cavity * cavityCount

			cavityDrive[cavity] += area * uncoupledMidpointVelocity
			for other, gain := range gains {
				cavityMatrix[row+int(cavities[other])] += area * gain
			}
		}
	}

	if d.config.Cavity.Enabled {
		d.solveCavityMidpoint(timeStep, inverseTimeStep)
	} else {
		clear(d.cavityMidpointPa)
	}

	batterStrain := 0.0
	resonantStrain := 0.0

	// The same header hoisting the prologue above does, and needed for the same
	// reason: the stores into couplingEnd, couplingBar and midpointVelocities may
	// alias d, so read as d.field these six are reloaded from the struct on every
	// mode and again on every coupling slot.
	//
	// Each half's per-mode arrays are additionally cut to one common length, for
	// the bounds-check reason observe documents. The coupling table is not: its
	// slots are addressed by an opaque offset, so those checks stay.
	couplingGain := d.couplingGain
	couplingCavity := d.couplingCavity
	cavityMidpointPa := d.cavityMidpointPa

	modeCount := len(modes)
	batterModeCount = min(batterModeCount, modeCount)

	batterCouplingRange := d.couplingRange[:batterModeCount]
	batterStrainWeight := d.strainWeight[:batterModeCount]
	batterDisplacement := displacement[:batterModeCount]
	batterMidpointVelocity := midpointVelocities[:batterModeCount]

	halfTimeStep := 0.5 * timeStep

	// Split for the same reason the update loops are: which head a mode belongs to
	// decides which strain it feeds and whether it has a coupling endpoint to
	// record, and neither question changes within a run of indices.
	//
	// The batter half is written twice on couplingActive, which is loop-invariant
	// and was tested per mode. That is not only the branch: with the coupling
	// inactive couplingEnd and couplingBar are nil — installCoupling allocates
	// them only when the table is live — so they can be cut to batterModeCount
	// exactly where they are indexed and nowhere else.
	if couplingActive {
		couplingEnd := d.couplingEnd[:batterModeCount]
		couplingBar := d.couplingBar[:batterModeCount]

		for index, midpointVelocity := range batterMidpointVelocity {
			couplingRange := batterCouplingRange[index]

			first := int(couplingRange.first)
			for slot, last := first, first+int(couplingRange.count); slot < last; slot++ {
				midpointVelocity -= couplingGain[slot] *
					cavityMidpointPa[int(couplingCavity[slot])]
			}

			batterMidpointVelocity[index] = midpointVelocity
			newDisplacement := batterDisplacement[index] +
				timeStep*midpointVelocity

			batterStrain += batterStrainWeight[index] *
				newDisplacement * newDisplacement

			couplingEnd[index] = newDisplacement
			couplingBar[index] = batterDisplacement[index] +
				halfTimeStep*midpointVelocity
		}
	} else {
		for index, midpointVelocity := range batterMidpointVelocity {
			couplingRange := batterCouplingRange[index]

			first := int(couplingRange.first)
			for slot, last := first, first+int(couplingRange.count); slot < last; slot++ {
				midpointVelocity -= couplingGain[slot] *
					cavityMidpointPa[int(couplingCavity[slot])]
			}

			batterMidpointVelocity[index] = midpointVelocity
			newDisplacement := batterDisplacement[index] +
				timeStep*midpointVelocity

			batterStrain += batterStrainWeight[index] *
				newDisplacement * newDisplacement
		}
	}

	resonantCouplingRange := d.couplingRange[batterModeCount:modeCount]
	resonantStrainWeight := d.strainWeight[batterModeCount:modeCount]
	resonantDisplacement := displacement[batterModeCount:modeCount]
	resonantMidpointVelocity := midpointVelocities[batterModeCount:modeCount]

	for index, midpointVelocity := range resonantMidpointVelocity {
		couplingRange := resonantCouplingRange[index]

		first := int(couplingRange.first)
		for slot, last := first, first+int(couplingRange.count); slot < last; slot++ {
			midpointVelocity -= couplingGain[slot] *
				cavityMidpointPa[int(couplingCavity[slot])]
		}

		resonantMidpointVelocity[index] = midpointVelocity
		newDisplacement := resonantDisplacement[index] +
			timeStep*midpointVelocity

		resonantStrain += resonantStrainWeight[index] *
			newDisplacement * newDisplacement
	}

	if couplingActive {
		d.channelValuesAt(d.couplingEnd, d.channelTrial)
	}

	return batterStrain, resonantStrain
}

// solveCavityMidpoint finishes the implicit-midpoint elimination of the enclosed
// air. Applying the rule to
//
//	Pdot_c = K_c sum_i C_ic qdot_i + omega_c H_c - lambda P_c,
//	Hdot_c = -omega_c P_c
//
// and substituting the head modes' own midpoint velocities gives, for each
// cavity mode,
//
//	P_c (2/dt + lambda + omega_c^2 dt/2) + K_c sum_b sum_i C_ic C_ib/(M_i D_i) P_b
//	  = 2 P_c^old/dt + omega_c H_c^old + K_c sum_i C_ic u_i,
//
// which is the k x k Woodbury form of what used to be one Sherman-Morrison
// division. The matrix is diag(K_c) times a symmetric positive definite matrix —
// the diagonal term is strictly positive and the coupling block is a Gram matrix
// with positive weights 1/(M_i D_i) — so every pivot is positive and elimination
// without pivoting is safe. At k = 1 it is literally the old single division,
// which is what keeps a one-mode cavity bit-exact.
func (d *DoubleHead) solveCavityMidpoint(timeStep, inverseTimeStep float64) {
	cavityCount := len(d.cavityModes)
	lossPerSecond := d.config.Cavity.LossPerSecond

	// Hoisted for the aliasing reason the solve's prologue documents: the stores
	// through these four force a reload of every header from d on each access
	// otherwise. The row-major indices below are products of two locals, which
	// the prover cannot relate to a length, so the bounds checks stay — at
	// cavityCount <= 6 there is no loop to hoist them out of that would pay.
	matrix := d.cavityMatrix
	drive := d.cavityDrive
	midpointPa := d.cavityMidpointPa
	cavityModes := d.cavityModes

	for index := range cavityCount {
		mode := &cavityModes[index]
		row := index * cavityCount

		stiffness := mode.StiffnessPaPerM3
		for column := range cavityCount {
			matrix[row+column] *= stiffness
		}

		matrix[row+index] += 2*inverseTimeStep + lossPerSecond +
			0.5*mode.AngularFrequency*mode.AngularFrequency*timeStep
		drive[index] = 2*d.cavityPressurePa[index]*inverseTimeStep +
			mode.AngularFrequency*d.cavityFlowPa[index] +
			stiffness*drive[index]
	}

	for pivotIndex := range cavityCount {
		pivot := matrix[pivotIndex*cavityCount+pivotIndex]
		for rowIndex := pivotIndex + 1; rowIndex < cavityCount; rowIndex++ {
			leading := matrix[rowIndex*cavityCount+pivotIndex]
			if leading == 0 {
				continue
			}

			factor := leading / pivot
			for column := pivotIndex + 1; column < cavityCount; column++ {
				matrix[rowIndex*cavityCount+column] -= factor *
					matrix[pivotIndex*cavityCount+column]
			}

			drive[rowIndex] -= factor * drive[pivotIndex]
		}
	}

	for rowIndex := cavityCount - 1; rowIndex >= 0; rowIndex-- {
		sum := drive[rowIndex]
		for column := rowIndex + 1; column < cavityCount; column++ {
			sum -= matrix[rowIndex*cavityCount+column] *
				midpointPa[column]
		}

		midpointPa[rowIndex] = sum /
			matrix[rowIndex*cavityCount+rowIndex]
	}
}

func tensionConverged(current, next, maximum float64) bool {
	return math.Abs(next-current) <=
		nonlinearSolveTolerance*max(1, maximum)
}
