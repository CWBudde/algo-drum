package drum

import (
	"math"
	"testing"
)

func TestPeakLimiterCatchesSingleSampleTransient(t *testing.T) {
	limiter := newPeakLimiter(testSampleRate)
	var peak float64

	for sample := 0; sample < len(limiter.delay)+2; sample++ {
		input := 0.0
		if sample == 0 {
			input = 2
		}

		peak = math.Max(peak, math.Abs(limiter.ProcessSample(input)))
	}

	if peak == 0 || peak >= 2 {
		t.Fatalf("limited impulse peak = %v, want audible attenuation", peak)
	}
	if peak > limiter.ceiling+1e-12 {
		t.Fatalf("limited impulse peak = %v, exceeds ceiling %v", peak, limiter.ceiling)
	}
}

func TestDensePatternDoesNotReachHardClamp(t *testing.T) {
	engine := NewEngine(testSampleRate)
	for track := range TrackCount {
		engine.SetDecay(track, 1)
		for step := range MaxSteps {
			engine.SetCell(track, step, 1)
		}
	}
	engine.SetTempo(300)
	engine.SetReverb(1)
	engine.SetRunning(true)

	buf := renderTotal(engine, int(2*testSampleRate))
	if engine.hardClipCount != 0 {
		t.Fatalf("dense render used the final hard clamp %d times", engine.hardClipCount)
	}
	if peak := peakOf(buf); peak > engine.limiter.ceiling+1e-6 {
		t.Fatalf("dense render peak %v exceeds limiter ceiling %v", peak, engine.limiter.ceiling)
	}
}

func TestZeroWetReverbIsTransparent(t *testing.T) {
	processed := NewEngine(testSampleRate)
	bypassed := NewEngine(testSampleRate)
	processed.limiter = nil
	bypassed.limiter = nil
	bypassed.reverb = nil
	processed.TriggerVoice(1, 1)
	bypassed.TriggerVoice(1, 1)

	want := renderTotal(bypassed, 4096)
	got := renderTotal(processed, 4096)
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("zero-wet reverb changed sample %d: got %v want %v", i, got[i], want[i])
		}
	}
}

func TestReverbAdvancesWhileMuted(t *testing.T) {
	muted := NewEngine(testSampleRate)
	reference := NewEngine(testSampleRate)
	muted.limiter = nil
	reference.limiter = nil
	muted.SetReverb(1)
	reference.SetReverb(1)
	muted.liveReverbAmount = 1
	reference.liveReverbAmount = 1
	logErr("muted reverb wet", muted.reverb.SetWet(reverbMaxWet))
	logErr("reference reverb wet", reference.reverb.SetWet(reverbMaxWet))
	muted.TriggerVoice(0, 1)
	reference.TriggerVoice(0, 1)
	renderTotal(muted, 4096)
	renderTotal(reference, 4096)

	muted.SetReverb(0)
	muted.liveReverbAmount = 0
	logErr("muted reverb wet", muted.reverb.SetWet(0))
	renderTotal(muted, int(testSampleRate/4))
	renderTotal(reference, int(testSampleRate/4))

	muted.SetReverb(1)
	muted.liveReverbAmount = 1
	logErr("muted reverb wet", muted.reverb.SetWet(reverbMaxWet))
	want := renderTotal(reference, 4096)
	got := renderTotal(muted, 4096)
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("re-enabled reverb resurrected stale state at sample %d: got %v want %v", i, got[i], want[i])
		}
	}
}

func TestReverbAmountRamps(t *testing.T) {
	engine := NewEngine(testSampleRate)
	engine.SetReverb(1)
	if engine.liveReverbAmount != 0 {
		t.Fatalf("SetReverb jumped live amount to %v", engine.liveReverbAmount)
	}

	renderTotal(engine, 1)
	first := engine.liveReverbAmount
	if first <= 0 || first >= 1 {
		t.Fatalf("first wet ramp sample = %v, want inside (0,1)", first)
	}

	engine.SetReverb(0)
	renderTotal(engine, 1)
	if engine.liveReverbAmount <= 0 || engine.liveReverbAmount >= first {
		t.Fatalf("wet ramp after mute = %v, want inside (0,%v)", engine.liveReverbAmount, first)
	}
}

func TestPauseFreezesTransportAndPendingHits(t *testing.T) {
	engine := NewEngine(testSampleRate)
	engine.limiter = nil
	engine.reverb = nil
	engine.SetCell(0, 0, 1)
	engine.SetRunning(true)
	renderTotal(engine, 100)
	engine.schedule(1, 1, 10)
	phase := engine.stepPhase
	step := engine.currentStep
	pending := engine.pendingMask
	countdown := engine.pending[0].countdown

	engine.Pause()
	pausedTail := renderTotal(engine, 100)
	if engine.CurrentStep() != step || engine.stepPhase != phase {
		t.Fatalf("pause moved transport to step %d phase %d; want step %d phase %d",
			engine.CurrentStep(), engine.stepPhase, step, phase)
	}
	if engine.pendingMask != pending || engine.pending[0].countdown != countdown {
		t.Fatal("pause advanced a delayed humanized hit")
	}
	if peakOf(pausedTail) == 0 {
		t.Fatal("pause cut off an already-triggered voice tail")
	}

	engine.SetRunning(true)
	renderTotal(engine, countdown-1)
	if engine.pendingMask == 0 {
		t.Fatal("pending hit fired before its remaining countdown")
	}
	renderTotal(engine, 1)
	if engine.pendingMask != 0 {
		t.Fatal("pending hit did not fire after resume")
	}
}

func TestTransportSnapshotOwnsTransitionsAndRevisions(t *testing.T) {
	engine := NewEngine(48000)

	assertTransport := func(wantState TransportState, wantStep int, wantRevision uint64) {
		t.Helper()

		got := engine.TransportSnapshot()
		if got.State != wantState || got.Step != wantStep || got.Revision != wantRevision {
			t.Fatalf("TransportSnapshot() = %+v, want state=%q step=%d revision=%d",
				got, wantState, wantStep, wantRevision)
		}
	}

	assertTransport(TransportStopped, -1, 0)

	engine.BeginStart()
	assertTransport(TransportStarting, -1, 1)

	// Repeating a state request is not a new epoch.
	engine.BeginStart()
	assertTransport(TransportStarting, -1, 1)

	engine.SetRunning(true)
	assertTransport(TransportPlaying, 0, 2)

	engine.Pause()
	assertTransport(TransportPaused, 0, 3)

	engine.BeginStart()
	assertTransport(TransportStarting, -1, 4)

	engine.SetRunning(true)
	assertTransport(TransportPlaying, 0, 5)

	engine.SetRunning(false)
	assertTransport(TransportStopped, -1, 6)

	// Stop still resets position when repeated, but the already-stopped epoch
	// remains valid for chunks rendered after the first Stop.
	engine.SetRunning(false)
	assertTransport(TransportStopped, -1, 6)
}

func TestStopResetsTransportButLeavesVoiceTail(t *testing.T) {
	engine := NewEngine(testSampleRate)
	engine.limiter = nil
	engine.reverb = nil
	engine.SetCell(0, 0, 1)
	engine.SetRunning(true)
	renderTotal(engine, 100)
	engine.schedule(1, 1, 20)

	engine.SetRunning(false)
	if engine.CurrentStep() != -1 || engine.currentStep != 0 || engine.stepPhase != 0 || engine.stepTriggered {
		t.Fatalf("stop left transport at reported=%d raw=%d phase=%d triggered=%v",
			engine.CurrentStep(), engine.currentStep, engine.stepPhase, engine.stepTriggered)
	}
	if engine.pendingMask != 0 {
		t.Fatal("stop kept delayed hits queued")
	}
	if peak := peakOf(renderTotal(engine, 100)); peak == 0 {
		t.Fatal("stop cut off an already-triggered voice tail")
	}
}

func TestFixedPointClockDoesNotAccumulateDrift(t *testing.T) {
	for _, tc := range []struct {
		name  string
		bpm   float64
		steps int
		swing float64
		loops uint64
	}{
		{name: "odd swung loop", bpm: 137, steps: 7, swing: 0.5, loops: 200},
		{name: "full straight bar", bpm: 173, steps: 16, swing: 0, loops: 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine := NewEngine(testSampleRate)
			engine.reverb = nil
			engine.limiter = nil
			engine.SetTempo(tc.bpm)
			engine.SetStepCount(tc.steps)
			engine.SetSwing(tc.swing)

			baseQ := uint64(math.Round(testSampleRate * secondsPerMinute / tc.bpm / stepsPerBeat * float64(stepPhaseUnit)))
			var loopQ uint64
			for _, duration := range engine.stepDuration[:tc.steps] {
				loopQ += duration
			}
			if want := uint64(tc.steps) * baseQ; loopQ != want {
				t.Fatalf("fixed-point loop duration = %d, want %d", loopQ, want)
			}

			totalQ := loopQ * tc.loops
			samples := (totalQ + stepPhaseUnit - 1) / stepPhaseUnit
			engine.SetRunning(true)
			renderInChunks(engine, samples)

			if engine.currentStep != 0 {
				t.Fatalf("after %d loops current step = %d, want 0", tc.loops, engine.currentStep)
			}
			if want := samples*stepPhaseUnit - totalQ; engine.stepPhase != want {
				t.Fatalf("residual phase = %d, want %d", engine.stepPhase, want)
			}
			if engine.stepPhase >= stepPhaseUnit {
				t.Fatalf("cumulative timing error reached one sample: %d", engine.stepPhase)
			}
		})
	}
}

func renderInChunks(engine *Engine, samples uint64) {
	buf := make([]float32, 4096)
	for samples > 0 {
		count := min(samples, uint64(len(buf)))
		engine.Render(buf[:count])
		samples -= count
	}
}

func TestCopyPatternDoesNotAllocate(t *testing.T) {
	engine := NewEngine(testSampleRate)
	var dst [PatternSize]float32
	if allocs := testing.AllocsPerRun(100, func() { engine.CopyPattern(&dst) }); allocs != 0 {
		t.Fatalf("CopyPattern allocated %v times, want 0", allocs)
	}
}

func TestSetPatternDoesNotAllocate(t *testing.T) {
	engine := NewEngine(testSampleRate)
	pattern := make([]float64, PatternSize)
	if allocs := testing.AllocsPerRun(100, func() { engine.SetPattern(pattern) }); allocs != 0 {
		t.Fatalf("SetPattern allocated %v times, want 0", allocs)
	}
}

func TestHumanizedRenderIsDeterministic(t *testing.T) {
	build := func() *Engine {
		engine := NewEngine(testSampleRate)
		engine.SetHumanize(1)
		for track := range TrackCount {
			for step := range MaxSteps {
				engine.SetCell(track, step, 1)
			}
		}
		engine.SetRunning(true)

		return engine
	}

	first := renderTotal(build(), int(testSampleRate))
	second := renderTotal(build(), int(testSampleRate))
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("humanized render differs at sample %d: %v vs %v", i, first[i], second[i])
		}
	}
}

// hatTrackIndex is the shortest tail in the bank, and at its minimum decay it
// crosses engineSilence in about a quarter of a second. The idle tests use it
// so they measure the mechanism rather than a voice's release: the bass drum's
// resonant tail alone takes some six seconds to fall that far.
const hatTrackIndex = 2

func TestRenderIdlesAfterTailDecays(t *testing.T) {
	engine := NewEngine(testSampleRate)
	engine.SetDecay(hatTrackIndex, 0)
	engine.TriggerVoice(hatTrackIndex, 1)

	const chunk = 512

	buf := make([]float32, chunk)
	limit := int(2*testSampleRate) / chunk // the hat tail is an order shorter

	idleAfter := -1

	for i := range limit {
		engine.Render(buf)

		if engine.IsIdle() {
			idleAfter = i

			break
		}
	}

	if idleAfter < 0 {
		t.Fatal("engine never idled after a hit decayed away while stopped")
	}

	// The window is only confirmed at its end, so the first idle chunk still
	// contains the samples that filled it; the next one is the first that the
	// fast path wrote in full.
	engine.Render(buf)

	for i, sample := range buf {
		if sample != 0 {
			t.Fatalf("idle render wrote %v at sample %d, want exact silence", sample, i)
		}
	}

	if !engine.IsIdle() {
		t.Fatal("engine left the idle state while rendering its own silence")
	}
}

func TestIdleDoesNotTruncateAudibleTail(t *testing.T) {
	idling := NewEngine(testSampleRate)

	// The reference is the same engine with the idle window pushed out of
	// reach, which is exactly the pre-B10 always-render behaviour: silentRun
	// still counts, IsIdle is simply never satisfied.
	reference := NewEngine(testSampleRate)
	reference.idleSamples = math.MaxInt64

	for _, engine := range []*Engine{idling, reference} {
		engine.SetDecay(hatTrackIndex, 0)
		engine.TriggerVoice(hatTrackIndex, 1)
	}

	// Long enough to contain the whole tail and the idling that follows it.
	const renderSeconds = 1

	want := renderTotal(reference, int(renderSeconds*testSampleRate))
	got := renderTotal(idling, int(renderSeconds*testSampleRate))

	// The audible tail is everything up to the last sample the reference put
	// above the threshold; that is the part idling is not allowed to touch.
	lastAudible := -1

	for i, sample := range want {
		if math.Abs(float64(sample)) >= engineSilence {
			lastAudible = i
		}
	}

	if lastAudible <= 0 {
		t.Fatal("reference render produced no audible tail, so this proves nothing")
	}

	for i := 0; i <= lastAudible; i++ {
		if got[i] != want[i] {
			t.Fatalf("idling changed audible sample %d: got %v want %v (tail ends at %d)",
				i, got[i], want[i], lastAudible)
		}
	}

	// Past that point idling may only ever remove signal, never add any.
	for i := lastAudible + 1; i < len(got); i++ {
		if math.Abs(float64(got[i])) >= engineSilence {
			t.Fatalf("idle render emitted %v at sample %d, above the silence threshold %v",
				got[i], i, engineSilence)
		}
	}

	if !idling.IsIdle() {
		t.Fatalf("engine never idled in %d seconds; audible tail ended at sample %d",
			renderSeconds, lastAudible)
	}

	if reference.IsIdle() {
		t.Fatal("reference engine idled despite an unreachable confirm window")
	}
}
