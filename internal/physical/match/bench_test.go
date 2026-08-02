package match

import (
	"slices"
	"testing"
)

// benchTones is a drum-shaped partial table rather than the four well-separated
// ones the correctness fixtures use: sixteen modes, inharmonically spaced and
// spread over 40 dB, so the detector fills its MaxPartials budget and the decay
// fit runs the number of times a real hit makes it run. A benchmark over four
// partials would measure a quarter of the work a fit actually pays for.
func benchTones() []tone {
	return []tone{
		{frequencyHz: 118.05, amplitude: 1.000, t60Seconds: 1.40, glideCents: 70},
		{frequencyHz: 139.60, amplitude: 0.720, t60Seconds: 1.10},
		{frequencyHz: 212.78, amplitude: 0.880, t60Seconds: 0.62},
		{frequencyHz: 255.70, amplitude: 0.310, t60Seconds: 0.48},
		{frequencyHz: 297.40, amplitude: 0.240, t60Seconds: 0.55},
		{frequencyHz: 358.10, amplitude: 0.190, t60Seconds: 0.90},
		{frequencyHz: 431.60, amplitude: 0.150, t60Seconds: 0.44},
		{frequencyHz: 512.30, amplitude: 0.120, t60Seconds: 0.40},
		{frequencyHz: 618.90, amplitude: 0.095, t60Seconds: 0.36},
		{frequencyHz: 744.20, amplitude: 0.075, t60Seconds: 0.33},
		{frequencyHz: 889.70, amplitude: 0.060, t60Seconds: 0.30},
		{frequencyHz: 1043.10, amplitude: 0.048, t60Seconds: 0.27},
		{frequencyHz: 1256.40, amplitude: 0.038, t60Seconds: 0.24},
		{frequencyHz: 1502.90, amplitude: 0.030, t60Seconds: 0.21},
		{frequencyHz: 1811.50, amplitude: 0.023, t60Seconds: 0.18},
		{frequencyHz: 2210.70, amplitude: 0.018, t60Seconds: 0.15},
	}
}

// benchHit is the synthetic recording every benchmark below measures, built
// once because building it costs more than the thing being timed.
func benchHit(tb testing.TB) []float64 {
	tb.Helper()

	return synthesizeNoisy(benchTones(), testSampleRate, testHitSeconds, -80)
}

// BenchmarkExtract is the whole feature estimator: the unit cmd/fit-physical
// pays once per take per candidate, and the one that dominates a fit.
func BenchmarkExtract(b *testing.B) {
	samples := benchHit(b)
	options := DefaultOptions()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := Extract(samples, testSampleRate, options); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkExtractDiagnostics is the same measurement with the reporting-only
// statistics switched on, which is what cmd/measure-tom runs.
func BenchmarkExtractDiagnostics(b *testing.B) {
	samples := benchHit(b)
	options := DefaultOptions()
	options.Diagnostics = true

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := Extract(samples, testSampleRate, options); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkDistance scores an extraction against a perturbed copy of itself, so
// every term has something to do — a Features against itself would leave the
// partial matcher with sixteen exact pairs and no work.
func BenchmarkDistance(b *testing.B) {
	samples := benchHit(b)
	options := DefaultOptions()

	reference, err := Extract(samples, testSampleRate, options)
	if err != nil {
		b.Fatal(err)
	}

	shifted := benchTones()
	for i := range shifted {
		shifted[i].frequencyHz *= 1.01
		shifted[i].t60Seconds *= 0.85
	}

	candidate, err := Extract(synthesizeNoisy(shifted, testSampleRate, testHitSeconds, -80),
		testSampleRate, options)
	if err != nil {
		b.Fatal(err)
	}

	weights := DefaultWeights()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = Distance(reference, candidate, weights)
	}
}

// BenchmarkHeterodyne is one partial's shift to baseband and its zero-phase
// filtering, which Extract performs once per detected partial plus once for the
// glide.
func BenchmarkHeterodyne(b *testing.B) {
	hit := normalizePeak(benchHit(b))

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		heterodyne(hit, testSampleRate, 212.78, 20, 2)
	}
}

// BenchmarkMeasureDecays is the per-partial decay fit over the whole detected
// table, heterodyne included.
func BenchmarkMeasureDecays(b *testing.B) {
	hit := normalizePeak(benchHit(b))
	options := DefaultOptions()

	work := acquireExtractScratch()

	detected, err := detectPartials(work, hit, testSampleRate, options)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	// measureDecays writes through its argument, so each iteration gets its own
	// copy. Sixteen structs is nothing beside the fit it feeds.
	for b.Loop() {
		measureDecays(work, hit, testSampleRate, options, slices.Clone(detected))
	}
}

// BenchmarkZeroPhaseLowpassPair isolates the filter from the phasor that feeds
// it: the single most expensive loop in the package.
func BenchmarkZeroPhaseLowpassPair(b *testing.B) {
	hit := normalizePeak(benchHit(b))

	first := make([]float64, len(hit))
	second := make([]float64, len(hit))

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		copy(first, hit)
		copy(second, hit)
		zeroPhaseLowpassPair(first, second, []float64{0.5411961, 1.3065630}, 20, testSampleRate)
	}
}
