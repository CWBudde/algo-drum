package drum

import (
	"fmt"
	"math"
	"testing"

	"github.com/cwbudde/algo-drum/internal/physical"
)

// physicalTomReleaseCapSeconds bounds the measurement itself, so a voice that
// never releases fails in seconds rather than hanging the suite. It is above
// physical.ReleaseBoundSeconds with room to report a real overrun, and far
// below the worst case this sweep found before the bound existed (see
// TestTheVoiceReleasesAtEveryKnobPosition's comment).
const physicalTomReleaseCapSeconds = 12.0

// releaseSeconds triggers a fresh physical Tom at the given knob position and
// returns how long it stayed active, in seconds of audio.
//
// The second return is false when the voice was still active at the cap, in
// which case the duration is the cap and not a measurement.
func releaseSeconds(t *testing.T, values01 map[int]float64, decayAmount float64) (float64, bool) {
	t.Helper()

	voice, err := newPhysicalTom(testSampleRate)
	if err != nil {
		t.Fatal(err)
	}

	for index, value := range values01 {
		voice.SetParam(index, value)
	}

	voice.SetDecay(decayAmount)
	voice.Trigger(1)

	maxSamples := int(testSampleRate * physicalTomReleaseCapSeconds)
	for sampleIndex := range maxSamples {
		sample := voice.Tick()
		if math.IsNaN(sample) || math.IsInf(sample, 0) {
			t.Fatalf("non-finite sample at %d: %v", sampleIndex, sample)
		}

		if !voice.IsActive() {
			return float64(sampleIndex+1) / testSampleRate, true
		}
	}

	return physicalTomReleaseCapSeconds, false
}

// TestTheVoiceReleasesAtEveryKnobPosition is the physical Tom's half of the
// contract TestVoiceParamExtremesStayFiniteAndTerminate holds for the
// procedural voices: whatever the knobs say, the voice eventually stops.
//
// It is a separate test rather than an entry in newTestVoices() because the
// physical model does not satisfy that map's other contracts, and should not be
// made to. TestVoicesVelocityScalesPeak asserts a peak linear in velocity to
// within 2 %, which the Berger tension nonlinearity exists to violate — a
// hard hit is meant to be brighter and not merely louder. Enrolling the voice
// there would either fail for a correct reason or force the nonlinearity to be
// switched off for the test, and both are worse than one focused test.
//
// The sweep is at DEC 1, the ring-maximising end: the strip trim enters as the
// divisor 0.5+amount, so the largest position is the smallest loss scale.
//
// Measured 2026-08-02 before the release bound existed, with the cap lifted to
// 120 s: the shipped default releases at 1.527 s, D.TILT at its lower stop
// alone reaches 21.5 s, and the DAMP-min / D.TILT-0 / DEC-max corner reaches
// 65.3 s. The 14x excursion from a single knob is scaleHeadLosses multiplying
// d1, d2 *and* every mode-decay correction by lossScale*tilt, so a zero tilt
// leaves only d0 and the flat radiation term.
func TestTheVoiceReleasesAtEveryKnobPosition(t *testing.T) {
	t.Parallel()

	specs := physicalTomSpecs

	type knobCase struct {
		name   string
		values map[int]float64
		decay  float64
	}

	cases := []knobCase{{name: "defaults", values: nil, decay: NeutralDecayAmount}}

	for index := range specs {
		for _, value := range []float64{0, 1} {
			cases = append(cases, knobCase{
				name:   fmt.Sprintf("%s=%g", specs[index].Label, value),
				values: map[int]float64{index: value},
				decay:  1,
			})
		}
	}

	// The reasoned worst case, named rather than left to the one-knob sweep to
	// stumble on: every multiplier that lengthens the ring at once.
	cases = append(cases, knobCase{
		name: "DAMP=0 D.TILT=0 DEC=1",
		values: map[int]float64{
			physicalTomParamDamping:     0,
			physicalTomParamDampingTilt: 0,
		},
		decay: 1,
	})

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			// One render per case is seconds of audio each; without this the
			// sweep is the slowest thing in the package by a wide margin.
			t.Parallel()

			seconds, released := releaseSeconds(t, testCase.values, testCase.decay)
			if !released {
				t.Errorf(
					"still active after %.0f s — the voice never releases",
					physicalTomReleaseCapSeconds,
				)

				return
			}

			t.Logf("released after %.3f s", seconds)

			if seconds > physical.ReleaseBoundSeconds {
				t.Errorf(
					"released after %.3f s, want at most %.1f s",
					seconds, physical.ReleaseBoundSeconds,
				)
			}
		})
	}
}
