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

	for step := 1; step < MaxSteps; step++ {
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
		if diff := straight - swung; diff < 0 || diff > MaxSteps {
			t.Fatalf("swing %.2f changed bar length by %d samples", swing, diff)
		}
	}
}

func TestSixteenStepsSpanOneBar(t *testing.T) {
	engine := NewEngine(testSampleRate)
	engine.SetTempo(120)

	// Steps are 16th notes: 16 of them must add up to one 4/4 bar
	// (4 beats = 2 s at 120 BPM), give or take rounding per step.
	var total int64

	for _, length := range engine.stepLen {
		total += length
	}

	wantBar := int64(testSampleRate * 60.0 / 120.0 * 4.0)
	if diff := wantBar - total; diff < 0 || diff > MaxSteps {
		t.Fatalf("16 steps span %d samples, want one bar = %d", total, wantBar)
	}
}

func TestSetStepCountClamps(t *testing.T) {
	engine := NewEngine(testSampleRate)

	if engine.stepCount != MaxSteps {
		t.Fatalf("default stepCount = %d, want %d", engine.stepCount, MaxSteps)
	}

	engine.SetStepCount(0)

	if engine.stepCount != 1 {
		t.Fatalf("SetStepCount(0): stepCount = %d, want clamp to 1", engine.stepCount)
	}

	engine.SetStepCount(99)

	if engine.stepCount != MaxSteps {
		t.Fatalf("SetStepCount(99): stepCount = %d, want clamp to %d", engine.stepCount, MaxSteps)
	}

	engine.SetStepCount(8)

	if engine.stepCount != 8 {
		t.Fatalf("SetStepCount(8): stepCount = %d, want 8", engine.stepCount)
	}
}

func TestSetStepCountWrapsCurrentStep(t *testing.T) {
	engine := NewEngine(testSampleRate)
	engine.SetRunning(true)
	engine.currentStep = 10

	engine.SetStepCount(4)

	if got := engine.CurrentStep(); got != 10%4 {
		t.Fatalf("shrinking to 4 steps left currentStep = %d, want %d", got, 10%4)
	}
}

func TestStepAdvanceWrapsAtStepCount(t *testing.T) {
	engine := NewEngine(testSampleRate)
	engine.SetSwing(0)
	engine.SetStepCount(4)
	engine.SetRunning(true)

	// Render six steps' worth of audio one step at a time; the step index
	// must stay inside the shortened loop the whole way.
	for i := 0; i < 6; i++ {
		if got := engine.CurrentStep(); got != i%4 {
			t.Fatalf("after %d steps CurrentStep() = %d, want %d", i, got, i%4)
		}

		renderTotal(engine, int(engine.stepLen[0]))
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

func TestVolumeChangeRampsSmoothly(t *testing.T) {
	engine := NewEngine(testSampleRate)

	// Drop the target from 1 to 0: one rendered sample may only move the
	// applied gain by the one-pole coefficient, not jump instantly.
	engine.SetVolume(0, 0)
	renderTotal(engine, 1)

	if engine.liveVol[0] < 1-2*engine.volCoef {
		t.Fatalf("live volume jumped to %v after one sample (coef %v)", engine.liveVol[0], engine.volCoef)
	}

	// After many time constants the ramp must have converged.
	renderTotal(engine, int(testSampleRate/10))

	if engine.liveVol[0] > 0.01 {
		t.Fatalf("live volume %v has not converged to target 0 after 100 ms", engine.liveVol[0])
	}
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

	engine.SetCell(-1, 0, 1)
	engine.SetCell(TrackCount, 0, 1)
	engine.SetCell(0, -1, 1)
	engine.SetCell(0, MaxSteps, 1)

	for track := range engine.pattern {
		for step := range engine.pattern[track] {
			if engine.pattern[track][step] != 0 {
				t.Fatalf("out-of-range SetCell activated cell (%d, %d)", track, step)
			}
		}
	}
}

func TestSetCellClampsVelocity(t *testing.T) {
	engine := NewEngine(testSampleRate)

	engine.SetCell(0, 0, 2.5)

	if engine.pattern[0][0] != 1 {
		t.Fatalf("SetCell velocity 2.5 stored %v, want clamp to 1", engine.pattern[0][0])
	}

	engine.SetCell(0, 0, -1)

	if engine.pattern[0][0] != 0 {
		t.Fatalf("SetCell velocity -1 stored %v, want clamp to 0", engine.pattern[0][0])
	}
}

func TestSetPatternPatternRoundtrip(t *testing.T) {
	engine := NewEngine(testSampleRate)

	in := make([]float64, TrackCount*MaxSteps)
	for i := range in {
		in[i] = float64(i%4) / 4.0 // 0, 0.25, 0.5, 0.75, ...
	}

	engine.SetPattern(in)

	out := engine.Pattern()
	if len(out) != TrackCount*MaxSteps {
		t.Fatalf("Pattern() length = %d, want %d", len(out), TrackCount*MaxSteps)
	}

	for i := range in {
		if out[i] != in[i] {
			t.Fatalf("pattern[%d] = %v after roundtrip, want %v", i, out[i], in[i])
		}
	}
}

func TestSetPatternClampsAndTolerantLengths(t *testing.T) {
	engine := NewEngine(testSampleRate)

	// Out-of-range velocities clamp; over-long input must not panic.
	over := make([]float64, TrackCount*MaxSteps+7)
	for i := range over {
		over[i] = 5
	}

	engine.SetPattern(over)

	if engine.pattern[0][0] != 1 || engine.pattern[TrackCount-1][MaxSteps-1] != 1 {
		t.Fatal("SetPattern did not clamp velocities to 1")
	}

	// A short slice updates only the cells it covers.
	engine.SetPattern([]float64{0.5})

	if engine.pattern[0][0] != 0.5 {
		t.Fatalf("short SetPattern: cell (0,0) = %v, want 0.5", engine.pattern[0][0])
	}

	if engine.pattern[0][1] != 1 {
		t.Fatalf("short SetPattern touched cell (0,1): %v, want untouched 1", engine.pattern[0][1])
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
	engine.SetCell(0, 0, 1)
	engine.SetRunning(true)

	buf := renderTotal(engine, int(engine.stepLen[0]))

	if peak := peakOf(buf); peak < 0.05 {
		t.Fatalf("pattern set before start rendered peak %v, want audible output", peak)
	}
}

func TestRenderVelocityScalesOutput(t *testing.T) {
	renderHit := func(velocity float64) float64 {
		engine := NewEngine(testSampleRate)
		engine.SetCell(0, 0, velocity)
		engine.SetRunning(true)

		return peakOf(renderTotal(engine, int(engine.stepLen[0])))
	}

	full := renderHit(1.0)
	half := renderHit(0.5)

	if full <= 0 || half <= 0 {
		t.Fatalf("hits rendered no output: full=%v half=%v", full, half)
	}

	// The signal path below the limiter threshold is linear, so half
	// velocity must come out at ~half the peak.
	if ratio := half / full; math.Abs(ratio-0.5) > 0.05 {
		t.Fatalf("velocity 0.5 peak ratio = %.3f, want ~0.5 (full=%v half=%v)", ratio, full, half)
	}
}

func TestRenderOutputBoundedAndFinite(t *testing.T) {
	engine := NewEngine(testSampleRate)

	// Worst case: everything on with accent, full volume, full reverb,
	// long decays.
	for track := 0; track < TrackCount; track++ {
		engine.SetVolume(track, 1)
		engine.SetDecay(track, 1)

		for step := 0; step < MaxSteps; step++ {
			engine.SetCell(track, step, 1)
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
		engine.SetCell(0, 0, 1)
		engine.SetCell(1, 2, 0.7)
		engine.SetCell(2, 4, 1)
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
