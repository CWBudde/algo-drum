package physical

import (
	"math"
	"testing"

	"github.com/cwbudde/algo-dsp/dsp/filter/biquad"
	"github.com/cwbudde/algo-dsp/dsp/filter/design"
	"github.com/cwbudde/algo-dsp/measure/ir"
)

// The rendered damping shape.
//
// TestDefaultDampingHoldsConstantQ already pins the shape of the *mode table*:
// every retained mode's zeta lands inside a factor of 1.5, which fixes any band
// statistic of gamma arithmetically. Restating that here would look like
// evidence without being any.
//
// What has never been asserted is the claim about the *audio*. Between a mode's
// gamma and the band envelope a listener hears sit the radiation weights and
// strike amplitudes, the two-head coupling and the cavity (the resonant head's
// modes interleave with the batter's), the Berger nonlinearity, the pickup
// filters and the attack skirt. N3 is the standing proof that those are not the
// same quantity: the *fitted* bank's own decay slope came out at -0.21 where its
// gamma table says -1.

const (
	// bandRenderSeconds is 3.4x the slowest band's expected ring, which is what
	// keeps the Schroeder integral off the end of the buffer.
	//
	// The 8 s release bound cannot truncate this: Render loops Tick
	// unconditionally and never consults IsActive. That coupling is not obvious
	// and this note is the only thing standing between it and a future change
	// that quietly shortens every measurement in this file.
	bandRenderSeconds = 2.0

	// bandQualityFactor is the third-octave Q. A third-octave band has a
	// fractional bandwidth of 2^(1/6) - 2^(-1/6), and Q is its reciprocal.
	bandQualityFactor = 4.318

	// bandSections cascades the bandpass, and it is not a refinement — the
	// measurement is wrong without it.
	//
	// One biquad falls at 6 dB/octave outside its band, which is nowhere near
	// enough here: the (1,1) at 238 Hz is the loudest and longest-ringing thing
	// in the render, and at 1.26 kHz a single section leaves it only ~15 dB down.
	// Once that band's own modes have died the leaked fundamental is what is
	// left, so every high band reports the *fundamental's* ring time and the
	// measured slope collapses towards zero. Measured, not reasoned: with one
	// section the shipped bank came out at slope +0.004 against a mode table
	// saying -1, and TestTheBandT60EstimatorMeasuresItsOwnLeakage is the
	// calibration that catches it.
	bandSections = 4

	// The retained span, excluding the (0,1). The correction table deliberately
	// over-damps the fundamental relative to the series, so it is not part of
	// the constant-Q claim and is asserted separately below.
	bandLowHz  = 200.0
	bandHighHz = 1400.0
)

// thirdOctaveCentres returns the ISO third-octave centres inside [low, high].
func thirdOctaveCentres(lowHz, highHz float64) []float64 {
	var centres []float64

	// Anchored on 1 kHz, the way the ISO series is defined, so the bands do not
	// move when the span does.
	for index := -20; index <= 20; index++ {
		centre := 1000 * math.Pow(2, float64(index)/3)
		if centre >= lowHz && centre <= highHz {
			centres = append(centres, centre)
		}
	}

	return centres
}

// bandT60 filters signal into one third-octave band and returns the reverberation
// time the shipped analyser reports for it.
//
// match's decayFloorFit is deliberately not used: N17 records that the
// Karjalainen exponential-plus-noise-floor model is degenerate on a noiseless
// synthetic, and a model render is exactly that.
func bandT60(t *testing.T, signal []float64, centreHz, sampleRateHz float64) float64 {
	t.Helper()

	sections := make([]biquad.Section, bandSections)
	for index := range sections {
		sections[index] = biquad.Section{Coefficients: design.Bandpass(
			centreHz,
			bandQualityFactor,
			sampleRateHz,
		)}
	}

	filtered := make([]float64, len(signal))
	for index, sample := range signal {
		for section := range sections {
			sample = sections[section].ProcessSample(sample)
		}

		filtered[index] = sample
	}

	metrics, err := ir.NewAnalyzer(sampleRateHz).Analyze(filtered)
	if err != nil {
		t.Fatalf("%.0f Hz: %v", centreHz, err)
	}

	return metrics.RT60
}

// logLogSlope fits log10(y) against log10(x) by least squares.
func logLogSlope(x, y []float64) float64 {
	var sumX, sumY, sumXX, sumXY float64

	count := float64(len(x))
	for index := range x {
		logX := math.Log10(x[index])
		logY := math.Log10(y[index])
		sumX += logX
		sumY += logY
		sumXX += logX * logX
		sumXY += logX * logY
	}

	return (count*sumXY - sumX*sumY) / (count*sumXX - sumX*sumX)
}

// renderBandProbe renders one velocity-1 strike of the given configuration.
func renderBandProbe(t *testing.T, config PhysicalDrum) []float64 {
	t.Helper()

	model, err := NewDoubleHead(config)
	if err != nil {
		t.Fatalf("NewDoubleHead: %v", err)
	}

	if err := model.Trigger(1); err != nil {
		t.Fatal(err)
	}

	out := make([]float64, int(bandRenderSeconds*config.SampleRateHz))
	model.Render(out)

	return out
}

// bandSlope measures the band T60s of one render and returns them with the
// log-log slope against centre frequency.
func bandSlope(t *testing.T, config PhysicalDrum) ([]float64, []float64, float64) {
	t.Helper()

	signal := renderBandProbe(t, config)
	centres := thirdOctaveCentres(bandLowHz, bandHighHz)

	times := make([]float64, len(centres))
	for index, centre := range centres {
		times[index] = bandT60(t, signal, centre, config.SampleRateHz)
	}

	return centres, times, logLogSlope(centres, times)
}

// TestTheBandT60EstimatorIsCalibrated measures what the routine above gets
// wrong before anything is asserted with it.
//
// This is component 2 of the tolerance in the test below, and it is measured
// rather than allowed for: a synthetic single exponential at each band centre,
// where the answer is known exactly, and the largest relative error over the
// span is the floor the real measurement is read against.
func TestTheBandT60EstimatorIsCalibrated(t *testing.T) {
	t.Parallel()

	const sampleRateHz = 48_000.0

	worst := 0.0

	for _, centre := range thirdOctaveCentres(bandLowHz, bandHighHz) {
		// A constant-zeta bank would give exactly this, so the synthetic follows
		// the law the measurement is testing for rather than a flat ring time.
		wantT60 := 1.0 * (500 / centre)

		signal := make([]float64, int(bandRenderSeconds*sampleRateHz))
		decay := math.Log(1000) / wantT60
		for index := range signal {
			seconds := float64(index) / sampleRateHz
			signal[index] = math.Exp(-decay*seconds) *
				math.Sin(2*math.Pi*centre*seconds)
		}

		got := bandT60(t, signal, centre, sampleRateHz)
		relative := math.Abs(got-wantT60) / wantT60
		worst = max(worst, relative)

		t.Logf("%7.1f Hz: want %.4f s, got %.4f s (%.2f %%)", centre, wantT60, got, 100*relative)
	}

	t.Logf("estimator floor over the span: %.3f %% relative", 100*worst)

	// Pinned so that a regression in the estimator shows up here rather than as
	// a mysteriously widened tolerance in the measurement below.
	if worst > 0.05 {
		t.Errorf("estimator error %.3f %% exceeds 5 %% on a known exponential", 100*worst)
	}
}

// bandSlopeTolerance is the whole allowance, and it has exactly two parts.
//
// Component 1 is the shape budget the loss law already ships: every retained
// mode's zeta is held inside [targetZetaLowFactor, targetZetaHighFactor], and
// since T60 = ln(1000)/(zeta*omega), zeta drifting by that ratio from one end of
// the measured span to the other tilts the log-log slope by
// log10(ratio)/span-in-decades. Computed here from the same constants
// TestDefaultDampingHoldsConstantQ enforces, so it cannot be hand-tuned: widening
// it means widening the shipped calibration, in the open.
//
// Component 2 is the estimator's own floor, measured in
// TestTheBandT60EstimatorIsCalibrated at 0.04 % relative on a known exponential.
// A relative error of that size at both ends moves the slope by
// 2*log10(1+e)/span, which is under 0.001 — it is included because leaving it
// out would be an assumption, not because it matters.
//
// Nothing else gets an allowance. The radiation weights, the coupling, the
// cavity, the Berger nonlinearity, the pickup filters and the attack skirt are
// what this test exists to measure; budgeting for them in advance would be
// budgeting for the answer.
func bandSlopeTolerance(centres []float64) float64 {
	span := math.Log10(centres[len(centres)-1] / centres[0])
	shape := math.Log10(targetZetaHighFactor/targetZetaLowFactor) / span
	estimator := 2 * math.Log10(1+bandEstimatorFloor) / span

	return shape + estimator
}

// bandEstimatorFloor is the relative error TestTheBandT60EstimatorIsCalibrated
// measures on a known exponential. It is pinned there, and read here.
const bandEstimatorFloor = 0.0005

// TestTheRenderedDampingFallsWithFrequency is the claim itself: the audible band
// envelope, not the mode table, falls as 1/f.
//
// Measured 2026-08-02 on the shipped default at 48 kHz, third-octave bands from
// 250 to 1260 Hz:
//
//	250.0 Hz  0.661 s      630.0 Hz  0.743 s
//	315.0 Hz  0.428 s      793.7 Hz  0.426 s
//	396.9 Hz  0.366 s     1000.0 Hz  0.119 s
//	500.0 Hz  0.291 s     1259.9 Hz  0.107 s
//
// Slope -0.917 against the -1 constant zeta predicts, inside a tolerance of
// about 0.25 that is almost entirely the shipped zeta window.
func TestTheRenderedDampingFallsWithFrequency(t *testing.T) {
	t.Parallel()

	config := DefaultPhysicalDrum()
	config.SampleRateHz = 48_000

	centres, times, slope := bandSlope(t, config)
	for index, centre := range centres {
		t.Logf("%7.1f Hz: T60 %.4f s", centre, times[index])
	}

	tolerance := bandSlopeTolerance(centres)
	t.Logf("rendered log-log slope %.4f, want -1 within %.4f", slope, tolerance)

	if math.Abs(slope+1) > tolerance {
		t.Errorf(
			"rendered band T60 falls as f^%.4f, want f^-1 within %.4f — the "+
				"audible decay law has left the shape the mode table is "+
				"calibrated to. Record the number, do not widen the tolerance",
			slope, tolerance,
		)
	}

	// The coarse, slope-independent version of the same claim, which holds even
	// where the fit does not: the top of the retained span must die far sooner
	// than the bottom. Under the flat law this repository shipped once, it was 1.
	ratio := times[0] / times[len(times)-1]
	t.Logf("lowest/highest band ring-time ratio: %.2f", ratio)

	if ratio < 3 {
		t.Errorf("lowest band rings only %.2fx the highest, want at least 3x", ratio)
	}
}

// TestTheRenderedDampingIsNotMonotoneAndTheCavityIsWhy records what the
// measurement above found and could not assert.
//
// Band T60 is *not* monotone across the span: 630 Hz rings 0.743 s where its
// neighbours at 500 and 794 Hz ring 0.291 s and 0.426 s. This test is the
// diagnosis, pinned so the explanation cannot quietly stop being true.
//
// It is the enclosed air. Disabling the cavity drops that band to 0.225 s and
// takes the whole-span slope from -0.917 to -1.115 — i.e. without it the render
// tracks the head loss law more closely than with it. Disabling the *resonant
// head* instead makes it far worse, 1.494 s, because the air is then driven by
// the batter alone and damped by one head instead of two.
//
// So in the 630-800 Hz region the least-damped element in the instrument is the
// cavity, and it rather than the head loss law sets what a listener hears decay.
// That is a statement about the model, not about the estimator: it survives
// turning off the tension asymmetry (0.764 s), so it is not doublet beating
// stretching a Schroeder integral, which was the first and wrong guess.
//
// Whether the cavity should be that lightly damped is a calibration question
// nobody has asked; cavity.lossPerSecond is a free field with a validated range
// of [0, 10 000] and no measurement behind its default.
func TestTheRenderedDampingIsNotMonotoneAndTheCavityIsWhy(t *testing.T) {
	t.Parallel()

	config := DefaultPhysicalDrum()
	config.SampleRateHz = 48_000

	centres, withCavity, _ := bandSlope(t, config)

	quiet := config
	quiet.Cavity.Enabled = false

	_, withoutCavity, _ := bandSlope(t, quiet)

	// The band the anomaly lives in, located by name rather than by index so a
	// change to the span cannot silently point this at a different band.
	const anomalyHz = 630.0

	index := -1

	for position, centre := range centres {
		if math.Abs(centre-anomalyHz) < 1 {
			index = position
		}
	}

	if index < 0 {
		t.Fatalf("the %g Hz band is no longer in the measured span", anomalyHz)
	}

	t.Logf(
		"%g Hz band: %.4f s with the cavity, %.4f s without",
		anomalyHz, withCavity[index], withoutCavity[index],
	)

	if withCavity[index] <= withoutCavity[index] {
		t.Errorf(
			"the %g Hz band no longer rings longer with the cavity than without "+
				"(%.4f vs %.4f) — the diagnosis in this test's comment has stopped "+
				"being true and the comment must be rewritten, not the assertion",
			anomalyHz, withCavity[index], withoutCavity[index],
		)
	}
}

// TestAUniformlyDampedBankIsRejectedByTheBandMeasurement is the control that
// makes the measurement non-vacuous.
//
// A bank damped at one flat rate satisfies almost every other assertion in this
// package while sounding like a struck sine bank — it is how the flat law
// shipped. If the band measurement could not tell that apart from the real one,
// it would be measuring the filters rather than the drum.
//
// Measured at slope +0.150 against the shipped bank's -0.917, i.e. on the far
// side of -1 by more than four tolerances.
func TestAUniformlyDampedBankIsRejectedByTheBandMeasurement(t *testing.T) {
	t.Parallel()

	config := DefaultPhysicalDrum()
	config.SampleRateHz = 48_000

	// The radiation-weighted mean rate of the shipped bank, so the control has
	// the same overall decay and differs from it only in shape.
	modes, err := generateHeadModes(config, config.Batter)
	if err != nil {
		t.Fatal(err)
	}

	var weighted, weight float64

	for index := range modes {
		w := modes[index].RadiationWeight * modes[index].RadiationWeight
		weighted += w * modes[index].DecayRatePerSecond
		weight += w
	}

	setUniformLoss(&config.Batter, weighted/weight)
	setUniformLoss(&config.Resonant, weighted/weight)

	centres, times, slope := bandSlope(t, config)
	for index, centre := range centres {
		t.Logf("%7.1f Hz: T60 %.4f s", centre, times[index])
	}

	tolerance := bandSlopeTolerance(centres)
	t.Logf("uniformly damped slope %.4f, tolerance %.4f", slope, tolerance)

	if math.Abs(slope+1) <= tolerance {
		t.Errorf(
			"a uniformly damped bank measured slope %.4f, inside the tolerance "+
				"the real bank is judged by — this measurement cannot tell the "+
				"shape of the loss law and asserts nothing",
			slope,
		)
	}
}

// TestTheAttackLayerDoesNotSetTheRenderedDampingSlope is the second control.
//
// The attack layer is three bands of shaped noise decaying at rates read off the
// head's own loss law, so it could in principle be producing the 1/f the modal
// bank is being credited with. It is not: removing it moves the slope from
// -0.917 to -0.828.
//
// That difference is worth naming rather than waving through. It is 0.089, which
// is 180x the estimator's floor — so the layer is *not* neutral to this
// measurement, it simply is not the source of the trend. The agreement asserted
// is therefore against the shape budget, which is what a mechanism inside the
// instrument is allowed to move, and not against the estimator floor, which is
// what only a measurement artefact would stay inside.
func TestTheAttackLayerDoesNotSetTheRenderedDampingSlope(t *testing.T) {
	t.Parallel()

	config := DefaultPhysicalDrum()
	config.SampleRateHz = 48_000

	centres, _, withAttack := bandSlope(t, config)

	quiet := config
	quiet.Attack.Enabled = false

	_, _, withoutAttack := bandSlope(t, quiet)

	tolerance := bandSlopeTolerance(centres)
	t.Logf(
		"slope %.4f with the attack layer, %.4f without (difference %.4f, tolerance %.4f)",
		withAttack, withoutAttack, math.Abs(withAttack-withoutAttack), tolerance,
	)

	if math.Abs(withAttack-withoutAttack) > tolerance {
		t.Errorf(
			"disabling the attack layer moved the slope by %.4f — the trend this "+
				"file credits to the modal loss law is coming from the noise layer",
			math.Abs(withAttack-withoutAttack),
		)
	}

	if math.Abs(withoutAttack+1) > tolerance {
		t.Errorf(
			"without the attack layer the modal bank alone renders slope %.4f, "+
				"want -1 within %.4f",
			withoutAttack, tolerance,
		)
	}
}
