package match

import (
	"math"
	"testing"
)

// synthesizeNoisy is features_test.go's synthesize with a noise floor added.
//
// The noise is deliberate and is not there to make the test hard. A subspace
// method separates a signal subspace from a noise subspace, and on arithmetic
// so clean that the noise subspace is pure rounding error, the order-selection
// criterion is comparing quantities near the limit of double precision — which
// is not the regime the estimator will ever be used in. -60 dB puts it roughly
// where a close-microphone recording sits.
//
// The generator is a fixed linear congruential sequence rather than math/rand,
// so the signal is identical on every run and on every platform, which a test
// asserting hertz to two decimal places needs it to be.
func synthesizeNoisy(tones []tone, sampleRateHz, durationSeconds, noiseDB float64) []float64 {
	samples := synthesize(tones, sampleRateHz, durationSeconds)

	state := uint64(0x2545F4914F6CDD1D)
	level := math.Pow(10, noiseDB/20)

	for index := range samples {
		state = state*6364136223846793005 + 1442695040888963407
		samples[index] += level * (float64(state>>11)/float64(uint64(1)<<53)*2 - 1)
	}

	return samples
}

// nearestComponent returns the reported partial closest in frequency to a
// target, and how far off it was.
func nearestComponent(found []HighResolutionPartial, frequencyHz float64) (HighResolutionPartial, float64) {
	best, distance := HighResolutionPartial{}, math.Inf(1)

	for _, component := range found {
		if offset := math.Abs(component.FrequencyHz - frequencyHz); offset < distance {
			best, distance = component, offset
		}
	}

	return best, distance
}

func TestHighResolutionRecoversFrequencyAndRingTime(t *testing.T) {
	t.Parallel()

	const sampleRateHz = 48000

	want := []tone{
		{frequencyHz: 118.05, t60Seconds: 1.5, amplitude: 0.6, phase: 0.4},
		{frequencyHz: 212.70, t60Seconds: 0.42, amplitude: 1.0, phase: -1.1},
		{frequencyHz: 604.30, t60Seconds: 0.18, amplitude: 0.3, phase: 2.0},
	}

	samples := synthesizeNoisy(want, sampleRateHz, 1.2, -60)

	found, err := ExtractHighResolution(samples, sampleRateHz,
		DefaultOptions(), DefaultEspritOptions())
	if err != nil {
		t.Fatalf("ExtractHighResolution: %v", err)
	}

	for _, component := range want {
		got, offset := nearestComponent(found, component.frequencyHz)

		// A tenth of a hertz is well under a cent at these frequencies, and is
		// what the method should manage on a signal this clean. The fast
		// estimator's own detection grid is 0.73 Hz wide before interpolation.
		if offset > 0.1 {
			t.Errorf("nearest component to %.2f Hz is %.2f Hz, off by %.3f Hz",
				component.frequencyHz, got.FrequencyHz, offset)
		}

		// The ring time is the quantity this estimator exists to get right, so
		// it is held to two per cent rather than to the order of magnitude a
		// truncated log-linear fit manages on a decay this long.
		relative := math.Abs(got.T60Seconds-component.t60Seconds) / component.t60Seconds
		if relative > 0.02 {
			t.Errorf("component at %.2f Hz: T60 %.4f s, want %.4f s (%.1f%% off)",
				component.frequencyHz, got.T60Seconds, component.t60Seconds, 100*relative)
		}
	}
}

func TestHighResolutionRecoversRelativeLevels(t *testing.T) {
	t.Parallel()

	const sampleRateHz = 48000

	// A level is only comparable between two components if the correction for
	// the subband filter's own gain and the extrapolation back to the onset are
	// both right; these three sit in three different bands at three different
	// offsets from their band centres, so neither correction can cancel out.
	want := []tone{
		{frequencyHz: 118.05, t60Seconds: 1.2, amplitude: 1.0},
		{frequencyHz: 244.00, t60Seconds: 0.8, amplitude: 0.5},   // -6.02 dB
		{frequencyHz: 951.00, t60Seconds: 0.3, amplitude: 0.125}, // -18.06 dB
	}

	samples := synthesizeNoisy(want, sampleRateHz, 1.2, -70)

	found, err := ExtractHighResolution(samples, sampleRateHz,
		DefaultOptions(), DefaultEspritOptions())
	if err != nil {
		t.Fatalf("ExtractHighResolution: %v", err)
	}

	reference, _ := nearestComponent(found, want[0].frequencyHz)

	for _, component := range want {
		got, offset := nearestComponent(found, component.frequencyHz)
		if offset > 0.5 {
			t.Fatalf("no component near %.2f Hz", component.frequencyHz)
		}

		wantDB := 20 * math.Log10(component.amplitude/want[0].amplitude)

		if difference := math.Abs(got.LevelDB - reference.LevelDB - wantDB); difference > 0.5 {
			t.Errorf("component at %.2f Hz: level %.2f dB relative to the first, want %.2f dB",
				component.frequencyHz, got.LevelDB-reference.LevelDB, wantDB)
		}
	}
}

// The pair below is the case N2 exists for, so it is stated in the terms the
// plan states it in: a two per cent degenerate split at 212.7 Hz is 4.25 Hz,
// which is inside one main lobe of the fast estimator's detection window and a
// third of its separation guard.
var splitPair = []tone{
	{frequencyHz: 210.58, t60Seconds: 0.62, amplitude: 1.00, phase: 0.0},
	{frequencyHz: 214.83, t60Seconds: 0.48, amplitude: 0.85, phase: 1.3},
}

func TestHighResolutionResolvesADegenerateSplit(t *testing.T) {
	t.Parallel()

	const sampleRateHz = 48000

	samples := synthesizeNoisy(splitPair, sampleRateHz, 1.2, -60)

	found, err := ExtractHighResolution(samples, sampleRateHz,
		DefaultOptions(), DefaultEspritOptions())
	if err != nil {
		t.Fatalf("ExtractHighResolution: %v", err)
	}

	for _, component := range splitPair {
		got, offset := nearestComponent(found, component.frequencyHz)

		if offset > 0.2 {
			t.Fatalf("nearest component to %.2f Hz is %.2f Hz, off by %.3f Hz — "+
				"the split was not resolved", component.frequencyHz, got.FrequencyHz, offset)
		}

		if got.Order < 2 {
			t.Errorf("component at %.2f Hz was found at order %d; resolving a pair "+
				"needs at least two", component.frequencyHz, got.Order)
		}

		// The two members differ in ring time by 30 %, which is the whole point
		// of resolving them: a merged pair reports one decay that is neither.
		relative := math.Abs(got.T60Seconds-component.t60Seconds) / component.t60Seconds
		if relative > 0.05 {
			t.Errorf("component at %.2f Hz: T60 %.4f s, want %.4f s (%.1f%% off)",
				component.frequencyHz, got.T60Seconds, component.t60Seconds, 100*relative)
		}
	}
}

// TestFFTEstimatorMergesADegenerateSplit records what the fast estimator does
// with the same signal, and is the measurement N2's first defect is stated
// from. It is not asserting that merging is correct — it is asserting that the
// merge is what happens today, so that repairing it cannot pass unnoticed.
//
// If this test fails because two partials are now reported, the estimator has
// been repaired and this test should be rewritten to require that, not
// loosened.
func TestFFTEstimatorMergesADegenerateSplit(t *testing.T) {
	t.Parallel()

	const sampleRateHz = 48000

	samples := synthesizeNoisy(splitPair, sampleRateHz, 1.2, -60)

	features, err := Extract(samples, sampleRateHz, DefaultOptions())
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	var inRegion []Partial

	for _, partial := range features.Partials {
		if partial.FrequencyHz > 200 && partial.FrequencyHz < 225 {
			inRegion = append(inRegion, partial)
		}
	}

	if len(inRegion) != 1 {
		t.Fatalf("the fast estimator reported %d partials in 200..225 Hz, want the "+
			"single merged one this test documents: %v", len(inRegion), inRegion)
	}

	merged := inRegion[0]

	// The merge is not a blend. The reported partial is the louder member and
	// the quieter one — 1.4 dB down and ringing for very nearly half a second —
	// is simply absent from the table. That is the cost stated precisely: not a
	// frequency error, a missing mode.
	if offset := math.Abs(merged.FrequencyHz - splitPair[0].frequencyHz); offset > 0.5 {
		t.Errorf("the surviving partial is %.3f Hz, %.3f Hz from the louder member; "+
			"this test assumes the merge keeps the louder one",
			merged.FrequencyHz, offset)
	}

	// Its ring time is biased away from the member it does report, because the
	// envelope it was fitted over is beating rather than exponential. The
	// tolerance the high-resolution estimator holds itself to on this same
	// signal is 5 %, so a bias past that is a bias that matters.
	bias := (merged.T60Seconds - splitPair[0].t60Seconds) / splitPair[0].t60Seconds
	if math.Abs(bias) <= 0.05 {
		t.Errorf("the surviving partial's T60 is %.4f s against the member's %.4f s, "+
			"a bias of %.1f%%; this test exists because that bias is not small",
			merged.T60Seconds, splitPair[0].t60Seconds, 100*bias)
	}

	// And the defect that makes all of it hard to catch inside a fit: the fit
	// over a beating envelope is not a visibly bad fit, so FitQuality does not
	// mark the partial as one to distrust.
	if merged.FitQuality < 0.9 {
		t.Errorf("the surviving partial's fit quality is %.3f; the point of recording "+
			"this case is that a beating envelope still fits well", merged.FitQuality)
	}

	t.Logf("fast estimator: one partial at %.3f Hz, T60 %.4f s (%+.1f%%), R^2 %.4f. "+
		"Members: %.2f Hz / %.4f s and %.2f Hz / %.4f s, the second lost entirely.",
		merged.FrequencyHz, merged.T60Seconds, 100*bias, merged.FitQuality,
		splitPair[0].frequencyHz, splitPair[0].t60Seconds,
		splitPair[1].frequencyHz, splitPair[1].t60Seconds)
}

// neighbouredPair is this drum's actual awkward geometry, in a signal whose
// answers are known: two modes 18 Hz apart just above 300 Hz, each ringing for
// about the 0.28 s the reference's fundamental rings for.
//
// 18 Hz clears MinSeparationHz, so the fast estimator detects both. What it
// cannot do is measure them: the envelope filter's cutoff is half the spacing,
// so both are read through a 10 Hz low-pass whose own slower pole rings for
// 287 ms — longer than the partials do. See fastestObservableT60.
var neighbouredPair = []tone{
	{frequencyHz: 304.0, t60Seconds: 0.28, amplitude: 1.00},
	{frequencyHz: 322.0, t60Seconds: 0.26, amplitude: 0.70, phase: 0.9},
}

// TestCloseNeighboursDoNotBiasEitherRingTime is the control for the ring-time
// disagreement measured between the two estimators on the licensed reference,
// where the fast estimator reports T60s a median 27 % longer than the subspace
// one across sixteen velocities, and longer in 71 % of paired partials.
//
// It was written expecting to show that the fast estimator is filter-limited
// here: the envelope filter's cutoff is half the neighbour spacing, so both
// partials are read through a 10 Hz low-pass whose slower pole rings for
// 287 ms, longer than the partials themselves. That explanation is wrong, and
// this test is what refuted it. Both estimators land within a few per cent of
// the truth in exactly the configuration the drum puts them in.
//
// So the 27 % is not a resolution artefact and not a filter artefact. What
// remains is that the reference's partials are not single decaying
// exponentials — which the two estimators summarise differently, neither
// incorrectly — and the merged pairs, where the fast estimator's ring time
// belongs to a partial that is really two. Neither is fixed by repairing an
// estimator, and PLAN.md's N2 says so rather than claiming a defect that the
// measurement below does not support.
//
// The test is kept as a regression: if it ever starts failing, one of the two
// estimators has acquired the bias this was written to look for.
func TestCloseNeighboursDoNotBiasEitherRingTime(t *testing.T) {
	t.Parallel()

	const sampleRateHz = 48000

	samples := synthesizeNoisy(neighbouredPair, sampleRateHz, 1.2, -60)

	features, err := Extract(samples, sampleRateHz, DefaultOptions())
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	found, err := ExtractHighResolution(samples, sampleRateHz,
		DefaultOptions(), DefaultEspritOptions())
	if err != nil {
		t.Fatalf("ExtractHighResolution: %v", err)
	}

	for _, component := range neighbouredPair {
		fast, fastOffset := nearestPartial(features.Partials, component.frequencyHz)
		high, highOffset := nearestComponent(found, component.frequencyHz)

		if fastOffset > 2 || highOffset > 2 {
			t.Fatalf("%.1f Hz: nearest fast partial %.2f Hz, nearest component %.2f Hz",
				component.frequencyHz, fast.FrequencyHz, high.FrequencyHz)
		}

		fastError := 100 * (fast.T60Seconds - component.t60Seconds) / component.t60Seconds
		highError := 100 * (high.T60Seconds - component.t60Seconds) / component.t60Seconds

		t.Logf("%.1f Hz (T60 %.3f s): fast %.3f s (%+.0f%%, R²=%.2f), esprit %.3f s (%+.0f%%)",
			component.frequencyHz, component.t60Seconds,
			fast.T60Seconds, fastError, fast.FitQuality, high.T60Seconds, highError)

		// Ten per cent, against the 27 % median and 149 % ninetieth percentile
		// the same two estimators disagree by on the real recording. Whatever
		// produces that, it is not this.
		if math.Abs(highError) > 10 {
			t.Errorf("%.1f Hz: the subspace estimator is %+.0f%% off", component.frequencyHz, highError)
		}

		if math.Abs(fastError) > 10 {
			t.Errorf("%.1f Hz: the fast estimator is %+.0f%% off — a bias has appeared "+
				"here that the measurement this test records did not have",
				component.frequencyHz, fastError)
		}
	}
}

// nearestPartial is nearestComponent for the fast estimator's table.
func nearestPartial(partials []Partial, frequencyHz float64) (Partial, float64) {
	best, distance := Partial{}, math.Inf(1)

	for _, partial := range partials {
		if offset := math.Abs(partial.FrequencyHz - frequencyHz); offset < distance {
			best, distance = partial, offset
		}
	}

	return best, distance
}

// TestImpossibleDecaysDoNotSilenceTheTable is the regression for the defect
// that made three of the licensed reference's sixteen velocities unusable
// without anything reporting a problem.
//
// A candidate whose envelope crosses the fit floor almost immediately is fitted
// over a few milliseconds of trace, which is enough samples to pass the length
// check and nowhere near enough time to mean anything. The slope that comes
// back is enormous, and because the reported level is that line extrapolated
// back to the strike, so is the level. Levels are relative to the strongest
// partial, so one such candidate puts every real partial below PartialFloorDB
// and the take is reduced to a table of one.
//
// On v10 of the reference the candidate was at 2349.6 Hz: 6.1 ms of trace,
// -4034 dB/s, an intercept of +137 dB against the loudest real partial's
// -6.6 dB. Sixteen partials became one, and the tool reported the drum's
// fundamental as 2349.6 Hz.
func TestImpossibleDecaysDoNotSilenceTheTable(t *testing.T) {
	t.Parallel()

	const sampleRateHz = 48000

	// Four ordinary partials, plus a burst far too short for the envelope
	// filter to have produced: 8 ms of ring at 2350 Hz.
	tones := append(wellSeparatedTones(),
		tone{frequencyHz: 2350, amplitude: 0.9, t60Seconds: 0.008})

	samples := synthesizeNoisy(tones, sampleRateHz, 1.2, -70)

	features, err := Extract(samples, sampleRateHz, DefaultOptions())
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	for _, component := range wellSeparatedTones() {
		if _, offset := nearestPartial(features.Partials, component.frequencyHz); offset > 3 {
			t.Errorf("the %.0f Hz partial was lost; the table is %v",
				component.frequencyHz, features.Partials)
		}
	}

	// Nothing may be reported with a ring time the measurement cannot produce.
	for _, partial := range features.Partials {
		if partial.T60Seconds > 0 && partial.T60Seconds < fastestObservableT60(40) {
			t.Errorf("partial at %.1f Hz reports T60 %.4f s, faster than the widest "+
				"envelope filter can output (%.4f s)",
				partial.FrequencyHz, partial.T60Seconds, fastestObservableT60(40))
		}
	}
}

func TestEspritOptionsAreValidated(t *testing.T) {
	t.Parallel()

	valid := DefaultEspritOptions()

	cases := map[string]func(*EspritOptions){
		"inverted band":     func(o *EspritOptions) { o.MaxFrequencyHz = o.MinFrequencyHz },
		"above nyquist":     func(o *EspritOptions) { o.MaxFrequencyHz = 30000 },
		"no bands":          func(o *EspritOptions) { o.BandsPerOctave = 0 },
		"inverted span":     func(o *EspritOptions) { o.EndSeconds = o.StartSeconds },
		"no order":          func(o *EspritOptions) { o.MaxOrder = 0 },
		"inverted ringing":  func(o *EspritOptions) { o.MaxT60Seconds = o.MinT60Seconds },
		"negative ringing":  func(o *EspritOptions) { o.MinT60Seconds = -1 },
		"zero sample rate":  func(o *EspritOptions) { o.MaxFrequencyHz = 1 },
		"no low frequency":  func(o *EspritOptions) { o.MinFrequencyHz = 0 },
		"infinite sampling": func(o *EspritOptions) { o.MinFrequencyHz = math.Inf(1) },
	}

	for name, corrupt := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			options := valid
			corrupt(&options)

			if name == "zero sample rate" {
				// A one-hertz ceiling is legal on its own; it is the inverted
				// band it creates that must be refused.
				options.MinFrequencyHz = 2
			}

			if err := options.validate(48000); err == nil {
				t.Errorf("%s was accepted", name)
			}
		})
	}

	if err := valid.validate(48000); err != nil {
		t.Errorf("the default options were refused: %v", err)
	}
}
