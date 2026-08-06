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

func samplesForStep(engine *Engine, step int) int {
	return int((engine.stepDuration[step] + stepPhaseUnit - 1) / stepPhaseUnit)
}

func TestStepLengthsNoSwingAreEqual(t *testing.T) {
	engine := NewEngine(testSampleRate)
	engine.SetSwing(0)

	for step := 1; step < MaxSteps; step++ {
		if engine.stepDuration[step] != engine.stepDuration[0] {
			t.Fatalf("step %d duration %d != step 0 duration %d",
				step, engine.stepDuration[step], engine.stepDuration[0])
		}
	}
}

func TestStepLengthsFullSwingRatio(t *testing.T) {
	engine := NewEngine(testSampleRate)
	engine.SetSwing(0.5)

	// swing=0.5 must give even steps 1.5× base and odd steps 0.5× base,
	// i.e. a 3:1 ratio — this was previously halved by double scaling.
	longStep := float64(engine.stepDuration[0])
	shortStep := float64(engine.stepDuration[1])

	ratio := longStep / shortStep
	if math.Abs(ratio-3.0) > 0.01 {
		t.Fatalf("full swing ratio = %.3f, want 3.0", ratio)
	}
}

func TestStepLengthsSwingPreservesBarLength(t *testing.T) {
	engine := NewEngine(testSampleRate)

	engine.SetSwing(0)

	var straight uint64

	for _, length := range engine.stepDuration {
		straight += length
	}

	for _, swing := range []float64{0.1, 0.25, 0.5} {
		engine.SetSwing(swing)

		var swung uint64

		for _, length := range engine.stepDuration {
			swung += length
		}

		if swung != straight {
			t.Fatalf("swing %.2f changed fixed-point bar length from %d to %d", swing, straight, swung)
		}
	}
}

func TestSixteenStepsSpanOneBar(t *testing.T) {
	engine := NewEngine(testSampleRate)
	engine.SetTempo(120)

	// Steps are 16th notes: 16 of them must add up to one 4/4 bar
	// (4 beats = 2 s at 120 BPM), give or take rounding per step.
	var total uint64

	for _, length := range engine.stepDuration {
		total += length
	}

	wantBar := uint64(testSampleRate*60.0/120.0*4.0) * stepPhaseUnit
	if total != wantBar {
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

		renderTotal(engine, samplesForStep(engine, 0))
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

	for step, duration := range engine.stepDuration {
		if duration == 0 {
			t.Fatalf("step %d has zero duration after clamped tempi", step)
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

	var out [PatternSize]float32
	engine.CopyPattern(&out)
	if len(out) != TrackCount*MaxSteps {
		t.Fatalf("CopyPattern length = %d, want %d", len(out), PatternSize)
	}

	for i := range in {
		if float64(out[i]) != in[i] {
			t.Fatalf("pattern[%d] = %v after roundtrip, want %v", i, out[i], in[i])
		}
	}
}

func TestSetPatternClampsAndRejectsWrongLengths(t *testing.T) {
	engine := NewEngine(testSampleRate)

	full := make([]float64, PatternSize)
	for i := range full {
		full[i] = 5
	}

	engine.SetPattern(full)

	if engine.pattern[0][0] != 1 || engine.pattern[TrackCount-1][MaxSteps-1] != 1 {
		t.Fatal("SetPattern did not clamp velocities to 1")
	}

	// A partial or version-skewed snapshot is rejected atomically rather than
	// being merged into or erasing part of the current pattern.
	engine.SetPattern([]float64{0.5})
	engine.SetPattern(make([]float64, PatternSize+1))
	for track := range engine.pattern {
		for step, velocity := range engine.pattern[track] {
			if velocity != 1 {
				t.Fatalf("wrong-sized snapshot changed cell (%d,%d) to %v", track, step, velocity)
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
	renderTotal(engine, samplesForStep(engine, 0)+1)

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

	buf := renderTotal(engine, samplesForStep(engine, 0))

	if peak := peakOf(buf); peak < 0.05 {
		t.Fatalf("pattern set before start rendered peak %v, want audible output", peak)
	}
}

func TestRenderVelocityScalesOutput(t *testing.T) {
	renderHit := func(velocity float64) float64 {
		engine := NewEngine(testSampleRate)
		engine.SetCell(0, 0, velocity)
		engine.SetRunning(true)

		return peakOf(renderTotal(engine, samplesForStep(engine, 0)))
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

// firstOnset returns the index of the first sample whose magnitude crosses a
// small threshold, or -1 if the buffer stays silent.
func firstOnset(buf []float32) int {
	for i, sample := range buf {
		if math.Abs(float64(sample)) > 1e-3 {
			return i
		}
	}

	return -1
}

func TestProbabilityZeroNeverTriggers(t *testing.T) {
	engine := NewEngine(testSampleRate)

	// Dense pattern: a hit on every cell of every track.
	for track := 0; track < TrackCount; track++ {
		for step := 0; step < MaxSteps; step++ {
			engine.SetCell(track, step, 1)
		}
	}

	engine.SetProbability(0)
	engine.SetRunning(true)

	buf := renderTotal(engine, int(testSampleRate*2))
	if peak := peakOf(buf); peak != 0 {
		t.Fatalf("probability 0 rendered peak %v, want silence", peak)
	}
}

func TestProbabilityOneAlwaysTriggers(t *testing.T) {
	engine := NewEngine(testSampleRate)
	engine.SetSwing(0)
	engine.SetStepCount(1) // one-step loop: a bass hit on every step
	engine.SetCell(0, 0, 1)
	engine.SetProbability(1)
	engine.SetRunning(true)

	// Every step must produce an onset near its boundary.
	stepLen := samplesForStep(engine, 0)
	for step := 0; step < 8; step++ {
		buf := renderTotal(engine, stepLen)
		if firstOnset(buf) < 0 {
			t.Fatalf("probability 1: step %d produced no onset", step)
		}
	}
}

func TestHumanizeZeroIsSampleExact(t *testing.T) {
	build := func(setHumanize bool) *Engine {
		engine := NewEngine(testSampleRate)
		engine.SetCell(0, 0, 1)
		engine.SetCell(2, 4, 0.7)

		if setHumanize {
			engine.SetHumanize(0)
		}

		engine.SetRunning(true)

		return engine
	}

	reference := renderTotal(build(false), int(testSampleRate))
	explicit := renderTotal(build(true), int(testSampleRate))

	for i := range reference {
		if reference[i] != explicit[i] {
			t.Fatalf("humanize 0 changed sample %d: %v vs %v", i, reference[i], explicit[i])
		}
	}
}

func TestHumanizeTimingStaysWithinBounds(t *testing.T) {
	maxDelay := int(humanizeTimingMaxS * testSampleRate)

	// Many trials: with a single hit on step 0, the onset must never land
	// before the boundary nor later than the maximum timing jitter.
	for trial := 0; trial < 200; trial++ {
		engine := NewEngine(testSampleRate)
		engine.SetHumanize(1)
		engine.SetCell(0, 0, 1)
		engine.SetRunning(true)

		buf := renderTotal(engine, maxDelay*4)

		onset := firstOnset(buf)
		if onset < 0 {
			t.Fatalf("trial %d: humanized hit never fired", trial)
		}

		// Allow a couple of samples of slack for envelope ramp-up.
		if onset > maxDelay+4 {
			t.Fatalf("trial %d: onset at %d exceeds max jitter %d", trial, onset, maxDelay)
		}
	}
}

func TestHumanizeRenderDoesNotAllocate(t *testing.T) {
	engine := NewEngine(testSampleRate)
	engine.SetHumanize(1)
	engine.SetReverb(0) // isolate the humanize path from reverb internals

	for track := 0; track < TrackCount; track++ {
		for step := 0; step < MaxSteps; step++ {
			engine.SetCell(track, step, 1)
		}
	}

	engine.SetTempo(300) // fastest tempo → most step boundaries per render
	engine.SetRunning(true)

	buf := make([]float32, 4096)
	allocs := testing.AllocsPerRun(50, func() {
		engine.Render(buf)
	})

	if allocs != 0 {
		t.Fatalf("humanized Render allocated %v times per call, want 0", allocs)
	}
}

func TestHumanizeLongRenderIsFinite(t *testing.T) {
	engine := NewEngine(testSampleRate)
	engine.SetHumanize(1)
	engine.SetProbability(0.5)

	for track := 0; track < TrackCount; track++ {
		for step := 0; step < MaxSteps; step++ {
			engine.SetCell(track, step, 1)
		}
	}

	engine.SetReverb(1)
	engine.SetTempo(300)
	engine.SetRunning(true)

	buf := renderTotal(engine, int(testSampleRate*10))
	for i, sample := range buf {
		value := float64(sample)
		if math.IsNaN(value) || math.IsInf(value, 0) {
			t.Fatalf("sample %d not finite over long humanized render: %v", i, value)
		}

		if math.Abs(value) > 1.0 {
			t.Fatalf("sample %d = %v exceeds ±1.0 ceiling", i, value)
		}
	}
}

// nonFiniteInputs are the values every float setter must reject outright.
func nonFiniteInputs() map[string]float64 {
	return map[string]float64{
		"NaN":  math.NaN(),
		"+Inf": math.Inf(1),
		"-Inf": math.Inf(-1),
	}
}

// engineState is the parameter state a rejected setter must leave untouched.
type engineState struct {
	bpm      float64
	swing    float64
	prob     float64
	humanize float64
	reverb   float64
	volume   float64
	decay    float64
	cell     float64
	// voiceParam must stay a plain float64: engineState is compared with !=.
	voiceParam   float64
	stepDuration [MaxSteps]uint64
}

func snapshotState(engine *Engine) engineState {
	return engineState{
		bpm:      engine.bpm,
		swing:    engine.swing,
		prob:     engine.prob,
		humanize: engine.humanize,
		reverb:   engine.reverbAmount,
		volume:   engine.volumes[1],
		decay:    engine.decays[1],
		cell:     engine.pattern[1][3],
		// Snare param 0 is snare.toneHz; see params.go.
		voiceParam:   engine.voices[1].Param(0),
		stepDuration: engine.stepDuration,
	}
}

// configuredEngine returns an engine whose every parameter sits at a distinct
// non-default value, so any accidental reset is visible.
func configuredEngine() *Engine {
	engine := NewEngine(testSampleRate)
	engine.SetTempo(137)
	engine.SetSwing(0.25)
	engine.SetProbability(0.6)
	engine.SetHumanize(0.2)
	engine.SetReverb(0.3)
	engine.SetVolume(1, 0.4)
	engine.SetDecay(1, 0.8)
	engine.SetCell(1, 3, 0.7)
	engine.SetVoiceParam(1, 0, 0.9)

	return engine
}

func TestNonFiniteSettersLeaveStateUnchanged(t *testing.T) {
	for name, bad := range nonFiniteInputs() {
		engine := configuredEngine()
		want := snapshotState(engine)

		engine.SetTempo(bad)
		engine.SetSwing(bad)
		engine.SetProbability(bad)
		engine.SetHumanize(bad)
		engine.SetReverb(bad)
		engine.SetVolume(1, bad)
		engine.SetDecay(1, bad)
		engine.SetCell(1, 3, bad)
		engine.SetVoiceParam(1, 0, bad)
		engine.TriggerVoice(1, bad)
		engine.SetPattern([]float64{bad, bad, bad})

		if got := snapshotState(engine); got != want {
			t.Fatalf("%s input changed engine state:\n got %+v\nwant %+v", name, got, want)
		}
	}
}

func TestNonFiniteCellVelocityKeepsRenderFinite(t *testing.T) {
	for name, bad := range nonFiniteInputs() {
		engine := NewEngine(testSampleRate)
		engine.SetCell(0, 0, bad)

		if engine.pattern[0][0] != 0 {
			t.Fatalf("%s velocity stored as %v, want the cell left at 0", name, engine.pattern[0][0])
		}

		engine.SetRunning(true)

		buf := renderTotal(engine, 1024)
		for i, sample := range buf {
			value := float64(sample)
			if math.IsNaN(value) || math.IsInf(value, 0) {
				t.Fatalf("%s velocity: sample %d is not finite: %v", name, i, value)
			}
		}
	}
}

func TestNonFiniteTempoKeepsStepLengthsSane(t *testing.T) {
	for name, bad := range nonFiniteInputs() {
		engine := NewEngine(testSampleRate)
		engine.SetTempo(120)

		want := engine.stepDuration

		engine.SetTempo(bad)

		if engine.bpm != 120 {
			t.Fatalf("%s tempo stored as %v, want 120 kept", name, engine.bpm)
		}

		if engine.stepDuration != want {
			t.Fatalf("%s tempo rewrote step durations: %v", name, engine.stepDuration)
		}

		for step, duration := range engine.stepDuration {
			if duration == 0 {
				t.Fatalf("%s tempo: step %d duration is zero", name, step)
			}
		}
	}
}

func TestSetPatternRejectsNonFiniteSnapshotAtomically(t *testing.T) {
	engine := NewEngine(testSampleRate)
	initial := make([]float64, PatternSize)
	for i := range initial {
		initial[i] = 0.5
	}
	engine.SetPattern(initial)

	invalid := make([]float64, PatternSize)
	invalid[0], invalid[1], invalid[2] = math.NaN(), 1, math.Inf(-1)
	engine.SetPattern(invalid)

	for track := range engine.pattern {
		for step, velocity := range engine.pattern[track] {
			if velocity != 0.5 {
				t.Fatalf("invalid snapshot changed cell (%d,%d) to %v", track, step, velocity)
			}
		}
	}
}

func TestNewEngineToleratesInvalidSampleRate(t *testing.T) {
	for _, sr := range []float64{0, -1, math.NaN(), math.Inf(1), math.Inf(-1)} {
		engine := NewEngine(sr)

		if engine.sr != defaultSampleRate {
			t.Fatalf("NewEngine(%v): sr = %v, want fallback %v", sr, engine.sr, defaultSampleRate)
		}

		// The DSP constructors must have produced usable objects — and even
		// if they had not, Render must not panic.
		engine.SetReverb(1)
		engine.SetCell(0, 0, 1)
		engine.SetRunning(true)

		buf := renderTotal(engine, 1024)
		for i, sample := range buf {
			value := float64(sample)
			if math.IsNaN(value) || math.IsInf(value, 0) {
				t.Fatalf("NewEngine(%v): sample %d is not finite: %v", sr, i, value)
			}
		}
	}
}

// TestSwingPreservesLoopLengthForEveryStepCount pins the swing invariant: the
// active loop always spans stepCount base steps, whatever the swing amount or
// (odd or even) loop length. A 7-step loop at full swing used to run 7.14 %
// slow because long/short was picked by absolute step-index parity.
func TestSwingPreservesLoopLengthForEveryStepCount(t *testing.T) {
	engine := NewEngine(testSampleRate)

	for _, bpm := range []float64{60, 120, 137, 300} {
		engine.SetTempo(bpm)

		base := uint64(math.Round(testSampleRate * 60.0 / bpm / 4.0 * float64(stepPhaseUnit)))

		for count := 1; count <= MaxSteps; count++ {
			for _, swing := range []float64{0, 0.1, 0.25, 0.5} {
				engine.SetStepCount(count)
				engine.SetSwing(swing)

				var total uint64

				for step, duration := range engine.stepDuration[:count] {
					if duration == 0 {
						t.Fatalf("bpm %v swing %v count %d: step %d duration is zero",
							bpm, swing, count, step)
					}

					total += duration
				}

				if want := uint64(count) * base; total != want {
					t.Fatalf("bpm %v swing %v count %d: loop spans %d fixed-point samples, want %d",
						bpm, swing, count, total, want)
				}

				// An odd loop cannot pair its final step, so that step must
				// keep the plain base length instead of doubling up on long.
				if count%2 == 1 && engine.stepDuration[count-1] != base {
					t.Fatalf("bpm %v swing %v count %d: unpaired final step is %d, want base %d",
						bpm, swing, count, engine.stepDuration[count-1], base)
				}
			}
		}
	}
}

func TestSevenStepLoopAtFullSwingKeepsTempo(t *testing.T) {
	engine := NewEngine(testSampleRate)
	engine.SetTempo(120)
	engine.SetSwing(0.5)
	engine.SetStepCount(7)

	var total uint64

	for _, length := range engine.stepDuration[:7] {
		total += length
	}

	// 120 BPM at 48 kHz = 6000 samples per 16th note; seven of them = 42000.
	if want := uint64(42000) * stepPhaseUnit; total != want {
		t.Fatalf("7-step loop at full swing spans %d fixed-point samples, want %d", total, want)
	}
}

func TestSetStepCountRestartsWrappedStep(t *testing.T) {
	engine := NewEngine(testSampleRate)
	engine.SetSwing(0)
	engine.SetRunning(true)

	// Part-way through step 10, shrink the loop under the playhead.
	engine.currentStep = 10
	engine.currentStepDuration = engine.stepDuration[10]
	engine.stepPhase = engine.currentStepDuration - stepPhaseUnit

	engine.SetStepCount(4)

	if engine.stepPhase != 0 {
		t.Fatalf("wrapping the playhead left stepPhase = %d, want 0", engine.stepPhase)
	}

	if got := engine.CurrentStep(); got != 10%4 {
		t.Fatalf("shrinking to 4 steps left currentStep = %d, want %d", got, 10%4)
	}

	// The step it landed on must play out in full rather than ending after
	// the single sample the old step had left.
	renderTotal(engine, samplesForStep(engine, 2)-1)

	if got := engine.CurrentStep(); got != 2 {
		t.Fatalf("wrapped step ended early: CurrentStep() = %d, want 2", got)
	}
}

func TestIndexedSettersNoOpOutOfRange(t *testing.T) {
	engine := configuredEngine()
	want := snapshotState(engine)

	for _, track := range []int{-1, TrackCount, math.MaxInt} {
		engine.SetVolume(track, 0.1)
		engine.SetDecay(track, 0.1)
		engine.SetCell(track, 0, 1)
		engine.SetVoiceParam(track, 0, 0.1)
		engine.TriggerVoice(track, 1)
	}

	for _, index := range []int{-1, maxVoiceParams, math.MaxInt} {
		engine.SetVoiceParam(1, index, 0.1)
	}

	for _, step := range []int{-1, MaxSteps, math.MaxInt} {
		engine.SetCell(0, step, 1)
	}

	if got := snapshotState(engine); got != want {
		t.Fatalf("out-of-range indices changed engine state:\n got %+v\nwant %+v", got, want)
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

// ── Per-voice synthesis parameters (PLAN.md G20) ───────────────────────────

func TestSetVoiceParamClamps(t *testing.T) {
	engine := NewEngine(testSampleRate)

	engine.SetVoiceParam(0, 0, -2)

	if got := engine.voices[0].Param(0); got != 0 {
		t.Fatalf("SetVoiceParam(-2) stored %v, want 0", got)
	}

	engine.SetVoiceParam(0, 0, 3)

	if got := engine.voices[0].Param(0); got != 1 {
		t.Fatalf("SetVoiceParam(3) stored %v, want 1", got)
	}

	engine.SetVoiceParam(0, 0, 0.25)

	if got := engine.voices[0].Param(0); got != 0.25 {
		t.Fatalf("SetVoiceParam(0.25) stored %v, want 0.25", got)
	}
}

// TestDefaultVoiceParamsPreserveRender is the engine-level twin of
// TestVoiceParamDefaultsAreShippedConstants: explicitly writing every
// parameter's default must leave the mix bit-identical to a never-touched
// engine, so restoring a saved state cannot subtly retune the kit.
func TestDefaultVoiceParamsPreserveRender(t *testing.T) {
	build := func() *Engine {
		engine := NewEngine(testSampleRate)
		engine.SetCell(0, 0, 1)
		engine.SetCell(1, 2, 0.7)
		engine.SetCell(2, 4, 1)
		engine.SetCell(3, 6, 0.7)
		engine.SetCell(4, 8, 1)
		engine.SetReverb(0.5)
		engine.SetRunning(true)

		return engine
	}

	untouched := build()

	explicit := build()
	for track := range TrackCount {
		for index, spec := range SpecsForTrack(track) {
			explicit.SetVoiceParam(track, index, spec.Default)
		}
	}

	want := make([]float32, int(testSampleRate))
	untouched.Render(want)

	got := make([]float32, len(want))
	explicit.Render(got)

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sample %d differs after writing every default: %v vs %v", i, got[i], want[i])
		}
	}
}

func TestTriggerVoiceSoundsWhileStopped(t *testing.T) {
	engine := NewEngine(testSampleRate)

	engine.TriggerVoice(0, 1)

	if step := engine.CurrentStep(); step != -1 {
		t.Fatalf("CurrentStep() = %d after an audition, want -1 (still stopped)", step)
	}

	buf := make([]float32, int(testSampleRate/2))
	engine.Render(buf)

	var peak float64

	for _, sample := range buf {
		if abs := math.Abs(float64(sample)); abs > peak {
			peak = abs
		}
	}

	if peak < 0.05 {
		t.Fatalf("audition peak %v, want an audible hit while stopped", peak)
	}
}

func TestTriggerVoiceIgnoresInvalidInput(t *testing.T) {
	engine := NewEngine(testSampleRate)

	for _, track := range []int{-1, TrackCount, math.MaxInt} {
		engine.TriggerVoice(track, 1)
	}

	engine.TriggerVoice(0, 0)
	engine.TriggerVoice(0, -1)

	for _, bad := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		engine.TriggerVoice(0, bad)
	}

	buf := make([]float32, int(testSampleRate/4))
	engine.Render(buf)

	for i, sample := range buf {
		if sample != 0 {
			t.Fatalf("rejected audition still produced %v at sample %d", sample, i)
		}
	}
}

func TestPhysicalTomCanBeSelectedAndAuditioned(t *testing.T) {
	for _, track := range []int{tomTrackIndex, tom2TrackIndex} {
		engine := NewEngine(testSampleRate)
		engine.SetTomModel(track, TomModelPhysical)

		if got := engine.TomModel(track); got != TomModelPhysical {
			t.Fatalf("TomModel(%d) = %v, want physical", track, got)
		}

		engine.TriggerVoice(track, 1)
		buffer := make([]float32, int(testSampleRate/2))
		engine.Render(buffer)

		peak := 0.0
		for index, sample := range buffer {
			value := float64(sample)
			if math.IsNaN(value) || math.IsInf(value, 0) {
				t.Fatalf("physical Tom track %d sample %d is non-finite: %v",
					track, index, value)
			}

			peak = math.Max(peak, math.Abs(value))
		}

		if peak < 0.05 {
			t.Fatalf("physical Tom track %d audition peak = %v, want audible output",
				track, peak)
		}
	}
}

func TestTomModelSwitchResetsTailsAndPreservesProceduralParams(t *testing.T) {
	engine := NewEngine(testSampleRate)
	engine.SetVoiceParam(tomTrackIndex, tomParamPitchTo, 0.8)
	engine.TriggerVoice(tomTrackIndex, 1)

	engine.SetTomModel(tomTrackIndex, TomModelPhysical)
	if engine.proceduralToms[tomTrackIndex].IsActive() {
		t.Fatal("procedural Tom remained active after switching models")
	}

	engine.SetVoiceParam(tomTrackIndex, tomParamPitchTo, 0.3)
	engine.TriggerVoice(tomTrackIndex, 1)
	engine.SetTomModel(tomTrackIndex, TomModelProcedural)

	if engine.physicalToms[tomTrackIndex].IsActive() {
		t.Fatal("physical Tom remained active after switching models")
	}
	if got := engine.proceduralToms[tomTrackIndex].Param(tomParamPitchTo); got != 0.3 {
		t.Fatalf("procedural Tom parameter = %v, want 0.3", got)
	}
}

func TestPhysicalTomParametersAreIndependentAndSurviveModelSwitch(t *testing.T) {
	engine := NewEngine(testSampleRate)
	engine.SetPhysicalTomParam(tomTrackIndex, physicalTomParamBatterTension, 0.8)
	engine.SetPhysicalTomParam(tom2TrackIndex, physicalTomParamBatterTension, 0.3)

	if engine.physicalToms[tomTrackIndex] != nil || engine.physicalToms[tom2TrackIndex] != nil {
		t.Fatal("editing an inactive physical bank eagerly initialized a physical Tom")
	}
	if got := engine.physicalTomParams[tomTrackIndex][physicalTomParamBatterTension]; got != 0.8 {
		t.Fatalf("physical batter tension position = %v, want 0.8", got)
	}
	if got := engine.physicalTomParams[tom2TrackIndex][physicalTomParamBatterTension]; got != 0.3 {
		t.Fatalf("physical Tom 2 batter tension position = %v, want 0.3", got)
	}

	engine.SetTomModel(tomTrackIndex, TomModelPhysical)
	if got := engine.physicalToms[tomTrackIndex].Param(physicalTomParamBatterTension); got != 0.8 {
		t.Fatalf("lazily constructed physical batter tension position = %v, want 0.8", got)
	}
	wantTension := physicalTomSpecs[physicalTomParamBatterTension].Map(0.8)
	if got := engine.physicalToms[tomTrackIndex].config.Batter.TensionNPerM; got != wantTension {
		t.Fatalf("physical batter tension = %v, want %v", got, wantTension)
	}

	engine.SetTomModel(tomTrackIndex, TomModelProcedural)
	if got := engine.physicalToms[tomTrackIndex].Param(physicalTomParamBatterTension); got != 0.8 {
		t.Fatalf("physical parameter after A/B switch = %v, want 0.8", got)
	}
	if got := engine.proceduralToms[tomTrackIndex].Param(tomParamPitchTo); got != tomSpecs[tomParamPitchTo].Default {
		t.Fatalf("physical edit changed procedural tuning to %v", got)
	}
}

func TestPhysicalTomAsymmetryParametersMapToBothHeads(t *testing.T) {
	engine := NewEngine(testSampleRate)
	engine.SetPhysicalTomParam(tomTrackIndex, physicalTomParamAsymmetry, 0.75)
	engine.SetPhysicalTomParam(tomTrackIndex, physicalTomParamAsymmetryAxis, 0.25)
	engine.SetTomModel(tomTrackIndex, TomModelPhysical)

	config := engine.physicalToms[tomTrackIndex].config
	wantSplit := physicalTomSpecs[physicalTomParamAsymmetry].Map(0.75) / 100
	wantAxis := physicalTomSpecs[physicalTomParamAsymmetryAxis].Map(0.25) *
		math.Pi / 180

	if config.Batter.TensionAsymmetry.SplitRatio != wantSplit ||
		config.Resonant.TensionAsymmetry.SplitRatio != wantSplit*0.75 {
		t.Fatalf(
			"head split ratios = batter %v, resonant %v; want %v and %v",
			config.Batter.TensionAsymmetry.SplitRatio,
			config.Resonant.TensionAsymmetry.SplitRatio,
			wantSplit,
			wantSplit*0.75,
		)
	}
	if config.Batter.TensionAsymmetry.PrincipalAxisAngleRad != wantAxis ||
		config.Resonant.TensionAsymmetry.PrincipalAxisAngleRad != wantAxis {
		t.Fatalf(
			"head axes = batter %v, resonant %v; want %v",
			config.Batter.TensionAsymmetry.PrincipalAxisAngleRad,
			config.Resonant.TensionAsymmetry.PrincipalAxisAngleRad,
			wantAxis,
		)
	}
}

func TestPhysicalTomParameterRejectsInvalidInput(t *testing.T) {
	engine := NewEngine(testSampleRate)
	engine.SetPhysicalTomParam(tomTrackIndex, physicalTomParamHardness, 0.4)

	for _, index := range []int{-1, len(physicalTomSpecs), math.MaxInt} {
		engine.SetPhysicalTomParam(tomTrackIndex, index, 0.8)
	}
	for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		engine.SetPhysicalTomParam(tomTrackIndex, physicalTomParamHardness, value)
	}

	if got := engine.physicalTomParams[tomTrackIndex][physicalTomParamHardness]; got != 0.4 {
		t.Fatalf("invalid edit changed hardness to %v", got)
	}
}

func TestInvalidTomModelIsIgnored(t *testing.T) {
	engine := NewEngine(testSampleRate)
	engine.SetTomModel(tomTrackIndex, TomModel(99))
	engine.SetTomModel(0, TomModelPhysical)

	if got := engine.TomModel(tomTrackIndex); got != TomModelProcedural {
		t.Fatalf("TomModel() = %v after invalid selection, want procedural", got)
	}
	if engine.voices[tomTrackIndex] != engine.proceduralToms[tomTrackIndex] {
		t.Fatal("invalid selection changed the active Tom voice")
	}
}

func TestPhysicalTomRenderDoesNotAllocate(t *testing.T) {
	engine := NewEngine(testSampleRate)
	engine.SetTomModel(tomTrackIndex, TomModelPhysical)
	engine.TriggerVoice(tomTrackIndex, 1)
	buffer := make([]float32, 512)

	allocations := testing.AllocsPerRun(100, func() {
		engine.Render(buffer)
	})
	if allocations != 0 {
		t.Fatalf("physical Tom Render allocated %v times per call, want 0", allocations)
	}
}
