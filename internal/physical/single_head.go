package physical

import (
	"errors"
	"fmt"
	"math"
)

const (
	softContactSeconds = 0.008
	hardContactSeconds = 0.00025
)

var ErrInvalidVelocity = errors.New("physical strike velocity must be finite and in [0,1]")

// Output exposes diagnostic head motion separately from the provisional
// radiated pickup signal.
type Output struct {
	DisplacementM     float64
	VelocityMPerS     float64
	Radiated          float64
	MechanicalEnergyJ float64
}

// SingleHead is the P1 real-time modal prototype. It owns all working memory;
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

	pulse      []float64
	pulseIndex int
	pulseScale float64
	energy     float64
}

// NewSingleHead precomputes the circular modes, exact damped state-transition
// matrices, strike projection, pickup weights, and normalized contact pulse.
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

	model.pulse = contactPulse(config.SampleRateHz, config.Strike.Hardness01)
	model.pulseIndex = len(model.pulse)

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

// PulseSamples reports the precomputed strike-contact duration.
func (s *SingleHead) PulseSamples() int { return len(s.pulse) }

// Trigger starts a finite contact pulse. Existing modal motion is retained, so
// closely spaced hits superpose instead of restarting the drum.
func (s *SingleHead) Trigger(velocity01 float64) error {
	if math.IsNaN(velocity01) || math.IsInf(velocity01, 0) || velocity01 < 0 || velocity01 > 1 {
		return ErrInvalidVelocity
	}

	s.pulseIndex = 0
	impulseKgMPerS := s.config.Strike.MalletMassKg * s.config.Strike.VelocityMPerS * velocity01
	s.pulseScale = impulseKgMPerS * s.config.SampleRateHz

	return nil
}

// Reset silences the head and discards any pending contact pulse.
func (s *SingleHead) Reset() {
	clear(s.displacement)
	clear(s.velocity)
	s.pulseIndex = len(s.pulse)
	s.pulseScale = 0
	s.energy = 0
}

// IsActive reports whether contact is pending or mechanical energy is above
// the configured threshold.
func (s *SingleHead) IsActive() bool {
	return s.pulseIndex < len(s.pulse) ||
		s.energy > s.config.Batter.InactiveEnergyThresholdJ
}

// Tick advances the exact linear modal state by one sample.
func (s *SingleHead) Tick() Output {
	forceN := 0.0
	if s.pulseIndex < len(s.pulse) {
		forceN = s.pulseScale * s.pulse[s.pulseIndex]
		s.pulseIndex++
	}

	inverseSampleRate := 1 / s.config.SampleRateHz

	var output Output

	for index, mode := range s.modes {
		oldDisplacement := s.displacement[index]
		oldVelocity := s.velocity[index] +
			forceN*mode.StrikeAccelerationPerN*inverseSampleRate
		newDisplacement := s.matrix11[index]*oldDisplacement +
			s.matrix12[index]*oldVelocity
		newVelocity := s.matrix21[index]*oldDisplacement +
			s.matrix22[index]*oldVelocity

		s.displacement[index] = newDisplacement
		s.velocity[index] = newVelocity

		output.DisplacementM += mode.PickupShape * newDisplacement
		output.VelocityMPerS += mode.PickupShape * newVelocity
		output.MechanicalEnergyJ += 0.5 * mode.ModalMassKg *
			(newVelocity*newVelocity +
				mode.AngularFrequency*mode.AngularFrequency*newDisplacement*newDisplacement)
	}

	output.Radiated = s.config.Pickup.OutputGain * output.VelocityMPerS
	s.energy = output.MechanicalEnergyJ

	return output
}

// Render writes the provisional radiated signal into dst.
func (s *SingleHead) Render(dst []float64) {
	for index := range dst {
		dst[index] = s.Tick().Radiated
	}
}

func stateTransition(
	angularFrequency, decayRate, sampleRate float64,
) (float64, float64, float64, float64, error) {
	if decayRate >= angularFrequency {
		return 0, 0, 0, 0, fmt.Errorf(
			"%w: overdamped mode decay=%v angular frequency=%v",
			ErrInvalidConfig,
			decayRate,
			angularFrequency,
		)
	}

	timeStep := 1 / sampleRate
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

func contactPulse(sampleRate, hardness01 float64) []float64 {
	duration := softContactSeconds *
		math.Pow(hardContactSeconds/softContactSeconds, hardness01)
	sampleCount := max(2, int(math.Round(duration*sampleRate)))
	pulse := make([]float64, sampleCount)
	sum := 0.0

	for index := range pulse {
		pulse[index] = math.Sin(math.Pi * (float64(index) + 0.5) / float64(sampleCount))
		sum += pulse[index]
	}

	for index := range pulse {
		pulse[index] /= sum
	}

	return pulse
}
