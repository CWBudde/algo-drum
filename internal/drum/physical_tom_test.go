package drum

import (
	"math"
	"reflect"
	"testing"

	"github.com/cwbudde/algo-tom/tomparams"
)

// TestPhysicalTomReachesProductLevelWithoutACompensatingGain is the assertion
// that keeps physicalTomOutputGain deleted.
//
// The voice used to multiply the model's output by 4 to reach usable level,
// because the radiated sum was a modal velocity weighted by a far-field
// efficiency times a near-field mode shape — a quantity with no physical
// magnitude, so its level had to be recovered downstream. It is now a calibrated
// volume acceleration and Pickup.OutputGain is fitted against this measurement,
// so a compensating factor in the voice would mean the calibration has drifted
// rather than that the level needs help.
func TestPhysicalTomReachesProductLevelWithoutACompensatingGain(t *testing.T) {
	t.Parallel()

	const (
		minimumPeak = 0.70
		maximumPeak = 0.95
	)

	peakAt := func(velocity float64) float64 {
		voice, err := newPhysicalTom(48_000)
		if err != nil {
			t.Fatal(err)
		}

		voice.Trigger(velocity)

		peak := 0.0
		for range 96_000 {
			peak = max(peak, math.Abs(voice.Tick()))
		}

		return peak
	}

	loud := peakAt(1)
	t.Logf("velocity 1 peak %.4f", loud)
	if loud < minimumPeak || loud > maximumPeak {
		t.Fatalf(
			"velocity-1 peak %.4f outside [%.2f, %.2f]; refit Pickup.OutputGain "+
				"rather than reintroducing a gain in the voice",
			loud,
			minimumPeak,
			maximumPeak,
		)
	}

	// And it must still have dynamics: a limiter-friendly peak is no use if
	// every velocity arrives at the same level.
	quiet := peakAt(0.35)
	t.Logf("velocity 0.35 peak %.4f", quiet)
	if quiet >= loud*0.6 {
		t.Fatalf(
			"velocity-0.35 peak %.4f is not clearly below the velocity-1 peak %.4f",
			quiet,
			loud,
		)
	}
}

// TestPhysicalTomConfigMatchesTheVoice keeps the voice and the shared mapping
// from diverging: whatever the voice ends up configured with, tomparams.Config
// must reproduce from the same bank.
//
// It is the assertion that makes the *fitter* trustworthy. The fitter scores
// candidates by calling tomparams.Config directly; the voice reaches it through
// reconfigure. If those two ever parted company, every offline fit would be
// describing an instrument the product does not ship, and nothing upstream
// could notice — tomparams cannot see this package's voice.
func TestPhysicalTomConfigMatchesTheVoice(t *testing.T) {
	t.Parallel()

	voice, err := newPhysicalTom(48_000)
	if err != nil {
		t.Fatal(err)
	}

	// A spread of positions, none of them a default, so every branch of the
	// mapping is exercised away from its detent.
	for index := range physicalTomSpecs {
		voice.SetParam(index, math.Mod(0.17+0.23*float64(index), 1))
	}

	voice.SetDecay(0.8)

	want, err := tomparams.Config(voice.params.vals, 0.8, 48_000)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(voice.config, want) {
		t.Errorf("voice config = %+v, want %+v", voice.config, want)
	}
}
