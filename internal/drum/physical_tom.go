package drum

import (
	"github.com/cwbudde/algo-tom/physical"
	"github.com/cwbudde/algo-tom/tomparams"
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
		params:      newParamBank(tomparams.Specs()),
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

// replaceParams atomically installs a complete normalized bank and performs a
// single model reconfiguration. Callers validate the bank shape and values
// before reaching this package-internal operation.
func (v *physicalTom) replaceParams(values []float64) error {
	config, err := tomparams.Config(values, v.decayAmount, v.config.SampleRateHz)
	if err != nil {
		return err
	}

	if err := v.model.Reconfigure(config); err != nil {
		return err
	}

	copy(v.params.vals, values)
	v.config = config

	return nil
}

// reconfigure rebuilds the SI configuration from the current knob bank.
//
// tomparams.Config is the *only* correct spelling of this mapping — the
// constant-ζ retune rule, the DAMP/DEC/D.TILT composition and the resonant
// head's reduced asymmetry are calibration decisions with their own evidence,
// and the offline fitter scores candidates through this same function. A copy
// here would mean the fitter measured a different instrument than the one that
// ships, which is exactly why the mapping lives in algo-tom rather than here.
func (v *physicalTom) reconfigure() error {
	config, err := tomparams.Config(v.params.vals, v.decayAmount, v.config.SampleRateHz)
	if err != nil {
		return err
	}

	if err := v.model.Reconfigure(config); err != nil {
		return err
	}

	v.config = config

	return nil
}
