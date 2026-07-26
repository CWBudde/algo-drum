package drum

import (
	"math"
	"testing"
)

func TestParamSpecsWellFormed(t *testing.T) {
	seen := map[string]string{}

	for track := range TrackCount {
		specs := SpecsForTrack(track)
		if len(specs) == 0 {
			t.Fatalf("track %d has no parameters", track)
		}

		if len(specs) > maxVoiceParams {
			t.Fatalf("track %d has %d parameters, over the %d cap",
				track, len(specs), maxVoiceParams)
		}

		for i, spec := range specs {
			if spec.ID == "" || spec.Label == "" || spec.Name == "" {
				t.Errorf("track %d param %d has an empty ID/Label/Name: %+v", track, i, spec)
			}

			if prev, dup := seen[spec.ID]; dup {
				t.Errorf("duplicate param ID %q on %s and track %d", spec.ID, prev, track)
			}

			seen[spec.ID] = VoiceName(track)

			if !(spec.Min < spec.Max) {
				t.Errorf("%s: Min %v is not below Max %v", spec.ID, spec.Min, spec.Max)
			}

			if spec.Kind == paramExp && spec.Min <= 0 {
				t.Errorf("%s: exponential params need a positive Min, got %v", spec.ID, spec.Min)
			}

			if spec.Shipped < spec.Min || spec.Shipped > spec.Max {
				t.Errorf("%s: Shipped %v outside [%v, %v]",
					spec.ID, spec.Shipped, spec.Min, spec.Max)
			}

			if spec.Default < 0 || spec.Default > 1 {
				t.Errorf("%s: Default %v outside [0, 1]", spec.ID, spec.Default)
			}

			if spec.Digits < 0 {
				t.Errorf("%s: negative Digits %d", spec.ID, spec.Digits)
			}
		}
	}
}

// TestParamSpecDefaultMapsToShippedExactly is what the byte-step snap in Map
// buys: a knob left (or reset) at its default must produce the exact constant
// the voice shipped with, not a value one ulp away.
func TestParamSpecDefaultMapsToShippedExactly(t *testing.T) {
	for track := range TrackCount {
		for _, spec := range SpecsForTrack(track) {
			if got := spec.Map(spec.Default); got != spec.Shipped {
				t.Errorf("%s: Map(Default) = %v, want Shipped %v", spec.ID, got, spec.Shipped)
			}
		}
	}
}

// TestParamSpecSnapSurvivesByteQuantisation covers the actual failure the snap
// exists for: persistence stores every scalar as one byte, so a default round-
// trips as a slightly different float.
func TestParamSpecSnapSurvivesByteQuantisation(t *testing.T) {
	for track := range TrackCount {
		for _, spec := range SpecsForTrack(track) {
			quantised := math.Round(spec.Default*255) / 255
			if got := spec.Map(quantised); got != spec.Shipped {
				t.Errorf("%s: Map(%v) = %v after byte round-trip, want Shipped %v",
					spec.ID, quantised, got, spec.Shipped)
			}
		}
	}
}

func TestParamSpecMapEndpointsAndMonotonicity(t *testing.T) {
	for track := range TrackCount {
		for _, spec := range SpecsForTrack(track) {
			// The snap only covers a ±half-byte window, so unless a default
			// sits at an endpoint the endpoints map to Min/Max exactly.
			if got := spec.Map(0); got != spec.Min {
				t.Errorf("%s: Map(0) = %v, want Min %v", spec.ID, got, spec.Min)
			}

			if got := spec.Map(1); got != spec.Max {
				t.Errorf("%s: Map(1) = %v, want Max %v", spec.ID, got, spec.Max)
			}

			prev := spec.Map(0)

			for step := 1; step <= 100; step++ {
				got := spec.Map(float64(step) / 100)
				if got < prev {
					t.Fatalf("%s: Map is not monotonic at %d/100: %v after %v",
						spec.ID, step, got, prev)
				}

				prev = got
			}
		}
	}
}

func TestParamSpecMapClampsBadInput(t *testing.T) {
	spec := bassSpecs[bassParamPitchTo]

	for name, in := range map[string]float64{
		"negative":  -1,
		"above one": 2,
		"NaN":       math.NaN(),
		"+Inf":      math.Inf(1),
		"-Inf":      math.Inf(-1),
	} {
		got := spec.Map(in)
		if got < spec.Min || got > spec.Max {
			t.Errorf("%s input: Map(%v) = %v, outside [%v, %v]",
				name, in, got, spec.Min, spec.Max)
		}
	}
}

// TestParamSpecIDsAreStable pins the persistence addresses. Parameter values
// travel in share links as (track, index) byte slots, so reordering or
// renaming an entry silently reinterprets every link already in the wild —
// append only.
func TestParamSpecIDsAreStable(t *testing.T) {
	want := [TrackCount][]string{
		{"bass.pitchFrom", "bass.pitchTo", "bass.sweepTime", "bass.sweepRate", "bass.decay"},
		{
			"snare.toneHz", "snare.toneLevel", "snare.toneDecay",
			"snare.noiseDecay", "snare.hpHz", "snare.hpQ",
		},
		{"hat.bpHz", "hat.bpQ", "hat.decay", "hat.gain"},
		{
			"tom.pitchFrom", "tom.pitchTo", "tom.sweepTime",
			"tom.sweepRate", "tom.decay", "tom.gain",
		},
		{"cym.bpHz", "cym.bpQ", "cym.decay", "cym.gain"},
	}

	for track := range TrackCount {
		specs := SpecsForTrack(track)
		if len(specs) != len(want[track]) {
			t.Fatalf("track %d has %d params, want %d", track, len(specs), len(want[track]))
		}

		for i, spec := range specs {
			if spec.ID != want[track][i] {
				t.Errorf("track %d param %d is %q, want %q", track, i, spec.ID, want[track][i])
			}
		}
	}
}

// TestParamSpecMapMatchesTheGeneratedCurve pins a handful of triples that the
// Vitest counterpart asserts against the generated TypeScript table, so the
// two curve implementations cannot drift apart unnoticed.
func TestParamSpecMapMatchesTheGeneratedCurve(t *testing.T) {
	cases := []struct {
		id    string
		specs []ParamSpec
		index int
		v01   float64
		want  float64
	}{
		{"bass.pitchTo", bassSpecs, bassParamPitchTo, 0, 25},
		{"bass.pitchTo", bassSpecs, bassParamPitchTo, 1, 120},
		{"bass.pitchTo", bassSpecs, bassParamPitchTo, 0.25, 25 * math.Pow(120.0/25.0, 0.25)},
		{"hat.gain", hatSpecs, hatParamGain, 0.25, 0.625},
		{"hat.gain", hatSpecs, hatParamGain, 0.8, 2.0},
		{"cym.decay", cymSpecs, cymParamDecay, 0.75, 0.1 * math.Pow(40, 0.75)},
	}

	for _, tc := range cases {
		got := tc.specs[tc.index].Map(tc.v01)
		if math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("%s: Map(%v) = %v, want %v", tc.id, tc.v01, got, tc.want)
		}
	}
}

func TestSpecsForTrackRejectsBadTrack(t *testing.T) {
	for _, track := range []int{-1, TrackCount, math.MaxInt} {
		if specs := SpecsForTrack(track); specs != nil {
			t.Errorf("SpecsForTrack(%d) = %v, want nil", track, specs)
		}

		if name := VoiceName(track); name != "" {
			t.Errorf("VoiceName(%d) = %q, want empty", track, name)
		}
	}
}

// TestClampDesignHzKeepsFiltersDefined guards the silent-voice failure mode:
// design.Bandpass/Highpass return all-zero coefficients at or above Nyquist.
func TestClampDesignHzKeepsFiltersDefined(t *testing.T) {
	for _, sr := range []float64{8000, 44100, 48000} {
		for _, hz := range []float64{0, 1, 1e6, sr, sr * 2} {
			got := clampDesignHz(hz, sr)
			if got <= 0 || got >= sr/2 {
				t.Errorf("clampDesignHz(%v, %v) = %v, not inside (0, Nyquist)", hz, sr, got)
			}
		}
	}
}
