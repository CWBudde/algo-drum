package physical

import (
	"errors"
	"math"
	"reflect"
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

	for _, index := range [][2]int{
		{-1, 1},
		{maxModeOrder + 1, 1},
		{0, 0},
		{0, maxModeOrder + 1},
	} {
		if _, err := BesselZero(index[0], index[1]); !errors.Is(err, ErrInvalidModeIndex) {
			t.Errorf(
				"BesselZero(%d, %d) error = %v, want ErrInvalidModeIndex",
				index[0],
				index[1],
				err,
			)
		}
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

func TestModalLossLawAndCorrection(t *testing.T) {
	t.Parallel()

	config := DefaultPhysicalDrum()
	config.Batter.ModeDecayCorrections = []ModeDecayCorrection{{
		AzimuthalOrder:     0,
		RadialOrder:        1,
		DecayRatePerSecond: -0.25,
	}}
	modes, err := GenerateModes(config)
	if err != nil {
		t.Fatal(err)
	}

	first := modes[0]
	wantStructural := config.Batter.Loss0PerSecond +
		config.Batter.Loss1MPerSecond*first.WavenumberPerM +
		config.Batter.Loss2M2PerSecond*first.WavenumberPerM*first.WavenumberPerM
	if first.StructuralDecayPerSecond != wantStructural {
		t.Fatalf(
			"first structural decay = %v, want three-parameter law %v",
			first.StructuralDecayPerSecond,
			wantStructural,
		)
	}
	if first.DecayCorrectionPerSecond != -0.25 {
		t.Fatalf("first correction = %v, want -0.25", first.DecayCorrectionPerSecond)
	}
	if first.DecayRatePerSecond != first.StructuralDecayPerSecond+
		first.RadiationDecayPerSecond+first.DecayCorrectionPerSecond {
		t.Fatalf("first total decay does not equal separated loss terms: %#v", first)
	}

	last := modes[len(modes)-1]
	if last.StructuralDecayPerSecond <= first.StructuralDecayPerSecond {
		t.Fatalf(
			"frequency-dependent loss did not increase: first=%v last=%v",
			first.StructuralDecayPerSecond,
			last.StructuralDecayPerSecond,
		)
	}
}

func TestModalRadiationWeightsDependOnModeAndMicrophone(t *testing.T) {
	t.Parallel()

	nearConfig := DefaultPhysicalDrum()
	nearConfig.Pickup.DistanceM = 0.05
	farConfig := nearConfig
	farConfig.Pickup.DistanceM = 1

	near, err := GenerateModes(nearConfig)
	if err != nil {
		t.Fatal(err)
	}
	far, err := GenerateModes(farConfig)
	if err != nil {
		t.Fatal(err)
	}

	weightVaries := false
	for index := range near {
		if math.Abs(far[index].RadiationWeight) >=
			math.Abs(near[index].RadiationWeight) {
			t.Fatalf(
				"mode %d far weight %v is not below near weight %v",
				index,
				far[index].RadiationWeight,
				near[index].RadiationWeight,
			)
		}
		if index > 0 &&
			math.Abs(near[index].RadiationWeight) !=
				math.Abs(near[index-1].RadiationWeight) {
			weightVaries = true
		}
	}
	if !weightVaries {
		t.Fatal("all modal radiation weights are identical")
	}
}

func TestModeCorrectionCannotMakeDecayNegative(t *testing.T) {
	t.Parallel()

	config := DefaultPhysicalDrum()
	config.Batter.ModeDecayCorrections = []ModeDecayCorrection{{
		AzimuthalOrder:     0,
		RadialOrder:        1,
		DecayRatePerSecond: -100,
	}}
	if _, err := GenerateModes(config); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("GenerateModes() error = %v, want ErrInvalidConfig", err)
	}
}

func TestGenerateModesOrderingPairsAndNormalization(t *testing.T) {
	t.Parallel()

	config := DefaultPhysicalDrum()
	config.Batter.TensionAsymmetry = TensionAsymmetry{}
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

func TestTensionAsymmetrySplitsAndRotatesDegenerateModes(t *testing.T) {
	t.Parallel()

	idealConfig := DefaultPhysicalDrum()
	idealConfig.Batter.TensionAsymmetry = TensionAsymmetry{}
	idealModes, err := GenerateModes(idealConfig)
	if err != nil {
		t.Fatal(err)
	}

	const (
		splitRatio = 0.01
		axisAngle  = 0.37
	)
	splitConfig := idealConfig
	splitConfig.Batter.TensionAsymmetry = TensionAsymmetry{
		SplitRatio:            splitRatio,
		PrincipalAxisAngleRad: axisAngle,
	}
	splitConfig.Strike.AngleRad = axisAngle
	splitModes, err := GenerateModes(splitConfig)
	if err != nil {
		t.Fatal(err)
	}

	idealAxisymmetric := findMode(t, idealModes, 0, 1, OrientationCosine)
	splitAxisymmetric := findMode(t, splitModes, 0, 1, OrientationCosine)
	if splitAxisymmetric.FrequencyHz != idealAxisymmetric.FrequencyHz {
		t.Fatalf(
			"axisymmetric frequency changed from %v to %v",
			idealAxisymmetric.FrequencyHz,
			splitAxisymmetric.FrequencyHz,
		)
	}

	ideal := findMode(t, idealModes, 1, 1, OrientationCosine)
	low := findMode(t, splitModes, 1, 1, OrientationCosine)
	high := findMode(t, splitModes, 1, 1, OrientationSine)

	if got := (high.FrequencyHz - low.FrequencyHz) / ideal.FrequencyHz; math.Abs(got-splitRatio) > 1e-14 {
		t.Fatalf("relative pair separation = %.15g, want %.15g", got, splitRatio)
	}
	if got := (high.FrequencyHz + low.FrequencyHz) / 2; math.Abs(got-ideal.FrequencyHz) > 1e-12 {
		t.Fatalf("split midpoint = %.15g Hz, ideal %.15g Hz", got, ideal.FrequencyHz)
	}
	if math.Abs(high.StrikeAccelerationPerN) > 1e-14 {
		t.Fatalf("strike on principal axis excited orthogonal mode by %v", high.StrikeAccelerationPerN)
	}
	if low.StrikeAccelerationPerN == 0 {
		t.Fatal("strike on principal axis did not excite aligned mode")
	}

	repeated, err := GenerateModes(splitConfig)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(repeated, splitModes) {
		t.Fatal("asymmetric mode generation is not deterministic")
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

func TestOnlyAxisymmetricModesSweepCavityVolume(t *testing.T) {
	t.Parallel()

	config := DefaultPhysicalDrum()
	modes, err := GenerateModes(config)
	if err != nil {
		t.Fatal(err)
	}

	for _, mode := range modes {
		if mode.AzimuthalOrder != 0 {
			if mode.SweptAreaM2 != 0 {
				t.Errorf(
					"non-axisymmetric mode (%d,%d,%s) swept area = %v",
					mode.AzimuthalOrder,
					mode.RadialOrder,
					mode.Orientation,
					mode.SweptAreaM2,
				)
			}
			continue
		}

		want := 2 * math.Pi * config.Batter.RadiusM *
			config.Batter.RadiusM * math.J1(mode.BesselZero) /
			mode.BesselZero
		if relativeDifference(mode.SweptAreaM2, want) > 1e-14 {
			t.Errorf(
				"axisymmetric mode (%d,%d) swept area = %.15g, analytic %.15g",
				mode.AzimuthalOrder,
				mode.RadialOrder,
				mode.SweptAreaM2,
				want,
			)
		}
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

func findMode(
	t *testing.T,
	modes []Mode,
	azimuthalOrder, radialOrder int,
	orientation Orientation,
) Mode {
	t.Helper()

	for _, mode := range modes {
		if mode.AzimuthalOrder == azimuthalOrder &&
			mode.RadialOrder == radialOrder &&
			mode.Orientation == orientation {
			return mode
		}
	}

	t.Fatalf(
		"mode (%d,%d,%s) not found",
		azimuthalOrder,
		radialOrder,
		orientation,
	)

	return Mode{}
}
