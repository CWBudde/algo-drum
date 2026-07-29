package physical

import (
	"fmt"
	"math"

	"github.com/cwbudde/algo-dsp/dsp/filter/biquad"
	"github.com/cwbudde/algo-dsp/dsp/filter/design"
)

// DoubleHeadOutput exposes the two head pickups, their radiated contributions,
// and the energy stored in the heads and enclosed air.
type DoubleHeadOutput struct {
	BatterDisplacementM          float64
	BatterVelocityMPerS          float64
	ResonantDisplacementM        float64
	ResonantVelocityMPerS        float64
	BatterRawRadiated            float64
	ResonantRawRadiated          float64
	RawRadiated                  float64
	Radiated                     float64
	CavityPressurePa             float64
	SweptVolumeM3                float64
	BatterTensionIncreaseNPerM   float64
	ResonantTensionIncreaseNPerM float64
	LinearHeadMechanicalEnergyJ  float64
	NonlinearPotentialEnergyJ    float64
	NonlinearSolveIterations     int
	HeadMechanicalEnergyJ        float64
	CavityMechanicalEnergyJ      float64
	TotalMechanicalEnergyJ       float64
}

// DoubleHead is the passive two-head, lumped-cavity, and bounded Berger-tension
// model. Its implicit-midpoint/discrete-gradient update conserves the complete
// linear plus nonlinear stored energy when all losses are zero and dissipates
// energy monotonically when head or cavity losses are enabled.
//
// Trigger, Tick, Reset, and Render perform no allocations. Reconfigure
// validates and builds a complete replacement before installing it as one
// owner-goroutine operation; a rejected update leaves the current model
// untouched and a successful update resets all dynamic state.
type DoubleHead struct {
	config          PhysicalDrum
	modes           []Mode
	batterModeCount int

	displacement     []float64
	velocity         []float64
	midpointVelocity []float64
	matrix11         []float64
	matrix12         []float64
	matrix21         []float64
	matrix22         []float64
	midpointDenom    []float64
	pressureGain     []float64
	strainWeight     []float64
	radiationHP      biquad.Section
	radiationLP      biquad.Section

	pulse          []float64
	pendingForce   []float64
	pendingIndex   int
	pendingSamples int

	cavityVolumeM3             float64
	cavityBulkStiffnessPaPerM3 float64
	cavityPressurePa           float64
	batterNonlinear            nonlinearHead
	resonantNonlinear          nonlinearHead
	nonlinearSolveIterations   int
	energy                     float64
}

// NewDoubleHead precomputes two independently tuned modal banks and their
// axisymmetric swept-volume coupling to the enclosed air.
func NewDoubleHead(config PhysicalDrum) (*DoubleHead, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	batterModes, err := generateHeadModes(config, config.Batter)
	if err != nil {
		return nil, fmt.Errorf("batter modes: %w", err)
	}

	var resonantModes []Mode
	if config.Resonant.Enabled {
		resonantModes, err = generateHeadModes(config, config.Resonant)
		if err != nil {
			return nil, fmt.Errorf("resonant modes: %w", err)
		}
	}

	modes := make([]Mode, 0, len(batterModes)+len(resonantModes))
	modes = append(modes, batterModes...)
	modes = append(modes, resonantModes...)

	modeCount := len(modes)
	model := &DoubleHead{
		config:           cloneConfig(config),
		modes:            modes,
		batterModeCount:  len(batterModes),
		displacement:     make([]float64, modeCount),
		velocity:         make([]float64, modeCount),
		midpointVelocity: make([]float64, modeCount),
		matrix11:         make([]float64, modeCount),
		matrix12:         make([]float64, modeCount),
		matrix21:         make([]float64, modeCount),
		matrix22:         make([]float64, modeCount),
		midpointDenom:    make([]float64, modeCount),
		pressureGain:     make([]float64, modeCount),
		strainWeight:     make([]float64, modeCount),
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

	timeStep := 1 / config.SampleRateHz
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

		denominator := 2/timeStep +
			0.5*mode.AngularFrequency*mode.AngularFrequency*timeStep +
			2*mode.DecayRatePerSecond
		model.midpointDenom[index] = denominator
		effectiveSweptArea := config.Cavity.Coupling01 * mode.SweptAreaM2
		model.pressureGain[index] = effectiveSweptArea /
			(mode.ModalMassKg * denominator)

		surfaceDensity := config.Batter.SurfaceDensityKgPerM2
		if index >= len(batterModes) {
			surfaceDensity = config.Resonant.SurfaceDensityKgPerM2
		}

		model.strainWeight[index] = mode.ModalMassKg /
			surfaceDensity * mode.WavenumberPerM * mode.WavenumberPerM
	}

	model.batterNonlinear = newNonlinearHead(
		config.Nonlinearity.Enabled,
		config.Nonlinearity.BatterTensionCoefficientNPerM3,
		config.Nonlinearity.MaximumTensionRatio,
		config.Batter.TensionNPerM,
	)
	model.resonantNonlinear = newNonlinearHead(
		config.Nonlinearity.Enabled && config.Resonant.Enabled,
		config.Nonlinearity.ResonantTensionCoefficientNPerM3,
		config.Nonlinearity.MaximumTensionRatio,
		config.Resonant.TensionNPerM,
	)

	model.cavityVolumeM3 = math.Pi * config.Batter.RadiusM *
		config.Batter.RadiusM * config.Cavity.DepthM
	if config.Cavity.Enabled {
		model.cavityBulkStiffnessPaPerM3 =
			config.Cavity.AirDensityKgPerM3 *
				config.Cavity.SoundSpeedMPerS *
				config.Cavity.SoundSpeedMPerS /
				model.cavityVolumeM3
	}

	model.pulse = contactPulse(config.SampleRateHz, config.Strike.Hardness01)
	model.pendingForce = make([]float64, len(model.pulse))

	return model, nil
}

// Reconfigure safely applies tuning, shell-depth, air, coupling, loss, strike,
// pickup, and quality changes. Successful updates deliberately reset the tail.
func (d *DoubleHead) Reconfigure(config PhysicalDrum) error {
	replacement, err := NewDoubleHead(config)
	if err != nil {
		return err
	}

	*d = *replacement

	return nil
}

// Config returns an independent copy of the active physical parameters.
func (d *DoubleHead) Config() PhysicalDrum {
	return cloneConfig(d.config)
}

// BatterModeCount reports the retained batter-head oscillator count.
func (d *DoubleHead) BatterModeCount() int { return d.batterModeCount }

// ResonantModeCount reports the retained resonant-head oscillator count.
func (d *DoubleHead) ResonantModeCount() int {
	return len(d.modes) - d.batterModeCount
}

// BatterMode returns immutable batter-head metadata by value.
func (d *DoubleHead) BatterMode(index int) (Mode, bool) {
	if index < 0 || index >= d.batterModeCount {
		return Mode{}, false
	}

	return d.modes[index], true
}

// ResonantMode returns immutable resonant-head metadata by value.
func (d *DoubleHead) ResonantMode(index int) (Mode, bool) {
	absoluteIndex := d.batterModeCount + index
	if index < 0 || absoluteIndex >= len(d.modes) {
		return Mode{}, false
	}

	return d.modes[absoluteIndex], true
}

// PulseSamples reports the precomputed strike-contact duration.
func (d *DoubleHead) PulseSamples() int { return len(d.pulse) }

// CavityVolumeM3 reports the ideal cylindrical cavity volume.
func (d *DoubleHead) CavityVolumeM3() float64 { return d.cavityVolumeM3 }

// CavityBulkStiffnessPaPerM3 reports rho*c²/V, or zero when coupling is off.
func (d *DoubleHead) CavityBulkStiffnessPaPerM3() float64 {
	return d.cavityBulkStiffnessPaPerM3
}

// Trigger starts a finite batter-head contact pulse.
func (d *DoubleHead) Trigger(velocity01 float64) error {
	if math.IsNaN(velocity01) || math.IsInf(velocity01, 0) ||
		velocity01 < 0 || velocity01 > 1 {
		return ErrInvalidVelocity
	}

	impulseKgMPerS := d.config.Strike.MalletMassKg *
		d.config.Strike.VelocityMPerS * velocity01

	pulseScale := impulseKgMPerS * d.config.SampleRateHz
	for pulseIndex, sample := range d.pulse {
		pendingIndex := (d.pendingIndex + pulseIndex) % len(d.pendingForce)
		d.pendingForce[pendingIndex] += pulseScale * sample
	}

	d.pendingSamples = len(d.pulse)

	return nil
}

// Reset silences both heads and the cavity and discards pending contact.
func (d *DoubleHead) Reset() {
	clear(d.displacement)
	clear(d.velocity)
	clear(d.midpointVelocity)
	clear(d.pendingForce)
	d.pendingIndex = 0
	d.pendingSamples = 0
	d.cavityPressurePa = 0
	d.batterNonlinear.strainMeasureM2 = 0
	d.resonantNonlinear.strainMeasureM2 = 0
	d.nonlinearSolveIterations = 0
	d.energy = 0
	d.radiationHP.Reset()
	d.radiationLP.Reset()
}

// IsActive reports whether contact is pending or stored energy exceeds either
// enabled head's inactivity threshold.
func (d *DoubleHead) IsActive() bool {
	threshold := d.config.Batter.InactiveEnergyThresholdJ
	if d.config.Resonant.Enabled {
		threshold = min(threshold, d.config.Resonant.InactiveEnergyThresholdJ)
	}

	return d.pendingSamples > 0 || d.energy > threshold
}

// Tick advances both modal banks and the cavity by one sample.
func (d *DoubleHead) Tick() DoubleHeadOutput {
	forceN := d.nextForce()
	if (!d.config.Cavity.Enabled || d.config.Cavity.Coupling01 == 0) &&
		!d.config.Nonlinearity.Enabled {
		return d.tickUncoupled(forceN)
	}

	return d.tickCoupled(forceN)
}

// Render writes the filtered combined radiation signal into dst.
func (d *DoubleHead) Render(dst []float64) {
	for index := range dst {
		dst[index] = d.Tick().Radiated
	}
}

func (d *DoubleHead) nextForce() float64 {
	if d.pendingSamples == 0 {
		return 0
	}

	forceN := d.pendingForce[d.pendingIndex]
	d.pendingForce[d.pendingIndex] = 0

	d.pendingIndex++
	if d.pendingIndex == len(d.pendingForce) {
		d.pendingIndex = 0
	}

	d.pendingSamples--

	return forceN
}

func (d *DoubleHead) tickUncoupled(forceN float64) DoubleHeadOutput {
	inverseSampleRate := 1 / d.config.SampleRateHz
	for index, mode := range d.modes {
		oldVelocity := d.velocity[index]
		if index < d.batterModeCount {
			oldVelocity += forceN * mode.StrikeAccelerationPerN * inverseSampleRate
		}

		oldDisplacement := d.displacement[index]
		d.displacement[index] = d.matrix11[index]*oldDisplacement +
			d.matrix12[index]*oldVelocity
		d.velocity[index] = d.matrix21[index]*oldDisplacement +
			d.matrix22[index]*oldVelocity
	}

	return d.observe()
}

func (d *DoubleHead) tickCoupled(forceN float64) DoubleHeadOutput {
	timeStep := 1 / d.config.SampleRateHz
	batterTension := d.batterNonlinear.tensionAt(
		d.batterNonlinear.strainMeasureM2,
	)
	resonantTension := d.resonantNonlinear.tensionAt(
		d.resonantNonlinear.strainMeasureM2,
	)
	pressureMidpoint := 0.0
	batterStrain := d.batterNonlinear.strainMeasureM2
	resonantStrain := d.resonantNonlinear.strainMeasureM2

	iterationCount := 1
	if d.config.Nonlinearity.Enabled {
		iterationCount = nonlinearSolveIterations
	}

	iterationsUsed := 0
	for range iterationCount {
		iterationsUsed++
		batterStrain, resonantStrain, pressureMidpoint = d.solveMidpoint(forceN, batterTension, resonantTension)
		nextBatterTension := d.batterNonlinear.discreteTension(
			d.batterNonlinear.strainMeasureM2,
			batterStrain,
		)

		nextResonantTension := d.resonantNonlinear.discreteTension(
			d.resonantNonlinear.strainMeasureM2,
			resonantStrain,
		)
		if tensionConverged(
			batterTension,
			nextBatterTension,
			d.batterNonlinear.maxTensionNPerM,
		) && tensionConverged(
			resonantTension,
			nextResonantTension,
			d.resonantNonlinear.maxTensionNPerM,
		) {
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

	for index := range d.modes {
		midpointVelocity := d.midpointVelocity[index]
		d.displacement[index] += timeStep * midpointVelocity
		d.velocity[index] = 2*midpointVelocity - d.velocity[index]
	}

	if d.config.Cavity.Enabled {
		d.cavityPressurePa = 2*pressureMidpoint - d.cavityPressurePa
	}

	d.batterNonlinear.strainMeasureM2 = batterStrain
	d.resonantNonlinear.strainMeasureM2 = resonantStrain

	return d.observe()
}

func (d *DoubleHead) solveMidpoint(
	forceN, batterTensionNPerM, resonantTensionNPerM float64,
) (float64, float64, float64) {
	timeStep := 1 / d.config.SampleRateHz
	inverseTimeStep := d.config.SampleRateHz
	sweptMidpointVelocity := 0.0
	pressureFeedback := 0.0

	for index, mode := range d.modes {
		surfaceDensity := d.config.Batter.SurfaceDensityKgPerM2
		tensionIncrease := batterTensionNPerM

		if index >= d.batterModeCount {
			surfaceDensity = d.config.Resonant.SurfaceDensityKgPerM2
			tensionIncrease = resonantTensionNPerM
		}

		nonlinearAngularFrequencySquared := tensionIncrease /
			surfaceDensity * mode.WavenumberPerM * mode.WavenumberPerM
		angularFrequencySquared := mode.AngularFrequency*
			mode.AngularFrequency + nonlinearAngularFrequencySquared
		denominator := d.midpointDenom[index] +
			0.5*nonlinearAngularFrequencySquared*timeStep

		numerator := 2*d.velocity[index]*inverseTimeStep -
			angularFrequencySquared*d.displacement[index]
		if index < d.batterModeCount {
			numerator += forceN * mode.StrikeAccelerationPerN
		}

		uncoupledMidpointVelocity := numerator / denominator
		d.midpointVelocity[index] = uncoupledMidpointVelocity
		effectiveSweptArea := d.config.Cavity.Coupling01 * mode.SweptAreaM2
		d.pressureGain[index] = effectiveSweptArea /
			(mode.ModalMassKg * denominator)
		sweptMidpointVelocity += effectiveSweptArea * uncoupledMidpointVelocity
		pressureFeedback += effectiveSweptArea * d.pressureGain[index]
	}

	pressureMidpoint := 0.0

	if d.config.Cavity.Enabled {
		stiffness := d.cavityBulkStiffnessPaPerM3
		pressureMidpoint = (2*d.cavityPressurePa*inverseTimeStep +
			stiffness*sweptMidpointVelocity) /
			(2*inverseTimeStep + d.config.Cavity.LossPerSecond +
				stiffness*pressureFeedback)
	}

	batterStrain := 0.0
	resonantStrain := 0.0

	for index := range d.modes {
		midpointVelocity := d.midpointVelocity[index] -
			d.pressureGain[index]*pressureMidpoint
		d.midpointVelocity[index] = midpointVelocity
		newDisplacement := d.displacement[index] +
			timeStep*midpointVelocity

		strain := d.strainWeight[index] *
			newDisplacement * newDisplacement
		if index < d.batterModeCount {
			batterStrain += strain
		} else {
			resonantStrain += strain
		}
	}

	return batterStrain, resonantStrain, pressureMidpoint
}

func tensionConverged(current, next, maximum float64) bool {
	return math.Abs(next-current) <=
		nonlinearSolveTolerance*max(1, maximum)
}

func (d *DoubleHead) observe() DoubleHeadOutput {
	var output DoubleHeadOutput

	batterStrain := 0.0
	resonantStrain := 0.0

	for index, mode := range d.modes {
		displacement := d.displacement[index]
		velocity := d.velocity[index]
		pickupDisplacement := mode.PickupShape * displacement
		pickupVelocity := mode.PickupShape * velocity
		rawRadiated := mode.RadiationWeight * velocity

		if index < d.batterModeCount {
			output.BatterDisplacementM += pickupDisplacement
			output.BatterVelocityMPerS += pickupVelocity
			output.BatterRawRadiated += rawRadiated
			batterStrain += d.strainWeight[index] *
				displacement * displacement
		} else {
			output.ResonantDisplacementM += pickupDisplacement
			output.ResonantVelocityMPerS += pickupVelocity
			output.ResonantRawRadiated += rawRadiated
			resonantStrain += d.strainWeight[index] *
				displacement * displacement
		}

		output.SweptVolumeM3 += d.config.Cavity.Coupling01 *
			mode.SweptAreaM2 * displacement
		output.LinearHeadMechanicalEnergyJ += 0.5 * mode.ModalMassKg *
			(velocity*velocity +
				mode.AngularFrequency*mode.AngularFrequency*
					displacement*displacement)
	}

	d.batterNonlinear.strainMeasureM2 = batterStrain
	d.resonantNonlinear.strainMeasureM2 = resonantStrain
	output.BatterTensionIncreaseNPerM = d.batterNonlinear.tensionAt(batterStrain)
	output.ResonantTensionIncreaseNPerM = d.resonantNonlinear.tensionAt(resonantStrain)
	output.NonlinearPotentialEnergyJ =
		d.batterNonlinear.potentialEnergy(batterStrain) +
			d.resonantNonlinear.potentialEnergy(resonantStrain)
	output.HeadMechanicalEnergyJ = output.LinearHeadMechanicalEnergyJ +
		output.NonlinearPotentialEnergyJ
	output.NonlinearSolveIterations = d.nonlinearSolveIterations

	if d.cavityBulkStiffnessPaPerM3 > 0 {
		output.CavityMechanicalEnergyJ = 0.5 *
			d.cavityPressurePa * d.cavityPressurePa /
			d.cavityBulkStiffnessPaPerM3
	}

	output.TotalMechanicalEnergyJ = output.HeadMechanicalEnergyJ +
		output.CavityMechanicalEnergyJ
	output.CavityPressurePa = d.cavityPressurePa
	output.RawRadiated = output.BatterRawRadiated +
		output.ResonantRawRadiated

	radiated := d.radiationHP.ProcessSample(output.RawRadiated)
	radiated = d.radiationLP.ProcessSample(radiated)
	output.Radiated = d.config.Pickup.OutputGain * radiated
	d.energy = output.TotalMechanicalEnergyJ

	return output
}

func cloneConfig(config PhysicalDrum) PhysicalDrum {
	config.Batter.ModeDecayCorrections = append([]ModeDecayCorrection(nil), config.Batter.ModeDecayCorrections...)
	config.Resonant.ModeDecayCorrections = append([]ModeDecayCorrection(nil), config.Resonant.ModeDecayCorrections...)

	return config
}
