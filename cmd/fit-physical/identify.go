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
	// rebased is set when reconcileBaseline let a drifted baseline through, so
	// the report can say so rather than the fact living only in a stderr line.
	rebased bool
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
	StoredCost float64 `json:"storedCost"`
	// BaselineDrift is present only when the checkpoint's recorded baseline no
	// longer matches this build's and -hessian accepted it anyway. A report
	// carrying it was made across a change in the last bits of the measurement;
	// see baselineDriftTolerance for why that is admissible here and nowhere else.
	BaselineDrift *BaselineDrift `json:"baselineDrift,omitempty"`
	Evaluations   int            `json:"searchEvaluations"`
	Scope         []ScopeEntry   `json:"scope"`
	StepSweep     []StepSweep    `json:"stepSweep"`
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

// BaselineDrift is the checkpoint's baseline against this build's.
type BaselineDrift struct {
	Stored   float64 `json:"stored"`
	Measured float64 `json:"measured"`
	Relative float64 `json:"relative"`
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
	// Curvature is the second difference measured *along the direction itself*,
	// with its own three-point stencil at the report's step. Two extra
	// evaluations, and it is the headline number because it is the only one that
	// survives this objective: the model's angle symmetry is exact, so a stencil
	// that moves along it lands on bit-identical renders and returns a zero that
	// means something.
	Curvature float64 `json:"curvature"`
	// RayleighQuotient is dᵀHd read off the assembled matrix. For a quadratic
	// the two are the same number. They are not the same number here, and the
	// gap is a measurement rather than an error: the objective is piecewise, so
	// each coordinate stencil carries its own jumps and the sum of nine of them
	// does not cancel the way the function itself does. Reported beside the
	// direct measurement so the size of that gap is on the record.
	RayleighQuotient float64 `json:"rayleighQuotient"`
	// RelativeToLargest is |Curvature| / max|λ|; DecadesBelowLargest is its
	// log10, which is the unit the sloppy-model literature reads spectra in.
	RelativeToLargest   float64 `json:"relativeToLargest"`
	DecadesBelowLargest float64 `json:"decadesBelowLargest"`
	// OverlapWithSmallest is |d·v₀| — the alignment with the *first* eigenvector
	// in ascending order, which is the softest direction only when the spectrum
	// is positive. It need not be here: the stored point is where a stochastic
	// search stopped rather than a converged minimum, so a negative eigenvalue
	// sorts to the front. BestOverlap is the largest |d·vₖ| over every k and
	// BestOverlapIndex says which, which is what to read when the two disagree.
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

	if options.rebased {
		report.BaselineDrift = &BaselineDrift{
			Stored:   fingerprint.BaselineCost,
			Measured: options.baselineCost,
			Relative: math.Abs(fingerprint.BaselineCost-options.baselineCost) / math.Abs(options.baselineCost),
		}

		_, _ = fmt.Fprintf(stderr,
			"WARNING: %s records a baseline of %v and this build measures %v (relative %.3g). "+
				"The stored position is still this drum, so the measurement goes ahead and the "+
				"report records the drift — but the checkpoint can no longer be resumed.\n",
			options.checkpointPath, fingerprint.BaselineCost, options.baselineCost,
			report.BaselineDrift.Relative)
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

// baselineDriftTolerance is how far the checkpoint's recorded baseline may sit
// from this build's before -hessian refuses the stored position, as a fraction.
//
// A part in a billion, and the *only* mode that gets a tolerance at all. A
// resume must be bit-exact, because it forms a best-of across measurements taken
// by two builds and a mixture of two objectives is not a fit; loadStore is right
// to refuse on the last bit. -hessian mixes nothing: it reads a stored *position*
// and re-measures everything around it with the current build, so a baseline that
// has moved in its twelfth significant figure changes which drum is being
// differentiated not at all.
//
// The number is set from the drift this repository actually has. The deep series
// checkpoint (fits/fit-tt08x08-lp-hd-series-deep.checkpoint) records a baseline of
// 39.882034409395786 and this build measures 39.882034409620715 — a relative
// 5.6e-12, deterministic across runs, and left behind by one of the physical
// model's performance refactors, which were meant to be bit-exact and are not
// against this path. A genuine change to the model or the measurement moves the
// baseline by percent, not by parts in a trillion, so a billionth is three
// decades of headroom over the drift and three below anything that would matter.
const baselineDriftTolerance = 1e-9

// reconcileBaseline lets -hessian past loadStore's bit-exact fingerprint check
// when the only disagreement is a baseline that has drifted within tolerance.
//
// It edits the fingerprint the caller is about to hand to loadStore rather than
// touching loadStore, because every other mode must keep the strict check: this
// is a statement about what -hessian does with a checkpoint, not a loosening of
// what a checkpoint means. Nothing else in the fingerprint is touched, so a
// changed reference, quality, seed or fixed parameter is refused exactly as
// before, by the same code and with the same message.
//
// The checkpoint is never written back. -hessian records no restarts and does not
// flush, so the file keeps the baseline it was written with.
func reconcileBaseline(fingerprint *Fingerprint, wanted bool, path string) (bool, error) {
	if !wanted || path == "" {
		return false, nil
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		// A missing file is not this function's business: loadStore creates one,
		// and the "holds no best point yet" error is the right thing to say.
		return false, nil //nolint:nilerr // an unreadable checkpoint is loadStore's error to report.
	}

	var existing Checkpoint
	if err := json.Unmarshal(raw, &existing); err != nil {
		return false, nil //nolint:nilerr // likewise: loadStore reports a corrupt checkpoint.
	}

	stored := existing.Fingerprint.BaselineCost
	if stored == fingerprint.BaselineCost || stored == 0 {
		return false, nil
	}

	drift := math.Abs(stored-fingerprint.BaselineCost) / math.Abs(fingerprint.BaselineCost)
	if drift > baselineDriftTolerance {
		return false, nil
	}

	fingerprint.BaselineCost = stored

	return true, nil
}

// verifyFingerprint restates, at the point of use, the invariant loadStore
// enforces when it opens an existing file.
//
// loadStore is the primary check and this is not a substitute for it. It is here
// because -hessian is the one mode that reads a stored position and then spends
// a hundred evaluations differentiating around it: if the baseline this build
// measures and the baseline the checkpoint recorded ever came apart by anything
// real, every number downstream would be a curvature of one objective evaluated
// at another objective's optimum, and nothing in the output would look wrong.
func verifyFingerprint(fingerprint Fingerprint, options identifyOptions) error {
	if fingerprint.BaselineCost == options.baselineCost {
		return nil
	}

	drift := math.Abs(fingerprint.BaselineCost-options.baselineCost) / math.Abs(options.baselineCost)
	if drift <= baselineDriftTolerance {
		return nil
	}

	return fmt.Errorf(
		"%w: %s was written against a baseline cost of %v and this build measures %v "+
			"(relative %.3g, past the %.0e -hessian tolerates), so the model or the "+
			"measurement changed under the stored position",
		errCheckpointMismatch, options.checkpointPath,
		fingerprint.BaselineCost, options.baselineCost, drift, baselineDriftTolerance,
	)
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
			errInvalidFitOption, name,
		)
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

	if report.BaselineDrift != nil {
		_, _ = fmt.Fprintf(stdout,
			"the checkpoint's baseline is %.17g and this build measures %.17g "+
				"(relative %.3g): the stored position is the same drum, the checkpoint "+
				"is no longer resumable.\n",
			report.BaselineDrift.Stored, report.BaselineDrift.Measured, report.BaselineDrift.Relative)
	}

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

		_, _ = fmt.Fprintf(stdout, "  %s\n    curvature along it %.6g (%.1f decades below the "+
			"largest eigenvalue); dᵀHd off the matrix %.6g; overlap with the softest "+
			"eigenvector %.4f\n    %s\n",
			probe.Name, probe.Curvature, probe.DecadesBelowLargest, probe.RayleighQuotient,
			probe.OverlapWithSmallest, probe.Verdict)
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
