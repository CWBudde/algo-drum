package physical

import (
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

func TestConfigRejectsNonFiniteValue(t *testing.T) {
	t.Parallel()

	config := DefaultPhysicalDrum()
	config.Batter.TensionNPerM = math.NaN()

	err := config.Validate()
	if !errors.Is(err, ErrInvalidConfig) {
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
