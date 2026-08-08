package drum

import (
	"math"
	"testing"
)

func TestMetronomeClickIsDeterministicFiniteAndAccented(t *testing.T) {
	regular := newMetronomeClick(testSampleRate)
	accent := newMetronomeClick(testSampleRate)
	repeat := newMetronomeClick(testSampleRate)
	regular.Trigger(false)
	accent.Trigger(true)
	repeat.Trigger(false)

	var regularPeak, accentPeak float64
	for range int(testSampleRate / 10) {
		r := regular.Tick()
		a := accent.Tick()
		if r != repeat.Tick() {
			t.Fatal("identically triggered clicks diverged")
		}
		if math.IsNaN(r) || math.IsInf(r, 0) || math.IsNaN(a) || math.IsInf(a, 0) {
			t.Fatal("click produced a non-finite sample")
		}
		regularPeak = max(regularPeak, math.Abs(r))
		accentPeak = max(accentPeak, math.Abs(a))
	}

	if regularPeak == 0 || accentPeak <= regularPeak {
		t.Fatalf("regular/accent peaks = %.4f/%.4f, want a louder accent", regularPeak, accentPeak)
	}
	if regular.Tick() != 0 || accent.Tick() != 0 {
		t.Fatal("click did not decay to exact inactivity")
	}
}

func TestCountInUsesFixedBarsAndQuarterClicks(t *testing.T) {
	engine := NewEngine(testSampleRate)
	engine.limiter = nil
	engine.reverb = nil
	engine.SetStepCount(0, 7) // count-in length is independent of pattern length
	engine.SetCountInBars(1)
	engine.BeginStart()
	engine.SetRunning(true)

	if got := engine.TransportSnapshot(); got.State != TransportCountingIn || got.Step != -1 || got.Revision != 2 {
		t.Fatalf("start snapshot = %+v, want counting-in@2 with no playhead", got)
	}

	quarterSamples := int(testSampleRate * secondsPerMinute / 120)
	first := renderTotal(engine, 1)[0]
	if math.Abs(float64(first)-metronomeAccentLevel) > 1e-6 {
		t.Fatalf("count-in downbeat = %v, want accent level %v", first, metronomeAccentLevel)
	}

	sixteenthSamples := quarterSamples / int(stepsPerBeat)
	renderTotal(engine, sixteenthSamples-1)
	betweenBeats := renderTotal(engine, 1)[0]
	if math.Abs(float64(betweenBeats)) > 1e-3 {
		t.Fatalf("count-in retriggered on a sixteenth-note step: %v", betweenBeats)
	}
	renderTotal(engine, quarterSamples-sixteenthSamples-1)
	regular := renderTotal(engine, 1)[0]
	if math.Abs(float64(regular)-metronomeRegularLevel) > 1e-6 {
		t.Fatalf("count-in quarter = %v, want regular level %v", regular, metronomeRegularLevel)
	}

	// We consumed one quarter note plus one sample; finish the fixed 16-step
	// bar and verify that the pattern has not triggered during the pre-roll.
	renderTotal(engine, 4*quarterSamples-quarterSamples-1)
	if got := engine.TransportSnapshot(); got.State != TransportPlaying || got.Step != 0 || got.Revision != 3 {
		t.Fatalf("completed count-in snapshot = %+v, want playing step 0@3", got)
	}
	if engine.stepTriggered {
		t.Fatal("count-in triggered the pattern before entering playback")
	}
}

func TestPauseDuringCountInSkipsItOnResume(t *testing.T) {
	engine := NewEngine(testSampleRate)
	engine.SetCountInBars(2)
	engine.BeginStart()
	engine.SetRunning(true)
	renderTotal(engine, 100)

	engine.Pause()
	if got := engine.TransportSnapshot(); got.State != TransportPaused || got.Step != 0 {
		t.Fatalf("paused count-in snapshot = %+v, want paused at pattern step 0", got)
	}
	engine.BeginStart()
	engine.SetRunning(true)
	if got := engine.TransportSnapshot(); got.State != TransportPlaying || got.Step != 0 {
		t.Fatalf("resumed snapshot = %+v, want immediate playback", got)
	}
}

func TestPausedPlaybackResumesWithoutAnotherCountIn(t *testing.T) {
	engine := NewEngine(testSampleRate)
	engine.SetCountInBars(1)
	engine.SetRunning(true)
	renderTotal(engine, int(4*testSampleRate*secondsPerMinute/120))
	if engine.TransportSnapshot().State != TransportPlaying {
		t.Fatal("initial count-in did not complete")
	}

	engine.Pause()
	engine.BeginStart()
	engine.SetRunning(true)
	if got := engine.TransportSnapshot().State; got != TransportPlaying {
		t.Fatalf("paused transport resumed as %q, want immediate playing", got)
	}
}

func TestMetronomeOnlySoundsWhenEnabledDuringPlayback(t *testing.T) {
	build := func(enabled bool) *Engine {
		engine := NewEngine(testSampleRate)
		engine.limiter = nil
		engine.reverb = nil
		engine.SetMetronomeEnabled(enabled)
		engine.SetProbability(0)
		for track := range TrackCount {
			engine.SetMuted(track, true)
		}
		engine.SetRunning(true)

		return engine
	}

	if peak := peakOf(renderTotal(build(false), 1024)); peak != 0 {
		t.Fatalf("disabled metronome peak = %v, want silence", peak)
	}
	if peak := peakOf(renderTotal(build(true), 1024)); peak == 0 {
		t.Fatal("enabled metronome was silenced by probability or track mutes")
	}
}

func TestCountInSettersClampAndRoundTripState(t *testing.T) {
	engine := NewEngine(testSampleRate)
	engine.SetMetronomeEnabled(true)
	engine.SetCountInBars(99)
	state := engine.State()
	if !state.MetronomeEnabled || state.CountInBars != MaxCountInBars {
		t.Fatalf("state metronome/count-in = %v/%d", state.MetronomeEnabled, state.CountInBars)
	}

	state.CountInBars = -8
	if err := engine.ReplaceState(state); err != nil {
		t.Fatal(err)
	}
	if got := engine.State().CountInBars; got != 0 {
		t.Fatalf("restored count-in bars = %d, want clamped 0", got)
	}
}

func TestDefensiveBoundaryFallbackAdvancesOneStep(t *testing.T) {
	engine := NewEngine(testSampleRate)
	engine.commitUnscheduledBoundary()
	if engine.currentStep != 1 || engine.clockStep != 1 {
		t.Fatalf("fallback advanced to step/clock %d/%d, want 1/1", engine.currentStep, engine.clockStep)
	}
}
