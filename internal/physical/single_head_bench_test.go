package physical

import (
	"math"
	"testing"
)

// retriggerEvery bounds how long a single strike is allowed to decay before the
// benchmark hits it again.
//
// Without it these benchmarks measure subnormals rather than the model. One
// strike left to ring decays into the subnormal range after roughly fifteen
// seconds of audio, and from there every multiply and add in the modal update
// takes a microcode assist: measured on this bank, 40x realtime at 3 s of tail,
// 20x at 11 s, 2.4x at 21 s and 0.77x at 85 s, a hundredfold slowdown that says
// nothing about the cost of rendering a drum hit. Nothing reaches that state in
// practice — physicalTom.Tick gates on IsActive long before it, and the offline
// tools render 1.2 to 2 seconds — so the benchmark is what needed fixing, not
// the model. 256 chunks of 512 samples is about 2.7 s of audio, well inside the
// normal range.
const retriggerEvery = 256

func BenchmarkSingleHeadRender48k(b *testing.B) {
	config := DefaultPhysicalDrum()
	model, err := NewSingleHead(config)
	if err != nil {
		b.Fatal(err)
	}

	const chunkSamples = 512
	buffer := make([]float64, chunkSamples)
	b.ReportAllocs()
	b.SetBytes(chunkSamples * 8)
	b.ResetTimer()
	for index := range b.N {
		if index%retriggerEvery == 0 {
			if err := model.Trigger(1); err != nil {
				b.Fatal(err)
			}
		}

		model.Render(buffer)
	}
	b.StopTimer()

	samples := float64(b.N * chunkSamples)
	samplesPerSecond := samples / b.Elapsed().Seconds()
	b.ReportMetric(samplesPerSecond, "samples/s")
	b.ReportMetric(samplesPerSecond/config.SampleRateHz, "x_realtime")
	b.ReportMetric(float64(model.ModeCount()), "modes")
}

// BenchmarkDoubleHeadRender48k's x_realtime is still -benchtime dependent, and
// nothing may be quoted from it. retriggerEvery bounded the tail and so removed
// the subnormal blowup, but the run is still b.N x 512 samples against a strike
// every 256 chunks: at 20x every chunk is inside the first 0.21 s after a strike,
// and at 200x most of the window is settled ring. Measured host, three repeats
// each: 2.71-4.03x at 20x against 5.42-6.65x at 200x and 5.81-6.60x at 1000x —
// saturating rather than growing without bound, but not one number. The
// nonlinear cases below render a whole number of hit periods per iteration for
// exactly this reason.
func BenchmarkDoubleHeadRender48k(b *testing.B) {
	config := DefaultPhysicalDrum()
	model, err := NewDoubleHead(config)
	if err != nil {
		b.Fatal(err)
	}

	const chunkSamples = 512
	buffer := make([]float64, chunkSamples)
	b.ReportAllocs()
	b.SetBytes(chunkSamples * 8)
	b.ResetTimer()
	for index := range b.N {
		if index%retriggerEvery == 0 {
			if err := model.Trigger(1); err != nil {
				b.Fatal(err)
			}
		}

		model.Render(buffer)
	}
	b.StopTimer()

	samples := float64(b.N * chunkSamples)
	samplesPerSecond := samples / b.Elapsed().Seconds()
	b.ReportMetric(samplesPerSecond, "samples/s")
	b.ReportMetric(samplesPerSecond/config.SampleRateHz, "x_realtime")
	b.ReportMetric(
		float64(model.BatterModeCount()+model.ResonantModeCount()),
		"modes",
	)
}

// The three nonlinear cases below differ only in how often the strike lands, so
// each is one call to benchmarkNonlinearHits with a hit period:
//
//   - Active: a retrigger before every 512-sample chunk, i.e. 93.75 hits/s. The
//     worst case, and nothing a player produces; it is here because it never lets
//     the solve idle.
//   - Musical: 8 hits/s, a 16th-note roll at 120 bpm. This is the rate the N9
//     contract (">= 1.0x real time at musical hit rates") is written against.
//   - Steady: one strike per 0.25 s window, so most of each window is decayed
//     tail rather than attack.
//
// Every one of them renders a whole number of hit periods per iteration, which
// is what keeps their x_realtime independent of -benchtime. BenchmarkDoubleHead-
// Render48k is not built that way and still drifts; see the note there.
const (
	musicalTempoBPM     = 120.0
	musicalStepsPerBeat = 4.0
	// 8 hits/s, derived rather than written down as a sample count.
	musicalHitsPerSecond = musicalTempoBPM / 60 * musicalStepsPerBeat
	steadyWindowSeconds  = 0.25
)

// couplingBudget is one column of the §Cost table in
// docs/physical-nonlinearity.md. 408 is the full candidate table the shipped
// geometry produces — TestCoupledRenderIsBitExact asserts exactly that count at
// MaxCoefficients = 4096 — so any cap at or above it retains the whole table.
type couplingBudget struct {
	name            string
	enabled         bool
	maxCoefficients int
}

var couplingBudgets = []couplingBudget{
	{name: "off"},
	{name: "64", enabled: true, maxCoefficients: 64},
	{name: "128", enabled: true, maxCoefficients: 128},
	{name: "256", enabled: true, maxCoefficients: 256},
	{name: "408", enabled: true, maxCoefficients: 408},
}

func (budget couplingBudget) config() PhysicalDrum {
	config := DefaultPhysicalDrum()
	config.Nonlinearity.Coupling.Enabled = budget.enabled
	if budget.enabled {
		config.Nonlinearity.Coupling.MaxCoefficients = budget.maxCoefficients
	}

	return config
}

// renderTicked is DoubleHead.Render's body plus the one accumulation the
// solve-iteration metric needs. Render returns only the radiated sample, so a
// benchmark that wants NonlinearSolveIterations has to drive Tick itself; the
// added work is one integer add per sample.
func renderTicked(model *DoubleHead, dst []float64) int {
	iterations := 0
	for index := range dst {
		output := model.Tick()
		dst[index] = output.Radiated
		iterations += output.NonlinearSolveIterations
	}

	return iterations
}

// benchmarkNonlinearHits triggers once per iteration and renders exactly one hit
// period, sweeping the retained-coefficient budget as sub-benchmarks so the
// whole cost table comes out of one command.
//
// solve_iters/sample is the mean fixed-point iteration count. It is reported
// because "the cost is the table walk itself and not a harder solve" is a claim
// about the difference between the off row and the others, and that claim had no
// committed reproduction.
func benchmarkNonlinearHits(b *testing.B, hitsPerSecond float64) {
	b.Helper()

	for _, budget := range couplingBudgets {
		b.Run(budget.name, func(b *testing.B) {
			config := budget.config()
			model, err := NewDoubleHead(config)
			if err != nil {
				b.Fatal(err)
			}

			hitSamples := int(math.Round(config.SampleRateHz / hitsPerSecond))
			buffer := make([]float64, hitSamples)
			iterations := 0

			b.ReportAllocs()
			b.SetBytes(int64(hitSamples) * 8)
			b.ResetTimer()
			for range b.N {
				if err := model.Trigger(1); err != nil {
					b.Fatal(err)
				}

				iterations += renderTicked(model, buffer)
			}
			b.StopTimer()

			samples := float64(b.N * hitSamples)
			samplesPerSecond := samples / b.Elapsed().Seconds()
			b.ReportMetric(samplesPerSecond, "samples/s")
			b.ReportMetric(samplesPerSecond/config.SampleRateHz, "x_realtime")
			b.ReportMetric(
				float64(model.BatterModeCount()+model.ResonantModeCount()),
				"modes",
			)
			b.ReportMetric(float64(iterations)/samples, "solve_iters/sample")
			b.ReportMetric(
				float64(model.CouplingCoefficientCount()),
				"coefficients",
			)
		})
	}
}

func BenchmarkNonlinearDoubleHeadActive48k(b *testing.B) {
	const chunkSamples = 512

	benchmarkNonlinearHits(b, DefaultPhysicalDrum().SampleRateHz/chunkSamples)
}

func BenchmarkNonlinearDoubleHeadMusical48k(b *testing.B) {
	benchmarkNonlinearHits(b, musicalHitsPerSecond)
}

func BenchmarkNonlinearDoubleHeadSteady48k(b *testing.B) {
	benchmarkNonlinearHits(b, 1/steadyWindowSeconds)
}
