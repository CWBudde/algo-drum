package physical

import (
	"math"
	"testing"
)

func TestBesselZeroKnownValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		order       int
		radialIndex int
		want        float64
	}{
		{0, 1, 2.404825557695773},
		{0, 2, 5.520078110286311},
		{1, 1, 3.8317059702075125},
		{2, 1, 5.135622301840683},
	}

	for _, test := range tests {
		got, err := BesselZero(test.order, test.radialIndex)
		if err != nil {
			t.Fatalf("BesselZero(%d, %d) error = %v", test.order, test.radialIndex, err)
		}
		if difference := math.Abs(got - test.want); difference > 1e-12 {
			t.Errorf(
				"BesselZero(%d, %d) = %.15g, want %.15g (difference %.3g)",
				test.order,
				test.radialIndex,
				got,
				test.want,
				difference,
			)
		}
	}
}

func TestBesselZeroRejectsInvalidIndex(t *testing.T) {
	t.Parallel()

	if _, err := BesselZero(-1, 1); err == nil {
		t.Fatal("BesselZero(-1, 1) succeeded, want error")
	}
	if _, err := BesselZero(0, 0); err == nil {
		t.Fatal("BesselZero(0, 0) succeeded, want error")
	}
}

func TestNaturalFrequencyMembraneRatio(t *testing.T) {
	t.Parallel()

	head := DefaultPhysicalDrum().Batter
	head.BendingStiffnessNM = 0
	firstZero, err := BesselZero(0, 1)
	if err != nil {
		t.Fatal(err)
	}
	secondZero, err := BesselZero(1, 1)
	if err != nil {
		t.Fatal(err)
	}

	got := NaturalFrequencyHz(head, secondZero) / NaturalFrequencyHz(head, firstZero)
	want := secondZero / firstZero
	if difference := math.Abs(got - want); difference > 1e-13 {
		t.Fatalf("frequency ratio = %.15g, want %.15g", got, want)
	}
}

func TestGenerateModesOrderingPairsAndNormalization(t *testing.T) {
	t.Parallel()

	config := DefaultPhysicalDrum()
	modes, err := GenerateModes(config)
	if err != nil {
		t.Fatalf("GenerateModes() error = %v", err)
	}
	if len(modes) != config.Quality.ModeLimit() {
		t.Fatalf("mode count = %d, want %d", len(modes), config.Quality.ModeLimit())
	}

	frequencyLimit := config.SampleRateHz * config.Batter.FrequencyLimitFraction
	for index, mode := range modes {
		if !isFinite(mode.FrequencyHz) || mode.FrequencyHz <= 0 ||
			mode.FrequencyHz > frequencyLimit {
			t.Errorf("mode %d frequency = %v, want finite value in (0, %v]", index, mode.FrequencyHz, frequencyLimit)
		}
		if !isFinite(mode.ModalMassKg) || mode.ModalMassKg <= 0 {
			t.Errorf("mode %d mass = %v, want positive finite value", index, mode.ModalMassKg)
		}
		if index > 0 && modes[index-1].FrequencyHz > mode.FrequencyHz {
			t.Errorf("mode %d frequency %v follows %v", index, mode.FrequencyHz, modes[index-1].FrequencyHz)
		}

		expectedAngularIntegral := math.Pi
		if mode.AzimuthalOrder == 0 {
			expectedAngularIntegral = 2 * math.Pi
		}
		boundarySlope := math.Jn(mode.AzimuthalOrder+1, mode.BesselZero)
		expectedMass := config.Batter.SurfaceDensityKgPerM2 *
			expectedAngularIntegral * config.Batter.RadiusM * config.Batter.RadiusM *
			boundarySlope * boundarySlope / 2
		if relativeDifference(mode.ModalMassKg, expectedMass) > 1e-14 {
			t.Errorf("mode %d mass = %.15g, analytic normalization = %.15g", index, mode.ModalMassKg, expectedMass)
		}

		if mode.AzimuthalOrder == 0 {
			if mode.Orientation != OrientationCosine {
				t.Errorf("axisymmetric mode %d orientation = %s, want cos", index, mode.Orientation)
			}
			continue
		}
		if mode.Orientation != OrientationCosine {
			continue
		}
		if index+1 >= len(modes) {
			t.Fatalf("cosine mode %d lacks sine partner", index)
		}
		partner := modes[index+1]
		if partner.AzimuthalOrder != mode.AzimuthalOrder ||
			partner.RadialOrder != mode.RadialOrder ||
			partner.Orientation != OrientationSine ||
			partner.FrequencyHz != mode.FrequencyHz ||
			partner.ModalMassKg != mode.ModalMassKg {
			t.Errorf("mode %d cosine/sine pair mismatch: %#v and %#v", index, mode, partner)
		}
	}
}

func TestCenterStrikeSelectsAxisymmetricModes(t *testing.T) {
	t.Parallel()

	config := DefaultPhysicalDrum()
	config.Strike.Radius01 = 0
	modes, err := GenerateModes(config)
	if err != nil {
		t.Fatalf("GenerateModes() error = %v", err)
	}

	axisymmetricModes := 0
	for _, mode := range modes {
		if mode.AzimuthalOrder == 0 {
			axisymmetricModes++
			if mode.StrikeAccelerationPerN == 0 {
				t.Errorf("axisymmetric mode (%d,%d) has zero center-strike coupling", mode.AzimuthalOrder, mode.RadialOrder)
			}
			continue
		}
		if math.Abs(mode.StrikeAccelerationPerN) > 1e-14 {
			t.Errorf(
				"non-axisymmetric mode (%d,%d,%s) center-strike coupling = %v, want zero",
				mode.AzimuthalOrder,
				mode.RadialOrder,
				mode.Orientation,
				mode.StrikeAccelerationPerN,
			)
		}
	}
	if axisymmetricModes == 0 {
		t.Fatal("generated basis contains no axisymmetric modes")
	}
}

func TestCircularFootprintLimitAndAttenuation(t *testing.T) {
	t.Parallel()

	if got := circularFootprint(0); got != 1 {
		t.Fatalf("circularFootprint(0) = %v, want 1", got)
	}
	if low, high := math.Abs(circularFootprint(0.5)), math.Abs(circularFootprint(3)); high >= low {
		t.Fatalf("footprint attenuation did not increase: |H(0.5)|=%v, |H(3)|=%v", low, high)
	}
}

func isFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func relativeDifference(left, right float64) float64 {
	scale := math.Max(math.Abs(left), math.Abs(right))
	if scale == 0 {
		return 0
	}

	return math.Abs(left-right) / scale
}
