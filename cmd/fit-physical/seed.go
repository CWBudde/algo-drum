package main

import (
	"math"
	"math/rand"
	"slices"
	"sort"

	"github.com/cwbudde/algo-drum/internal/drum"
	"github.com/cwbudde/algo-drum/internal/physical"
	"github.com/cwbudde/algo-drum/internal/physical/match"
)

// Seeding a restart from the reference's own partial frequencies.
//
// A drum's mode frequencies are analytic: physical.GenerateDrumModes reads them off
// the tension, radius and cavity without rendering a sample or taking a
// transform. That costs about a hundredth of a full evaluation, which makes it
// worth spending thousands of them deciding where a restart should begin.
//
// It is worth doing because the search demonstrably does not find the frequency
// agreement the model can reach. Twenty thousand random banks contain one within
// 11.5 cents of the reference, audibility weighted, and a few hundred
// hill-climbing steps take that to 10.5 — against a fit that settled at 59.7 and
// a gate at 25. The modes are reachable; the search was not reaching them.
//
// What this is not: it is not a term, a constraint or a filter. A seeded restart
// starts somewhere better and is then judged by the same nine terms as every
// other, so a seed that suits the frequencies and ruins the decay loses on its
// own merits. Frequency-only agreement is necessary and nowhere near sufficient
// — a mode at the right frequency may be inaudible, or ring for far too long —
// which is exactly why this may not appear anywhere except in the starting point.

// modeFrequencyError is the audibility-weighted RMS distance, in cents, from
// each reference partial to the nearest mode the bank produces.
//
// Audibility weighting rather than a plain mean, and for the same reason the
// distance itself weights that way: the reference's partials span 41 dB, and a
// component one dB above the detection floor should not be allowed to outvote
// the one carrying the drum. Unmatched modes cost nothing here — this asks only
// whether the reference's partials are reachable, and the spurious term in the
// real distance is what argues the other direction.
func modeFrequencyError(bank []float64, reference match.Features, floorDB, sampleRateHz float64) float64 {
	config, err := drum.PhysicalTomConfig(bank, drum.NeutralDecayAmount, sampleRateHz)
	if err != nil {
		return math.Inf(1)
	}

	modes, err := physical.GenerateDrumModes(config)
	if err != nil || len(modes) == 0 {
		return math.Inf(1)
	}

	var sum, weight float64

	for _, partial := range reference.Partials {
		if partial.FrequencyHz <= 0 {
			continue
		}

		audibility := max(0, partial.LevelDB-floorDB)
		if audibility <= 0 {
			continue
		}

		nearest := math.Inf(1)

		for _, mode := range modes {
			if mode.FrequencyHz <= 0 {
				continue
			}

			if cents := math.Abs(1200 * math.Log2(mode.FrequencyHz/partial.FrequencyHz)); cents < nearest {
				nearest = cents
			}
		}

		if math.IsInf(nearest, 1) {
			continue
		}

		sum += audibility * nearest * nearest
		weight += audibility
	}

	if weight <= 0 {
		return math.Inf(1)
	}

	return math.Sqrt(sum / weight)
}

// seedCandidate is one starting point and the frequency error that earned it.
type seedCandidate struct {
	position   []float64
	errorCents float64
}

// seedBudget is how much analytic work the pre-solve may do: samples random
// banks, then hill-climbs the best keep of them.
//
// A parameter rather than a constant so the tests can run it small. The default
// is twenty thousand samples, which costs less than a minute against a run that
// spends the better part of an hour — but a test suite is not a run, and a gate
// nobody waits for is a gate nobody keeps.
type seedBudget struct {
	samples int
	keep    int
}

// defaultSeedBudget is what the command uses.
var defaultSeedBudget = seedBudget{samples: 20000, keep: 24}

// seedSeparation is how far apart two seeds must be, per free dimension, to
// count as different starting points. Seeds any closer describe the same drum,
// and eight restarts that all begin in one basin are one restart.
const seedSeparation = 0.15

// frequencySeeds pre-solves for count diverse starting positions, best first.
//
// Random sampling then hill-climbing, rather than anything cleverer, because the
// objective is cheap and the answer only has to be a good starting point. The
// climb runs on the free dimensions alone; fixed parameters keep whatever -fix
// gave them, exactly as the search does.
func frequencySeeds(
	reference match.Features,
	bank []float64,
	free []int,
	count int,
	floorDB, sampleRateHz float64,
	rng *rand.Rand,
	budget seedBudget,
) []seedCandidate {
	if count <= 0 || len(free) == 0 || budget.samples <= 0 || budget.keep <= 0 {
		return nil
	}

	trial := slices.Clone(bank)

	score := func(position []float64) float64 {
		for i, index := range free {
			trial[index] = position[i]
		}

		return modeFrequencyError(trial, reference, floorDB, sampleRateHz)
	}

	found := make([]seedCandidate, 0, budget.samples/64)

	for range budget.samples {
		position := make([]float64, len(free))
		for i := range position {
			position[i] = rng.Float64()
		}

		if cost := score(position); !math.IsInf(cost, 1) {
			found = append(found, seedCandidate{position: position, errorCents: cost})
		}
	}

	sort.Slice(found, func(a, b int) bool { return found[a].errorCents < found[b].errorCents })

	if len(found) > budget.keep {
		found = found[:budget.keep]
	}

	for index := range found {
		found[index] = climb(found[index], score)
	}

	sort.Slice(found, func(a, b int) bool { return found[a].errorCents < found[b].errorCents })

	return diverse(found, count)
}

// climb is a coordinate descent with a shrinking step. It is deliberately plain:
// the objective costs microseconds, and a local optimum reached quickly is worth
// more here than a global one reached slowly, since eight seeds start in eight
// different basins anyway.
func climb(seed seedCandidate, score func([]float64) float64) seedCandidate {
	current := slices.Clone(seed.position)
	best := seed.errorCents

	for step := 0.2; step > 1e-4; step *= 0.6 {
		for improved := true; improved; {
			improved = false

			for index := range current {
				for _, delta := range [2]float64{step, -step} {
					moved := clamp01(current[index] + delta)
					if moved == current[index] {
						continue
					}

					previous := current[index]
					current[index] = moved

					if cost := score(current); cost < best {
						best, improved = cost, true
					} else {
						current[index] = previous
					}
				}
			}
		}
	}

	return seedCandidate{position: current, errorCents: best}
}

// diverse picks the best count seeds that are not minor variations of each
// other, then falls back to the plain ranking if separation leaves too few.
func diverse(ranked []seedCandidate, count int) []seedCandidate {
	picked := make([]seedCandidate, 0, count)

	for _, candidate := range ranked {
		if len(picked) == count {
			break
		}

		apart := true

		for _, chosen := range picked {
			if distance(candidate.position, chosen.position) < seedSeparation {
				apart = false

				break
			}
		}

		if apart {
			picked = append(picked, candidate)
		}
	}

	for _, candidate := range ranked {
		if len(picked) == count {
			break
		}

		if !slices.ContainsFunc(picked, func(chosen seedCandidate) bool {
			return slices.Equal(chosen.position, candidate.position)
		}) {
			picked = append(picked, candidate)
		}
	}

	return picked
}

// distance is the largest per-dimension gap, which is the shape of the box a
// seeded restart searches — so two seeds are "apart" exactly when neither sits
// inside the other's box.
func distance(left, right []float64) float64 {
	worst := 0.0
	for index := range left {
		worst = max(worst, math.Abs(left[index]-right[index]))
	}

	return worst
}

// frequencyRelevant reports, per free dimension, whether the pre-solve has any
// opinion about it at all — measured, by moving each one and watching whether
// the mode frequencies follow.
//
// This is the correction to a first version that boxed every dimension, and the
// measurement that forced it is worth keeping: seeded restarts came back at
// 31.7 and 19.1 against unseeded ones at 14.9 and 16.6, and widening the box did
// not rescue them. The pre-solve scores mode frequencies. Damping, the
// microphone position, the attack layer and the strike hardness do not move a
// mode frequency by so much as a cent, so a seed's value for them is not
// evidence — it is whatever the random draw happened to leave there. Confining a
// restart to a neighbourhood of *those* is how a seed that was right about
// frequency ended up worse overall than no seed at all.
//
// Probed rather than listed, so it stays true when the parameter table changes.
func frequencyRelevant(
	reference match.Features,
	bank []float64,
	free []int,
	floorDB, sampleRateHz float64,
	rng *rand.Rand,
) []bool {
	relevant := make([]bool, len(free))
	trial := slices.Clone(bank)

	score := func(position []float64) float64 {
		for i, index := range free {
			trial[index] = position[i]
		}

		return modeFrequencyError(trial, reference, floorDB, sampleRateHz)
	}

	// A few independent bases, because one dimension can be inert at a point
	// where another has collapsed the mode set.
	for range 8 {
		base := make([]float64, len(free))
		for i := range base {
			base[i] = 0.25 + 0.5*rng.Float64()
		}

		reference := score(base)
		if math.IsInf(reference, 1) {
			continue
		}

		for index := range base {
			if relevant[index] {
				continue
			}

			for _, probe := range [2]float64{0.05, 0.95} {
				previous := base[index]
				base[index] = probe
				moved := score(base)
				base[index] = previous

				// Any real movement counts. The question is whether the seed
				// knows anything about this dimension, not how much.
				if math.IsInf(moved, 1) || math.Abs(moved-reference) > 1e-6 {
					relevant[index] = true

					break
				}
			}
		}
	}

	return relevant
}

// boxAround maps mayfly's [0,1] cube onto a box of the given half-width around
// the seed, in the dimensions the seed is evidence about and no others.
//
// Everything else — the strike velocity in the trailing dimension, and every
// free parameter frequencyRelevant found inert — passes through untouched, so
// the restart still searches those over their whole range. A seed is a claim
// about where the modes are. It is not a claim about how hard the drum was hit,
// how long it rings or where the microphone was, and a box that pretended
// otherwise is what made the first version of this lose to no seeding at all.
//
// The warp is applied at the mayfly boundary and nowhere else, so every position
// that reaches the objective, the progress reporter, the checkpoint or the
// report is in ordinary bank coordinates. A stored position therefore means the
// same thing whether the restart that found it was seeded or not.
func boxAround(seed []float64, relevant []bool, halfWidth float64) func([]float64) []float64 {
	if seed == nil || halfWidth <= 0 || halfWidth >= 0.5 {
		return func(position []float64) []float64 { return position }
	}

	return func(position []float64) []float64 {
		warped := slices.Clone(position)

		for index := range seed {
			if relevant != nil && !relevant[index] {
				continue
			}

			warped[index] = clamp01(seed[index] + (position[index]-0.5)*2*halfWidth)
		}

		return warped
	}
}
