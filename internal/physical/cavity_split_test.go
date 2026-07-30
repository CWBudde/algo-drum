package physical

import (
	"math"
	"testing"

	algofft "github.com/cwbudde/algo-fft"
)

// Measured two-headed drums separate the two (0,1) branches by 10–20 %: Fischer
// (Modal Analysis of a Snare Drum, Illinois 2014) found 186 Hz with one head and
// 215 Hz once the resonant head was added at unchanged tuning, a ratio of 1.16.
const (
	minimumCavitySplitRatio = 1.10
	maximumCavitySplitRatio = 1.20
)

// spectralPeak is one local maximum of a rendered spectrum, in dB relative to
// the strongest peak in the analysed band.
type spectralPeak struct {
	frequencyHz float64
	levelDB     float64
}

// centreHitPeaks renders a dead-centre strike and returns its spectral peaks
// below limitHz.
//
// The strike has to be central: an offset hit excites the (1,1) pair, which is
// louder than either (0,1) branch and would bury the doublet this file is about.
// Only axisymmetric modes have non-zero swept area, so a central hit isolates
// exactly the modes the cavity couples.
func centreHitPeaks(t *testing.T, config PhysicalDrum, limitHz float64) []spectralPeak {
	t.Helper()

	config.Strike.Radius01 = 0
	model, err := NewDoubleHead(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := model.Trigger(0.8); err != nil {
		t.Fatal(err)
	}

	// 341 ms at 48 kHz: 2.9 Hz resolution, enough to separate branches that sit
	// 17 Hz apart, and long enough to cover the fundamental's decay.
	const fftSize = 16_384
	samples := make([]float64, fftSize)
	for index := range samples {
		samples[index] = model.Tick().Radiated * (0.5 - 0.5*math.Cos(
			2*math.Pi*float64(index)/float64(fftSize-1),
		))
	}

	plan, err := algofft.NewPlanReal64(fftSize)
	if err != nil {
		t.Fatal(err)
	}
	bins := make([]complex128, plan.SpectrumLen())
	if err := plan.Forward(bins, samples); err != nil {
		t.Fatal(err)
	}

	binHz := config.SampleRateHz / fftSize
	limitBin := min(len(bins)-1, int(limitHz/binHz))
	magnitude := make([]float64, limitBin+1)
	strongest := 0.0
	for index := range magnitude {
		magnitude[index] = math.Hypot(real(bins[index]), imag(bins[index]))
		strongest = math.Max(strongest, magnitude[index])
	}
	if strongest == 0 {
		t.Fatal("central strike produced no radiated output")
	}

	// Start at 60 Hz. The lowest mode of either head is above 100 Hz and the
	// pickup high-passes at 35 Hz, so the residue of the strike pulse in between
	// is not modal content and must not be mistaken for a branch.
	var peaks []spectralPeak
	for index := max(2, int(60/binHz)); index < limitBin; index++ {
		if magnitude[index] <= magnitude[index-1] || magnitude[index] < magnitude[index+1] {
			continue
		}
		if 20*math.Log10(magnitude[index]/strongest) < -40 {
			continue
		}
		peaks = append(peaks, spectralPeak{
			frequencyHz: float64(index) * binHz,
			levelDB:     20 * math.Log10(magnitude[index]/strongest),
		})
	}
	return peaks
}

// firstOvertoneFamilyHz is just below the lowest uncoupled (0,2), which is
// 238.9 Hz on the batter and 258.0 Hz on the resonant head. Cavity stiffening
// only ever raises an axisymmetric branch, so no member of the (0,2) family can
// appear below this and anything that does belongs to the (0,1) doublet.
const firstOvertoneFamilyHz = 235.0

// axisymmetricDoublet returns the unstiffened and stiffened (0,1) branches. The
// unstiffened one is the lowest peak — interlacing keeps it between the two
// heads' uncoupled (0,1) frequencies whatever the stiffness is — and the
// stiffened one is the loudest peak above it that still belongs to the family.
func axisymmetricDoublet(t *testing.T, peaks []spectralPeak) (spectralPeak, spectralPeak) {
	t.Helper()

	if len(peaks) == 0 {
		t.Fatal("no spectral peaks found")
	}
	lower := peaks[0]

	upper := spectralPeak{levelDB: math.Inf(-1)}
	for _, peak := range peaks[1:] {
		if peak.frequencyHz > firstOvertoneFamilyHz {
			break
		}
		if peak.levelDB > upper.levelDB {
			upper = peak
		}
	}
	if math.IsInf(upper.levelDB, -1) {
		t.Fatalf(
			"no stiffened (0,1) branch between %.1f and %.1f Hz; peaks were %v",
			lower.frequencyHz, firstOvertoneFamilyHz, peaks,
		)
	}
	return lower, upper
}

// TestDefaultCavitySplitMatchesMeasuredDrums is the S3 exit condition: the
// fitted stiffness has to put the (0,1) doublet where measurements do.
func TestDefaultCavitySplitMatchesMeasuredDrums(t *testing.T) {
	t.Parallel()

	config := DefaultPhysicalDrum()
	lower, upper := axisymmetricDoublet(t, centreHitPeaks(t, config, 700))
	ratio := upper.frequencyHz / lower.frequencyHz
	t.Logf(
		"(0,1) doublet %.1f Hz / %.1f Hz (%.1f dB), ratio %.4f",
		lower.frequencyHz, upper.frequencyHz, upper.levelDB, ratio,
	)

	if ratio < minimumCavitySplitRatio || ratio > maximumCavitySplitRatio {
		t.Fatalf(
			"(0,1) doublet ratio %.4f (%.1f / %.1f Hz), want [%.2f, %.2f]",
			ratio, lower.frequencyHz, upper.frequencyHz,
			minimumCavitySplitRatio, maximumCavitySplitRatio,
		)
	}
}

// TestDefaultCavityLeavesNoPartialWhereTom2Belongs guards the audible
// consequence of getting the split wrong, which is not visible in the ratio
// alone. The rigid stiffness drove the upper branch to 219.7 Hz at −3 dB,
// landing a loud partial on top of the (2,1) at 221.8 Hz. Correcting the split
// has to clear that region, not merely narrow the doublet.
func TestDefaultCavityLeavesNoPartialWhereTom2Belongs(t *testing.T) {
	t.Parallel()

	config := DefaultPhysicalDrum()
	for _, peak := range centreHitPeaks(t, config, 700) {
		if peak.frequencyHz < 200 || peak.frequencyHz > 235 {
			continue
		}
		if peak.levelDB > -10 {
			t.Fatalf(
				"partial at %.1f Hz is only %.1f dB down, want more than 10 dB "+
					"below the fundamental so it does not mask the (2,1)",
				peak.frequencyHz, peak.levelDB,
			)
		}
	}
}

// TestRigidCavityStiffnessOverpredictsTheSplit records why
// Cavity.StiffnessScale exists at all. Deriving the bulk stiffness from
// rho*c^2/V — a rigid, sealed, piston-driven enclosure — more than doubles the
// (0,1) instead of raising it by a sixth, so the scale is a required correction
// rather than a spare knob left at a round number.
func TestRigidCavityStiffnessOverpredictsTheSplit(t *testing.T) {
	t.Parallel()

	config := DefaultPhysicalDrum()
	config.Cavity.StiffnessScale = 1
	lower, upper := axisymmetricDoublet(t, centreHitPeaks(t, config, 700))
	ratio := upper.frequencyHz / lower.frequencyHz
	t.Logf("rigid-cavity doublet %.1f / %.1f Hz, ratio %.4f",
		lower.frequencyHz, upper.frequencyHz, ratio)

	if ratio <= maximumCavitySplitRatio {
		t.Fatalf(
			"rigid stiffness gave ratio %.4f, expected it to exceed the measured "+
				"ceiling %.2f — if it no longer does, the fitted scale is obsolete",
			ratio, maximumCavitySplitRatio,
		)
	}
}

// TestCavityStiffnessScaleScalesBulkStiffness pins the field's meaning: it is a
// factor on rho*c^2/V, and zero is the same uncoupled limit as a disabled
// cavity.
func TestCavityStiffnessScaleScalesBulkStiffness(t *testing.T) {
	t.Parallel()

	config := DefaultPhysicalDrum()
	config.Cavity.StiffnessScale = 1
	rigid, err := NewDoubleHead(config)
	if err != nil {
		t.Fatal(err)
	}

	expected := config.Cavity.AirDensityKgPerM3 *
		config.Cavity.SoundSpeedMPerS * config.Cavity.SoundSpeedMPerS /
		rigid.CavityVolumeM3()
	if relativeDifference(rigid.CavityBulkStiffnessPaPerM3(), expected) > 1e-12 {
		t.Fatalf(
			"unscaled bulk stiffness = %g, want rho*c^2/V = %g",
			rigid.CavityBulkStiffnessPaPerM3(), expected,
		)
	}

	for _, scale := range []float64{0, 0.04, 0.5} {
		scaled := config
		scaled.Cavity.StiffnessScale = scale
		model, err := NewDoubleHead(scaled)
		if err != nil {
			t.Fatal(err)
		}
		want := scale * expected
		if math.Abs(model.CavityBulkStiffnessPaPerM3()-want) > 1e-9*expected {
			t.Fatalf(
				"scale %g gave bulk stiffness %g, want %g",
				scale, model.CavityBulkStiffnessPaPerM3(), want,
			)
		}
	}
}
