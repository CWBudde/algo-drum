package physical

import (
	"errors"
	"fmt"
	"math"

	"github.com/cwbudde/algo-dsp/dsp/filter/biquad"
	"github.com/cwbudde/algo-dsp/dsp/filter/design"
)

var ErrInvalidVelocity = errors.New("physical strike velocity must be finite and in [0,1]")

// Output exposes point-pickup head motion, the unfiltered modal radiation sum,
// and the filtered microphone signal separately.
type Output struct {
	DisplacementM     float64
	VelocityMPerS     float64
	ContactForceN     float64
	RawRadiated       float64
	Radiated          float64
	MechanicalEnergyJ float64
}

// SingleHead is the P2 real-time modal prototype. It owns all working memory;
// Trigger, Tick, Reset, and Render perform no allocations.
type SingleHead struct {
	config PhysicalDrum
	modes  []Mode

	displacement []float64
	velocity     []float64
	matrix11     []float64
	matrix12     []float64
	matrix21     []float64
	matrix22     []float64
	strikeWeight []float64
	radiationHP  biquad.Section
	radiationLP  biquad.Section

	contact contact
	energy  float64
}

// NewSingleHead precomputes the circular modes, exact damped state-transition
// matrices, strike projection, pickup weights, and maximum contact storage.
func NewSingleHead(config PhysicalDrum) (*SingleHead, error) {
	modes, err := GenerateModes(config)
	if err != nil {
		return nil, err
	}

	modeCount := len(modes)

	model := &SingleHead{
		config:       config,
		modes:        modes,
		displacement: make([]float64, modeCount),
		velocity:     make([]float64, modeCount),
		matrix11:     make([]float64, modeCount),
		matrix12:     make([]float64, modeCount),
		matrix21:     make([]float64, modeCount),
		matrix22:     make([]float64, modeCount),
		radiationHP: biquad.Section{Coefficients: design.Highpass(
			min(config.Pickup.HighpassHz, config.SampleRateHz*0.45),
			1/math.Sqrt2,
			config.SampleRateHz,
		)},
		radiationLP: biquad.Section{Coefficients: design.Lowpass(
			min(config.Pickup.LowpassHz, config.SampleRateHz*0.45),
			1/math.Sqrt2,
			config.SampleRateHz,
		)},
	}
	for index, mode := range modes {
		matrix11, matrix12, matrix21, matrix22, matrixErr := stateTransition(
			mode.AngularFrequency,
			mode.DecayRatePerSecond,
			config.SampleRateHz,
		)
		if matrixErr != nil {
			return nil, fmt.Errorf(
				"mode (%d,%d,%s): %w",
				mode.AzimuthalOrder,
				mode.RadialOrder,
				mode.Orientation,
				matrixErr,
			)
		}

		model.matrix11[index] = matrix11
		model.matrix12[index] = matrix12
		model.matrix21[index] = matrix21
		model.matrix22[index] = matrix22
	}

	model.strikeWeight = make([]float64, modeCount)
	for index, mode := range modes {
		model.strikeWeight[index] = mode.StrikeAccelerationPerN * mode.ModalMassKg
	}

	model.contact = newContact(config)
	model.contact.setSubsteps(strikePointMassKg(modes))

	return model, nil
}

// ModeCount reports the retained individual oscillator count.
func (s *SingleHead) ModeCount() int { return len(s.modes) }

// Mode returns immutable metadata by value.
func (s *SingleHead) Mode(index int) (Mode, bool) {
	if index < 0 || index >= len(s.modes) {
		return Mode{}, false
	}

	return s.modes[index], true
}

// PulseSamples reports the window the most recent strike's contact acts over.
// Under ContactPrescribed that window is the force pulse; under ContactHertzian
// the force is an output of the model, so it is the interval inside which the
// stick may still be touched by the head. See ContactMetrics for what the
// contact then actually did.
func (s *SingleHead) PulseSamples() int { return s.contact.pulseSamples() }

// LastContact reports the measured duration, count and impulse of the most
// recently completed contact. It is populated only under ContactHertzian, where
// those are results rather than settings.
func (s *SingleHead) LastContact() ContactMetrics { return s.contact.metrics() }

// Trigger starts a finite, velocity- and hardness-dependent contact. Existing
// modal motion is retained, so closely spaced hits superpose instead of
// restarting the drum.
func (s *SingleHead) Trigger(velocity01 float64) error {
	if math.IsNaN(velocity01) || math.IsInf(velocity01, 0) || velocity01 < 0 || velocity01 > 1 {
		return ErrInvalidVelocity
	}

	strikePointM, _ := s.strikePointState()
	s.contact.trigger(velocity01, strikePointM)

	return nil
}

// Reset silences the head and discards any pending contact.
func (s *SingleHead) Reset() {
	clear(s.displacement)
	clear(s.velocity)
	s.contact.reset()
	s.energy = 0
	s.radiationHP.Reset()
	s.radiationLP.Reset()
}

// IsActive reports whether contact is pending or mechanical energy is above
// the configured threshold.
func (s *SingleHead) IsActive() bool {
	return s.contact.isActive() ||
		s.energy > s.config.Batter.InactiveEnergyThresholdJ
}

// Tick advances the exact linear modal state by one sample.
func (s *SingleHead) Tick() Output {
	forceN := s.contact.nextForce(s.strikePointState())

	inverseSampleRate := 1 / s.config.SampleRateHz

	output := Output{ContactForceN: forceN}

	for index, mode := range s.modes {
		oldDisplacement := s.displacement[index]
		// Captured before the contact impulse, so the acceleration below
		// includes it. Taking it afterwards would leave only
		// (matrix22 - 1)*F*a*dt of the strike, which is very nearly nothing.
		previousVelocity := s.velocity[index]
		oldVelocity := previousVelocity +
			forceN*mode.StrikeAccelerationPerN*inverseSampleRate
		newDisplacement := s.matrix11[index]*oldDisplacement +
			s.matrix12[index]*oldVelocity
		newVelocity := s.matrix21[index]*oldDisplacement +
			s.matrix22[index]*oldVelocity

		s.displacement[index] = newDisplacement
		s.velocity[index] = newVelocity

		output.DisplacementM += mode.PickupShape * newDisplacement
		output.VelocityMPerS += mode.PickupShape * newVelocity
		output.RawRadiated += mode.RadiationWeight *
			(newVelocity - previousVelocity) * s.config.SampleRateHz
		output.MechanicalEnergyJ += 0.5 * mode.ModalMassKg *
			(newVelocity*newVelocity +
				mode.AngularFrequency*mode.AngularFrequency*newDisplacement*newDisplacement)
	}

	radiated := s.radiationHP.ProcessSample(output.RawRadiated)
	radiated = s.radiationLP.ProcessSample(radiated)
	output.Radiated = s.config.Pickup.OutputGain * radiated
	s.energy = output.MechanicalEnergyJ

	return output
}

// Render writes the filtered radiated microphone signal into dst.
func (s *SingleHead) Render(dst []float64) {
	for index := range dst {
		dst[index] = s.Tick().Radiated
	}
}

func stateTransition(
	angularFrequency, decayRate, sampleRate float64,
) (float64, float64, float64, float64, error) {
	timeStep := 1 / sampleRate
	if math.Abs(decayRate-angularFrequency) <= angularFrequency*1e-8 {
		decay := math.Exp(-decayRate * timeStep)

		return decay * (1 + decayRate*timeStep),
			decay * timeStep,
			-decay * angularFrequency * angularFrequency * timeStep,
			decay * (1 - decayRate*timeStep),
			nil
	}

	if decayRate > angularFrequency {
		rateDifference := math.Sqrt(
			decayRate*decayRate - angularFrequency*angularFrequency,
		)
		slowRate := -angularFrequency * angularFrequency / (decayRate + rateDifference)
		fastRate := -decayRate - rateDifference
		slowDecay := math.Exp(slowRate * timeStep)
		fastDecay := math.Exp(fastRate * timeStep)
		denominator := slowRate - fastRate

		return (-fastRate*slowDecay + slowRate*fastDecay) / denominator,
			(slowDecay - fastDecay) / denominator,
			angularFrequency * angularFrequency * (fastDecay - slowDecay) / denominator,
			(slowRate*slowDecay - fastRate*fastDecay) / denominator,
			nil
	}

	dampedFrequency := math.Sqrt(angularFrequency*angularFrequency - decayRate*decayRate)
	sine := math.Sin(dampedFrequency * timeStep)
	cosine := math.Cos(dampedFrequency * timeStep)
	decay := math.Exp(-decayRate * timeStep)
	decayOverFrequency := decayRate / dampedFrequency

	return decay * (cosine + decayOverFrequency*sine),
		decay * sine / dampedFrequency,
		-decay * angularFrequency * angularFrequency * sine / dampedFrequency,
		decay * (cosine - decayOverFrequency*sine),
		nil
}

// strikePointState returns the head's displacement and velocity under the
// stick.
//
// The weight is the strike projection multiplied back by the modal mass, which
// is the mode shape times the contact footprint — the same quantity the force is
// distributed over. Using it on the way back guarantees that force times this
// velocity is exactly the power the modes receive, so the contact cannot inject
// or destroy energy through an inconsistent projection.
func (s *SingleHead) strikePointState() (float64, float64) {
	displacementM := 0.0
	velocityMPerS := 0.0

	for index, weight := range s.strikeWeight {
		displacementM += weight * s.displacement[index]
		velocityMPerS += weight * s.velocity[index]
	}

	return displacementM, velocityMPerS
}

// strikePointMassKg is the head's driving-point mass under the stick: the
// instantaneous velocity a unit impulse there produces is its reciprocal. The
// contact integrator sizes its substeps against it.
func strikePointMassKg(modes []Mode) float64 {
	sum := 0.0
	for _, mode := range modes {
		sum += mode.StrikeAccelerationPerN * mode.StrikeAccelerationPerN *
			mode.ModalMassKg
	}

	if sum <= 0 {
		return math.Inf(1)
	}

	return 1 / sum
}
