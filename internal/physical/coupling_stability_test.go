package physical

import (
	"errors"
	"math"
	"testing"
)

// couplingStressConfig is the tier the coefficient ceiling was measured against:
// the quality and pump budget whose divergence threshold came out lowest, at the
// sample rate the engine actually runs at.
func couplingStressConfig() PhysicalDrum {
	config := DefaultPhysicalDrum()
	config.Quality = QualityHigh
	config.Nonlinearity.Coupling.PumpCount = 8
	config.Nonlinearity.Coupling.MaxCoefficients = 4096

	return config
}

func renderCoupled(
	t *testing.T,
	config PhysicalDrum,
	velocity float64,
	coefficientOverrideNPerM float64,
) (*DoubleHead, []float64) {
	t.Helper()

	model, err := NewDoubleHead(config)
	if err != nil {
		t.Fatalf("NewDoubleHead: %v", err)
	}

	if coefficientOverrideNPerM != 0 {
		// Reaching past Validate on purpose. A ceiling in the validator does not
		// help a PhysicalDrum that arrives at Render some other way, and this is
		// the only way to exercise the path that defends against one.
		model.coupling.coefficientNPerM = coefficientOverrideNPerM
	}

	if err := model.Trigger(velocity); err != nil {
		t.Fatalf("Trigger: %v", err)
	}

	buffer := make([]float64, int(config.SampleRateHz))
	model.Render(buffer)

	return model, buffer
}

func firstNonFinite(samples []float64) int {
	for index, sample := range samples {
		if math.IsNaN(sample) || math.IsInf(sample, 0) {
			return index
		}
	}

	return -1
}

// TestCouplingCoefficientCeilingIsEnforced pins the range itself.
//
// The ceiling used to be 1e9, which is above the coefficient at which the
// measured worst tier renders NaN (6.98e8) and above the band in which the render
// degenerates before it does. Both of those are now outside the validated range.
func TestCouplingCoefficientCeilingIsEnforced(t *testing.T) {
	t.Parallel()

	if DefaultNonlinearCoupling().CoefficientNPerM >=
		maxCouplingCoefficientNPerM {
		t.Fatalf(
			"shipped coefficient %v is not below the ceiling %v",
			DefaultNonlinearCoupling().CoefficientNPerM,
			maxCouplingCoefficientNPerM,
		)
	}

	config := DefaultPhysicalDrum()
	config.Nonlinearity.Coupling.CoefficientNPerM = maxCouplingCoefficientNPerM

	if err := config.Validate(); err != nil {
		t.Fatalf("coefficient at the ceiling must validate: %v", err)
	}

	config.Nonlinearity.Coupling.CoefficientNPerM = math.Nextafter(maxCouplingCoefficientNPerM, math.Inf(1))
	if err := config.Validate(); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf(
			"coefficient one ulp past the ceiling must not validate, got %v",
			err,
		)
	}
}

// TestCouplingAtCeilingRendersFinite is the other half of the contract: a
// configuration the validator accepts renders finite, on the tier the ceiling was
// measured against and at the velocity that makes the fixed point work hardest.
func TestCouplingAtCeilingRendersFinite(t *testing.T) {
	t.Parallel()

	config := couplingStressConfig()
	config.Nonlinearity.Coupling.CoefficientNPerM = maxCouplingCoefficientNPerM

	for _, velocity := range []float64{0.25, 0.5, 1} {
		model, buffer := renderCoupled(t, config, velocity, 0)

		if index := firstNonFinite(buffer); index >= 0 {
			t.Fatalf("velocity %v: sample %d is not finite", velocity, index)
		}

		if steps := model.CouplingDivergedSteps(); steps != 0 {
			t.Errorf(
				"velocity %v: the guard fired %d times at the validated ceiling",
				velocity,
				steps,
			)
		}
	}
}

// TestCouplingGuardIsInertOnShippedConfigurations states the property the
// bit-exactness of the shipped voice rests on: on everything that ships, the
// fallback path is never taken, so the arithmetic is the arithmetic that was
// there before it existed.
func TestCouplingGuardIsInertOnShippedConfigurations(t *testing.T) {
	t.Parallel()

	qualities := []Quality{QualityDraft, QualityStandard, QualityHigh}
	rates := []float64{44_100, 48_000, 96_000}

	for _, quality := range qualities {
		for _, rate := range rates {
			for _, velocity := range []float64{0.1, 0.5, 1} {
				config := DefaultPhysicalDrum()
				config.Quality = quality
				config.SampleRateHz = rate

				model, buffer := renderCoupled(t, config, velocity, 0)

				if index := firstNonFinite(buffer); index >= 0 {
					t.Fatalf(
						"%s %.0f Hz velocity %v: sample %d is not finite",
						quality, rate, velocity, index,
					)
				}

				if steps := model.CouplingDivergedSteps(); steps != 0 {
					t.Errorf(
						"%s %.0f Hz velocity %v: the guard fired %d times",
						quality, rate, velocity, steps,
					)
				}
			}
		}
	}
}

// TestCouplingDivergenceFallsBackToTheBergerLaw exercises the failure path.
//
// 7e8 is the coefficient the measurement that opened this reported: on the high
// tier it used to produce 52754 non-finite samples in a one-second render with a
// peak of 3.0e9. It is now outside the validated range, so the only way to reach
// it is the way a caller that skips Validate would — which is exactly the case
// the guard exists for.
func TestCouplingDivergenceFallsBackToTheBergerLaw(t *testing.T) {
	t.Parallel()

	config := couplingStressConfig()

	model, buffer := renderCoupled(t, config, 1, 7e8)

	if index := firstNonFinite(buffer); index >= 0 {
		t.Fatalf("sample %d is not finite; the guard did not hold", index)
	}

	if model.CouplingDivergedSteps() == 0 {
		t.Fatal("the diverging coefficient did not trip the guard")
	}

	// The fallback is the Berger-only update, whose tension is tanh-capped, so
	// the render stays inside the range a hard hit reaches rather than merely
	// being non-NaN.
	peak := 0.0
	for _, sample := range buffer {
		peak = max(peak, math.Abs(sample))
	}

	if peak > 1e6 {
		t.Fatalf("peak %g: the fallback did not bound the state", peak)
	}

	// Reset clears the count, so a diagnostic reading is per-voice and not
	// cumulative across the life of the process.
	model.Reset()

	if steps := model.CouplingDivergedSteps(); steps != 0 {
		t.Fatalf("Reset left %d diverged steps behind", steps)
	}
}

// TestCouplingKnobExtremesRenderFinite covers the rest of the coupling and
// nonlinearity ranges at their validated limits. None of them admits a diverging
// render, which is why only the coefficient's ceiling moved.
func TestCouplingKnobExtremesRenderFinite(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		apply func(PhysicalDrum) PhysicalDrum
	}{
		{"pumpCount at the cap", func(config PhysicalDrum) PhysicalDrum {
			config.Nonlinearity.Coupling.PumpCount = maxCouplingPumps

			return config
		}},
		{"maxCoefficients at the cap", func(config PhysicalDrum) PhysicalDrum {
			config.Nonlinearity.Coupling.MaxCoefficients = maxCouplingCoefficients

			return config
		}},
		{"aliasFraction at the cap", func(config PhysicalDrum) PhysicalDrum {
			config.Nonlinearity.Coupling.AliasFraction = 0.5

			return config
		}},
		{"tension coefficients at the cap", func(config PhysicalDrum) PhysicalDrum {
			config.Nonlinearity.BatterTensionCoefficientNPerM3 = 1e9
			config.Nonlinearity.ResonantTensionCoefficientNPerM3 = 1e9

			return config
		}},
		{"everything at once", func(config PhysicalDrum) PhysicalDrum {
			config.Nonlinearity.BatterTensionCoefficientNPerM3 = 1e9
			config.Nonlinearity.ResonantTensionCoefficientNPerM3 = 1e9
			config.Nonlinearity.MaximumTensionRatio = 0.23
			config.Nonlinearity.Coupling.PumpCount = maxCouplingPumps
			config.Nonlinearity.Coupling.MaxCoefficients = maxCouplingCoefficients
			config.Nonlinearity.Coupling.CoefficientNPerM = maxCouplingCoefficientNPerM

			return config
		}},
	}

	for _, testCase := range cases {
		config := testCase.apply(DefaultPhysicalDrum())
		if err := config.Validate(); err != nil {
			t.Fatalf("%s: %v", testCase.name, err)
		}

		model, buffer := renderCoupled(t, config, 1, 0)

		if index := firstNonFinite(buffer); index >= 0 {
			t.Errorf("%s: sample %d is not finite", testCase.name, index)
		}

		if steps := model.CouplingDivergedSteps(); steps != 0 {
			t.Errorf("%s: the guard fired %d times", testCase.name, steps)
		}
	}
}
