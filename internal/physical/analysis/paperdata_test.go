package analysis_test

import (
	"math"
	"testing"

	"github.com/cwbudde/algo-drum/internal/physical"
	"github.com/cwbudde/algo-drum/internal/physical/analysis"
)

func paperData(t *testing.T) analysis.PaperData {
	t.Helper()

	data, err := analysis.GeneratePaperData(physical.DefaultPhysicalDrum())
	if err != nil {
		t.Fatalf("GeneratePaperData: %v", err)
	}

	return data
}

// TestPaperDataModesAreTheWholeInstrument guards the property the mode-map and
// bandwidth figures rest on: the artefact carries both heads, split at the index
// the model itself splits them at. Emitting the batter bank alone would draw a
// figure of half the instrument and look entirely plausible.
func TestPaperDataModesAreTheWholeInstrument(t *testing.T) {
	data := paperData(t)

	counts := map[string]int{}
	for _, mode := range data.Modes {
		counts[mode.Head]++
	}

	for _, head := range data.Heads {
		if counts[head.Name] != head.ModeCount {
			t.Errorf(
				"%s: %d modes tagged, head reports %d",
				head.Name,
				counts[head.Name],
				head.ModeCount,
			)
		}

		if head.ModeCount == 0 {
			t.Errorf("%s: no modes", head.Name)
		}
	}

	if counts["batter"]+counts["resonant"] != len(data.Modes) {
		t.Errorf("modes carry a head other than batter or resonant: %v", counts)
	}
}

// TestPaperDataSweptAreaIsAxisymmetricOnly restates in the artefact the property
// the cavity section claims: the air spring couples through the swept area, and
// that area is exactly zero for every m > 0 mode. A figure asserting it should
// not be drawn from data that contradicts it.
func TestPaperDataSweptAreaIsAxisymmetricOnly(t *testing.T) {
	for _, mode := range paperData(t).Modes {
		if mode.AzimuthalOrder > 0 && mode.SweptAreaM2 != 0 {
			t.Errorf(
				"(%d,%d) %s: swept area %v, want exactly 0",
				mode.AzimuthalOrder,
				mode.RadialOrder,
				mode.Orientation,
				mode.SweptAreaM2,
			)
		}

		if mode.AzimuthalOrder == 0 && mode.SweptAreaM2 == 0 {
			t.Errorf("(0,%d): axisymmetric mode has no swept area", mode.RadialOrder)
		}
	}
}

// TestPaperDataAttackBandsFollowTheLossLaw is the check the attack-layer figure
// makes visually. The releases are derived from the head's own loss law, so they
// must fall with frequency; a flat set of rates is the defect the three-band
// layer replaced and would be invisible in a plot of three points.
func TestPaperDataAttackBandsFollowTheLossLaw(t *testing.T) {
	data := paperData(t)

	if len(data.AttackBands) < 2 {
		t.Fatalf("got %d attack bands, want at least 2", len(data.AttackBands))
	}

	config := physical.DefaultPhysicalDrum()
	speed := physical.WaveSpeedMPerS(config.Batter)

	for index, band := range data.AttackBands {
		want := physical.ModalDecayRatePerSecond(
			config.Batter,
			2*math.Pi*band.CentreHz/speed,
		)
		if math.Abs(band.DecayRatePerSecond-want) > 1e-9 {
			t.Errorf(
				"band %d at %.0f Hz: rate %v, loss law gives %v",
				index,
				band.CentreHz,
				band.DecayRatePerSecond,
				want,
			)
		}

		if index > 0 && band.T60Seconds >= data.AttackBands[index-1].T60Seconds {
			t.Errorf(
				"band %d at %.0f Hz rings %.4f s, not shorter than %.4f s below it",
				index,
				band.CentreHz,
				band.T60Seconds,
				data.AttackBands[index-1].T60Seconds,
			)
		}
	}
}

// TestPaperDataCavityBranchesInterlace is the substantive assertion behind the
// cavity figure.
//
// Adding a positive-definite rank-one stiffness to a symmetric eigenproblem can
// only raise eigenvalues, and by eigenvalue interlacing the lower branch cannot
// pass the second uncoupled fundamental. So the doublet must open monotonically
// with the stiffness scale while its lower member stays penned between the two
// heads' own fundamentals. If a future change to the cavity solve breaks that,
// the figure would still draw — three curves that no longer mean what the
// caption says.
func TestPaperDataCavityBranchesInterlace(t *testing.T) {
	data := paperData(t)
	curves := data.CavityResponse.Curves

	if len(curves) < 2 {
		t.Fatalf("got %d cavity curves, want at least 2", len(curves))
	}

	uncoupledLower := curves[0].LowerBranchHz
	uncoupledUpper := curves[0].UpperBranchHz

	if curves[0].StiffnessScale != 0 {
		t.Fatalf("first curve is scale %v, want the uncoupled 0", curves[0].StiffnessScale)
	}

	previousRatio := 0.0

	for index, curve := range curves {
		if curve.LowerBranchHz < uncoupledLower ||
			curve.LowerBranchHz > uncoupledUpper {
			t.Errorf(
				"scale %v: lower branch %.2f Hz outside the interlacing bound [%.2f, %.2f]",
				curve.StiffnessScale,
				curve.LowerBranchHz,
				uncoupledLower,
				uncoupledUpper,
			)
		}

		if curve.UpperBranchHz < uncoupledUpper {
			t.Errorf(
				"scale %v: upper branch %.2f Hz below the uncoupled %.2f Hz; "+
					"the air spring can only stiffen",
				curve.StiffnessScale,
				curve.UpperBranchHz,
				uncoupledUpper,
			)
		}

		ratio := curve.UpperBranchHz / curve.LowerBranchHz
		if index > 0 && ratio <= previousRatio {
			t.Errorf(
				"scale %v: doublet ratio %.4f did not open past %.4f",
				curve.StiffnessScale,
				ratio,
				previousRatio,
			)
		}

		previousRatio = ratio

		if len(curve.MagnitudeDB) != len(data.CavityResponse.FrequencyHz) {
			t.Errorf(
				"scale %v: %d magnitudes against %d frequencies",
				curve.StiffnessScale,
				len(curve.MagnitudeDB),
				len(data.CavityResponse.FrequencyHz),
			)
		}
	}
}

// TestPaperDataTiersAreOrdered pins the property the paper's search chapter
// reports at a different tuning: a bigger budget buys more modes and more
// bandwidth, and bandwidth grows far slower than the count.
func TestPaperDataTiersAreOrdered(t *testing.T) {
	tiers := paperData(t).Tiers

	if len(tiers) != 3 {
		t.Fatalf("got %d tiers, want 3", len(tiers))
	}

	for index := 1; index < len(tiers); index++ {
		previous := tiers[index-1]
		current := tiers[index]

		if current.SlotBudget <= previous.SlotBudget {
			t.Errorf("%s budget %d not above %s's %d",
				current.Quality, current.SlotBudget, previous.Quality, previous.SlotBudget)
		}

		if current.TopModeHz <= previous.TopModeHz {
			t.Errorf("%s ceiling %.1f Hz not above %s's %.1f Hz",
				current.Quality, current.TopModeHz, previous.Quality, previous.TopModeHz)
		}

		countRatio := float64(current.SlotBudget) / float64(previous.SlotBudget)
		bandRatio := current.TopModeHz / previous.TopModeHz

		if bandRatio >= countRatio {
			t.Errorf(
				"%s buys %.2fx the bandwidth for %.2fx the modes; "+
					"a membrane's mode count grows as f^2, so bandwidth must grow slower",
				current.Quality,
				bandRatio,
				countRatio,
			)
		}
	}
}
