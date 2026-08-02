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

// modeCouplingRange is one head mode's slice of the cavity coupling table:
// where its (cavity index, coefficient) run starts and how long it is.
//
// # Why one struct rather than two []int32
//
// This is the one uncontested interleaving candidate in the bank — every reader
// of `first` reads `count` at the same mode in the same expression, and no other
// loop wants either of them on its own. Two parallel slices are two prefetch
// streams and two bounds-checked loads to fetch 8 bytes that are always wanted
// together; one struct slice is one stream, one check and one 8-byte load.
//
// The measured effect is what the number below says it is, and it is small: the
// bank is 120 modes, so both layouts are L1-resident either way (measured L1
// miss rate for the render path is 0.06 %), and there is no memory traffic to
// remove. What is left is the load and address arithmetic, and that is where the
// change shows up.
type modeCouplingRange struct {
	first int32
	count int32
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

	// Struct-of-arrays mirrors of the three Mode fields the vectorised midpoint
	// update reads. These exist for the kernel, not for cache friendliness — a
	// 144-byte stride cannot be loaded into a vector register without a gather,
	// and the gather would cost more than the vectorisation returns. d.modes stays
	// the source of truth; syncModeArrays re-derives these, and
	// TestModeArraysMirrorTheBank fails if it is ever not called.
	modeWavenumberPerM  []float64
	modeOmegaSquared    []float64
	modeStrikeAccelPerN []float64
	// modeRadiationWeight is a mirror for the opposite reason to the three above:
	// the two tick loops read this one field per mode per sample and nothing else
	// from the bank, so walking []Mode for it touched 144 bytes to consume 8. This
	// one is exactly the cache-friendliness argument the block above disclaims.
	modeRadiationWeight []float64
	// The three observe reads, mirrored for the same reason as
	// modeRadiationWeight. observe runs every sample over the whole bank and
	// consumed four fields — these three plus omega squared, which
	// modeOmegaSquared already carries — spanning offsets +88 to +136 of a
	// 144-byte Mode, so it dragged 17.3 KB of bank through L1 to use 3.8 KB of it.
	// Read together at the same index, the four mirrors are four unit-stride
	// streams instead.
	modePickupShape []float64
	modeSweptAreaM2 []float64
	modeModalMassKg []float64
	// stepDenominator carries D_i — midpointDenom plus this iteration's nonlinear
	// tension term — from the two update loops to the cavity fill, which used to
	// run inside them and now runs as its own pass. Written before it is read on
	// every call, so unlike a mirror of the mode bank it cannot go stale.
	stepDenominator []float64
	radiationHP     biquad.Section
	radiationLP     biquad.Section
	attack          attackLayer

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
	// couplingRange indexes a run of (cavity index, coefficient) pairs per head
	// mode. The azimuthal selection rule makes that run short — a head mode
	// couples only to cavity modes of its own azimuthal order — so the k x k
	// system the midpoint solve assembles is mostly empty and is never walked
	// densely.
	//
	// The two int32 are one struct rather than two parallel slices because every
	// reader wants both at the same mode; see the interleaving note on
	// modeCouplingRange.
	cavityModes                []CavityMode
	cavityPressurePa           []float64
	cavityFlowPa               []float64
	cavityMidpointPa           []float64
	cavityDrive                []float64
	cavityMatrix               []float64
	couplingRange              []modeCouplingRange
	couplingCavity             []int32
	couplingAreaM2             []float64
	couplingGain               []float64
	cavityVolumeM3             float64
	cavityBulkStiffnessPaPerM3 float64
	batterNonlinear            nonlinearHead
	resonantNonlinear          nonlinearHead
	nonlinearSolveIterations   int
	energy                     float64
	release                    releaseBound

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
		couplingRange:    make([]modeCouplingRange, modeCount),
		release:          newReleaseBound(config.SampleRateHz),

		modeWavenumberPerM:  make([]float64, modeCount),
		modeOmegaSquared:    make([]float64, modeCount),
		modeStrikeAccelPerN: make([]float64, modeCount),
		modeRadiationWeight: make([]float64, modeCount),
		modePickupShape:     make([]float64, modeCount),
		modeSweptAreaM2:     make([]float64, modeCount),
		modeModalMassKg:     make([]float64, modeCount),
		stepDenominator:     make([]float64, modeCount),
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

	model.syncModeArrays()

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

// syncModeArrays re-derives the mirrors the midpoint kernel reads from d.modes.
//
// d.modes is the source of truth and these are a cache, so anything editing a
// mode after construction must call this — the obligation strikeWeight already
// carried. NewDoubleHead calls it once and nothing on the audio path calls it at
// all. TestModeArraysMirrorTheBank is the guard: a mirror that stops matching the
// bank fails there rather than silently detuning the solve.
func (d *DoubleHead) syncModeArrays() {
	for index := range d.modes {
		mode := &d.modes[index]

		d.modeWavenumberPerM[index] = mode.WavenumberPerM
		// The same product solveMidpoint used to form per mode per iteration.
		d.modeOmegaSquared[index] = mode.AngularFrequency * mode.AngularFrequency
		d.modeStrikeAccelPerN[index] = mode.StrikeAccelerationPerN
		d.modeRadiationWeight[index] = mode.RadiationWeight
		d.modePickupShape[index] = mode.PickupShape
		d.modeSweptAreaM2[index] = mode.SweptAreaM2
		d.modeModalMassKg[index] = mode.ModalMassKg
	}
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

		d.couplingRange[index].first = int32(len(d.couplingAreaM2))

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

		d.couplingRange[index].count = int32(len(d.couplingAreaM2)) -
			d.couplingRange[index].first
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
	// Re-arms rather than extends: a strike buys the voice another
	// ReleaseBoundSeconds, so a roll never trips the bound and the bound only
	// bites once the player stops. Existing modal motion is retained here, so
	// this is the one piece of state a Trigger clears.
	d.release.restart()

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
	d.release.restart()
	d.radiationHP.Reset()
	d.radiationLP.Reset()
}

// IsActive reports whether contact is pending or stored energy exceeds either
// enabled head's inactivity threshold, and the release bound has not expired.
//
// The bound is checked first and it is unconditional: the loss law is reachable
// from the product's own knobs, so the energy test alone does not terminate.
// See ReleaseBoundSeconds.
func (d *DoubleHead) IsActive() bool {
	if d.release.expired() {
		return false
	}

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
	// The release fade, applied to the microphone signal and to nothing else.
	// The state update above already ran in full, so d.energy and every raw
	// component are what they always were — which is what keeps the
	// conservation and passivity tests measuring the model rather than this.
	output.Radiated *= d.release.advance()

	return output
}

// Render writes the filtered batter-side microphone signal into dst.
func (d *DoubleHead) Render(dst []float64) {
	for index := range dst {
		dst[index] = d.Tick().Radiated
	}
}

// tickUncoupled is the linear path: no cavity, no Berger tension, so each mode
// advances by its own precomputed 2x2 transition.
//
// The loop is split at batterModeCount and every slice header is taken into a
// local first. Both are the treatment solveMidpoint's prologue already applies
// for the reason given there: the stores into displacement and velocity may
// alias d, so without the locals the compiler reloads all seven headers — plus
// batterModeCount and config.SampleRateHz — on every mode. Splitting also lifts
// the strike-force predicate out of the body, since it is loop-invariant per
// half exactly as it is in solveMidpoint.
//
// Each half takes its own sub-slices cut to one common length, for the reason
// observe and SingleHead.Tick document: eight independent slices indexed by one
// counter cannot be proven in range from the counter's bound alone, so the
// half-open index form emits a bounds check per slice per mode. Cut to a common
// length and ranged over, every check leaves the body — `go build
// -gcflags=-d=ssa/check_bce/debug=1` reports none inside either loop.
//
// The arithmetic is untouched, operand order included; see the note on radiated
// below and TestZeroCouplingMatchesShipped.
func (d *DoubleHead) tickUncoupled(forceN float64) DoubleHeadOutput {
	sampleRate := d.config.SampleRateHz
	inverseSampleRate := 1 / sampleRate

	// min is a no-op on any bank NewDoubleHead builds — batterModeCount is a
	// prefix length of modes — and is written anyway so the slice expressions
	// below are provably in range rather than provably in range by argument.
	modeCount := len(d.modes)
	batterModeCount := min(d.batterModeCount, modeCount)

	batterVelocity := d.velocity[:batterModeCount]
	batterDisplacement := d.displacement[:batterModeCount]
	batterMatrix11 := d.matrix11[:batterModeCount]
	batterMatrix12 := d.matrix12[:batterModeCount]
	batterMatrix21 := d.matrix21[:batterModeCount]
	batterMatrix22 := d.matrix22[:batterModeCount]
	batterStrikeAccel := d.modeStrikeAccelPerN[:batterModeCount]
	batterRadiationWeight := d.modeRadiationWeight[:batterModeCount]

	batterRadiated := 0.0
	resonantRadiated := 0.0

	// The two bodies are written out rather than shared through a helper: the
	// only difference is the strike term, and a closure over the seven slices
	// would put them back in a frame, which is the cost this function is trying
	// to avoid. solveMidpoint's two kernels are duplicated for the same reason.
	//
	// previousVelocity is captured before the contact impulse, so the
	// acceleration includes it. Taking it afterwards would leave only
	// (matrix22 - 1)*F*a*dt of the strike, which is very nearly nothing. Ranging
	// over batterVelocity reads it at the head of each iteration, before the
	// store below, so it is the same value the indexed form captured.
	for index, previousVelocity := range batterVelocity {
		oldVelocity := previousVelocity +
			forceN*batterStrikeAccel[index]*inverseSampleRate

		oldDisplacement := batterDisplacement[index]
		batterDisplacement[index] = batterMatrix11[index]*oldDisplacement +
			batterMatrix12[index]*oldVelocity
		newVelocity := batterMatrix21[index]*oldDisplacement +
			batterMatrix22[index]*oldVelocity
		batterVelocity[index] = newVelocity

		// Written identically in SingleHead.Tick. The zero-coupling equivalence
		// test compares the two to the last bit, so the operand order matters.
		batterRadiated += batterRadiationWeight[index] *
			(newVelocity - previousVelocity) * sampleRate
	}

	resonantVelocity := d.velocity[batterModeCount:modeCount]
	resonantDisplacement := d.displacement[batterModeCount:modeCount]
	resonantMatrix11 := d.matrix11[batterModeCount:modeCount]
	resonantMatrix12 := d.matrix12[batterModeCount:modeCount]
	resonantMatrix21 := d.matrix21[batterModeCount:modeCount]
	resonantMatrix22 := d.matrix22[batterModeCount:modeCount]
	resonantRadiationWeight := d.modeRadiationWeight[batterModeCount:modeCount]

	for index, oldVelocity := range resonantVelocity {
		oldDisplacement := resonantDisplacement[index]
		resonantDisplacement[index] = resonantMatrix11[index]*oldDisplacement +
			resonantMatrix12[index]*oldVelocity
		newVelocity := resonantMatrix21[index]*oldDisplacement +
			resonantMatrix22[index]*oldVelocity
		resonantVelocity[index] = newVelocity

		resonantRadiated += resonantRadiationWeight[index] *
			(newVelocity - oldVelocity) * sampleRate
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

	// Same hoisting as tickUncoupled, and for the same aliasing reason: the
	// stores into displacement and velocity force a reload of every header —
	// and of config.SampleRateHz, which is 1/timeStep already in scope — on
	// each mode otherwise. Sliced to a common length per half for the
	// bounds-check reason tickUncoupled documents.
	sampleRate := d.config.SampleRateHz
	modeCount := len(d.modes)
	batterModeCount := min(d.batterModeCount, modeCount)

	batterVelocity := d.velocity[:batterModeCount]
	batterDisplacement := d.displacement[:batterModeCount]
	batterMidpointVelocity := d.midpointVelocity[:batterModeCount]
	batterRadiationWeight := d.modeRadiationWeight[:batterModeCount]

	// For the midpoint rule v_new - v_old = 2*(v_new - v_mid), so this is the
	// same discrete acceleration the uncoupled path forms directly. The contact
	// force is included even though it never appears as a velocity increment
	// here: it enters solveMidpoint as an acceleration, v_mid gains F*a*dt/2
	// from it, and the reflection below doubles that.
	for index, midpointVelocity := range batterMidpointVelocity {
		batterDisplacement[index] += timeStep * midpointVelocity
		newVelocity := 2*midpointVelocity - batterVelocity[index]
		batterVelocity[index] = newVelocity

		batterRadiated += batterRadiationWeight[index] *
			2 * (newVelocity - midpointVelocity) * sampleRate
	}

	resonantVelocity := d.velocity[batterModeCount:modeCount]
	resonantDisplacement := d.displacement[batterModeCount:modeCount]
	resonantMidpointVelocity := d.midpointVelocity[batterModeCount:modeCount]
	resonantRadiationWeight := d.modeRadiationWeight[batterModeCount:modeCount]

	for index, midpointVelocity := range resonantMidpointVelocity {
		resonantDisplacement[index] += timeStep * midpointVelocity
		newVelocity := 2*midpointVelocity - resonantVelocity[index]
		resonantVelocity[index] = newVelocity

		resonantRadiated += resonantRadiationWeight[index] *
			2 * (newVelocity - midpointVelocity) * sampleRate
	}

	d.batterRadiatedM3PerS2 = batterRadiated
	d.resonantRadiatedM3PerS2 = resonantRadiated

	if d.config.Cavity.Enabled {
		cavityCount := len(d.cavityModes)
		cavityMidpointPa := d.cavityMidpointPa[:cavityCount]
		cavityPressurePa := d.cavityPressurePa[:cavityCount]
		cavityFlowPa := d.cavityFlowPa[:cavityCount]

		// Index-only: CavityMode is 64 bytes and the value form would copy one
		// per iteration to read a single field.
		cavityModes := d.cavityModes
		for index := range cavityModes {
			midpoint := cavityMidpointPa[index]
			cavityPressurePa[index] = 2*midpoint - cavityPressurePa[index]
			// Hdot = -omega*P integrated by the same midpoint rule, so
			// H_new = H_old - omega*dt*P_mid. The uniform mode has omega = 0 and
			// its H therefore never leaves zero, which is what collapses this
			// pair back onto the lumped compliance.
			cavityFlowPa[index] -= cavityModes[index].AngularFrequency *
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

	// Hoisted for the reason perf annotate exposed in the solve: read as d.field
	// inside the mode loop, each slice header is reloaded from the struct per mode.
	//
	// The four per-mode quantities come from the struct-of-arrays mirrors rather
	// than from d.modes: this loop used to walk the 144-byte bank for 32 useful
	// bytes per mode. modeOmegaSquared is the same product the energy term formed
	// inline — omega*omega rounded once, then multiplied by the displacement twice
	// exactly as before — so the substitution is bit-identical, not merely equal to
	// within rounding.
	//
	// Split at batterModeCount rather than testing it per mode. The branch this
	// replaces already partitioned the bank at exactly this index, and the two
	// shared accumulators below keep running across both halves, so every
	// accumulation order is the one that shipped.
	//
	// Each half takes its own sub-slices, and that is not decoration. Seven
	// independent slices indexed by one counter cannot be proven in range from the
	// counter's bound alone, so the naive form emits seven bounds checks per mode
	// and measured *slower* than the array-of-structs loop it replaced — the
	// single-pointer form got five fields for one check. Sliced to a common length
	// and ranged over, the check moves out of the loop entirely; `go build
	// -gcflags=-d=ssa/check_bce/debug=1` reports none inside either body.
	// min is a no-op on any bank NewDoubleHead builds — batterModeCount is a
	// prefix length of modes — and is written anyway so the two slice expressions
	// below are provably in range rather than provably in range by argument.
	modeCount := len(d.modes)
	batterModeCount := min(d.batterModeCount, modeCount)

	batterDisplacement := d.displacement[:batterModeCount]
	batterVelocity := d.velocity[:batterModeCount]
	batterStrainWeight := d.strainWeight[:batterModeCount]
	batterPickup := d.modePickupShape[:batterModeCount]
	batterSweptArea := d.modeSweptAreaM2[:batterModeCount]
	batterModalMass := d.modeModalMassKg[:batterModeCount]
	batterOmegaSquared := d.modeOmegaSquared[:batterModeCount]

	for index, displacement := range batterDisplacement {
		velocity := batterVelocity[index]
		shape := batterPickup[index]

		output.BatterDisplacementM += shape * displacement
		output.BatterVelocityMPerS += shape * velocity
		batterStrain += batterStrainWeight[index] *
			displacement * displacement

		output.SweptVolumeM3 += coupling *
			batterSweptArea[index] * displacement
		output.LinearHeadMechanicalEnergyJ += 0.5 * batterModalMass[index] *
			(velocity*velocity +
				batterOmegaSquared[index]*
					displacement*displacement)
	}

	resonantDisplacement := d.displacement[batterModeCount:modeCount]
	resonantVelocity := d.velocity[batterModeCount:modeCount]
	resonantStrainWeight := d.strainWeight[batterModeCount:modeCount]
	resonantPickup := d.modePickupShape[batterModeCount:modeCount]
	resonantSweptArea := d.modeSweptAreaM2[batterModeCount:modeCount]
	resonantModalMass := d.modeModalMassKg[batterModeCount:modeCount]
	resonantOmegaSquared := d.modeOmegaSquared[batterModeCount:modeCount]

	for index, displacement := range resonantDisplacement {
		velocity := resonantVelocity[index]
		shape := resonantPickup[index]

		output.ResonantDisplacementM += shape * displacement
		output.ResonantVelocityMPerS += shape * velocity
		resonantStrain += resonantStrainWeight[index] *
			displacement * displacement

		output.SweptVolumeM3 += coupling *
			resonantSweptArea[index] * displacement
		output.LinearHeadMechanicalEnergyJ += 0.5 * resonantModalMass[index] *
			(velocity*velocity +
				resonantOmegaSquared[index]*
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
