package physical

import (
	"math"
	"testing"

	algofft "github.com/cwbudde/algo-fft"
)

func TestNonlinearTensionProducesVelocityDependentPitchGlide(t *testing.T) {
	t.Parallel()

	quietEarly, quietLate := nonlinearFirstModeFrequencies(t, 0.2)
	loudEarly, loudLate := nonlinearFirstModeFrequencies(t, 1)
	t.Logf(
		"first-mode frequency: quiet %.3f -> %.3f Hz, loud %.3f -> %.3f Hz",
		quietEarly,
		quietLate,
		loudEarly,
		loudLate,
	)

	// In cents, because that is the unit the property is stated in: a glide has
	// to be heard as a bend, and the ratio test below passes at 38 cents, which
	// is not. 60 is a floor with the shipped 102.8 well clear of it; the ceiling
	// is there because the tension cap is at 157 cents and a glide pressed
	// against it stops being velocity-dependent.
	glideCents := 1200 * math.Log2(loudEarly/loudLate)
	t.Logf("loud glide %.1f cents", glideCents)

	if glideCents < 60 || glideCents > 140 {
		t.Fatalf("loud glide %.1f cents outside [60, 140]", glideCents)
	}
	if loudEarly <= loudLate*1.015 {
		t.Fatalf(
			"loud frequency did not glide down enough: %.3f -> %.3f Hz",
			loudEarly,
			loudLate,
		)
	}
	if loudEarly <= quietEarly*1.01 {
		t.Fatalf(
			"loud attack frequency %.3f Hz not above quiet %.3f Hz",
			loudEarly,
			quietEarly,
		)
	}
	if loudEarly-loudLate <= 2*(quietEarly-quietLate) {
		t.Fatalf(
			"loud glide %.3f Hz not clearly above quiet glide %.3f Hz",
			loudEarly-loudLate,
			quietEarly-quietLate,
		)
	}
}

func TestNonlinearAttackSpectrumBrightensWithVelocityAndAvoidsNyquist(t *testing.T) {
	t.Parallel()

	quietCentroid, quietAlias := nonlinearAttackSpectrum(t, 0.2)
	loudCentroid, loudAlias := nonlinearAttackSpectrum(t, 1)
	t.Logf(
		"overtone centroid: quiet %.1f Hz, loud %.1f Hz; top-band energy %.3g / %.3g",
		quietCentroid,
		loudCentroid,
		quietAlias,
		loudAlias,
	)

	if loudCentroid <= quietCentroid*1.01 {
		t.Fatalf(
			"loud overtone centroid %.3f Hz not above quiet %.3f Hz",
			loudCentroid,
			quietCentroid,
		)
	}
	if quietAlias > 1e-5 || loudAlias > 1e-5 {
		t.Fatalf(
			"energy in top 1%% of spectrum = quiet %.3g, loud %.3g",
			quietAlias,
			loudAlias,
		)
	}
}

func TestNonlinearUpdateMatchesOversampledReference(t *testing.T) {
	t.Parallel()

	const (
		baseSampleRate = 48_000.0
		oversampling   = 4
		initialMotion  = 0.003
		duration       = 0.08
	)

	makeModel := func(sampleRate float64) *DoubleHead {
		config := isolatedNonlinearConfig()
		config.SampleRateHz = sampleRate
		silenceLosses(&config.Batter)
		model, err := NewDoubleHead(config)
		if err != nil {
			t.Fatal(err)
		}
		model.displacement[0] = initialMotion
		model.batterNonlinear.strainMeasureM2 =
			model.strainWeight[0] * initialMotion * initialMotion

		return model
	}

	base := makeModel(baseSampleRate)
	reference := makeModel(baseSampleRate * oversampling)
	initialEnergy := base.observe().TotalMechanicalEnergyJ
	maximumError := 0.0
	for range int(duration * baseSampleRate) {
		base.Tick()
		for range oversampling {
			reference.Tick()
		}
		maximumError = max(
			maximumError,
			math.Abs(base.displacement[0]-reference.displacement[0]),
		)
	}

	relativeError := maximumError / initialMotion
	energyError := relativeDifference(
		base.observe().TotalMechanicalEnergyJ,
		initialEnergy,
	)
	t.Logf(
		"48/192 kHz trajectory maximum error %.4f%%, energy drift %.3g",
		100*relativeError,
		energyError,
	)
	if relativeError > 0.015 {
		t.Fatalf(
			"48 kHz trajectory differs from 192 kHz reference by %.3f%%",
			100*relativeError,
		)
	}
	if energyError > 2e-10 {
		t.Fatalf("lossless nonlinear energy drift = %.3g", energyError)
	}
}

func TestNonlinearDoubleHeadLosslessEnergyIsConserved(t *testing.T) {
	t.Parallel()

	config := DefaultPhysicalDrum()
	silenceLosses(&config.Batter)
	silenceLosses(&config.Resonant)
	config.Cavity.LossPerSecond = 0
	model, err := NewDoubleHead(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := model.Trigger(1); err != nil {
		t.Fatal(err)
	}

	var referenceEnergy, nonlinearPeak float64
	for sampleIndex := range model.PulseSamples() + 20_000 {
		output := model.Tick()
		nonlinearPeak = max(
			nonlinearPeak,
			output.NonlinearPotentialEnergyJ,
		)
		if sampleIndex == model.PulseSamples()-1 {
			referenceEnergy = output.TotalMechanicalEnergyJ
		}
		if sampleIndex >= model.PulseSamples() {
			if difference := relativeDifference(
				output.TotalMechanicalEnergyJ,
				referenceEnergy,
			); difference > 5e-10 {
				t.Fatalf(
					"sample %d nonlinear energy drift = %.3g",
					sampleIndex,
					difference,
				)
			}
		}
	}
	if nonlinearPeak <= referenceEnergy*1e-5 {
		t.Fatalf(
			"nonlinear potential peak %v is negligible beside total %v",
			nonlinearPeak,
			referenceEnergy,
		)
	}
}

func TestNonlinearMaximumStrengthRemainsFiniteAndBounded(t *testing.T) {
	t.Parallel()

	config := DefaultPhysicalDrum()
	config.Nonlinearity.BatterTensionCoefficientNPerM3 = 1e9
	config.Nonlinearity.ResonantTensionCoefficientNPerM3 = 1e9
	model, err := NewDoubleHead(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := model.Trigger(1); err != nil {
		t.Fatal(err)
	}

	batterMaximum := config.Nonlinearity.MaximumTensionRatio *
		config.Batter.TensionNPerM
	resonantMaximum := config.Nonlinearity.MaximumTensionRatio *
		config.Resonant.TensionNPerM
	for sampleIndex := range int(config.SampleRateHz) {
		output := model.Tick()
		values := [...]float64{
			output.BatterDisplacementM,
			output.BatterVelocityMPerS,
			output.ResonantDisplacementM,
			output.ResonantVelocityMPerS,
			output.RawRadiated,
			output.Radiated,
			output.CavityPressurePa,
			output.TotalMechanicalEnergyJ,
		}
		for _, value := range values {
			if math.IsNaN(value) || math.IsInf(value, 0) {
				t.Fatalf("sample %d produced non-finite output: %#v", sampleIndex, output)
			}
		}
		if output.BatterTensionIncreaseNPerM > batterMaximum ||
			output.ResonantTensionIncreaseNPerM > resonantMaximum {
			t.Fatalf("sample %d exceeded tension cap: %#v", sampleIndex, output)
		}
	}
}

func TestNonlinearMaximumStrengthConservesLosslessEnergy(t *testing.T) {
	t.Parallel()

	config := isolatedNonlinearConfig()
	silenceLosses(&config.Batter)
	config.Nonlinearity.BatterTensionCoefficientNPerM3 = 1e9
	model, err := NewDoubleHead(config)
	if err != nil {
		t.Fatal(err)
	}
	model.displacement[0] = 0.003
	model.batterNonlinear.strainMeasureM2 =
		model.strainWeight[0] * model.displacement[0] * model.displacement[0]
	referenceEnergy := model.observe().TotalMechanicalEnergyJ

	for sampleIndex := range 10_000 {
		energy := model.Tick().TotalMechanicalEnergyJ
		if difference := relativeDifference(
			energy,
			referenceEnergy,
		); difference > 2e-9 {
			t.Fatalf(
				"sample %d maximum-strength energy drift = %.3g",
				sampleIndex,
				difference,
			)
		}
	}
}

func TestNonlinearFrequencyBoundKeepsRetainedModesBelowNyquist(t *testing.T) {
	t.Parallel()

	config := DefaultPhysicalDrum()
	model, err := NewDoubleHead(config)
	if err != nil {
		t.Fatal(err)
	}

	for index, mode := range model.modes {
		head := config.Batter
		if index >= model.batterModeCount {
			head = config.Resonant
		}
		maximumAngularFrequencySquared :=
			mode.AngularFrequency*mode.AngularFrequency +
				config.Nonlinearity.MaximumTensionRatio*
					head.TensionNPerM/head.SurfaceDensityKgPerM2*
					mode.WavenumberPerM*mode.WavenumberPerM
		maximumFrequencyHz := math.Sqrt(maximumAngularFrequencySquared) /
			(2 * math.Pi)
		if maximumFrequencyHz >= config.SampleRateHz/2 {
			t.Fatalf(
				"mode %d maximum frequency %.3f Hz reaches Nyquist %.3f Hz",
				index,
				maximumFrequencyHz,
				config.SampleRateHz/2,
			)
		}
	}
}

func nonlinearFirstModeFrequencies(t *testing.T, velocity float64) (float64, float64) {
	t.Helper()

	return nonlinearFirstModeFrequenciesFor(t, isolatedNonlinearConfig(), velocity)
}

func nonlinearFirstModeFrequenciesFor(
	t *testing.T,
	config PhysicalDrum,
	velocity float64,
) (float64, float64) {
	t.Helper()

	model, err := NewDoubleHead(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := model.Trigger(velocity); err != nil {
		t.Fatal(err)
	}

	crossings := make([]int, 0, 64)
	previous := model.displacement[0]
	for sampleIndex := range int(0.7 * config.SampleRateHz) {
		model.Tick()
		current := model.displacement[0]
		if previous < 0 && current >= 0 {
			crossings = append(crossings, sampleIndex)
		}
		previous = current
	}
	if len(crossings) < 40 {
		t.Fatalf("only %d first-mode crossings", len(crossings))
	}

	averageFrequency := func(firstPeriod, periodCount int) float64 {
		samples := crossings[firstPeriod+periodCount] - crossings[firstPeriod]

		return float64(periodCount) * config.SampleRateHz / float64(samples)
	}

	return averageFrequency(1, 5), averageFrequency(len(crossings)-7, 5)
}

func nonlinearAttackSpectrum(t *testing.T, velocity float64) (float64, float64) {
	t.Helper()

	// 2048 samples, 43 ms: the attack, which is what this measures. Over the
	// 171 ms window this used to take, the Hann taper suppresses exactly the
	// interval where the tension is raised and emphasises the settled middle, so
	// a large glide moves the measured centroid *down* — at the shipped
	// coefficients, loud 363.8 Hz against quiet 371.2 Hz. That was a property of
	// the window, not of the model.
	const fftSize = 2048
	config := isolatedNonlinearConfig()
	setUniformLoss(&config.Batter, 3)
	model, err := NewDoubleHead(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := model.Trigger(velocity); err != nil {
		t.Fatal(err)
	}

	samples := make([]float64, fftSize)
	iterationTotal := 0
	maximumIterations := 0
	for index := range samples {
		output := model.Tick()
		iterationTotal += output.NonlinearSolveIterations
		maximumIterations = max(
			maximumIterations,
			output.NonlinearSolveIterations,
		)
		window := 0.5 - 0.5*math.Cos(
			2*math.Pi*float64(index)/float64(fftSize),
		)
		samples[index] = output.BatterRawRadiated * window
	}

	plan, err := algofft.NewPlanReal64(fftSize)
	if err != nil {
		t.Fatal(err)
	}
	bins := make([]complex128, plan.SpectrumLen())
	if err := plan.Forward(bins, samples); err != nil {
		t.Fatal(err)
	}

	// The centroid is taken above the fundamental, not across the whole band.
	// Tension modulation shifts every partial upward; it does not move energy
	// into the top of the spectrum. A full-band centroid is dominated by the
	// fundamental's *level* instead, and with the corrected microphone model —
	// where a centre hit is almost entirely (0,1) — it barely moves at all:
	// 112.373 Hz quiet against 112.377 Hz loud, which is not a measurement of
	// anything. Above the fundamental the mechanism is unambiguous.
	fundamental, ok := model.BatterMode(0)
	if !ok {
		t.Fatal("batter head has no modes")
	}

	overtoneFloorHz := 1.4 * fundamental.FrequencyHz
	fullPower := 0.0
	overtonePower := 0.0
	weightedFrequency := 0.0
	topBandPower := 0.0
	for index, bin := range bins {
		power := real(bin)*real(bin) + imag(bin)*imag(bin)
		frequency := float64(index) * config.SampleRateHz / fftSize
		fullPower += power
		if frequency >= overtoneFloorHz {
			overtonePower += power
			weightedFrequency += frequency * power
		}
		if frequency >= 0.99*config.SampleRateHz/2 {
			topBandPower += power
		}
	}
	if fullPower == 0 || overtonePower == 0 {
		t.Fatal("zero attack spectrum")
	}
	t.Logf(
		"velocity %.2f nonlinear solve: mean %.2f, maximum %d iterations",
		velocity,
		float64(iterationTotal)/fftSize,
		maximumIterations,
	)

	return weightedFrequency / overtonePower, topBandPower / fullPower
}

func isolatedNonlinearConfig() PhysicalDrum {
	config := DefaultPhysicalDrum()
	config.Quality = QualityDraft
	config.Cavity.Enabled = false
	config.Resonant.Enabled = false
	// One flat rate: this configuration measures the nonlinear glide, so the
	// calibrated frequency-dependent law would only shorten the window the
	// first mode stays measurable in.
	setUniformLoss(&config.Batter, 8)
	config.Strike.Radius01 = 0
	config.Strike.AngleRad = 0

	return config
}
