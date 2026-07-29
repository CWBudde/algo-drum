// Package physical implements reduced physical models of acoustic drums.
//
// The package is deliberately independent of the sequencer and the existing
// procedural voices. Physical parameters use SI units; UI mapping and
// persistence integration belong at the application boundary.
package physical

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
)

const (
	legacyConfigVersion           = 1
	linearDoubleHeadConfigVersion = 2
	// ConfigVersion is the physical-drum JSON schema emitted by EncodeConfig.
	ConfigVersion = 3

	minSampleRateHz = 8_000.0
	maxSampleRateHz = 384_000.0
)

var (
	// ErrConfigVersion reports an unsupported persisted configuration version.
	ErrConfigVersion = errors.New("unsupported physical drum config version")
	// ErrInvalidConfig reports a non-finite or out-of-range physical parameter.
	ErrInvalidConfig = errors.New("invalid physical drum config")
)

// Quality selects a maximum real-time modal-state budget.
type Quality string

const (
	QualityDraft    Quality = "draft"
	QualityStandard Quality = "standard"
	QualityHigh     Quality = "high"
)

// ModeLimit returns the maximum number of individual modal oscillators.
// Non-axisymmetric eigenmodes consume two slots (cosine and sine orientation).
func (q Quality) ModeLimit() int {
	switch q {
	case QualityDraft:
		return 24
	case QualityStandard:
		return 48
	case QualityHigh:
		return 96
	default:
		return 0
	}
}

// PhysicalDrum is the versioned, serializable physical-model configuration.
type PhysicalDrum struct {
	Version      int          `json:"version"`
	SampleRateHz float64      `json:"sampleRateHz"`
	Quality      Quality      `json:"quality"`
	Batter       Head         `json:"batter"`
	Resonant     Head         `json:"resonant"`
	Strike       Strike       `json:"strike"`
	Cavity       Cavity       `json:"cavity"`
	Nonlinearity Nonlinearity `json:"nonlinearity"`
	Pickup       Pickup       `json:"pickup"`
}

// Head describes one circular membrane/plate.
type Head struct {
	Enabled                  bool                  `json:"enabled"`
	RadiusM                  float64               `json:"radiusM"`
	SurfaceDensityKgPerM2    float64               `json:"surfaceDensityKgPerM2"`
	TensionNPerM             float64               `json:"tensionNPerM"`
	BendingStiffnessNM       float64               `json:"bendingStiffnessNM"`
	Loss0PerSecond           float64               `json:"loss0PerSecond"`
	Loss2M2PerSecond         float64               `json:"loss2M2PerSecond"`
	RadiationLossPerSecond   float64               `json:"radiationLossPerSecond"`
	ModeDecayCorrections     []ModeDecayCorrection `json:"modeDecayCorrections,omitempty"`
	FrequencyLimitFraction   float64               `json:"frequencyLimitFraction"`
	InactiveEnergyThresholdJ float64               `json:"inactiveEnergyThresholdJ"`
}

// ModeDecayCorrection adds a measured residual to the two-parameter loss law.
// A correction applies to both orientations of a non-axisymmetric mode.
type ModeDecayCorrection struct {
	AzimuthalOrder     int     `json:"azimuthalOrder"`
	RadialOrder        int     `json:"radialOrder"`
	DecayRatePerSecond float64 `json:"decayRatePerSecond"`
}

// Mode describes one retained circular-head oscillator.
type Mode struct {
	AzimuthalOrder           int
	RadialOrder              int
	Orientation              Orientation
	BesselZero               float64
	WavenumberPerM           float64
	FrequencyHz              float64
	AngularFrequency         float64
	StructuralDecayPerSecond float64
	RadiationDecayPerSecond  float64
	DecayCorrectionPerSecond float64
	DecayRatePerSecond       float64
	ModalMassKg              float64
	StrikeAccelerationPerN   float64
	PickupShape              float64
	RadiationWeight          float64
	SweptAreaM2              float64
}

// Strike describes the mallet and its finite contact footprint.
type Strike struct {
	Radius01       float64 `json:"radius01"`
	AngleRad       float64 `json:"angleRad"`
	ContactRadiusM float64 `json:"contactRadiusM"`
	MalletMassKg   float64 `json:"malletMassKg"`
	VelocityMPerS  float64 `json:"velocityMPerS"`
	Hardness01     float64 `json:"hardness01"`
}

// Cavity describes the lumped enclosed-air spring and its pressure loss.
type Cavity struct {
	Enabled           bool    `json:"enabled"`
	DepthM            float64 `json:"depthM"`
	AirDensityKgPerM3 float64 `json:"airDensityKgPerM3"`
	SoundSpeedMPerS   float64 `json:"soundSpeedMPerS"`
	LossPerSecond     float64 `json:"lossPerSecond"`
}

// Nonlinearity controls the Berger-style tension increase shared by every
// retained mode of each head. TensionCoefficientNPerM3 is the small-strain
// slope dT/dS, where S is the integral of the squared head gradient in m².
// MaximumTensionRatio caps the tension increase relative to each head's static
// tension so retained modes remain below Nyquist.
type Nonlinearity struct {
	Enabled                          bool    `json:"enabled"`
	BatterTensionCoefficientNPerM3   float64 `json:"batterTensionCoefficientNPerM3"`
	ResonantTensionCoefficientNPerM3 float64 `json:"resonantTensionCoefficientNPerM3"`
	MaximumTensionRatio              float64 `json:"maximumTensionRatio"`
}

// Pickup selects a diagnostic observation point and compact microphone
// response. Radius01 and AngleRad locate the microphone projection over the
// head; DistanceM controls geometric attenuation.
type Pickup struct {
	Radius01   float64 `json:"radius01"`
	AngleRad   float64 `json:"angleRad"`
	DistanceM  float64 `json:"distanceM"`
	HighpassHz float64 `json:"highpassHz"`
	LowpassHz  float64 `json:"lowpassHz"`
	OutputGain float64 `json:"outputGain"`
}

// DefaultPhysicalDrum returns a conservative 12-inch double-headed tom
// configuration.
func DefaultPhysicalDrum() PhysicalDrum {
	head := Head{
		Enabled:                  true,
		RadiusM:                  0.1524,
		SurfaceDensityKgPerM2:    0.35,
		TensionNPerM:             600,
		BendingStiffnessNM:       0.001,
		Loss0PerSecond:           3,
		Loss2M2PerSecond:         2e-5,
		RadiationLossPerSecond:   1.5,
		FrequencyLimitFraction:   0.45,
		InactiveEnergyThresholdJ: 1e-12,
	}

	return PhysicalDrum{
		Version:      ConfigVersion,
		SampleRateHz: 48_000,
		Quality:      QualityStandard,
		Batter:       head,
		Resonant: Head{
			Enabled:                  true,
			RadiusM:                  head.RadiusM,
			SurfaceDensityKgPerM2:    0.25,
			TensionNPerM:             500,
			BendingStiffnessNM:       0.0007,
			Loss0PerSecond:           4,
			Loss2M2PerSecond:         2e-5,
			RadiationLossPerSecond:   1.5,
			FrequencyLimitFraction:   head.FrequencyLimitFraction,
			InactiveEnergyThresholdJ: head.InactiveEnergyThresholdJ,
		},
		Strike: Strike{
			Radius01:       0.45,
			AngleRad:       0.2,
			ContactRadiusM: 0.01,
			MalletMassKg:   0.015,
			VelocityMPerS:  3,
			Hardness01:     0.7,
		},
		Cavity: Cavity{
			Enabled:           true,
			DepthM:            0.20,
			AirDensityKgPerM3: 1.204,
			SoundSpeedMPerS:   343,
			LossPerSecond:     5,
		},
		Nonlinearity: Nonlinearity{
			Enabled:                          true,
			BatterTensionCoefficientNPerM3:   3.0e5,
			ResonantTensionCoefficientNPerM3: 2.0e5,
			MaximumTensionRatio:              0.2,
		},
		Pickup: Pickup{
			Radius01:   0.32,
			AngleRad:   0.6,
			DistanceM:  0.30,
			HighpassHz: 35,
			LowpassHz:  12_000,
			OutputGain: 0.6,
		},
	}
}

// Validate checks every persisted field.
func (d PhysicalDrum) Validate() error {
	if d.Version != ConfigVersion {
		return fmt.Errorf("%w: got %d, want %d", ErrConfigVersion, d.Version, ConfigVersion)
	}

	if err := finiteRange("sampleRateHz", d.SampleRateHz, minSampleRateHz, maxSampleRateHz); err != nil {
		return err
	}

	if d.Quality.ModeLimit() == 0 {
		return fmt.Errorf("%w: unknown quality %q", ErrInvalidConfig, d.Quality)
	}

	if err := validateHead("batter", d.Batter, true); err != nil {
		return err
	}

	if err := validateHead("resonant", d.Resonant, false); err != nil {
		return err
	}

	if err := finiteRange("strike.radius01", d.Strike.Radius01, 0, 1); err != nil {
		return err
	}

	if err := finiteRange("strike.angleRad", d.Strike.AngleRad, -2*math.Pi, 2*math.Pi); err != nil {
		return err
	}

	if err := finiteRange("strike.contactRadiusM", d.Strike.ContactRadiusM, 1e-4, d.Batter.RadiusM/2); err != nil {
		return err
	}

	if err := finiteRange("strike.malletMassKg", d.Strike.MalletMassKg, 1e-4, 1); err != nil {
		return err
	}

	if err := finiteRange("strike.velocityMPerS", d.Strike.VelocityMPerS, 0, 20); err != nil {
		return err
	}

	if err := finiteRange("strike.hardness01", d.Strike.Hardness01, 0, 1); err != nil {
		return err
	}

	if err := finiteRange("cavity.depthM", d.Cavity.DepthM, 0.01, 2); err != nil {
		return err
	}

	if err := finiteRange("cavity.airDensityKgPerM3", d.Cavity.AirDensityKgPerM3, 0.5, 2); err != nil {
		return err
	}

	if err := finiteRange("cavity.soundSpeedMPerS", d.Cavity.SoundSpeedMPerS, 250, 400); err != nil {
		return err
	}

	if err := finiteRange("cavity.lossPerSecond", d.Cavity.LossPerSecond, 0, 10_000); err != nil {
		return err
	}

	if err := validateNonlinearity(d); err != nil {
		return err
	}

	if err := finiteRange("pickup.radius01", d.Pickup.Radius01, 0, 1); err != nil {
		return err
	}

	if err := finiteRange("pickup.angleRad", d.Pickup.AngleRad, -2*math.Pi, 2*math.Pi); err != nil {
		return err
	}

	if err := finiteRange("pickup.distanceM", d.Pickup.DistanceM, 0.01, 10); err != nil {
		return err
	}

	if err := finiteRange("pickup.highpassHz", d.Pickup.HighpassHz, 1, maxSampleRateHz/2); err != nil {
		return err
	}

	if err := finiteRange("pickup.lowpassHz", d.Pickup.LowpassHz, d.Pickup.HighpassHz, maxSampleRateHz/2); err != nil {
		return err
	}

	if err := finiteRange("pickup.outputGain", d.Pickup.OutputGain, 0, 100); err != nil {
		return err
	}

	return nil
}

// EncodeConfig validates and serializes a physical-drum configuration.
func EncodeConfig(config PhysicalDrum) ([]byte, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	return json.Marshal(config)
}

// DecodeConfig decodes the current schema, migrates supported older schemas,
// and validates every field.
func DecodeConfig(data []byte) (PhysicalDrum, error) {
	var config PhysicalDrum
	if err := json.Unmarshal(data, &config); err != nil {
		return PhysicalDrum{}, fmt.Errorf("%w: decode: %v", ErrInvalidConfig, err)
	}

	if config.Version == legacyConfigVersion {
		migrateV1Config(&config)
	}
	if config.Version == linearDoubleHeadConfigVersion {
		migrateV2Config(&config)
	}

	if err := config.Validate(); err != nil {
		return PhysicalDrum{}, err
	}

	return config, nil
}

func migrateV1Config(config *PhysicalDrum) {
	defaults := DefaultPhysicalDrum()

	config.Version = linearDoubleHeadConfigVersion
	config.Batter.RadiationLossPerSecond = defaults.Batter.RadiationLossPerSecond
	config.Resonant.RadiationLossPerSecond = defaults.Resonant.RadiationLossPerSecond
	config.Pickup.DistanceM = defaults.Pickup.DistanceM
	config.Pickup.HighpassHz = defaults.Pickup.HighpassHz
	config.Pickup.LowpassHz = defaults.Pickup.LowpassHz

	// P1's scalar gain preceded distance attenuation and radiation filtering.
	// Move legacy configs onto the P2 output level.
	config.Pickup.OutputGain = defaults.Pickup.OutputGain
}

func migrateV2Config(config *PhysicalDrum) {
	config.Version = ConfigVersion
	// Version 2 was the linear double-head model. Preserve its sound exactly;
	// newly created version-3 configs opt into the nonlinear extension.
	config.Nonlinearity = Nonlinearity{}
}

func validateNonlinearity(config PhysicalDrum) error {
	nonlinearity := config.Nonlinearity
	if err := finiteRange(
		"nonlinearity.batterTensionCoefficientNPerM3",
		nonlinearity.BatterTensionCoefficientNPerM3,
		0,
		1e9,
	); err != nil {
		return err
	}
	if err := finiteRange(
		"nonlinearity.resonantTensionCoefficientNPerM3",
		nonlinearity.ResonantTensionCoefficientNPerM3,
		0,
		1e9,
	); err != nil {
		return err
	}
	if err := finiteRange(
		"nonlinearity.maximumTensionRatio",
		nonlinearity.MaximumTensionRatio,
		0,
		1,
	); err != nil {
		return err
	}
	if !nonlinearity.Enabled {
		return nil
	}
	if nonlinearity.BatterTensionCoefficientNPerM3 == 0 &&
		(!config.Resonant.Enabled ||
			nonlinearity.ResonantTensionCoefficientNPerM3 == 0) {
		return fmt.Errorf(
			"%w: enabled nonlinearity has no positive tension coefficient",
			ErrInvalidConfig,
		)
	}
	if nonlinearity.MaximumTensionRatio == 0 {
		return fmt.Errorf(
			"%w: enabled nonlinearity has zero maximum tension ratio",
			ErrInvalidConfig,
		)
	}

	safeRatio := maximumSafeTensionRatio(config.Batter)
	if config.Resonant.Enabled {
		safeRatio = min(safeRatio, maximumSafeTensionRatio(config.Resonant))
	}
	if nonlinearity.MaximumTensionRatio > safeRatio {
		return fmt.Errorf(
			"%w: nonlinearity.maximumTensionRatio %v exceeds anti-alias bound %v",
			ErrInvalidConfig,
			nonlinearity.MaximumTensionRatio,
			safeRatio,
		)
	}

	return nil
}

func maximumSafeTensionRatio(head Head) float64 {
	frequencyLimit := head.FrequencyLimitFraction

	return 1/(4*frequencyLimit*frequencyLimit) - 1
}

func validateHead(name string, head Head, required bool) error {
	if required && !head.Enabled {
		return fmt.Errorf("%w: %s must be enabled", ErrInvalidConfig, name)
	}

	checks := []struct {
		field    string
		value    float64
		minValue float64
		maxValue float64
	}{
		{"radiusM", head.RadiusM, 0.02, 1},
		{"surfaceDensityKgPerM2", head.SurfaceDensityKgPerM2, 0.01, 10},
		{"tensionNPerM", head.TensionNPerM, 1, 100_000},
		{"bendingStiffnessNM", head.BendingStiffnessNM, 0, 100},
		{"loss0PerSecond", head.Loss0PerSecond, 0, 10_000},
		{"loss2M2PerSecond", head.Loss2M2PerSecond, 0, 10},
		{"radiationLossPerSecond", head.RadiationLossPerSecond, 0, 10_000},
		{"frequencyLimitFraction", head.FrequencyLimitFraction, 0.05, 0.49},
		{"inactiveEnergyThresholdJ", head.InactiveEnergyThresholdJ, 0, 1},
	}
	for _, check := range checks {
		if err := finiteRange(name+"."+check.field, check.value, check.minValue, check.maxValue); err != nil {
			return err
		}
	}

	seenCorrections := make(map[[2]int]struct{}, len(head.ModeDecayCorrections))
	for _, correction := range head.ModeDecayCorrections {
		if correction.AzimuthalOrder < 0 || correction.AzimuthalOrder > maxModeOrder ||
			correction.RadialOrder < 1 || correction.RadialOrder > maxModeOrder {
			return fmt.Errorf(
				"%w: %s mode correction index=(%d,%d)",
				ErrInvalidConfig,
				name,
				correction.AzimuthalOrder,
				correction.RadialOrder,
			)
		}

		if err := finiteRange(
			name+".modeDecayCorrection",
			correction.DecayRatePerSecond,
			-10_000,
			10_000,
		); err != nil {
			return err
		}

		key := [2]int{correction.AzimuthalOrder, correction.RadialOrder}
		if _, exists := seenCorrections[key]; exists {
			return fmt.Errorf(
				"%w: duplicate %s mode correction index=(%d,%d)",
				ErrInvalidConfig,
				name,
				correction.AzimuthalOrder,
				correction.RadialOrder,
			)
		}

		seenCorrections[key] = struct{}{}
	}

	return nil
}

func finiteRange(name string, value, minValue, maxValue float64) error {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < minValue || value > maxValue {
		return fmt.Errorf(
			"%w: %s=%v outside [%v,%v]",
			ErrInvalidConfig,
			name,
			value,
			minValue,
			maxValue,
		)
	}

	return nil
}
