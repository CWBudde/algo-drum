package main

import (
	"errors"
	"io"
	"math"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/cwbudde/algo-drum/internal/physical"
	"github.com/cwbudde/algo-drum/internal/physical/match"
)

// identifyProbe builds an evaluator over a synthetic reference, following
// benchEvaluator: the reference is this model's own render of the shipped bank,
// so none of these tests needs a recording and none of them depends on
// reference/ being present.
//
// QUAL is pinned to draft for the reason a fit pins it — it buys mode count with
// CPU and nothing here is about mode count — which also keeps the finite
// difference away from a quantized choice parameter.
func identifyProbe(tb testing.TB) *evaluator {
	tb.Helper()

	const (
		sampleRateHz    = 44100
		durationSeconds = 1.2
	)

	fixed := assignmentFlag{}
	if err := pinQuality(fixed, string(physical.QualityDraft)); err != nil {
		tb.Fatalf("pinQuality: %v", err)
	}

	bank, free, err := resolveFixed(fixed, true, false)
	if err != nil {
		tb.Fatalf("resolveFixed: %v", err)
	}

	options := match.DefaultOptions()

	probe := &evaluator{
		options:         options,
		weights:         match.DefaultWeights(),
		bank:            bank,
		free:            free,
		sampleRateHz:    sampleRateHz,
		durationSeconds: durationSeconds,
		buffer:          make([]float64, int(durationSeconds*sampleRateHz)),
	}

	samples, err := probe.render(1)
	if err != nil {
		tb.Fatalf("render: %v", err)
	}

	target, err := match.Extract(samples, sampleRateHz, options)
	if err != nil {
		tb.Fatalf("extract: %v", err)
	}

	probe.references = []match.Features{target}
	probe.referencePaths = []string{"synthetic"}
	probe.velocities = make([]float64, 1)
	probe.rendered = make([]match.Features, 1)

	return probe
}

// componentOf is the position of one label in the search vector.
func componentOf(tb testing.TB, probe *evaluator, label string) int {
	tb.Helper()

	scope, err := resolveScope(probe, label+",VEL1")
	if err != nil {
		tb.Fatalf("resolveScope %s: %v", label, err)
	}

	return scope[0]
}

// interiorPoint is a bank a little away from the reference's own, so the
// objective is measured where it is locally smooth rather than at the cone tip
// it has at an exact match. A second difference taken at a perfect match
// measures the kink in |·| and scales as 1/h, which is a property of the
// aggregation and not of the drum.
func interiorPoint(tb testing.TB, probe *evaluator) []float64 {
	tb.Helper()

	position := probe.position(1)
	for _, shift := range []struct {
		label string
		delta float64
	}{
		{"B.TUNE", 0.06},
		{"R.TUNE", -0.05},
		{"DAMP", 0.07},
	} {
		position[componentOf(tb, probe, shift.label)] += shift.delta
	}

	return position
}

// TestObjectiveIsInvariantUnderCommonAngleRotation asserts the model symmetry
// the Hessian tool is then checked against, and needs no Hessian to do it.
//
// Every angle in the model enters through exactly two expressions,
// Strike.AngleRad − PrincipalAxisAngleRad and Pickup.AngleRad −
// PrincipalAxisAngleRad — three sites in internal/physical/modes.go and no
// others — and AXIS is a free fit parameter. So rotating HIT.A, MIC.A and AXIS
// together is an exact symmetry of the shipped model at the shipped ASYM. In
// normalized coordinates the direction is (1,1,2)/√6 and not (1,1,1)/√3,
// because AXIS spans ±90° where the other two span ±180°.
//
// The rotation stays inside [0,1] in every component, which also keeps it inside
// the ±2π bound physical/bounds.go puts on strike.angleRad and pickup.angleRad:
// the whole normalized range is ±π.
func TestObjectiveIsInvariantUnderCommonAngleRotation(t *testing.T) {
	t.Parallel()

	probe := identifyProbe(t)
	base := interiorPoint(t, probe)

	strike := componentOf(t, probe, "HIT.A")
	pickup := componentOf(t, probe, "MIC.A")
	axis := componentOf(t, probe, "AXIS")

	reference := probe.cost(base)
	if !isUsable(reference) || reference <= 0 {
		t.Fatalf("the probe point does not score: %v", reference)
	}

	for _, amount := range []float64{0.01, 0.05, -0.03} {
		rotated := slices.Clone(base)
		rotated[strike] += amount
		rotated[pickup] += amount
		rotated[axis] += 2 * amount

		got := probe.cost(rotated)

		// A tenth of a part per million of the cost. Not zero: the three angles
		// reach the render through two floating-point subtractions, so the
		// invariance is exact in the mathematics and holds to rounding in the
		// arithmetic.
		if relative := math.Abs(got-reference) / reference; relative > 1e-7 {
			t.Errorf("rotating by %v moved the cost from %v to %v (relative %g); "+
				"the common angle rotation is supposed to be an exact symmetry",
				amount, reference, got, relative)
		}
	}

	// The control, and the half that makes the test discriminating: holding AXIS
	// still breaks the rotation, so the same displacement must move the cost. A
	// test that only checked the invariance would pass against an objective that
	// had stopped depending on the angles at all.
	held := slices.Clone(base)
	held[strike] += 0.05
	held[pickup] += 0.05

	if got := probe.cost(held); math.Abs(got-reference)/reference < 1e-4 {
		t.Errorf("rotating HIT.A and MIC.A with AXIS held moved the cost from %v to %v, "+
			"which is not a break; the probe cannot tell a symmetry from a dead parameter",
			reference, got)
	}
}

// hessianOver builds the Hessian of the real objective over the named labels at
// a fixed step, and returns the reduced matrix with its spectrum.
//
// The step is fixed rather than swept here on purpose. The sweep is a
// measurement of *this reference set's* piecewise structure and belongs to the
// real run; what these tests are about is whether the Hessian machinery recovers
// a symmetry the model provably has, and re-measuring the plateau on a synthetic
// target would only add evaluations and a second thing to fail.
func hessianOver(
	tb testing.TB,
	probe *evaluator,
	position []float64,
	step float64,
	labels ...string,
) (IdentifiabilityReport, []float64, [][]float64) {
	tb.Helper()

	scope, err := resolveScope(probe, strings.Join(labels, ","))
	if err != nil {
		tb.Fatalf("resolveScope: %v", err)
	}

	counted := &counter{cost: probe.cost}

	cost := counted.at(position)
	if !isUsable(cost) {
		tb.Fatalf("the probe point does not score: %v", cost)
	}

	report := IdentifiabilityReport{Cost: cost, Scope: describeScope(probe, scope, position)}
	names := make([]string, len(scope))

	for slot := range scope {
		names[slot] = report.Scope[slot].Label
	}

	keep := admissible(&report, position, scope, step)
	report.Hessian = fillHessian(counted, position, scope, cost, step, keep)
	report.Dropped = append(report.Dropped, dropNulls(report.Hessian, names, keep)...)
	report.ReducedLabels, report.Reduced = reduce(report.Hessian, names, keep)
	report.ReducedDimension = len(report.ReducedLabels)

	if report.ReducedDimension != len(labels) {
		tb.Fatalf("only %d of %d components survived: %v", report.ReducedDimension, len(labels), report.Dropped)
	}

	values, vectors := jacobiEigen(report.Reduced)
	report.Eigenvalues = values
	report.Eigenvectors = describeEigenvectors(values, vectors, report.ReducedLabels)
	report.ConstrainedCounts = countDecades(values)
	report.Predictions = scorePredictions(report.Reduced, values, vectors, report.ReducedLabels)

	return report, values, vectors
}

// TestHessianRecoversTheFlatAngleDirection is the tool checked against the
// symmetry the test above establishes: the measured 3×3 over HIT.A, MIC.A and
// AXIS must be near-singular along (1,1,2)/√6, and must not be along the same
// rotation with AXIS held.
func TestHessianRecoversTheFlatAngleDirection(t *testing.T) {
	t.Parallel()

	probe := identifyProbe(t)
	report, values, _ := hessianOver(t, probe, interiorPoint(t, probe), 3e-3,
		"HIT.A", "MIC.A", "AXIS")

	rotation := report.Predictions.AngleRotation
	pinned := report.Predictions.AngleRotationAxisPinned

	if !rotation.Available || !pinned.Available {
		t.Fatalf("both angle probes should be available, got %+v and %+v", rotation, pinned)
	}

	// The two-condition test PLAN.md N6 asks for. Zero in both would mean the
	// probe is dead rather than that the model is symmetric.
	if pinned.RelativeToLargest < 1e-3 {
		t.Errorf("the AXIS-pinned rotation came back at %g of the largest eigenvalue, "+
			"which is not a break: the probe is measuring nothing", pinned.RelativeToLargest)
	}

	contrast := math.Abs(pinned.Curvature / rotation.Curvature)
	if contrast < symmetryContrast {
		t.Errorf("the free rotation is only %.1f× flatter than the AXIS-pinned one "+
			"(dᵀHd %g against %g); an exact symmetry should sit at the tool's floor",
			contrast, rotation.Curvature, pinned.Curvature)
	}

	if !strings.HasPrefix(rotation.Verdict, "borne out") {
		t.Errorf("verdict: %s", rotation.Verdict)
	}

	// The softest measured direction should be the symmetry itself, which is the
	// form the answer takes in a report: eigenvalues ascending, and the first
	// eigenvector is the combination the data cannot see.
	if rotation.OverlapWithSmallest < 0.95 {
		t.Errorf("the smallest eigenvector overlaps (1,1,2)/√6 by only %.4f; eigenvalues %v",
			rotation.OverlapWithSmallest, values)
	}
}

// TestHessianReportsSoftRadiusPairWithoutClaimingFlatness checks the prediction
// N6 originally got wrong.
//
// The Φ(r_s)·Φ(r_m) exchange argument is about an idealised amplitude, and this
// model departs from it on both sides: the strike side carries a contact
// footprint the pickup side has not, and the pickup side carries azimuthal
// directivity, a radiating moment, a distance gain and a near-field term the
// strike side has not. What the near-product structure predicts is a soft
// direction, and what an exchange symmetry would give is a second minimum rather
// than a flat direction — a discrete symmetry produces no zero eigenvalue even
// when it is exact. So the test asserts softness *and* asserts the tool does not
// claim flatness.
func TestHessianReportsSoftRadiusPairWithoutClaimingFlatness(t *testing.T) {
	t.Parallel()

	probe := identifyProbe(t)
	report, values, _ := hessianOver(t, probe, interiorPoint(t, probe), 3e-3, "HIT.R", "MIC.R")

	swap := report.Predictions.RadiusPair[0]
	together := report.Predictions.RadiusPair[1]

	if !swap.Available || !together.Available {
		t.Fatalf("both radius probes should be available, got %+v and %+v", swap, together)
	}

	if swap.DecadesBelowLargest >= flatDecades {
		t.Errorf("the exchange direction came back %.1f decades below the largest eigenvalue, "+
			"i.e. flat; the exchange is discrete and predicts no zero eigenvalue",
			swap.DecadesBelowLargest)
	}

	if !strings.HasPrefix(swap.Verdict, "borne out") {
		t.Errorf("verdict: %s", swap.Verdict)
	}

	// Softness is a comparison, so it is asserted as one: the exchange direction
	// must be the softer of the pair's two.
	if math.Abs(swap.Curvature) >= math.Abs(together.Curvature) {
		t.Errorf("exchange dᵀHd %g is not softer than the common direction's %g; eigenvalues %v",
			swap.Curvature, together.Curvature, values)
	}
}

// TestHessianRefusesInfiniteAndBoundedEntries covers the two ways a stencil can
// be undefined, against a synthetic objective so the refusal is exercised
// exactly rather than hoped for.
func TestHessianRefusesInfiniteAndBoundedEntries(t *testing.T) {
	t.Parallel()

	const step = 0.01

	// Four components: one hard against the lower bound, one whose neighbourhood
	// the model rejects at every scale, one it rejects only at the full step, and
	// one ordinary.
	position := []float64{0.001, 0.5, 0.5, 0.5}
	scope := []int{0, 1, 2, 3}
	labels := []string{"BOUND", "INF", "NARROW", "GOOD"}

	counted := &counter{cost: func(probe []float64) float64 {
		if probe[1] != position[1] {
			return math.Inf(1)
		}

		if math.Abs(probe[2]-position[2]) > step/2 {
			return math.Inf(1)
		}

		total := 0.0
		for index, value := range probe {
			total += float64(index+1) * value * value
		}

		return total
	}}

	report := IdentifiabilityReport{Scope: []ScopeEntry{
		{Label: "BOUND"}, {Label: "INF"}, {Label: "NARROW"}, {Label: "GOOD"},
	}}

	cost := counted.at(position)

	keep := admissible(&report, position, scope, step)

	if len(report.ActiveBounds) != 1 || report.ActiveBounds[0].Label != "BOUND" ||
		report.ActiveBounds[0].PinnedAt != "lower" {
		t.Fatalf("active bounds: %+v", report.ActiveBounds)
	}

	if keep[0] {
		t.Error("a component within h of a bound must not be differentiated: its stencil is one-sided")
	}

	report.Hessian = fillHessian(counted, position, scope, cost, step, keep)

	// NARROW is rescued by the single retry at h/3, which is the whole point of
	// the retry: a rejected configuration is usually a bound of the model rather
	// than of the parameter.
	if report.Hessian[2][2] == nil {
		t.Error("NARROW should have been recovered at h/3")
	}

	if report.Hessian[1][1] != nil {
		t.Error("INF must be null, never substituted")
	}

	report.Dropped = append(report.Dropped, dropNulls(report.Hessian, labels, keep)...)
	report.ReducedLabels, report.Reduced = reduce(report.Hessian, labels, keep)

	if !slices.Equal(report.ReducedLabels, []string{"NARROW", "GOOD"}) {
		t.Errorf("reduced to %v, want the two components with two-sided finite stencils",
			report.ReducedLabels)
	}

	// The reduced matrix must hold no substituted value: GOOD's own curvature is
	// 2·4 = 8 for the term 4x², and the pair is uncoupled.
	if got := report.Reduced[1][1]; math.Abs(got-8) > 1e-6 {
		t.Errorf("GOOD curvature %v, want 8", got)
	}

	if got := report.Reduced[0][1]; math.Abs(got) > 1e-6 {
		t.Errorf("off-diagonal %v, want 0 for a separable objective", got)
	}
}

// TestStepSweepReportsAParameterWithNoPlateauAsUnavailable pins the third
// refusal: a step size is measured, not chosen, and a component whose second
// difference never stops depending on h has no curvature this tool may report.
func TestStepSweepReportsAParameterWithNoPlateauAsUnavailable(t *testing.T) {
	t.Parallel()

	flat := make([]StepSample, 0, len(hessianSteps))
	sloped := make([]StepSample, 0, len(hessianSteps))

	for _, step := range hessianSteps {
		// A genuine curvature: the same number at every step.
		steady := 4.0
		// A pure step artefact: one quantum of cost divided by h², which grows
		// without bound as h shrinks and is what the piecewise objective does.
		artefact := 1e-3 / (step * step)

		flat = append(flat, StepSample{Step: step, Curvature: &steady})
		sloped = append(sloped, StepSample{Step: step, Curvature: &artefact})
	}

	steady := StepSweep{Label: "STEADY", Samples: flat}
	steady.PlateauFrom, steady.PlateauTo, steady.Available, steady.Note = findPlateau(flat)

	noisy := StepSweep{Label: "NOISY", Samples: sloped}
	noisy.PlateauFrom, noisy.PlateauTo, noisy.Available, noisy.Note = findPlateau(sloped)

	if !steady.Available {
		t.Errorf("a constant second difference is a plateau: %s", steady.Note)
	}

	if noisy.Available {
		t.Errorf("a 1/h² artefact is not a plateau: %v-%v", noisy.PlateauFrom, noisy.PlateauTo)
	}

	step, rationale := chooseStep([]StepSweep{steady, noisy})
	if step < steady.PlateauFrom || step > steady.PlateauTo {
		t.Errorf("chose h = %g outside the only plateau (%v-%v): %s",
			step, steady.PlateauFrom, steady.PlateauTo, rationale)
	}

	if _, rationale := chooseStep([]StepSweep{noisy}); rationale != "no component produced a plateau" {
		t.Errorf("rationale %q", rationale)
	}
}

// TestJacobiEigenMatchesAnalyticSpectrum builds a matrix from a known
// spectrum spanning nine decades and asks for it back. The decades are the
// point: a sloppy spectrum is read at its floor, and an eigensolver that is only
// accurate on the large end would answer this measurement's actual question
// wrongly while looking fine.
func TestJacobiEigenMatchesAnalyticSpectrum(t *testing.T) {
	t.Parallel()

	want := []float64{1e-9, 1e-6, 1e-3, 1}
	basis := orthonormalBasis(t, [][]float64{
		{1, 1, 0, 0},
		{0, 1, 1, 0},
		{0, 0, 1, 1},
		{1, 0, 0, 1.5},
	})

	matrix := make([][]float64, len(want))
	for row := range matrix {
		matrix[row] = make([]float64, len(want))
	}

	for index, value := range want {
		for row := range matrix {
			for column := range matrix[row] {
				matrix[row][column] += value * basis[index][row] * basis[index][column]
			}
		}
	}

	values, vectors := jacobiEigen(matrix)

	for index, value := range values {
		if relative := math.Abs(value-want[index]) / want[index]; relative > 1e-10 {
			t.Errorf("eigenvalue %d: got %g, want %g (relative %g)", index, value, want[index], relative)
		}

		// Hv = λv, checked directly rather than through the eigenvectors'
		// orthogonality alone: an orthonormal set that solves nothing would pass
		// the weaker test.
		for row := range matrix {
			product := 0.0
			for column := range matrix[row] {
				product += matrix[row][column] * vectors[index][column]
			}

			if math.Abs(product-value*vectors[index][row]) > 1e-12 {
				t.Errorf("eigenvector %d row %d: Hv = %g, λv = %g",
					index, row, product, value*vectors[index][row])
			}
		}
	}

	counts := countDecades(values)
	for _, count := range counts {
		want := 1 + count.Decade/3
		if count.Decade%3 == 0 && count.Count != want {
			t.Errorf("decade %d: %d eigenvalues above %g, want %d",
				count.Decade, count.Count, count.Threshold, want)
		}
	}
}

// orthonormalBasis is Gram-Schmidt over the rows, used only to build a test
// matrix with a chosen spectrum.
func orthonormalBasis(tb testing.TB, rows [][]float64) [][]float64 {
	tb.Helper()

	basis := make([][]float64, len(rows))

	for index, row := range rows {
		vector := slices.Clone(row)

		for _, earlier := range basis[:index] {
			projection := dot(vector, earlier)
			for slot := range vector {
				vector[slot] -= projection * earlier[slot]
			}
		}

		norm := math.Sqrt(dot(vector, vector))
		if norm < 1e-9 {
			tb.Fatalf("row %d is dependent on the earlier ones", index)
		}

		for slot := range vector {
			vector[slot] /= norm
		}

		basis[index] = vector
	}

	return basis
}

// TestIdentifiabilityRefusesAFingerprintMismatch is the guard that stops a
// stored position being differentiated against a different drum. The baseline
// cost in the fingerprint is measured end to end through the same synthesis and
// the same feature extraction the search uses, so any edit that moves a rendered
// sample or a measured feature moves it — and a Hessian taken across that change
// would be a curvature of one objective at another objective's optimum, with
// nothing in the output to show it.
func TestIdentifiabilityRefusesAFingerprintMismatch(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "fit.checkpoint")

	stored := Fingerprint{Reference: "synthetic", Quality: "draft", BaselineCost: 12.5}

	checkpoint, err := loadStore(path, stored)
	if err != nil {
		t.Fatalf("loadStore: %v", err)
	}

	checkpoint.recordBest(12.5, []float64{0.5, 0.5}, 1)

	if err := checkpoint.flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	// The primary check, on the way in.
	moved := stored
	moved.BaselineCost = 12.500000000000002

	if _, err := loadStore(path, moved); !errors.Is(err, errCheckpointMismatch) {
		t.Errorf("loadStore across a moved baseline: %v", err)
	}

	// And restated at the point of use, which is where the hundred evaluations
	// are about to be spent.
	err = runIdentifiability(io.Discard, io.Discard, identifyProbe(t), checkpoint, identifyOptions{
		scope:          defaultHessianScope,
		outputPath:     "-",
		checkpointPath: path,
		baselineCost:   moved.BaselineCost,
	})
	if !errors.Is(err, errCheckpointMismatch) {
		t.Errorf("runIdentifiability across a moved baseline: %v", err)
	}
}
