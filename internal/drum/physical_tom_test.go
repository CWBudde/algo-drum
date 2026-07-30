package drum

import (
	"math"
	"testing"
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
