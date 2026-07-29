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
}

// ReferenceFrequencyResponse solves the diagonal modal system plus its
// rank-one axisymmetric cavity coupling in the frequency domain. This is the
// small-signal response linearized at zero displacement; nonlinear
// level-dependent behaviour is verified in the time domain.
func (d *DoubleHead) ReferenceFrequencyResponse(
	frequencyHz float64,
) (FrequencyResponse, error) {
	if math.IsNaN(frequencyHz) || math.IsInf(frequencyHz, 0) ||
		frequencyHz <= 0 || frequencyHz >= d.config.SampleRateHz/2 {
		return FrequencyResponse{}, ErrInvalidFrequency
	}

	angularFrequency := 2 * math.Pi * frequencyHz
	imaginaryAngularFrequency := complex(0, angularFrequency)

	cavityImpedance := complex(0, 0)
	if d.config.Cavity.Enabled {
		cavityImpedance = complex(d.cavityBulkStiffnessPaPerM3, 0) *
			imaginaryAngularFrequency /
			(imaginaryAngularFrequency +
				complex(d.config.Cavity.LossPerSecond, 0))
	}

	uncoupledDisplacement := make([]complex128, len(d.modes))
	pressureDisplacement := make([]complex128, len(d.modes))
	sweptUncoupled := complex(0, 0)
	sweptPressure := complex(0, 0)

	for index, mode := range d.modes {
		effectiveSweptArea := d.config.Cavity.Coupling01 * mode.SweptAreaM2
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
		pressureDisplacement[index] =
			complex(effectiveSweptArea, 0) / dynamicStiffness
		sweptUncoupled += complex(effectiveSweptArea, 0) *
			uncoupledDisplacement[index]
		sweptPressure += complex(effectiveSweptArea, 0) *
			pressureDisplacement[index]
	}

	pressure := cavityImpedance * sweptUncoupled /
		(1 + cavityImpedance*sweptPressure)

	var response FrequencyResponse

	for index, mode := range d.modes {
		displacement := uncoupledDisplacement[index] -
			pressureDisplacement[index]*pressure
		velocity := imaginaryAngularFrequency * displacement
		pickupVelocity := complex(mode.PickupShape, 0) * velocity
		rawRadiated := complex(mode.RadiationWeight, 0) * velocity

		if index < d.batterModeCount {
			response.BatterVelocityMPerS += pickupVelocity
			response.BatterRawRadiated += rawRadiated
		} else {
			response.ResonantVelocityMPerS += pickupVelocity
			response.ResonantRawRadiated += rawRadiated
		}
	}

	response.RawRadiated = response.BatterRawRadiated +
		response.ResonantRawRadiated
	response.CavityPressurePa = pressure

	if !finiteComplex(response.RawRadiated) ||
		!finiteComplex(response.CavityPressurePa) {
		return FrequencyResponse{}, ErrInvalidConfig
	}

	return response, nil
}

func finiteComplex(value complex128) bool {
	return !math.IsNaN(real(value)) && !math.IsNaN(imag(value)) &&
		!math.IsInf(real(value), 0) && !math.IsInf(imag(value), 0) &&
		!math.IsNaN(cmplx.Abs(value))
}
