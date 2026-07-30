package physical

import (
	"math"
	"testing"
)

// TestAxisymmetricResonantHeadIsBitExact is the whole justification for
// Head.AxisymmetricOnly, and the guard that keeps it true.
//
// Nothing can excite an m > 0 resonant mode. The strike force is added only
// where the mode index is below batterModeCount, and the cavity — the only path
// between the two heads — couples through SweptAreaM2, which is written only for
// m = 0. So those states never leave zero, contribute exactly zero strain to the
// tension law and exactly zero stored energy, and removing them changes the
// output not approximately but not at all.
//
// If a later change gives them any route to move — direct head-to-head coupling,
// a shell mode, resonant-side excitation — this test fails instead of the model
// quietly losing most of its resonant head.
func TestAxisymmetricResonantHeadIsBitExact(t *testing.T) {
	t.Parallel()

	const sampleCount = 48_000

	renderWith := func(axisymmetricOnly bool) ([]float64, int) {
		config := DefaultPhysicalDrum()
		config.Resonant.AxisymmetricOnly = axisymmetricOnly
		model, err := NewDoubleHead(config)
		if err != nil {
			t.Fatal(err)
		}
		if err := model.Trigger(1); err != nil {
			t.Fatal(err)
		}

		samples := make([]float64, sampleCount)
		for index := range samples {
			samples[index] = model.Tick().Radiated
		}

		return samples, model.ResonantModeCount()
	}

	full, fullCount := renderWith(false)
	reduced, reducedCount := renderWith(true)
	t.Logf(
		"resonant oscillators %d -> %d, reclaiming %d",
		fullCount,
		reducedCount,
		fullCount-reducedCount,
	)

	if reducedCount >= fullCount {
		t.Fatalf("resonant mode count did not fall: %d -> %d", fullCount, reducedCount)
	}

	// Compared with ==, not with the bit patterns. The only arithmetic difference
	// is that the accumulators receive fewer additions of exact zero, and x + 0.0
	// equals x for every x — but it maps -0.0 to +0.0, so a fully silent stretch
	// can legitimately differ in the sign of its zero.
	for index := range full {
		if full[index] != reduced[index] {
			t.Fatalf(
				"sample %d differs: %v with all resonant modes, %v with m=0 only",
				index,
				full[index],
				reduced[index],
			)
		}
	}
}

// TestNonAxisymmetricResonantModesNeverMove states the mechanism the bit-exact
// result rests on, separately from the result, so a failure says which of the
// two broke.
func TestNonAxisymmetricResonantModesNeverMove(t *testing.T) {
	t.Parallel()

	config := DefaultPhysicalDrum()
	config.Resonant.AxisymmetricOnly = false
	model, err := NewDoubleHead(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := model.Trigger(1); err != nil {
		t.Fatal(err)
	}

	nonAxisymmetric := make([]int, 0, model.ResonantModeCount())
	for index := model.batterModeCount; index < len(model.modes); index++ {
		if model.modes[index].AzimuthalOrder > 0 {
			nonAxisymmetric = append(nonAxisymmetric, index)
		}
	}
	if len(nonAxisymmetric) == 0 {
		t.Fatal("no non-axisymmetric resonant modes to check")
	}

	axisymmetricMoved := false
	for range 24_000 {
		model.Tick()
		for _, index := range nonAxisymmetric {
			if model.displacement[index] != 0 || model.velocity[index] != 0 {
				mode := model.modes[index]
				t.Fatalf(
					"resonant (%d,%d)%s moved: displacement %v velocity %v",
					mode.AzimuthalOrder,
					mode.RadialOrder,
					mode.Orientation,
					model.displacement[index],
					model.velocity[index],
				)
			}
		}
		for index := model.batterModeCount; index < len(model.modes); index++ {
			if model.modes[index].AzimuthalOrder == 0 &&
				math.Abs(model.displacement[index]) > 0 {
				axisymmetricMoved = true
			}
		}
	}

	// The complementary half: the axisymmetric resonant modes must be excited,
	// or the heads are not coupled and the test above proves nothing.
	if !axisymmetricMoved {
		t.Fatal("no axisymmetric resonant mode was excited, so the cavity is not coupling")
	}
}
