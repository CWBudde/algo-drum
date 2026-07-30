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

func TestDecodeConfigMigratesVersionThreeWithFullCavityCoupling(t *testing.T) {
	t.Parallel()

	legacy := DefaultPhysicalDrum()
	legacy.Version = nonlinearConfigVersion
	legacy.Cavity.Coupling01 = 0
	encoded, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}

	decoded, err := DecodeConfig(encoded)
	if err != nil {
		t.Fatalf("DecodeConfig(v3) error = %v", err)
	}
	if decoded.Version != ConfigVersion || decoded.Cavity.Coupling01 != 1 {
		t.Fatalf("migrated v3 cavity = %#v, version %d",
			decoded.Cavity, decoded.Version)
	}
}

func TestDecodeConfigMigratesVersionFourWithoutAddingAsymmetry(t *testing.T) {
	t.Parallel()

	legacy := DefaultPhysicalDrum()
	legacy.Version = fullCouplingConfigVersion
	legacy.Batter.TensionAsymmetry = TensionAsymmetry{}
	legacy.Resonant.TensionAsymmetry = TensionAsymmetry{}
	encoded, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}

	decoded, err := DecodeConfig(encoded)
	if err != nil {
		t.Fatalf("DecodeConfig(v4) error = %v", err)
	}
	if decoded.Version != ConfigVersion ||
		decoded.Batter.TensionAsymmetry != (TensionAsymmetry{}) ||
		decoded.Resonant.TensionAsymmetry != (TensionAsymmetry{}) {
		t.Fatalf("migrated v4 asymmetry = batter %#v, resonant %#v, version %d",
			decoded.Batter.TensionAsymmetry,
			decoded.Resonant.TensionAsymmetry,
			decoded.Version)
	}
}

// TestDecodeConfigMigratesVersionSixToTheRigidCavity covers the one migration in
// the chain whose compatibility value is not the zero value. A version-6
// document has no stiffnessScale at all, and taking the decoded zero at face
// value would silently uncouple the cavity rather than preserve it, so the
// migration has to write the rigid 1 explicitly.
func TestDecodeConfigMigratesVersionSixToTheRigidCavity(t *testing.T) {
	t.Parallel()

	legacy := DefaultPhysicalDrum()
	legacy.Version = tiltedDampingConfigVersion
	encoded, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	// Version 6 had no such field; drop it the way a real stored document would.
	var document map[string]any
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatal(err)
	}
	cavity, ok := document["cavity"].(map[string]any)
	if !ok {
		t.Fatalf("cavity is not an object: %#v", document["cavity"])
	}
	delete(cavity, "stiffnessScale")
	encoded, err = json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}

	decoded, err := DecodeConfig(encoded)
	if err != nil {
		t.Fatalf("DecodeConfig(v6) error = %v", err)
	}
	if decoded.Version != ConfigVersion || decoded.Cavity.StiffnessScale != 1 {
		t.Fatalf("migrated v6 cavity = %#v, version %d",
			decoded.Cavity, decoded.Version)
	}
}

// TestDecodeConfigMigratesVersionEightKeepsItsTuning covers the half of the v8
// migration that is a deliberate *non*-migration.
//
// Version 9 moved the default tuning from 600 N/m to 1250 and requoted the loss
// coefficients against the new wave speed. A stored document's tension and losses
// are its own measured content, though, so migrating them would retune somebody's
// drum; only the attack layer, whose absolute release has no image in the derived
// rates, is rewritten.
func TestDecodeConfigMigratesVersionEightKeepsItsTuning(t *testing.T) {
	t.Parallel()

	legacy := DefaultPhysicalDrum()
	legacy.Version = radiatedAccelerationVersion
	legacy.Batter.TensionNPerM = 600
	legacy.Batter.Loss1MPerSecond = 0.4554
	encoded, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	// Version 8 had decaySeconds where version 9 has decayScale, so a real
	// document carries neither field the new schema reads.
	var document map[string]any
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatal(err)
	}
	attack, ok := document["attack"].(map[string]any)
	if !ok {
		t.Fatalf("attack is not an object: %#v", document["attack"])
	}
	delete(attack, "decayScale")
	attack["decaySeconds"] = 0.02
	encoded, err = json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}

	decoded, err := DecodeConfig(encoded)
	if err != nil {
		t.Fatalf("DecodeConfig(v8) error = %v", err)
	}
	if decoded.Version != ConfigVersion {
		t.Fatalf("migrated v8 version = %d, want %d", decoded.Version, ConfigVersion)
	}
	if decoded.Attack.DecayScale != DefaultPhysicalDrum().Attack.DecayScale {
		t.Fatalf("migrated v8 attack = %#v", decoded.Attack)
	}
	if decoded.Batter.TensionNPerM != 600 || decoded.Batter.Loss1MPerSecond != 0.4554 {
		t.Fatalf("migration retuned a stored head: %#v", decoded.Batter)
	}
}

func TestConfigRejectsInvalidCavityCoupling(t *testing.T) {
	t.Parallel()

	config := DefaultPhysicalDrum()
	config.Cavity.Coupling01 = 1.01

	if err := config.Validate(); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("Validate() error = %v, want ErrInvalidConfig", err)
	}
}

func TestConfigRejectsInvalidTensionAsymmetry(t *testing.T) {
	t.Parallel()

	for name, mutate := range map[string]func(*PhysicalDrum){
		"split above bound": func(config *PhysicalDrum) {
			config.Batter.TensionAsymmetry.SplitRatio = 0.021
		},
		"negative split": func(config *PhysicalDrum) {
			config.Resonant.TensionAsymmetry.SplitRatio = -0.001
		},
		"non-finite axis": func(config *PhysicalDrum) {
			config.Batter.TensionAsymmetry.PrincipalAxisAngleRad = math.NaN()
		},
		"axis above pi": func(config *PhysicalDrum) {
			config.Resonant.TensionAsymmetry.PrincipalAxisAngleRad = math.Pi + 0.01
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			config := DefaultPhysicalDrum()
			mutate(&config)

			if err := config.Validate(); !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("Validate() error = %v, want ErrInvalidConfig", err)
			}
		})
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

	// The tiers doubled once the resonant head stopped spending a full bank on
	// modes nothing can excite. Zero for an unknown tier is not a fourth data
	// point but a contract: Validate uses ModeLimit() == 0 as its is-this-a-known
	// -quality test, so a tier that returned a real budget for a bogus name would
	// make every configuration validate.
	tests := []struct {
		quality Quality
		want    int
	}{
		{QualityDraft, 48},
		{QualityStandard, 96},
		{QualityHigh, 160},
		{Quality("unknown"), 0},
	}

	for _, test := range tests {
		if got := test.quality.ModeLimit(); got != test.want {
			t.Errorf("%q.ModeLimit() = %d, want %d", test.quality, got, test.want)
		}
	}
}

func TestDecodeConfigMigratesVersionNineToThePrescribedContact(t *testing.T) {
	t.Parallel()

	legacy := DefaultPhysicalDrum()
	legacy.Version = multiBandAttackConfigVersion
	legacy.Strike.Contact = Contact{}
	encoded, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	// Version 9 had no contact block at all, so a real document carries none.
	var document map[string]any
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatal(err)
	}
	strike, ok := document["strike"].(map[string]any)
	if !ok {
		t.Fatalf("strike is not an object: %#v", document["strike"])
	}
	delete(strike, "contact")
	encoded, err = json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}

	decoded, err := DecodeConfig(encoded)
	if err != nil {
		t.Fatalf("DecodeConfig(v9) error = %v", err)
	}
	if decoded.Version != ConfigVersion {
		t.Fatalf("migrated v9 version = %d, want %d", decoded.Version, ConfigVersion)
	}
	// Version 9 had only the half-sine, so naming it reproduces the sound exactly.
	if decoded.Strike.Contact.Model != ContactPrescribed {
		t.Errorf("migrated v9 contact model = %q, want %q",
			decoded.Strike.Contact.Model, ContactPrescribed)
	}
	// The Hertzian coefficients come along inert, so the model can be switched
	// without the document first failing validation.
	if decoded.Strike.Contact.StiffnessNPerMAlpha != DefaultContact().StiffnessNPerMAlpha ||
		decoded.Strike.Contact.Exponent != DefaultContact().Exponent {
		t.Errorf("migrated v9 contact = %#v", decoded.Strike.Contact)
	}
}

func TestConfigRejectsInvalidContact(t *testing.T) {
	t.Parallel()

	cases := map[string]func(*Contact){
		"unknown model":       func(c *Contact) { c.Model = "hertz" },
		"empty model":         func(c *Contact) { c.Model = "" },
		"zero stiffness":      func(c *Contact) { c.StiffnessNPerMAlpha = 0 },
		"linear exponent":     func(c *Contact) { c.Exponent = 0.9 },
		"clipping hysteresis": func(c *Contact) { c.HysteresisSPerM = 1.5 },
		"unbounded window":    func(c *Contact) { c.MaxDurationSeconds = 0 },
	}

	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			config := DefaultPhysicalDrum()
			mutate(&config.Strike.Contact)

			if err := config.Validate(); err == nil {
				t.Errorf("Validate() accepted %s: %#v", name, config.Strike.Contact)
			}
		})
	}
}
