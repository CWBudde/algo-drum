package physical

import (
	"math"
	"testing"
)

// silenceLosses removes every loss channel from a head. Tests that check the
// conservative core — exact state transitions, energy exchange, the passive
// cavity solve, the discrete-gradient nonlinear update — need a genuinely
// lossless configuration, so this must cover all four channels. Missing one
// turns an energy-conservation assertion into an unexplained slow drift.
func silenceLosses(head *Head) {
	head.Loss0PerSecond = 0
	head.Loss1MPerSecond = 0
	head.Loss2M2PerSecond = 0
	head.RadiationLossPerSecond = 0
	head.ModeDecayCorrections = nil
}

// setUniformLoss silences the frequency-dependent channels and leaves one flat
// decay rate, for tests that want a known, mode-independent envelope.
func setUniformLoss(head *Head, ratePerSecond float64) {
	silenceLosses(head)
	head.Loss0PerSecond = ratePerSecond
}

// modeByOrder finds one retained mode by its (m,n) index. Non-axisymmetric
// modes return whichever orientation comes first; their decay rates are equal.
func modeByOrder(modes []Mode, azimuthalOrder, radialOrder int) (Mode, bool) {
	for _, mode := range modes {
		if mode.AzimuthalOrder == azimuthalOrder &&
			mode.RadialOrder == radialOrder {
			return mode, true
		}
	}

	return Mode{}, false
}

func t60Milliseconds(decayRatePerSecond float64) float64 {
	return 1000 * math.Log(1000) / decayRatePerSecond
}

// The constant-Q window the shipped loss law is calibrated to hold.
//
// Package-level rather than local to the test below, because
// damping_band_test.go derives its own tolerance from this window: the shape
// budget the mode table is allowed is exactly the shape budget the rendered
// measurement must be read against, and two copies of it would drift.
const (
	targetZeta           = 0.0072
	targetZetaLowFactor  = 0.9
	targetZetaHighFactor = 1.35
)

// TestDefaultDampingHoldsConstantQ pins the *shape* of the decay law, not only
// its scale. A bank damped at one uniform rate satisfies every other assertion
// in this package while sounding like a struck sine bank rather than a drum,
// which is how the flat law shipped: T60 has to fall with frequency.
func TestDefaultDampingHoldsConstantQ(t *testing.T) {
	t.Parallel()

	config := DefaultPhysicalDrum()
	modes, err := generateHeadModes(config, config.Batter)
	if err != nil {
		t.Fatal(err)
	}

	// The (0,1) is deliberately over-damped relative to the series by the
	// two-head correction, so the constant-Q band starts above it.
	//
	// 0.72 %, not the 1.1 % this was written against: the retuned default holds
	// the ζ the old coefficients happened to produce at the top of B.TUNE's old
	// range, which is the tuning that sounded right. RetuneTension now keeps this
	// number fixed across the knob's whole travel, where it used to run from
	// 2.20 % to 0.72 %.
	for _, mode := range modes {
		if mode.AzimuthalOrder == 0 && mode.RadialOrder == 1 {
			continue
		}

		zeta := mode.DecayRatePerSecond / mode.AngularFrequency
		if zeta < targetZeta*targetZetaLowFactor || zeta > targetZeta*targetZetaHighFactor {
			t.Errorf(
				"mode (%d,%d) at %.1f Hz: zeta = %.4f, want %.3f within [%g,%g]x",
				mode.AzimuthalOrder,
				mode.RadialOrder,
				mode.FrequencyHz,
				zeta,
				targetZeta,
				targetZetaLowFactor,
				targetZetaHighFactor,
			)
		}
	}
}

// TestDefaultDampingDecaysFasterWithFrequency is the coarse, law-independent
// version of the same property: the highest retained mode must die much sooner
// than the lowest. Under the former flat law this ratio was 1.0.
func TestDefaultDampingDecaysFasterWithFrequency(t *testing.T) {
	t.Parallel()

	config := DefaultPhysicalDrum()
	modes, err := generateHeadModes(config, config.Batter)
	if err != nil {
		t.Fatal(err)
	}

	lowest, highest := modes[0], modes[0]
	for _, mode := range modes {
		if mode.AzimuthalOrder == 0 && mode.RadialOrder == 1 {
			continue
		}
		if lowest.AzimuthalOrder == 0 && lowest.RadialOrder == 1 {
			lowest, highest = mode, mode
		}
		if mode.FrequencyHz < lowest.FrequencyHz {
			lowest = mode
		}
		if mode.FrequencyHz > highest.FrequencyHz {
			highest = mode
		}
	}

	frequencyRatio := highest.FrequencyHz / lowest.FrequencyHz
	decayRatio := highest.DecayRatePerSecond / lowest.DecayRatePerSecond
	t.Logf(
		"%.1f Hz T60 %.0f ms -> %.1f Hz T60 %.0f ms (frequency x%.2f, decay x%.2f)",
		lowest.FrequencyHz,
		t60Milliseconds(lowest.DecayRatePerSecond),
		highest.FrequencyHz,
		t60Milliseconds(highest.DecayRatePerSecond),
		frequencyRatio,
		decayRatio,
	)

	// Constant Q would make the two ratios equal; the frequency-independent
	// floor keeps the decay ratio slightly below that.
	if decayRatio < 0.8*frequencyRatio {
		t.Errorf(
			"decay rate grew x%.2f across a x%.2f frequency span, want near-proportional",
			decayRatio,
			frequencyRatio,
		)
	}
}

// TestDefaultFundamentalDecaysFastest covers S2 directly. A two-headed drum
// loses the axisymmetric fundamental fastest, into the cavity and the opposite
// head; the shipped model had it ringing longest of every mode, which is the
// single most audible defect of the voice.
func TestDefaultFundamentalDecaysFastest(t *testing.T) {
	t.Parallel()

	config := DefaultPhysicalDrum()
	for name, head := range map[string]Head{
		"batter":   config.Batter,
		"resonant": config.Resonant,
	} {
		modes, err := generateHeadModes(config, head)
		if err != nil {
			t.Fatal(err)
		}

		fundamental, ok := modeByOrder(modes, 0, 1)
		if !ok {
			t.Fatalf("%s: no (0,1) mode retained", name)
		}

		// Constant Q means modes far above the fundamental legitimately decay
		// faster in absolute terms, so the comparison is against the low band
		// the fundamental competes with. Under the shipped flat law the (0,1)
		// was the slowest mode of the whole bank.
		lowBandCeilingHz := 3 * fundamental.FrequencyHz
		for _, mode := range modes {
			if mode.AzimuthalOrder == 0 && mode.RadialOrder == 1 {
				continue
			}
			if mode.FrequencyHz > lowBandCeilingHz {
				continue
			}
			if mode.DecayRatePerSecond >= fundamental.DecayRatePerSecond {
				t.Errorf(
					"%s: (%d,%d) at %.1f Hz decays at %.2f /s, "+
						"faster than the (0,1) at %.2f /s",
					name,
					mode.AzimuthalOrder,
					mode.RadialOrder,
					mode.FrequencyHz,
					mode.DecayRatePerSecond,
					fundamental.DecayRatePerSecond,
				)

				break
			}
		}

		// A thump, not a boing, and the number the anchor is stated in is the
		// T60, not the ζ. The correction is set to hold the fundamental at about
		// 210 ms, which is the length the drum had at the tuning that sounded
		// right; ζ then follows from wherever the pitch is, and at the retuned
		// 150 Hz default it comes out near 3.4 % rather than the 5 % that the
		// 104 Hz default produced from the same rate.
		//
		// The property this file is about is the one asserted above: the
		// fundamental is the shortest partial in the low band. At 3.4 % against
		// the band's 0.72 % it still is, by a factor of nearly five.
		zeta := fundamental.DecayRatePerSecond / fundamental.AngularFrequency
		t60 := t60Milliseconds(fundamental.DecayRatePerSecond)
		t.Logf("%s (0,1): %.2f Hz, zeta %.4f, T60 %.0f ms",
			name, fundamental.FrequencyHz, zeta, t60)
		if t60 < 150 || t60 > 300 {
			t.Errorf("%s (0,1) T60 = %.0f ms, want near 210", name, t60)
		}
		if zeta < 0.025 || zeta > 0.05 {
			t.Errorf("%s (0,1) zeta = %.4f, want near 0.034", name, zeta)
		}
	}
}

// TestModalDecayLawIsSeparableByTerm keeps the three coefficients individually
// meaningful, so calibration can attribute a change to a cause.
func TestModalDecayLawIsSeparableByTerm(t *testing.T) {
	t.Parallel()

	const wavenumber = 20.0

	head := Head{Loss0PerSecond: 2}
	if got := ModalDecayRatePerSecond(head, wavenumber); got != 2 {
		t.Errorf("d0 only = %v, want 2", got)
	}

	head = Head{Loss1MPerSecond: 0.5}
	if got := ModalDecayRatePerSecond(head, wavenumber); got != 10 {
		t.Errorf("d1 only = %v, want 10", got)
	}

	head = Head{Loss2M2PerSecond: 0.25}
	if got := ModalDecayRatePerSecond(head, wavenumber); got != 100 {
		t.Errorf("d2 only = %v, want 100", got)
	}

	head = Head{Loss0PerSecond: 2, Loss1MPerSecond: 0.5, Loss2M2PerSecond: 0.25}
	if got := ModalDecayRatePerSecond(head, wavenumber); got != 112 {
		t.Errorf("combined = %v, want 112", got)
	}
}

// TestLoss1ProducesConstantQOnAnIdealMembrane checks the calibration identity
// the defaults are derived from: on a membrane where ω = c·k, choosing
// d1 = ζ·c makes ζ exactly frequency-independent.
func TestLoss1ProducesConstantQOnAnIdealMembrane(t *testing.T) {
	t.Parallel()

	config := DefaultPhysicalDrum()
	head := config.Batter
	silenceLosses(&head)

	const zeta = 0.011
	waveSpeed := math.Sqrt(head.TensionNPerM / head.SurfaceDensityKgPerM2)
	head.Loss1MPerSecond = zeta * waveSpeed
	// Bending stiffness lifts ω above c·k, and the asymmetry split moves ω
	// without moving k, so the identity is exact only for the ideal membrane.
	head.BendingStiffnessNM = 0
	head.TensionAsymmetry = TensionAsymmetry{}

	modes, err := generateHeadModes(config, head)
	if err != nil {
		t.Fatal(err)
	}

	for _, mode := range modes {
		got := mode.DecayRatePerSecond / mode.AngularFrequency
		if math.Abs(got-zeta) > 1e-9 {
			t.Fatalf(
				"mode (%d,%d) at %.1f Hz: zeta = %.12f, want %.12f",
				mode.AzimuthalOrder,
				mode.RadialOrder,
				mode.FrequencyHz,
				got,
				zeta,
			)
		}
	}
}

// TestTheLongestRingingModeIsTheLowestUncorrectedOne states the shape of the
// loss law as a property rather than as a description, because that shape is
// PLAN item N3's defect and the item named the wrong mode for it.
//
// γ is monotone in k — d0 + d1k + d2k² plus a radiation term that is small
// everywhere — with exactly one exception, the (0,1) correction. So the mode
// that rings longest is forced: it is the lowest-wavenumber mode that the
// correction table does not name, and today that is the (1,1). Nothing about
// this is a tuning accident, and no value of DAMP or D.TILT moves it, because
// both scale the whole law.
//
// The consequence for N3 is the reason this is pinned. Its instance — "a mode at
// 186 Hz with T60 1.81 s, the longest-ringing thing the model produces" — is that
// (1,1), seen at the fitted config where the law is scaled down by about 2.6x.
// It is not the coupled (0,1) doublet, which is what N3 prescribed damping, and
// which the correction table already damps by more than twice its structural
// rate. Damping the doublet harder cannot move the longest mode. Anything that
// does has to either put a second entry in the correction table or give the law
// a shape it does not have.
func TestTheLongestRingingModeIsTheLowestUncorrectedOne(t *testing.T) {
	t.Parallel()

	config := DefaultPhysicalDrum()

	modes, err := generateHeadModes(config, config.Batter)
	if err != nil {
		t.Fatal(err)
	}

	longest, lowestUncorrected := modes[0], Mode{WavenumberPerM: math.Inf(1)}

	for _, mode := range modes {
		if mode.DecayRatePerSecond < longest.DecayRatePerSecond {
			longest = mode
		}

		if mode.DecayCorrectionPerSecond == 0 && mode.WavenumberPerM < lowestUncorrected.WavenumberPerM {
			lowestUncorrected = mode
		}
	}

	if longest.AzimuthalOrder != lowestUncorrected.AzimuthalOrder ||
		longest.RadialOrder != lowestUncorrected.RadialOrder {
		t.Errorf("longest-ringing mode is (%d,%d) at %.1f Hz but the lowest uncorrected "+
			"mode is (%d,%d) at %.1f Hz: the loss law is no longer monotone in k plus "+
			"one correction, and N3's analysis needs redoing",
			longest.AzimuthalOrder, longest.RadialOrder, longest.FrequencyHz,
			lowestUncorrected.AzimuthalOrder, lowestUncorrected.RadialOrder,
			lowestUncorrected.FrequencyHz)
	}

	if longest.AzimuthalOrder != 1 || longest.RadialOrder != 1 {
		t.Errorf("longest-ringing mode is (%d,%d) at %.1f Hz, want the (1,1)",
			longest.AzimuthalOrder, longest.RadialOrder, longest.FrequencyHz)
	}

	fundamental, ok := modeByOrder(modes, 0, 1)
	if !ok {
		t.Fatal("no (0,1) mode retained")
	}

	// And the doublet N3 prescribed damping is already the most heavily damped
	// mode in the bank, by the correction rather than by the law.
	if fundamental.DecayCorrectionPerSecond <= fundamental.StructuralDecayPerSecond {
		t.Errorf("(0,1) correction %.2f /s against a structural rate of %.2f /s: the "+
			"premise that the fundamental is under-damped is back",
			fundamental.DecayCorrectionPerSecond, fundamental.StructuralDecayPerSecond)
	}

	t.Logf("longest (%d,%d) at %.1f Hz, T60 %.0f ms; (0,1) at %.1f Hz, T60 %.0f ms",
		longest.AzimuthalOrder, longest.RadialOrder, longest.FrequencyHz,
		t60Milliseconds(longest.DecayRatePerSecond),
		fundamental.FrequencyHz, t60Milliseconds(fundamental.DecayRatePerSecond))
}
