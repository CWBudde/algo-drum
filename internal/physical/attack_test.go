package physical

import (
	"math"
	"testing"
)

// TestAttackLayerSuppliesTheMissingBandwidth is the reason the hybrid layer
// exists, stated as a measurement.
//
// Modal synthesis cannot reach a drum's bandwidth in a browser, and the numbers
// here are what "no bandwidth" actually sounds like: with the layer off there is
// nothing above 1 kHz within 60 dB of the fundamental. The exit criterion for
// this work is audible content above 1 kHz, and this is the assertion that
// enforces it.
func TestAttackLayerSuppliesTheMissingBandwidth(t *testing.T) {
	t.Parallel()

	// 43 ms: the attack window. Over a longer one the modal tail dominates and
	// the ratio measures the decay rather than the presence of a high band.
	const fftSize = 2048

	bandLevelsDB := func(enabled bool) (float64, float64) {
		config := DefaultPhysicalDrum()
		config.Attack.Enabled = enabled
		model, err := NewDoubleHead(config)
		if err != nil {
			t.Fatal(err)
		}
		if err := model.Trigger(1); err != nil {
			t.Fatal(err)
		}

		samples := make([]float64, fftSize)
		for index := range samples {
			samples[index] = model.Tick().Radiated
		}

		magnitudes := spectralMagnitudes(t, samples)
		binHz := config.SampleRateHz / float64(fftSize)
		peakBetween := func(lowHz, highHz float64) float64 {
			best := 0.0
			for index := int(lowHz / binHz); index <= int(highHz/binHz) &&
				index < len(magnitudes); index++ {
				best = max(best, magnitudes[index])
			}

			return best
		}

		reference := peakBetween(60, 1_000)

		return 20 * math.Log10(peakBetween(1_000, 2_000)/reference),
			20 * math.Log10(peakBetween(2_000, 5_000)/reference)
	}

	offLow, offHigh := bandLevelsDB(false)
	onLow, onHigh := bandLevelsDB(true)
	t.Logf(
		"relative to the strongest low partial: 1-2 kHz %.1f -> %.1f dB, 2-5 kHz %.1f -> %.1f dB",
		offLow,
		onLow,
		offHigh,
		onHigh,
	)

	if offHigh > -60 {
		t.Fatalf(
			"modal synthesis alone reaches %.1f dB at 2-5 kHz; if it now covers "+
				"that band on its own the attack layer may be redundant",
			offHigh,
		)
	}
	if onLow < -40 || onHigh < -35 {
		t.Fatalf(
			"attack layer too quiet: 1-2 kHz %.1f dB, 2-5 kHz %.1f dB",
			onLow,
			onHigh,
		)
	}
	// And not so loud that it reads as a click rather than a stick.
	if onHigh > -15 {
		t.Fatalf("attack layer dominates the low band: 2-5 kHz %.1f dB", onHigh)
	}
}

// TestAttackLayerIsDeterministic pins the property that makes a stochastic layer
// safe to put in this codebase at all: every render from a fresh or reset model
// is bit-identical. Much of the suite compares renders exactly, and a global
// random source would break all of it in a way that only shows up sometimes.
func TestAttackLayerIsDeterministic(t *testing.T) {
	t.Parallel()

	render := func(model *DoubleHead) []float64 {
		if err := model.Trigger(0.8); err != nil {
			t.Fatal(err)
		}

		samples := make([]float64, 4_096)
		for index := range samples {
			samples[index] = model.Tick().Radiated
		}

		return samples
	}

	config := DefaultPhysicalDrum()
	first, err := NewDoubleHead(config)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewDoubleHead(config)
	if err != nil {
		t.Fatal(err)
	}

	fresh := render(first)
	other := render(second)
	for index := range fresh {
		if fresh[index] != other[index] {
			t.Fatalf(
				"sample %d differs between two fresh models: %v against %v",
				index,
				fresh[index],
				other[index],
			)
		}
	}

	// Reset has to rewind the noise sequence too, or a second hit on the same
	// model diverges from a first hit on a new one.
	first.Reset()

	replayed := render(first)
	for index := range fresh {
		if fresh[index] != replayed[index] {
			t.Fatalf(
				"sample %d differs after Reset: %v against %v",
				index,
				fresh[index],
				replayed[index],
			)
		}
	}
}

// TestAttackLayerScalesWithVelocityAndKeepsTheVoiceAlive covers the two ways the
// layer is wired into the rest of the model: it is driven by the contact force
// rather than triggered beside it, and its envelope has to be visible to
// IsActive.
func TestAttackLayerScalesWithVelocityAndKeepsTheVoiceAlive(t *testing.T) {
	t.Parallel()

	peakAt := func(velocity float64) float64 {
		config := DefaultPhysicalDrum()
		model, err := NewDoubleHead(config)
		if err != nil {
			t.Fatal(err)
		}
		if err := model.Trigger(velocity); err != nil {
			t.Fatal(err)
		}

		peak := 0.0
		for range 4_096 {
			peak = max(peak, math.Abs(model.Tick().AttackRawRadiated))
		}

		return peak
	}

	quiet := peakAt(0.25)
	loud := peakAt(1)
	t.Logf("attack layer peak: velocity 0.25 %.4g, velocity 1 %.4g", quiet, loud)

	if quiet <= 0 {
		t.Fatal("attack layer produced nothing at velocity 0.25")
	}
	if loud <= quiet*2 {
		t.Fatalf(
			"attack layer barely scales with velocity: %.4g against %.4g",
			loud,
			quiet,
		)
	}

	// A layer that outlives the modal energy threshold must keep the voice
	// active, or the tail is cut off mid-burst. Silence the heads so the modal
	// energy is negligible and only the layer can be keeping it alive.
	config := DefaultPhysicalDrum()
	config.Attack.DecayScale = 30
	model, err := NewDoubleHead(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := model.Trigger(1); err != nil {
		t.Fatal(err)
	}
	for range model.PulseSamples() + 1 {
		model.Tick()
	}
	if !model.IsActive() {
		t.Fatal("model went inactive while the attack layer was still ringing")
	}
}

func TestAttackLayerDisabledIsExactlySilent(t *testing.T) {
	t.Parallel()

	for name, config := range map[string]PhysicalDrum{
		"disabled":   attackConfigWith(func(a *Attack) { a.Enabled = false }),
		"zero level": attackConfigWith(func(a *Attack) { a.LevelRelative = 0 }),
	} {
		model, err := NewDoubleHead(config)
		if err != nil {
			t.Fatal(err)
		}
		if err := model.Trigger(1); err != nil {
			t.Fatal(err)
		}

		for index := range 4_096 {
			output := model.Tick()
			if output.AttackRawRadiated != 0 {
				t.Fatalf(
					"%s: sample %d attack contribution %v, want exactly 0",
					name,
					index,
					output.AttackRawRadiated,
				)
			}
			if output.RawRadiated != output.BatterRawRadiated {
				t.Fatalf(
					"%s: sample %d radiated sum %v is not the batter term %v",
					name,
					index,
					output.RawRadiated,
					output.BatterRawRadiated,
				)
			}
		}
	}
}

func attackConfigWith(apply func(*Attack)) PhysicalDrum {
	config := DefaultPhysicalDrum()
	apply(&config.Attack)

	return config
}

// TestAttackBandsDecayAtTheirOwnRate is the fix for the layer sounding like
// noise rather than like a stick.
//
// The layer used to be one band held at an absolute 20 ms release — a 138 ms T60,
// flat across 1-8 kHz — where the head's own loss law puts that span between
// 149 ms at 1 kHz and 18 ms at 8 kHz. Broadband noise sustained that far past the
// strike does not fuse into the attack; it is heard as a separate hiss sitting on
// the drum. Now each band decays at the rate the loss law gives for its own
// centre, so the top of the layer dies several times faster than the bottom.
func TestAttackBandsDecayAtTheirOwnRate(t *testing.T) {
	t.Parallel()

	config := DefaultPhysicalDrum()
	layer := newAttackLayer(config.Attack, config.Batter, config.SampleRateHz)
	if !layer.enabled {
		t.Fatal("default attack layer is disabled")
	}

	speed := WaveSpeedMPerS(config.Batter)
	previous := 0.0

	for index, ratio := range attackBandRatios {
		centreHz := config.Attack.CentreHz * ratio
		want := ModalDecayRatePerSecond(config.Batter, 2*math.Pi*centreHz/speed)
		// decayFactor is tau*fs/(tau*fs + 1); invert it back to a rate.
		factor := layer.bands[index].decayFactor
		got := config.SampleRateHz * (1/factor - 1)
		t.Logf(
			"band %d at %.0f Hz: tau %.2f ms, T60 %.0f ms",
			index,
			centreHz,
			1e3/got,
			1e3*math.Log(1000)/got,
		)

		if math.Abs(got/want-1) > 1e-6 {
			t.Fatalf(
				"band %d rate %.3f /s does not match the loss law's %.3f /s",
				index,
				got,
				want,
			)
		}
		if index > 0 && got <= previous {
			t.Fatalf(
				"band %d decays at %.3f /s, no faster than the band below at %.3f /s",
				index,
				got,
				previous,
			)
		}

		previous = got
	}

	// The whole layer has to be finished inside the attack. Anything still
	// audible at 100 ms is a tail, and a noise tail is exactly the complaint.
	if t60 := 1e3 * math.Log(1000) /
		ModalDecayRatePerSecond(
			config.Batter,
			2*math.Pi*config.Attack.CentreHz*attackBandRatios[0]/speed,
		); t60 > 110 {
		t.Fatalf("slowest attack band rings for %.0f ms", t60)
	}
}

// TestAttackLayerStartsAboveTheModalBand keeps the two halves of the voice from
// describing the same frequencies. Below the top retained mode the model has real
// modes, which are heard as pitch; filling that region with noise as well is both
// a double count and the wrong texture.
func TestAttackLayerStartsAboveTheModalBand(t *testing.T) {
	t.Parallel()

	config := DefaultPhysicalDrum()

	modes, err := GenerateModes(config)
	if err != nil {
		t.Fatal(err)
	}

	topModeHz := 0.0
	for _, mode := range modes {
		topModeHz = max(topModeHz, mode.FrequencyHz)
	}

	lowestBandHz := config.Attack.CentreHz * attackBandRatios[0]
	t.Logf(
		"top retained mode %.0f Hz, lowest attack band %.0f Hz",
		topModeHz,
		lowestBandHz,
	)

	if lowestBandHz <= topModeHz {
		t.Fatalf(
			"lowest attack band at %.0f Hz sits inside the modal band, which "+
				"reaches %.0f Hz",
			lowestBandHz,
			topModeHz,
		)
	}
}
