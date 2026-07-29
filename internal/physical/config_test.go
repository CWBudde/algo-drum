package physical

import (
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"testing"
)

func TestDefaultPhysicalDrumValidates(t *testing.T) {
	t.Parallel()

	if err := DefaultPhysicalDrum().Validate(); err != nil {
		t.Fatalf("default config does not validate: %v", err)
	}
}

func TestConfigRoundTrip(t *testing.T) {
	t.Parallel()

	config := DefaultPhysicalDrum()
	encoded, err := EncodeConfig(config)
	if err != nil {
		t.Fatalf("EncodeConfig() error = %v", err)
	}

	decoded, err := DecodeConfig(encoded)
	if err != nil {
		t.Fatalf("DecodeConfig() error = %v", err)
	}
	if !reflect.DeepEqual(decoded, config) {
		t.Fatalf("round trip mismatch:\ngot  %#v\nwant %#v", decoded, config)
	}
}

func TestConfigRejectsUnsupportedVersion(t *testing.T) {
	t.Parallel()

	config := DefaultPhysicalDrum()
	config.Version++

	err := config.Validate()
	if !errors.Is(err, ErrConfigVersion) {
		t.Fatalf("Validate() error = %v, want ErrConfigVersion", err)
	}
}

func TestDecodeConfigMigratesVersionOne(t *testing.T) {
	t.Parallel()

	legacy := DefaultPhysicalDrum()
	legacy.Version = legacyConfigVersion
	legacy.Batter.RadiationLossPerSecond = 0
	legacy.Resonant.RadiationLossPerSecond = 0
	legacy.Pickup.DistanceM = 0
	legacy.Pickup.HighpassHz = 0
	legacy.Pickup.LowpassHz = 0
	legacy.Pickup.OutputGain = 0.15
	encoded, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}

	decoded, err := DecodeConfig(encoded)
	if err != nil {
		t.Fatalf("DecodeConfig(v1) error = %v", err)
	}
	defaults := DefaultPhysicalDrum()
	if decoded.Version != ConfigVersion ||
		decoded.Batter.RadiationLossPerSecond != defaults.Batter.RadiationLossPerSecond ||
		decoded.Pickup.DistanceM != defaults.Pickup.DistanceM ||
		decoded.Pickup.HighpassHz != defaults.Pickup.HighpassHz ||
		decoded.Pickup.LowpassHz != defaults.Pickup.LowpassHz ||
		decoded.Pickup.OutputGain != defaults.Pickup.OutputGain {
		t.Fatalf("migrated v1 config = %#v", decoded)
	}
}

func TestDecodeConfigMigratesLinearDoubleHeadWithoutChangingSound(t *testing.T) {
	t.Parallel()

	legacy := DefaultPhysicalDrum()
	legacy.Version = linearDoubleHeadConfigVersion
	legacy.Nonlinearity = Nonlinearity{}
	encoded, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}

	decoded, err := DecodeConfig(encoded)
	if err != nil {
		t.Fatalf("DecodeConfig(v2) error = %v", err)
	}
	if decoded.Version != ConfigVersion {
		t.Fatalf("migrated version = %d, want %d", decoded.Version, ConfigVersion)
	}
	if decoded.Nonlinearity.Enabled ||
		decoded.Nonlinearity.BatterTensionCoefficientNPerM3 != 0 ||
		decoded.Nonlinearity.ResonantTensionCoefficientNPerM3 != 0 ||
		decoded.Nonlinearity.MaximumTensionRatio != 0 {
		t.Fatalf("v2 migration enabled nonlinearity: %#v", decoded.Nonlinearity)
	}
}

func TestConfigRejectsNonFiniteValue(t *testing.T) {
	t.Parallel()

	config := DefaultPhysicalDrum()
	config.Batter.TensionNPerM = math.NaN()

	err := config.Validate()
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("Validate() error = %v, want ErrInvalidConfig", err)
	}
}

func TestConfigValidatesDisabledHeadFields(t *testing.T) {
	t.Parallel()

	config := DefaultPhysicalDrum()
	config.Resonant.Enabled = false
	config.Resonant.TensionNPerM = math.NaN()

	err := config.Validate()
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("Validate() error = %v, want ErrInvalidConfig", err)
	}
}

func TestConfigRejectsDuplicateModeCorrection(t *testing.T) {
	t.Parallel()

	config := DefaultPhysicalDrum()
	correction := ModeDecayCorrection{
		AzimuthalOrder:     0,
		RadialOrder:        1,
		DecayRatePerSecond: 0.5,
	}
	config.Batter.ModeDecayCorrections = []ModeDecayCorrection{
		correction,
		correction,
	}

	if err := config.Validate(); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("Validate() error = %v, want ErrInvalidConfig", err)
	}
}

func TestConfigRejectsInvalidMicrophoneBand(t *testing.T) {
	t.Parallel()

	config := DefaultPhysicalDrum()
	config.Pickup.LowpassHz = config.Pickup.HighpassHz - 1

	if err := config.Validate(); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("Validate() error = %v, want ErrInvalidConfig", err)
	}
}

func TestConfigRejectsUnsafeNonlinearTensionBound(t *testing.T) {
	t.Parallel()

	config := DefaultPhysicalDrum()
	config.Nonlinearity.MaximumTensionRatio =
		maximumSafeTensionRatio(config.Batter) + 1e-6

	if err := config.Validate(); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("Validate() error = %v, want ErrInvalidConfig", err)
	}
}

func TestDisabledNonlinearityStillRejectsNonFiniteFields(t *testing.T) {
	t.Parallel()

	config := DefaultPhysicalDrum()
	config.Nonlinearity.Enabled = false
	config.Nonlinearity.BatterTensionCoefficientNPerM3 = math.NaN()

	if err := config.Validate(); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("Validate() error = %v, want ErrInvalidConfig", err)
	}
}

func TestQualityModeLimits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		quality Quality
		want    int
	}{
		{QualityDraft, 24},
		{QualityStandard, 48},
		{QualityHigh, 96},
		{Quality("unknown"), 0},
	}

	for _, test := range tests {
		if got := test.quality.ModeLimit(); got != test.want {
			t.Errorf("%q.ModeLimit() = %d, want %d", test.quality, got, test.want)
		}
	}
}
