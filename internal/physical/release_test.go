package physical

import (
	"math"
	"testing"
)

// tickUntilInactive renders a model until IsActive goes false and returns how
// long that took in seconds, plus whether it happened inside the cap.
func tickUntilInactive(t *testing.T, model *DoubleHead, capSeconds float64) (float64, bool) {
	t.Helper()

	sampleRate := model.Config().SampleRateHz
	for sampleIndex := range int(capSeconds * sampleRate) {
		model.Tick()

		if !model.IsActive() {
			return float64(sampleIndex+1) / sampleRate, true
		}
	}

	return capSeconds, false
}

// TestALosslessHeadStillReleases is the case Validate deliberately does not
// reject.
//
// A head with no loss at all is a legitimate analytic object — three
// conservation tests and the discrete-gradient passivity argument are built on
// one — so the validator must keep accepting it. What must not happen is that
// accepting it produces a voice that runs forever, and before the release bound
// that is exactly what it produced.
func TestALosslessHeadStillReleases(t *testing.T) {
	t.Parallel()

	config := losslessDoubleHeadConfig()

	model, err := NewDoubleHead(config)
	if err != nil {
		t.Fatalf("a lossless head must still validate: %v", err)
	}

	if err := model.Trigger(1); err != nil {
		t.Fatal(err)
	}

	seconds, released := tickUntilInactive(t, model, 2*ReleaseBoundSeconds)
	if !released {
		t.Fatalf("still active after %.1f s", 2*ReleaseBoundSeconds)
	}

	t.Logf("lossless head released after %.3f s", seconds)

	if seconds > ReleaseBoundSeconds {
		t.Errorf("released after %.3f s, want at most %.1f s", seconds, ReleaseBoundSeconds)
	}
}

// TestAZeroInactivityThresholdStillReleases closes the second door into the
// same hang.
//
// InactiveEnergyThresholdJ is validated over [0, 1] and may be exactly 0, at
// which point `energy > threshold` is true for any nonzero energy however small
// — an immortal voice reached without touching the loss law at all. The bound
// does not care which door was opened.
func TestAZeroInactivityThresholdStillReleases(t *testing.T) {
	t.Parallel()

	config := DefaultPhysicalDrum()
	config.Batter.InactiveEnergyThresholdJ = 0
	config.Resonant.InactiveEnergyThresholdJ = 0

	if err := config.Validate(); err != nil {
		t.Fatalf("a zero inactivity threshold must still validate: %v", err)
	}

	model, err := NewDoubleHead(config)
	if err != nil {
		t.Fatal(err)
	}

	if err := model.Trigger(1); err != nil {
		t.Fatal(err)
	}

	seconds, released := tickUntilInactive(t, model, 2*ReleaseBoundSeconds)
	if !released {
		t.Fatalf("still active after %.1f s", 2*ReleaseBoundSeconds)
	}

	t.Logf("zero-threshold model released after %.3f s", seconds)

	if seconds > ReleaseBoundSeconds {
		t.Errorf("released after %.3f s, want at most %.1f s", seconds, ReleaseBoundSeconds)
	}
}

// TestTheReleaseBoundIsInertOnTheShippedVoice is what makes the bound safe to
// add to a calibrated model: on everything that ships it never runs.
//
// The margin is asserted rather than merely observed, because a bound that
// happened to sit just past the shipped release would be shaping the sound
// without anyone deciding that it should. `just check-physical-reference` is
// the other half of this proof — the analysis renders 1.2-2 s windows, so a
// bound that had started biting would move the CI-diffed fixture.
func TestTheReleaseBoundIsInertOnTheShippedVoice(t *testing.T) {
	t.Parallel()

	// The shipped voice releases at about 1.5 s. Requiring a factor of two of
	// clearance leaves the calibration room to move without silently arming
	// this, and fails loudly if it ever moves far enough to matter.
	const wantClearance = 2.0

	model, err := NewDoubleHead(DefaultPhysicalDrum())
	if err != nil {
		t.Fatal(err)
	}

	if err := model.Trigger(1); err != nil {
		t.Fatal(err)
	}

	seconds, released := tickUntilInactive(t, model, ReleaseBoundSeconds)
	if !released {
		t.Fatalf("the shipped voice did not release inside its own bound")
	}

	t.Logf("shipped voice released after %.3f s, bound %.1f s", seconds, ReleaseBoundSeconds)

	if seconds*wantClearance > ReleaseBoundSeconds {
		t.Errorf(
			"shipped voice releases after %.3f s, within %gx of the %.1f s bound — "+
				"the bound is no longer inert and is shaping the shipped sound",
			seconds, wantClearance, ReleaseBoundSeconds,
		)
	}
}

// TestTheReleaseFadeIsMonotoneAndReachesZero pins the ramp itself.
//
// The fade exists because physicalTom.Tick hard-returns 0 the moment IsActive
// goes false, so the gain has to arrive at zero before that happens rather than
// at the same moment. Both halves are asserted: it reaches exactly zero, and it
// does so no later than the deadline the voice is declared over at.
func TestTheReleaseFadeIsMonotoneAndReachesZero(t *testing.T) {
	t.Parallel()

	const sampleRate = 48_000.0

	bound := newReleaseBound(sampleRate)

	previous := math.Inf(1)
	sawZero := false

	for sample := range bound.boundSamples {
		gain := bound.advance()

		if gain < 0 || gain > 1 {
			t.Fatalf("gain %v at sample %d outside [0, 1]", gain, sample)
		}

		if gain > previous {
			t.Fatalf("gain rose from %v to %v at sample %d", previous, gain, sample)
		}

		previous = gain

		if gain == 0 {
			sawZero = true
		}

		if bound.expired() != (sample+1 >= bound.boundSamples) {
			t.Fatalf("expired() disagrees with the deadline at sample %d", sample)
		}
	}

	if !sawZero {
		t.Error("the fade never reached zero before the voice was declared over")
	}

	if !bound.expired() {
		t.Error("the bound did not expire at its own deadline")
	}
}

// TestATriggerReArmsTheReleaseBound is why a roll is unaffected.
//
// The deadline is measured from the last strike, not from the first, so a
// played passage never trips it; only a voice nobody is hitting does.
func TestATriggerReArmsTheReleaseBound(t *testing.T) {
	t.Parallel()

	const sampleRate = 48_000.0

	bound := newReleaseBound(sampleRate)
	for range bound.boundSamples - 1 {
		bound.advance()
	}

	if bound.expired() {
		t.Fatal("expired one sample early")
	}

	bound.restart()

	if bound.expired() {
		t.Fatal("still expired after a strike re-armed it")
	}

	if gain := bound.advance(); gain != 1 {
		t.Errorf("gain %v on the sample after a strike, want 1", gain)
	}
}
