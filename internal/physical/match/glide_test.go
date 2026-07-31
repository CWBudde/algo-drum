package match

import (
	"fmt"
	"math"
	"testing"
)

// sweptTone is a decaying sinusoid whose frequency ramps linearly from
// startHz to endHz over rampSeconds and then holds.
//
// Linear rather than the exponential bend `tone` produces, because the probe
// averages phase over a symmetric window and the envelope filter is
// zero-phase: on a linear ramp both of those are unbiased, so the answer the
// measure should return is known exactly rather than to within the convexity
// of a curve. It is the shape that can tell a mistake from a smear.
type sweptTone struct {
	startHz     float64
	endHz       float64
	rampSeconds float64
	amplitude   float64
	t60Seconds  float64
}

func (s sweptTone) frequencyAt(seconds float64) float64 {
	if seconds >= s.rampSeconds {
		return s.endHz
	}

	return s.startHz + (s.endHz-s.startHz)*seconds/s.rampSeconds
}

func synthesizeSwept(tones []sweptTone, sampleRateHz, durationSeconds float64) []float64 {
	samples := make([]float64, int(durationSeconds*sampleRateHz))

	for _, s := range tones {
		decay := math.Log(1000) / s.t60Seconds
		phase := 0.0

		for n := range samples {
			seconds := float64(n) / sampleRateHz
			samples[n] += s.amplitude * math.Exp(-decay*seconds) * math.Sin(phase)
			phase += 2 * math.Pi * s.frequencyAt(seconds) / sampleRateHz
		}
	}

	return samples
}

// TestGlideRecoversAKnownLinearSweep is the calibration: one partial, nothing
// else in the signal, and a bend whose size follows from the construction.
func TestGlideRecoversAKnownLinearSweep(t *testing.T) {
	t.Parallel()

	// T60 of 3 s so the partial is barely 8 dB down at the late probe and the
	// support floor never binds — the late probe stays where the options put
	// it, and the expected value below is the one the measure should return.
	swept := sweptTone{startHz: 158, endHz: 148, rampSeconds: 0.5, amplitude: 1, t60Seconds: 3}

	options := DefaultOptions()

	features, err := Extract(synthesizeSwept([]sweptTone{swept}, testSampleRate, 1.5),
		testSampleRate, options)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	if !features.GlideMeasured {
		t.Fatalf("glide not measured on a partial that rings for 3 s")
	}

	want := 1200 * math.Log2(swept.frequencyAt(options.GlideEarlySeconds)/
		swept.frequencyAt(options.GlideLateSeconds))

	if math.Abs(features.GlideCents-want) > 5 {
		t.Errorf("glide = %.1f cents, want %.1f ± 5", features.GlideCents, want)
	}
}

// TestGlideIgnoresASecondPartial is the defect this measure was rebuilt for.
//
// Two partials, neither of them bending, at the separations the model's (0,1)
// doublet actually takes as cavity coupling is turned up: 1.18 at the shipped
// stiffness, 1.43 at 0.30, 1.84 at a rigid shell. The fundamental decays the
// way the model's does and the upper partial outlives it, which is the whole
// trap — once the fundamental is gone the tracker has nothing left in its
// passband but the leakage from above, and the offset to it reads as an
// enormous downglide.
//
// Measured on the previous estimator, which probed at a fixed 0.400 s: −13,
// −717 and −625 cents across these three ratios on the model's own renders.
// The true answer is zero at every ratio.
//
// The tolerance is 15 cents rather than 1, and that is a statement about what
// this method can do, not slack. The probe filter is second-order and its
// cutoff is set from the spacing itself, so the neighbour is only ever 30 dB
// or so down inside the passband; what is left of it makes the phase slope
// swing at the beat rate, and the wide late probe averages that down but not
// away. Fifteen cents against a weight of 1/40 is a third of one "clearly
// wrong" unit, and it moves smoothly with the coupling instead of falling off
// a cliff — which is the whole difference between a bias and the defect.
func TestGlideIgnoresASecondPartial(t *testing.T) {
	t.Parallel()

	const fundamentalHz = 154

	for _, ratio := range []float64{1.18, 1.43, 1.84} {
		t.Run(ratioName(ratio), func(t *testing.T) {
			t.Parallel()

			tones := []sweptTone{
				// The model's own fundamental: 0.21 s of T60, so it is a
				// hundred dB down by the time a 0.400 s probe fires.
				{
					startHz: fundamentalHz, endHz: fundamentalHz, rampSeconds: 1,
					amplitude: 1, t60Seconds: 0.21,
				},
				// The upper branch: quieter, but it rings three times as long.
				{
					startHz: fundamentalHz * ratio, endHz: fundamentalHz * ratio, rampSeconds: 1,
					amplitude: 0.32, t60Seconds: 0.64,
				},
			}

			features, err := Extract(synthesizeSwept(tones, testSampleRate, 1.2),
				testSampleRate, DefaultOptions())
			if err != nil {
				t.Fatalf("Extract() error = %v", err)
			}

			if !features.GlideMeasured {
				t.Fatalf("glide not measured; the fundamental rings for 0.21 s, "+
					"which is long enough for a %.0f ms span", 1000*DefaultOptions().GlideMinSpanSeconds)
			}

			if math.Abs(features.GlideCents) > 15 {
				t.Errorf("glide of two unbent partials %.2f apart = %.1f cents, want ~0",
					ratio, features.GlideCents)
			}
		})
	}
}

// TestGlideBendsWithTheFundamentalNotTheNeighbour is the other half: the same
// pair, but now the fundamental really does bend. Rejecting the neighbour must
// not have cost the measure its sensitivity.
func TestGlideBendsWithTheFundamentalNotTheNeighbour(t *testing.T) {
	t.Parallel()

	const ratio = 1.43

	// The bend is over by 120 ms, so it is complete wherever the late probe
	// lands and the expected value does not depend on the fundamental's decay.
	bent := sweptTone{startHz: 162, endHz: 154, rampSeconds: 0.12, amplitude: 1, t60Seconds: 0.5}
	neighbour := sweptTone{
		startHz: 154 * ratio, endHz: 154 * ratio, rampSeconds: 1,
		amplitude: 0.32, t60Seconds: 0.64,
	}

	features, err := Extract(synthesizeSwept([]sweptTone{bent, neighbour}, testSampleRate, 1.2),
		testSampleRate, DefaultOptions())
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	if !features.GlideMeasured {
		t.Fatal("glide not measured")
	}

	// Loose, because the probe filter cannot be both selective enough to reject
	// a neighbour 66 Hz away and quick enough to follow a bend that finishes in
	// 120 ms: its ring time spreads the early reading over part of the bend and
	// the recovered size comes in short. What the test holds is that the bend is
	// seen, with the right sign and the right order of magnitude, on the
	// fundamental — with the neighbour present, which is where the old measure
	// reported hundreds of cents in the wrong direction.
	want := 1200 * math.Log2(bent.frequencyAt(DefaultOptions().GlideEarlySeconds)/bent.endHz)
	if math.Abs(features.GlideCents-want) > 25 {
		t.Errorf("glide = %.1f cents, want %.1f ± 25", features.GlideCents, want)
	}

	if features.GlideCents < 30 {
		t.Errorf("glide = %.1f cents, want a clear downward bend", features.GlideCents)
	}
}

// TestGlideRefusesADeadFundamental pins the sentinel. A partial that is gone
// before the shortest span the options allow has no measurable bend, and saying
// so is the point: the alternative is a number read off whatever was left in
// the filter.
//
// Driven through measureGlide rather than Extract because a partial this short
// is not detected at all — detection opens at 0.05 s — and the sentinel has to
// hold for the partials that *are* detected and then die.
func TestGlideRefusesADeadFundamental(t *testing.T) {
	t.Parallel()

	options := DefaultOptions()

	tones := []sweptTone{
		{startHz: 154, endHz: 154, rampSeconds: 1, amplitude: 1, t60Seconds: 0.03},
		{startHz: 154 * 1.43, endHz: 154 * 1.43, rampSeconds: 1, amplitude: 0.32, t60Seconds: 0.64},
	}
	hit := normalizePeak(synthesizeSwept(tones, testSampleRate, 1.2))

	cents, ok := measureGlide(hit, testSampleRate, options, 154, glideCutoffHz([]Partial{
		{FrequencyHz: 154}, {FrequencyHz: 154 * 1.43},
	}, 0))
	if ok {
		t.Errorf("glide = %.1f cents on a partial with a 30 ms T60, want no reading", cents)
	}

	if cents != 0 {
		t.Errorf("unmeasured glide = %.1f cents, want zero", cents)
	}

	// The same signal with a fundamental that survives does produce one, so the
	// refusal above is about support and not about the setup.
	tones[0].t60Seconds = 0.5
	alive := normalizePeak(synthesizeSwept(tones, testSampleRate, 1.2))

	if _, ok := measureGlide(alive, testSampleRate, options, 154, glideCutoffHz([]Partial{
		{FrequencyHz: 154}, {FrequencyHz: 154 * 1.43},
	}, 0)); !ok {
		t.Error("no glide measured on a fundamental that rings for half a second")
	}
}

// TestGlideErrorHandlesAnAbsentReading pins what the objective does with the
// sentinel, which is the part that would otherwise score noise as agreement.
func TestGlideErrorHandlesAnAbsentReading(t *testing.T) {
	t.Parallel()

	measured := func(cents float64) Features {
		return Features{GlideCents: cents, GlideMeasured: true}
	}

	cases := []struct {
		name                 string
		reference, candidate Features
		want                 float64
	}{
		{"both measured", measured(90), measured(50), 40},
		{"candidate unreadable", measured(90), Features{}, unreadableGlideCents},
		{"reference unreadable", Features{}, measured(50), 0},
		{"neither", Features{}, Features{}, 0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			if got := glideError(c.reference, c.candidate); got != c.want {
				t.Errorf("glideError() = %v, want %v", got, c.want)
			}
		})
	}

	// An unreadable candidate must cost more than a candidate that is merely
	// wrong by a comfortable margin, or the cheapest route through this term is
	// a drum with no sustain to measure.
	if glideError(measured(90), Features{}) <= glideError(measured(90), measured(80)) {
		t.Error("an unreadable glide costs no more than a slightly wrong one")
	}
}

// TestGlidePartialPrefersTheFundamental pins the choice of partial. On this
// repository's tom reference the loudest partial is a 212 Hz mode with a 0.16 s
// T60 sitting above a 118 Hz fundamental that rings for 1.5 s; reading the bend
// off the loudest was half of the defect.
func TestGlidePartialPrefersTheFundamental(t *testing.T) {
	t.Parallel()

	partials := []Partial{
		{FrequencyHz: 118, LevelDB: -7.7},
		{FrequencyHz: 212, LevelDB: 0},
	}

	if got := glidePartial(partials, DefaultOptions().GlidePartialWindowDB); got != 0 {
		t.Errorf("glide partial = %d (%.0f Hz), want the fundamental",
			got, partials[got].FrequencyHz)
	}

	// But not a room mode 40 dB below the note.
	buried := []Partial{
		{FrequencyHz: 61, LevelDB: -40},
		{FrequencyHz: 118, LevelDB: -7.7},
		{FrequencyHz: 212, LevelDB: 0},
	}

	if got := glidePartial(buried, DefaultOptions().GlidePartialWindowDB); got != 1 {
		t.Errorf("glide partial = %d (%.0f Hz), want the fundamental, not the buried peak",
			got, buried[got].FrequencyHz)
	}

	if got := glidePartial(nil, 20); got != -1 {
		t.Errorf("glide partial of nothing = %d, want -1", got)
	}
}

func ratioName(ratio float64) string {
	return fmt.Sprintf("ratio %.2f", ratio)
}
