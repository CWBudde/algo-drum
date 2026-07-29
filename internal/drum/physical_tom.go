package drum

import "github.com/cwbudde/algo-drum/internal/physical"

const physicalTomOutputGain = 0.25

// TomModel selects the implementation used by the Tom track.
type TomModel uint8

const (
	// TomModelProcedural preserves the original swept-sine Tom.
	TomModelProcedural TomModel = iota
	// TomModelPhysical selects the experimental single-head modal model.
	TomModelPhysical
)

type physicalTom struct {
	config            physical.PhysicalDrum
	model             *physical.SingleHead
	decayAmount       float64
	baseLoss0         float64
	baseLoss2         float64
	baseRadiationLoss float64
}

func newPhysicalTom(sampleRate float64) (*physicalTom, error) {
	config := physical.DefaultPhysicalDrum()
	config.SampleRateHz = sampleRate

	model, err := physical.NewSingleHead(config)
	if err != nil {
		return nil, err
	}

	return &physicalTom{
		config:            config,
		model:             model,
		decayAmount:       0.5,
		baseLoss0:         config.Batter.Loss0PerSecond,
		baseLoss2:         config.Batter.Loss2M2PerSecond,
		baseRadiationLoss: config.Batter.RadiationLossPerSecond,
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

	// Match the procedural DEC strip's 0.5×–1.5× decay-time convention.
	// Modal loss rates are inverse time constants, hence the reciprocal.
	lossScale := 1 / (decayScaleMin + decayAmount)
	config := v.config
	config.Batter.Loss0PerSecond = v.baseLoss0 * lossScale
	config.Batter.Loss2M2PerSecond = v.baseLoss2 * lossScale
	config.Batter.RadiationLossPerSecond = v.baseRadiationLoss * lossScale

	model, err := physical.NewSingleHead(config)
	if err != nil {
		logErr("physical tom decay", err)

		return
	}

	v.config = config
	v.model = model
	v.decayAmount = decayAmount
}

// The first web integration deliberately exposes the model selector only.
// Physical parameters receive their own generated metadata in a later phase;
// procedural Tom parameters remain stored in Engine.proceduralTom while this
// voice is selected.
func (v *physicalTom) SetParam(_ int, _ float64) {}

func (v *physicalTom) Param(_ int) float64 { return 0 }

func (v *physicalTom) ParamSpecs() []ParamSpec { return nil }
