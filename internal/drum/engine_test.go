package drum

import (
	"math"
	"testing"
)

const testSampleRate = 48000.0

func renderTotal(engine *Engine, samples int) []float32 {
	buf := make([]float32, samples)
	engine.Render(buf)

	return buf
}

func peakOf(buf []float32) float64 {
	var peak float64

	for _, sample := range buf {
		if abs := math.Abs(float64(sample)); abs > peak {
			peak = abs
		}
	}

	return peak
}

func TestStepLengthsNoSwingAreEqual(t *testing.T) {
	engine := NewEngine(testSampleRate)
	engine.SetSwing(0)

	for step := 1; step < StepCount; step++ {
		if engine.stepLen[step] != engine.stepLen[0] {
			t.Fatalf("step %d length %d != step 0 length %d", step, engine.stepLen[step], engine.stepLen[0])
		}
	}
}

func TestStepLengthsFullSwingRatio(t *testing.T) {
	engine := NewEngine(testSampleRate)
	engine.SetSwing(0.5)

	// swing=0.5 must give even steps 1.5× base and odd steps 0.5× base,
	// i.e. a 3:1 ratio — this was previously halved by double scaling.
	longStep := float64(engine.stepLen[0])
	shortStep := float64(engine.stepLen[1])

	ratio := longStep / shortStep
	if math.Abs(ratio-3.0) > 0.01 {
		t.Fatalf("full swing ratio = %.3f, want 3.0", ratio)
	}
}

func TestStepLengthsSwingPreservesBarLength(t *testing.T) {
	engine := NewEngine(testSampleRate)

	engine.SetSwing(0)

	var straight int64

	for _, length := range engine.stepLen {
		straight += length
	}

	for _, swing := range []float64{0.1, 0.25, 0.5} {
		engine.SetSwing(swing)

		var swung int64

		for _, length := range engine.stepLen {
			swung += length
		}

		// Rounding may cost at most one sample per step.
		if diff := straight - swung; diff < 0 || diff > StepCount {
			t.Fatalf("swing %.2f changed bar length by %d samples", swing, diff)
		}
	}
}

func TestSetTempoClamps(t *testing.T) {
	engine := NewEngine(testSampleRate)

	engine.SetTempo(0)

	if engine.bpm != 30 {
		t.Fatalf("SetTempo(0): bpm = %v, want clamp to 30", engine.bpm)
	}

	engine.SetTempo(-100)

	if engine.bpm != 30 {
		t.Fatalf("SetTempo(-100): bpm = %v, want clamp to 30", engine.bpm)
	}

	engine.SetTempo(10000)

	if engine.bpm != 300 {
		t.Fatalf("SetTempo(10000): bpm = %v, want clamp to 300", engine.bpm)
	}

	for step, length := range engine.stepLen {
		if length <= 0 {
			t.Fatalf("step %d has non-positive length %d after clamped tempi", step, length)
		}
	}
}

func TestSetSwingClamps(t *testing.T) {
	engine := NewEngine(testSampleRate)

	engine.SetSwing(-1)

	if engine.swing != 0 {
		t.Fatalf("SetSwing(-1): swing = %v, want 0", engine.swing)
	}

	engine.SetSwing(5)

	if engine.swing != 0.5 {
		t.Fatalf("SetSwing(5): swing = %v, want 0.5", engine.swing)
	}
}

func TestSetVolumeClamps(t *testing.T) {
	engine := NewEngine(testSampleRate)

	engine.SetVolume(0, -5)

	if engine.volumes[0] != 0 {
		t.Fatalf("SetVolume(0, -5): volume = %v, want 0", engine.volumes[0])
	}

	engine.SetVolume(0, 7)

	if engine.volumes[0] != 1 {
		t.Fatalf("SetVolume(0, 7): volume = %v, want 1", engine.volumes[0])
	}

	// Out-of-range tracks must be ignored without panicking.
	engine.SetVolume(-1, 0.5)
	engine.SetVolume(TrackCount, 0.5)
}

func TestSetDecayClamps(t *testing.T) {
	engine := NewEngine(testSampleRate)

	engine.SetDecay(1, -2)

	if engine.decays[1] != 0 {
		t.Fatalf("SetDecay(1, -2): decay = %v, want 0", engine.decays[1])
	}

	engine.SetDecay(1, 3)

	if engine.decays[1] != 1 {
		t.Fatalf("SetDecay(1, 3): decay = %v, want 1", engine.decays[1])
	}

	engine.SetDecay(-1, 0.5)
	engine.SetDecay(TrackCount, 0.5)
}

func TestSetCellIgnoresOutOfRange(t *testing.T) {
	engine := NewEngine(testSampleRate)

	engine.SetCell(-1, 0, true)
	engine.SetCell(TrackCount, 0, true)
	engine.SetCell(0, -1, true)
	engine.SetCell(0, StepCount, true)

	for track := range engine.pattern {
		for step := range engine.pattern[track] {
			if engine.pattern[track][step] {
				t.Fatalf("out-of-range SetCell activated cell (%d, %d)", track, step)
			}
		}
	}
}

func TestCurrentStepLifecycle(t *testing.T) {
	engine := NewEngine(testSampleRate)

	if got := engine.CurrentStep(); got != -1 {
		t.Fatalf("stopped engine CurrentStep() = %d, want -1", got)
	}

	engine.SetRunning(true)

	if got := engine.CurrentStep(); got != 0 {
		t.Fatalf("freshly started CurrentStep() = %d, want 0", got)
	}

	// Render slightly more than one step; the step index must advance.
	renderTotal(engine, int(engine.stepLen[0])+1)

	if got := engine.CurrentStep(); got != 1 {
		t.Fatalf("after one step of audio CurrentStep() = %d, want 1", got)
	}

	engine.SetRunning(false)

	if got := engine.CurrentStep(); got != -1 {
		t.Fatalf("stopped CurrentStep() = %d, want -1", got)
	}

	engine.SetRunning(true)

	if got := engine.CurrentStep(); got != 0 {
		t.Fatalf("restart CurrentStep() = %d, want 0 (stop must reset)", got)
	}
}

func TestRenderSilentWithEmptyPattern(t *testing.T) {
	engine := NewEngine(testSampleRate)
	engine.SetRunning(true)

	buf := renderTotal(engine, int(testSampleRate/2))

	if peak := peakOf(buf); peak != 0 {
		t.Fatalf("empty pattern rendered peak %v, want silence", peak)
	}
}

func TestRenderPlaysPatternSetBeforeStart(t *testing.T) {
	engine := NewEngine(testSampleRate)

	// Program the pattern BEFORE starting — the original bug dropped this.
	engine.SetCell(0, 0, true)
	engine.SetRunning(true)

	buf := renderTotal(engine, int(engine.stepLen[0]))

	if peak := peakOf(buf); peak < 0.05 {
		t.Fatalf("pattern set before start rendered peak %v, want audible output", peak)
	}
}

func TestRenderOutputBoundedAndFinite(t *testing.T) {
	engine := NewEngine(testSampleRate)

	// Worst case: everything on, full volume, full reverb, long decays.
	for track := 0; track < TrackCount; track++ {
		engine.SetVolume(track, 1)
		engine.SetDecay(track, 1)

		for step := 0; step < StepCount; step++ {
			engine.SetCell(track, step, true)
		}
	}

	engine.SetReverb(1)
	engine.SetTempo(300)
	engine.SetRunning(true)

	buf := renderTotal(engine, int(testSampleRate*2))

	for i, sample := range buf {
		value := float64(sample)
		if math.IsNaN(value) || math.IsInf(value, 0) {
			t.Fatalf("sample %d is not finite: %v", i, value)
		}

		if math.Abs(value) > 1.0 {
			t.Fatalf("sample %d = %v exceeds ±1.0 output ceiling", i, value)
		}
	}

	if peak := peakOf(buf); peak < 0.1 {
		t.Fatalf("worst-case pattern rendered peak %v, expected loud output", peak)
	}
}

func TestRenderDeterministic(t *testing.T) {
	build := func() *Engine {
		engine := NewEngine(testSampleRate)
		engine.SetCell(0, 0, true)
		engine.SetCell(1, 2, true)
		engine.SetCell(2, 4, true)
		engine.SetReverb(0.5)
		engine.SetRunning(true)

		return engine
	}

	first := renderTotal(build(), int(testSampleRate))
	second := renderTotal(build(), int(testSampleRate))

	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("sample %d differs between identical engines: %v vs %v", i, first[i], second[i])
		}
	}
}
