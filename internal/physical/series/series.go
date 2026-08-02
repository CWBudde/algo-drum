// Package series relates the measurements of one take to the measurements of
// the others.
//
// It exists because the tooling around the physical model could measure a
// signal and could not compare measurements. `match` reduces one recording to
// features, `analysis` reports on one configuration, and cmd/fit-physical
// reports on one fit — and every question worth asking of a reference series
// turned out to be a question *between* those artefacts: does this fit agree
// with that one, does this quantity trend with the take order, is this partial
// present in some takes and absent in others. Those were repeatedly answered by
// hand in a scratch script, which is how a wrong answer survives: nothing is
// pinned, nothing is reviewed, and the next question starts from zero.
//
// Nothing here is signal processing. algo-dsp covers that layer and covers it
// well; what was missing sits above it.
package series

import (
	"errors"
	"fmt"
	"math"
	"slices"
)

// ErrSeries reports a comparison that cannot be made from the values given.
var ErrSeries = errors.New("invalid series comparison")

// Indices is 0, 1, … n-1 as a series, which is what a quantity is correlated
// against when the question is whether it trends with the take order.
//
// The take order is a claim rather than a measurement — see the fitting
// discipline in AGENTS.md — so a correlation against it is evidence *about*
// that claim and never an assumption of it.
func Indices(n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = float64(i)
	}

	return out
}

// Ranks returns the fractional ranks of values, averaging ties.
//
// Ties are averaged rather than broken arbitrarily because the alternative
// makes the result depend on input order: two takes measured at exactly the
// same level would correlate differently depending on which was listed first,
// and a statistic that moves when the file list is reordered is not measuring
// the drum.
func Ranks(values []float64) []float64 {
	order := make([]int, len(values))
	for i := range order {
		order[i] = i
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

	ranks := make([]float64, len(values))

	for start := 0; start < len(order); {
		end := start + 1
		for end < len(order) && values[order[end]] == values[order[start]] {
			end++
		}

		// The mean of the positions start … end-1, which for a run of one is
		// just that position.
		shared := float64(start+end-1) / 2

		for _, index := range order[start:end] {
			ranks[index] = shared
		}

		start = end
	}

	return ranks
}

// Pearson is the linear correlation of two equal-length series.
func Pearson(a, b []float64) (float64, error) {
	if len(a) != len(b) {
		return 0, fmt.Errorf("%w: %d values against %d", ErrSeries, len(a), len(b))
	}

	if len(a) < minimumPairs {
		return 0, fmt.Errorf("%w: %d pairs, need at least %d",
			ErrSeries, len(a), minimumPairs)
	}

	meanA, meanB := mean(a), mean(b)

	var covariance, varianceA, varianceB float64

	for i := range a {
		da, db := a[i]-meanA, b[i]-meanB
		covariance += da * db
		varianceA += da * da
		varianceB += db * db
	}

	// A series with no variance has no correlation with anything, and reporting
	// 0 would be a claim of independence rather than of absence. Sixteen takes
	// that all measured the same is a fact about the takes; it is not evidence
	// that the quantity fails to trend.
	if varianceA == 0 || varianceB == 0 {
		return 0, fmt.Errorf("%w: a series is constant, so no correlation exists", ErrSeries)
	}

	return covariance / math.Sqrt(varianceA*varianceB), nil
}

// Spearman is the rank correlation of two equal-length series: Pearson over
// Ranks.
//
// Rank rather than linear correlation is the default for these comparisons
// because the questions are ordinal. "Do the loud takes come later in the file
// list" does not assert that loudness rises linearly with the index, and a
// linear coefficient would be depressed by a perfectly monotone ramp that
// happens to be curved.
func Spearman(a, b []float64) (float64, error) {
	if len(a) != len(b) {
		return 0, fmt.Errorf("%w: %d values against %d", ErrSeries, len(a), len(b))
	}

	return Pearson(Ranks(a), Ranks(b))
}

// minimumPairs is the fewest pairs a correlation is reported from. Two points
// correlate ±1 exactly, always, which is a statement about the arithmetic
// rather than about the drum.
const minimumPairs = 3

func mean(values []float64) float64 {
	var total float64
	for _, value := range values {
		total += value
	}

	return total / float64(len(values))
}
