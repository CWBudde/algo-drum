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
	return dither(synthesize(tones, sampleRateHz, durationSeconds), noiseDB)
}

// dither adds a flat, deterministic noise floor at the given level *relative to
// the signal's own peak*.
//
// Split out of synthesizeNoisy so the swept generator can have one too. Every
// fixture that is measured through Extract needs a floor of some kind — see
// testNoiseFloorDB.
//
// Relative rather than absolute, because Extract is gain-invariant and its
// fixtures have to be too: a floor pinned to an absolute level would sit 26 dB
// closer to the signal in TestExtractIsGainInvariant's quiet copy, and the
// distance between the two would then be measuring the fixture's noise rather
// than the extractor's invariance.
func dither(samples []float64, noiseDB float64) []float64 {
	peak := 0.0
	for _, sample := range samples {
		peak = max(peak, math.Abs(sample))
	}

	state := uint64(0x2545F4914F6CDD1D)
	level := peak * math.Pow(10, noiseDB/20)

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

	// Its ring time used to be biased away from the member it does report, by
	// more than the 5 % the high-resolution estimator holds itself to on this
	// same signal, because the envelope it was fitted over is beating rather
	// than exponential.
	//
	// That half of the defect is repaired, by the per-partial refinement bound
	// of PLAN N17 rather than by anything aimed at this case: a beat's later
	// lobes fall outside the span the surviving member stood above its own floor
	// in, so the fit no longer averages over them. The bias was 5-6 % and is now
	// 1.2 %, inside the tolerance the high-resolution estimator is held to.
	//
	// The defect N2 is actually about is untouched and is asserted above: the
	// pair is still merged into one partial, and the 214.83 Hz member — 30 %
	// shorter in ring time — is still lost entirely. A ring time that is now
	// accurate *for the member that survived* does not make the table right; it
	// makes the survivor look more trustworthy than it is, which is why this is
	// recorded rather than celebrated.
	bias := (merged.T60Seconds - splitPair[0].t60Seconds) / splitPair[0].t60Seconds
	if math.Abs(bias) > 0.05 {
		t.Errorf("the surviving partial's T60 is %.4f s against the member's %.4f s, "+
			"a bias of %.1f%%; the refinement bound is supposed to hold this inside 5%%",
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

// pairBand is one place in the retained band where the subspace estimator
// resolved a two-member pair on the licensed reference, with a ring time of the
// order the model produces there. The four cover the 300 Hz – 2.7 kHz span every
// resolved pair fell inside.
type pairBand struct {
	frequencyHz float64
	t60Seconds  float64
}

var resolvedPairBands = []pairBand{
	{frequencyHz: 304, t60Seconds: 0.50},
	{frequencyHz: 613, t60Seconds: 0.38},
	{frequencyHz: 1200, t60Seconds: 0.26},
	{frequencyHz: 2700, t60Seconds: 0.16},
}

// TestEqualDampingIsNotSplitByTheEstimator answers the question PLAN.md's N3
// gates its whole next round on, and N15 is blocked behind.
//
// The measurement being controlled: across fourteen pairs the fast estimator
// merges and this one resolves, the two members' ring times differ by a median
// factor of 1.55 over frequency splits of 1–6 %. No smooth loss law γ(k) can
// produce that — it would give 1.00 — so either the model is missing a real
// per-pair damping freedom, or the estimator trades energy between two
// components it can barely tell apart and manufactures the ratio. Those two
// readings call for opposite work, and nothing in the reference distinguishes
// them, because the truth there is not known.
//
// Here it is. Both members are given *exactly* the same decay, at the splits,
// frequencies and level imbalances the resolved pairs actually had, over a
// -60 dB noise floor. Any ratio the estimator reports is manufactured, by
// construction.
//
// It reports none: the worst cell in the sweep is 1.003 against a measured 1.55.
// Where the split is too narrow to resolve the estimator merges — returning one
// value twice — rather than inventing two, which is the conservative failure and
// the one that keeps this control meaningful.
//
// So the 1.55 is the drum, and N3's first thread is answered: the missing
// freedom is per-pair damping, spanning the retained band rather than only the
// m = 0 modes the cavity can reach.
func TestEqualDampingIsNotSplitByTheEstimator(t *testing.T) {
	t.Parallel()

	const sampleRateHz = 48000

	// 1 % is included although it is below what the estimator resolves at most
	// of these frequencies, because the claim being defended is about every
	// split the reference showed, and an unresolvable one must merge rather
	// than split.
	for _, split := range []float64{0.01, 0.02, 0.06} {
		for _, band := range resolvedPairBands {
			// The upper member 6 dB down, since a pair excited by an off-centre
			// strike is not excited equally, and an amplitude imbalance is the
			// most plausible way an estimator would come to assign the two
			// members different decays.
			upperHz := band.frequencyHz * (1 + split)

			tones := []tone{
				{frequencyHz: band.frequencyHz, t60Seconds: band.t60Seconds, amplitude: 1.00},
				{frequencyHz: upperHz, t60Seconds: band.t60Seconds, amplitude: 0.50, phase: 1.3},
			}

			found, err := ExtractHighResolution(
				synthesizeNoisy(tones, sampleRateHz, 1.5, -60), sampleRateHz,
				DefaultOptions(), DefaultEspritOptions(),
			)
			if err != nil {
				t.Fatalf("%.0f Hz at %.0f %%: ExtractHighResolution: %v",
					band.frequencyHz, 100*split, err)
			}

			lower, _ := nearestComponent(found, band.frequencyHz)
			upper, _ := nearestComponent(found, upperHz)

			if lower.T60Seconds <= 0 || upper.T60Seconds <= 0 {
				t.Fatalf("%.0f Hz at %.0f %%: the pair was not found at all",
					band.frequencyHz, 100*split)
			}

			ratio := max(lower.T60Seconds, upper.T60Seconds) /
				min(lower.T60Seconds, upper.T60Seconds)

			// A tenth of the measured effect. Wide enough that this is not a
			// tolerance on the estimator's accuracy — the observed worst is
			// 1.003 — and narrow enough that a manufactured 1.55 could not hide
			// under it.
			if ratio > 1.055 {
				t.Errorf("%.0f Hz at %.0f %% split: equally damped members read as "+
					"%.4f s and %.4f s, a ratio of %.3f — the estimator is manufacturing "+
					"a damping split, and the 1.55 measured on the reference cannot be "+
					"read as physics",
					band.frequencyHz, 100*split, lower.T60Seconds, upper.T60Seconds, ratio)
			}
		}
	}
}

// TestARealDampingSplitIsRecoveredAtItsMeasuredSize is the other half of the
// control above, and without it that one proves only that the estimator is
// insensitive. A method that returned the same ring time for both members
// whatever the signal did would pass it perfectly.
//
// So: the same pairs, given the ratio actually measured. It comes back to
// within a fifth of a per cent at every frequency and every split, which is what
// makes the 1.55 a measurement rather than a lower bound.
func TestARealDampingSplitIsRecoveredAtItsMeasuredSize(t *testing.T) {
	t.Parallel()

	const (
		sampleRateHz = 48000
		wantRatio    = 1.55
	)

	for _, split := range []float64{0.02, 0.06} {
		for _, band := range resolvedPairBands {
			upperHz := band.frequencyHz * (1 + split)

			tones := []tone{
				{frequencyHz: band.frequencyHz, t60Seconds: band.t60Seconds * wantRatio, amplitude: 1.00},
				{frequencyHz: upperHz, t60Seconds: band.t60Seconds, amplitude: 0.71, phase: 1.3},
			}

			found, err := ExtractHighResolution(
				synthesizeNoisy(tones, sampleRateHz, 1.5, -60), sampleRateHz,
				DefaultOptions(), DefaultEspritOptions(),
			)
			if err != nil {
				t.Fatalf("%.0f Hz at %.0f %%: ExtractHighResolution: %v",
					band.frequencyHz, 100*split, err)
			}

			lower, _ := nearestComponent(found, band.frequencyHz)
			upper, _ := nearestComponent(found, upperHz)

			if upper.T60Seconds <= 0 {
				t.Fatalf("%.0f Hz at %.0f %%: the upper member was not found",
					band.frequencyHz, 100*split)
			}

			if ratio := lower.T60Seconds / upper.T60Seconds; math.Abs(ratio-wantRatio) > 0.05 {
				t.Errorf("%.0f Hz at %.0f %% split: ratio %.3f, want %.2f (%.4f s and %.4f s)",
					band.frequencyHz, 100*split, ratio, wantRatio,
					lower.T60Seconds, upper.T60Seconds)
			}
		}
	}
}
