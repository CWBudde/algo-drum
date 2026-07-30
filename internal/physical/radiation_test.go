package physical

import (
	"math"
	"testing"
)

// onAxisConfig puts the microphone on the head's axis and switches the
// near-field term off, which isolates the far-field weight.
func onAxisConfig() PhysicalDrum {
	config := DefaultPhysicalDrum()
	config.Pickup.Radius01 = 0
	config.Pickup.AngleRad = 0
	config.Pickup.NearFieldScale = 0

	return config
}

// TestOnAxisFarFieldWeightIsTheSweptArea pins the reduction that makes the
// radiating moment the same object as the cavity's swept area rather than a
// second, differently normalized guess at it.
func TestOnAxisFarFieldWeightIsTheSweptArea(t *testing.T) {
	t.Parallel()

	config := onAxisConfig()
	modes, err := GenerateModes(config)
	if err != nil {
		t.Fatal(err)
	}

	distanceGain := 1 / (1 + config.Pickup.DistanceM/config.Batter.RadiusM)
	axisymmetric := 0
	for _, mode := range modes {
		if mode.AzimuthalOrder > 0 {
			// Every non-axisymmetric mode has an exactly cancelling far field on
			// axis. Not "small": zero.
			if mode.RadiatingMomentM2 != 0 || mode.RadiationWeight != 0 {
				t.Fatalf(
					"on-axis (%d,%d)%s radiates: moment %v, weight %v",
					mode.AzimuthalOrder,
					mode.RadialOrder,
					mode.Orientation,
					mode.RadiatingMomentM2,
					mode.RadiationWeight,
				)
			}

			continue
		}

		axisymmetric++
		if difference := math.Abs(
			mode.RadiatingMomentM2 - mode.SweptAreaM2,
		); difference > math.Abs(mode.SweptAreaM2)*1e-12 {
			t.Fatalf(
				"(0,%d) moment %v differs from swept area %v by %v",
				mode.RadialOrder,
				mode.RadiatingMomentM2,
				mode.SweptAreaM2,
				difference,
			)
		}

		want := mode.SweptAreaM2 * distanceGain
		if difference := math.Abs(
			mode.RadiationWeight - want,
		); difference > math.Abs(want)*1e-12 {
			t.Fatalf(
				"(0,%d) weight %v differs from swept area times distance gain %v",
				mode.RadialOrder,
				mode.RadiationWeight,
				want,
			)
		}
	}
	if axisymmetric == 0 {
		t.Fatal("no axisymmetric modes were retained")
	}
}

// TestAxisymmetricRadiationWeightIsFrequencyIndependent is the assertion that
// would have caught the error this model was nearly built with.
//
// Far-field pressure from a compact source is proportional to volume
// acceleration with no further frequency dependence. The weight must therefore
// contain no factor of omega at all. The previous model summed modal velocity
// weighted by an efficiency that is proportional to ka at low ka, which supplies
// that factor implicitly — so moving to acceleration while keeping the efficiency
// would count it twice, roughly +10 dB across the retained band, and the fitted
// output gain would have hidden the mistake.
//
// Quadrupling the tension doubles every frequency without touching a Bessel zero
// or a radius, so any surviving frequency dependence shows up here.
func TestAxisymmetricRadiationWeightIsFrequencyIndependent(t *testing.T) {
	t.Parallel()

	config := onAxisConfig()
	modes, err := GenerateModes(config)
	if err != nil {
		t.Fatal(err)
	}

	raised := config
	raised.Batter.TensionNPerM *= 4
	raisedModes, err := GenerateModes(raised)
	if err != nil {
		t.Fatal(err)
	}

	byMode := make(map[[2]int]float64, len(raisedModes))
	for _, mode := range raisedModes {
		if mode.AzimuthalOrder == 0 {
			byMode[[2]int{mode.AzimuthalOrder, mode.RadialOrder}] = mode.RadiationWeight
		}
	}

	compared := 0
	for _, mode := range modes {
		if mode.AzimuthalOrder != 0 {
			continue
		}

		key := [2]int{mode.AzimuthalOrder, mode.RadialOrder}
		raisedWeight, ok := byMode[key]
		if !ok {
			continue
		}

		compared++
		if mode.FrequencyHz*2 < raisedModes[0].FrequencyHz {
			t.Fatalf("tension change did not raise (0,%d)", mode.RadialOrder)
		}
		if difference := math.Abs(
			raisedWeight - mode.RadiationWeight,
		); difference > math.Abs(mode.RadiationWeight)*1e-12 {
			t.Fatalf(
				"(0,%d) weight moved with frequency: %v at %.2f Hz against %v at %.2f Hz",
				mode.RadialOrder,
				mode.RadiationWeight,
				mode.FrequencyHz,
				raisedWeight,
				2*mode.FrequencyHz,
			)
		}
	}
	if compared < 2 {
		t.Fatalf("compared only %d axisymmetric modes, want at least 2", compared)
	}
}

// TestFarFieldMomentMatchesTheClosedForm checks the m > 0 normalization against
// the Lommel result independently of how buildMode assembles it, because the
// multipole factor is the part it is easy to be quietly wrong about: folding it
// into a power of the radiation efficiency drops 1/(2^m m!), which is 1.03e7 at
// the highest retained azimuthal order.
func TestFarFieldMomentMatchesTheClosedForm(t *testing.T) {
	t.Parallel()

	config := DefaultPhysicalDrum()
	config.Pickup.NearFieldScale = 0
	modes, err := GenerateModes(config)
	if err != nil {
		t.Fatal(err)
	}

	radius := config.Batter.RadiusM
	observationSine := observationPolarSine(config.Pickup, radius)
	checked := 0
	for _, mode := range modes {
		if mode.AzimuthalOrder == 0 {
			continue
		}

		checked++
		trace := mode.AngularFrequency * radius /
			config.Cavity.SoundSpeedMPerS * observationSine
		zero := mode.BesselZero
		want := 2 * math.Pi * radius * radius * zero *
			math.Jn(mode.AzimuthalOrder+1, zero) *
			math.Jn(mode.AzimuthalOrder, trace) /
			(zero*zero - trace*trace)
		if difference := math.Abs(
			mode.RadiatingMomentM2 - want,
		); difference > math.Abs(want)*1e-12 {
			t.Fatalf(
				"(%d,%d)%s moment %v differs from the closed form %v",
				mode.AzimuthalOrder,
				mode.RadialOrder,
				mode.Orientation,
				mode.RadiatingMomentM2,
				want,
			)
		}
	}
	if checked == 0 {
		t.Fatal("no non-axisymmetric modes were retained")
	}
}

// TestNearFieldTermCarriesTheNonAxisymmetricModes records why the near-field
// term exists at all, in numbers, so it cannot be removed as a redundant fitted
// coefficient the way the plan for this change nearly omitted it.
//
// With the far-field weight alone a 12-inch head below 600 Hz is so nearly a
// monopole that the whole (1,1) family is inaudible, which is right for a distant
// microphone and wrong for the close one a tom is actually recorded with.
func TestNearFieldTermCarriesTheNonAxisymmetricModes(t *testing.T) {
	t.Parallel()

	firstOvertoneLevel := func(scale float64) float64 {
		config := DefaultPhysicalDrum()
		config.Pickup.NearFieldScale = scale
		modes, err := GenerateModes(config)
		if err != nil {
			t.Fatal(err)
		}

		fundamental := 0.0
		overtone := 0.0
		for _, mode := range modes {
			switch {
			case mode.AzimuthalOrder == 0 && mode.RadialOrder == 1:
				fundamental = math.Abs(mode.RadiationWeight)
			case mode.AzimuthalOrder == 1 && mode.RadialOrder == 1 &&
				mode.Orientation == OrientationCosine:
				overtone = math.Abs(mode.RadiationWeight)
			}
		}
		if fundamental == 0 || overtone == 0 {
			t.Fatal("expected both a (0,1) and a (1,1) cosine mode")
		}

		return 20 * math.Log10(overtone/fundamental)
	}

	farFieldOnly := firstOvertoneLevel(0)
	shipped := firstOvertoneLevel(DefaultPhysicalDrum().Pickup.NearFieldScale)
	t.Logf(
		"(1,1) relative to (0,1): far field alone %.1f dB, shipped %.1f dB",
		farFieldOnly,
		shipped,
	)

	if farFieldOnly > -20 {
		t.Fatalf(
			"far-field-only (1,1) is %.1f dB down, expected well below -20 dB; "+
				"if this no longer holds the near-field term may be redundant",
			farFieldOnly,
		)
	}
	if shipped < -12 {
		t.Fatalf(
			"shipped (1,1) is %.1f dB down, want within 12 dB of the fundamental",
			shipped,
		)
	}
}

// TestPickupRadiusMovesTheNonAxisymmetricBalance covers the control the
// microphone radius became. It used to multiply every mode's far-field weight by
// a near-field point mode shape, which nulled modes arbitrarily as it moved;
// now it sets the observation direction and the near-field pickup point.
func TestPickupRadiusMovesTheNonAxisymmetricBalance(t *testing.T) {
	t.Parallel()

	weightOf := func(radius01 float64, azimuthal, radial int) float64 {
		config := DefaultPhysicalDrum()
		config.Pickup.Radius01 = radius01
		config.Pickup.AngleRad = 0
		modes, err := GenerateModes(config)
		if err != nil {
			t.Fatal(err)
		}

		for _, mode := range modes {
			if mode.AzimuthalOrder == azimuthal &&
				mode.RadialOrder == radial &&
				mode.Orientation == OrientationCosine {
				return math.Abs(mode.RadiationWeight)
			}
		}
		t.Fatalf("mode (%d,%d) was not retained", azimuthal, radial)

		return 0
	}

	if onAxis := weightOf(0, 1, 1); onAxis != 0 {
		t.Fatalf("(1,1) weight on axis = %v, want exactly 0", onAxis)
	}

	// Not monotone across the whole head, and it should not be: the near-field
	// term follows the mode shape, and J1 has its maximum at 1.84, which for the
	// (1,1) is 0.48 of the radius. So it rises off the axis, peaks inside the
	// head, and falls again toward the clamped rim.
	inner := weightOf(0.2, 1, 1)
	peak := weightOf(0.48, 1, 1)
	outer := weightOf(0.9, 1, 1)
	t.Logf("(1,1) weight: 0.2 -> %.4g, 0.48 -> %.4g, 0.9 -> %.4g", inner, peak, outer)

	if inner <= 0 || outer <= 0 {
		t.Fatalf("(1,1) weight vanished off axis: %v at 0.2, %v at 0.9", inner, outer)
	}
	if peak <= inner || peak <= outer {
		t.Fatalf(
			"(1,1) weight %v at the mode shape's maximum is not above %v at 0.2 and %v at 0.9",
			peak,
			inner,
			outer,
		)
	}

	// The axisymmetric fundamental is not silenced anywhere, so moving the
	// microphone changes the balance rather than muting the drum.
	for _, radius01 := range []float64{0, 0.4, 0.8} {
		if weight := weightOf(radius01, 0, 1); weight <= 0 {
			t.Fatalf("(0,1) weight %v at radius %.1f", weight, radius01)
		}
	}
}
