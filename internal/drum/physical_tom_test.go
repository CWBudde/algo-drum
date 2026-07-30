package drum

import (
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/cwbudde/algo-drum/internal/physical"
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

// TestPhysicalTomConfigAtDefaultsIsTheShippedModel pins the one property that
// makes the exported mapping trustworthy to an offline fitter: an untouched
// bank, at the strip's neutral DEC position, reproduces DefaultPhysicalDrum()
// exactly. Every knob's Shipped constant is supposed to be the model default it
// was read from, and the multiplicative knobs (DAMP, D.TILT, NLIN) are supposed
// to be neutral at 1. If either drifts, a fit run offline would be measuring a
// different instrument than the one the app ships, and nothing else would say
// so.
func TestPhysicalTomConfigAtDefaultsIsTheShippedModel(t *testing.T) {
	t.Parallel()

	const sampleRate = 48_000

	defaults := make([]float64, len(physicalTomSpecs))
	for i, spec := range physicalTomSpecs {
		defaults[i] = spec.Default
	}

	got, err := PhysicalTomConfig(defaults, NeutralDecayAmount, sampleRate)
	if err != nil {
		t.Fatal(err)
	}

	want := physical.DefaultPhysicalDrum()
	want.SampleRateHz = sampleRate

	// Two departures exist and are enumerated here rather than tolerated by a
	// loose comparison, so that a third one fails this test.
	//
	// MIC.R: the bank ships 0.32 where the model's fitted close-microphone
	// geometry is 0.65 of the radius (docs/physical-calibration.md). The
	// difference is audible — it is the partial structure the near-field term
	// was fitted at — so it is recorded, not quietly corrected here.
	want.Pickup.Radius01 = physicalTomSpecs[physicalTomParamPickupRadius].Shipped
	// MIC.A: the knob is in degrees, so the shipped 0.6 rad round-trips through
	// a division and a multiplication by 180/π and lands one ulp away.
	want.Pickup.AngleRad = physicalTomSpecs[physicalTomParamPickupAngle].Shipped *
		math.Pi / 180

	if !reflect.DeepEqual(got, want) {
		t.Errorf("PhysicalTomConfig at defaults = %+v, want %+v", got, want)
	}
}

// TestPhysicalTomConfigMatchesTheVoice keeps the exported mapping and the voice
// that uses it from diverging: whatever the voice ends up configured with, the
// function must reproduce from the same bank.
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

	want, err := PhysicalTomConfig(voice.params.vals, 0.8, 48_000)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(voice.config, want) {
		t.Errorf("voice config = %+v, want %+v", voice.config, want)
	}
}

func TestPhysicalTomConfigRejectsTheWrongBankWidth(t *testing.T) {
	t.Parallel()

	if _, err := PhysicalTomConfig(make([]float64, len(physicalTomSpecs)-1), 0.5, 48_000); !errors.Is(err, ErrPhysicalTomParamCount) {
		t.Errorf("error = %v, want ErrPhysicalTomParamCount", err)
	}
}
