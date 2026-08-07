package drum

import (
	"math"
	"testing"
)

type timingVoice struct {
	sample     int
	triggers   []int
	velocities []float64
}

func (v *timingVoice) Trigger(velocity float64) {
	v.triggers = append(v.triggers, v.sample)
	v.velocities = append(v.velocities, velocity)
}
func (v *timingVoice) Tick() float64         { v.sample++; return 0 }
func (*timingVoice) SetDecay(float64)        {}
func (*timingVoice) SetParam(int, float64)   {}
func (*timingVoice) Param(int) float64       { return 0 }
func (*timingVoice) ParamSpecs() []ParamSpec { return nil }

func installTimingVoice(engine *Engine, track int) *timingVoice {
	voice := &timingVoice{}
	engine.voices[track] = voice

	return voice
}

func TestCellHumanizeDefaultsAndCenteredTiming(t *testing.T) {
	engine := NewEngine(testSampleRate)
	voice := installTimingVoice(engine, 0)
	engine.SetStepCount(0, 1)
	engine.SetCell(0, 0, 0, 1)
	engine.SetHumanize(1)
	engine.SetRunning(true)

	stepSamples := samplesForStep(engine, 0)
	engine.Render(make([]float32, stepSamples*80+engine.humanizeLookahead+2))
	if len(voice.triggers) < 70 {
		t.Fatalf("trigger count = %d, want enough trials", len(voice.triggers))
	}

	sawEarly := false
	sawLate := false
	maxJitter := int(humanizeTimingMaxS * testSampleRate)
	for hit, sample := range voice.triggers[1:] { // startup is the documented no-pre-roll exception
		nominal := (hit + 1) * stepSamples
		offset := sample - nominal
		if offset < -maxJitter || offset > maxJitter {
			t.Fatalf("hit %d offset = %d, outside ±%d", hit+1, offset, maxJitter)
		}
		sawEarly = sawEarly || offset < 0
		sawLate = sawLate || offset > 0
	}
	if !sawEarly || !sawLate {
		t.Fatalf("centered timing did not produce both signs: early=%v late=%v", sawEarly, sawLate)
	}
}

func TestCellHumanizeZeroIsMechanicalWithGlobalHumanize(t *testing.T) {
	engine := NewEngine(testSampleRate)
	voice := installTimingVoice(engine, 0)
	engine.SetStepCount(0, 1)
	engine.SetCell(0, 0, 0, 1)
	engine.SetCellHumanize(0, 0, 0, 0)
	engine.SetHumanize(1)
	engine.SetRunning(true)

	stepSamples := samplesForStep(engine, 0)
	engine.Render(make([]float32, stepSamples*8+1))
	for hit, sample := range voice.triggers {
		if want := hit * stepSamples; sample != want {
			t.Fatalf("mechanical hit %d at %d, want %d", hit, sample, want)
		}
		if voice.velocities[hit] != 1 {
			t.Fatalf("mechanical hit %d velocity = %v, want 1", hit, voice.velocities[hit])
		}
	}
}

func TestCellRatchetsAreEvenlySpacedWithinStep(t *testing.T) {
	engine := NewEngine(testSampleRate)
	voice := installTimingVoice(engine, 0)
	engine.SetStepCount(0, 1)
	engine.SetCell(0, 0, 0, 0.75)
	engine.SetCellRepeats(0, 0, 0, 4)
	engine.SetRunning(true)

	stepSamples := samplesForStep(engine, 0)
	engine.Render(make([]float32, stepSamples))

	want := []int{0, stepSamples / 4, stepSamples / 2, stepSamples * 3 / 4}
	if len(voice.triggers) != len(want) {
		t.Fatalf("ratchet trigger count = %d, want %d", len(voice.triggers), len(want))
	}

	for hit, sample := range voice.triggers {
		if sample != want[hit] {
			t.Errorf("ratchet hit %d at %d, want %d", hit, sample, want[hit])
		}
		if voice.velocities[hit] != 0.75 {
			t.Errorf("ratchet hit %d velocity = %v, want 0.75", hit, voice.velocities[hit])
		}
	}
}

func TestCellRatchetSetterClamps(t *testing.T) {
	engine := NewEngine(testSampleRate)
	engine.SetCellRepeats(2, 3, 4, 0)
	if got := engine.banks[2].cellRepeats[3][4]; got != 1 {
		t.Fatalf("low repeat count = %d, want 1", got)
	}

	engine.SetCellRepeats(2, 3, 4, 9)
	if got := engine.banks[2].cellRepeats[3][4]; got != 4 {
		t.Fatalf("high repeat count = %d, want 4", got)
	}
}

func TestCellRatchetConditionGatesTheGroupOnce(t *testing.T) {
	engine := NewEngine(testSampleRate)
	voices := installCountingVoices(engine)
	engine.SetStepCount(0, 1)
	engine.SetCell(0, 0, 0, 1)
	engine.SetCellRepeats(0, 0, 0, 4)
	engine.SetCellCondition(0, 0, 0, TriggerEvery2)
	engine.SetRunning(true)

	renderSteps(engine, 2)
	if got := voices[0].triggers; got != 4 {
		t.Fatalf("every-second-loop ratchet triggered %d times, want one four-hit group", got)
	}
}

func TestCellHumanizeMovesRatchetAsOneGroup(t *testing.T) {
	engine := NewEngine(testSampleRate)
	voice := installTimingVoice(engine, 0)
	engine.SetStepCount(0, 1)
	engine.SetCell(0, 0, 0, 0.8)
	engine.SetCellRepeats(0, 0, 0, 4)
	engine.SetHumanize(1)
	engine.SetRunning(true)

	stepSamples := samplesForStep(engine, 0)
	engine.Render(make([]float32, stepSamples*3))
	if len(voice.triggers) < 8 {
		t.Fatalf("ratchet trigger count = %d, want at least 8", len(voice.triggers))
	}

	spacing := stepSamples / 4
	for hit := 5; hit < 8; hit++ {
		if got := voice.triggers[hit] - voice.triggers[hit-1]; got != spacing {
			t.Errorf("humanized ratchet spacing %d = %d, want %d", hit-4, got, spacing)
		}
		if voice.velocities[hit] != voice.velocities[4] {
			t.Errorf("humanized ratchet velocity %d = %v, want group velocity %v",
				hit-4, voice.velocities[hit], voice.velocities[4])
		}
	}
}

func TestBankSelectionAndChainBoundaries(t *testing.T) {
	engine := NewEngine(testSampleRate)
	engine.SetStepCount(0, 2)
	engine.SetStepCount(1, 1)
	engine.SetStepCount(2, 1)
	engine.SetTrackLength(0, 0, 3)

	engine.RequestBank(1)
	if engine.ActiveBank() != 1 || engine.QueuedBank() != NoBank {
		t.Fatalf("stopped request: active=%d queued=%d, want 1/%d",
			engine.ActiveBank(), engine.QueuedBank(), NoBank)
	}
	engine.RequestBank(0)
	engine.SetChain([]int{0, 0, 2})
	engine.SetChainEnabled(true)
	if engine.ActiveBank() != 0 || engine.ChainPosition() != 0 {
		t.Fatalf("enabled chain did not select entry zero")
	}

	engine.SetRunning(true)
	stepSamples := samplesForStep(engine, 0)
	engine.Render(make([]float32, stepSamples*2))
	if engine.ActiveBank() != 0 || engine.ChainPosition() != 1 || engine.trackStep[0] != 2 {
		t.Fatalf("duplicate chain entry lost phase: active=%d cursor=%d trackStep=%d",
			engine.ActiveBank(), engine.ChainPosition(), engine.trackStep[0])
	}
	engine.Render(make([]float32, stepSamples*2))
	if engine.ActiveBank() != 2 || engine.ChainPosition() != 2 || engine.currentStep != 0 ||
		engine.trackStep[0] != 0 || engine.trackPass[0] != 0 {
		t.Fatalf("different-bank chain switch did not reset phase: bank=%d cursor=%d master=%d track=%d pass=%d",
			engine.ActiveBank(), engine.ChainPosition(), engine.currentStep,
			engine.trackStep[0], engine.trackPass[0])
	}
	engine.RequestBank(3)
	if engine.QueuedBank() != NoBank {
		t.Fatalf("chain-enabled manual request queued bank %d", engine.QueuedBank())
	}

	engine.SetRunning(false)
	if engine.ActiveBank() != 0 || engine.ChainPosition() != 0 || engine.QueuedBank() != NoBank {
		t.Fatalf("Stop did not restore chain origin: active=%d cursor=%d queued=%d",
			engine.ActiveBank(), engine.ChainPosition(), engine.QueuedBank())
	}
}

func TestStandaloneQueueCommitsAtMasterWrap(t *testing.T) {
	engine := NewEngine(testSampleRate)
	engine.SetStepCount(0, 2)
	engine.SetStepCount(1, 1)
	engine.RequestBank(0)
	engine.SetRunning(true)
	engine.RequestBank(1)
	if engine.ActiveBank() != 0 || engine.QueuedBank() != 1 {
		t.Fatalf("playing request = active %d queued %d, want 0/1",
			engine.ActiveBank(), engine.QueuedBank())
	}

	stepSamples := samplesForStep(engine, 0)
	engine.Render(make([]float32, stepSamples*2))
	if engine.ActiveBank() != 1 || engine.QueuedBank() != NoBank || engine.currentStep != 0 {
		t.Fatalf("queued switch did not commit: active=%d queued=%d step=%d",
			engine.ActiveBank(), engine.QueuedBank(), engine.currentStep)
	}
}

func TestCellHumanizeSetterClampsAndRejects(t *testing.T) {
	engine := NewEngine(testSampleRate)
	engine.SetCellHumanize(3, 1, 2, 2)
	if got := engine.banks[3].cellHumanize[1][2]; got != 1 {
		t.Fatalf("clamped cell humanize = %v, want 1", got)
	}
	for _, bad := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		engine.SetCellHumanize(3, 1, 2, bad)
		if got := engine.banks[3].cellHumanize[1][2]; got != 1 {
			t.Fatalf("non-finite %v changed cell humanize to %v", bad, got)
		}
	}
}
