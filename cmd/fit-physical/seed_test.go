package main

import (
	"math"
	"math/rand"
	"testing"

	"github.com/cwbudde/algo-drum/internal/drum"
	"github.com/cwbudde/algo-drum/internal/physical/match"
)

// tomLikePartials is a reference the model can plausibly reach: a fundamental
// and five higher components at membrane-ish ratios, with levels spanning the
// detection range so the audibility weighting has something to do.
func tomLikePartials() match.Features {
	return match.Features{Partials: []match.Partial{
		{FrequencyHz: 118, LevelDB: -25, T60Seconds: 1.0},
		{FrequencyHz: 213, LevelDB: 0, T60Seconds: 0.15},
		{FrequencyHz: 259, LevelDB: -30, T60Seconds: 0.26},
		{FrequencyHz: 297, LevelDB: -41, T60Seconds: 0.55},
		{FrequencyHz: 380, LevelDB: -38, T60Seconds: 0.47},
		{FrequencyHz: 503, LevelDB: -28, T60Seconds: 0.26},
	}}
}

func freeIndices(t *testing.T) ([]float64, []int) {
	t.Helper()

	specs := drum.PhysicalTomSpecs()

	bank := make([]float64, len(specs))
	free := make([]int, 0, len(specs))

	for index, spec := range specs {
		bank[index] = spec.Default

		// QUAL is pinned in every real run — it buys mode count with CPU, so a
		// free one would let the pre-solve wander into High and make this test
		// slower than the thing it is testing.
		if spec.ID != "physicalTom.quality" {
			free = append(free, index)
		}
	}

	return bank, free
}

// TestSeedsBeatTheBankTheyStartFrom is the whole justification for the
// pre-solve. If a few thousand analytic trials cannot place the modes closer to
// the reference than the shipped defaults do, seeding buys nothing and should
// not exist.
func TestSeedsBeatTheBankTheyStartFrom(t *testing.T) {
	t.Parallel()

	reference := tomLikePartials()
	bank, free := freeIndices(t)

	shipped := modeFrequencyError(bank, reference, -42, 44100)

	seeds := frequencySeeds(reference, bank, free, 4, -42, 44100, rand.New(rand.NewSource(1)), seedBudget{samples: 250, keep: 4})
	if len(seeds) != 4 {
		t.Fatalf("frequencySeeds returned %d seeds, want 4", len(seeds))
	}

	if seeds[0].errorCents >= shipped {
		t.Errorf("best seed %.1f cents is no better than the shipped bank's %.1f",
			seeds[0].errorCents, shipped)
	}

	// Best first, so a caller reporting seeds[0] is reporting the best one.
	for index := 1; index < len(seeds); index++ {
		if seeds[index].errorCents < seeds[index-1].errorCents {
			t.Errorf("seed %d (%.1f) is better than seed %d (%.1f); seeds must be ranked",
				index, seeds[index].errorCents, index-1, seeds[index-1].errorCents)
		}
	}
}

// TestSeedsAreDistinct guards the reason there is more than one: eight restarts
// that all begin in the same basin are one restart with a larger bill.
func TestSeedsAreDistinct(t *testing.T) {
	t.Parallel()

	bank, free := freeIndices(t)

	seeds := frequencySeeds(tomLikePartials(), bank, free, 6, -42, 44100, rand.New(rand.NewSource(7)), seedBudget{samples: 300, keep: 6})
	if len(seeds) < 2 {
		t.Fatalf("frequencySeeds returned %d seeds, want at least 2", len(seeds))
	}

	for i := range seeds {
		for j := i + 1; j < len(seeds); j++ {
			if distance(seeds[i].position, seeds[j].position) == 0 {
				t.Errorf("seeds %d and %d are the same point", i, j)
			}
		}
	}
}

// TestSeedsRespectFixedParameters: the pre-solve moves the free dimensions and
// nothing else, or a seeded run would quietly search parameters that -fix said
// to hold.
func TestSeedsRespectFixedParameters(t *testing.T) {
	t.Parallel()

	bank, all := freeIndices(t)
	free := all[:4]

	seeds := frequencySeeds(tomLikePartials(), bank, free, 3, -42, 44100, rand.New(rand.NewSource(3)), seedBudget{samples: 200, keep: 3})
	if len(seeds) == 0 {
		t.Fatal("frequencySeeds returned nothing")
	}

	for _, seed := range seeds {
		if len(seed.position) != len(free) {
			t.Errorf("seed has %d dimensions, want %d", len(seed.position), len(free))
		}
	}
}

// TestBoxAroundStaysInsideItsBox is what makes the warp safe to apply at the
// mayfly boundary: whatever the optimizer proposes, the position handed to the
// objective is a normalized bank position inside the seed's neighbourhood.
func TestBoxAroundStaysInsideItsBox(t *testing.T) {
	t.Parallel()

	seed := []float64{0.5, 0.02, 0.98}
	warp := boxAround(seed, 0.25)

	for _, raw := range [][]float64{
		{0, 0, 0, 0},
		{1, 1, 1, 1},
		{0.5, 0.5, 0.5, 0.5},
		{0.13, 0.87, 0.41, 0.62},
	} {
		got := warp(raw)

		for index := range seed {
			if got[index] < 0 || got[index] > 1 {
				t.Errorf("warped %v to %v, which is outside the unit cube", raw, got)
			}

			// Clamping is what lets a seed sit near a bound, so the box may be
			// clipped — but it must never reach further than its half-width.
			if math.Abs(got[index]-seed[index]) > 0.25+1e-12 {
				t.Errorf("dimension %d moved %.4f from the seed, want at most 0.25",
					index, math.Abs(got[index]-seed[index]))
			}
		}

		// The velocity dimension is not seeded: frequencies say nothing about
		// how hard the drum was hit, so narrowing it would be pure loss.
		if got[len(seed)] != raw[len(seed)] {
			t.Errorf("velocity moved from %v to %v", raw[len(seed)], got[len(seed)])
		}
	}
}

// TestBoxAroundWithoutASeedIsIdentity keeps unseeded restarts bit-for-bit what
// they were: the whole design rests on being able to mix seeded and unseeded
// restarts and compare them.
func TestBoxAroundWithoutASeedIsIdentity(t *testing.T) {
	t.Parallel()

	position := []float64{0.1, 0.9, 0.4}

	for _, warp := range []func([]float64) []float64{
		boxAround(nil, 0.25),
		boxAround([]float64{0.5}, 0),
		boxAround([]float64{0.5}, 0.5),
	} {
		got := warp(position)
		for index := range position {
			if got[index] != position[index] {
				t.Errorf("identity warp changed %v to %v", position, got)
			}
		}
	}
}
