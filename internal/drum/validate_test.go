package drum

import (
	"math"
	"strings"
	"testing"
)

// corruptEngine reaches past the setters to break one invariant directly, the
// way a bug inside the engine would (C11 rewrote step lengths, not parameters).
type corruptEngine func(engine *Engine)

func TestValidateAcceptsFreshEngine(t *testing.T) {
	engine := NewEngine(testSampleRate)

	if err := engine.Validate(); err != nil {
		t.Fatalf("fresh engine invalid: %v", err)
	}
}

func TestValidateAcceptsEngineAfterUse(t *testing.T) {
	engine := NewEngine(testSampleRate)
	engine.SetTempo(174)
	engine.SetSwing(maxSwing)
	engine.SetStepCount(0, 7)
	engine.SetCell(0, 0, 0, 1)
	engine.SetCell(0, 2, 3, 0.7)
	engine.SetHumanize(1)
	engine.SetProbability(0.5)
	engine.SetReverb(0.8)
	engine.SetVolume(1, 0.25)
	engine.SetDecay(4, 0.9)
	engine.SetRunning(true)

	// Long enough to wrap the odd-length loop several times, so pending
	// humanize triggers are in flight when the render stops.
	renderTotal(engine, samplesForStep(engine, 0)*20)

	if err := engine.Validate(); err != nil {
		t.Fatalf("engine invalid after playback: %v", err)
	}
}

func TestValidateRejectsBrokenInvariants(t *testing.T) {
	cases := []struct {
		name    string
		corrupt corruptEngine
		want    string
	}{
		{
			name:    "zero step duration",
			corrupt: func(engine *Engine) { engine.stepDuration[3] = 0 },
			want:    "step 3 duration is zero",
		},
		{
			name:    "playhead past the loop end",
			corrupt: func(engine *Engine) { engine.currentStep = engine.stepCount },
			want:    "playhead 16 outside loop of 16 steps",
		},
		{
			name:    "negative playhead",
			corrupt: func(engine *Engine) { engine.currentStep = -1 },
			want:    "playhead -1",
		},
		{
			name:    "step count below one",
			corrupt: func(engine *Engine) { engine.stepCount = 0 },
			want:    "step count 0",
		},
		{
			name:    "step count past capacity",
			corrupt: func(engine *Engine) { engine.stepCount = MaxSteps + 1 },
			want:    "step count 17",
		},
		{
			name:    "track length below one",
			corrupt: func(engine *Engine) { engine.trackLength[2] = 0 },
			want:    "track 2 length 0",
		},
		{
			name: "track playhead past loop end",
			corrupt: func(engine *Engine) {
				engine.trackLength[2] = 3
				engine.trackStep[2] = 3
			},
			want: "track 2 playhead 3 outside loop of 3 steps",
		},
		{
			name: "pending trigger past its deadline",
			corrupt: func(engine *Engine) {
				engine.pending[2] = pendingTrigger{countdown: 0, track: 1, velocity: 0.7}
				engine.pendingMask = 1 << 2
			},
			want: "pending trigger 2 past its deadline",
		},
		{
			name: "pending trigger on an unknown track",
			corrupt: func(engine *Engine) {
				engine.pending[0] = pendingTrigger{countdown: 10, track: TrackCount, velocity: 0.7}
				engine.pendingMask = 1
			},
			want: "pending trigger 0 targets invalid track 7",
		},
		{
			name: "pending trigger with a non-finite velocity",
			corrupt: func(engine *Engine) {
				engine.pending[0] = pendingTrigger{countdown: 10, track: 0, velocity: math.NaN()}
				engine.pendingMask = 1
			},
			want: "pending trigger 0 velocity",
		},
		{
			name:    "non-finite tempo",
			corrupt: func(engine *Engine) { engine.bpm = math.NaN() },
			want:    "tempo",
		},
		{
			name:    "swing past full shuffle",
			corrupt: func(engine *Engine) { engine.swing = 1 },
			want:    "swing",
		},
		{
			name:    "sample rate zero",
			corrupt: func(engine *Engine) { engine.sr = 0 },
			want:    "sample rate",
		},
		{
			name:    "non-finite cell velocity",
			corrupt: func(engine *Engine) { engine.pattern[1][2] = math.Inf(1) },
			want:    "cell (1, 2) velocity",
		},
		{
			name:    "non-finite cell probability",
			corrupt: func(engine *Engine) { engine.cellProbability[1][2] = math.NaN() },
			want:    "cell (1, 2) probability",
		},
		{
			name:    "invalid cell condition",
			corrupt: func(engine *Engine) { engine.cellCondition[1][2] = TriggerCondition(255) },
			want:    "cell (1, 2) condition",
		},
		{
			name:    "volume past unity",
			corrupt: func(engine *Engine) { engine.volumes[3] = 2 },
			want:    "track 3 volume",
		},
		{
			name:    "non-finite smoothed volume",
			corrupt: func(engine *Engine) { engine.liveVol[0] = math.NaN() },
			want:    "track 0 smoothed volume",
		},
		{
			name:    "negative decay",
			corrupt: func(engine *Engine) { engine.decays[2] = -0.5 },
			want:    "track 2 decay",
		},
		{
			name:    "non-finite probability",
			corrupt: func(engine *Engine) { engine.prob = math.NaN() },
			want:    "probability",
		},
		{
			name:    "humanize past unity",
			corrupt: func(engine *Engine) { engine.humanize = 1.5 },
			want:    "humanize",
		},
		{
			name:    "reverb amount past unity",
			corrupt: func(engine *Engine) { engine.reverbAmount = 4 },
			want:    "reverb amount",
		},
		{
			name:    "silent run past the confirm window",
			corrupt: func(engine *Engine) { engine.silentRun = engine.idleSamples + 1 },
			want:    "silent run",
		},
		{
			name:    "negative silent run",
			corrupt: func(engine *Engine) { engine.silentRun = -1 },
			want:    "silent run -1",
		},
		{
			name:    "idle confirm window of zero samples",
			corrupt: func(engine *Engine) { engine.idleSamples = 0 },
			want:    "idle confirm window",
		},
		{
			name:    "missing voice",
			corrupt: func(engine *Engine) { engine.voices[4] = nil },
			want:    "track 4 has no voice",
		},
		{
			name: "physical shadow has wrong width",
			corrupt: func(engine *Engine) {
				engine.physicalTomParams[tomTrackIndex] = nil
			},
			want: "physical Tom track 3 shadow parameter count",
		},
		{
			name: "physical shadow has non-finite value",
			corrupt: func(engine *Engine) {
				engine.physicalTomParams[tomTrackIndex][0] = math.NaN()
			},
			want: "physical Tom track 3 shadow param 0",
		},
		{
			name: "physical instance diverges from shadow",
			corrupt: func(engine *Engine) {
				engine.SetTomModel(tomTrackIndex, TomModelPhysical)
				engine.physicalTomParams[tomTrackIndex][0] = 0.9
			},
			want: "physical Tom track 3 param 0",
		},
		{
			name: "selected physical Tom has no voice",
			corrupt: func(engine *Engine) {
				engine.tomModels[tomTrackIndex] = TomModelPhysical
			},
			want: "physical Tom track 3 has no physical voice",
		},
		{
			name: "selected Tom does not own active voice",
			corrupt: func(engine *Engine) {
				engine.voices[tomTrackIndex] = engine.voices[0]
			},
			want: "procedural Tom track 3 does not own its active voice",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			engine := NewEngine(testSampleRate)
			tc.corrupt(engine)

			err := engine.Validate()
			if err == nil {
				t.Fatalf("Validate accepted a broken engine, want %q", tc.want)
			}

			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate reported %q, want it to mention %q", err, tc.want)
			}
		})
	}
}

func TestValidateReportsEveryViolation(t *testing.T) {
	engine := NewEngine(testSampleRate)
	engine.stepDuration[0] = 0
	engine.currentStep = MaxSteps

	err := engine.Validate()
	if err == nil {
		t.Fatal("Validate accepted a broken engine")
	}

	for _, want := range []string{"step 0 duration is zero", "playhead 16"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("Validate reported %q, want it to mention %q too", err, want)
		}
	}
}
