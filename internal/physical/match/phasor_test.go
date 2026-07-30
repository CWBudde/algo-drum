package match

import (
	"math"
	"math/rand"
	"testing"
)

// withExactPhasor runs fn with the heterodyne phasor re-derived on every
// sample, which is the exact per-sample Sincos form the recurrence replaced.
func withExactPhasor(tb testing.TB, fn func()) {
	tb.Helper()

	restore := phasorAnchorInterval
	phasorAnchorInterval = 1

	defer func() { phasorAnchorInterval = restore }()

	fn()
}

// TestPhasorRecurrenceMatchesExactAngle bounds the drift the recursive phasor
// introduces against re-deriving the angle every sample, across the audio
// range. The rotor's modulus is not exactly one, so this is what the periodic
// re-anchoring has to hold in check.
func TestPhasorRecurrenceMatchesExactAngle(t *testing.T) {
	const (
		sampleRateHz = 44100
		samples      = 88200
		cutoffHz     = 40
		sections     = 2
	)

	rng := rand.New(rand.NewSource(3))

	hit := make([]float64, samples)
	for i := range hit {
		hit[i] = rng.NormFloat64()
	}

	// Loose enough not to trip on a compiler or platform reassociating the
	// recurrence, tight enough that nothing downstream could notice.
	const tolerance = 1e-9

	for _, frequencyHz := range []float64{50, 150.08, 523.7, 2000, 7999.9, 19000} {
		gotInPhase, gotQuadrature := heterodyne(hit, sampleRateHz, frequencyHz, cutoffHz, sections)

		var wantInPhase, wantQuadrature []float64

		withExactPhasor(t, func() {
			wantInPhase, wantQuadrature = heterodyne(hit, sampleRateHz, frequencyHz, cutoffHz, sections)
		})

		worst := 0.0
		for n := range wantInPhase {
			worst = math.Max(worst, math.Abs(gotInPhase[n]-wantInPhase[n]))
			worst = math.Max(worst, math.Abs(gotQuadrature[n]-wantQuadrature[n]))
		}

		t.Logf("%9.2f Hz: worst absolute error %.3e", frequencyHz, worst)

		if worst > tolerance {
			t.Errorf("%v Hz: worst error %.3e exceeds %.3e", frequencyHz, worst, tolerance)
		}
	}
}

// TestPhasorRecurrenceLeavesFeaturesIntact measures the drift where it
// actually matters: in the partial frequencies, ring times and levels the
// fitter scores. A tone stack with distinct ring times exercises the decay
// measurement that heterodyne feeds.
func TestPhasorRecurrenceLeavesFeaturesIntact(t *testing.T) {
	const sampleRateHz = 44100

	rng := rand.New(rand.NewSource(11))

	partials := []struct{ frequencyHz, t60Seconds float64 }{
		{150.08, 1.6},
		{231.4, 1.1},
		{318.9, 0.8},
		{452.3, 0.55},
		{701.7, 0.4},
		{1103.2, 0.25},
	}

	samples := make([]float64, int(1.2*sampleRateHz))
	for n := range samples {
		seconds := float64(n) / sampleRateHz

		value := 0.0
		for _, partial := range partials {
			value += math.Exp(-math.Log(1000)*seconds/partial.t60Seconds) *
				math.Sin(2*math.Pi*partial.frequencyHz*seconds)
		}

		samples[n] = value/float64(len(partials)) + 0.0005*rng.NormFloat64()
	}

	options := DefaultOptions()

	got, err := Extract(samples, sampleRateHz, options)
	if err != nil {
		t.Fatal(err)
	}

	var want Features

	withExactPhasor(t, func() {
		want, err = Extract(samples, sampleRateHz, options)
	})

	if err != nil {
		t.Fatal(err)
	}

	if len(got.Partials) != len(want.Partials) {
		t.Fatalf("detected %d partials, want %d", len(got.Partials), len(want.Partials))
	}

	worstFrequency, worstT60, worstLevel := 0.0, 0.0, 0.0

	for i := range want.Partials {
		recurrence, exact := got.Partials[i], want.Partials[i]
		worstFrequency = math.Max(worstFrequency,
			math.Abs(recurrence.FrequencyHz-exact.FrequencyHz))
		worstT60 = math.Max(worstT60,
			math.Abs(recurrence.T60Seconds-exact.T60Seconds))
		worstLevel = math.Max(worstLevel,
			math.Abs(recurrence.LevelDB-exact.LevelDB))
	}

	distance := Distance(want, got, DefaultWeights()).Total

	t.Logf("%d partials: frequency %.3e Hz, T60 %.3e s, level %.3e dB, distance %.3e",
		len(want.Partials), worstFrequency, worstT60, worstLevel, distance)

	// The fitter's own costs are order one; anything at this scale is far
	// below the resolution of every decision made from it.
	if distance > 1e-6 {
		t.Errorf("distance between extractions = %.3e, want below 1e-6", distance)
	}

	if worstFrequency > 1e-6 || worstT60 > 1e-6 || worstLevel > 1e-6 {
		t.Errorf("frequency %.3e Hz, T60 %.3e s, level %.3e dB: one exceeds 1e-6",
			worstFrequency, worstT60, worstLevel)
	}
}
