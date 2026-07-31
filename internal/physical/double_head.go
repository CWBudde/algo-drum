package physical

import (
	"fmt"
	"math"

	"github.com/cwbudde/algo-dsp/dsp/filter/biquad"
	"github.com/cwbudde/algo-dsp/dsp/filter/design"
)

// DoubleHeadOutput exposes the two head pickups, their separate radiated
// contributions, the batter-side microphone signal, and the energy stored in
// the heads and enclosed air.
type DoubleHeadOutput struct {
	BatterDisplacementM          float64
	BatterVelocityMPerS          float64
	ResonantDisplacementM        float64
	ResonantVelocityMPerS        float64
	BatterRawRadiated            float64
	ResonantRawRadiated          float64
	AttackRawRadiated            float64
	ContactForceN                float64
	RawRadiated                  float64
	Radiated                     float64
	CavityPressurePa             float64
	SweptVolumeM3                float64
	BatterTensionIncreaseNPerM   float64
	ResonantTensionIncreaseNPerM float64
	LinearHeadMechanicalEnergyJ  float64
	NonlinearPotentialEnergyJ    float64
	CouplingPotentialEnergyJ     float64
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
	strainWeight     []float64
	strikeWeight     []float64
	radiationHP      biquad.Section
	radiationLP      biquad.Section
	attack           attackLayer

	contact contact

	// The radiated sums are formed inside the update loops, where the discrete
	// acceleration is available without keeping a second velocity history, and
	// read back by observe. A consequence worth knowing: observe called on its
	// own, without a preceding update, reports the previous tick's radiation.
	batterRadiatedM3PerS2   float64
	resonantRadiatedM3PerS2 float64
	attackRadiatedM3PerS2   float64

	// The enclosed air. cavityPressurePa holds one pressure state per retained
	// cavity mode and cavityFlowPa its conjugate, so that (P_c, H_c) is a
	// harmonic pair at the mode's own frequency; the uniform mode has zero
	// frequency, so its H stays at zero and the pair degenerates into the single
	// compliance the lumped model used to be.
	//
	// couplingFirst/couplingCount index a run of (cavity index, coefficient)
	// pairs per head mode. The azimuthal selection rule makes that run short —
	// a head mode couples only to cavity modes of its own azimuthal order — so
	// the k x k system the midpoint solve assembles is mostly empty and is never
	// walked densely.
	cavityModes                []CavityMode
	cavityPressurePa           []float64
	cavityFlowPa               []float64
	cavityMidpointPa           []float64
	cavityDrive                []float64
	cavityMatrix               []float64
	couplingFirst              []int32
	couplingCount              []int32
	couplingCavity             []int32
	couplingAreaM2             []float64
	couplingGain               []float64
	cavityVolumeM3             float64
	cavityBulkStiffnessPaPerM3 float64
	batterNonlinear            nonlinearHead
	resonantNonlinear          nonlinearHead
	nonlinearSolveIterations   int
	energy                     float64

	// The nonlinear mode coupling: the channels of the local quartic potential
	// the Berger law projects away. coupling is empty unless the coupling is
	// configured, and every loop below is guarded on that rather than multiplied
	// by zero, so a disabled coupling leaves the shipped arithmetic untouched
	// sample for sample.
	//
	// channelValue holds g_c at the current state, channelTrial the fixed-point
	// solve's endpoint guess, and channelTension the discrete gradient
	// T_c = beta_tilde (g_c^{n+1} + g_c^n)/2 the modal force is formed from.
	// couplingBar is the midpoint displacement the force is evaluated at and
	// couplingAccel the resulting modal acceleration.
	coupling            couplingTable
	couplingActive      bool
	couplingBar         []float64
	couplingEnd         []float64
	couplingAccel       []float64
	couplingInverseMass []float64
	channelValue        []float64
	channelTrial        []float64
	channelTension      []float64
	channelTensionScale float64
	// couplingDivergedSteps counts samples whose coupled fixed point was
	// abandoned and re-solved without the coupling. See tickCoupled.
	couplingDivergedSteps uint64
}

// NewDoubleHead precomputes two independently tuned modal banks, the enclosed
// air's modal basis, and the overlap integrals that couple them.
//
// The coupling coefficient is the overlap of a head mode shape against a cavity
// mode shape over the head's disc. Its azimuthal factor vanishes unless the two
// azimuthal orders match, which is the only reason this is affordable: a head
// mode couples to at most one cavity mode per radial order rather than to all of
// them. Against the uniform cavity mode the overlap collapses to the mode's
// signed swept area, which is why a one-mode cavity is the lumped compliance this
// model used to carry, exactly and not approximately.
//
// With only the uniform mode retained, every m > 0 head mode has an identically
// zero coefficient and neither drives the air nor is driven by it — the property
// Head.AxisymmetricOnly rests on. That was never a fact about drums: a real
// cylindrical cavity has transverse modes, the j'_mn series, and those couple to
// m > 0 head modes with a coefficient that is not zero. Enabling them is what
// Cavity.ModeCount above 1 does. See docs/physical-cavity.md.
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
		strainWeight:     make([]float64, modeCount),
		couplingFirst:    make([]int32, modeCount),
		couplingCount:    make([]int32, modeCount),
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

	model.attack = newAttackLayer(config.Attack, config.Batter, config.SampleRateHz)

	model.cavityVolumeM3 = math.Pi * config.Batter.RadiusM *
		config.Batter.RadiusM * config.Cavity.DepthM

	cavityModes, err := generateCavityModes(config)
	if err != nil {
		return nil, fmt.Errorf("cavity modes: %w", err)
	}

	model.installCavity(config, cavityModes, len(batterModes))

	// Batter modes only: the stick never touches the resonant head, and the
	// cavity that couples them carries no transverse force to the strike point.
	model.strikeWeight = make([]float64, len(batterModes))
	for index, mode := range batterModes {
		model.strikeWeight[index] = mode.StrikeAccelerationPerN * mode.ModalMassKg
	}

	model.installCoupling(config, batterModes)

	model.contact = newContact(config)
	model.contact.setSubsteps(strikePointMassKg(batterModes))

	return model, nil
}

// installCoupling builds the quartic channel table and its audio-path scratch.
//
// Batter modes only, and deliberately: the resonant head is never struck, its
// bank is reachability-reduced to what the cavity can drive, and a cubic source
// term there would be spending the whole coupling budget on modes that are two
// coupling stages away from any excitation.
//
// With the coupling disabled this installs a zero-length table and leaves
// couplingActive false, which is the checkable limit the bit-exactness test
// rests on: Tick then takes the path that shipped, not a coupled path multiplied
// by zero.
func (d *DoubleHead) installCoupling(config PhysicalDrum, batterModes []Mode) {
	d.coupling = buildCouplingTable(config, batterModes)
	if !d.coupling.active() {
		d.coupling = couplingTable{}

		return
	}

	d.couplingActive = true
	d.couplingBar = make([]float64, len(batterModes))
	d.couplingEnd = make([]float64, len(batterModes))
	d.couplingAccel = make([]float64, len(batterModes))
	d.couplingInverseMass = make([]float64, len(batterModes))

	for index := range batterModes {
		d.couplingInverseMass[index] = 1 / batterModes[index].ModalMassKg
	}

	d.channelValue = make([]float64, d.coupling.channelCount)
	d.channelTrial = make([]float64, d.coupling.channelCount)
	d.channelTension = make([]float64, d.coupling.channelCount)
	// The convergence test compares channel tensions, which carry the same N/m
	// units as the Berger tension, against the same absolute floor the head
	// tensions use.
	d.channelTensionScale = max(1, d.batterNonlinear.maxTensionNPerM)
}

// CouplingCoefficientCount reports the retained quartic coefficients. Zero means
// the coupling is off and the model is the one that shipped.
func (d *DoubleHead) CouplingCoefficientCount() int {
	return len(d.coupling.entryValue)
}

// CouplingChannelCount reports the retained potential channels beyond the
// uniform one the Berger law already carries.
func (d *DoubleHead) CouplingChannelCount() int { return d.coupling.channelCount }

// CouplingPumpModes reports the batter-mode indices the channel set was built
// from, in selection order.
func (d *DoubleHead) CouplingPumpModes() []int {
	return append([]int(nil), d.coupling.pumpIndices...)
}

// CouplingWorstForceHz reports the highest frequency the retained cubic force
// can reach, measured on the table that was actually built rather than on the
// conservative bound Validate applies.
func (d *DoubleHead) CouplingWorstForceHz() float64 {
	return d.coupling.worstForceFrequencyHz
}

// CouplingDivergedSteps reports how many samples since the last Reset had their
// coupled fixed point abandoned and re-solved without the coupling. On any
// configuration the validator admits this stays zero; a non-zero count means the
// coefficient is too large for the step at the amplitude being played, and what
// was heard was the Berger-only law for those samples.
func (d *DoubleHead) CouplingDivergedSteps() uint64 {
	return d.couplingDivergedSteps
}

// installCavity stores the enclosed-air basis and precomputes every non-zero
// head/cavity overlap integral, grouped so that each head mode owns a contiguous
// run. Cavity modes are ordered by azimuthal family, so a head mode's partners
// are always adjacent and the run is a slice rather than a scatter.
func (d *DoubleHead) installCavity(
	config PhysicalDrum,
	cavityModes []CavityMode,
	batterModeCount int,
) {
	cavityCount := len(cavityModes)
	d.cavityModes = cavityModes
	d.cavityPressurePa = make([]float64, cavityCount)
	d.cavityFlowPa = make([]float64, cavityCount)
	d.cavityMidpointPa = make([]float64, cavityCount)
	d.cavityDrive = make([]float64, cavityCount)
	d.cavityMatrix = make([]float64, cavityCount*cavityCount)
	d.cavityBulkStiffnessPaPerM3 = cavityModes[0].StiffnessPaPerM3

	shellRadiusM := config.Batter.RadiusM
	for index, mode := range d.modes {
		head := config.Batter
		if index >= batterModeCount {
			head = config.Resonant
		}

		d.couplingFirst[index] = int32(len(d.couplingAreaM2))

		for cavityIndex, cavity := range cavityModes {
			coefficient := HeadCavityCouplingM2(head, shellRadiusM, mode, cavity)
			if coefficient == 0 {
				continue
			}

			// Scaling drive and feedback by the same product control is what
			// keeps the coupling passive at any setting; see docs.
			d.couplingAreaM2 = append(
				d.couplingAreaM2,
				config.Cavity.Coupling01*coefficient,
			)
			d.couplingCavity = append(d.couplingCavity, int32(cavityIndex))
		}

		d.couplingCount[index] = int32(len(d.couplingAreaM2)) -
			d.couplingFirst[index]
	}

	d.couplingGain = make([]float64, len(d.couplingAreaM2))
}

// CavityModeCount reports the retained enclosed-air pressure states.
func (d *DoubleHead) CavityModeCount() int { return len(d.cavityModes) }

// CavityMode returns immutable enclosed-air mode metadata by value.
func (d *DoubleHead) CavityMode(index int) (CavityMode, bool) {
	if index < 0 || index >= len(d.cavityModes) {
		return CavityMode{}, false
	}

	return d.cavityModes[index], true
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

// PulseSamples reports the window the most recent strike's contact acts over.
// Under ContactPrescribed that window is the force pulse; under ContactHertzian
// the force is an output of the model, so it is the interval inside which the
// stick may still be touched by the head.
func (d *DoubleHead) PulseSamples() int { return d.contact.pulseSamples() }

// LastContact reports the measured duration, count and impulse of the most
// recently completed contact. It is populated only under ContactHertzian, where
// those are results rather than settings.
func (d *DoubleHead) LastContact() ContactMetrics { return d.contact.metrics() }

// strikePointState returns the batter head's displacement and velocity under
// the stick. See the note on SingleHead.strikePointState for why the weight is
// the strike projection multiplied back by the modal mass.
func (d *DoubleHead) strikePointState() (float64, float64) {
	displacementM := 0.0
	velocityMPerS := 0.0

	for index, weight := range d.strikeWeight {
		displacementM += weight * d.displacement[index]
		velocityMPerS += weight * d.velocity[index]
	}

	return displacementM, velocityMPerS
}

// CavityVolumeM3 reports the ideal cylindrical cavity volume.
func (d *DoubleHead) CavityVolumeM3() float64 { return d.cavityVolumeM3 }

// CavityBulkStiffnessPaPerM3 reports the fitted stiffness scale times rho*c²/V,
// or zero when coupling is off.
func (d *DoubleHead) CavityBulkStiffnessPaPerM3() float64 {
	return d.cavityBulkStiffnessPaPerM3
}

// Trigger starts a finite batter-head contact.
func (d *DoubleHead) Trigger(velocity01 float64) error {
	if math.IsNaN(velocity01) || math.IsInf(velocity01, 0) ||
		velocity01 < 0 || velocity01 > 1 {
		return ErrInvalidVelocity
	}

	strikePointM, _ := d.strikePointState()
	d.contact.trigger(velocity01, strikePointM)

	return nil
}

// Reset silences both heads and the cavity and discards pending contact.
func (d *DoubleHead) Reset() {
	clear(d.displacement)
	clear(d.velocity)
	clear(d.midpointVelocity)
	d.contact.reset()
	d.attackRadiatedM3PerS2 = 0
	clear(d.cavityPressurePa)
	clear(d.cavityFlowPa)
	clear(d.cavityMidpointPa)
	d.batterNonlinear.strainMeasureM2 = 0
	d.resonantNonlinear.strainMeasureM2 = 0
	d.nonlinearSolveIterations = 0
	clear(d.channelValue)
	clear(d.channelTrial)
	clear(d.channelTension)
	clear(d.couplingBar)
	clear(d.couplingEnd)
	clear(d.couplingAccel)
	d.couplingDivergedSteps = 0
	d.batterRadiatedM3PerS2 = 0
	d.resonantRadiatedM3PerS2 = 0
	d.attack.reset()
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

	return d.contact.isActive() || d.energy > threshold ||
		d.attack.isRinging()
}

// Tick advances both modal banks and the cavity by one sample.
func (d *DoubleHead) Tick() DoubleHeadOutput {
	forceN := d.contact.nextForce(d.strikePointState())
	// Advanced before the modal update so the layer sees the same contact sample
	// the modes do, and so it keeps running once the pulse is over.
	d.attackRadiatedM3PerS2 = d.attack.tick(forceN)

	var output DoubleHeadOutput
	if (!d.config.Cavity.Enabled || d.config.Cavity.Coupling01 == 0) &&
		!d.config.Nonlinearity.Enabled {
		output = d.tickUncoupled(forceN)
	} else {
		output = d.tickCoupled(forceN)
	}

	output.ContactForceN = forceN

	return output
}

// Render writes the filtered batter-side microphone signal into dst.
func (d *DoubleHead) Render(dst []float64) {
	for index := range dst {
		dst[index] = d.Tick().Radiated
	}
}

func (d *DoubleHead) tickUncoupled(forceN float64) DoubleHeadOutput {
	inverseSampleRate := 1 / d.config.SampleRateHz
	batterRadiated := 0.0
	resonantRadiated := 0.0

	for index, mode := range d.modes {
		// Captured before the contact impulse, so the acceleration below
		// includes it. Taking it afterwards would leave only
		// (matrix22 - 1)*F*a*dt of the strike, which is very nearly nothing.
		previousVelocity := d.velocity[index]

		oldVelocity := previousVelocity
		if index < d.batterModeCount {
			oldVelocity += forceN * mode.StrikeAccelerationPerN * inverseSampleRate
		}

		oldDisplacement := d.displacement[index]
		d.displacement[index] = d.matrix11[index]*oldDisplacement +
			d.matrix12[index]*oldVelocity
		newVelocity := d.matrix21[index]*oldDisplacement +
			d.matrix22[index]*oldVelocity
		d.velocity[index] = newVelocity

		// Written identically in SingleHead.Tick. The zero-coupling equivalence
		// test compares the two to the last bit, so the operand order matters.
		radiated := mode.RadiationWeight *
			(newVelocity - previousVelocity) * d.config.SampleRateHz
		if index < d.batterModeCount {
			batterRadiated += radiated
		} else {
			resonantRadiated += radiated
		}
	}

	d.batterRadiatedM3PerS2 = batterRadiated
	d.resonantRadiatedM3PerS2 = resonantRadiated

	return d.observe(false)
}

func (d *DoubleHead) tickCoupled(forceN float64) DoubleHeadOutput {
	timeStep := 1 / d.config.SampleRateHz

	batterStrain, resonantStrain, diverged := d.solveNonlinearStep(
		forceN,
		d.couplingActive,
	)
	// The coupled fixed point stopped contracting, so its iterate means nothing
	// and the head state it would write is arbitrary. Nothing has been committed
	// yet — solveNonlinearStep reads d.displacement and d.velocity and writes
	// only scratch — so the step is simply re-solved from the same pre-step
	// state with the coupling switched off, which lands on the Berger-only
	// update whose stability is unconditional. See couplingResidualGrowth.
	if diverged {
		d.couplingDivergedSteps++

		batterStrain, resonantStrain, _ = d.solveNonlinearStep(forceN, false)
	}

	batterRadiated := 0.0
	resonantRadiated := 0.0

	for index := range d.modes {
		mode := &d.modes[index]

		midpointVelocity := d.midpointVelocity[index]
		d.displacement[index] += timeStep * midpointVelocity
		newVelocity := 2*midpointVelocity - d.velocity[index]
		d.velocity[index] = newVelocity

		// For the midpoint rule v_new - v_old = 2*(v_new - v_mid), so this is
		// the same discrete acceleration the uncoupled path forms directly. The
		// contact force is included even though it never appears as a velocity
		// increment here: it enters solveMidpoint as an acceleration, v_mid
		// gains F*a*dt/2 from it, and the reflection above doubles that.
		radiated := mode.RadiationWeight *
			2 * (newVelocity - midpointVelocity) * d.config.SampleRateHz
		if index < d.batterModeCount {
			batterRadiated += radiated
		} else {
			resonantRadiated += radiated
		}
	}

	d.batterRadiatedM3PerS2 = batterRadiated
	d.resonantRadiatedM3PerS2 = resonantRadiated

	if d.config.Cavity.Enabled {
		timeStep := 1 / d.config.SampleRateHz
		for index := range d.cavityModes {
			midpoint := d.cavityMidpointPa[index]
			d.cavityPressurePa[index] = 2*midpoint - d.cavityPressurePa[index]
			// Hdot = -omega*P integrated by the same midpoint rule, so
			// H_new = H_old - omega*dt*P_mid. The uniform mode has omega = 0 and
			// its H therefore never leaves zero, which is what collapses this
			// pair back onto the lumped compliance.
			d.cavityFlowPa[index] -= d.cavityModes[index].AngularFrequency *
				timeStep * midpoint
		}
	}

	d.batterNonlinear.strainMeasureM2 = batterStrain
	d.resonantNonlinear.strainMeasureM2 = resonantStrain

	// The displacement committed above is exactly the couplingEnd the last
	// solveMidpoint evaluated the channels at, so channelTrial is current.
	return d.observe(true)
}

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
			d.batterNonlinear.strainMeasureM2,
			batterStrain,
		)

		nextResonantTension := d.resonantNonlinear.discreteTension(
			d.resonantNonlinear.strainMeasureM2,
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
	clear(d.couplingAccel)

	for index := range d.coupling.runs {
		run := &d.coupling.runs[index]

		tension := d.channelTension[run.channel]
		if tension == 0 {
			continue
		}

		// Constant across the run, which is the point of iterating over runs.
		row := run.row
		barRow := d.couplingBar[row]
		rowTotal := 0.0

		for slot := run.first; slot < run.last; slot++ {
			column := d.coupling.entryColumn[slot]
			scaled := tension * d.coupling.entryValue[slot]

			rowTotal += scaled * d.couplingBar[column]
			if row != column {
				d.couplingAccel[column] -= scaled * barRow *
					d.couplingInverseMass[column]
			}
		}

		d.couplingAccel[row] -= rowTotal * d.couplingInverseMass[row]
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

	for index := range d.coupling.runs {
		run := &d.coupling.runs[index]

		total := 0.0
		for slot := run.first; slot < run.last; slot++ {
			total += d.coupling.entryDoubledValue[slot] *
				displacement[d.coupling.entryColumn[slot]]
		}

		dst[run.channel] += displacement[run.row] * total
	}
}

func (d *DoubleHead) solveMidpoint(
	forceN, batterTensionNPerM, resonantTensionNPerM float64,
) (float64, float64) {
	timeStep := 1 / d.config.SampleRateHz
	inverseTimeStep := d.config.SampleRateHz

	// Hoisted out of the loop below: this is the innermost code the model has,
	// run once per mode per nonlinear iteration per sample, and each d.config
	// reach walked a nested struct to reload a constant.
	batterDensity := d.config.Batter.SurfaceDensityKgPerM2
	resonantDensity := d.config.Resonant.SurfaceDensityKgPerM2

	cavityCount := len(d.cavityModes)
	clear(d.cavityDrive)
	clear(d.cavityMatrix)

	for index := range d.modes {
		mode := &d.modes[index]

		surfaceDensity := batterDensity
		tensionIncrease := batterTensionNPerM

		if index >= d.batterModeCount {
			surfaceDensity = resonantDensity
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
			if d.couplingActive {
				numerator += d.couplingAccel[index]
			}
		}

		uncoupledMidpointVelocity := numerator / denominator
		d.midpointVelocity[index] = uncoupledMidpointVelocity

		first := int(d.couplingFirst[index])

		last := first + int(d.couplingCount[index])
		if first == last {
			continue
		}

		modalDenominator := mode.ModalMassKg * denominator
		for slot := first; slot < last; slot++ {
			d.couplingGain[slot] = d.couplingAreaM2[slot] / modalDenominator
		}

		// Both accumulations are restricted to this mode's own azimuthal family,
		// so the k x k feedback matrix is filled block by block and the loop is
		// linear in the retained mode count exactly as the rank-one form was.
		for slot := first; slot < last; slot++ {
			area := d.couplingAreaM2[slot]
			row := int(d.couplingCavity[slot]) * cavityCount

			d.cavityDrive[int(d.couplingCavity[slot])] += area *
				uncoupledMidpointVelocity
			for other := first; other < last; other++ {
				d.cavityMatrix[row+int(d.couplingCavity[other])] += area *
					d.couplingGain[other]
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

	for index := range d.modes {
		midpointVelocity := d.midpointVelocity[index]

		first := int(d.couplingFirst[index])
		for slot, last := first, first+int(d.couplingCount[index]); slot < last; slot++ {
			midpointVelocity -= d.couplingGain[slot] *
				d.cavityMidpointPa[int(d.couplingCavity[slot])]
		}

		d.midpointVelocity[index] = midpointVelocity
		newDisplacement := d.displacement[index] +
			timeStep*midpointVelocity

		strain := d.strainWeight[index] *
			newDisplacement * newDisplacement
		if index < d.batterModeCount {
			batterStrain += strain

			if d.couplingActive {
				d.couplingEnd[index] = newDisplacement
				d.couplingBar[index] = d.displacement[index] +
					0.5*timeStep*midpointVelocity
			}
		} else {
			resonantStrain += strain
		}
	}

	if d.couplingActive {
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

	for index := range cavityCount {
		mode := &d.cavityModes[index]
		row := index * cavityCount

		stiffness := mode.StiffnessPaPerM3
		for column := range cavityCount {
			d.cavityMatrix[row+column] *= stiffness
		}

		d.cavityMatrix[row+index] += 2*inverseTimeStep + lossPerSecond +
			0.5*mode.AngularFrequency*mode.AngularFrequency*timeStep
		d.cavityDrive[index] = 2*d.cavityPressurePa[index]*inverseTimeStep +
			mode.AngularFrequency*d.cavityFlowPa[index] +
			stiffness*d.cavityDrive[index]
	}

	for pivotIndex := range cavityCount {
		pivot := d.cavityMatrix[pivotIndex*cavityCount+pivotIndex]
		for rowIndex := pivotIndex + 1; rowIndex < cavityCount; rowIndex++ {
			leading := d.cavityMatrix[rowIndex*cavityCount+pivotIndex]
			if leading == 0 {
				continue
			}

			factor := leading / pivot
			for column := pivotIndex + 1; column < cavityCount; column++ {
				d.cavityMatrix[rowIndex*cavityCount+column] -= factor *
					d.cavityMatrix[pivotIndex*cavityCount+column]
			}

			d.cavityDrive[rowIndex] -= factor * d.cavityDrive[pivotIndex]
		}
	}

	for rowIndex := cavityCount - 1; rowIndex >= 0; rowIndex-- {
		sum := d.cavityDrive[rowIndex]
		for column := rowIndex + 1; column < cavityCount; column++ {
			sum -= d.cavityMatrix[rowIndex*cavityCount+column] *
				d.cavityMidpointPa[column]
		}

		d.cavityMidpointPa[rowIndex] = sum /
			d.cavityMatrix[rowIndex*cavityCount+rowIndex]
	}
}

func tensionConverged(current, next, maximum float64) bool {
	return math.Abs(next-current) <=
		nonlinearSolveTolerance*max(1, maximum)
}

// observe reads the committed state into one output sample.
//
// channelTrialCurrent states whether d.channelTrial already holds g_c at that
// state. Inside the tick it does: solveMidpoint's last iterate evaluated the
// channels at couplingEnd, and couplingEnd is displacement + dt*midpointVelocity
// — precisely the update tickCoupled went on to commit. Saying so spares a third
// traversal of the coupling table per sample, which is otherwise the single
// largest avoidable cost in the coupled path. Called from outside the tick on a
// state something else wrote — a test seeding d.displacement by hand, say —
// channelTrial means nothing and the channels have to be evaluated.
//
// Passing true wrongly produces a stale coupling potential energy and no other
// symptom, so it is worth being sure at each call site rather than defaulting.
func (d *DoubleHead) observe(channelTrialCurrent bool) DoubleHeadOutput {
	var output DoubleHeadOutput

	batterStrain := 0.0
	resonantStrain := 0.0

	coupling := d.config.Cavity.Coupling01

	for index := range d.modes {
		mode := &d.modes[index]

		displacement := d.displacement[index]
		velocity := d.velocity[index]
		pickupDisplacement := mode.PickupShape * displacement
		pickupVelocity := mode.PickupShape * velocity

		if index < d.batterModeCount {
			output.BatterDisplacementM += pickupDisplacement
			output.BatterVelocityMPerS += pickupVelocity
			batterStrain += d.strainWeight[index] *
				displacement * displacement
		} else {
			output.ResonantDisplacementM += pickupDisplacement
			output.ResonantVelocityMPerS += pickupVelocity
			resonantStrain += d.strainWeight[index] *
				displacement * displacement
		}

		output.SweptVolumeM3 += coupling *
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

	// The channel potentials are part of the conserved quantity, not a
	// diagnostic: the discrete-gradient force below trades linear modal energy
	// against exactly this sum, so an energy test that omitted it would report a
	// drift that is not there.
	if d.couplingActive {
		if channelTrialCurrent {
			// Same function, same inputs, same bits — a copy, not an estimate.
			copy(d.channelValue, d.channelTrial)
		} else {
			d.channelValuesAt(d.displacement, d.channelValue)
		}

		quarter := 0.25 * d.coupling.coefficientNPerM
		for _, value := range d.channelValue {
			output.CouplingPotentialEnergyJ += quarter * value * value
		}

		output.NonlinearPotentialEnergyJ += output.CouplingPotentialEnergyJ
	}

	output.HeadMechanicalEnergyJ = output.LinearHeadMechanicalEnergyJ +
		output.NonlinearPotentialEnergyJ
	output.NonlinearSolveIterations = d.nonlinearSolveIterations

	// One term per cavity mode, each the same p^2/(2K) the lumped compliance
	// stored plus the matching term for its conjugate state. Together with the
	// heads' mechanical energy this is the quantity the coupled system conserves
	// exactly when every loss is zero.
	for index := range d.cavityModes {
		stiffness := d.cavityModes[index].StiffnessPaPerM3
		if stiffness <= 0 {
			continue
		}

		pressure := d.cavityPressurePa[index]
		flow := d.cavityFlowPa[index]
		output.CavityMechanicalEnergyJ += 0.5*pressure*pressure/stiffness +
			0.5*flow*flow/stiffness
	}

	output.TotalMechanicalEnergyJ = output.HeadMechanicalEnergyJ +
		output.CavityMechanicalEnergyJ
	output.CavityPressurePa = d.cavityPressurePa[0]
	output.BatterRawRadiated = d.batterRadiatedM3PerS2
	output.ResonantRawRadiated = d.resonantRadiatedM3PerS2
	output.AttackRawRadiated = d.attackRadiatedM3PerS2
	// Pickup describes a batter-side microphone projection. The resonant head
	// remains fully coupled into the batter dynamics, but its outward
	// radiation leaves the opposite side of the shell and cannot be added at
	// the same point, phase, distance, and polarity. Keep its raw diagnostic
	// separate until a propagation/diffraction model supplies that transfer.
	output.RawRadiated = output.BatterRawRadiated + output.AttackRawRadiated

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
