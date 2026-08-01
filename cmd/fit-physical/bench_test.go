package main

import (
	"math/rand"
	"testing"

	"github.com/cwbudde/algo-drum/internal/drum"
	"github.com/cwbudde/algo-drum/internal/physical"
	"github.com/cwbudde/algo-drum/internal/physical/match"
)

// benchEvaluator builds an evaluator over a synthetic reference, so the
// benchmark needs no recording and measures only the candidate side.
func benchEvaluator(tb testing.TB, durationSeconds float64) *evaluator {
	tb.Helper()

	const sampleRateHz = 44100

	bank, free, err := resolveFixed(assignmentFlag{}, true, false)
	if err != nil {
		tb.Fatalf("resolveFixed: %v", err)
	}

	options := match.DefaultOptions()

	// Any plausible drum works as the target; the distance is symmetric in cost
	// and this keeps the benchmark independent of reference/.
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

// BenchmarkCost measures one objective evaluation end to end — the unit the
// search spends all its time in.
func BenchmarkCost(b *testing.B) {
	probe := benchEvaluator(b, 1.2)
	rng := rand.New(rand.NewSource(1))

	positions := make([][]float64, 16)
	for i := range positions {
		position := make([]float64, probe.dimensions())
		for j := range position {
			position[j] = rng.Float64()
		}

		positions[i] = position
	}

	b.ResetTimer()

	for i := 0; b.Loop(); i++ {
		probe.cost(positions[i%len(positions)])
	}
}

// BenchmarkRender isolates synthesis from measurement.
func BenchmarkRender(b *testing.B) {
	probe := benchEvaluator(b, 1.2)

	b.ResetTimer()

	for b.Loop() {
		if _, err := probe.render(1); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkNewDoubleHead isolates model construction, which the search repeats
// once per evaluation because every candidate is a different drum.
func BenchmarkNewDoubleHead(b *testing.B) {
	probe := benchEvaluator(b, 1.2)

	config, err := probe.config()
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()

	for b.Loop() {
		if _, err := physical.NewDoubleHead(config); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkExtract isolates feature extraction.
func BenchmarkExtract(b *testing.B) {
	probe := benchEvaluator(b, 1.2)

	samples, err := probe.render(1)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()

	for b.Loop() {
		if _, err := match.Extract(samples, probe.sampleRateHz, probe.options); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkPhysicalTomConfig isolates the knob-bank mapping, which is pure
// arithmetic and should not register at all.
func BenchmarkPhysicalTomConfig(b *testing.B) {
	probe := benchEvaluator(b, 1.2)

	b.ResetTimer()

	for b.Loop() {
		if _, err := drum.PhysicalTomConfig(probe.bank, drum.NeutralDecayAmount, probe.sampleRateHz); err != nil {
			b.Fatal(err)
		}
	}
}
