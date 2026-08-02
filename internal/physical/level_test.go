package physical

import (
	"math"
	"testing"
)

// peakRadiated renders two seconds of a velocity-1 strike and returns the
// largest absolute sample of the microphone signal — the same quantity
// internal/drum's TestPhysicalTomReachesProductLevelWithoutACompensatingGain
// measures, i.e. what reaches the master chain before the mix headroom, the
// reverb and the limiter.
func peakRadiated(t *testing.T, config PhysicalDrum) float64 {
	t.Helper()

	model, err := NewDoubleHead(config)
	if err != nil {
		t.Fatalf("NewDoubleHead: %v", err)
	}

	if err := model.Trigger(1); err != nil {
		t.Fatal(err)
	}

	peak := 0.0
	for range int(2 * config.SampleRateHz) {
		peak = max(peak, math.Abs(model.Tick().Radiated))
	}

	return peak
}

// renderRadiated is peakRadiated's sibling for the cases that need the whole
// signal rather than its peak.
func renderRadiated(t *testing.T, config PhysicalDrum) []float64 {
	t.Helper()

	model, err := NewDoubleHead(config)
	if err != nil {
		t.Fatalf("NewDoubleHead: %v", err)
	}

	if err := model.Trigger(1); err != nil {
		t.Fatal(err)
	}

	out := make([]float64, int(2*config.SampleRateHz))
	model.Render(out)

	return out
}

// TestValidatedLevelCeilingsAreMeasuredBounds is what stops the two level
// ceilings drifting back into taste.
//
// Both used to be round numbers with no measurement behind them — 1 000 and 100,
// about four orders of magnitude above anything reachable. Each is now derived
// from a specific quantity, and this test re-derives that quantity and fails if
// the ceiling no longer matches it. Without this the derivation would be a
// comment, which is to say it would survive exactly until the calibration next
// moved.
func TestValidatedLevelCeilingsAreMeasuredBounds(t *testing.T) {
	t.Parallel()

	config := DefaultPhysicalDrum()
	config.SampleRateHz = 48_000

	// The attack layer's contribution is isolated by difference rather than by
	// turning the level up until it dominates, which the ceiling now forbids —
	// the measurement would have to leave the range it is measuring. The layer
	// is a pure addition to the radiated sum and feeds nothing back into the
	// modal state, so rendering with and without it and subtracting gives the
	// layer's own signal exactly. Linearity in LevelRelative is asserted rather
	// than assumed, since the whole derivation rests on it.
	silent := config
	silent.Attack.Enabled = false
	withoutAttack := renderRadiated(t, silent)

	perUnit := 0.0
	for _, level := range []float64{attackLevelRelativeCeiling / 4, attackLevelRelativeCeiling} {
		probe := config
		probe.Attack.LevelRelative = level

		withAttack := renderRadiated(t, probe)

		peak := 0.0
		for index := range withAttack {
			peak = max(peak, math.Abs(withAttack[index]-withoutAttack[index]))
		}

		measured := peak / level
		t.Logf("attack layer contributes %.4f per unit of levelRelative at %.4f", measured, level)

		if perUnit != 0 && math.Abs(measured-perUnit)/perUnit > 1e-6 {
			t.Fatalf(
				"the attack layer is not linear in levelRelative: %.6f per unit "+
					"against %.6f — the derivation below assumes it is",
				measured, perUnit,
			)
		}

		perUnit = measured
	}

	zeroDBFSLevel := 1 / perUnit

	t.Logf(
		"attack layer reaches 0 dBFS on its own at levelRelative %.4f; ceiling is %.4f",
		zeroDBFSLevel, attackLevelRelativeCeiling,
	)

	// At or just above, never below: below it the ceiling would exclude settings
	// that still carry information, and far above it would be headroom again.
	if attackLevelRelativeCeiling < zeroDBFSLevel {
		t.Errorf(
			"attackLevelRelativeCeiling %.4f is below the level at which the "+
				"attack layer alone reaches 0 dBFS (%.4f), so it excludes settings "+
				"that still change the sound",
			attackLevelRelativeCeiling, zeroDBFSLevel,
		)
	}

	if attackLevelRelativeCeiling > 1.1*zeroDBFSLevel {
		t.Errorf(
			"attackLevelRelativeCeiling %.4f is more than 10%% above the measured "+
				"0 dBFS level %.4f — re-derive it rather than leaving the slack",
			attackLevelRelativeCeiling, zeroDBFSLevel,
		)
	}

	// The output-gain ceiling is 100x the calibrated gain, the factor taken from
	// migrateV7Config's own record that the pre-v8 gain ran "roughly two orders
	// of magnitude hot". Pinned to the calibrated value rather than to today's
	// number, so a recalibration carries the ceiling with it instead of silently
	// changing what the ceiling means.
	if want := 100 * config.Pickup.OutputGain; pickupOutputGainCeiling != want {
		t.Errorf(
			"pickupOutputGainCeiling = %v, want 100x the calibrated gain %v = %v; "+
				"the gain was recalibrated and the ceiling was not carried with it",
			pickupOutputGainCeiling, config.Pickup.OutputGain, want,
		)
	}
}

// TestTheProductReachableLevelIsBounded states what the level actually is
// across the knobs, which until now was measured only at the defaults.
//
// TestPhysicalTomReachesProductLevelWithoutACompensatingGain pins the velocity-1
// default into [0.70, 0.95]. Everywhere else was unmeasured, and "unmeasured"
// was being read as "fine" — the ceilings above were set on that reading.
//
// The bound is generous and deliberately so: this is the signal *before* the
// 0.5 mix headroom, the reverb and the -1 dBFS limiter, so exceeding 0 dBFS here
// is normal and absorbed downstream. What would not be fine is exceeding it by
// an order of magnitude, which is what an unbounded ceiling permits.
func TestTheProductReachableLevelIsBounded(t *testing.T) {
	t.Parallel()

	// Measured worst case over the product-reachable space is +9.05 dBFS, with
	// B.TUNE at its lower stop: a slacker head is louder. +12 dBFS leaves the
	// calibration a factor of about 1.2 to move in before this needs looking at.
	const boundDBFS = 12.0

	config := DefaultPhysicalDrum()
	config.SampleRateHz = 48_000

	// The reasoned level-maximising corner, plus the two ceilings themselves, so
	// the bound is stated where it is loosest rather than at the defaults.
	cases := []struct {
		name  string
		apply func(*PhysicalDrum)
	}{
		{"defaults", func(*PhysicalDrum) {}},
		{"attack.levelRelative=ceiling", func(c *PhysicalDrum) {
			c.Attack.LevelRelative = attackLevelRelativeCeiling
		}},
		{"batter.tensionNPerM=slack", func(c *PhysicalDrum) {
			RetuneTension(&c.Batter, 300)
		}},
		{"slack batter at the attack ceiling", func(c *PhysicalDrum) {
			RetuneTension(&c.Batter, 300)
			c.Attack.LevelRelative = attackLevelRelativeCeiling
		}},
	}

	for _, testCase := range cases {
		probe := config
		testCase.apply(&probe)

		if err := probe.Validate(); err != nil {
			t.Fatalf("%s: %v", testCase.name, err)
		}

		peak := peakRadiated(t, probe)
		dbfs := 20 * math.Log10(peak)

		t.Logf("%-36s peak %.4f (%+.2f dBFS)", testCase.name, peak, dbfs)

		if dbfs > boundDBFS {
			t.Errorf(
				"%s: peak %+.2f dBFS exceeds the measured bound of %+.1f dBFS",
				testCase.name, dbfs, boundDBFS,
			)
		}
	}
}
