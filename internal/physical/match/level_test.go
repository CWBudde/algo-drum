package match

import (
	"math"
	"testing"
)

// decayingSine adds one exponentially decaying sinusoid to signal, starting at
// amplitude and falling by 60 dB over t60 seconds.
func decayingSine(signal []float64, sampleRateHz, frequencyHz, amplitude, t60 float64) {
	rate := math.Log(1000) / t60

	for sample := range signal {
		seconds := float64(sample) / sampleRateHz
		signal[sample] += amplitude *
			math.Exp(-rate*seconds) *
			math.Sin(2*math.Pi*frequencyHz*seconds)
	}
}

// TestShortPartialsDoNotOutrankLongOnes is the regression for a level estimator
// that reported a partial's strike amplitude as its sustain-transform magnitude
// divided by the window attenuation implied by its fitted decay.
//
// That divisor grows without bound as the ring time shortens — 16 dB at T60 1.5 s,
// 100 dB at 0.1 s for the default window — so a short partial's level was
// dominated by its fitted rate rather than by its amplitude. On the reference
// recording a 73 ms component came out as the loudest partial present, and since
// every level is expressed relative to the strongest, that pushed genuine
// partials below PartialFloorDB and silently shrank the target.
//
// Here the quiet partial rings for a tenth as long as the loud one. Any
// estimator that pays attention to ring time when asked for level will get this
// backwards.
func TestShortPartialsDoNotOutrankLongOnes(t *testing.T) {
	t.Parallel()

	const (
		sampleRateHz = 44100
		loudHz       = 220
		quietHz      = 1200
		// 6 dB apart at the strike, and an order of magnitude apart in ring time.
		loudAmplitude  = 1.0
		quietAmplitude = 0.5
		loudT60        = 1.2
		quietT60       = 0.12
	)

	signal := make([]float64, int(1.2*sampleRateHz))
	decayingSine(signal, sampleRateHz, loudHz, loudAmplitude, loudT60)
	decayingSine(signal, sampleRateHz, quietHz, quietAmplitude, quietT60)

	features, err := Extract(signal, sampleRateHz, DefaultOptions())
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	find := func(target float64) (Partial, bool) {
		for _, partial := range features.Partials {
			if math.Abs(partial.FrequencyHz-target) < 10 {
				return partial, true
			}
		}

		return Partial{}, false
	}

	loud, ok := find(loudHz)
	if !ok {
		t.Fatalf("the %d Hz partial was not detected at all; got %v", loudHz, features.Partials)
	}

	quiet, ok := find(quietHz)
	if !ok {
		t.Fatalf("the %d Hz partial was not detected at all; got %v", quietHz, features.Partials)
	}

	if quiet.LevelDB >= loud.LevelDB {
		t.Errorf("the short partial reported %.1f dB against the long one's %.1f dB; "+
			"it is half the amplitude and must rank below it",
			quiet.LevelDB, loud.LevelDB)
	}

	// And it should be roughly the 6 dB it really is, not merely on the right
	// side of the comparison: the point is a level estimate, not an ordering.
	if gap := loud.LevelDB - quiet.LevelDB; gap < 3 || gap > 12 {
		t.Errorf("levels are %.1f dB apart; the strike amplitudes are 6 dB apart", gap)
	}

	// The ring times are the other half of the measurement and must survive the
	// level change unharmed.
	for _, measured := range []struct {
		name   string
		got    float64
		expect float64
	}{
		{"long", loud.T60Seconds, loudT60},
		{"short", quiet.T60Seconds, quietT60},
	} {
		if ratio := measured.got / measured.expect; ratio < 0.7 || ratio > 1.4 {
			t.Errorf("%s partial T60 %.3f s, want about %.3f s",
				measured.name, measured.got, measured.expect)
		}
	}
}
