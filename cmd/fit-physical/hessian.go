package main

// The numerics of the -hessian mode: the step sweep, the central-difference
// stencils, the reduction that drops what could not be measured, the cyclic
// Jacobi eigensolver, and the scoring of the two directions PLAN.md N6 predicts.
//
// Split from identify.go, which holds the mode itself and the shape of its
// report, only because the two together run past this repository's 1500-line
// file limit. The division is a real one all the same: everything here is
// arithmetic on a cost function and knows nothing about checkpoints, evaluators
// or the parameter table, which is what makes it testable without a recording.

import (
	"fmt"
	"io"
	"math"
	"slices"
	"strings"
)

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

	report.StepSweep = sweepSteps(counted, position, scope, labels, cost, stderr)

	step, rationale := chooseStep(report.StepSweep)
	report.Step, report.StepRationale = step, rationale

	// The predictions are scored whether or not a step size was justified, and
	// deliberately so. They need no matrix: each is a *direction*, and the second
	// difference along a direction is its own three-point stencil. That makes
	// them the part of this measurement that survives an objective too rough to
	// differentiate coordinate-wise — which is the case the sweep above has just
	// found this one to be — and they are the two things N6 is validated against.
	report.Predictions = scorePredictions(sweepDirections(counted, position, scope, labels, cost, stderr))

	if step == 0 {
		return fmt.Errorf("%w: the step sweep found no h on a plateau for any component, "+
			"so there is no step size this Hessian could honestly be taken at; the sweep "+
			"and the direction probes are in the report, which is the evidence for that",
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
	relateToSpectrum(&report.Predictions, report.Reduced, values, vectors, report.ReducedLabels, step)

	return nil
}

// directionIndex maps each differentiated component's label to its place in the
// search vector. Every component in scope, not only the ones a Hessian kept: a
// direction probe is a single stencil that never touches the matrix, so a
// component the matrix had to drop can still be walked along.
func directionIndex(scope []int, labels []string) map[string]int {
	index := make(map[string]int, len(scope))
	for slot, component := range scope {
		index[labels[slot]] = component
	}

	return index
}

// sweepDirections measures the curvature along each predicted direction at every
// swept step.
//
// A direction probe is not the same thing as reading dᵀHd off the assembled
// matrix, and the difference is the whole reason it exists. A coordinate stencil
// crosses this objective's jumps — on reference/tt08x08/lp/hd the diagonal second
// difference in HIT.A runs 4.9e6 at h = 1e-4 down to 1.2e3 at h = 3e-2, which is
// a numerator that barely moves while h² moves by 1e5, i.e. a staircase and not a
// curvature — and a sum of nine such stencils does not cancel. A stencil that
// walks *along* an exact symmetry lands on renders identical to rounding, jumps
// included, because a jump surface of an invariant function contains the
// invariant direction.
//
// Swept over the same steps as the diagonals rather than taken at one, because
// without a plateau there is no privileged h, and the prediction being tested is
// scale-free: an exact symmetry reads at the tool's floor at *every* step.
func sweepDirections(
	counted *counter,
	position []float64,
	scope []int,
	labels []string,
	cost float64,
	stderr io.Writer,
) map[string][]StepSample {
	index := directionIndex(scope, labels)
	samples := make(map[string][]StepSample, len(predictedDirections))

	for _, predicted := range predictedDirections {
		row, ok := sweepDirection(counted, position, cost, index, predicted.weights)
		if !ok {
			continue
		}

		samples[predicted.key] = row

		_, _ = fmt.Fprintf(stderr, "  probe %-8s %s\n", predicted.key, formatSamples(row))
	}

	return samples
}

// sweepDirection is one direction's row, or false when the scope does not carry
// every component the direction is made of.
func sweepDirection(
	counted *counter,
	position []float64,
	cost float64,
	index map[string]int,
	weights map[string]float64,
) ([]StepSample, bool) {
	direction := make(map[int]float64, len(weights))
	norm := 0.0

	for label, weight := range weights {
		component, ok := index[label]
		if !ok {
			return nil, false
		}

		direction[component] = weight
		norm += weight * weight
	}

	norm = math.Sqrt(norm)
	row := make([]StepSample, 0, len(hessianSteps))

	for _, step := range hessianSteps {
		sample := StepSample{Step: step}
		plus, minus := slices.Clone(position), slices.Clone(position)
		bounded := false

		for component, weight := range direction {
			offset := step * weight / norm
			plus[component] += offset
			minus[component] -= offset

			if min(plus[component], minus[component]) < 0 || max(plus[component], minus[component]) > 1 {
				bounded = true
			}
		}

		// Checked before evaluating rather than after, so a bounded step costs
		// nothing: on a sixteen-take joint fit two evaluations are half a minute.
		if bounded {
			sample.Note = "the stencil would cross a [0,1] bound and be clamped"
			row = append(row, sample)

			continue
		}

		high, low := counted.at(plus), counted.at(minus)
		if !isUsable(high) || !isUsable(low) {
			sample.Note = "a stencil point was not a drum"
			row = append(row, sample)

			continue
		}

		curvature := (high - 2*cost + low) / (step * step)
		sample.Curvature = &curvature
		row = append(row, sample)
	}

	return row, true
}

// sweepSteps measures every component's diagonal second difference at every h.
//
// The diagonal alone, because it is the cheapest thing that shows the plateau
// and because the off-diagonals inherit whichever h it justifies. 2 evaluations
// per (component, h) pair — which on the sixteen-take series is a good half of
// the whole run, so each component's row is printed to stderr as it finishes:
// an interrupt an hour in should not cost the evidence gathered so far.
func sweepSteps(
	counted *counter,
	position []float64,
	scope []int,
	labels []string,
	cost float64,
	stderr io.Writer,
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

				switch {
				case !ok:
					sample.Note = "a stencil point was not a drum, at h and at h/3"
				case value == 0:
					curvature := value
					sample.Curvature = &curvature
					sample.Note = "the cost did not move at all: at or inside Map's ±0.2 % " +
						"default detent, or below the render's own resolution"
				default:
					curvature := value
					sample.Curvature = &curvature
				}
			}

			sweep.Samples = append(sweep.Samples, sample)
		}

		sweep.PlateauFrom, sweep.PlateauTo, sweep.Available, sweep.Note = findPlateau(sweep.Samples)
		sweeps[slot] = sweep

		_, _ = fmt.Fprintf(stderr, "  sweep %-8s %s | %s\n",
			sweep.Label, formatSamples(sweep.Samples), sweep.Note)
	}

	return sweeps
}

// formatSamples is one sweep row, for the progress line and the summary table.
func formatSamples(samples []StepSample) string {
	parts := make([]string, 0, len(samples))

	for _, sample := range samples {
		if sample.Curvature == nil {
			parts = append(parts, fmt.Sprintf("%12s", "-"))

			continue
		}

		parts = append(parts, fmt.Sprintf("%12.4g", *sample.Curvature))
	}

	return strings.Join(parts, "")
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
			minimumPlateauLength, 100*plateauTolerance,
		)
	}

	return samples[bestStart].Step, samples[bestStart+best-1].Step, true,
		fmt.Sprintf("%d consecutive steps agree to %.0f%%", best, 100*plateauTolerance)
}

// agree is the plateau test for one neighbouring pair.
//
// An exactly-zero second difference never agrees with anything, including
// another zero. It is not a small curvature, it is the objective not having
// moved at all — every stencil point returned a bit-identical cost — and there
// are two ways that happens here, neither of which is a measurement:
// drum.ParamSpec.Map returns Shipped verbatim within half a persistence byte of
// Default (±0.2 % normalized, the detent the search's own multi-start comment
// names), so a component sitting on its default is genuinely constant over the
// first ~2e-3 of any step; and below that the render can be bit-identical anyway.
// Admitting a run of zeros as a plateau would pick a step inside the detent and
// report a flat spectrum for every parameter, which is the most confident wrong
// answer this tool could give.
func agree(left, right float64) bool {
	if left == 0 || right == 0 {
		return false
	}

	scale := max(math.Abs(left), math.Abs(right))

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
				bound.PinnedAt, bound.Normalized, bound.Step,
			),
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
		for column := range matrix[row] {
			// keep[row] is re-read inside the loop rather than only at the top:
			// once a row is dropped its remaining nulls say nothing about any
			// other component, and letting them cascade would take the whole
			// matrix out over one bad parameter.
			if !keep[row] {
				break
			}

			if !keep[column] || matrix[row][column] != nil {
				continue
			}

			reason := fmt.Sprintf("the (%s, %s) stencil could not be evaluated",
				labels[row], labels[column])

			dropped = append(dropped, DroppedComponent{Label: labels[row], Reason: reason})
			keep[row] = false

			if column != row {
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

// predictedDirections are the two things PLAN.md N6 says this measurement must
// be checked against, with the AXIS-pinned control that makes the first one a
// test rather than an assertion. Both were corrected against the code before the
// tool was written and neither is what the item originally claimed.
//
// The rotation is (1,1,2)/sqrt(6) and not (1,1,1)/sqrt(3). Every angle in the
// model enters only as Strike.AngleRad - PrincipalAxisAngleRad or
// Pickup.AngleRad - PrincipalAxisAngleRad (three sites in
// internal/physical/modes.go, and no others) and AXIS is a free fit parameter,
// so rotating the three together is an exact symmetry — but in *normalized*
// coordinates the axis moves twice as fast per unit, because AXIS spans +/-90
// degrees where HIT.A and MIC.A span +/-180. A tool testing (1,1,1) would report
// a failed prediction that was its own arithmetic.
var predictedDirections = []struct {
	key     string
	name    string
	weights map[string]float64
}{
	{
		"ROT", "common rotation of HIT.A, MIC.A and AXIS — an exact symmetry of the model",
		map[string]float64{"HIT.A": 1, "MIC.A": 1, "AXIS": 2},
	},
	{
		"ROT.PIN", "the same rotation with AXIS held — broken only through the 0.4 % split, so soft, not flat",
		map[string]float64{"HIT.A": 1, "MIC.A": 1},
	},
	{
		"R.SWAP", "exchange of HIT.R and MIC.R — a discrete near-symmetry, so soft and never flat",
		map[string]float64{"HIT.R": 1, "MIC.R": -1},
	},
	{
		"R.BOTH", "HIT.R and MIC.R moved together — the stiff direction of the same pair",
		map[string]float64{"HIT.R": 1, "MIC.R": 1},
	},
}

// scorePredictions turns the swept direction rows into the report's predictions
// block. No matrix is involved: this is what survives an objective too rough to
// differentiate coordinate-wise, which is what the shipped reference set is.
func scorePredictions(samples map[string][]StepSample) Predictions {
	probes := make(map[string]DirectionProbe, len(predictedDirections))

	for _, predicted := range predictedDirections {
		probe := DirectionProbe{Name: predicted.name}

		row, ok := samples[predicted.key]
		if !ok {
			probe.Reason = fmt.Sprintf("the scope does not carry all of %s",
				strings.Join(sortedKeys(predicted.weights), ", "))
			probes[predicted.key] = probe

			continue
		}

		probe.Samples = row
		probe.Direction = unitDirection(predicted.weights)
		probe.Curvature, probe.CurvatureStep, probe.Available = coarsest(row)

		if !probe.Available {
			probe.Reason = "no step in the sweep produced a usable stencil along this direction"
		}

		probes[predicted.key] = probe
	}

	predictions := Predictions{
		AngleRotation:           probes["ROT"],
		AngleRotationAxisPinned: probes["ROT.PIN"],
		RadiusPair:              []DirectionProbe{probes["R.SWAP"], probes["R.BOTH"]},
	}

	judge(&predictions)

	return predictions
}

// coarsest picks the sample a probe is quoted at: the largest step that produced
// a usable stencil.
//
// The largest rather than the smallest, and it is the one place this file
// prefers a coarse step. A direction probe is not estimating a derivative — the
// prediction is that an exactly invariant direction reads at the tool's floor at
// every scale — and the coarse end is where the objective's staircase contributes
// least, so it is where a genuinely non-flat direction is least likely to be
// mistaken for a flat one. The whole row is reported beside it, so a reader who
// disagrees can take another.
func coarsest(row []StepSample) (float64, float64, bool) {
	for index := len(row) - 1; index >= 0; index-- {
		if row[index].Curvature != nil {
			return *row[index].Curvature, row[index].Step, true
		}
	}

	return 0, 0, false
}

// unitDirection is a labelled unit vector, for the report.
func unitDirection(weights map[string]float64) []VectorComponent {
	norm := 0.0
	for _, weight := range weights {
		norm += weight * weight
	}

	norm = math.Sqrt(norm)

	components := make([]VectorComponent, 0, len(weights))
	for _, label := range sortedKeys(weights) {
		components = append(components, VectorComponent{Label: label, Weight: weights[label] / norm})
	}

	return components
}

// relateToSpectrum adds the quantities that only exist when a Hessian was taken:
// each direction's Rayleigh quotient off the assembled matrix and its overlap
// with the eigenvectors.
//
// These are corroboration and not the headline. An overlap is only meaningful
// when the eigenvalue it belongs to is isolated — two near-degenerate soft
// directions rotate freely into each other and their individual eigenvectors
// carry no information — while the curvature along the direction is defined
// whatever the eigenvectors do. On a piecewise objective the corroboration is
// weak: the coordinate stencils carry the jumps and their sum does not cancel
// the way the function itself does, so dTHd along an exact symmetry comes back
// hundreds where the direct stencil comes back at 1e-4. Both are printed, and
// the gap between them is a measurement of how far from quadratic this objective
// is at the step it was taken at.
func relateToSpectrum(
	predictions *Predictions,
	reduced [][]float64,
	values []float64,
	vectors [][]float64,
	labels []string,
	step float64,
) {
	largest := 0.0
	for _, value := range values {
		largest = max(largest, math.Abs(value))
	}

	for index, predicted := range predictedDirections {
		probe := pick(predictions, index)
		if probe == nil || !probe.Available {
			continue
		}

		direction := make([]float64, len(labels))
		found := 0

		for slot, label := range labels {
			if weight, ok := predicted.weights[label]; ok {
				direction[slot] = weight
				found++
			}
		}

		if found != len(predicted.weights) {
			continue
		}

		norm := math.Sqrt(dot(direction, direction))
		for slot := range direction {
			direction[slot] /= norm
		}

		probe.RayleighQuotient = rayleigh(reduced, direction)
		probe.RayleighStep = step
		probe.OverlapWithSmallest = math.Abs(dot(direction, vectors[0]))

		for slot, vector := range vectors {
			if overlap := math.Abs(dot(direction, vector)); overlap > probe.BestOverlap {
				probe.BestOverlap = overlap
				probe.BestOverlapIndex = slot
				probe.BestOverlapEigenvalue = values[slot]
			}
		}

		if largest > 0 {
			probe.RelativeToLargest = math.Abs(probe.Curvature) / largest
			probe.DecadesBelowLargest = -math.Log10(probe.RelativeToLargest)
		}
	}
}

// pick addresses one probe of the predictions block by its position in
// predictedDirections, so the two lists cannot drift apart silently.
func pick(predictions *Predictions, index int) *DirectionProbe {
	switch index {
	case 0:
		return &predictions.AngleRotation
	case 1:
		return &predictions.AngleRotationAxisPinned
	case 2, 3:
		if slot := index - 2; slot < len(predictions.RadiusPair) {
			return &predictions.RadiusPair[slot]
		}
	}

	return nil
}

// symmetryContrast is how much flatter the free rotation has to be than the
// AXIS-pinned one before the symmetry counts as measured.
//
// A hundredfold, and the contrast rather than an absolute threshold is the test
// on purpose. The rotation's true curvature is exactly zero, so what comes back
// is a floor rather than a measurement: the three angles reach the render
// through two floating-point subtractions and the objective on top of that is
// piecewise in bins, peaks and admissibility. Any absolute cutoff would be a
// claim about the tool's own noise, which changes with h. The contrast is
// scale-free and is exactly the two-condition test PLAN.md N6 asks for: flat
// with AXIS free, soft with AXIS pinned. A tool reporting zero for both has
// measured nothing, and this ratio is what catches that.
//
// A hundred is conservative by two decades. On the synthetic probe the measured
// contrast runs 9.6e4 (h = 3e-4) to 3.9e7 (h = 1e-2), so nothing near the
// threshold has ever been seen; it is set where it is so that a genuine
// weakening of the symmetry would be caught rather than absorbed.
const symmetryContrast = 100

// judge writes the verdicts. It is done here rather than inside scorePredictions
// because no statement about any of these directions is trustworthy on its own:
// the exact symmetry is read against the AXIS-pinned control, and the radius pair
// is read against the floor the exact symmetry establishes. The floor is a
// property of the tool at a given step, so every comparison below is made at the
// same step and never across two.
func judge(predictions *Predictions) {
	rotation, pinned := &predictions.AngleRotation, &predictions.AngleRotationAxisPinned

	if !rotation.Available || !pinned.Available {
		return
	}

	low, high, steps := contrastRange(rotation.Samples, pinned.Samples)

	switch {
	case steps == 0:
		rotation.Verdict = "unavailable: no step produced both this direction and its AXIS-pinned control"
	case low >= symmetryContrast:
		rotation.Verdict = fmt.Sprintf(
			"borne out: %.3g to %.3g times flatter than the same rotation with AXIS held, "+
				"at every one of the %d steps both were measured at",
			low, high, steps,
		)
	default:
		rotation.Verdict = fmt.Sprintf(
			"REFUTED: only %.3g times flatter than the AXIS-pinned rotation at its worst step "+
				"(best %.3g over %d steps), where an exact symmetry should sit at the tool's own floor",
			low, high, steps,
		)
	}

	pinned.Verdict = againstFloor(pinned.Samples, rotation.Samples,
		"the split-broken rotation",
		"AXIS would then do nothing at this drum's asymmetry",
		"How far above the floor is not a measure of how weakly the split breaks it: "+
			"the number is set by the objective's staircase, not by the 0.4 %.")

	for index := range predictions.RadiusPair {
		probe := &predictions.RadiusPair[index]
		if !probe.Available {
			continue
		}

		probe.Verdict = againstFloor(probe.Samples, rotation.Samples, "this direction",
			"the exchange argument does not predict a flat direction — it is discrete, "+
				"and a discrete symmetry produces no zero eigenvalue even when it is exact",
			"Whether it is also *softer* than its partner direction is a separate question, "+
				"and not one this answers: compare the two radius rows against each other.")
	}
}

// contrastRange is the smallest and largest ratio between two directions'
// curvatures over the steps both were measured at, and how many steps that was.
func contrastRange(numerator, denominator []StepSample) (low, high float64, steps int) {
	low, high = math.Inf(1), 0

	for index := range numerator {
		if numerator[index].Curvature == nil || denominator[index].Curvature == nil {
			continue
		}

		if *numerator[index].Curvature == 0 {
			continue
		}

		ratio := math.Abs(*denominator[index].Curvature / *numerator[index].Curvature)
		low, high = math.Min(low, ratio), math.Max(high, ratio)
		steps++
	}

	if steps == 0 {
		return 0, 0, 0
	}

	return low, high, steps
}

// againstFloor reads a direction predicted to be soft rather than flat, against
// the floor an exactly flat direction reaches at the same steps.
//
// The floor is measured rather than assumed, which is what makes this a test at
// all. The common angle rotation is an exact symmetry of the model, so whatever
// it returns is what this tool reports for zero — and a direction is flat only if
// it is down at that level.
func againstFloor(samples, floor []StepSample, subject, flatMeans, caveat string) string {
	low, high, steps := contrastRange(floor, samples)
	if steps == 0 {
		return "unavailable: no step measured both this direction and the exact symmetry"
	}

	// contrastRange(floor, samples) is |samples| / |floor|, so a large number is
	// far above the floor, which is what "not flat" means here.
	if low < symmetryContrast {
		return fmt.Sprintf(
			"REFUTED: %s comes within %.3g of the floor an exact symmetry reaches (over %d steps) — %s",
			subject, low, steps, flatMeans,
		)
	}

	// "Not flat" and nothing more. This says the direction is not a symmetry; it
	// does not say the direction is soft, because a ratio to the floor is not a
	// measure of curvature — the numerator is the objective's staircase.
	return fmt.Sprintf(
		"borne out: %s stands %.3g to %.3g times above the floor an exact symmetry reaches, "+
			"over %d steps — not flat. %s",
		subject, low, high, steps, caveat,
	)
}

func sortedKeys(weights map[string]float64) []string {
	keys := make([]string, 0, len(weights))
	for key := range weights {
		keys = append(keys, key)
	}

	slices.Sort(keys)

	return keys
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
