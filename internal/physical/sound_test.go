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

func TestDefaultBatterSideSoundIsFundamentalLed(t *testing.T) {
	t.Parallel()

	config := DefaultPhysicalDrum()
	model, err := NewDoubleHead(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := model.Trigger(0.8); err != nil {
		t.Fatal(err)
	}

	const fftSize = 65_536
	samples := make([]float64, fftSize)
	resonantBecameAudible := false
	for index := range samples {
		output := model.Tick()
		samples[index] = output.Radiated
		if output.RawRadiated != output.BatterRawRadiated {
			t.Fatalf(
				"sample %d batter-side pickup %v differs from batter radiation %v",
				index,
				output.RawRadiated,
				output.BatterRawRadiated,
			)
		}
		if output.ResonantRawRadiated != 0 {
			resonantBecameAudible = true
		}
	}
	if !resonantBecameAudible {
		t.Fatal("resonant head was not dynamically excited")
	}

	strongestHz := strongestSpectralPeakHz(t, samples, config.SampleRateHz, 60, 1_000)
	if strongestHz < 90 || strongestHz > 130 {
		t.Fatalf(
			"default strongest sustain peak = %.2f Hz, want fundamental in [90,130] Hz",
			strongestHz,
		)
	}
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
