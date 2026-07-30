package drum

import (
	"math"

	"github.com/cwbudde/algo-drum/internal/physical"
)

// TomModel selects the implementation used by either Tom track.
type TomModel uint8

const (
	// TomModelProcedural preserves the original swept-sine Tom.
	TomModelProcedural TomModel = iota
	// TomModelPhysical selects the experimental double-headed modal model.
	TomModelPhysical
)

type physicalTom struct {
	config      physical.PhysicalDrum
	model       *physical.DoubleHead
	params      paramBank
	decayAmount float64
}

func newPhysicalTom(sampleRate float64) (*physicalTom, error) {
	config := physical.DefaultPhysicalDrum()
	config.SampleRateHz = sampleRate

	model, err := physical.NewDoubleHead(config)
	if err != nil {
		return nil, err
	}

	return &physicalTom{
		config:      config,
		model:       model,
		params:      newParamBank(physicalTomSpecs),
		decayAmount: 0.5,
	}, nil
}

func (v *physicalTom) Trigger(velocity float64) {
	// clamp01 applies the Voice contract before the stricter physical API.
	_ = v.model.Trigger(clamp01(velocity))
}

func (v *physicalTom) Tick() float64 {
	if !v.model.IsActive() {
		return 0
	}

	// No compensating gain: the radiated sum is a calibrated volume
	// acceleration and Pickup.OutputGain is fitted to reach product level.
	return v.model.Tick().Radiated
}

func (v *physicalTom) IsActive() bool {
	return v.model.IsActive()
}

func (v *physicalTom) Reset() {
	v.model.Reset()
}

func (v *physicalTom) SetDecay(amount float64) {
	decayAmount := clamp01(amount)
	if decayAmount == v.decayAmount {
		return
	}

	oldDecay := v.decayAmount

	v.decayAmount = decayAmount
	if err := v.reconfigure(); err != nil {
		logErr("physical tom decay", err)

		v.decayAmount = oldDecay

		return
	}
}

func (v *physicalTom) SetParam(index int, value01 float64) {
	if index < 0 || index >= len(v.params.specs) {
		return
	}

	old := v.params.vals[index]
	if !v.params.set(index, value01) {
		return
	}

	if err := v.reconfigure(); err != nil {
		v.params.vals[index] = old

		logErr("physical tom parameter", err)
	}
}

func (v *physicalTom) Param(index int) float64 {
	return v.params.Param(index)
}

func (v *physicalTom) ParamSpecs() []ParamSpec {
	return v.params.ParamSpecs()
}

// scaleHeadLosses applies the UI's damping scale and frequency tilt to one
// head's loss law.
//
// The measured (0,1) correction scales with the rest: it is a loss like any
// other, and leaving it fixed would make DAMP and DEC unable to shorten the
// one mode whose length is most audible. It follows the tilt too, because the
// coupling loss it stands for is frequency-dependent in origin.
func scaleHeadLosses(head *physical.Head, lossScale, tilt float64) {
	head.Loss0PerSecond *= lossScale
	head.Loss1MPerSecond *= lossScale * tilt
	head.Loss2M2PerSecond *= lossScale * tilt
	head.RadiationLossPerSecond *= lossScale

	for index := range head.ModeDecayCorrections {
		head.ModeDecayCorrections[index].DecayRatePerSecond *= lossScale * tilt
	}
}

func (v *physicalTom) reconfigure() error {
	config := physical.DefaultPhysicalDrum()
	config.SampleRateHz = v.config.SampleRateHz

	diameterM := v.params.value(physicalTomParamDiameter)
	config.Batter.RadiusM = diameterM / 2
	config.Resonant.RadiusM = diameterM / 2
	// RetuneTension, not a bare assignment: the loss coefficients in the default
	// config are quoted at its tension, so writing a new tension over them would
	// leave ζ — and with it the whole decay calibration — drifting with the
	// tuning knob. It used to, by a factor of three across B.TUNE's travel.
	physical.RetuneTension(&config.Batter, v.params.value(physicalTomParamBatterTension))
	physical.RetuneTension(&config.Resonant, v.params.value(physicalTomParamResonantTension))

	// DAMP scales every loss rate together; the strip DEC knob then trims them
	// all by its documented reciprocal 0.5×–1.5× time scale. D.TILT is applied
	// on top and only to the frequency-dependent terms, so it changes the shape
	// of the decay across the mode series rather than its overall level.
	lossScale := v.params.value(physicalTomParamDamping) /
		(decayScaleMin + v.decayAmount)
	tilt := v.params.value(physicalTomParamDampingTilt)
	scaleHeadLosses(&config.Batter, lossScale, tilt)
	scaleHeadLosses(&config.Resonant, lossScale, tilt)

	config.Strike.Radius01 = v.params.value(physicalTomParamStrikeRadius)
	config.Strike.AngleRad = v.params.value(physicalTomParamStrikeAngle) *
		math.Pi / 180
	config.Strike.Hardness01 = v.params.value(physicalTomParamHardness)
	config.Cavity.DepthM = v.params.value(physicalTomParamShellDepth)
	config.Cavity.Coupling01 = v.params.value(physicalTomParamCavityCoupling)

	nonlinearScale := v.params.value(physicalTomParamNonlinearity)
	config.Nonlinearity.Enabled = nonlinearScale > 0
	config.Nonlinearity.BatterTensionCoefficientNPerM3 *= nonlinearScale
	config.Nonlinearity.ResonantTensionCoefficientNPerM3 *= nonlinearScale

	config.Attack.LevelRelative = v.params.value(physicalTomParamAttackLevel)
	config.Attack.CentreHz = v.params.value(physicalTomParamAttackTone)
	// Nothing to do for the layer's decay: it is derived from the batter head's
	// loss law, which scaleHeadLosses has already scaled. So DAMP, the strip DEC
	// and D.TILT all reach the attack for free, and D.TILT now genuinely applies
	// to it — three bands at different rates are a shape to tilt, where the single
	// band this replaced was not.

	config.Pickup.Radius01 = v.params.value(physicalTomParamPickupRadius)
	config.Pickup.AngleRad = v.params.value(physicalTomParamPickupAngle) *
		math.Pi / 180

	splitRatio := v.params.value(physicalTomParamAsymmetry) / 100
	asymmetryAxis := v.params.value(physicalTomParamAsymmetryAxis) *
		math.Pi / 180
	config.Batter.TensionAsymmetry.SplitRatio = splitRatio
	config.Batter.TensionAsymmetry.PrincipalAxisAngleRad = asymmetryAxis
	// The thinner resonant head keeps a slightly smaller default departure,
	// while the UI presents one comprehensible "amount" for the instrument.
	config.Resonant.TensionAsymmetry.SplitRatio = splitRatio * 0.75
	config.Resonant.TensionAsymmetry.PrincipalAxisAngleRad = asymmetryAxis

	switch int(v.params.value(physicalTomParamQuality)) {
	case 0:
		config.Quality = physical.QualityDraft
	case 1:
		config.Quality = physical.QualityStandard
	case 2:
		config.Quality = physical.QualityHigh
	}

	if err := v.model.Reconfigure(config); err != nil {
		return err
	}

	v.config = config

	return nil
}
