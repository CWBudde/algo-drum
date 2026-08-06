package drum

import (
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestStateHasCompleteEngineMajorShapeWithoutConstructingPhysicalToms(t *testing.T) {
	engine := NewEngine(testSampleRate)
	state := engine.State()

	if len(state.Pattern) != PatternSize {
		t.Fatalf("pattern length = %d, want %d", len(state.Pattern), PatternSize)
	}
	if len(state.Tracks) != TrackCount {
		t.Fatalf("track count = %d, want %d", len(state.Tracks), TrackCount)
	}

	for track, trackState := range state.Tracks {
		if got, want := len(trackState.VoiceParams), len(SpecsForTrack(track)); got != want {
			t.Fatalf("track %d voice parameter count = %d, want %d", track, got, want)
		}

		if validTomTrack(track) {
			if trackState.Tom == nil {
				t.Fatalf("Tom track %d has no Tom state", track)
			}
			if got := len(trackState.Tom.PhysicalParams); got != len(physicalTomSpecs) {
				t.Fatalf("Tom track %d physical parameter count = %d, want %d",
					track, got, len(physicalTomSpecs))
			}
		} else if trackState.Tom != nil {
			t.Fatalf("non-Tom track %d has Tom state", track)
		}
	}

	if engine.physicalToms[tomTrackIndex] != nil || engine.physicalToms[tom2TrackIndex] != nil {
		t.Fatal("taking a state snapshot constructed an inactive physical Tom")
	}
}

func TestStateIsADeepCopy(t *testing.T) {
	engine := NewEngine(testSampleRate)
	want := engine.State()
	got := engine.State()

	got.Pattern[0] = 1
	got.Tracks[0].VoiceParams[0] = 1
	got.Tracks[tomTrackIndex].Tom.PhysicalParams[0] = 1
	got.Tracks[tomTrackIndex].Tom.Model = TomModelPhysical

	if after := engine.State(); !reflect.DeepEqual(after, want) {
		t.Fatalf("mutating a returned snapshot changed the engine:\n got %#v\nwant %#v", after, want)
	}
}

func TestReplaceStateDoesNotRetainCallerSlices(t *testing.T) {
	engine := NewEngine(testSampleRate)
	input := engine.State()
	if err := engine.ReplaceState(input); err != nil {
		t.Fatal(err)
	}
	want := engine.State()

	input.Pattern[0] = 1
	input.Tracks[0].VoiceParams[0] = 1
	input.Tracks[tomTrackIndex].Tom.PhysicalParams[0] = 1

	if got := engine.State(); !reflect.DeepEqual(got, want) {
		t.Fatalf("mutating ReplaceState input changed engine:\n got %#v\nwant %#v", got, want)
	}
}

func TestReplaceStateRoundTripsEveryBank(t *testing.T) {
	source := NewEngine(testSampleRate)
	source.SetTempo(173.25)
	source.SetSwing(0.31)
	source.SetStepCount(11)
	source.SetReverb(0.62)
	source.SetProbability(0.73)
	source.SetHumanize(0.28)

	for track := range TrackCount {
		source.SetVolume(track, 0.13+0.1*float64(track))
		source.SetDecay(track, 0.89-0.1*float64(track))
		source.SetMuted(track, track%2 == 1)

		for step := range MaxSteps {
			source.SetCell(track, step, float64((track+step)%5)/4)
		}
		for index := range SpecsForTrack(track) {
			source.SetVoiceParam(track, index, math.Mod(0.17+0.11*float64(track+index), 1))
		}
	}

	for index := range physicalTomSpecs {
		source.SetPhysicalTomParam(tomTrackIndex, index, math.Mod(0.19+0.07*float64(index), 1))
		source.SetPhysicalTomParam(tom2TrackIndex, index, math.Mod(0.83-0.03*float64(index), 1))
	}
	source.SetTomModel(tomTrackIndex, TomModelPhysical)

	want := source.State()
	target := NewEngine(testSampleRate)
	if err := target.ReplaceState(want); err != nil {
		t.Fatalf("ReplaceState failed: %v", err)
	}

	if got := target.State(); !reflect.DeepEqual(got, want) {
		t.Fatalf("round-trip mismatch:\n got %#v\nwant %#v", got, want)
	}
	if target.physicalToms[tomTrackIndex] == nil {
		t.Fatal("selected physical Tom was not constructed")
	}
	if target.physicalToms[tom2TrackIndex] != nil {
		t.Fatal("restoring the inactive Tom 2 physical bank constructed its model")
	}
	for index, wantParam := range want.Tracks[tomTrackIndex].Tom.PhysicalParams {
		if got := target.physicalToms[tomTrackIndex].Param(index); got != wantParam {
			t.Fatalf("active physical param %d = %v, want %v", index, got, wantParam)
		}
	}
	if err := target.Validate(); err != nil {
		t.Fatalf("restored engine is invalid: %v", err)
	}
}

func TestReplaceStateClampsEveryFiniteNumericClass(t *testing.T) {
	engine := NewEngine(testSampleRate)
	state := engine.State()
	state.TempoBPM = 1e6
	state.Swing = -1
	state.StepCount = MaxSteps + 100
	state.Reverb = 2
	state.Probability = -2
	state.Humanize = 3
	state.Pattern[0] = -1
	state.Pattern[1] = 2
	state.Tracks[0].Volume = 4
	state.Tracks[0].Decay = -4
	state.Tracks[0].VoiceParams[0] = 9
	state.Tracks[tomTrackIndex].Tom.PhysicalParams[physicalTomParamStrikeRadius] = -9

	if err := engine.ReplaceState(state); err != nil {
		t.Fatalf("finite out-of-range state was rejected: %v", err)
	}

	got := engine.State()
	checks := []struct {
		name string
		got  float64
		want float64
	}{
		{"tempo", got.TempoBPM, maxTempoBPM},
		{"swing", got.Swing, 0},
		{"reverb", got.Reverb, 1},
		{"probability", got.Probability, 0},
		{"humanize", got.Humanize, 1},
		{"pattern low", got.Pattern[0], 0},
		{"pattern high", got.Pattern[1], 1},
		{"volume", got.Tracks[0].Volume, 1},
		{"decay", got.Tracks[0].Decay, 0},
		{"voice param", got.Tracks[0].VoiceParams[0], 1},
		{"physical param", got.Tracks[tomTrackIndex].Tom.PhysicalParams[physicalTomParamStrikeRadius], 0},
	}
	for _, check := range checks {
		if check.got != check.want {
			t.Errorf("%s = %v, want %v", check.name, check.got, check.want)
		}
	}
	if got.StepCount != MaxSteps {
		t.Errorf("step count = %d, want %d", got.StepCount, MaxSteps)
	}
}

func TestReplaceStateRejectsMalformedOrNonFiniteBeforeMutation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*EngineState)
		want   string
	}{
		{"short pattern", func(s *EngineState) { s.Pattern = s.Pattern[:PatternSize-1] }, "pattern length"},
		{"short tracks", func(s *EngineState) { s.Tracks = s.Tracks[:TrackCount-1] }, "track count"},
		{"non-finite tempo", func(s *EngineState) { s.TempoBPM = math.NaN() }, "tempo"},
		{"non-finite swing", func(s *EngineState) { s.Swing = math.Inf(1) }, "swing"},
		{"non-finite reverb", func(s *EngineState) { s.Reverb = math.Inf(-1) }, "reverb"},
		{"non-finite probability", func(s *EngineState) { s.Probability = math.NaN() }, "probability"},
		{"non-finite humanize", func(s *EngineState) { s.Humanize = math.NaN() }, "humanize"},
		{"non-finite pattern", func(s *EngineState) { s.Pattern[80] = math.NaN() }, "pattern value 80"},
		{"non-finite volume", func(s *EngineState) { s.Tracks[2].Volume = math.NaN() }, "track 2 volume"},
		{"non-finite decay", func(s *EngineState) { s.Tracks[4].Decay = math.Inf(1) }, "track 4 decay"},
		{"short voice bank", func(s *EngineState) { s.Tracks[1].VoiceParams = nil }, "track 1 voice parameter count"},
		{"non-finite voice param", func(s *EngineState) { s.Tracks[1].VoiceParams[0] = math.NaN() }, "track 1 voice parameter 0"},
		{"missing Tom state", func(s *EngineState) { s.Tracks[tomTrackIndex].Tom = nil }, "has no Tom state"},
		{"invalid Tom model", func(s *EngineState) { s.Tracks[tomTrackIndex].Tom.Model = TomModel(9) }, "invalid model"},
		{"short physical bank", func(s *EngineState) { s.Tracks[tomTrackIndex].Tom.PhysicalParams = nil }, "physical parameter count"},
		{"non-finite physical param", func(s *EngineState) {
			s.Tracks[tomTrackIndex].Tom.PhysicalParams[3] = math.NaN()
		}, "physical parameter 3"},
		{"Tom state on cymbal", func(s *EngineState) { s.Tracks[4].Tom = &TomState{} }, "non-Tom track 4"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			engine := configuredEngine()
			engine.SetMuted(1, true)
			before := engine.State()
			invalid := engine.State()
			test.mutate(&invalid)

			err := engine.ReplaceState(invalid)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ReplaceState error = %v, want it to mention %q", err, test.want)
			}
			if after := engine.State(); !reflect.DeepEqual(after, before) {
				t.Fatalf("rejected state partially mutated engine:\n got %#v\nwant %#v", after, before)
			}
		})
	}
}

func TestMuteRampsWithoutOverwritingStoredVolume(t *testing.T) {
	engine := NewEngine(testSampleRate)
	engine.SetVolume(0, 0.65)
	engine.SetRunning(true) // keep the empty engine out of the idle fast path

	engine.SetMuted(0, true)
	if got := engine.State().Tracks[0].Volume; got != 0.65 {
		t.Fatalf("muting changed stored volume to %v, want 0.65", got)
	}
	renderTotal(engine, int(testSampleRate/10))
	if engine.liveVol[0] > 0.001 {
		t.Fatalf("muted live volume = %v after 100 ms, want near zero", engine.liveVol[0])
	}

	engine.SetMuted(0, false)
	renderTotal(engine, int(testSampleRate/10))
	if math.Abs(engine.liveVol[0]-0.65) > 0.001 {
		t.Fatalf("unmuted live volume = %v after 100 ms, want stored 0.65", engine.liveVol[0])
	}
	if engine.volumes[0] != 0.65 {
		t.Fatalf("mute round trip changed stored volume to %v", engine.volumes[0])
	}
}

func TestSetMutedRejectsInvalidTrack(t *testing.T) {
	engine := NewEngine(testSampleRate)
	for _, track := range []int{-1, TrackCount, math.MaxInt} {
		engine.SetMuted(track, true)
		if engine.Muted(track) {
			t.Fatalf("Muted(%d) = true for invalid track", track)
		}
	}

	for track, muted := range engine.muted {
		if muted {
			t.Fatalf("invalid mute changed track %d", track)
		}
	}
}

func TestUnmuteWakesPossiblyFrozenTail(t *testing.T) {
	engine := NewEngine(testSampleRate)
	engine.SetMuted(0, true)
	engine.silentRun = engine.idleSamples

	engine.SetMuted(0, false)

	if engine.IsIdle() {
		t.Fatal("unmuting left the engine in its frozen idle path")
	}
}
