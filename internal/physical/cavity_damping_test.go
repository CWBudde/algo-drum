package physical

import (
	"math"
	"testing"
)

// The 630 Hz third-octave band, which is where
// TestTheRenderedDampingIsNotMonotoneAndTheCavityIsWhy found the anomaly. Stated
// as edges rather than as a centre so the containment check below reads as the
// question it is asking.
const (
	anomalyBandLowHz  = 630.0 / bandEdgeRatio
	anomalyBandHighHz = 630.0 * bandEdgeRatio
)

// bandEdgeRatio is the third-octave half-width, 2^(1/6).
const bandEdgeRatio = 1.1224620483093730

// TestTheCavityIsUnderdampedAgainstTheHeadLossLaw names the mechanism behind the
// 630 Hz band and puts a number on it.
//
// TestTheRenderedDampingIsNotMonotoneAndTheCavityIsWhy establishes *that* the
// cavity is responsible, by ablation: the band rings 0.743 s with the enclosed
// air and 0.225 s without. What it could not say is which cavity mode carries it
// or by how much the air is out of step, because it measures a band rather than
// a mode. This does both, and it is the first half of PLAN.md N19.
//
// The answer is not subtle once the two are put side by side. The cavity's (1,1)
// transverse pair sits at 659.5 Hz, inside the 630 Hz band, and the cavity's
// single damping field puts its equivalent zeta a factor of about twelve below
// the constant-zeta law every head mode in that region obeys. So the band is not
// measuring a head at all: it is measuring the one element in the instrument
// that the damping calibration never reached.
//
// This asserts the *disagreement*, not a target. Nothing here says what the
// cavity's loss should be — that is the open half of N19, and it needs
// docs/physical-cavity.md and a real drum rather than a test. What this pins is
// that the disagreement is real, is about the (1,1) pair specifically, and does
// not quietly go away. If a future calibration closes it, this test fails and
// the closing is what gets written down.
func TestTheCavityIsUnderdampedAgainstTheHeadLossLaw(t *testing.T) {
	t.Parallel()

	config := DefaultPhysicalDrum()
	config.SampleRateHz = 48_000

	model, err := NewDoubleHead(config)
	if err != nil {
		t.Fatalf("NewDoubleHead: %v", err)
	}

	// The cavity's second-order sections are (s^2 + gamma*s + omega^2) — see
	// frequency_response.go, and the matching 2/dt + gamma + ... diagonal in
	// solveCavityMidpoint — so LossPerSecond is the coefficient of s and the
	// envelope decays at half of it. Getting this factor wrong is the whole
	// difference between "the cavity is out by twelve" and "by six", so it is
	// derived here rather than quoted.
	cavityDecayPerSecond := config.Cavity.LossPerSecond / 2

	// The mode the band anomaly lives on, found by frequency rather than by
	// index: the cavity bank's composition is documented in config.go and may
	// legitimately change, and an index would then silently point at another
	// mode.
	carrier := -1

	for index := range model.cavityModes {
		mode := &model.cavityModes[index]
		if mode.FrequencyHz >= anomalyBandLowHz && mode.FrequencyHz <= anomalyBandHighHz {
			carrier = index

			break
		}
	}

	if carrier < 0 {
		t.Fatalf(
			"no cavity mode lies in the %.1f-%.1f Hz band, so the diagnosis in "+
				"TestTheRenderedDampingIsNotMonotoneAndTheCavityIsWhy no longer has "+
				"a mechanism behind it",
			anomalyBandLowHz, anomalyBandHighHz,
		)
	}

	mode := &model.cavityModes[carrier]
	cavityZeta := cavityDecayPerSecond / mode.AngularFrequency
	cavityT60 := math.Log(1000) / cavityDecayPerSecond

	t.Logf(
		"cavity (%d,%d) at %.2f Hz: decay %.3f /s, T60 %.4f s, zeta %.3e",
		mode.AzimuthalOrder, mode.RadialOrder, mode.FrequencyHz,
		cavityDecayPerSecond, cavityT60, cavityZeta,
	)

	// Every head mode sharing the band, as the thing the cavity is out of step
	// with. Both heads, because the band measurement hears both.
	slowestHeadT60, headModes := 0.0, 0

	for index := range model.modes {
		head := &model.modes[index]
		if head.FrequencyHz < anomalyBandLowHz || head.FrequencyHz > anomalyBandHighHz {
			continue
		}

		headModes++
		slowestHeadT60 = max(slowestHeadT60, math.Log(1000)/head.DecayRatePerSecond)
	}

	if headModes == 0 {
		t.Fatal("no head mode shares the band, so there is nothing to compare against")
	}

	t.Logf(
		"%d head modes share the band; the slowest rings %.4f s, against the "+
			"cavity's %.4f s — a factor of %.1f",
		headModes, slowestHeadT60, cavityT60, cavityT60/slowestHeadT60,
	)

	// The heads are held to targetZeta within a factor of 1.5 by
	// TestDefaultDampingHoldsConstantQ. The claim is that the cavity is not
	// merely outside that window but far outside it, so the bound is the window's
	// own lower edge rather than a number chosen here.
	if cavityZeta >= targetZeta*targetZetaLowFactor {
		t.Errorf(
			"the cavity's equivalent zeta %.3e is no longer below the head loss "+
				"law's own lower edge %.3e — the 630 Hz anomaly has lost its "+
				"explanation and PLAN.md N19 must be rewritten against whatever "+
				"replaced it",
			cavityZeta, targetZeta*targetZetaLowFactor,
		)
	}

	// Measured at 10.9 against the slowest head mode in the band, which is the
	// conservative reading; in equivalent zeta, where the comparison is against
	// the law rather than against one mode, the gap is 11.9. Both are logged
	// above and they are ratios of different quantities, so neither is a
	// correction of the other.
	//
	// Asserted loosely, at 4, because the point is the order of
	// magnitude and not the digit: a recalibration that halved the gap would
	// still leave the cavity the least-damped element in the region, and this
	// test should keep saying so rather than failing on a number it never
	// justified.
	const leastDampedByAtLeast = 4.0

	if ratio := cavityT60 / slowestHeadT60; ratio < leastDampedByAtLeast {
		t.Errorf(
			"the cavity now rings only %.1fx the slowest head mode in the band, "+
				"under the %.0fx this test exists to record",
			ratio, leastDampedByAtLeast,
		)
	}
}
