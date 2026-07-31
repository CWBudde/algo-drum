package physical

import (
	"fmt"
	"math"
	"testing"
)

// TestAxisymmetricResonantHeadIsBitExact is the whole justification for
// Head.AxisymmetricOnly, and the guard that keeps it true.
//
// Nothing can excite a resonant mode the enclosed air cannot reach. The strike
// force is added only where the mode index is below batterModeCount, and the
// cavity — the only path between the two heads — couples through an overlap
// integral whose azimuthal factor is exactly zero unless the head mode's
// azimuthal order matches a cavity mode's. So those states never leave zero,
// contribute exactly zero strain to the tension law and exactly zero stored
// energy, and removing them changes the output not approximately but not at all.
//
// The reachable set depends on the cavity, so this runs at both ends of it: with
// one uniform state, where it is m = 0 alone and the field means literally what
// it is named, and at the shipped transverse basis, where it widens to the orders
// the transverse modes carry. The reduction has to be exact in both — the filter
// widening with the cavity is what keeps it so.
//
// If a later change gives an unreachable mode any route to move — direct
// head-to-head coupling, a shell mode, resonant-side excitation — this test fails
// instead of the model quietly losing most of its resonant head.
//
// The reduced head passes through a *second* reduction after this one:
// PhysicalDrum.ResonantModeLimit truncates what is left. That one is a plain
// approximation and is not claimed to be exact, so it is held out of the way here
// — its own effect is measured in TestResonantModeLimitTruncatesTheReachableBank
// and in DefaultResonantModeLimit's note. Holding it out of the way is what lets
// this test go on asserting the same thing it always did, at both ends of the
// cavity, rather than only where the two happen not to overlap.
func TestAxisymmetricResonantHeadIsBitExact(t *testing.T) {
	t.Parallel()

	for _, cavityModeCount := range []int{1, DefaultPhysicalDrum().Cavity.ModeCount} {
		t.Run(fmt.Sprintf("cavityModes=%d", cavityModeCount), func(t *testing.T) {
			t.Parallel()
			assertAxisymmetricReductionIsBitExact(t, cavityModeCount)
		})
	}
}

func assertAxisymmetricReductionIsBitExact(t *testing.T, cavityModeCount int) {
	t.Helper()

	const sampleCount = 48_000

	renderWith := func(axisymmetricOnly bool) ([]float64, int) {
		config := DefaultPhysicalDrum()
		config.Cavity.ModeCount = cavityModeCount
		config.ResonantModeLimit = maxResonantModeLimit
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

// TestUnreachableResonantModesNeverMove states the mechanism the bit-exact
// result rests on, separately from the result, so a failure says which of the
// two broke.
//
// "Unreachable" is decided by the cavity, not by the azimuthal order: at the
// shipped basis the resonant (1,1) and (2,1) families *do* move, which is the
// whole of P9/M2, and everything above them still cannot.
func TestUnreachableResonantModesNeverMove(t *testing.T) {
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

	reachable := cavityAzimuthalOrders(config)
	unreachable := make([]int, 0, model.ResonantModeCount())
	coupled := make([]int, 0, model.ResonantModeCount())
	for index := model.batterModeCount; index < len(model.modes); index++ {
		if _, ok := reachable[model.modes[index].AzimuthalOrder]; ok {
			coupled = append(coupled, index)

			continue
		}

		unreachable = append(unreachable, index)
	}
	if len(unreachable) == 0 {
		t.Fatal("no unreachable resonant modes to check")
	}

	reachableMoved := false
	transverseMoved := false
	for range 24_000 {
		model.Tick()
		for _, index := range unreachable {
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
		for _, index := range coupled {
			if math.Abs(model.displacement[index]) == 0 {
				continue
			}

			reachableMoved = true
			if model.modes[index].AzimuthalOrder > 0 {
				transverseMoved = true
			}
		}
	}

	// The complementary half: the reachable resonant modes must be excited, or
	// the heads are not coupled and the test above proves nothing.
	if !reachableMoved {
		t.Fatal("no reachable resonant mode was excited, so the cavity is not coupling")
	}
	if config.Cavity.ModeCount > 1 && !transverseMoved {
		t.Fatal("transverse cavity modes are enabled but no m>0 resonant mode moved")
	}
}

// TestResonantModeLimitTruncatesTheReachableBank pins what the second reduction
// is allowed to be: a frequency-ordered prefix of the reachable bank and nothing
// else.
//
// That is the property retainCavityReachable's note argues for and this one has
// to keep. A budget applied by re-running the selection loop would refill the
// slots it freed from further up the frequency-sorted list, which substitutes
// modes rather than dropping them — a different instrument rather than a smaller
// one. So every retained mode must be the mode the uncapped bank has at the same
// index.
func TestResonantModeLimitTruncatesTheReachableBank(t *testing.T) {
	t.Parallel()

	uncapped := DefaultPhysicalDrum()
	uncapped.ResonantModeLimit = maxResonantModeLimit
	full, err := generateHeadModes(uncapped, uncapped.Resonant)
	if err != nil {
		t.Fatal(err)
	}

	config := DefaultPhysicalDrum()
	capped, err := generateHeadModes(config, config.Resonant)
	if err != nil {
		t.Fatal(err)
	}

	if len(capped) != config.ResonantModeLimit {
		t.Fatalf("capped resonant bank has %d modes, want %d",
			len(capped), config.ResonantModeLimit)
	}
	if len(full) <= len(capped) {
		t.Fatalf("the shipped cavity leaves only %d reachable modes, so the "+
			"budget of %d never binds and this test proves nothing",
			len(full), config.ResonantModeLimit)
	}

	for index, mode := range capped {
		if mode != full[index] {
			t.Fatalf("capped mode %d is (%d,%d)%s at %.2f Hz, uncapped has "+
				"(%d,%d)%s at %.2f Hz: the budget substituted a mode instead of "+
				"dropping one",
				index,
				mode.AzimuthalOrder, mode.RadialOrder, mode.Orientation,
				mode.FrequencyHz,
				full[index].AzimuthalOrder, full[index].RadialOrder,
				full[index].Orientation, full[index].FrequencyHz)
		}
	}
}

// TestResonantModeLimitNeverBindsOnALumpedCavity is the compatibility half of the
// budget, and the reason schema 11 could take the field without a sound change
// for anything that predates it.
//
// Every v1-v10 document migrates to a one-state cavity, whose reachable set is
// {0}. The number of axisymmetric modes in a bank of N slots grows only as about
// (2/pi)*sqrt(N), so it is 4, 6 and 8 at the three tiers — far below the default
// budget, which therefore cannot bind and cannot move a single sample. If a later
// change lowers the default below those counts, this fails rather than silently
// retuning every stored drum.
func TestResonantModeLimitNeverBindsOnALumpedCavity(t *testing.T) {
	t.Parallel()

	for _, quality := range []Quality{QualityDraft, QualityStandard, QualityHigh} {
		t.Run(string(quality), func(t *testing.T) {
			t.Parallel()

			render := func(limit int) []float64 {
				config := DefaultPhysicalDrum()
				config.Quality = quality
				config.Cavity.ModeCount = 1
				config.ResonantModeLimit = limit
				model, err := NewDoubleHead(config)
				if err != nil {
					t.Fatal(err)
				}
				if err := model.Trigger(1); err != nil {
					t.Fatal(err)
				}

				samples := make([]float64, 24_000)
				for index := range samples {
					samples[index] = model.Tick().Radiated
				}
				if model.ResonantModeCount() >= DefaultResonantModeLimit {
					t.Fatalf("lumped cavity left %d resonant modes at %s, which "+
						"the default budget of %d would truncate",
						model.ResonantModeCount(), quality, DefaultResonantModeLimit)
				}

				return samples
			}

			shipped := render(DefaultResonantModeLimit)
			uncapped := render(maxResonantModeLimit)
			for index := range shipped {
				if shipped[index] != uncapped[index] {
					t.Fatalf("sample %d differs: %v capped, %v uncapped",
						index, shipped[index], uncapped[index])
				}
			}
		})
	}
}
