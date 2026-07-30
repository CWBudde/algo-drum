package physical

import (
	"math"
	"testing"
)

// TestRetuningHoldsConstantQ is the property the tuning knob was missing.
//
// Before RetuneTension existed, the loss coefficients were absolute constants
// quoted at one tension, so moving B.TUNE moved ζ with it: 2.20 % at the bottom
// of its travel against 0.72 % at the top, which stretched a 300 Hz partial's
// T60 from 0.166 s to 0.423 s. Turning the drum up therefore made it ring
// longer as well as higher, and "it only sounds good at high B.TUNE" was partly
// a report about that.
func TestRetuningHoldsConstantQ(t *testing.T) {
	t.Parallel()

	reference := DefaultPhysicalDrum()
	targetZeta := reference.Batter.Loss1MPerSecond /
		WaveSpeedMPerS(reference.Batter)

	for _, tension := range []float64{300, 600, 1250, 2000, 3500} {
		config := DefaultPhysicalDrum()
		config.Resonant.Enabled = false
		RetuneTension(&config.Batter, tension)

		modes, err := GenerateModes(config)
		if err != nil {
			t.Fatal(err)
		}

		for _, mode := range modes {
			// The (0,1) carries the coupling correction and is deliberately
			// outside the constant-Q band; it scales with the tuning too, which
			// the T60 check below covers.
			if mode.AzimuthalOrder == 0 && mode.RadialOrder == 1 {
				continue
			}

			zeta := mode.DecayRatePerSecond / mode.AngularFrequency
			if zeta < targetZeta*0.9 || zeta > targetZeta*1.35 {
				t.Fatalf(
					"at %.0f N/m, mode (%d,%d) at %.1f Hz has zeta %.4f, want %.4f",
					tension,
					mode.AzimuthalOrder,
					mode.RadialOrder,
					mode.FrequencyHz,
					zeta,
					targetZeta,
				)
			}
		}
	}
}

// TestRetuningMovesPitchAndNotMuchElse states the same thing the way a player
// would: the knob is a pitch control. T60 still falls as the pitch rises,
// because that is what constant Q means, but it falls in proportion rather than
// by the factor of three the drifting ζ used to add on top.
func TestRetuningMovesPitchAndNotMuchElse(t *testing.T) {
	t.Parallel()

	fundamentalOf := func(tension float64) Mode {
		config := DefaultPhysicalDrum()
		config.Resonant.Enabled = false
		RetuneTension(&config.Batter, tension)

		modes, err := GenerateModes(config)
		if err != nil {
			t.Fatal(err)
		}

		mode, ok := modeByOrder(modes, 0, 1)
		if !ok {
			t.Fatal("no fundamental")
		}

		return mode
	}

	low := fundamentalOf(600)
	high := fundamentalOf(2400)

	// Four times the tension is twice the wave speed is twice the pitch.
	if ratio := high.FrequencyHz / low.FrequencyHz; math.Abs(ratio-2) > 0.01 {
		t.Fatalf("2x pitch expected from 4x tension, got %.4f", ratio)
	}

	// And T60 halves, so the number of cycles it rings for is unchanged.
	lowCycles := low.FrequencyHz / low.DecayRatePerSecond
	highCycles := high.FrequencyHz / high.DecayRatePerSecond
	t.Logf(
		"(0,1) %.1f Hz T60 %.0f ms against %.1f Hz T60 %.0f ms",
		low.FrequencyHz, t60Milliseconds(low.DecayRatePerSecond),
		high.FrequencyHz, t60Milliseconds(high.DecayRatePerSecond),
	)

	if relative := math.Abs(highCycles/lowCycles - 1); relative > 0.02 {
		t.Fatalf(
			"ring length in cycles moved by %.1f%% across two octaves of tuning",
			100*relative,
		)
	}
}

// TestDefaultTuningIsARackTom guards the pitch itself. At the 600 N/m this
// shipped with, the 12-inch batter head's fundamental was 104 Hz — a floor tom,
// and the reason the drum only read correctly near the top of B.TUNE's range.
func TestDefaultTuningIsARackTom(t *testing.T) {
	t.Parallel()

	config := DefaultPhysicalDrum()
	config.Resonant.Enabled = false

	modes, err := GenerateModes(config)
	if err != nil {
		t.Fatal(err)
	}

	fundamental, ok := modeByOrder(modes, 0, 1)
	if !ok {
		t.Fatal("no fundamental")
	}

	t.Logf("default (0,1) at %.2f Hz", fundamental.FrequencyHz)

	if fundamental.FrequencyHz < 135 || fundamental.FrequencyHz > 175 {
		t.Fatalf(
			"default 12-inch fundamental %.2f Hz outside the [135,175] a rack tom sits in",
			fundamental.FrequencyHz,
		)
	}
}
