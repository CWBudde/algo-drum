package drum

import (
	"errors"
	"fmt"
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

// ScaleHeadLosses multiplies one head's whole loss law by scale, which is
// exactly what DAMP does and nothing else: the frequency tilt is left alone, so
// the shape of the decay across the mode series does not move.
//
// It exists for offline experiments that need to reach outside DAMP's own
// range. A fit that pins DAMP against a bound has said something about the
// bound rather than about the drum, and the only way to tell which is to look
// past it — but widening the shipped spec to find out would move every stored
// preset, since presets hold normalized positions. Applying an extra factor
// here answers the question without touching the product.
//
// Nothing in the engine calls it. If a fitted value ever justifies moving the
// spec, that is a calibration decision with its own migration, not this.
func ScaleHeadLosses(head *physical.Head, scale float64) {
	scaleHeadLosses(head, scale, 1)
}

// ErrPhysicalTomParamCount reports a normalized bank of the wrong width.
var ErrPhysicalTomParamCount = errors.New("physical tom parameter count")

// NeutralDecayAmount is the strip DEC position at which the knob contributes
// nothing: decayScaleMin + amount == 1, so the loss law is left as the
// parameter bank set it. Offline callers that want to fit DAMP alone should
// pass it rather than reaching for a bare 0.5.
const NeutralDecayAmount = 1 - decayScaleMin

// PhysicalTomConfig maps a normalized parameter bank to the SI configuration
// the physical model consumes.
//
// values01 holds one 0..1 knob position per PhysicalTomSpecs() entry, in the
// same order; decayAmount is the strip DEC position, also 0..1. Both are
// clamped rather than rejected, matching the setter contract everywhere else.
//
// It is exported because it is the *only* correct spelling of this mapping —
// the constant-ζ retune rule, the DAMP/DEC/D.TILT composition and the resonant
// head's reduced asymmetry are all calibration decisions with their own
// evidence. Offline tools that build a configuration from knob positions must
// reuse it, or they measure a different instrument than the one that ships.
func PhysicalTomConfig(values01 []float64, decayAmount, sampleRateHz float64) (physical.PhysicalDrum, error) {
	if len(values01) != len(physicalTomSpecs) {
		return physical.PhysicalDrum{}, fmt.Errorf(
			"%w: got %d, want %d", ErrPhysicalTomParamCount,
			len(values01), len(physicalTomSpecs),
		)
	}

	value := func(index int) float64 {
		return physicalTomSpecs[index].Map(clamp01(values01[index]))
	}

	config := physical.DefaultPhysicalDrum()
	config.SampleRateHz = sampleRateHz

	diameterM := value(physicalTomParamDiameter)
	config.Batter.RadiusM = diameterM / 2
	config.Resonant.RadiusM = diameterM / 2
	// RetuneTension, not a bare assignment: the loss coefficients in the default
	// config are quoted at its tension, so writing a new tension over them would
	// leave ζ — and with it the whole decay calibration — drifting with the
	// tuning knob. It used to, by a factor of three across B.TUNE's travel.
	physical.RetuneTension(&config.Batter, value(physicalTomParamBatterTension))
	physical.RetuneTension(&config.Resonant, value(physicalTomParamResonantTension))

	// DAMP scales every loss rate together; the strip DEC knob then trims them
	// all by its documented reciprocal 0.5×–1.5× time scale. D.TILT is applied
	// on top and only to the frequency-dependent terms, so it changes the shape
	// of the decay across the mode series rather than its overall level.
	lossScale := value(physicalTomParamDamping) /
		(decayScaleMin + clamp01(decayAmount))
	tilt := value(physicalTomParamDampingTilt)
	scaleHeadLosses(&config.Batter, lossScale, tilt)
	scaleHeadLosses(&config.Resonant, lossScale, tilt)

	config.Strike.Radius01 = value(physicalTomParamStrikeRadius)
	config.Strike.AngleRad = value(physicalTomParamStrikeAngle) *
		math.Pi / 180
	config.Strike.Hardness01 = value(physicalTomParamHardness)
	config.Cavity.DepthM = value(physicalTomParamShellDepth)
	config.Cavity.Coupling01 = value(physicalTomParamCavityCoupling)

	nonlinearScale := value(physicalTomParamNonlinearity)
	config.Nonlinearity.Enabled = nonlinearScale > 0
	config.Nonlinearity.BatterTensionCoefficientNPerM3 *= nonlinearScale
	config.Nonlinearity.ResonantTensionCoefficientNPerM3 *= nonlinearScale

	config.Attack.LevelRelative = value(physicalTomParamAttackLevel)
	config.Attack.CentreHz = value(physicalTomParamAttackTone)
	// Nothing to do for the layer's decay: it is derived from the batter head's
	// loss law, which scaleHeadLosses has already scaled. So DAMP, the strip DEC
	// and D.TILT all reach the attack for free, and D.TILT now genuinely applies
	// to it — three bands at different rates are a shape to tilt, where the single
	// band this replaced was not.

	config.Pickup.Radius01 = value(physicalTomParamPickupRadius)
	config.Pickup.AngleRad = value(physicalTomParamPickupAngle) *
		math.Pi / 180

	splitRatio := value(physicalTomParamAsymmetry) / 100
	asymmetryAxis := value(physicalTomParamAsymmetryAxis) *
		math.Pi / 180
	config.Batter.TensionAsymmetry.SplitRatio = splitRatio
	config.Batter.TensionAsymmetry.PrincipalAxisAngleRad = asymmetryAxis
	// The thinner resonant head keeps a slightly smaller default departure,
	// while the UI presents one comprehensible "amount" for the instrument.
	config.Resonant.TensionAsymmetry.SplitRatio = splitRatio * 0.75
	config.Resonant.TensionAsymmetry.PrincipalAxisAngleRad = asymmetryAxis

	switch int(value(physicalTomParamQuality)) {
	case 0:
		config.Quality = physical.QualityDraft
	case 1:
		config.Quality = physical.QualityStandard
	case 2:
		config.Quality = physical.QualityHigh
	}

	return config, nil
}

func (v *physicalTom) reconfigure() error {
	config, err := PhysicalTomConfig(v.params.vals, v.decayAmount, v.config.SampleRateHz)
	if err != nil {
		return err
	}

	if err := v.model.Reconfigure(config); err != nil {
		return err
	}

	v.config = config

	return nil
}
