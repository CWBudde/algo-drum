package physical

import (
	"math"
	"testing"

	algofft "github.com/cwbudde/algo-fft"
)

func TestDefaultStickContactMatchesMeasuredTomRange(t *testing.T) {
	t.Parallel()

	config := DefaultPhysicalDrum()
	quiet, err := NewSingleHead(config)
	if err != nil {
		t.Fatal(err)
	}
	loud, err := NewSingleHead(config)
	if err != nil {
		t.Fatal(err)
	}

	if err := quiet.Trigger(0); err != nil {
		t.Fatal(err)
	}
	if err := loud.Trigger(1); err != nil {
		t.Fatal(err)
	}

	quietSeconds := float64(quiet.PulseSamples()) / config.SampleRateHz
	loudSeconds := float64(loud.PulseSamples()) / config.SampleRateHz
	sampleTolerance := 0.5 / config.SampleRateHz
	if math.Abs(quietSeconds-quietStickContactSeconds) > sampleTolerance {
		t.Fatalf(
			"quiet contact = %.6f s, want %.6f s within half a sample",
			quietSeconds,
			quietStickContactSeconds,
		)
	}
	if math.Abs(loudSeconds-loudStickContactSeconds) > sampleTolerance {
		t.Fatalf(
			"loud contact = %.6f s, want %.6f s within half a sample",
			loudSeconds,
			loudStickContactSeconds,
		)
	}
	if loud.PulseSamples() >= quiet.PulseSamples() {
		t.Fatalf(
			"loud contact %d samples is not shorter than quiet %d",
			loud.PulseSamples(),
			quiet.PulseSamples(),
		)
	}
}

func TestContactHardnessChangesDurationWithoutAllocating(t *testing.T) {
	config := DefaultPhysicalDrum()
	softConfig := config
	softConfig.Strike.Hardness01 = 0.2
	hardConfig := config
	hardConfig.Strike.Hardness01 = 0.9

	soft, err := NewDoubleHead(softConfig)
	if err != nil {
		t.Fatal(err)
	}
	hard, err := NewDoubleHead(hardConfig)
	if err != nil {
		t.Fatal(err)
	}
	if err := soft.Trigger(0.8); err != nil {
		t.Fatal(err)
	}
	if err := hard.Trigger(0.8); err != nil {
		t.Fatal(err)
	}
	if hard.PulseSamples() >= soft.PulseSamples() {
		t.Fatalf(
			"hard contact %d samples is not shorter than soft %d",
			hard.PulseSamples(),
			soft.PulseSamples(),
		)
	}

	allocations := testing.AllocsPerRun(100, func() {
		hard.Reset()
		if triggerErr := hard.Trigger(0.8); triggerErr != nil {
			panic(triggerErr)
		}
	})
	if allocations != 0 {
		t.Fatalf("Trigger allocated %v times, want 0", allocations)
	}
}

func TestContactPulsePreservesPrescribedImpulse(t *testing.T) {
	t.Parallel()

	const (
		scale       = 37.5
		shortLength = 64
		longLength  = 640
	)

	for _, sampleCount := range []int{shortLength, longLength} {
		pending := make([]float64, sampleCount)
		addContactPulse(pending, 0, sampleCount, scale)

		sum := 0.0
		for _, sample := range pending {
			if sample < 0 {
				t.Fatalf("%d-sample contact contains negative force %v", sampleCount, sample)
			}
			sum += sample
		}
		if math.Abs(sum-scale) > scale*1e-13 {
			t.Fatalf(
				"%d-sample contact sum = %.16g, want %.16g",
				sampleCount,
				sum,
				scale,
			)
		}
	}
}

// TestCentreStrikeSoundIsFundamentalLedInTheAttack asserts where the
// fundamental is supposed to lead, which is neither the window nor the strike
// position this test used to use.
//
// It first required the (0,1) to be the strongest partial over 1.4 s. That is
// the signature of the defect P8 corrects, not of a drum: the axisymmetric
// fundamental dumps its energy into the cavity and the opposite head fastest, so
// it defines the initial pitch of the thump and is gone well before the sustain.
//
// It then required the same of the *default* configuration's attack, which the
// corrected microphone model shows is not a property of the model at all but of
// where the drum is hit. Measured across strike radius and window length:
//
//	radius  43 ms   85 ms   171 ms
//	0.00    117.2   105.5   105.5    fundamental throughout
//	0.12    117.2   105.5   164.1
//	0.30    164.1   164.1   164.1    the (1,1) pair throughout
//
// A centre hit is fundamental-led and an off-centre hit is (1,1)-led. That is
// the real behaviour of a tom — it is why players strike toward the middle for a
// full tone — so the invariant belongs to the centre hit, and pinning it to
// whatever the shipped strike radius happens to be would forbid moving it.
func TestCentreStrikeSoundIsFundamentalLedInTheAttack(t *testing.T) {
	t.Parallel()

	config := DefaultPhysicalDrum()
	config.Strike.Radius01 = 0
	model, err := NewDoubleHead(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := model.Trigger(0.8); err != nil {
		t.Fatal(err)
	}

	// 171 ms: long enough to resolve 104 from 165 Hz, short enough to sit
	// inside the fundamental's 211 ms T60.
	const fftSize = 8192
	samples := make([]float64, fftSize)
	resonantBecameAudible := false
	for index := range samples {
		output := model.Tick()
		samples[index] = output.Radiated
		// The microphone hears the batter head plus the attack layer, and
		// specifically not the resonant head's own outward radiation — that
		// leaves the far side of the shell and needs a propagation transfer
		// nothing here supplies.
		if output.RawRadiated !=
			output.BatterRawRadiated+output.AttackRawRadiated {
			t.Fatalf(
				"sample %d pickup %v is not the batter radiation %v plus the attack layer %v",
				index,
				output.RawRadiated,
				output.BatterRawRadiated,
				output.AttackRawRadiated,
			)
		}
		if output.ResonantRawRadiated != 0 {
			resonantBecameAudible = true
		}
	}
	if !resonantBecameAudible {
		t.Fatal("resonant head was not dynamically excited")
	}

	attackHz := strongestSpectralPeakHz(t, samples, config.SampleRateHz, 60, 1_000)
	if attackHz < 90 || attackHz > 130 {
		t.Fatalf(
			"centre-strike strongest attack peak = %.2f Hz, want fundamental in [90,130] Hz",
			attackHz,
		)
	}

	// And the complementary half of the same property: by the time the
	// fundamental has decayed the sustain belongs to the higher modes, so the
	// hit has a pitch envelope instead of one steady tone.
	sustain := make([]float64, 32_768)
	for index := range sustain {
		sustain[index] = model.Tick().Radiated
	}

	sustainHz := strongestSpectralPeakHz(t, sustain, config.SampleRateHz, 60, 1_000)
	t.Logf("attack peak %.2f Hz, sustain peak %.2f Hz", attackHz, sustainHz)
	if sustainHz <= attackHz*1.1 {
		t.Fatalf(
			"sustain peak %.2f Hz did not move above the attack peak %.2f Hz",
			sustainHz,
			attackHz,
		)
	}
}

// TestDefaultSoundKeepsTheFundamentalAudible is the other half of the property
// above. The shipped strike is off centre, so the fundamental is not required to
// lead — but a drum whose fundamental has gone missing has no body at all, and
// with the (1,1) pair leading nothing else in the suite would notice.
//
// The bound is a regression guard, not a target. Measured against strike radius,
// the fundamental sits 2.07 dB below the strongest partial at 0.12, 7.23 at
// 0.22, 9.78 at the shipped 0.30 and 11.22 at 0.36 — monotone, with no sweet
// spot to find. So this catches the fundamental disappearing, and where inside
// the range the drum sits is a tuning decision that HIT.R exposes.
func TestDefaultSoundKeepsTheFundamentalAudible(t *testing.T) {
	t.Parallel()

	config := DefaultPhysicalDrum()
	model, err := NewDoubleHead(config)
	if err != nil {
		t.Fatal(err)
	}

	fundamental, ok := model.BatterMode(0)
	if !ok {
		t.Fatal("batter head has no modes")
	}
	if fundamental.AzimuthalOrder != 0 || fundamental.RadialOrder != 1 {
		t.Fatalf(
			"lowest batter mode is (%d,%d), want the (0,1)",
			fundamental.AzimuthalOrder,
			fundamental.RadialOrder,
		)
	}
	if err := model.Trigger(0.8); err != nil {
		t.Fatal(err)
	}

	samples := make([]float64, 8192)
	for index := range samples {
		samples[index] = model.Tick().Radiated
	}

	magnitudes := spectralMagnitudes(t, samples)
	binHz := config.SampleRateHz / float64(len(samples))
	fundamentalBin := int(math.Round(fundamental.FrequencyHz / binHz))
	// One bin either side: the coupled and tension-modulated frequency is close
	// to the analytic one but not identical to it.
	fundamentalMagnitude := 0.0
	for offset := -1; offset <= 1; offset++ {
		fundamentalMagnitude = max(
			fundamentalMagnitude,
			magnitudes[fundamentalBin+offset],
		)
	}

	strongest := 0.0
	for index := max(1, int(60/binHz)); index <= int(1_000/binHz); index++ {
		strongest = max(strongest, magnitudes[index])
	}

	belowStrongestDB := 20 * math.Log10(strongest/fundamentalMagnitude)
	t.Logf(
		"fundamental %.2f Hz sits %.2f dB below the strongest partial",
		fundamental.FrequencyHz,
		belowStrongestDB,
	)
	if belowStrongestDB > 12 {
		t.Fatalf(
			"fundamental is %.2f dB below the strongest partial, want within 12 dB",
			belowStrongestDB,
		)
	}
}

func spectralMagnitudes(t *testing.T, samples []float64) []float64 {
	t.Helper()

	windowed := make([]float64, len(samples))
	for index, sample := range samples {
		windowed[index] = sample * (0.5 - 0.5*math.Cos(
			2*math.Pi*float64(index)/float64(len(samples)-1),
		))
	}

	plan, err := algofft.NewPlanReal64(len(windowed))
	if err != nil {
		t.Fatal(err)
	}
	bins := make([]complex128, plan.SpectrumLen())
	if err := plan.Forward(bins, windowed); err != nil {
		t.Fatal(err)
	}

	magnitudes := make([]float64, len(bins))
	for index, bin := range bins {
		magnitudes[index] = math.Hypot(real(bin), imag(bin))
	}

	return magnitudes
}

func strongestSpectralPeakHz(
	t *testing.T,
	samples []float64,
	sampleRate, minimumHz, maximumHz float64,
) float64 {
	t.Helper()

	for index := range samples {
		samples[index] *= 0.5 - 0.5*math.Cos(
			2*math.Pi*float64(index)/float64(len(samples)-1),
		)
	}

	plan, err := algofft.NewPlanReal64(len(samples))
	if err != nil {
		t.Fatal(err)
	}
	bins := make([]complex128, plan.SpectrumLen())
	if err := plan.Forward(bins, samples); err != nil {
		t.Fatal(err)
	}

	minimumBin := max(1, int(math.Ceil(minimumHz*float64(len(samples))/sampleRate)))
	maximumBin := min(
		len(bins)-1,
		int(math.Floor(maximumHz*float64(len(samples))/sampleRate)),
	)
	strongestBin := minimumBin
	strongestMagnitude := 0.0
	for index := minimumBin; index <= maximumBin; index++ {
		magnitude := math.Hypot(real(bins[index]), imag(bins[index]))
		if magnitude > strongestMagnitude {
			strongestMagnitude = magnitude
			strongestBin = index
		}
	}

	return float64(strongestBin) * sampleRate / float64(len(samples))
}
