package drum

import (
	"math"

	"github.com/cwbudde/algo-drum/internal/physical"
)

const physicalTomOutputGain = 0.25

// TomModel selects the implementation used by the Tom track.
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

	return v.model.Tick().Radiated * physicalTomOutputGain
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

func (v *physicalTom) reconfigure() error {
	config := physical.DefaultPhysicalDrum()
	config.SampleRateHz = v.config.SampleRateHz

	diameterM := v.params.value(physicalTomParamDiameter)
	config.Batter.RadiusM = diameterM / 2
	config.Resonant.RadiusM = diameterM / 2
	config.Batter.TensionNPerM = v.params.value(physicalTomParamBatterTension)
	config.Resonant.TensionNPerM = v.params.value(physicalTomParamResonantTension)

	// The one damping control preserves the default frequency-dependent and
	// radiation-loss proportions on both heads. The strip DEC knob then trims
	// all rates by its documented reciprocal 0.5×–1.5× time scale.
	dampingScale := v.params.value(physicalTomParamDamping) /
		physicalTomSpecs[physicalTomParamDamping].Shipped
	decayScale := 1 / (decayScaleMin + v.decayAmount)
	lossScale := dampingScale * decayScale
	config.Batter.Loss0PerSecond *= lossScale
	config.Batter.Loss2M2PerSecond *= lossScale
	config.Batter.RadiationLossPerSecond *= lossScale
	config.Resonant.Loss0PerSecond *= lossScale
	config.Resonant.Loss2M2PerSecond *= lossScale
	config.Resonant.RadiationLossPerSecond *= lossScale

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
