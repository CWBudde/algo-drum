package drum

import (
	"math"
	"testing"
)

func newTestVoices() map[string]Voice {
	return map[string]Voice{
		"BassDrum": NewBassDrum(testSampleRate),
		"Snare":    NewSnare(testSampleRate),
		"HiHat":    NewHiHat(testSampleRate, true),
		"Tom":      NewTom(testSampleRate),
		"Cymbal":   NewCymbal(testSampleRate),
	}
}

// tickUntilInactive runs a voice until it deactivates, returning the number
// of samples it stayed active and the absolute peak it produced.
func tickUntilInactive(t *testing.T, name string, voice Voice) (int, float64) {
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
		voice.Trigger()

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

func TestVoicesLongerDecaySettingRingsLonger(t *testing.T) {
	for name := range newTestVoices() {
		shortVoice := newTestVoices()[name]
		longVoice := newTestVoices()[name]

		shortVoice.SetDecay(0)
		longVoice.SetDecay(1)

		shortVoice.Trigger()
		longVoice.Trigger()

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

		first.Trigger()
		second.Trigger()

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
		voice.Trigger()
		tickUntilInactive(t, name, voice)

		voice.Trigger()

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
