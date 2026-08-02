package physical

import (
	"errors"
	"math"
	"math/cmplx"
)

var ErrInvalidFrequency = errors.New("analysis frequency must be finite and in (0, Nyquist)")

// FrequencyResponse is the continuous-time reduced-model response to one
// newton of sinusoidal force at the configured batter strike footprint. It is
// intended for offline validation, not the audio-rate path.
type FrequencyResponse struct {
	BatterVelocityMPerS   complex128
	ResonantVelocityMPerS complex128
	BatterRawRadiated     complex128
	ResonantRawRadiated   complex128
	RawRadiated           complex128
	CavityPressurePa      complex128
	// CavityPressuresPa is the pressure of every retained cavity mode, in the
	// order GenerateCavityModes returns them. CavityPressurePa above is its
	// uniform member, kept as its own field because that is the one quantity
	// the lumped model ever had.
	CavityPressuresPa []complex128
}

// ReferenceFrequencyResponse solves the diagonal modal system plus its modal
// cavity coupling in the frequency domain. This is the small-signal response
// linearized at zero displacement; nonlinear level-dependent behaviour is
// verified in the time domain.
//
// Each cavity mode contributes an impedance
//
//	Z_c = K_c (jW)^2 / ((jW)^2 + lambda jW + omega_c^2),
//
// which for the uniform mode's omega_c = 0 is the K jW/(jW + lambda) the lumped
// compliance always had. Eliminating the pressures leaves
//
//	(I + Z M) P = Z S,   M_cb = sum_i C_ic C_ib / D_i,   S_c = sum_i C_ic u_i,
//
// a k x k complex system whose k = 1 case is the Sherman-Morrison form this
// replaced. Allocation here is deliberate: the routine is offline.
func (d *DoubleHead) ReferenceFrequencyResponse(
	frequencyHz float64,
) (FrequencyResponse, error) {
	if math.IsNaN(frequencyHz) || math.IsInf(frequencyHz, 0) ||
		frequencyHz <= 0 || frequencyHz >= d.config.SampleRateHz/2 {
		return FrequencyResponse{}, ErrInvalidFrequency
	}

	angularFrequency := 2 * math.Pi * frequencyHz
	imaginaryAngularFrequency := complex(0, angularFrequency)

	cavityCount := len(d.cavityModes)
	impedance := make([]complex128, cavityCount)

	if d.config.Cavity.Enabled {
		for index, cavity := range d.cavityModes {
			squared := imaginaryAngularFrequency * imaginaryAngularFrequency
			impedance[index] = complex(cavity.StiffnessPaPerM3, 0) * squared /
				(squared +
					complex(d.config.Cavity.LossPerSecond, 0)*
						imaginaryAngularFrequency +
					complex(cavity.AngularFrequency*cavity.AngularFrequency, 0))
		}
	}

	uncoupledDisplacement := make([]complex128, len(d.modes))
	inverseStiffness := make([]complex128, len(d.modes))
	drive := make([]complex128, cavityCount)
	coupling := make([]complex128, cavityCount*cavityCount)

	for index, mode := range d.modes {
		dynamicStiffness := complex(
			mode.ModalMassKg*
				(mode.AngularFrequency*mode.AngularFrequency-
					angularFrequency*angularFrequency),
			2*mode.ModalMassKg*mode.DecayRatePerSecond*angularFrequency,
		)

		forceShape := 0.0
		if index < d.batterModeCount {
			forceShape = mode.StrikeAccelerationPerN * mode.ModalMassKg
		}

		uncoupledDisplacement[index] =
			complex(forceShape, 0) / dynamicStiffness
		inverseStiffness[index] = 1 / dynamicStiffness

		first := int(d.couplingRange[index].first)

		last := first + int(d.couplingRange[index].count)
		for slot := first; slot < last; slot++ {
			area := complex(d.couplingAreaM2[slot], 0)
			row := int(d.couplingCavity[slot])

			drive[row] += area * uncoupledDisplacement[index]
			for other := first; other < last; other++ {
				coupling[row*cavityCount+int(d.couplingCavity[other])] +=
					area * complex(d.couplingAreaM2[other], 0) *
						inverseStiffness[index]
			}
		}
	}

	system := make([]complex128, cavityCount*cavityCount)

	rightHandSide := make([]complex128, cavityCount)
	for row := range cavityCount {
		rightHandSide[row] = impedance[row] * drive[row]
		for column := range cavityCount {
			system[row*cavityCount+column] = impedance[row] *
				coupling[row*cavityCount+column]
		}

		system[row*cavityCount+row] += 1
	}

	pressure, err := solveDenseComplex(system, rightHandSide, cavityCount)
	if err != nil {
		return FrequencyResponse{}, err
	}

	var response FrequencyResponse

	for index, mode := range d.modes {
		displacement := uncoupledDisplacement[index]

		first := int(d.couplingRange[index].first)

		last := first + int(d.couplingRange[index].count)
		for slot := first; slot < last; slot++ {
			displacement -= complex(d.couplingAreaM2[slot], 0) *
				inverseStiffness[index] *
				pressure[int(d.couplingCavity[slot])]
		}

		velocity := imaginaryAngularFrequency * displacement
		pickupVelocity := complex(mode.PickupShape, 0) * velocity
		// The radiated observable is volume acceleration, so one more jOmega.
		// The real-time model forms it as a first difference, whose magnitude is
		// 2*sin(w*dt/2)/dt rather than w — 1.3e-5 low at the transfer test's
		// probe frequency, with a half-sample advance the test does not see
		// because it compares magnitudes.
		acceleration := imaginaryAngularFrequency * velocity
		rawRadiated := complex(mode.RadiationWeight, 0) * acceleration

		if index < d.batterModeCount {
			response.BatterVelocityMPerS += pickupVelocity
			response.BatterRawRadiated += rawRadiated
		} else {
			response.ResonantVelocityMPerS += pickupVelocity
			response.ResonantRawRadiated += rawRadiated
		}
	}

	// The configured pickup is on the batter side; see DoubleHead.observe.
	response.RawRadiated = response.BatterRawRadiated
	response.CavityPressurePa = pressure[0]
	response.CavityPressuresPa = pressure

	if !finiteComplex(response.RawRadiated) ||
		!finiteComplex(response.CavityPressurePa) {
		return FrequencyResponse{}, ErrInvalidConfig
	}

	return response, nil
}

// solveDenseComplex solves a small dense complex system by Gaussian elimination
// with partial pivoting. The systems here are k x k with k at most
// maxCavityModes.
func solveDenseComplex(matrix, rightHandSide []complex128, size int) ([]complex128, error) {
	for pivotIndex := range size {
		best := pivotIndex
		for row := pivotIndex + 1; row < size; row++ {
			if cmplx.Abs(matrix[row*size+pivotIndex]) >
				cmplx.Abs(matrix[best*size+pivotIndex]) {
				best = row
			}
		}

		if cmplx.Abs(matrix[best*size+pivotIndex]) == 0 {
			return nil, ErrInvalidConfig
		}

		if best != pivotIndex {
			for column := range size {
				matrix[pivotIndex*size+column], matrix[best*size+column] =
					matrix[best*size+column], matrix[pivotIndex*size+column]
			}

			rightHandSide[pivotIndex], rightHandSide[best] =
				rightHandSide[best], rightHandSide[pivotIndex]
		}

		pivot := matrix[pivotIndex*size+pivotIndex]
		for row := pivotIndex + 1; row < size; row++ {
			factor := matrix[row*size+pivotIndex] / pivot
			for column := pivotIndex; column < size; column++ {
				matrix[row*size+column] -= factor * matrix[pivotIndex*size+column]
			}

			rightHandSide[row] -= factor * rightHandSide[pivotIndex]
		}
	}

	solution := make([]complex128, size)
	for row := size - 1; row >= 0; row-- {
		sum := rightHandSide[row]
		for column := row + 1; column < size; column++ {
			sum -= matrix[row*size+column] * solution[column]
		}

		solution[row] = sum / matrix[row*size+row]
	}

	return solution, nil
}

func finiteComplex(value complex128) bool {
	return !math.IsNaN(real(value)) && !math.IsNaN(imag(value)) &&
		!math.IsInf(real(value), 0) && !math.IsInf(imag(value), 0) &&
		!math.IsNaN(cmplx.Abs(value))
}
