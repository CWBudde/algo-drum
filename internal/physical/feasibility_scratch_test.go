package physical_test

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"sort"
	"testing"

	"github.com/cwbudde/algo-drum/internal/drum"
	"github.com/cwbudde/algo-drum/internal/physical"
)

// Scratch probe, not a test of anything: can the model's mode set reach the
// reference's partial frequencies at all, ignoring every other term? Run with
//
//	go test ./internal/physical -run TestModeReachScratch -v
var referenceHz = []float64{118.1, 212.8, 259.1, 296.7, 380.5, 502.6, 696.9}

// referenceDB is each partial's measured level; the distance weights a partial
// by how far it stands above the -42 dB detection floor, so an unweighted RMS
// over the seven is not the number the gate is set against.
var referenceDB = []float64{-25.2, 0, -29.8, -40.7, -38.2, -27.9, -41.0}

func audibility(index int) float64 { return math.Max(0, referenceDB[index]+42) }

func TestModeReachScratch(t *testing.T) {
	// A diagnostic, not an assertion: it prints, it takes seconds, and it has
	// no pass condition to defend. Gated so `go test ./...` stays a gate.
	if os.Getenv("PHYSICAL_PROBE") == "" {
		t.Skip("set PHYSICAL_PROBE=1 to run the mode-reach probe")
	}

	specs := drum.PhysicalTomSpecs()
	rng := rand.New(rand.NewSource(1))

	best := math.Inf(1)

	var bestPer []float64

	var bestBank []float64

	const samples = 20000

	bank := make([]float64, len(specs))

	for range samples {
		for index := range bank {
			bank[index] = rng.Float64()
		}
		// QUAL pinned to Standard, as every fit run pins it.
		for index, spec := range specs {
			if spec.ID == "physicalTom.quality" {
				bank[index] = 0.5
			}
		}

		config, err := drum.PhysicalTomConfig(bank, drum.NeutralDecayAmount, 44100)
		if err != nil {
			continue
		}

		modes, err := physical.GenerateModes(config)
		if err != nil {
			continue
		}

		freqs := make([]float64, 0, len(modes))
		for _, mode := range modes {
			if hz := mode.FrequencyHz; hz > 0 {
				freqs = append(freqs, hz)
			}
		}

		if len(freqs) == 0 {
			continue
		}

		sort.Float64s(freqs)

		per := make([]float64, len(referenceHz))
		sum, weight := 0.0, 0.0

		for i, target := range referenceHz {
			nearest := math.Inf(1)
			for _, hz := range freqs {
				if cents := math.Abs(1200 * math.Log2(hz/target)); cents < nearest {
					nearest = cents
				}
			}

			per[i] = nearest
			sum += audibility(i) * nearest * nearest
			weight += audibility(i)
		}

		if rms := math.Sqrt(sum / weight); rms < best {
			best, bestPer, bestBank = rms, per, append([]float64(nil), bank...)
		}
	}

	fmt.Printf("best audibility-weighted frequency error over %d random banks: %.1f cents\n", samples, best)

	for i, target := range referenceHz {
		fmt.Printf("  %7.1f Hz  nearest mode %8.1f cents\n", target, bestPer[i])
	}

	// Hill-climb from the best random bank, frequency only, to find the floor.
	score := func(candidate []float64) (float64, []float64) {
		config, err := drum.PhysicalTomConfig(candidate, drum.NeutralDecayAmount, 44100)
		if err != nil {
			return math.Inf(1), nil
		}

		modes, err := physical.GenerateModes(config)
		if err != nil {
			return math.Inf(1), nil
		}

		per := make([]float64, len(referenceHz))
		sum, weight := 0.0, 0.0

		for i, target := range referenceHz {
			nearest := math.Inf(1)

			for _, mode := range modes {
				if mode.FrequencyHz <= 0 {
					continue
				}

				if cents := math.Abs(1200 * math.Log2(mode.FrequencyHz/target)); cents < nearest {
					nearest = cents
				}
			}

			per[i] = nearest
			sum += audibility(i) * nearest * nearest
			weight += audibility(i)
		}

		return math.Sqrt(sum / weight), per
	}

	current := append([]float64(nil), bestBank...)
	currentScore := best

	for step := 0.20; step > 1e-5; step *= 0.7 {
		improved := true
		for improved {
			improved = false

			for index := range current {
				if specs[index].ID == "physicalTom.quality" {
					continue
				}

				for _, delta := range []float64{step, -step} {
					trial := append([]float64(nil), current...)
					trial[index] = math.Min(1, math.Max(0, trial[index]+delta))

					if got, per := score(trial); got < currentScore {
						current, currentScore, bestPer = trial, got, per
						improved = true
					}
				}
			}
		}
	}

	fmt.Printf("\nafter hill-climbing on frequency alone: %.1f cents RMS\n", currentScore)

	for i, target := range referenceHz {
		fmt.Printf("  %7.1f Hz  nearest mode %8.1f cents\n", target, bestPer[i])
	}

	for index, spec := range specs {
		fmt.Printf("  %-28s %.4f\n", spec.ID, current[index])
	}
}
