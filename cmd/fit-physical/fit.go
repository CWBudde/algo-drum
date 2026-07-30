package main

import (
	"errors"
	"fmt"
	"math"
	"slices"

	"github.com/cwbudde/algo-drum/internal/drum"
	"github.com/cwbudde/algo-drum/internal/physical"
	"github.com/cwbudde/algo-drum/internal/physical/match"
)

var errInvalidFitOption = errors.New("invalid fit option")

// evaluator turns a normalized search vector into a distance from the
// reference. One per goroutine: it owns a render buffer and nothing else.
type evaluator struct {
	reference match.Features
	options   match.Options
	weights   match.Weights

	// bank is the full parameter bank; free lists the indices the search may
	// move, so a fixed parameter keeps whatever bank already holds.
	bank []float64
	free []int

	sampleRateHz    float64
	durationSeconds float64

	// contact overrides the excitation model, and malletMassKg the stick's
	// mass, on top of whatever the parameter bank produced. Neither is in the
	// bank: the contact model is a build-level choice about how the strike is
	// computed rather than a knob, and the mallet mass is a property of the
	// player's stick. They are here because the head-dominated contact time
	// makes the mass the strongest lever the Hertzian model has, so a fit run
	// at one mass says nothing about the other.
	contact      physical.ContactModel
	malletMassKg float64

	buffer []float64
}

func newEvaluator(base *evaluator) *evaluator {
	clone := *base
	clone.bank = slices.Clone(base.bank)
	clone.buffer = make([]float64, len(base.buffer))

	return &clone
}

// dimensions is the search space's width: one per free parameter, plus a last
// entry for the strike velocity.
//
// Velocity is fitted rather than assumed because the recording cannot say what
// it was: the file is peak normalized and every measure here is deliberately
// gain invariant, so loudness carries no information about how hard the drum
// was hit. It is not a gain either — it sets the contact duration, the size of
// the nonlinear glide and the balance between the modal and stochastic layers
// — so pinning it would be asserting a dynamic on no evidence.
func (e *evaluator) dimensions() int {
	return len(e.free) + 1
}

// apply writes a search vector into the parameter bank and returns the
// velocity it carries.
func (e *evaluator) apply(position []float64) float64 {
	for i, index := range e.free {
		e.bank[index] = clamp01(position[i])
	}

	return clamp01(position[len(e.free)])
}

func (e *evaluator) config() (physical.PhysicalDrum, error) {
	// NeutralDecayAmount, not the shipped 0.5-equivalent by accident: the
	// strip DEC knob multiplies DAMP, so leaving it anywhere else would fold a
	// second, unrecorded factor into every fitted damping value.
	config, err := drum.PhysicalTomConfig(e.bank, drum.NeutralDecayAmount, e.sampleRateHz)
	if err != nil {
		return config, err
	}

	if e.contact != "" {
		config.Strike.Contact.Model = e.contact
	}

	if e.malletMassKg > 0 {
		config.Strike.MalletMassKg = e.malletMassKg
	}

	return config, nil
}

func (e *evaluator) render(velocity01 float64) ([]float64, error) {
	config, err := e.config()
	if err != nil {
		return nil, err
	}

	model, err := physical.NewDoubleHead(config)
	if err != nil {
		return nil, err
	}

	if err := model.Trigger(velocity01); err != nil {
		return nil, err
	}

	model.Render(e.buffer)

	return e.buffer, nil
}

func (e *evaluator) features(position []float64) (match.Features, error) {
	velocity := e.apply(position)

	samples, err := e.render(velocity)
	if err != nil {
		return match.Features{}, err
	}

	return match.Extract(samples, e.sampleRateHz, e.options)
}

// cost is the objective mayfly minimizes.
//
// A configuration the model rejects returns +Inf rather than an error: the
// search is allowed to wander into combinations that are not drums (a
// nonlinear coefficient past the anti-alias bound, say), and the cheapest
// honest answer is that they are infinitely far from sounding like one.
func (e *evaluator) cost(position []float64) float64 {
	features, err := e.features(position)
	if err != nil {
		return math.Inf(1)
	}

	if len(features.Partials) == 0 {
		return math.Inf(1)
	}

	terms := match.Distance(e.reference, features, e.weights)
	if math.IsNaN(terms.Total) {
		return math.Inf(1)
	}

	return terms.Total
}

// ParamValue reports one parameter of a fitted candidate.
type ParamValue struct {
	Index      int     `json:"index"`
	ID         string  `json:"id"`
	Label      string  `json:"label"`
	Unit       string  `json:"unit,omitempty"`
	Normalized float64 `json:"normalized"`
	Value      float64 `json:"value"`
	Fixed      bool    `json:"fixed"`
	// Pinned marks a free parameter the search pressed against a bound. It is
	// the report's most useful field: a pinned parameter means the fit wanted
	// something the product range cannot express, which is a finding about the
	// range rather than a result.
	Pinned bool `json:"pinned,omitempty"`
}

// Candidate is one evaluated point in the search space.
type Candidate struct {
	Velocity01 float64               `json:"velocity01"`
	Terms      match.Terms           `json:"terms"`
	Params     []ParamValue          `json:"params"`
	Config     physical.PhysicalDrum `json:"config"`
	Features   match.Features        `json:"features"`
	// Convergence is the winning restart's best cost after each iteration.
	Convergence []float64 `json:"convergence,omitempty"`
}

// pinnedTolerance is how close to a bound counts as pressed against it. One
// part in two hundred is finer than the persistence byte the value has to
// survive anyway, so anything inside it is at the bound for practical
// purposes.
const pinnedTolerance = 0.005

func (e *evaluator) describe(position []float64) (Candidate, error) {
	features, err := e.features(position)
	if err != nil {
		return Candidate{}, err
	}

	config, err := e.config()
	if err != nil {
		return Candidate{}, err
	}

	specs := drum.PhysicalTomSpecs()
	params := make([]ParamValue, len(specs))

	for index, spec := range specs {
		free := slices.Contains(e.free, index)
		normalized := e.bank[index]

		params[index] = ParamValue{
			Index:      index,
			ID:         spec.ID,
			Label:      spec.Label,
			Unit:       spec.Unit,
			Normalized: normalized,
			Value:      spec.Map(normalized),
			Fixed:      !free,
			Pinned: free &&
				(normalized <= pinnedTolerance || normalized >= 1-pinnedTolerance),
		}
	}

	return Candidate{
		Velocity01: e.apply(position),
		Terms:      match.Distance(e.reference, features, e.weights),
		Params:     params,
		Config:     config,
		Features:   features,
	}, nil
}

// position reads the current bank back into a search vector.
func (e *evaluator) position(velocity01 float64) []float64 {
	position := make([]float64, e.dimensions())
	for i, index := range e.free {
		position[i] = e.bank[index]
	}

	position[len(e.free)] = velocity01

	return position
}

// resolveFixed parses -fix ID=value pairs against the parameter table.
//
// searching says whether a search will follow. It only gates the "nothing left
// to search" error: fixing the whole bank is meaningless for a search but is
// exactly how -report-only measures one specific drum, which is the only way to
// score a candidate the search already produced — re-measuring a fitted bank at
// a different quality tier, say.
func resolveFixed(assignments map[string]float64, searching bool) ([]float64, []int, error) {
	specs := drum.PhysicalTomSpecs()

	bank := make([]float64, len(specs))
	for index, spec := range specs {
		bank[index] = spec.Default
	}

	pinned := map[int]bool{}

	for name, value := range assignments {
		index := slices.IndexFunc(specs, func(spec drum.ParamSpec) bool {
			return spec.ID == name || spec.Label == name
		})
		if index < 0 {
			return nil, nil, fmt.Errorf("%w: no parameter %q", errInvalidFitOption, name)
		}

		if value < 0 || value > 1 || math.IsNaN(value) {
			return nil, nil, fmt.Errorf("%w: %s = %v is not a normalized position",
				errInvalidFitOption, name, value)
		}

		bank[index] = value
		pinned[index] = true
	}

	var free []int

	for index := range specs {
		if !pinned[index] {
			free = append(free, index)
		}
	}

	if searching && len(free) == 0 {
		return nil, nil, fmt.Errorf("%w: every parameter is fixed, leaving nothing to search",
			errInvalidFitOption)
	}

	return bank, free, nil
}

func clamp01(value float64) float64 {
	if math.IsNaN(value) {
		return 0
	}

	return min(max(value, 0), 1)
}
