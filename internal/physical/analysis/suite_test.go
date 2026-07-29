package analysis

import "testing"

func TestGenerateReferenceSuiteCoverage(t *testing.T) {
	suite, err := GenerateReferenceSuite()
	if err != nil {
		t.Fatal(err)
	}
	if suite.SchemaVersion != ReferenceSchemaVersion {
		t.Fatalf("schema = %d, want %d", suite.SchemaVersion, ReferenceSchemaVersion)
	}
	if len(suite.ModeTargets) == 0 {
		t.Fatal("suite has no analytic modal targets")
	}
	if len(suite.Cases) != len(referenceCases) {
		t.Fatalf("cases = %d, want %d", len(suite.Cases), len(referenceCases))
	}

	velocities := make(map[float64]struct{})
	strikes := make(map[float64]struct{})
	microphones := make(map[[3]float64]struct{})
	for _, reference := range suite.Cases {
		report := reference.Report
		velocities[report.Velocity01] = struct{}{}
		strikes[report.StrikeRadius01] = struct{}{}
		microphones[[3]float64{
			report.PickupRadius01,
			report.PickupAngleRad,
			report.PickupDistanceM,
		}] = struct{}{}
		if report.Waveform.Peak == 0 || len(report.SpectralPeaks) == 0 {
			t.Fatalf("case %q has empty metrics", reference.ID)
		}
	}
	if len(velocities) < 3 || len(strikes) < 3 || len(microphones) < 3 {
		t.Fatalf(
			"suite coverage velocities=%d strikes=%d microphones=%d, want at least three each",
			len(velocities),
			len(strikes),
			len(microphones),
		)
	}
}
