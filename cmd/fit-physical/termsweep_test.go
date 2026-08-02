package main

import (
	"math"
	"math/rand"
	"testing"

	"github.com/cwbudde/algo-drum/internal/physical/match"
)

// TestTermsIsExactlyCostDecomposed is the load-bearing assertion under the whole
// per-term sweep.
//
// The claim the sweep makes is that it is the scalar sweep *decomposed* — the
// same evaluations, read nine more ways — rather than a second measurement of the
// same objective taken beside it. That claim is only true if the total the
// decomposed path produces is the same float64 the search's own cost produces,
// and "the same" has to mean bit-identical: a difference in the last place, fed
// through a second difference divided by h² at h = 1e-4, is amplified by 1e8 and
// would appear as curvature.
//
// Equality rather than a tolerance, therefore, and it holds by construction:
// evaluator.terms accumulates through meanTerms, which sums the same per-take
// totals in the same order cost does, and divides by the same count.
func TestTermsIsExactlyCostDecomposed(t *testing.T) {
	t.Parallel()

	probe := benchEvaluator(t, 0.6)
	rng := rand.New(rand.NewSource(7))

	for trial := range 8 {
		position := make([]float64, probe.dimensions())
		for index := range position {
			position[index] = rng.Float64()
		}

		scalar := probe.cost(position)

		terms, ok := probe.terms(position)
		if !ok {
			// cost's own refusal must agree, or the two paths disagree about what
			// a drum is, which would be a worse defect than a mismatched total.
			if isUsable(scalar) {
				t.Errorf("trial %d: terms refused a position cost scored at %v", trial, scalar)
			}

			continue
		}

		if !isUsable(scalar) {
			t.Errorf("trial %d: cost refused a position terms scored at %v", trial, terms.Total)

			continue
		}

		if terms.Total != scalar {
			t.Errorf("trial %d: terms.Total %.17g is not cost %.17g (differ by %g)",
				trial, terms.Total, scalar, terms.Total-scalar)
		}
	}
}

// TestTermVectorCarriesTheGateRatiosAndTheRawTotal pins the two conventions the
// sweep's columns are read under.
//
// The nine are gate ratios because that is the only unit they are comparable in.
// The total is match.Terms's own, copied rather than re-summed from the nine —
// the nine do sum to it, weight being 1/gate, but in a different order, so a
// re-summed total would differ in its last bits from the one the scalar sweep
// differences and the decomposition claim above would quietly stop holding.
func TestTermVectorCarriesTheGateRatiosAndTheRawTotal(t *testing.T) {
	t.Parallel()

	terms := match.Terms{
		PartialFrequency: 40, PartialLevel: 3, PartialDecay: 0.25,
		SpectralEnvelope: 6, Envelope: 2, Glide: 15,
		AttackBalance: 4, Unmatched: 0.1, Spurious: 0.2,
	}

	// The total stated the way match.Distance states it — weights rather than
	// gates — so that the sum check below compares two independent routes to the
	// same number instead of the gates against themselves.
	weights := match.DefaultWeights()
	terms.Total = terms.PartialFrequency*weights.PartialFrequency +
		terms.PartialLevel*weights.PartialLevel +
		terms.PartialDecay*weights.PartialDecay +
		terms.SpectralEnvelope*weights.SpectralEnvelope +
		terms.Envelope*weights.Envelope +
		terms.Glide*weights.Glide +
		terms.AttackBalance*weights.AttackBalance +
		terms.Unmatched*weights.Unmatched +
		terms.Spurious*weights.Spurious

	vector := newTermVector(terms)
	fields := termFields(terms)

	if len(fields) != termCount {
		t.Fatalf("termFields returned %d fields, but a termVector has room for %d",
			len(fields), termCount)
	}

	for slot, field := range fields {
		if vector[slot] != field.Ratio() {
			t.Errorf("slot %d (%s): %v, want the gate ratio %v",
				slot, field.Name, vector[slot], field.Ratio())
		}
	}

	if vector[termTotalSlot] != terms.Total {
		t.Errorf("total slot is %v, want match.Terms.Total %v exactly",
			vector[termTotalSlot], terms.Total)
	}

	// Not asserted equal, asserted *close*: the sum of the ratios is the total
	// again by construction, and the gap between the two is exactly the
	// summation-order difference this convention exists to keep out of the total.
	summed := 0.0
	for slot := range termCount {
		summed += vector[slot]
	}

	if relative := math.Abs(summed-terms.Total) / terms.Total; relative > 1e-12 {
		t.Errorf("the nine gate ratios sum to %v against a total of %v (relative %g), "+
			"which is too far apart to be summation order — the objective has stopped "+
			"being a plain weighted sum",
			summed, terms.Total, relative)
	}
}

// TestTermSweepTotalReproducesTheScalarStencil checks the two stencils against
// each other on one objective.
//
// vectorStencil and stencil are separate implementations, and the per-term sweep
// is only a decomposition of the scalar one if they agree exactly on the column
// they share. Run over the whole swept grid, because the thing that would break
// this is an operand order that differs only where the rounding does.
func TestTermSweepTotalReproducesTheScalarStencil(t *testing.T) {
	t.Parallel()

	// A deliberately awkward objective: smooth, but with terms of very different
	// magnitude, so that cancellation in the second difference is doing real work.
	objective := func(position []float64) float64 {
		return 1e6*position[0]*position[0] - 3.25*position[0]*position[1] +
			1e-3*math.Exp(position[1]) + 7
	}

	position := []float64{0.4375, 0.5625}

	scalar := &counter{cost: objective}
	decomposed := &counter{
		cost: objective,
		terms: func(probe []float64) (match.Terms, bool) {
			return match.Terms{Total: objective(probe)}, true
		},
	}

	origin, ok := decomposed.vectorAt(position)
	if !ok {
		t.Fatal("the base point did not evaluate")
	}

	base := scalar.at(position)
	if base != origin[termTotalSlot] {
		t.Fatalf("the two paths disagree at the base point: %.17g against %.17g",
			base, origin[termTotalSlot])
	}

	for _, step := range hessianSteps {
		want, okScalar := secondDifference(scalar, position, base, 0, 0, step)

		got, okVector := vectorSecondDifference(decomposed, position, origin, 0, step)
		if okScalar != okVector {
			t.Fatalf("h = %g: scalar ok = %v, vector ok = %v", step, okScalar, okVector)
		}

		if got[termTotalSlot] != want {
			t.Errorf("h = %g: vector total %.17g is not the scalar %.17g", step, got[termTotalSlot], want)
		}
	}
}

// TestTermSweepReportsTheTotalWhenTheTermsAreUnavailable pins the fallback.
//
// Every unit test in this package differentiates a synthetic scalar objective
// with no terms behind it, and so does any future one. The sweep must then behave
// exactly as it did before the decomposition existed: one column, no per-term
// block, and no nine copies of the total wearing term labels — which would be the
// most plausible-looking way for this to report nothing at all.
func TestTermSweepReportsTheTotalWhenTheTermsAreUnavailable(t *testing.T) {
	t.Parallel()

	counted := &counter{cost: func(position []float64) float64 {
		return position[0] * position[0]
	}}

	if counted.decomposed() {
		t.Fatal("a counter with no terms function reported itself decomposed")
	}

	sweeps, terms := sweepSteps(counted, []float64{0.5}, []int{0}, []string{"X"},
		termVector{termTotalSlot: 0.25}, testWriter{})

	if len(terms) != 0 {
		t.Errorf("got %d per-term sweeps from an objective that has no terms", len(terms))
	}

	if len(sweeps) != 1 {
		t.Fatalf("got %d scalar sweeps, want 1", len(sweeps))
	}

	if sweeps[0].Label != "X" {
		t.Errorf("scalar sweep is labelled %q, want the component name", sweeps[0].Label)
	}

	// x² has a second difference of exactly 2 at every step, so this doubles as a
	// check that the fallback path still computes the thing it always did.
	for _, sample := range sweeps[0].Samples {
		if sample.Curvature == nil {
			t.Errorf("h = %g: no curvature", sample.Step)

			continue
		}

		if math.Abs(*sample.Curvature-2) > 1e-6 {
			t.Errorf("h = %g: curvature %v, want 2", sample.Step, *sample.Curvature)
		}
	}
}

// testWriter discards a sweep's progress lines.
type testWriter struct{}

func (testWriter) Write(p []byte) (int, error) { return len(p), nil }
