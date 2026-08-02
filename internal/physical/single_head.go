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

	// Struct-of-arrays mirrors of the five Mode fields Tick reads, for the reason
	// DoubleHead.modeRadiationWeight documents: walking the 144-byte bank to
	// consume 40 bytes per mode drags 13.8 KB through L1 to use 3.8 KB of it.
	// s.modes stays the source of truth — strikeWeight above is the same pattern,
	// already derived rather than read — and these are rebuilt only in
	// NewSingleHead, which is the only place the bank is written.
	//
	// modeOmegaSquared is omega*omega rounded once at construction. The energy
	// term multiplies it by the displacement twice, exactly as the inline form
	// did, so this is bit-identical rather than merely equal to within rounding.
	modePickupShape     []float64
	modeRadiationWeight []float64
	modeModalMassKg     []float64
	modeStrikeAccelPerN []float64
	modeOmegaSquared    []float64

	release     releaseBound
	radiationHP biquad.Section
	radiationLP biquad.Section

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
		release:      newReleaseBound(config.SampleRateHz),
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
	model.modePickupShape = make([]float64, modeCount)
	model.modeRadiationWeight = make([]float64, modeCount)
	model.modeModalMassKg = make([]float64, modeCount)
	model.modeStrikeAccelPerN = make([]float64, modeCount)
	model.modeOmegaSquared = make([]float64, modeCount)

	for index, mode := range modes {
		model.strikeWeight[index] = mode.StrikeAccelerationPerN * mode.ModalMassKg

		model.modePickupShape[index] = mode.PickupShape
		model.modeRadiationWeight[index] = mode.RadiationWeight
		model.modeModalMassKg[index] = mode.ModalMassKg
		model.modeStrikeAccelPerN[index] = mode.StrikeAccelerationPerN
		// The same product Tick used to form per mode per sample.
		model.modeOmegaSquared[index] = mode.AngularFrequency * mode.AngularFrequency
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
	// Re-arms the release deadline; see DoubleHead.Trigger and
	// ReleaseBoundSeconds.
	s.release.restart()

	return nil
}

// Reset silences the head and discards any pending contact.
func (s *SingleHead) Reset() {
	clear(s.displacement)
	clear(s.velocity)
	s.contact.reset()
	s.energy = 0
	s.release.restart()
	s.radiationHP.Reset()
	s.radiationLP.Reset()
}

// IsActive reports whether contact is pending or mechanical energy is above
// the configured threshold, and the release bound has not expired.
//
// SingleHead is an offline reference rather than a product voice, so the bound
// cannot be reached from a knob here. It carries it anyway so that the two
// models answer the same question the same way; see ReleaseBoundSeconds.
func (s *SingleHead) IsActive() bool {
	if s.release.expired() {
		return false
	}

	return s.contact.isActive() ||
		s.energy > s.config.Batter.InactiveEnergyThresholdJ
}

// Tick advances the exact linear modal state by one sample.
func (s *SingleHead) Tick() Output {
	forceN := s.contact.nextForce(s.strikePointState())

	sampleRate := s.config.SampleRateHz
	inverseSampleRate := 1 / sampleRate

	output := Output{ContactForceN: forceN}

	// Hoisted for the reason DoubleHead.solveMidpoint documents: the stores into
	// displacement and velocity may alias s, so read as s.field the headers and
	// config.SampleRateHz are reloaded from the struct on every mode.
	//
	// The five per-mode quantities come from the struct-of-arrays mirrors rather
	// than from s.modes: this loop used to walk the 144-byte bank for 40 useful
	// bytes per mode, and the fields it wanted span offsets +48 to +112, so no
	// mode fitted in one cache line.
	//
	// All eleven are sliced to one common length, and that is not decoration:
	// eleven independent slices indexed by one counter cannot be proven in range
	// from the counter's bound alone, so the naive form emits a bounds check per
	// slice per mode and measures *slower* than the array-of-structs loop it
	// replaces — one pointer into the bank got five fields for one check. Sliced to
	// a common length and ranged over, every check moves out of the loop;
	// `go build -gcflags=-d=ssa/check_bce/debug=1` reports none inside the body.
	modeCount := len(s.modes)
	displacement := s.displacement[:modeCount]
	velocity := s.velocity[:modeCount]
	matrix11, matrix12 := s.matrix11[:modeCount], s.matrix12[:modeCount]
	matrix21, matrix22 := s.matrix21[:modeCount], s.matrix22[:modeCount]
	pickupShape := s.modePickupShape[:modeCount]
	radiationWeight := s.modeRadiationWeight[:modeCount]
	modalMass := s.modeModalMassKg[:modeCount]
	strikeAccel := s.modeStrikeAccelPerN[:modeCount]
	omegaSquared := s.modeOmegaSquared[:modeCount]

	for index := range displacement {
		oldDisplacement := displacement[index]
		// Captured before the contact impulse, so the acceleration below
		// includes it. Taking it afterwards would leave only
		// (matrix22 - 1)*F*a*dt of the strike, which is very nearly nothing.
		previousVelocity := velocity[index]
		oldVelocity := previousVelocity +
			forceN*strikeAccel[index]*inverseSampleRate
		newDisplacement := matrix11[index]*oldDisplacement +
			matrix12[index]*oldVelocity
		newVelocity := matrix21[index]*oldDisplacement +
			matrix22[index]*oldVelocity

		displacement[index] = newDisplacement
		velocity[index] = newVelocity

		shape := pickupShape[index]
		output.DisplacementM += shape * newDisplacement
		output.VelocityMPerS += shape * newVelocity
		output.RawRadiated += radiationWeight[index] *
			(newVelocity - previousVelocity) * sampleRate
		output.MechanicalEnergyJ += 0.5 * modalMass[index] *
			(newVelocity*newVelocity +
				omegaSquared[index]*newDisplacement*newDisplacement)
	}

	radiated := s.radiationHP.ProcessSample(output.RawRadiated)
	radiated = s.radiationLP.ProcessSample(radiated)
	output.Radiated = s.config.Pickup.OutputGain * radiated
	// The release fade, on the microphone signal only; see DoubleHead.Tick.
	output.Radiated *= s.release.advance()
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
