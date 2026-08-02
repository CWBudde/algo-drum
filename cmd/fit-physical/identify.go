package main

// Identifiability: a central-difference Hessian of the fit objective at a
// stored optimum, and its eigenspectrum.
//
// The converged fits show the textbook sloppy-model signature — a handful of
// stiff parameter combinations and a long tail of soft ones — and until this
// file there was no Jacobian, Hessian or Fisher-information code anywhere in
// the repository, so that was an impression rather than a measurement. The
// reference for reading the answer is Gutenkunst et al., "Universally sloppy
// parameter sensitivities in systems biology models", PLoS Comput. Biol.
// 3(10):e189 (2007): the eigenspectrum of the cost Hessian at the optimum says
// how many parameter *combinations* the data constrains, which is a different
// and much smaller number than how many parameters were fitted.
//
// It lives beside -inspect rather than in a package of its own because it needs
// exactly what -inspect needs and for the same reasons: the same flags, the same
// -fix/-set bank, the same takes, the same evaluator, and — the load-bearing one
// — the same reconstructed Fingerprint. The fingerprint's BaselineCost is what
// stops a stored position being differentiated against a different drum, and a
// Hessian taken against the wrong drum would still produce a plausible-looking
// spectrum with nothing whatever in it.
//
// What this file will not do is repair a measurement. There are three ways a
// finite-difference Hessian of *this* objective silently produces garbage, and
// each of them is refused rather than patched:
//
//   - cost returns +Inf for a configuration the model rejects, so a stencil
//     point can be undefined. It is retried once at h/3 and then nulled, and the
//     entry's row and column leave the matrix.
//   - apply clamps to [0,1], so a component within h of a bound has a one-sided
//     stencil wearing a two-sided one's clothes. It is refused and reported under
//     the report's existing Pinned/PinnedAt vocabulary. This is live rather than
//     hypothetical: DAMP sits on its lower stop in both recorded fits.
//   - the step size is a measurement, not a choice. The objective is
//     deterministic — attackNoiseSeed is fixed and reset per trigger — but it is
//     *piecewise*: FFT bin quantization, peak picking, partial matching and
//     slowestSupportedT60's admissibility all step, and no amount of averaging
//     removes a step. So h is swept, the whole sweep is reported as the evidence,
//     and a parameter with no plateau is reported unavailable rather than filled
//     in at whatever the last h happened to give.

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cwbudde/algo-drum/internal/drum"
)

// defaultHessianScope is the 5×5 block N6 starts from: the three angles that
// carry the model's one exact continuous symmetry, and the two radii whose
// near-product structure predicts a soft direction.
//
// It is the smallest defensible measurement and the only one whose answer is
// known in advance, which is exactly what a new instrument should be pointed at
// first. A central-difference Hessian over D parameters is 2D²+1 evaluations, so
// this is 51 against the 393 the 14 free parameters would cost and the 1801 the
// full 30-wide space (14 free + 16 velocities) would.
const defaultHessianScope = "HIT.A,MIC.A,AXIS,HIT.R,MIC.R"

// hessianSteps is the swept step grid, in normalized parameter units.
//
// Roughly 1e-4 to 3e-2, half-decade spaced. The lower end is where the piecewise
// steps in the objective dominate — a displacement that moves no FFT bin and
// re-matches no partial produces a second difference of zero divided by h², and
// one that moves exactly one produces an enormous one. The upper end is where
// the quadratic model stops being the thing being measured: 3e-2 of HIT.A is
// 10.8°, and the cost surface is not a paraboloid over 10°.
var hessianSteps = []float64{1e-4, 3e-4, 1e-3, 3e-3, 1e-2, 3e-2}

// plateauTolerance is how closely two neighbouring steps in the sweep must agree
// for the pair to count as being on a plateau, as a fraction of the larger.
//
// A third, which is coarse on purpose. What is being looked for is the region
// where the second difference stops depending on h at all; against an objective
// whose own reproducibility gates are set at tens of percent (match.DefaultWeights),
// demanding a few percent here would report "no plateau" for every parameter and
// the tool would never produce a number. Measured against the 5×5 run on
// fits/fit-tt08x08-lp-hd-series-deep.checkpoint, this admits the 1e-3…1e-2 band
// and rejects both ends, which is the shape the sweep actually has.
const plateauTolerance = 1.0 / 3.0

// minimumPlateauLength is how many consecutive steps must agree before the
// agreement counts as a plateau rather than as two numbers that happened to be
// close. Three is the smallest number for which "it stopped depending on h" is a
// statement about a trend instead of about a single pair.
const minimumPlateauLength = 3

// identifyOptions is what the -hessian mode needs from the command line, kept in
// one struct so main.go carries the flag registration and nothing else.
type identifyOptions struct {
	// scope names the components to differentiate: a comma-separated list of
	// parameter labels or IDs, "free" for every free parameter, or "all" for the
	// free parameters and the per-take velocities together.
	scope string
	// outputPath is where the JSON report goes; "-" prints it to stdout.
	outputPath string
	// checkpointPath is quoted in errors and recorded in the report, so a report
	// can be read a year later without guessing which run it describes.
	checkpointPath string
	// baselineCost is the shipped bank's distance from this reference set, as
	// this build measures it. It is checked against the checkpoint's own
	// BaselineCost; see verifyFingerprint.
	baselineCost float64
	// weightsFingerprint identifies the weight set every cost in the report was
	// scored under. A curvature is a second derivative of a total and a total is
	// a property of a weight set, so an eigenvalue does not survive a gate edit
	// any better than a total does.
	weightsFingerprint string
}

// IdentifiabilityReport is what -hessian writes out.
type IdentifiabilityReport struct {
	CheckpointPath     string      `json:"checkpointPath"`
	Fingerprint        Fingerprint `json:"fingerprint"`
	WeightsFingerprint string      `json:"weightsFingerprint"`
	// Cost is the objective at the stored point — the value every second
	// difference in this report is taken around.
	Cost float64 `json:"cost"`
	// StoredCost is what the checkpoint recorded for the same point. The two
	// differ only if the measurement changed under the stored position, which the
	// fingerprint should already have refused; it is here so that "should" is
	// checkable from the report alone.
	StoredCost  float64      `json:"storedCost"`
	Evaluations int          `json:"searchEvaluations"`
	Scope       []ScopeEntry `json:"scope"`
	StepSweep   []StepSweep  `json:"stepSweep"`
	// Step is the h the off-diagonals were taken at, and StepRationale says what
	// in the sweep justified it.
	Step          float64 `json:"step"`
	StepRationale string  `json:"stepRationale"`
	// Hessian is over Scope in Scope's order, with null for any entry whose
	// stencil could not be evaluated. Rows and columns of a null entry are absent
	// from Reduced, which is what the eigenspectrum was taken of.
	Hessian [][]*float64 `json:"hessian"`
	// ActiveBounds lists the components refused because the stencil would have
	// been clamped. Named with the report's own Pinned/PinnedAt vocabulary: a
	// component at a stop is the same finding here as it is in a fit report, and
	// it means the same thing — the range is a bound, not a fit.
	ActiveBounds []ActiveBound `json:"activeBounds"`
	// Dropped lists the components removed for any reason, each with the reason.
	Dropped          []DroppedComponent `json:"dropped"`
	ReducedLabels    []string           `json:"reducedLabels"`
	ReducedDimension int                `json:"reducedDimension"`
	Reduced          [][]float64        `json:"reduced"`
	// Eigenvalues are ascending, and Eigenvectors[k] belongs to Eigenvalues[k].
	Eigenvalues  []float64     `json:"eigenvalues"`
	Eigenvectors []Eigenvector `json:"eigenvectors"`
	// ConstrainedCounts is the sloppy-model reading: how many eigenvalues sit
	// above each decade below the largest. How many parameter combinations the
	// data constrains is a count, and this is that count at every threshold
	// rather than at one someone picked.
	ConstrainedCounts []DecadeCount `json:"constrainedCounts"`
	Predictions       Predictions   `json:"predictions"`
	Timing            Timing        `json:"timing"`
}

// ScopeEntry is one differentiated component, and where it sits.
type ScopeEntry struct {
	Label string `json:"label"`
	ID    string `json:"id,omitempty"`
	Unit  string `json:"unit,omitempty"`
	// Index is the position in the search vector, which is what a checkpoint
	// stores; Normalized and Value are that coordinate read two ways.
	Index      int     `json:"index"`
	Normalized float64 `json:"normalized"`
	Value      float64 `json:"value"`
}

// StepSweep is one component's second difference at every swept h.
type StepSweep struct {
	Label   string       `json:"label"`
	Samples []StepSample `json:"samples"`
	// PlateauFrom and PlateauTo bracket the agreeing run, and are zero when
	// there is none.
	PlateauFrom float64 `json:"plateauFrom,omitempty"`
	PlateauTo   float64 `json:"plateauTo,omitempty"`
	Available   bool    `json:"available"`
	Note        string  `json:"note"`
}

// StepSample is one (h, second difference) pair. Curvature is nil when the
// stencil could not be evaluated at that h, which is a fact about the objective
// at that displacement and is reported rather than skipped.
type StepSample struct {
	Step      float64  `json:"step"`
	Curvature *float64 `json:"curvature"`
	Note      string   `json:"note,omitempty"`
}

// ActiveBound is a component the stencil could not straddle.
type ActiveBound struct {
	Label      string  `json:"label"`
	Normalized float64 `json:"normalized"`
	// PinnedAt is "lower" or "upper", the same two words a fit report uses.
	PinnedAt string  `json:"pinnedAt"`
	Step     float64 `json:"step"`
}

// DroppedComponent records a row and column that left the matrix, and why.
type DroppedComponent struct {
	Label  string `json:"label"`
	Reason string `json:"reason"`
}

// Eigenvector is one eigenvector printed the only way it is readable: in
// parameter labels, sorted by magnitude, largest first.
type Eigenvector struct {
	Eigenvalue float64 `json:"eigenvalue"`
	// RelativeToLargest is |λ| / max|λ|. The absolute value is deliberate: at a
	// point a stochastic search stopped at rather than converged to, a soft
	// direction's curvature is as likely to come back slightly negative as
	// slightly positive, and calling that direction unconstrained is the finding
	// either way.
	RelativeToLargest float64           `json:"relativeToLargest"`
	Components        []VectorComponent `json:"components"`
}

// VectorComponent is one labelled entry of an eigenvector.
type VectorComponent struct {
	Label  string  `json:"label"`
	Weight float64 `json:"weight"`
}

// DecadeCount is how many eigenvalues sit within one decade band of the largest.
type DecadeCount struct {
	Decade    int     `json:"decade"`
	Threshold float64 `json:"threshold"`
	Count     int     `json:"count"`
}

// Predictions scores the two things N6 says this measurement must be checked
// against. Both were corrected against the code before the tool was written, and
// neither is what the item originally claimed.
type Predictions struct {
	// AngleRotation is the exact one: every angle enters the model only as
	// Strike.AngleRad − PrincipalAxisAngleRad or Pickup.AngleRad −
	// PrincipalAxisAngleRad (three sites in internal/physical/modes.go, and no
	// others), and AXIS is a free fit parameter, so rotating HIT.A, MIC.A and
	// AXIS together is an exact symmetry of the shipped model. In normalized
	// coordinates the direction is (1,1,2)/√6 rather than (1,1,1)/√3, because
	// AXIS spans ±90° against the other two's ±180°.
	AngleRotation DirectionProbe `json:"angleRotation"`
	// AngleRotationAxisPinned is the discriminating control. Holding AXIS still
	// breaks the rotation, but only through the 0.4 % split, so it should be soft
	// and not flat. A tool reporting zero for both this and AngleRotation has
	// measured nothing; the pair is what tells a real symmetry from a dead probe.
	AngleRotationAxisPinned DirectionProbe `json:"angleRotationAxisPinned"`
	// RadiusPair is the one N6 originally got wrong. The Φ(r_s)·Φ(r_m) exchange
	// argument is about an idealised amplitude and this model departs from it:
	// the strike side carries a contact footprint the pickup side has not, and
	// the pickup side carries azimuthal directivity, a radiating moment, a
	// distance gain and a near-field term the strike side has not. What the near
	// product structure predicts is a *degenerate pair of minima* — a discrete
	// symmetry, which produces no zero eigenvalue even when it is exact — and,
	// through the same structure, a soft direction. A small eigenvalue is the
	// prediction; a flat one would refute it.
	RadiusPair []DirectionProbe `json:"radiusPair"`
}

// DirectionProbe scores one predicted direction against the measured spectrum.
type DirectionProbe struct {
	Name string `json:"name"`
	// Direction is the unit vector being tested, in parameter labels.
	Direction []VectorComponent `json:"direction"`
	Available bool              `json:"available"`
	Reason    string            `json:"reason,omitempty"`
	// Curvature is the Rayleigh quotient dᵀHd — the second derivative of the
	// objective along the direction itself, which is the quantity the prediction
	// is about and does not depend on how the eigenvectors happened to rotate
	// among near-degenerate eigenvalues.
	Curvature float64 `json:"curvature"`
	// RelativeToLargest is |Curvature| / max|λ|; DecadesBelowLargest is its
	// log10, which is the unit the sloppy-model literature reads spectra in.
	RelativeToLargest   float64 `json:"relativeToLargest"`
	DecadesBelowLargest float64 `json:"decadesBelowLargest"`
	// OverlapWithSmallest is |d·v₀|, the alignment with the softest measured
	// direction, and BestOverlap is the largest |d·vₖ| over all k.
	OverlapWithSmallest   float64 `json:"overlapWithSmallest"`
	BestOverlap           float64 `json:"bestOverlap"`
	BestOverlapIndex      int     `json:"bestOverlapIndex"`
	BestOverlapEigenvalue float64 `json:"bestOverlapEigenvalue"`
	Verdict               string  `json:"verdict"`
}

// Timing is what the next, larger run should be sized from.
type Timing struct {
	Evaluations            int     `json:"evaluations"`
	SecondsPerEvaluation   float64 `json:"secondsPerEvaluation"`
	ElapsedSeconds         float64 `json:"elapsedSeconds"`
	ProjectedFreeBlock     float64 `json:"projectedFreeBlockSeconds"`
	ProjectedFreeBlockDim  int     `json:"projectedFreeBlockDimension"`
	ProjectedFullSpace     float64 `json:"projectedFullSpaceSeconds"`
	ProjectedFullSpaceDim  int     `json:"projectedFullSpaceDimension"`
	ProjectedNoteOnScaling string  `json:"projectedNoteOnScaling"`
}

// fingerprintCopy reads the store's fingerprint under its own lock.
//
// Defined here rather than in checkpoint.go because it exists for this file:
// nothing else needs to read a fingerprint back out of a store, since every
// other caller already has the one it built.
func (s *store) fingerprintCopy() Fingerprint {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.state.Fingerprint
}

// runIdentifiability is the whole -hessian mode.
func runIdentifiability(
	stdout, stderr io.Writer,
	base *evaluator,
	checkpoint *store,
	options identifyOptions,
) error {
	if checkpoint == nil {
		return fmt.Errorf("%w: -hessian needs a -checkpoint to read a stored optimum from",
			errInvalidFitOption)
	}

	fingerprint := checkpoint.fingerprintCopy()

	if err := verifyFingerprint(fingerprint, options); err != nil {
		return err
	}

	snapshot := checkpoint.best()
	if snapshot == nil {
		return fmt.Errorf("%w: %s holds no best point yet",
			errInvalidFitOption, options.checkpointPath)
	}

	local := newEvaluator(base)

	scope, err := resolveScope(local, options.scope)
	if err != nil {
		return err
	}

	if len(snapshot.Position) != local.dimensions() {
		return fmt.Errorf("%w: the stored position is %d wide and this run's search space is %d",
			errInvalidFitOption, len(snapshot.Position), local.dimensions())
	}

	report := IdentifiabilityReport{
		CheckpointPath:     options.checkpointPath,
		Fingerprint:        fingerprint,
		WeightsFingerprint: options.weightsFingerprint,
		StoredCost:         snapshot.Cost,
		Evaluations:        snapshot.Evaluations,
	}

	started := time.Now()
	counted := &counter{cost: local.cost}
	position := slices.Clone(snapshot.Position)

	report.Cost = counted.at(position)
	if !isUsable(report.Cost) {
		return fmt.Errorf("%w: the stored point does not evaluate (%v); it is not an optimum of this objective",
			errInvalidFitOption, report.Cost)
	}

	report.Scope = describeScope(local, scope, position)

	_, _ = fmt.Fprintf(stderr, "identifiability: %d component(s) at total %.6f, sweeping %d step sizes\n",
		len(scope), report.Cost, len(hessianSteps))

	if err := measureHessian(&report, counted, position, scope, report.Cost, stderr); err != nil {
		return err
	}

	report.Timing = sizeFollowUp(counted, started, local)

	writeIdentifiability(stdout, report)

	return writeIdentifiabilityReport(options.outputPath, report)
}

// verifyFingerprint restates, at the point of use, the invariant loadStore
// enforces when it opens an existing file.
//
// loadStore is the primary check and this is not a substitute for it. It is here
// because -hessian is the one mode that reads a stored position and then spends
// a hundred evaluations differentiating around it: if the baseline this build
// measures and the baseline the checkpoint recorded ever came apart, every
// number downstream would be a curvature of one objective evaluated at another
// objective's optimum, and nothing in the output would look wrong.
func verifyFingerprint(fingerprint Fingerprint, options identifyOptions) error {
	if fingerprint.BaselineCost == options.baselineCost {
		return nil
	}

	return fmt.Errorf(
		"%w: %s was written against a baseline cost of %v and this build measures %v, "+
			"so the model or the measurement changed under the stored position",
		errCheckpointMismatch, options.checkpointPath,
		fingerprint.BaselineCost, options.baselineCost)
}

// counter wraps the objective so the report can say what the measurement cost
// and the next, larger run can be sized from it.
type counter struct {
	cost  func([]float64) float64
	calls int
}

func (c *counter) at(position []float64) float64 {
	c.calls++

	return c.cost(position)
}

// isUsable is the one test every stencil point has to pass. +Inf is what cost
// returns for a configuration the model rejects, and NaN is what a broken one
// would return; neither can be differenced, and neither may be substituted for.
func isUsable(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

// resolveScope turns the -hessian argument into positions in the search vector.
//
// "free" is every free parameter, "all" adds the per-take velocities, and
// anything else is a comma-separated list of parameter labels, parameter IDs or
// VELn names. A name that is fixed by -fix or held out as blind is an error
// rather than a silent omission: differentiating a coordinate the search never
// moved would report a curvature of the *bank*, not of the fit.
func resolveScope(local *evaluator, scope string) ([]int, error) {
	specs := drum.PhysicalTomSpecs()

	switch strings.TrimSpace(scope) {
	case "block":
		return resolveScope(local, defaultHessianScope)
	case "free":
		indices := make([]int, len(local.free))
		for i := range local.free {
			indices[i] = i
		}

		return indices, nil
	case "all":
		indices := make([]int, local.dimensions())
		for i := range indices {
			indices[i] = i
		}

		return indices, nil
	}

	var indices []int

	for _, raw := range strings.Split(scope, ",") {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}

		index, err := resolveScopeName(local, specs, name)
		if err != nil {
			return nil, err
		}

		if slices.Contains(indices, index) {
			return nil, fmt.Errorf("%w: %s named twice in -hessian", errInvalidFitOption, name)
		}

		indices = append(indices, index)
	}

	if len(indices) < 2 {
		return nil, fmt.Errorf("%w: -hessian needs at least two components, got %d",
			errInvalidFitOption, len(indices))
	}

	return indices, nil
}

// resolveScopeName maps one name onto a search-vector position.
func resolveScopeName(local *evaluator, specs []drum.ParamSpec, name string) (int, error) {
	if take, ok := strings.CutPrefix(strings.ToUpper(name), "VEL"); ok {
		number, err := strconv.Atoi(take)
		if err != nil || number < 1 || number > len(local.references) {
			return 0, fmt.Errorf("%w: %q is not one of the %d take velocities",
				errInvalidFitOption, name, len(local.references))
		}

		return len(local.free) + number - 1, nil
	}

	spec := slices.IndexFunc(specs, func(candidate drum.ParamSpec) bool {
		return candidate.ID == name || candidate.Label == name
	})
	if spec < 0 {
		return 0, fmt.Errorf("%w: no parameter %q", errInvalidFitOption, name)
	}

	position := slices.Index(local.free, spec)
	if position < 0 {
		return 0, fmt.Errorf(
			"%w: %s is not a free parameter of this run, so the search never moved it "+
				"and its curvature would describe the bank rather than the fit",
			errInvalidFitOption, name)
	}

	return position, nil
}

// componentLabel names a search-vector position the way a reader thinks of it.
func componentLabel(local *evaluator, index int) string {
	if index < len(local.free) {
		return drum.PhysicalTomSpecs()[local.free[index]].Label
	}

	return fmt.Sprintf("VEL%02d", index-len(local.free)+1)
}

// describeScope records where each differentiated component sits, so the report
// says which drum it is a curvature of.
func describeScope(local *evaluator, scope []int, position []float64) []ScopeEntry {
	specs := drum.PhysicalTomSpecs()
	entries := make([]ScopeEntry, len(scope))

	for slot, index := range scope {
		entry := ScopeEntry{
			Label:      componentLabel(local, index),
			Index:      index,
			Normalized: position[index],
			Value:      position[index],
		}

		if index < len(local.free) {
			spec := specs[local.free[index]]
			entry.ID = spec.ID
			entry.Unit = spec.Unit
			entry.Value = spec.Map(position[index])
		}

		entries[slot] = entry
	}

	return entries
}

// measureHessian runs the sweep, picks h, fills the matrix and diagonalizes it.
func measureHessian(
	report *IdentifiabilityReport,
	counted *counter,
	position []float64,
	scope []int,
	cost float64,
	stderr io.Writer,
) error {
	labels := make([]string, len(scope))
	for slot := range scope {
		labels[slot] = report.Scope[slot].Label
	}

	report.StepSweep = sweepSteps(counted, position, scope, labels, cost)

	step, rationale := chooseStep(report.StepSweep)
	report.Step, report.StepRationale = step, rationale

	if step == 0 {
		return fmt.Errorf("%w: the step sweep found no h on a plateau for any component, "+
			"so there is no step size this Hessian could honestly be taken at",
			errInvalidFitOption)
	}

	_, _ = fmt.Fprintf(stderr, "step: h = %g — %s\n", step, rationale)

	keep := admissible(report, position, scope, step)

	report.Hessian = fillHessian(counted, position, scope, cost, step, keep)
	report.Dropped = append(report.Dropped, dropNulls(report.Hessian, labels, keep)...)

	report.ReducedLabels, report.Reduced = reduce(report.Hessian, labels, keep)
	report.ReducedDimension = len(report.ReducedLabels)

	if report.ReducedDimension < 2 {
		return fmt.Errorf("%w: %d component(s) survived the bound and stencil checks, "+
			"which is not a spectrum", errInvalidFitOption, report.ReducedDimension)
	}

	values, vectors := jacobiEigen(report.Reduced)
	report.Eigenvalues = values
	report.Eigenvectors = describeEigenvectors(values, vectors, report.ReducedLabels)
	report.ConstrainedCounts = countDecades(values)
	report.Predictions = scorePredictions(report.Reduced, values, vectors, report.ReducedLabels)

	return nil
}

// sweepSteps measures every component's diagonal second difference at every h.
//
// The diagonal alone, because it is the cheapest thing that shows the plateau
// and because the off-diagonals inherit whichever h it justifies. 2 evaluations
// per (component, h) pair.
func sweepSteps(
	counted *counter,
	position []float64,
	scope []int,
	labels []string,
	cost float64,
) []StepSweep {
	sweeps := make([]StepSweep, len(scope))

	for slot, index := range scope {
		sweep := StepSweep{Label: labels[slot]}

		for _, step := range hessianSteps {
			sample := StepSample{Step: step}

			switch {
			case position[index]-step < 0 || position[index]+step > 1:
				// apply clamps, so this stencil would be one-sided while
				// looking two-sided. Recorded rather than skipped: which steps a
				// component's position rules out is part of why its plateau is
				// where it is.
				sample.Note = "the stencil would cross a [0,1] bound and be clamped"
			default:
				value, ok := secondDifference(counted, position, cost, index, index, step)
				if ok {
					curvature := value
					sample.Curvature = &curvature
				} else {
					sample.Note = "a stencil point was not a drum, at h and at h/3"
				}
			}

			sweep.Samples = append(sweep.Samples, sample)
		}

		sweep.PlateauFrom, sweep.PlateauTo, sweep.Available, sweep.Note = findPlateau(sweep.Samples)
		sweeps[slot] = sweep
	}

	return sweeps
}

// findPlateau reports the widest run of consecutive steps whose curvatures agree
// to plateauTolerance.
//
// "Agree" is relative to the larger of the pair, so a component whose curvature
// is genuinely tiny is not held to an absolute tolerance it could never meet.
// A component with no such run is reported unavailable and is not differentiated
// at all — the alternative, taking whatever the middle h gave, is a number with
// nothing behind it, and this measurement exists precisely to stop that.
func findPlateau(samples []StepSample) (from, to float64, available bool, note string) {
	best, bestStart := 0, -1
	run, start := 1, -1

	for index, sample := range samples {
		if sample.Curvature == nil {
			run, start = 1, -1

			continue
		}

		if start < 0 {
			run, start = 1, index
		} else if agree(*samples[index-1].Curvature, *sample.Curvature) {
			run++
		} else {
			run, start = 1, index
		}

		if run > best {
			best, bestStart = run, start
		}
	}

	if best < minimumPlateauLength {
		return 0, 0, false, fmt.Sprintf(
			"no run of %d consecutive steps agrees to %.0f%%; unavailable, not estimated",
			minimumPlateauLength, 100*plateauTolerance)
	}

	return samples[bestStart].Step, samples[bestStart+best-1].Step, true,
		fmt.Sprintf("%d consecutive steps agree to %.0f%%", best, 100*plateauTolerance)
}

// agree is the plateau test for one neighbouring pair.
func agree(left, right float64) bool {
	scale := max(math.Abs(left), math.Abs(right))
	if scale == 0 {
		// Two exact zeros are a plateau in the only sense available: the
		// objective did not move at all over that pair of displacements.
		return true
	}

	return math.Abs(left-right)/scale <= plateauTolerance
}

// chooseStep picks the one h the off-diagonals are taken at.
//
// The h that lies inside the most components' plateaus, and among ties the
// largest — a larger step inside a plateau has the better ratio of curvature to
// the objective's piecewise steps, which is the noise this whole sweep exists to
// get above. The rationale is returned rather than left implicit because the
// step is the single choice in this measurement that a reader has to be able to
// second-guess.
func chooseStep(sweeps []StepSweep) (float64, string) {
	best, bestCount := 0.0, 0

	for _, step := range hessianSteps {
		count := 0

		for _, sweep := range sweeps {
			if sweep.Available && step >= sweep.PlateauFrom && step <= sweep.PlateauTo {
				count++
			}
		}

		if count > bestCount || (count == bestCount && count > 0 && step > best) {
			best, bestCount = step, count
		}
	}

	if bestCount == 0 {
		return 0, "no component produced a plateau"
	}

	return best, fmt.Sprintf("inside the plateau of %d of %d components, and the largest such step",
		bestCount, len(sweeps))
}

// admissible marks the components whose stencil fits inside [0,1] at the chosen
// step, and records the rest as active bounds.
//
// This is the second of the three silent-garbage routes. apply clamps every
// component to [0,1], so a base point within h of a bound gets f(x+h) from x+h
// and f(x−h) from the bound: a one-sided difference divided by h², reported as
// if it were central. DAMP came back at normalized 0.0084 in both recorded
// series fits, which is inside every step from 1e-2 up, so this is a live case
// and not a defensive flourish.
func admissible(report *IdentifiabilityReport, position []float64, scope []int, step float64) []bool {
	keep := make([]bool, len(scope))

	for slot, index := range scope {
		low, high := position[index]-step, position[index]+step

		switch {
		case low < 0:
			report.ActiveBounds = append(report.ActiveBounds, ActiveBound{
				Label: report.Scope[slot].Label, Normalized: position[index],
				PinnedAt: "lower", Step: step,
			})
		case high > 1:
			report.ActiveBounds = append(report.ActiveBounds, ActiveBound{
				Label: report.Scope[slot].Label, Normalized: position[index],
				PinnedAt: "upper", Step: step,
			})
		default:
			keep[slot] = true
		}
	}

	for _, bound := range report.ActiveBounds {
		report.Dropped = append(report.Dropped, DroppedComponent{
			Label: bound.Label,
			Reason: fmt.Sprintf(
				"pinned at the %s stop (normalized %.4f), so a central stencil at h = %g would be clamped",
				bound.PinnedAt, bound.Normalized, bound.Step),
		})
	}

	return keep
}

// fillHessian evaluates every surviving entry, leaving nil where it could not.
func fillHessian(
	counted *counter,
	position []float64,
	scope []int,
	cost, step float64,
	keep []bool,
) [][]*float64 {
	size := len(scope)
	matrix := make([][]*float64, size)

	for row := range matrix {
		matrix[row] = make([]*float64, size)
	}

	for row := range size {
		if !keep[row] {
			continue
		}

		for column := row; column < size; column++ {
			if !keep[column] {
				continue
			}

			value, ok := secondDifference(counted, position, cost, scope[row], scope[column], step)
			if !ok {
				continue
			}

			entry := value
			matrix[row][column] = &entry
			// Symmetric by construction rather than by measurement: the mixed
			// stencil is already symmetric in its four points, so evaluating the
			// transpose would spend half the budget re-deriving an identity.
			matrix[column][row] = &entry
		}
	}

	return matrix
}

// secondDifference is one entry of the Hessian, central, with the one retry the
// +Inf case is allowed.
//
// The diagonal is the three-point (f₊ − 2f₀ + f₋)/h² and the off-diagonal the
// four-point (f₊₊ − f₊₋ − f₋₊ + f₋₋)/4h². A stencil point that is not a drum
// makes the whole entry undefined; it is retried once at h/3, on the argument
// that a rejected configuration is usually a bound of the *model* rather than of
// the parameter and a third of the way out is often back inside it. If h/3 also
// fails the entry is nil, and the caller drops its row and column. Nothing is
// ever substituted — a zero here would read as "no coupling", which is the exact
// opposite of "we could not tell".
func secondDifference(
	counted *counter,
	position []float64,
	cost float64,
	first, second int,
	step float64,
) (float64, bool) {
	for _, attempt := range []float64{step, step / 3} {
		value, ok := stencil(counted, position, cost, first, second, attempt)
		if ok {
			return value, true
		}
	}

	return 0, false
}

// stencil evaluates one second difference at exactly the step it is given.
func stencil(
	counted *counter,
	position []float64,
	cost float64,
	first, second int,
	step float64,
) (float64, bool) {
	probe := slices.Clone(position)

	if first == second {
		plus, minus := shift(counted, probe, position, first, step), shift(counted, probe, position, first, -step)
		if !isUsable(plus) || !isUsable(minus) {
			return 0, false
		}

		return (plus - 2*cost + minus) / (step * step), true
	}

	total := 0.0

	for _, signs := range [4][2]float64{{1, 1}, {1, -1}, {-1, 1}, {-1, -1}} {
		copy(probe, position)
		probe[first] += signs[0] * step
		probe[second] += signs[1] * step

		value := counted.at(probe)
		if !isUsable(value) {
			return 0, false
		}

		total += signs[0] * signs[1] * value
	}

	return total / (4 * step * step), true
}

// shift evaluates the objective one component away from the base point.
func shift(counted *counter, probe, position []float64, index int, delta float64) float64 {
	copy(probe, position)
	probe[index] += delta

	return counted.at(probe)
}

// dropNulls removes the row and column of every entry the stencil could not
// evaluate, and says which entry did it.
//
// Both indices of a failed off-diagonal go, not one of them: the failure says
// the objective is undefined somewhere in the plane those two span, and there is
// no evidence about which of the two carries it. Iterated, because dropping one
// component cannot resurrect another's missing entry but can leave a matrix
// whose remaining nulls are all in dropped rows.
func dropNulls(matrix [][]*float64, labels []string, keep []bool) []DroppedComponent {
	var dropped []DroppedComponent

	for row := range matrix {
		if !keep[row] {
			continue
		}

		for column := range matrix[row] {
			if !keep[column] || matrix[row][column] != nil {
				continue
			}

			reason := fmt.Sprintf("the (%s, %s) stencil could not be evaluated",
				labels[row], labels[column])

			if keep[row] {
				dropped = append(dropped, DroppedComponent{Label: labels[row], Reason: reason})
				keep[row] = false
			}

			if keep[column] && column != row {
				dropped = append(dropped, DroppedComponent{Label: labels[column], Reason: reason})
				keep[column] = false
			}
		}
	}

	return dropped
}

// reduce extracts the submatrix of the components that survived.
func reduce(matrix [][]*float64, labels []string, keep []bool) ([]string, [][]float64) {
	var kept []int

	for index, ok := range keep {
		if ok {
			kept = append(kept, index)
		}
	}

	names := make([]string, len(kept))
	reduced := make([][]float64, len(kept))

	for row, source := range kept {
		names[row] = labels[source]
		reduced[row] = make([]float64, len(kept))

		for column, other := range kept {
			reduced[row][column] = *matrix[source][other]
		}
	}

	return names, reduced
}

// jacobiEigen diagonalizes a real symmetric matrix by cyclic Jacobi rotations,
// returning the eigenvalues ascending and vectors[k] as the eigenvector of
// values[k].
//
// Deliberately not shared with internal/physical/match/linalg.go, which holds a
// complex Hermitian Jacobi already. That file's own doc says it is "the dense
// complex linear algebra subspace estimation needs, and nothing else", and it
// means it: everything in it is unexported, it exists so that a js/wasm module
// does not acquire a linear-algebra dependency, and widening it for a non-DSP
// consumer in cmd/ would be against its grain and would make it the repository's
// general matrix package by accident. Sixty lines here costs less than that.
//
// Jacobi rather than a reduction plus QL for the same reason linalg.go gives:
// its accuracy on the *small* eigenvalues does not depend on getting a
// tridiagonal reduction right, and the small end is where this entire result
// lives. A sloppy spectrum is read at its floor.
func jacobiEigen(matrix [][]float64) ([]float64, [][]float64) {
	size := len(matrix)
	work := make([][]float64, size)
	vectors := make([][]float64, size)

	for row := range size {
		work[row] = slices.Clone(matrix[row])
		vectors[row] = make([]float64, size)
		vectors[row][row] = 1
	}

	// A hundred sweeps is far past what cyclic Jacobi needs at these sizes
	// (quadratic convergence sets in after three or four); it is a guard against
	// a non-terminating loop, not a tuning parameter.
	for range 100 {
		off := 0.0

		for row := range size {
			for column := row + 1; column < size; column++ {
				off += work[row][column] * work[row][column]
			}
		}

		if off <= 1e-30 {
			break
		}

		for row := range size {
			for column := row + 1; column < size; column++ {
				if work[row][column] == 0 {
					continue
				}

				// The standard stable form: theta is cot(2φ), and taking the
				// smaller root keeps |t| ≤ 1 so the rotation never mixes a large
				// eigenvalue into a small one more than it must.
				theta := (work[column][column] - work[row][row]) / (2 * work[row][column])
				tangent := 1 / (math.Abs(theta) + math.Sqrt(theta*theta+1))

				if theta < 0 {
					tangent = -tangent
				}

				cosine := 1 / math.Sqrt(tangent*tangent+1)
				sine := tangent * cosine

				for k := range size {
					first, second := work[k][row], work[k][column]
					work[k][row] = cosine*first - sine*second
					work[k][column] = sine*first + cosine*second
				}

				for k := range size {
					first, second := work[row][k], work[column][k]
					work[row][k] = cosine*first - sine*second
					work[column][k] = sine*first + cosine*second
				}

				for k := range size {
					first, second := vectors[k][row], vectors[k][column]
					vectors[k][row] = cosine*first - sine*second
					vectors[k][column] = sine*first + cosine*second
				}
			}
		}
	}

	values := make([]float64, size)
	order := make([]int, size)

	for index := range size {
		values[index] = work[index][index]
		order[index] = index
	}

	slices.SortStableFunc(order, func(a, b int) int {
		switch {
		case values[a] < values[b]:
			return -1
		case values[a] > values[b]:
			return 1
		default:
			return 0
		}
	})

	sorted := make([]float64, size)
	columns := make([][]float64, size)

	for slot, index := range order {
		sorted[slot] = values[index]
		columns[slot] = make([]float64, size)

		for row := range size {
			columns[slot][row] = vectors[row][index]
		}
	}

	return sorted, columns
}

// describeEigenvectors turns the columns into the only readable form: labelled
// weights, sorted by magnitude, largest first. A sloppy spectrum's soft vectors
// are read by which parameters they mix, and a bare list of numbers in matrix
// order makes that a manual join against the scope table every single time.
func describeEigenvectors(values []float64, vectors [][]float64, labels []string) []Eigenvector {
	largest := 0.0
	for _, value := range values {
		largest = max(largest, math.Abs(value))
	}

	described := make([]Eigenvector, len(values))

	for index, value := range values {
		components := make([]VectorComponent, len(labels))
		for slot, label := range labels {
			components[slot] = VectorComponent{Label: label, Weight: vectors[index][slot]}
		}

		slices.SortStableFunc(components, func(a, b VectorComponent) int {
			switch {
			case math.Abs(a.Weight) > math.Abs(b.Weight):
				return -1
			case math.Abs(a.Weight) < math.Abs(b.Weight):
				return 1
			default:
				return 0
			}
		})

		relative := 0.0
		if largest > 0 {
			relative = math.Abs(value) / largest
		}

		described[index] = Eigenvector{
			Eigenvalue:        value,
			RelativeToLargest: relative,
			Components:        components,
		}
	}

	return described
}

// countDecades is the sloppy-model reading of a spectrum: how many eigenvalues
// sit above each decade below the largest.
//
// Magnitudes, not signed values. At a point a stochastic search stopped at, the
// softest directions come back with either sign at the level of the objective's
// own piecewise noise, and "λ = −3e−9" and "λ = +3e−9" are the same finding —
// the data does not constrain that combination. Counting only positive
// eigenvalues would report a smaller, flattering number for exactly the
// directions this measurement is about.
func countDecades(values []float64) []DecadeCount {
	largest := 0.0
	for _, value := range values {
		largest = max(largest, math.Abs(value))
	}

	if largest == 0 {
		return nil
	}

	counts := make([]DecadeCount, 0, 12)

	for decade := 1; decade <= 12; decade++ {
		threshold := largest * math.Pow(10, -float64(decade))
		count := 0

		for _, value := range values {
			if math.Abs(value) >= threshold {
				count++
			}
		}

		counts = append(counts, DecadeCount{Decade: decade, Threshold: threshold, Count: count})

		if count == len(values) {
			break
		}
	}

	return counts
}

// scorePredictions scores the two directions N6 is validated against.
func scorePredictions(
	reduced [][]float64,
	values []float64,
	vectors [][]float64,
	labels []string,
) Predictions {
	// (1,1,2)/√6 and not (1,1,1)/√3. The model is invariant under a common
	// rotation of the strike angle, the pickup angle and the asymmetry axis; in
	// *normalized* coordinates the axis moves twice as fast per unit because
	// AXIS spans ±90° where HIT.A and MIC.A span ±180°. Testing (1,1,1) here
	// would report a failed prediction that was the tool's own arithmetic.
	rotation := map[string]float64{"HIT.A": 1, "MIC.A": 1, "AXIS": 2}
	pinned := map[string]float64{"HIT.A": 1, "MIC.A": 1}
	swap := map[string]float64{"HIT.R": 1, "MIC.R": -1}
	together := map[string]float64{"HIT.R": 1, "MIC.R": 1}

	predictions := Predictions{
		AngleRotation: probeDirection(
			"common rotation of HIT.A, MIC.A and AXIS — an exact symmetry of the model",
			rotation, reduced, values, vectors, labels),
		AngleRotationAxisPinned: probeDirection(
			"the same rotation with AXIS held — broken only through the 0.4 % split, so soft, not flat",
			pinned, reduced, values, vectors, labels),
		RadiusPair: []DirectionProbe{
			probeDirection(
				"exchange of HIT.R and MIC.R — a discrete near-symmetry, so soft and never flat",
				swap, reduced, values, vectors, labels),
			probeDirection(
				"HIT.R and MIC.R moved together — the stiff direction of the same pair",
				together, reduced, values, vectors, labels),
		},
	}

	judge(&predictions)

	return predictions
}

// symmetryContrast is how much flatter the free rotation has to be than the
// AXIS-pinned one before the symmetry counts as measured.
//
// A hundredfold, and the contrast rather than an absolute threshold is the test
// on purpose. The rotation's true curvature is exactly zero, so what comes back
// is a floor rather than a measurement: central differences carry an O(h²)
// truncation error, the three angles reach the render through two floating-point
// subtractions, and the objective on top of that is piecewise in bins, peaks and
// admissibility. Any absolute cutoff would therefore be a claim about the
// tool's own noise, which changes with h. The contrast is scale-free and is
// exactly the two-condition test PLAN.md N6 asks for: flat with AXIS free, soft
// with AXIS pinned. A tool reporting zero for both has measured nothing, and
// this ratio is what catches that.
const symmetryContrast = 100

// flatDecades is the corroborating absolute reading: how many decades below the
// largest eigenvalue a direction has to sit before the spectrum alone would call
// it flat. Four, which is where the O(h²) truncation floor sits at the steps
// this sweep admits (h ≈ 1e-3…1e-2 against a curvature scale of order 0.1).
const flatDecades = 4

// judge writes the verdicts, which is done here rather than inside
// probeDirection because the only trustworthy statement about the exact symmetry
// is a comparison between two of the probes.
func judge(predictions *Predictions) {
	rotation, pinned := &predictions.AngleRotation, &predictions.AngleRotationAxisPinned

	if rotation.Available && pinned.Available {
		contrast := math.Inf(1)
		if rotation.Curvature != 0 {
			contrast = math.Abs(pinned.Curvature / rotation.Curvature)
		}

		switch {
		case contrast >= symmetryContrast:
			rotation.Verdict = fmt.Sprintf(
				"borne out: %.1f decades below the largest eigenvalue and %.0f× flatter than "+
					"the same rotation with AXIS held, which is the discriminating pair",
				rotation.DecadesBelowLargest, contrast)
		default:
			rotation.Verdict = fmt.Sprintf(
				"REFUTED: only %.0f× flatter than the AXIS-pinned rotation (%.1f decades below "+
					"the largest), where an exact symmetry should be at the tool's own floor",
				contrast, rotation.DecadesBelowLargest)
		}

		pinned.Verdict = softVerdict(*pinned,
			"the split-broken rotation", "flat, which would mean AXIS does nothing here")
	}

	for index := range predictions.RadiusPair {
		probe := &predictions.RadiusPair[index]
		if probe.Available {
			probe.Verdict = softVerdict(*probe, "this direction",
				"flat, which the exchange argument does not predict — it is discrete, "+
					"and a discrete symmetry produces no zero eigenvalue even when it is exact")
		}
	}
}

// softVerdict reads a direction that was predicted to be soft rather than flat.
func softVerdict(probe DirectionProbe, subject, flatMeans string) string {
	if probe.DecadesBelowLargest >= flatDecades {
		return fmt.Sprintf("REFUTED: %s is %.1f decades below the largest eigenvalue — %s",
			subject, probe.DecadesBelowLargest, flatMeans)
	}

	return fmt.Sprintf("borne out: %s is %.1f decades below the largest eigenvalue — soft, and not flat",
		subject, probe.DecadesBelowLargest)
}

// probeDirection scores one predicted direction against the measured spectrum.
//
// The Rayleigh quotient is the headline rather than an overlap, because an
// overlap is only meaningful when the eigenvalue it belongs to is isolated: two
// near-degenerate soft directions rotate freely into each other and their
// individual eigenvectors carry no information, while dᵀHd is the curvature
// along the predicted direction whatever the eigenvectors do. The overlaps are
// reported beside it as corroboration.
//
// The verdict is deliberately not written here; see judge.
func probeDirection(
	name string,
	weights map[string]float64,
	reduced [][]float64,
	values []float64,
	vectors [][]float64,
	labels []string,
) DirectionProbe {
	probe := DirectionProbe{Name: name}

	direction := make([]float64, len(labels))
	found := 0

	for slot, label := range labels {
		if weight, ok := weights[label]; ok {
			direction[slot] = weight
			found++
		}
	}

	if found != len(weights) {
		probe.Reason = fmt.Sprintf(
			"needs all of %s, and only %d of them survived into the reduced matrix",
			strings.Join(sortedKeys(weights), ", "), found)

		return probe
	}

	norm := 0.0
	for _, weight := range direction {
		norm += weight * weight
	}

	norm = math.Sqrt(norm)
	for slot := range direction {
		direction[slot] /= norm
	}

	probe.Available = true
	probe.Direction = labelled(direction, labels)
	probe.Curvature = rayleigh(reduced, direction)

	largest := 0.0
	for _, value := range values {
		largest = max(largest, math.Abs(value))
	}

	if largest > 0 {
		probe.RelativeToLargest = math.Abs(probe.Curvature) / largest
		probe.DecadesBelowLargest = -math.Log10(probe.RelativeToLargest)
	}

	probe.OverlapWithSmallest = math.Abs(dot(direction, vectors[0]))

	for index, vector := range vectors {
		if overlap := math.Abs(dot(direction, vector)); overlap > probe.BestOverlap {
			probe.BestOverlap = overlap
			probe.BestOverlapIndex = index
			probe.BestOverlapEigenvalue = values[index]
		}
	}

	return probe
}

func sortedKeys(weights map[string]float64) []string {
	keys := make([]string, 0, len(weights))
	for key := range weights {
		keys = append(keys, key)
	}

	slices.Sort(keys)

	return keys
}

func labelled(direction []float64, labels []string) []VectorComponent {
	var components []VectorComponent

	for slot, weight := range direction {
		if weight != 0 {
			components = append(components, VectorComponent{Label: labels[slot], Weight: weight})
		}
	}

	return components
}

// rayleigh is dᵀHd for a unit d.
func rayleigh(matrix [][]float64, direction []float64) float64 {
	total := 0.0

	for row := range matrix {
		for column := range matrix[row] {
			total += direction[row] * matrix[row][column] * direction[column]
		}
	}

	return total
}

func dot(left, right []float64) float64 {
	total := 0.0
	for index := range left {
		total += left[index] * right[index]
	}

	return total
}

// sizeFollowUp turns this run's measured cost per evaluation into the estimate
// the next, larger block should be decided on.
//
// A central-difference Hessian over D parameters is 2D²+1 evaluations: 2D for
// the diagonal, 4·D(D−1)/2 for the off-diagonals, and one for the centre. The
// sweep this run also spends is excluded from the projection, because a larger
// block would reuse the step size this one justified rather than re-measuring
// it — which is the whole reason the sweep is reported.
func sizeFollowUp(counted *counter, started time.Time, local *evaluator) Timing {
	elapsed := time.Since(started).Seconds()
	perEvaluation := elapsed / float64(max(1, counted.calls))

	free := len(local.free)
	full := local.dimensions()

	return Timing{
		Evaluations:           counted.calls,
		SecondsPerEvaluation:  perEvaluation,
		ElapsedSeconds:        elapsed,
		ProjectedFreeBlockDim: free,
		ProjectedFreeBlock:    perEvaluation * float64(2*free*free+1),
		ProjectedFullSpaceDim: full,
		ProjectedFullSpace:    perEvaluation * float64(2*full*full+1),
		ProjectedNoteOnScaling: "2D²+1 evaluations, serial; the step sweep is excluded " +
			"because a larger block reuses the step this run justified",
	}
}

// writeIdentifiability prints the table a person reads.
func writeIdentifiability(stdout io.Writer, report IdentifiabilityReport) {
	_, _ = fmt.Fprintf(stdout, "\nidentifiability at the stored optimum of %s\n", report.CheckpointPath)
	_, _ = fmt.Fprintf(stdout, "total %.6f (checkpoint recorded %.6f) under weight set %s\n",
		report.Cost, report.StoredCost, report.WeightsFingerprint)

	_, _ = fmt.Fprintf(stdout, "\nstep sweep — diagonal second difference at each h:\n%-8s", "PARAM")

	for _, step := range hessianSteps {
		_, _ = fmt.Fprintf(stdout, "%12g", step)
	}

	_, _ = fmt.Fprintf(stdout, "   PLATEAU\n")

	for _, sweep := range report.StepSweep {
		_, _ = fmt.Fprintf(stdout, "%-8s", sweep.Label)

		for _, sample := range sweep.Samples {
			if sample.Curvature == nil {
				_, _ = fmt.Fprintf(stdout, "%12s", "-")

				continue
			}

			_, _ = fmt.Fprintf(stdout, "%12.4g", *sample.Curvature)
		}

		if sweep.Available {
			_, _ = fmt.Fprintf(stdout, "   %g-%g\n", sweep.PlateauFrom, sweep.PlateauTo)

			continue
		}

		_, _ = fmt.Fprintf(stdout, "   none (unavailable)\n")
	}

	_, _ = fmt.Fprintf(stdout, "\nh = %g: %s\n", report.Step, report.StepRationale)

	for _, dropped := range report.Dropped {
		_, _ = fmt.Fprintf(stdout, "dropped %s: %s\n", dropped.Label, dropped.Reason)
	}

	_, _ = fmt.Fprintf(stdout, "\neigenvalues over %v, ascending:\n", report.ReducedLabels)

	for index, vector := range report.Eigenvectors {
		parts := make([]string, 0, len(vector.Components))
		for _, component := range vector.Components {
			parts = append(parts, fmt.Sprintf("%s %+.4f", component.Label, component.Weight))
		}

		_, _ = fmt.Fprintf(stdout, "  %d  %14.6g  (%.2e of the largest)  %s\n",
			index, vector.Eigenvalue, vector.RelativeToLargest, strings.Join(parts, "  "))
	}

	_, _ = fmt.Fprintf(stdout, "\nconstrained combinations, by decade below the largest eigenvalue:\n")

	for _, count := range report.ConstrainedCounts {
		_, _ = fmt.Fprintf(stdout, "  1e-%-2d  >= %12.6g  %d of %d\n",
			count.Decade, count.Threshold, count.Count, report.ReducedDimension)
	}

	_, _ = fmt.Fprintf(stdout, "\npredictions:\n")

	// Printed in the order the two predictions are argued in — the exact
	// symmetry, its AXIS-pinned control, then the radius pair — rather than in
	// struct field order, because the control is only readable beside the
	// symmetry it controls for.
	probes := append([]DirectionProbe{
		report.Predictions.AngleRotation,
		report.Predictions.AngleRotationAxisPinned,
	}, report.Predictions.RadiusPair...)

	for _, probe := range probes {
		if !probe.Available {
			_, _ = fmt.Fprintf(stdout, "  %s\n    unavailable: %s\n", probe.Name, probe.Reason)

			continue
		}

		_, _ = fmt.Fprintf(stdout, "  %s\n    dᵀHd = %.6g, %.1f decades below the largest; "+
			"overlap with the softest eigenvector %.4f\n    %s\n",
			probe.Name, probe.Curvature, probe.DecadesBelowLargest, probe.OverlapWithSmallest, probe.Verdict)
	}

	_, _ = fmt.Fprintf(stdout,
		"\n%d evaluations in %.1f s (%.2f s each). Projected serial cost of the same "+
			"measurement over the %d free parameters: %.0f s; over the whole %d-wide space: %.0f s.\n",
		report.Timing.Evaluations, report.Timing.ElapsedSeconds, report.Timing.SecondsPerEvaluation,
		report.Timing.ProjectedFreeBlockDim, report.Timing.ProjectedFreeBlock,
		report.Timing.ProjectedFullSpaceDim, report.Timing.ProjectedFullSpace)
}

func writeIdentifiabilityReport(path string, report IdentifiabilityReport) error {
	if path == "-" || path == "" {
		return nil
	}

	if err := ensureDir(path); err != nil {
		return err
	}

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(report); err != nil {
		return err
	}

	return file.Close()
}
