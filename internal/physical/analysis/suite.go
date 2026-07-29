package analysis

import (
	"fmt"

	"github.com/cwbudde/algo-drum/internal/physical"
)

const ReferenceSchemaVersion = 1

// ReferenceSuite is a compact, deterministic calibration set. It stores
// derived metrics rather than generated audio so provenance is reviewable and
// no large binary fixture is required.
type ReferenceSuite struct {
	SchemaVersion int             `json:"schemaVersion"`
	Name          string          `json:"name"`
	License       string          `json:"license"`
	SourceType    string          `json:"sourceType"`
	Provenance    string          `json:"provenance"`
	Conditions    string          `json:"conditions"`
	ModeTargets   []ModeMetric    `json:"modeTargets"`
	Cases         []ReferenceCase `json:"cases"`
}

// ReferenceCase is one velocity, strike, and microphone configuration.
type ReferenceCase struct {
	ID     string `json:"id"`
	Report Report `json:"report"`
}

type referenceCaseConfig struct {
	id             string
	velocity       float64
	strikeRadius   float64
	pickupRadius   float64
	pickupAngle    float64
	pickupDistance float64
}

var referenceCases = [...]referenceCaseConfig{
	{"soft-center-near", 0.35, 0, 0, 0, 0.10},
	{"medium-center-near", 0.80, 0, 0, 0, 0.10},
	{"hard-center-near", 1.00, 0, 0, 0, 0.10},
	{"soft-offset-overhead", 0.35, 0.45, 0.32, 0.60, 0.30},
	{"medium-offset-overhead", 0.80, 0.45, 0.32, 0.60, 0.30},
	{"hard-offset-overhead", 1.00, 0.45, 0.32, 0.60, 0.30},
	{"soft-edge-far", 0.35, 0.70, 0.65, 1.20, 1.00},
	{"medium-edge-far", 0.80, 0.70, 0.65, 1.20, 1.00},
	{"hard-edge-far", 1.00, 0.70, 0.65, 1.20, 1.00},
}

// GenerateReferenceSuite renders the repository-owned synthetic calibration
// set. All cases use the default physical drum at 48 kHz in an anechoic,
// noiseless numerical environment.
func GenerateReferenceSuite() (ReferenceSuite, error) {
	options := DefaultOptions()
	options.DurationSeconds = 1.5
	options.PeakCount = 8

	suite := ReferenceSuite{
		SchemaVersion: ReferenceSchemaVersion,
		Name:          "algo-drum physical calibration v1",
		License:       "MIT; see repository LICENSE",
		SourceType:    "deterministic synthetic calibration",
		Provenance: "Generated from DefaultPhysicalDrum by " +
			"go run ./cmd/analyze-physical -suite -o testdata/physical-reference-v1.json",
		Conditions: "48 kHz mono; no room, noise, normalization, cavity coupling, " +
			"or nonlinear pitch modulation; algo-drum P2 single-head model",
		Cases: make([]ReferenceCase, 0, len(referenceCases)),
	}

	for _, specification := range referenceCases {
		config := physical.DefaultPhysicalDrum()
		config.Strike.Radius01 = specification.strikeRadius
		config.Pickup.Radius01 = specification.pickupRadius
		config.Pickup.AngleRad = specification.pickupAngle
		config.Pickup.DistanceM = specification.pickupDistance
		options.Velocity01 = specification.velocity

		report, err := Analyze(config, options)
		if err != nil {
			return ReferenceSuite{}, fmt.Errorf(
				"reference case %q: %w",
				specification.id,
				err,
			)
		}

		if specification.id == "medium-offset-overhead" {
			suite.ModeTargets = report.Modes
		}

		report.Modes = nil
		suite.Cases = append(suite.Cases, ReferenceCase{
			ID:     specification.id,
			Report: report,
		})
	}

	return suite, nil
}
