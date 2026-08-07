package drum

import (
	"math"
	"testing"
)

type activeVoice interface {
	Voice
	IsActive() bool
}

func newTestVoices() map[string]activeVoice {
	return map[string]activeVoice{
		"BassDrum": NewBassDrum(testSampleRate),
		"Snare":    NewSnare(testSampleRate),
		"HiHat":    NewHiHat(testSampleRate),
		"Tom":      NewTom(testSampleRate),
		"Cymbal":   NewCymbal(testSampleRate),
		"Tom2":     NewTom2(testSampleRate),
		"Perc":     NewPercussion(testSampleRate),
	}
}

// tickUntilInactive runs a voice until it deactivates, returning the number
// of samples it stayed active and the absolute peak it produced.
func tickUntilInactive(t *testing.T, name string, voice activeVoice) (int, float64) {
	t.Helper()

	const maxSamples = int(testSampleRate * 30)

	var peak float64

	for sampleIndex := 0; sampleIndex < maxSamples; sampleIndex++ {
		sample := voice.Tick()
		if math.IsNaN(sample) || math.IsInf(sample, 0) {
			t.Fatalf("%s produced non-finite sample at %d: %v", name, sampleIndex, sample)
		}

		if abs := math.Abs(sample); abs > peak {
			peak = abs
		}

		if !voice.IsActive() {
			return sampleIndex + 1, peak
		}
	}

	t.Fatalf("%s still active after %d samples", name, maxSamples)

	return 0, 0
}

func TestVoicesSilentWhenInactive(t *testing.T) {
	for name, voice := range newTestVoices() {
		if voice.IsActive() {
			t.Fatalf("%s active before first trigger", name)
		}

		if sample := voice.Tick(); sample != 0 {
			t.Fatalf("%s inactive Tick() = %v, want 0", name, sample)
		}
	}
}

func TestVoicesDecayToInactiveWithBoundedOutput(t *testing.T) {
	for name, voice := range newTestVoices() {
		voice.Trigger(1)

		if !voice.IsActive() {
			t.Fatalf("%s not active after Trigger", name)
		}

		length, peak := tickUntilInactive(t, name, voice)

		if length < 100 {
			t.Fatalf("%s deactivated after only %d samples", name, length)
		}

		if peak == 0 {
			t.Fatalf("%s produced no output", name)
		}

		// Raw voice output (pre mix headroom / limiter) should stay in a
		// sane range; the biquad-shaped noise voices can ring above 1.
		if peak > 3.0 {
			t.Fatalf("%s peak %v exceeds raw-voice bound of 3.0", name, peak)
		}
	}
}

func TestVoicesVelocityScalesPeak(t *testing.T) {
	peakAt := func(name string, velocity float64) float64 {
		voice := newTestVoices()[name]
		voice.Trigger(velocity)

		var peak float64

		for sampleIndex := 0; sampleIndex < 4800; sampleIndex++ {
			sample := voice.Tick()
			if math.IsNaN(sample) || math.IsInf(sample, 0) {
				t.Fatalf("%s velocity %v produced non-finite sample: %v", name, velocity, sample)
			}

			if abs := math.Abs(sample); abs > peak {
				peak = abs
			}
		}

		return peak
	}

	for name := range newTestVoices() {
		full := peakAt(name, 1.0)
		half := peakAt(name, 0.5)

		if full <= 0 {
			t.Fatalf("%s produced no output at full velocity", name)
		}

		// Voices are linear in velocity (same seed = same noise), so half
		// velocity must yield ~half the peak.
		if ratio := half / full; math.Abs(ratio-0.5) > 0.02 {
			t.Fatalf("%s velocity 0.5 peak ratio = %.3f, want ~0.5", name, ratio)
		}
	}
}

func TestVoicesLongerDecaySettingRingsLonger(t *testing.T) {
	for name := range newTestVoices() {
		shortVoice := newTestVoices()[name]
		longVoice := newTestVoices()[name]

		shortVoice.SetDecay(0)
		longVoice.SetDecay(1)

		shortVoice.Trigger(1)
		longVoice.Trigger(1)

		shortLen, _ := tickUntilInactive(t, name+" (decay 0)", shortVoice)
		longLen, _ := tickUntilInactive(t, name+" (decay 1)", longVoice)

		if longLen <= shortLen {
			t.Fatalf("%s: decay=1 length %d not longer than decay=0 length %d", name, longLen, shortLen)
		}
	}
}

func TestVoicesDeterministic(t *testing.T) {
	for name := range newTestVoices() {
		first := newTestVoices()[name]
		second := newTestVoices()[name]

		first.Trigger(1)
		second.Trigger(1)

		for sampleIndex := 0; sampleIndex < 4800; sampleIndex++ {
			a := first.Tick()

			b := second.Tick()
			if a != b {
				t.Fatalf("%s sample %d differs between identical instances: %v vs %v", name, sampleIndex, a, b)
			}
		}
	}
}

func TestVoicesRetriggerRestartsEnvelope(t *testing.T) {
	for name, voice := range newTestVoices() {
		voice.Trigger(1)
		tickUntilInactive(t, name, voice)

		voice.Trigger(1)

		if !voice.IsActive() {
			t.Fatalf("%s not active after retrigger", name)
		}

		var peak float64

		for sampleIndex := 0; sampleIndex < 4800; sampleIndex++ {
			if abs := math.Abs(voice.Tick()); abs > peak {
				peak = abs
			}
		}

		if peak < 0.01 {
			t.Fatalf("%s retrigger produced peak %v, want audible restart", name, peak)
		}
	}
}

// ── Per-voice synthesis parameters (PLAN.md G20) ───────────────────────────

// renderVoice ticks a freshly triggered voice for a fixed span, so two voices
// can be compared sample-for-sample regardless of when they deactivate.
func renderVoice(voice Voice, samples int) []float64 {
	voice.Trigger(1)

	out := make([]float64, samples)
	for i := range out {
		out[i] = voice.Tick()
	}

	return out
}

// TestVoiceParamDefaultsAreShippedConstants is the no-sound-change regression
// test. Turning the tuning constants into parameters must leave a freshly
// built voice bit-identical to one whose every parameter was explicitly set to
// its default — that equivalence is what the byte-step snap in ParamSpec.Map
// exists to guarantee.
func TestVoiceParamDefaultsAreShippedConstants(t *testing.T) {
	const samples = 4800

	for name := range newTestVoices() {
		untouched := newTestVoices()[name]
		explicit := newTestVoices()[name]

		for i, spec := range explicit.ParamSpecs() {
			if got := untouched.Param(i); got != spec.Default {
				t.Errorf("%s param %d starts at %v, want default %v", name, i, got, spec.Default)
			}

			explicit.SetParam(i, spec.Default)
		}

		want := renderVoice(untouched, samples)

		got := renderVoice(explicit, samples)
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("%s sample %d differs after setting every param to its default: %v vs %v",
					name, i, got[i], want[i])
			}
		}
	}
}

func TestVoiceParamOutOfRangeIsNoOp(t *testing.T) {
	for name, voice := range newTestVoices() {
		before := make([]float64, maxVoiceParams)
		for i := range before {
			before[i] = voice.Param(i)
		}

		for _, index := range []int{-1, len(voice.ParamSpecs()), maxVoiceParams, math.MaxInt} {
			voice.SetParam(index, 0.9)
		}

		for _, bad := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
			for i := range voice.ParamSpecs() {
				voice.SetParam(i, bad)
			}
		}

		for i := range before {
			if got := voice.Param(i); got != before[i] {
				t.Errorf("%s param %d changed to %v after bad input, want %v",
					name, i, got, before[i])
			}
		}
	}
}

// TestVoiceParamExtremesStayFiniteAndTerminate sweeps every parameter to both
// ends against both ends of the strip decay trim.
//
// The bounds are looser than TestVoicesDecayToInactiveWithBoundedOutput's
// because that test only covers the defaults. At the extremes a resonant
// bandpass (hat.bpQ / cym.bpQ at max) plus full make-up gain rings to ~4.4,
// which the master chain's 0.5 mix headroom and -1 dBFS limiter absorb; and a
// 4 s cymbal base decay trimmed 1.5x takes ~55 s to fall below envSilence,
// past the 30 s cap tickUntilInactive uses elsewhere.
func TestVoiceParamExtremesStayFiniteAndTerminate(t *testing.T) {
	const (
		maxSamples = int(testSampleRate * 90)
		peakBound  = 6.0
	)

	for name := range newTestVoices() {
		specCount := len(newTestVoices()[name].ParamSpecs())

		for index := range specCount {
			for _, value := range []float64{0, 1} {
				for _, decay := range []float64{0, 1} {
					voice := newTestVoices()[name]
					voice.SetParam(index, value)
					voice.SetDecay(decay)
					voice.Trigger(1)

					var (
						peak  float64
						ticks int
					)

					for ticks = range maxSamples {
						sample := voice.Tick()
						if math.IsNaN(sample) || math.IsInf(sample, 0) {
							t.Fatalf("%s param %d=%v decay=%v: non-finite sample at %d",
								name, index, value, decay, ticks)
						}

						if abs := math.Abs(sample); abs > peak {
							peak = abs
						}

						if !voice.IsActive() {
							break
						}
					}

					if voice.IsActive() {
						t.Fatalf("%s param %d=%v decay=%v still active after %d samples",
							name, index, value, decay, maxSamples)
					}

					if peak > peakBound {
						t.Fatalf("%s param %d=%v decay=%v peak %v exceeds bound %v",
							name, index, value, decay, peak, peakBound)
					}
				}
			}
		}
	}
}

// TestVoiceLevelParamsCanSilenceAVoice checks the zero end of every level /
// mix parameter actually reaches silence, so the modal can dial a layer out.
func TestVoiceLevelParamsCanSilenceAVoice(t *testing.T) {
	cases := map[string]int{
		"HiHat":  hatParamGain,
		"Tom":    tomParamGain,
		"Cymbal": cymParamGain,
	}

	for name, index := range cases {
		voice := newTestVoices()[name]
		voice.SetParam(index, 0)

		for _, sample := range renderVoice(voice, 4800) {
			if sample != 0 {
				t.Fatalf("%s at zero level produced %v, want silence", name, sample)
			}
		}
	}

	// The Snare's MIX only silences the body layer; the noise layer remains.
	snare := newTestVoices()["Snare"]
	snare.SetParam(snareParamToneLevel, 0)

	var peak float64

	for _, sample := range renderVoice(snare, 4800) {
		if abs := math.Abs(sample); abs > peak {
			peak = abs
		}
	}

	if peak == 0 {
		t.Fatal("Snare at zero body level is fully silent, want the noise layer to remain")
	}
}

// TestFilterParamChangeKeepsTail pins the deliberate choice in redesign(): the
// biquad's coefficients are swapped without touching its delay line, so a hit
// already ringing keeps ringing. Calling Reset() there would zero the tail
// mid-hit — an audible click.
func TestFilterParamChangeKeepsTail(t *testing.T) {
	cases := map[string]int{
		"Snare":  snareParamHPQ,
		"HiHat":  hatParamBPQ,
		"Cymbal": cymParamBPQ,
	}

	for name, index := range cases {
		voice := newTestVoices()[name]
		voice.Trigger(1)

		var before float64

		for range 100 {
			before = voice.Tick()
		}

		voice.SetParam(index, 0.9)

		after := voice.Tick()
		if math.IsNaN(after) || math.IsInf(after, 0) {
			t.Fatalf("%s produced %v right after a mid-tail filter change", name, after)
		}

		if !voice.IsActive() {
			t.Fatalf("%s deactivated by a mid-tail filter change", name)
		}

		// A coefficient swap changes the timbre, but the envelope and the
		// filter state carry over — the output must not collapse to silence.
		if after == 0 && before != 0 {
			t.Fatalf("%s output dropped to exactly 0 after a filter change (state was reset)", name)
		}
	}
}

// TestVoicesAudibleAtLowSampleRate guards the silent-voice failure clampDesignHz
// fixes: the engine accepts rates down to 8 kHz, where the shipped 10 kHz
// hi-hat bandpass centre sits above Nyquist and design.Bandpass returns
// all-zero coefficients.
func TestVoicesAudibleAtLowSampleRate(t *testing.T) {
	const lowRate = 8000

	voices := map[string]Voice{
		"BassDrum": NewBassDrum(lowRate),
		"Snare":    NewSnare(lowRate),
		"HiHat":    NewHiHat(lowRate),
		"Tom":      NewTom(lowRate),
		"Cymbal":   NewCymbal(lowRate),
	}

	for name, voice := range voices {
		var peak float64

		for _, sample := range renderVoice(voice, 4000) {
			if math.IsNaN(sample) || math.IsInf(sample, 0) {
				t.Fatalf("%s produced %v at %d Hz", name, sample, lowRate)
			}

			if abs := math.Abs(sample); abs > peak {
				peak = abs
			}
		}

		if peak == 0 {
			t.Fatalf("%s is silent at %d Hz", name, lowRate)
		}
	}
}

// TestVoiceDecayParamAndStripKnobCompose checks the two decay controls stay
// independent: effective decay = base parameter x (decayScaleMin + strip).
func TestVoiceDecayParamAndStripKnobCompose(t *testing.T) {
	cases := map[string]int{
		"BassDrum": bassParamDecay,
		"HiHat":    hatParamDecay,
		"Tom":      tomParamDecay,
		"Cymbal":   cymParamDecay,
	}

	for name, index := range cases {
		lengthFor := func(base, strip float64) int {
			voice := newTestVoices()[name]
			voice.SetParam(index, base)
			voice.SetDecay(strip)
			voice.Trigger(1)

			length, _ := tickUntilInactive(t, name, voice)

			return length
		}

		// A longer base rings longer at a fixed strip position...
		if short, long := lengthFor(0.2, 0.5), lengthFor(0.6, 0.5); long <= short {
			t.Errorf("%s: base 0.6 length %d not longer than base 0.2 length %d", name, long, short)
		}

		// ...and the strip trim still works at any base.
		if short, long := lengthFor(0.2, 0), lengthFor(0.2, 1); long <= short {
			t.Errorf("%s: strip 1 length %d not longer than strip 0 length %d", name, long, short)
		}
	}
}

// TestPitchedVoicesAcceptInvertedSweep covers a user dragging the attack pitch
// below the body pitch: the sweep simply rises instead of falling.
func TestPitchedVoicesAcceptInvertedSweep(t *testing.T) {
	cases := map[string][2]int{
		"BassDrum": {bassParamPitchFrom, bassParamPitchTo},
		"Tom":      {tomParamPitchFrom, tomParamPitchTo},
	}

	for name, indices := range cases {
		voice := newTestVoices()[name]
		voice.SetParam(indices[0], 0) // attack at the bottom of its range
		voice.SetParam(indices[1], 1) // body at the top of its range

		var peak float64

		for _, sample := range renderVoice(voice, 4800) {
			if math.IsNaN(sample) || math.IsInf(sample, 0) {
				t.Fatalf("%s produced %v with an inverted sweep", name, sample)
			}

			if abs := math.Abs(sample); abs > peak {
				peak = abs
			}
		}

		if peak == 0 {
			t.Fatalf("%s is silent with an inverted sweep", name)
		}
	}
}
