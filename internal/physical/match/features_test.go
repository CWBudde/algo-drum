package match

import (
	"errors"
	"math"
	"testing"
)

// tone describes one exponentially decaying sinusoid.
type tone struct {
	frequencyHz float64
	amplitude   float64
	t60Seconds  float64
	// glideCents bends the tone down by this much with a 60 ms time constant,
	// the way a struck head releases its excess tension.
	glideCents float64
	// phase is where the tone starts, in radians. It is zero for every case
	// where the partials are far enough apart not to interact; it matters only
	// when two of them are close enough to beat, where the beat's phase decides
	// where the first null falls. See esprit_test.go's split pair.
	phase float64
}

// synthesize builds a signal whose partial frequencies, levels and decay times
// are known exactly, so the extractor can be measured rather than compared to
// itself.
func synthesize(tones []tone, sampleRateHz, durationSeconds float64) []float64 {
	samples := make([]float64, int(durationSeconds*sampleRateHz))

	for _, t := range tones {
		decay := math.Log(1000) / t.t60Seconds
		phase := t.phase

		for n := range samples {
			seconds := float64(n) / sampleRateHz

			frequency := t.frequencyHz
			if t.glideCents != 0 {
				frequency *= math.Pow(2, t.glideCents/1200*math.Exp(-seconds/0.06))
			}

			samples[n] += t.amplitude * math.Exp(-decay*seconds) * math.Sin(phase)
			phase += 2 * math.Pi * frequency / sampleRateHz
		}
	}

	return samples
}

// wellSeparatedTones keeps every partial far enough apart that the envelope
// filter isolates it, which is the condition under which the recovered numbers
// are supposed to be exact.
func wellSeparatedTones() []tone {
	return []tone{
		{frequencyHz: 120, amplitude: 1.0, t60Seconds: 1.20},
		{frequencyHz: 197, amplitude: 0.50, t60Seconds: 0.80},
		{frequencyHz: 331, amplitude: 0.25, t60Seconds: 0.55},
		{frequencyHz: 512, amplitude: 0.12, t60Seconds: 0.40},
	}
}

const testSampleRate = 44100

// testHitSeconds is how long a synthetic hit must be to stand in for a recorded
// one, and it is derived rather than chosen so that widening the analysis span
// cannot silently outgrow the fixtures again.
//
// Two things have to fit inside it. The analysis span is the obvious one. The
// margin past it is the envelope filter's edge transient: heterodyne filters
// zero-phase, which rings at *both* ends of the array, so a fit window reaching
// the last sample is fitting that transient rather than the partial. It is not a
// small effect — with the fit window ending exactly at the end of a 1.5 s
// fixture, the 120 Hz partial came back with a decay range of 202 dB and an R²
// of 0.967, against 78 dB and 1.0000 once 0.2 s of margin was added. The ring
// times survived it; the confidence attached to them did not.
//
// Real recordings have this margin for free — reference/tt08x08/lp/hd is 2.083 s
// against a fit window ending at 1.60 s — which is why the defect showed up in
// the fixtures and not in the measurements.
var testHitSeconds = DefaultOptions().AnalysisSeconds + 0.2

// testNoiseFloorDB is the floor every fixture measured through Extract carries,
// and it is there to make the decay problem well-posed rather than to make it
// hard.
//
// decayFloorFit estimates an exponential *plus a stationary floor*, which is
// what a recorded partial is. Give it a mathematically perfect exponential and
// the floor parameter has nothing to identify it: the trace keeps falling
// through -200, -400 dB, the model tries to explain that with a floor of zero,
// and the fit degrades the further the window extends. This is not a defect the
// estimator can fix — it is a signal no recording produces.
//
// It showed up when the fit window widened to 1.60 s (PLAN N17). Halving every
// ring time in wellSeparatedTones should move the decay term by exactly ln 2 =
// 0.693 and nothing else. Floorless, it measured 0.591 with the level term
// dragged to 10.21 dB; at this floor it measures 0.696 with the level term at
// 0.54 dB. At the old 0.60 s window the floorless case happened to be fine,
// because the window ended before the trace ran away.
//
// -120 dB is chosen to be inaudible in every sense that matters: some 20 dB
// below the noise floor of a 20-bit recording, and far below the -45 dB
// truncation and the -42 dB partial floor, so no test's expectations about
// frequency or level shift because of it.
const testNoiseFloorDB = -120

func extractTones(t *testing.T, tones []tone) Features {
	t.Helper()

	options := DefaultOptions()

	features, err := Extract(
		synthesizeNoisy(tones, testSampleRate, testHitSeconds, testNoiseFloorDB),
		testSampleRate, options,
	)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	return features
}

func TestExtractRecoversFrequencyAndDecay(t *testing.T) {
	t.Parallel()

	tones := wellSeparatedTones()
	features := extractTones(t, tones)

	if len(features.Partials) != len(tones) {
		t.Fatalf("partials = %d, want %d: %+v", len(features.Partials), len(tones), features.Partials)
	}

	for i, want := range tones {
		got := features.Partials[i]

		cents := math.Abs(1200 * math.Log2(got.FrequencyHz/want.frequencyHz))
		if cents > 2 {
			t.Errorf("partial %d frequency = %.3f Hz, want %.3f Hz (%.2f cents out)",
				i, got.FrequencyHz, want.frequencyHz, cents)
		}

		wantLevelDB := 20 * math.Log10(want.amplitude/tones[0].amplitude)
		if math.Abs(got.LevelDB-wantLevelDB) > 1.5 {
			t.Errorf("partial %d level = %.2f dB, want %.2f dB", i, got.LevelDB, wantLevelDB)
		}

		if ratio := got.T60Seconds / want.t60Seconds; ratio < 0.95 || ratio > 1.05 {
			t.Errorf("partial %d T60 = %.4f s, want %.4f s (ratio %.3f)",
				i, got.T60Seconds, want.t60Seconds, ratio)
		}

		if got.FitQuality < 0.99 {
			t.Errorf("partial %d fit quality = %.4f, want an exponential", i, got.FitQuality)
		}
	}
}

func TestExtractMeasuresTheGlide(t *testing.T) {
	t.Parallel()

	tones := wellSeparatedTones()
	tones[0].glideCents = 100

	features := extractTones(t, tones)

	// The probes sit at 30 ms and 400 ms, so they cannot see the whole
	// exponential: exp(-0.03/0.06) − exp(-0.4/0.06) of it, or 60.6 %. What the
	// measure has to get right is the size and the sign, not the tone's peak
	// instantaneous frequency, which no listener hears either.
	// The measure runs a few cents high: the probe averages phase over ±5 ms
	// and the envelope filter smears a few more, and an exponential bend is
	// convex, so both pull the early reading up. It is the same bias on both
	// sides of a comparison, which is what a fit needs.
	want := 100 * (math.Exp(-0.03/0.06) - math.Exp(-0.4/0.06))
	if math.Abs(features.GlideCents-want) > 12 {
		t.Errorf("glide = %.1f cents, want %.1f ± 12", features.GlideCents, want)
	}

	flat := extractTones(t, wellSeparatedTones())
	if math.Abs(flat.GlideCents) > 3 {
		t.Errorf("glide of an unbent tone = %.1f cents, want ~0", flat.GlideCents)
	}
}

func TestExtractIsGainInvariant(t *testing.T) {
	t.Parallel()

	tones := wellSeparatedTones()

	quiet := make([]tone, len(tones))
	copy(quiet, tones)
	for i := range quiet {
		quiet[i].amplitude *= 0.05
	}

	loud := extractTones(t, tones)
	soft := extractTones(t, quiet)

	terms := Distance(loud, soft, DefaultWeights())
	if terms.Total > 1e-6 {
		t.Errorf("distance across a 26 dB gain change = %+v, want zero", terms)
	}
}

func TestExtractRejectsInvalidOptions(t *testing.T) {
	t.Parallel()

	samples := synthesize(wellSeparatedTones(), testSampleRate, 0.5)

	cases := map[string]func(*Options){
		"analysis seconds": func(o *Options) { o.AnalysisSeconds = 0 },
		"max partials":     func(o *Options) { o.MaxPartials = 0 },
		"partial band":     func(o *Options) { o.MaxFrequencyHz = o.MinFrequencyHz },
		"fft size":         func(o *Options) { o.FFTSize = 1000 },
		"window fft size":  func(o *Options) { o.WindowFFTSize = 0 },
		"decay fit span":   func(o *Options) { o.DecayFitEndSeconds = o.DecayFitStartSeconds },
		"band layout":      func(o *Options) { o.BandsPerOctave = 0 },
		"glide probes":     func(o *Options) { o.GlideLateSeconds = o.GlideEarlySeconds },
		"glide span":       func(o *Options) { o.GlideMinSpanSeconds = 0 },
		"glide span fit":   func(o *Options) { o.GlideMinSpanSeconds = o.GlideLateSeconds },
		"glide floor":      func(o *Options) { o.GlideFloorDB = 0 },
		"glide window":     func(o *Options) { o.GlidePartialWindowDB = -1 },
		"envelope frame":   func(o *Options) { o.EnvelopeFrameSeconds = 0 },
		"windows":          func(o *Options) { o.Windows = nil },
	}

	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			options := DefaultOptions()
			mutate(&options)

			if _, err := Extract(samples, testSampleRate, options); !errors.Is(err, ErrInvalidOptions) {
				t.Errorf("Extract() error = %v, want ErrInvalidOptions", err)
			}
		})
	}

	if _, err := Extract(samples, 0, DefaultOptions()); !errors.Is(err, ErrInvalidOptions) {
		t.Errorf("Extract(sampleRate=0) error = %v, want ErrInvalidOptions", err)
	}
}

func TestProminenceRejectsAShoulder(t *testing.T) {
	t.Parallel()

	// A tall peak with a bump on its skirt that never falls back down: the
	// bump is a local maximum but has almost no prominence.
	magnitude := []float64{0.1, 1.0, 10.0, 3.0, 3.2, 3.1, 0.5, 0.2}

	// 10.0 against the 1.0 valley on its left — the higher of the two valleys.
	if got := prominenceDB(magnitude, 2); math.Abs(got-20) > 0.01 {
		t.Errorf("prominence of the peak = %.2f dB, want 20", got)
	}

	if got := prominenceDB(magnitude, 4); got > 1 {
		t.Errorf("prominence of the shoulder = %.1f dB, want ~0", got)
	}
}
