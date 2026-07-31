package physical

import (
	"fmt"
	"math"
	"testing"

	algofft "github.com/cwbudde/algo-fft"
)

// This file is the P9/M2 confirmation test. The claim under test is that the
// partial the modal cavity adds near 660 Hz is a transverse *air* mode and not a
// head mode that the coupling coefficients put in the wrong place, and the
// discriminator is the one the item states: a rigid-walled cylinder's transverse
// resonance is c*j'_mn/(2*pi*a), so it moves with the sound speed and inversely
// with the shell radius, and it does not move with head tension at all. A
// membrane mode moves as sqrt(T)/a and is blind to c.
//
// Radius alone cannot separate the two — both families go as 1/a — so the
// weight is carried by the sound speed, which nothing about either head depends
// on, and by the tension, which nothing about the enclosed air depends on.
// See docs/physical-cavity.md for the measured sweeps.

const (
	// The analysis window and transform used by every measurement here. 32768
	// bins at 48 kHz is 683 ms — long enough to resolve 1.5 Hz, which is what
	// reading a 5 Hz coupling shift off a 660 Hz partial needs, and short enough
	// to fit inside a 0.75 s render that starts after the strike transient.
	transverseProbeSeconds  = 0.75
	transverseProbeStartSec = 0.05
	transverseProbeFFTSize  = 32_768
)

// transverseCavityHz is the uncoupled frequency of one transverse cavity mode.
func transverseCavityHz(t *testing.T, config PhysicalDrum, azimuthal, radial int) float64 {
	t.Helper()

	modes, err := GenerateCavityModes(config)
	if err != nil {
		t.Fatal(err)
	}

	for _, mode := range modes {
		if mode.AzimuthalOrder == azimuthal && mode.RadialOrder == radial {
			return mode.FrequencyHz
		}
	}

	t.Fatalf("cavity basis carries no (%d,%d) mode", azimuthal, radial)

	return 0
}

// transverseSpectrum renders one full-velocity strike and returns the magnitude
// spectrum of its sustain, together with the strongest magnitude in it.
//
// The strike is the configured off-centre one, unlike the central strike
// cavity_split_test.go needs: an m > 0 cavity mode can only be driven by an
// m > 0 head mode, and a central hit excites none.
func transverseSpectrum(t *testing.T, config PhysicalDrum) ([]float64, float64) {
	t.Helper()

	model, err := NewDoubleHead(config)
	if err != nil {
		t.Fatal(err)
	}

	if err := model.Trigger(1.0); err != nil {
		t.Fatal(err)
	}

	samples := make([]float64, int(transverseProbeSeconds*config.SampleRateHz))
	model.Render(samples)

	start := int(transverseProbeStartSec * config.SampleRateHz)
	if start+transverseProbeFFTSize > len(samples) {
		t.Fatalf("render of %d samples is too short for a %d-point window at %.0f Hz",
			len(samples), transverseProbeFFTSize, config.SampleRateHz)
	}

	windowed := make([]float64, transverseProbeFFTSize)
	for index := range windowed {
		windowed[index] = samples[start+index] * (0.5 - 0.5*math.Cos(
			2*math.Pi*float64(index)/float64(transverseProbeFFTSize-1),
		))
	}

	plan, err := algofft.NewPlanReal64(transverseProbeFFTSize)
	if err != nil {
		t.Fatal(err)
	}

	bins := make([]complex128, plan.SpectrumLen())
	if err := plan.Forward(bins, windowed); err != nil {
		t.Fatal(err)
	}

	binHz := config.SampleRateHz / transverseProbeFFTSize
	limit := min(len(bins)-2, int(1500/binHz))

	magnitude := make([]float64, limit+1)
	strongest := 0.0

	for index := max(2, int(100/binHz)); index <= limit; index++ {
		magnitude[index] = math.Hypot(real(bins[index]), imag(bins[index]))
		strongest = math.Max(strongest, magnitude[index])
	}

	if strongest == 0 {
		t.Fatal("strike produced no radiated output")
	}

	return magnitude, strongest
}

// transversePeakNear returns the interpolated position of the strongest local
// maximum within windowHz of targetHz, and its level relative to the strongest
// peak of the whole band.
func transversePeakNear(
	t *testing.T,
	config PhysicalDrum,
	magnitude []float64,
	strongest, targetHz, windowHz float64,
) (float64, float64, bool) {
	t.Helper()

	binHz := config.SampleRateHz / transverseProbeFFTSize
	low := max(2, int((targetHz-windowHz)/binHz))
	high := min(len(magnitude)-2, int((targetHz+windowHz)/binHz))

	best := -1
	for index := low; index <= high; index++ {
		if magnitude[index] <= magnitude[index-1] || magnitude[index] < magnitude[index+1] {
			continue
		}

		if best < 0 || magnitude[index] > magnitude[best] {
			best = index
		}
	}

	if best < 0 {
		return 0, transverseLevelAt(config, magnitude, strongest, targetHz), false
	}

	// Parabolic interpolation in the log domain: at 1.5 Hz per bin the bare bin
	// centre is worth a fifth of the shift being measured.
	left := math.Log(math.Max(magnitude[best-1], 1e-300))
	centre := math.Log(math.Max(magnitude[best], 1e-300))
	right := math.Log(math.Max(magnitude[best+1], 1e-300))

	offset := 0.0
	if denominator := left - 2*centre + right; denominator != 0 {
		offset = 0.5 * (left - right) / denominator
		if math.Abs(offset) > 0.5 || math.IsNaN(offset) {
			offset = 0
		}
	}

	return (float64(best) + offset) * binHz, 20 * math.Log10(magnitude[best]/strongest), true
}

func transverseLevelAt(config PhysicalDrum, magnitude []float64, strongest, targetHz float64) float64 {
	binHz := config.SampleRateHz / transverseProbeFFTSize

	bin := int(math.Round(targetHz / binHz))
	if bin < 0 || bin >= len(magnitude) || magnitude[bin] == 0 {
		return math.Inf(-1)
	}

	return 20 * math.Log10(magnitude[bin]/strongest)
}

// transverseBandEnergyDB is the summed energy within halfWidthHz of targetHz,
// in dB relative to the strongest bin of the band.
//
// A single bin is the wrong reading when the question is whether a mode is
// present at all: the coupled mode does not sit exactly on its uncoupled
// eigenfrequency — the cavity loading moves it by a few hertz, and by a
// different few in each arm of a comparison — so a bin-exact reading measures
// that displacement as much as it measures the level. Summing over the band
// asks only whether there is anything there.
func transverseBandEnergyDB(
	config PhysicalDrum,
	magnitude []float64,
	strongest, targetHz, halfWidthHz float64,
) float64 {
	binHz := config.SampleRateHz / transverseProbeFFTSize
	low := max(0, int((targetHz-halfWidthHz)/binHz))
	high := min(len(magnitude)-1, int((targetHz+halfWidthHz)/binHz))

	total := 0.0
	for index := low; index <= high; index++ {
		total += magnitude[index] * magnitude[index]
	}

	if total == 0 {
		return math.Inf(-1)
	}

	return 10 * math.Log10(total/(strongest*strongest))
}

// transverseProbeConfig is the shipped drum at the shipped quality tier, which
// is what the claim is about.
func transverseProbeConfig() PhysicalDrum {
	return DefaultPhysicalDrum()
}

// transverseProbeStiffnessScale is the Cavity.StiffnessScale every frequency
// tolerance in this file was measured at.
//
// It has to be pinned, because the tolerances are not independent of it. A cavity
// mode's own frequency c*j'/(2*pi*a) does not contain the stiffness scale — that
// scales K_c, which is how hard the air pushes back, not where it resonates — but
// the *coupled* mode is loaded by the heads and that load does. Measured, the
// partial sits 0.7 % above the bare air-mode formula at the shipped 0.083, 6.8 %
// above at 0.3, and 14.2 % above at the rigid ceiling of 1. So the sharp
// statement these tests make — the partial is where the air-mode formula puts it
// — is a weak-coupling statement, and a refit of the stiffness would invalidate
// the numbers rather than the mechanism.
const transverseProbeStiffnessScale = 0.083

func requireShippedStiffnessScale(t *testing.T, config PhysicalDrum) {
	t.Helper()

	if config.Cavity.StiffnessScale == transverseProbeStiffnessScale {
		return
	}

	t.Fatalf("Cavity.StiffnessScale is %v, not the %v the tolerances here were measured at; "+
		"the coupled partial's offset from the bare air-mode frequency scales with it "+
		"(0.7 %% at 0.083, 14.2 %% at 1), so re-measure the offset and retune these "+
		"tolerances rather than widening them. See docs/physical-cavity.md, "+
		"\"The confirmation is a weak-coupling result\".",
		config.Cavity.StiffnessScale, transverseProbeStiffnessScale)
}

// TestTransverseCavityPartialTracksSoundSpeedAndRadius is the first half of the
// P9/M2 criterion. The added partial has to sit where c*j'_11/(2*pi*a) puts it,
// through a sound-speed sweep the heads cannot see and a radius change.
func TestTransverseCavityPartialTracksSoundSpeedAndRadius(t *testing.T) {
	t.Parallel()

	// The measured offset is +0.5 to +1.2 % across every sweep in
	// docs/physical-cavity.md and is always positive: the head loading stiffens
	// the coupled mode above the uncoupled air resonance. 2 % admits that
	// without admitting the nearest head mode, which is 3 % away at its closest.
	const toleranceFraction = 0.02

	cases := []struct {
		name   string
		mutate func(*PhysicalDrum)
	}{
		{"shipped", func(*PhysicalDrum) {}},
		{"c=300", func(config *PhysicalDrum) { config.Cavity.SoundSpeedMPerS = 300 }},
		{"c=400", func(config *PhysicalDrum) { config.Cavity.SoundSpeedMPerS = 400 }},
		{"radius x0.85", func(config *PhysicalDrum) {
			config.Batter.RadiusM *= 0.85
			config.Resonant.RadiusM *= 0.85
		}},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			config := transverseProbeConfig()
			testCase.mutate(&config)
			requireShippedStiffnessScale(t, config)

			predicted := transverseCavityHz(t, config, 1, 1)
			magnitude, strongest := transverseSpectrum(t, config)

			observed, level, ok := transversePeakNear(t, config, magnitude, strongest, predicted, 25)
			if !ok {
				t.Fatalf("no partial within 25 Hz of the predicted %.2f Hz", predicted)
			}

			deviation := (observed - predicted) / predicted
			if math.Abs(deviation) > toleranceFraction {
				t.Errorf("partial at %.2f Hz is %+.2f %% from the predicted %.2f Hz, want within %.0f %%",
					observed, 100*deviation, predicted, 100*toleranceFraction)
			}

			t.Logf("predicted %.2f Hz, observed %.2f Hz (%+.2f %%), %.1f dB",
				predicted, observed, 100*deviation, level)
		})
	}
}

// TestTransverseCavityPartialIgnoresHeadTension is the discriminator P9/M2 names
// as the kill condition: if the added partial tracked head tension it would be a
// head mode, and the coupling coefficients would be wrong.
func TestTransverseCavityPartialIgnoresHeadTension(t *testing.T) {
	t.Parallel()

	// Wider than the sweep above, because a weakly driven partial's peak is
	// pulled a little toward whichever head mode is nearest, and the two
	// tunings put different head modes nearest. Still far below the 18 %
	// sqrt(T) excursion this is testing against.
	const toleranceFraction = 0.025

	reference := transverseProbeConfig()
	predicted := transverseCavityHz(t, reference, 1, 1)

	for _, scale := range []float64{0.70, 1.40} {
		t.Run(fmt.Sprintf("tension x%.2f", scale), func(t *testing.T) {
			t.Parallel()

			config := transverseProbeConfig()
			config.Batter.TensionNPerM *= scale
			config.Resonant.TensionNPerM *= scale
			requireShippedStiffnessScale(t, config)

			// The premise: the heads really did move. Without this the test
			// would pass on a configuration that changed nothing.
			before := NaturalFrequencyHz(reference.Batter, besselZeros(0, 1)[0])
			after := NaturalFrequencyHz(config.Batter, besselZeros(0, 1)[0])

			if shift := math.Abs(after-before) / before; shift < 0.15 {
				t.Fatalf("batter fundamental moved only %.1f %% (%.2f to %.2f Hz); "+
					"the tension sweep is not exercising anything", 100*shift, before, after)
			}

			// The prediction is the *unchanged* one: nothing about the enclosed
			// air depends on head tension.
			magnitude, strongest := transverseSpectrum(t, config)

			observed, level, ok := transversePeakNear(t, config, magnitude, strongest, predicted, 25)
			if !ok {
				t.Fatalf("no partial within 25 Hz of %.2f Hz after retuning the heads by x%.2f",
					predicted, scale)
			}

			deviation := (observed - predicted) / predicted
			if math.Abs(deviation) > toleranceFraction {
				t.Errorf("partial moved to %.2f Hz (%+.2f %%) when the heads were retuned by x%.2f; "+
					"a cavity mode does not move with tension",
					observed, 100*deviation, scale)
			}

			t.Logf("heads x%.2f (fundamental %.2f to %.2f Hz): partial at %.2f Hz (%+.2f %%), %.1f dB",
				scale, before, after, observed, 100*deviation, level)
		})
	}
}

// TestLumpedCavityHasNoTransversePartial is the negative control. The lumped
// cavity's only coupling coefficient is the swept area, which is identically
// zero for every m > 0 head mode, so nothing can drive an m = 1 air mode and
// there is no m = 1 air mode to drive. If this ever finds one, the transverse
// basis is not what is putting the partial there.
func TestLumpedCavityHasNoTransversePartial(t *testing.T) {
	t.Parallel()

	// The measured gap is about 58 dB. 30 is a wide margin that still fails
	// long before the partial could be called present.
	const minimumGapDB = 30

	predicted := transverseCavityHz(t, transverseProbeConfig(), 1, 1)

	modalConfig := transverseProbeConfig()
	modalMagnitude, modalStrongest := transverseSpectrum(t, modalConfig)
	modalLevel := transverseLevelAt(modalConfig, modalMagnitude, modalStrongest, predicted)

	lumpedConfig := transverseProbeConfig()
	lumpedConfig.Cavity.ModeCount = 1
	lumpedMagnitude, lumpedStrongest := transverseSpectrum(t, lumpedConfig)
	lumpedLevel := transverseLevelAt(lumpedConfig, lumpedMagnitude, lumpedStrongest, predicted)

	if gap := modalLevel - lumpedLevel; gap < minimumGapDB {
		t.Errorf("the modal cavity adds only %.1f dB at %.2f Hz (%.1f against %.1f), want at least %.0f",
			gap, predicted, modalLevel, lumpedLevel, float64(minimumGapDB))
	}

	t.Logf("level at %.2f Hz: modal cavity %.1f dB, lumped cavity %.1f dB",
		predicted, modalLevel, lumpedLevel)
}

// TestTransverseCavityGivesM1ModesOutput is the second half of the P9/M2
// criterion: m = 1 head modes have to acquire measurable output through the
// cavity path once the near-field pickup term is removed.
//
// The band read is the one holding the *resonant* head's m = 1 (1,3) pair, and
// the reason that band is the evidence needs stating, because it is not the
// obvious one. observe() does not put the resonant head's radiation into the
// output at all — Pickup describes a batter-side microphone and the resonant
// head radiates out of the other end of the shell — so nothing at a
// resonant-only frequency can reach the pickup by radiating. If output appears
// there anyway, it is the batter head being driven at those frequencies, and the
// enclosed air is the only thing that connects the two. Under the lumped cavity
// that connection is the swept area, identically zero for every m > 0 mode, so
// the path does not exist.
//
// Resonant.AxisymmetricOnly is forced off in both arms so the two runs carry the
// same modal bank and only the cavity differs; otherwise the lumped arm would
// have no m = 1 resonant mode at all and the comparison would be between a mode
// and no mode rather than between a driven mode and a silent one.
//
// The gain is 5.6 dB, not the tens of dB the internal modal energy shows in
// TestTransverseCavityFeedsNonAxisymmetricResonantModes (exactly zero to
// non-zero). Two things hold it down, and both are worth recording. The batter
// head's own m = 1 modes are directly struck and go on radiating with the near
// field removed — Pickup's documentation says a zero NearFieldScale leaves "very
// nearly the axisymmetric modes alone", and that holds for the far-field weight
// but not for what reaches this band — so the lumped arm's reading is the
// batter's skirt rather than silence. And the shipped Cavity.StiffnessScale of
// 0.083 sets the coupling this path carries. 5.6 dB against that background is
// what the output-side half of the criterion is worth; the mechanism itself is
// measured internally.
func TestTransverseCavityGivesM1ModesOutput(t *testing.T) {
	t.Parallel()

	// Measured 5.6 dB. 4 keeps a regression guard without pretending the effect
	// is larger than it is.
	const minimumGainDB = 4

	probe := func(modeCount int) (PhysicalDrum, []float64, float64) {
		config := transverseProbeConfig()
		config.Resonant.AxisymmetricOnly = false
		config.Pickup.NearFieldScale = 0
		config.Cavity.ModeCount = modeCount

		magnitude, strongest := transverseSpectrum(t, config)

		return config, magnitude, strongest
	}

	lumpedConfig, lumpedMagnitude, lumpedStrongest := probe(1)
	modalConfig, modalMagnitude, modalStrongest := probe(DefaultPhysicalDrum().Cavity.ModeCount)

	resonantModes, err := generateHeadModes(modalConfig, modalConfig.Resonant)
	if err != nil {
		t.Fatal(err)
	}

	cavityHz := transverseCavityHz(t, modalConfig, 1, 1)

	lowHz, highHz := math.Inf(1), math.Inf(-1)
	for _, mode := range resonantModes {
		if mode.AzimuthalOrder != 1 || math.Abs(mode.FrequencyHz-cavityHz) > 40 {
			continue
		}

		lowHz = math.Min(lowHz, mode.FrequencyHz)
		highHz = math.Max(highHz, mode.FrequencyHz)
	}

	if math.IsInf(lowHz, 1) {
		t.Fatal("no resonant m = 1 mode within 40 Hz of the transverse cavity mode; " +
			"the reachability premise this rests on no longer holds")
	}

	// Wide enough for the split pair and the coupling's own few-hertz shift,
	// narrow enough to exclude the transverse partial itself at 664 Hz and the
	// batter's (0,4) at 738.
	const marginHz = 6

	centreHz := 0.5 * (lowHz + highHz)
	halfWidthHz := 0.5*(highHz-lowHz) + marginHz

	lumped := transverseBandEnergyDB(lumpedConfig, lumpedMagnitude, lumpedStrongest, centreHz, halfWidthHz)
	modal := transverseBandEnergyDB(modalConfig, modalMagnitude, modalStrongest, centreHz, halfWidthHz)

	if gain := modal - lumped; gain < minimumGainDB {
		t.Errorf("the resonant m=1 band %.2f-%.2f Hz gains only %.1f dB from the transverse cavity "+
			"(%.1f against %.1f), want at least %.0f",
			centreHz-halfWidthHz, centreHz+halfWidthHz, gain, modal, lumped, float64(minimumGainDB))
	}

	t.Logf("resonant m=1 band %.2f-%.2f Hz: %.1f dB with transverse modes against %.1f without, +%.1f dB",
		centreHz-halfWidthHz, centreHz+halfWidthHz, modal, lumped, modal-lumped)
}
