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
	// references are the takes being fitted, and referencePaths names them in
	// the same order. A joint fit holds more than one and scores the same bank
	// against every one of them, each at its own velocity — see dimensions.
	references     []match.Features
	referencePaths []string

	options match.Options
	weights match.Weights

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

	// lossScale multiplies every head loss rate on top of DAMP, and is 1 unless
	// -loss-scale says otherwise. It is not in the bank either, and for a
	// sharper reason than the other two: it is not a knob at all but a way to
	// search past DAMP's own lower bound, which a fit can pin itself against.
	// A run with lossScale != 1 measures a drum the product cannot currently be
	// set to, so its result is evidence about the bound and not a bank to ship.
	lossScale float64

	// corrections override the loss law at named modes, on both heads, after
	// the bank has been mapped. It is the same kind of thing as lossScale and
	// not a knob: the shipped correction table has exactly one entry, the (0,1),
	// and N3 asks whether a second one at the (1,1) is worth its dishonesty.
	// A rate given here is the *effective* one — it is applied after DAMP and
	// D.TILT, which do scale the table's own entries, so a value measured with
	// this flag has to be divided by that product before it could become a
	// default.
	corrections []physical.ModeDecayCorrection

	buffer []float64

	// velocities and rendered are scratch, one entry per reference, reused
	// across evaluations so a joint fit allocates no more per candidate than a
	// single-take one does.
	velocities []float64
	rendered   []match.Features
}

func newEvaluator(base *evaluator) *evaluator {
	clone := *base
	clone.bank = slices.Clone(base.bank)
	clone.buffer = make([]float64, len(base.buffer))
	clone.velocities = make([]float64, len(base.references))
	clone.rendered = make([]match.Features, len(base.references))

	return &clone
}

// dimensions is the search space's width: one per free parameter, plus one
// strike velocity per reference.
//
// Velocity is fitted rather than assumed because the recording cannot say what
// it was: the file is peak normalized and every measure here is deliberately
// gain invariant, so loudness carries no information about how hard the drum
// was hit. It is not a gain either — it sets the contact duration, the size of
// the nonlinear glide and the balance between the modal and stochastic layers
// — so pinning it would be asserting a dynamic on no evidence.
//
// **Per reference, and independently.** The takes of a velocity series are
// named v01…v16 in what the source pack says is increasing order, but they were
// played by hand and that order is not evidence. Giving each take its own free
// velocity is what keeps the labelling out of the fit: nothing here reads the
// index, nothing constrains take n+1 to have been struck harder than take n, and
// a series whose middle two files are swapped costs the fit nothing. The fitted
// velocities then *measure* the order rather than assuming it — writeSummary
// prints them against the file order for exactly that reason.
func (e *evaluator) dimensions() int {
	return len(e.free) + len(e.references)
}

// apply writes a search vector into the parameter bank and returns the
// per-reference velocities it carries. The returned slice is owned by the
// evaluator and is overwritten by the next call.
func (e *evaluator) apply(position []float64) []float64 {
	for i, index := range e.free {
		e.bank[index] = clamp01(position[i])
	}

	for i := range e.references {
		e.velocities[i] = clamp01(position[len(e.free)+i])
	}

	return e.velocities
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

	if e.lossScale > 0 && e.lossScale != 1 {
		drum.ScaleHeadLosses(&config.Batter, e.lossScale)
		drum.ScaleHeadLosses(&config.Resonant, e.lossScale)
	}

	for _, correction := range e.corrections {
		setModeDecayCorrection(&config.Batter, correction)
		setModeDecayCorrection(&config.Resonant, correction)
	}

	return config, nil
}

// setModeDecayCorrection writes one correction into a head's table, replacing
// any entry for the same mode rather than appending a second one — which the
// configuration rejects outright, and rightly, since two rates for one mode has
// no meaning.
//
// It is applied to both heads because a correction names a mode, not a
// membrane, and the resonant head is AxisymmetricOnly: it has no m > 0 modes at
// all, so an entry for one is inert there and costs a table row.
func setModeDecayCorrection(head *physical.Head, correction physical.ModeDecayCorrection) {
	for index, existing := range head.ModeDecayCorrections {
		if existing.AzimuthalOrder == correction.AzimuthalOrder &&
			existing.RadialOrder == correction.RadialOrder {
			head.ModeDecayCorrections[index] = correction

			return
		}
	}

	head.ModeDecayCorrections = append(head.ModeDecayCorrections, correction)
}

// measure renders one candidate bank at every reference's own velocity and
// reduces each render to features. The returned slice is owned by the evaluator
// and is overwritten by the next call.
//
// One model serves every take. NewDoubleHead is the expensive half — it builds
// both modal banks, the cavity and the coupling matrix, none of which the
// velocity touches — while Reset silences the state a previous take left behind
// and re-seeds the attack layer's noise from the same constant a fresh model
// gets. The two are therefore bit-exact equivalent, which is not a nicety here:
// the checkpoint fingerprint carries the baseline cost, so a per-take render
// that differed from a single-take one by a bit would make two runs of the same
// search refuse to resume each other.
func (e *evaluator) measure(position []float64) ([]match.Features, error) {
	velocities := e.apply(position)

	config, err := e.config()
	if err != nil {
		return nil, err
	}

	model, err := physical.NewDoubleHead(config)
	if err != nil {
		return nil, err
	}

	for index, velocity := range velocities {
		if index > 0 {
			model.Reset()
		}

		if err := e.renderWith(model, velocity); err != nil {
			return nil, err
		}

		features, err := match.Extract(e.buffer, e.sampleRateHz, e.options)
		if err != nil {
			return nil, err
		}

		e.rendered[index] = features
	}

	return e.rendered, nil
}

// renderWith strikes an already-built model once and fills e.buffer.
func (e *evaluator) renderWith(model *physical.DoubleHead, velocity01 float64) error {
	if err := model.Trigger(velocity01); err != nil {
		return err
	}

	model.Render(e.buffer)

	return nil
}

// render is one strike from a fresh model. measure is what the search calls;
// this is for the places that want a single render without a reference to score
// it against, which is every benchmark in this package.
func (e *evaluator) render(velocity01 float64) ([]float64, error) {
	config, err := e.config()
	if err != nil {
		return nil, err
	}

	model, err := physical.NewDoubleHead(config)
	if err != nil {
		return nil, err
	}

	if err := e.renderWith(model, velocity01); err != nil {
		return nil, err
	}

	return e.buffer, nil
}

// cost is the objective mayfly minimizes: the mean distance over every take.
//
// A configuration the model rejects returns +Inf rather than an error: the
// search is allowed to wander into combinations that are not drums (a
// nonlinear coefficient past the anti-alias bound, say), and the cheapest
// honest answer is that they are infinitely far from sounding like one. One
// unusable take poisons the whole evaluation for the same reason: the bank is
// shared, so a bank that cannot produce one of the sixteen hits is not a
// candidate for the drum that produced all sixteen.
//
// The mean, not a trimmed or median aggregate. Every take is a legitimate
// observation of the same instrument, and trimming would drop whichever hits
// the model fits worst — which is precisely the evidence a joint fit exists to
// use. The trimming that is justified happens one level down, inside Distance,
// where it discards outlying *partials* within a take on a measured argument.
// Nothing measured says a whole take should be discarded, so nothing here does.
func (e *evaluator) cost(position []float64) float64 {
	rendered, err := e.measure(position)
	if err != nil {
		return math.Inf(1)
	}

	total := 0.0

	for index, features := range rendered {
		if len(features.Partials) == 0 {
			return math.Inf(1)
		}

		terms := match.Distance(e.references[index], features, e.weights)
		if math.IsNaN(terms.Total) {
			return math.Inf(1)
		}

		total += terms.Total
	}

	return total / float64(len(rendered))
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
	// range rather than a result — the shipped range is wrong, and the search
	// did not converge on anything.
	//
	// Only ever set for a free parameter. A fixed one sits wherever -fix or -set
	// put it, and QUAL sits at exactly 0 in every draft run; calling those pinned
	// would bury the one case worth reading in noise the caller created.
	Pinned bool `json:"pinned,omitempty"`
	// PinnedAt names which stop, "lower" or "upper", because the two ask for
	// opposite repairs and the normalized position alone makes the reader work
	// it out.
	PinnedAt string `json:"pinnedAt,omitempty"`
	// Blind marks a parameter held out of the search because the objective
	// cannot see it, as opposed to one the caller fixed. Both report Fixed, and
	// the distinction matters when reading a report: a blind parameter's value
	// is the shipped default and carries no information about the reference.
	// See blindParameters.
	Blind bool `json:"blind,omitempty"`
}

// Candidate is one evaluated point in the search space: one parameter bank,
// scored against every take.
type Candidate struct {
	// Terms is the aggregate the search minimizes — every field is the mean of
	// that field over Takes, which for Total is the mean of the per-take totals
	// because Distance composes its total linearly. With one take it is that
	// take's terms exactly.
	Terms match.Terms `json:"terms"`
	// TermsVsGate is Terms divided term by term by match.AdoptionGates, which is
	// the form every reading of a report actually wants: the nine raw terms are
	// in nine different units and cannot be compared with each other, and these
	// can. It is stored rather than left to the reader because it was being
	// re-derived by hand outside this tool every time.
	TermsVsGate GateRatios   `json:"termsVsGate"`
	Params      []ParamValue `json:"params"`
	Config physical.PhysicalDrum `json:"config"`
	Takes  []TakeResult          `json:"takes"`
	// Convergence is the winning restart's best cost after each iteration.
	Convergence []float64 `json:"convergence,omitempty"`
}

// TakeResult is how one candidate bank scored against one reference take.
//
// Velocity01 is the interesting field of a joint fit. It is what the search
// concluded about how hard *that* take was struck, arrived at without any
// reference to the file's position in the series, so comparing it against the
// file order tests the labelling instead of trusting it.
type TakeResult struct {
	Path        string         `json:"path"`
	Velocity01  float64        `json:"velocity01"`
	Terms       match.Terms    `json:"terms"`
	TermsVsGate GateRatios     `json:"termsVsGate"`
	Features    match.Features `json:"features"`
}

// meanTerms averages a candidate's per-take terms field by field.
func meanTerms(takes []TakeResult) match.Terms {
	mean := match.Terms{}
	if len(takes) == 0 {
		return mean
	}

	for _, take := range takes {
		mean.PartialFrequency += take.Terms.PartialFrequency
		mean.PartialLevel += take.Terms.PartialLevel
		mean.PartialDecay += take.Terms.PartialDecay
		mean.SpectralEnvelope += take.Terms.SpectralEnvelope
		mean.Envelope += take.Terms.Envelope
		mean.Glide += take.Terms.Glide
		mean.AttackBalance += take.Terms.AttackBalance
		mean.Unmatched += take.Terms.Unmatched
		mean.Spurious += take.Terms.Spurious
		mean.Total += take.Terms.Total
	}

	count := float64(len(takes))
	mean.PartialFrequency /= count
	mean.PartialLevel /= count
	mean.PartialDecay /= count
	mean.SpectralEnvelope /= count
	mean.Envelope /= count
	mean.Glide /= count
	mean.AttackBalance /= count
	mean.Unmatched /= count
	mean.Spurious /= count
	mean.Total /= count

	return mean
}

// pinnedTolerance is how close to a bound counts as pressed against it.
//
// One percent, widened from the half percent this was first written at, and the
// widening is a measured repair rather than a loosening. On the deep series fit
// of reference/tt08x08/lp/hd, DAMP came back at normalized 0.0084 — hard against
// its lower stop, in two independent runs, the most actionable single result
// either produced — and 0.0084 > 0.005, so the report said nothing at all. A
// threshold that misses the case it exists to catch is worse than no threshold,
// because its silence reads as a clean bill.
//
// One percent is still far coarser than the search's own resolution and about
// two and a half steps of the byte the value has to survive persistence in, so
// nothing inside it is distinguishable from the bound. It is deliberately not
// tighter: the cost of a false positive is one printed line a reader dismisses,
// and the cost of a false negative is a fit shipped against a range nobody
// discovered was wrong.
const pinnedTolerance = 0.01

func (e *evaluator) describe(position []float64) (Candidate, error) {
	rendered, err := e.measure(position)
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

		stop := ""

		switch {
		case !free:
		case normalized <= pinnedTolerance:
			stop = "lower"
		case normalized >= 1-pinnedTolerance:
			stop = "upper"
		}

		params[index] = ParamValue{
			Index:      index,
			ID:         spec.ID,
			Label:      spec.Label,
			Unit:       spec.Unit,
			Normalized: normalized,
			Value:      spec.Map(normalized),
			Fixed:      !free,
			Pinned:     stop != "",
			PinnedAt:   stop,
			Blind:      !free && isBlind(spec),
		}
	}

	// apply is called again rather than reusing what measure returned, because
	// measure's velocities slice is scratch and the candidate outlives it.
	velocities := e.apply(position)
	takes := make([]TakeResult, len(rendered))

	for index, features := range rendered {
		terms := match.Distance(e.references[index], features, e.weights)

		takes[index] = TakeResult{
			Path:        e.referencePaths[index],
			Velocity01:  velocities[index],
			Terms:       terms,
			TermsVsGate: gateRatios(terms),
			Features:    features,
		}
	}

	// The mean of the ratios and the ratios of the mean are the same numbers,
	// since a gate is a constant, so this is taken over the aggregate rather
	// than averaged a second way.
	mean := meanTerms(takes)

	return Candidate{
		Terms:       mean,
		TermsVsGate: gateRatios(mean),
		Params:      params,
		Config:      config,
		Takes:       takes,
	}, nil
}

// position reads the current bank back into a search vector, striking every
// take at the same velocity. That is what a baseline wants — the shipped bank
// quoted at one stated dynamic — and never what a fit produces, since the whole
// point of a joint run is that the sixteen velocities come out different.
func (e *evaluator) position(velocity01 float64) []float64 {
	position := make([]float64, e.dimensions())
	for i, index := range e.free {
		position[i] = e.bank[index]
	}

	for i := range e.references {
		position[len(e.free)+i] = velocity01
	}

	return position
}

// blindParameters are bank parameters the objective provably cannot see, and
// which are therefore held at their defaults instead of being searched.
//
// This list is not a tuning decision and nothing should be added to it on a
// hunch. A parameter belongs here only when there is a measurement showing the
// distance cannot respond to it, and the entry must say which one.
//
// `physicalTom.asymmetry` (ASYM) splits a degenerate pair's two members apart in
// frequency, over a 0–2 % range. Two measurements put it here, and the second is
// the one that makes the decision final:
//
//   - The objective cannot resolve the split. The fast estimator merges 15–24 of
//     ~160 matched partials into single peaks, recurring at 304, 351, 586–613 and
//     851 Hz — a 2 % split at 213 Hz is 4.3 Hz, inside one main lobe of an 800 ms
//     Hann window, and no value of MinSeparationHz changes that. So ASYM was being
//     fitted against a target with the asymmetry averaged out of it, and whatever
//     value came back was reported as fitted while resting on nothing.
//   - The split it models is the wrong one anyway. Subband ESPRIT resolves the
//     pairs the fast estimator merges, and their two members differ in **ring
//     time** by a median factor of 1.55 — real, and not an artefact of the
//     estimator: on synthesised pairs with identical damping it reports 1.001.
//     ASYM does not touch damping. So even a target that resolved the pairs would
//     not make ASYM the parameter representing what was measured.
//
// A third measurement, taken for another purpose, agrees: the excitation-gap
// sweep drives ASYM from its default to 1 and the spectral envelope moves from
// 13.02 to 13.12 dB, against 13.02 → 16.18 for the loss-law tilt in the same
// table. The objective is not merely mismeasuring ASYM, it barely responds to it.
//
// Evidence: docs/physical-objective-validation.md §5b and §Result 9,
// docs/physical-excitation-gap.md; the decision is PLAN.md's N15.
//
// ASYM remains a user knob — it is audible, and a player setting it is not making
// a claim about a recording. What it stops being is a fitted result.
var blindParameters = []string{"physicalTom.asymmetry"}

// isBlind reports whether a spec is one the objective cannot see. Both the ID
// and the label are matched, for the same reason -fix accepts either.
func isBlind(spec drum.ParamSpec) bool {
	return slices.Contains(blindParameters, spec.ID) ||
		slices.Contains(blindParameters, spec.Label)
}

// resolveFixed parses -fix ID=value pairs against the parameter table, and
// removes the parameters the objective is blind to from the search.
//
// searching says whether a search will follow. It only gates the "nothing left
// to search" error: fixing the whole bank is meaningless for a search but is
// exactly how -report-only measures one specific drum, which is the only way to
// score a candidate the search already produced — re-measuring a fitted bank at
// a different quality tier, say.
//
// searchBlind puts blindParameters back into the search. It exists so the
// measurement behind that list can be repeated rather than taken on trust — a
// run with it set is evidence about whether the objective has become able to see
// them, not a bank to ship, exactly as -loss-scale is.
func resolveFixed(assignments map[string]float64, searching, searchBlind bool) ([]float64, []int, error) {
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

	for index, spec := range specs {
		if pinned[index] || (!searchBlind && isBlind(spec)) {
			continue
		}

		free = append(free, index)
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
