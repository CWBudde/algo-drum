package drum

import (
	"math"
	"testing"
)

type countingVoice struct {
	triggers int
}

func (v *countingVoice) Trigger(float64)     { v.triggers++ }
func (*countingVoice) Tick() float64         { return 0 }
func (*countingVoice) SetDecay(float64)      {}
func (*countingVoice) SetParam(int, float64) {}
func (*countingVoice) Param(int) float64     { return 0 }
func (*countingVoice) ParamSpecs() []ParamSpec {
	return nil
}

func installCountingVoices(engine *Engine) [TrackCount]*countingVoice {
	var voices [TrackCount]*countingVoice
	for track := range TrackCount {
		if validTomTrack(track) {
			continue
		}
		voices[track] = &countingVoice{}
		engine.voices[track] = voices[track]
	}

	return voices
}

func renderSteps(engine *Engine, steps int) {
	stepSamples := samplesForStep(engine, 0)
	engine.Render(make([]float32, stepSamples*steps))
}

func TestCellProbabilityDefaultsToOneAndMultipliesGlobalProbability(t *testing.T) {
	engine := NewEngine(testSampleRate)
	voices := installCountingVoices(engine)
	engine.SetStepCount(1)
	engine.SetCell(0, 0, 1)
	engine.SetRunning(true)

	renderSteps(engine, 2)
	if got := voices[0].triggers; got != 2 {
		t.Fatalf("default cell probability produced %d hits, want 2", got)
	}

	engine.SetCellProbability(0, 0, 0)
	renderSteps(engine, 2)
	if got := voices[0].triggers; got != 2 {
		t.Fatalf("cell probability 0 increased hit count to %d", got)
	}

	engine.SetCellProbability(0, 0, 1)
	engine.SetProbability(0)
	renderSteps(engine, 2)
	if got := voices[0].triggers; got != 2 {
		t.Fatalf("global probability 0 increased hit count to %d", got)
	}
}

func TestDefaultCellSemanticsAreSampleExact(t *testing.T) {
	build := func(explicit bool) *Engine {
		engine := NewEngine(testSampleRate)
		engine.SetCell(0, 0, 1)
		engine.SetCell(2, 4, 0.7)
		if explicit {
			for track := range TrackCount {
				engine.SetTrackLength(track, MaxSteps)
				for step := range MaxSteps {
					engine.SetCellProbability(track, step, 1)
					engine.SetCellCondition(track, step, TriggerAlways)
				}
			}
			engine.SetFillMode(false)
		}
		engine.SetRunning(true)

		return engine
	}

	reference := renderTotal(build(false), int(testSampleRate))
	explicit := renderTotal(build(true), int(testSampleRate))
	for i := range reference {
		if reference[i] != explicit[i] {
			t.Fatalf("explicit defaults changed sample %d: %v vs %v", i, reference[i], explicit[i])
		}
	}
}

func TestPassConditionsUseIndependentTrackLoops(t *testing.T) {
	tests := []struct {
		name      string
		condition TriggerCondition
		loops     int
		want      int
	}{
		{"every second", TriggerEvery2, 8, 4},
		{"every third", TriggerEvery3, 9, 3},
		{"every fourth", TriggerEvery4, 8, 2},
		{"first loop", TriggerFirstLoop, 8, 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			engine := NewEngine(testSampleRate)
			voices := installCountingVoices(engine)
			engine.SetStepCount(1)
			engine.SetCell(0, 0, 1)
			engine.SetCellCondition(0, 0, test.condition)
			engine.SetRunning(true)
			renderSteps(engine, test.loops)

			if got := voices[0].triggers; got != test.want {
				t.Fatalf("trigger count = %d, want %d", got, test.want)
			}
		})
	}
}

func TestFillOnlyAndNotPreviousFiredConditions(t *testing.T) {
	t.Run("fill only", func(t *testing.T) {
		engine := NewEngine(testSampleRate)
		voices := installCountingVoices(engine)
		engine.SetStepCount(1)
		engine.SetCell(0, 0, 1)
		engine.SetCellCondition(0, 0, TriggerFillOnly)
		engine.SetRunning(true)
		renderSteps(engine, 3)
		if got := voices[0].triggers; got != 0 {
			t.Fatalf("fill-disabled hit count = %d, want 0", got)
		}

		engine.SetFillMode(true)
		renderSteps(engine, 3)
		if got := voices[0].triggers; got != 3 {
			t.Fatalf("fill-enabled hit count = %d, want 3", got)
		}
	})

	t.Run("not previous fired observes accepted gates", func(t *testing.T) {
		engine := NewEngine(testSampleRate)
		voices := installCountingVoices(engine)
		engine.SetStepCount(2)
		engine.SetCell(0, 0, 1)
		engine.SetCell(0, 1, 1)
		engine.SetCellProbability(0, 0, 0)
		engine.SetCellCondition(0, 1, TriggerNotPreviousFired)
		engine.SetRunning(true)
		renderSteps(engine, 4)

		if got := voices[0].triggers; got != 2 {
			t.Fatalf("trigger count = %d, want step 1 on both passes = 2", got)
		}
	})
}

func TestPerTrackLengthsCreatePolymeterAcrossMasterWraps(t *testing.T) {
	engine := NewEngine(testSampleRate)
	voices := installCountingVoices(engine)
	engine.SetStepCount(5)
	engine.SetTrackLength(0, 3)
	engine.SetTrackLength(1, 4)
	engine.SetCell(0, 0, 1)
	engine.SetCell(1, 0, 1)
	engine.SetRunning(true)

	renderSteps(engine, 12)
	if got := voices[0].triggers; got != 4 {
		t.Errorf("3-step track triggered %d times over 12 clocks, want 4", got)
	}
	if got := voices[1].triggers; got != 3 {
		t.Errorf("4-step track triggered %d times over 12 clocks, want 3", got)
	}
	if engine.currentStep != 2 {
		t.Errorf("5-step master playhead = %d after 12 clocks, want 2", engine.currentStep)
	}
}

func TestTrackLengthEditUsesTheAbsoluteClock(t *testing.T) {
	engine := NewEngine(testSampleRate)
	installCountingVoices(engine)
	engine.SetStepCount(5)
	engine.SetRunning(true)
	renderSteps(engine, 6)

	if engine.currentStep != 1 || engine.clockStep != 6 {
		t.Fatalf("clock before length edit: master=%d absolute=%d, want 1/6",
			engine.currentStep, engine.clockStep)
	}
	engine.SetTrackLength(0, 4)
	if got := engine.trackStep[0]; got != 2 {
		t.Fatalf("4-step track after clock 6 = %d, want 2", got)
	}
}

func TestStopResetsConditionalPassAndTrackPlayheads(t *testing.T) {
	engine := NewEngine(testSampleRate)
	voices := installCountingVoices(engine)
	engine.SetStepCount(4)
	engine.SetTrackLength(0, 3)
	engine.SetCell(0, 0, 1)
	engine.SetCellCondition(0, 0, TriggerFirstLoop)
	engine.SetRunning(true)
	renderSteps(engine, 4)
	if voices[0].triggers != 1 || engine.trackPass[0] != 1 || engine.trackStep[0] != 1 {
		t.Fatalf("unexpected pre-stop state: triggers=%d pass=%d step=%d",
			voices[0].triggers, engine.trackPass[0], engine.trackStep[0])
	}

	engine.SetRunning(false)
	if engine.clockStep != 0 || engine.trackPass[0] != 0 || engine.trackStep[0] != 0 || engine.previousFired[0] {
		t.Fatalf("stop left conditional runtime state: clock=%d pass=%d step=%d previous=%v",
			engine.clockStep, engine.trackPass[0], engine.trackStep[0], engine.previousFired[0])
	}
	engine.SetRunning(true)
	renderSteps(engine, 1)
	if got := voices[0].triggers; got != 2 {
		t.Fatalf("first-loop condition did not restart after Stop: triggers=%d", got)
	}
}

func TestDepthSettersClampAndRejectInvalidInput(t *testing.T) {
	engine := NewEngine(testSampleRate)
	engine.SetCellProbability(0, 0, 2)
	engine.SetTrackLength(0, 0)
	if engine.cellProbability[0][0] != 1 || engine.trackLength[0] != 1 {
		t.Fatalf("finite values were not clamped: probability=%v length=%d",
			engine.cellProbability[0][0], engine.trackLength[0])
	}

	for _, bad := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		engine.SetCellProbability(0, 0, bad)
		if engine.cellProbability[0][0] != 1 {
			t.Fatalf("non-finite probability %v changed the cell", bad)
		}
	}
	engine.SetCellCondition(0, 0, TriggerCondition(255))
	if engine.cellCondition[0][0] != TriggerAlways {
		t.Fatalf("invalid condition changed cell to %d", engine.cellCondition[0][0])
	}
}

func TestConditionalPolymeterRenderDoesNotAllocate(t *testing.T) {
	engine := NewEngine(testSampleRate)
	engine.SetTempo(300)
	for track := range TrackCount {
		engine.SetTrackLength(track, MaxSteps-track)
		for step := range MaxSteps {
			engine.SetCell(track, step, 1)
			engine.SetCellProbability(track, step, 0.75)
			engine.SetCellCondition(track, step, TriggerCondition(step%int(triggerConditionCount)))
		}
	}
	engine.SetFillMode(true)
	engine.SetRunning(true)
	buf := make([]float32, 4096)
	if allocs := testing.AllocsPerRun(50, func() { engine.Render(buf) }); allocs != 0 {
		t.Fatalf("conditional polymeter Render allocated %v times per call, want 0", allocs)
	}
}
