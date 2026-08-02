package physical

import (
	"math"
	"testing"
)

// hertzianDrum is the default drum with the Hertzian contact selected, at the
// reference recording's 44.1 kHz so the frequencies quoted here line up with
// docs/physical-excitation-gap.md.
func hertzianDrum() PhysicalDrum {
	config := DefaultPhysicalDrum()
	config.SampleRateHz = 44100
	config.Strike.Contact.Model = ContactHertzian

	return config
}

// contactForceSequence renders one strike and returns the contact force the
// modes were actually driven by, sample for sample.
func contactForceSequence(t *testing.T, config PhysicalDrum, velocity01 float64) (
	[]float64, ContactMetrics,
) {
	t.Helper()

	model, err := NewDoubleHead(config)
	if err != nil {
		t.Fatalf("NewDoubleHead: %v", err)
	}

	if err := model.Trigger(velocity01); err != nil {
		t.Fatalf("Trigger: %v", err)
	}

	forces := make([]float64, model.PulseSamples())
	for index := range forces {
		forces[index] = model.Tick().ContactForceN
	}

	return forces, model.LastContact()
}

// TestPrescribedContactForceHasAZeroComb is the mechanism behind the 476-700 Hz
// gap in docs/physical-excitation-gap.md, stated as the property that produces
// it rather than as the band levels it produces.
//
// A half-sine of width tau has magnitude spectrum |cos(pi*f*tau)|/|1-(2*f*tau)^2|,
// whose numerator vanishes at every (k+1/2)/tau. Those are exact zeros, not a
// roll-off: at the fitted 8.23 ms they fall every 121.5 Hz, so the gap contains
// two of them and the octave above contains three more. No mode inside one can
// be excited at all, whatever the mode count, the microphone or the loss law —
// which is why the earlier search for the gap in those three places found
// nothing.
//
// It is also why shortening the pulse to the measured force duration made things
// 14 dB worse rather than better: it does not remove the comb, it slides it.
func TestPrescribedContactForceHasAZeroComb(t *testing.T) {
	t.Parallel()

	const (
		sampleRateHz = 44100.0
		fundamental  = 118.0
		// The fitted bank's contact, 8.23 ms.
		sampleCount = 363
	)

	pulse := make([]float64, sampleCount)
	addContactPulse(pulse, 0, sampleCount, 1)

	total := 0.0
	for _, sample := range pulse {
		total += sample
	}

	if math.Abs(total-1) > 1e-9 {
		t.Errorf("normalized pulse sums to %v, want 1", total)
	}

	tau := float64(sampleCount) / sampleRateHz
	for order := 4; order <= 6; order++ {
		zeroHz := (float64(order) + 0.5) / tau
		level := contactPulseLevelDB(pulse, sampleRateHz, zeroHz, fundamental)

		t.Logf("predicted zero %d at %.0f Hz: %.1f dB relative to %.0f Hz",
			order, zeroHz, level, fundamental)

		if level > -40 {
			t.Errorf(
				"prescribed force at its own %d-th zero (%.0f Hz) is %.1f dB "+
					"relative to the fundamental, want at most -40 — the comb "+
					"docs/physical-excitation-gap.md attributes the gap to is not "+
					"there, so the document is wrong",
				order, zeroHz, level,
			)
		}
	}
}

// TestPrescribedContactExcitationBandwidth pins the excitation bandwidth of the
// prescribed contact force.
//
// The 5.5-8 ms this law prescribes is well supported by measurement, but as a
// contact *dwell* time rather than a force-pulse width — Wagner's stick leaves
// the head at about 3.5 ms and is touched again by the wave returning off the
// rim. What this test guards is the consequence of spending that dwell as one
// smooth half-sine: on top of the comb above, the envelope falls as 1/f^2, so
// the force is around 30 dB down through the band where a recorded tom carries
// much of its character, and no mode count, microphone geometry or loss law
// downstream can recover what was never injected.
//
// It is a characterization test, not an assertion that this is right.
// ContactHertzian makes these numbers move; see the test below.
func TestPrescribedContactExcitationBandwidth(t *testing.T) {
	t.Parallel()

	const (
		sampleRateHz = 44100.0
		fundamental  = 118.0
		sampleCount  = 363
	)

	pulse := make([]float64, sampleCount)
	addContactPulse(pulse, 0, sampleCount, 1)

	cases := []struct {
		frequencyHz float64
		atMostDB    float64
	}{
		{504, -20},
		{635, -25},
		{800, -35},
	}

	for _, testCase := range cases {
		level := contactPulseLevelDB(
			pulse, sampleRateHz, testCase.frequencyHz, fundamental,
		)

		t.Logf("contact force at %.0f Hz: %.1f dB relative to %.0f Hz",
			testCase.frequencyHz, level, fundamental)

		if level > testCase.atMostDB {
			t.Errorf(
				"contact force at %.0f Hz is %.1f dB relative to the fundamental, "+
					"above the pinned %.0f dB — the excitation reaches further than "+
					"docs/physical-excitation-gap.md records, so update it",
				testCase.frequencyHz, level, testCase.atMostDB,
			)
		}
	}
}

// TestHertzianContactShallowsAndMovesTheComb states what the model does to the
// comb, which is less than "removes it" and worth being exact about.
//
// A comb is not a property of the half-sine. It is a property of any *single
// smooth touch*: one lobe of duration tau puts near-zeros at roughly
// (k+1/2)/tau whatever its shape, and the Hertzian contact is still one lobe.
// What changes is that they stop being exact. The half-sine's zeros are analytic
// zeros of cos(pi*f*tau) and go to numerical nothing; the Hertzian pulse is
// asymmetric, so the same interference leaves a finite dip — 51 dB at its worst
// here against 309 dB — and the dips sit at the new duration's spacing rather
// than the old one's.
//
// The honest reading is that this halves the problem rather than solving it. A
// 51 dB hole is still a hole. Removing it needs structure *inside* the contact,
// which is what Wagner measured and what this model does not reproduce; see
// docs/physical-contact.md.
func TestHertzianContactShallowsAndMovesTheComb(t *testing.T) {
	t.Parallel()

	forces, _ := contactForceSequence(t, hertzianDrum(), 1)

	prescribed := make([]float64, 363)
	addContactPulse(prescribed, 0, len(prescribed), 1)

	worstHz, worstDB := 0.0, 0.0
	for frequencyHz := 150.0; frequencyHz <= 1000; frequencyHz += 2.5 {
		level := contactPulseLevelDB(forces, 44100, frequencyHz, 118)
		if level < worstDB {
			worstHz, worstDB = frequencyHz, level
		}
	}

	t.Logf("deepest Hertzian notch below 1 kHz: %.1f dB at %.0f Hz",
		worstDB, worstHz)

	// Finite, not analytic. The bound is far above any dip a smooth asymmetric
	// pulse produces and far below the prescribed model's zeros, so it fails only
	// if the release has acquired a discontinuity — which is what an inadmissible
	// hysteresis coefficient does, and how that bound was found.
	if worstDB < -60 {
		t.Errorf(
			"Hertzian contact force dips to %.1f dB at %.0f Hz, below the -60 dB "+
				"that separates a smooth pulse's interference dip from a true zero — "+
				"something has put a step into the release",
			worstDB, worstHz,
		)
	}

	// And the prescribed model's own zeros are no longer zeros, which is the part
	// that matters for the gap: 547 and 668 Hz are inside it.
	tau := float64(len(prescribed)) / 44100
	for order := 4; order <= 6; order++ {
		zeroHz := (float64(order) + 0.5) / tau
		hertzian := contactPulseLevelDB(forces, 44100, zeroHz, 118)
		half := contactPulseLevelDB(prescribed, 44100, zeroHz, 118)

		t.Logf("%.0f Hz: prescribed %.1f dB, Hertzian %.1f dB", zeroHz, half, hertzian)

		if hertzian-half < 100 {
			t.Errorf(
				"at the prescribed model's %.0f Hz zero the Hertzian force is only "+
					"%.1f dB above it, so the comb has not moved",
				zeroHz, hertzian-half,
			)
		}
	}
}

// TestHertzianContactDurationIsPredictedNotPrescribed checks the number the
// model is not told.
//
// Nothing in the Hertzian path carries a contact time: it has a tip stiffness,
// an exponent, a mallet mass and a striking speed, and the duration falls out of
// integrating them against the head. Landing inside the 5.5-8 ms that Dahl 1997
// and Wagner 2006 measure is therefore a result, and the single strongest piece
// of evidence that the mechanism is right even where the model's numbers are not.
//
// One touch, not three. Wagner measured a stick that separates at 3.5 ms and is
// caught twice more by the wave returning off the rim, and this model does not
// reproduce that — see docs/physical-contact.md, where the re-contacts it
// appeared to produce turned out to be a discretization artifact that converged
// away. The count is asserted so that a future change which genuinely produces
// them has to come and edit this line.
func TestHertzianContactDurationIsPredictedNotPrescribed(t *testing.T) {
	t.Parallel()

	_, metrics := contactForceSequence(t, hertzianDrum(), 1)

	t.Logf(
		"predicted contact: first lobe %.2f ms, dwell %.2f ms, %d touch(es), "+
			"peak %.1f N, impulse %.4f Ns",
		metrics.FirstLobeSeconds*1e3, metrics.DwellSeconds*1e3,
		metrics.TouchCount, metrics.PeakForceN, metrics.ImpulseNS,
	)

	dwellMs := metrics.DwellSeconds * 1e3
	if dwellMs < 5.5 || dwellMs > 8 {
		t.Errorf(
			"predicted contact dwell = %.2f ms, outside the 5.5-8 ms Dahl 1997 and "+
				"Wagner 2006 measure for a struck tom",
			dwellMs,
		)
	}

	if metrics.TouchCount != 1 {
		t.Errorf(
			"predicted touch count = %d, want 1 — docs/physical-contact.md records "+
				"that this model produces a single smooth contact and that anything "+
				"else was numerical, so update it before changing this",
			metrics.TouchCount,
		)
	}
}

// TestHertzianContactIsSubstepConverged guards the artifact that nearly got
// written up as a result.
//
// At the substep count the model shipped with before this test, the contact
// broke into seven separate impacts and put 17 dB more into the 1.5 kHz band
// than the converged answer. It looked like Wagner's re-contacts, which is
// exactly what made it dangerous. Refining the step must not move the spectrum.
func TestHertzianContactIsSubstepConverged(t *testing.T) {
	t.Parallel()

	config := hertzianDrum()
	reference := contactSpectrumAtSubsteps(t, config, contactMaxSubsteps)
	shipped := contactSpectrumAtSubsteps(t, config, 0)

	for index, frequencyHz := range contactProbeHz {
		difference := math.Abs(shipped[index] - reference[index])

		t.Logf("%.0f Hz: shipped %.1f dB, 64-substep %.1f dB, delta %.1f",
			frequencyHz, shipped[index], reference[index], difference)

		if difference > 1.5 {
			t.Errorf(
				"contact spectrum at %.0f Hz moves %.1f dB between the shipped "+
					"substep count and %d substeps, above the 1.5 dB bound — the "+
					"contact is not resolved, and an unresolved contact chatters in a "+
					"way that mimics a real re-contact",
				frequencyHz, difference, contactMaxSubsteps,
			)
		}
	}
}

var contactProbeHz = [...]float64{504, 635, 800, 1500}

func contactSpectrumAtSubsteps(
	t *testing.T, config PhysicalDrum, substeps int,
) [len(contactProbeHz)]float64 {
	t.Helper()

	model, err := NewDoubleHead(config)
	if err != nil {
		t.Fatalf("NewDoubleHead: %v", err)
	}

	if substeps > 0 {
		model.contact.substeps = substeps
	}

	if err := model.Trigger(1); err != nil {
		t.Fatalf("Trigger: %v", err)
	}

	forces := make([]float64, model.PulseSamples())
	for index := range forces {
		forces[index] = model.Tick().ContactForceN
	}

	var levels [len(contactProbeHz)]float64
	for index, frequencyHz := range contactProbeHz {
		levels[index] = contactPulseLevelDB(
			forces, config.SampleRateHz, frequencyHz, 118,
		)
	}

	return levels
}

// TestHertzianContactCannotAddEnergy checks the coupling in the direction a bug
// would break it.
//
// The contact projects force onto the modes through StrikeAccelerationPerN and
// reads the head back through the same weight times the modal mass. If those two
// ever disagree, force times head velocity stops being the power delivered and
// the contact becomes a source: energy appears from nowhere, quietly, and only
// at high velocities. The mallet's kinetic energy is the ceiling on everything
// the head can end up holding.
func TestHertzianContactCannotAddEnergy(t *testing.T) {
	t.Parallel()

	config := hertzianDrum()
	model, err := NewDoubleHead(config)
	if err != nil {
		t.Fatal(err)
	}

	if err := model.Trigger(1); err != nil {
		t.Fatal(err)
	}

	kineticJ := 0.5 * config.Strike.MalletMassKg *
		config.Strike.VelocityMPerS * config.Strike.VelocityMPerS
	peakJ := 0.0

	for range int(config.SampleRateHz * 0.05) {
		peakJ = max(peakJ, model.Tick().TotalMechanicalEnergyJ)
	}

	t.Logf("mallet kinetic energy %.4g J, peak stored %.4g J (%.1f %%)",
		kineticJ, peakJ, 100*peakJ/kineticJ)

	if peakJ > kineticJ {
		t.Errorf(
			"head stored %.4g J from a %.4g J strike — the contact is injecting "+
				"energy, which means the force projection and the head readback "+
				"disagree",
			peakJ, kineticJ,
		)
	}
}

// TestHertzianContactReachesPastTheModalCeiling is the payoff, measured on the
// instrument rather than on the force, and it is the whole of
// docs/physical-contact.md's "What it does buy" table rather than three rows of
// it, so that the table cannot go stale without this failing.
//
// The attack layer is disabled so this sees only what the excitation actually
// puts into the modes. Above 800 Hz the prescribed contact is in the tail of its
// comb and the modes there are effectively unexcited, which is the seam the
// fitted preset had to paper over by dragging ATK.T down to 1644 Hz.
//
// # Method
//
// Spelled out because the table's first version was made by a program that was
// never committed, and re-deriving it from the numbers alone failed: every
// plausible-looking alternative estimator gives a different answer, one of them
// with the opposite sign.
//
//   - One strike at velocity 1 into DefaultPhysicalDrum() at 44.1 kHz.
//     Strike.Contact.Model is the only difference between the two columns and
//     Nonlinearity.Coupling.Enabled the only difference between the two halves.
//   - "Modal only" means Attack.Enabled = false and nothing else. The cavity,
//     the Berger tension term and the mode coupling are all left as shipped.
//   - The level at f is a single-bin DFT of the entire one-second render at
//     exactly f, rectangular window. The window is the load-bearing choice: the
//     Hertzian advantage lives in the first few milliseconds, and any taper that
//     vanishes at sample 0 destroys it — a Hann window over one second is 60 dB
//     down at 7 ms, and measuring through one reverses the sign of this table.
//   - Levels are referred to contactReferenceHz; see the note there.
//
// The numbers are deterministic: with the attack layer off nothing in this path
// is stochastic, so the tolerance is for model changes, not for run-to-run
// spread.
func TestHertzianContactReachesPastTheModalCeiling(t *testing.T) {
	t.Parallel()

	// The recorded table. Prescribed and Hertzian are levels relative to
	// contactReferenceHz; the delta is what docs/physical-contact.md quotes.
	//
	// The coupled half is the shipped configuration. The uncoupled half is the
	// control, and it is what the pre-P9 table was measured under: the nonlinear
	// mode coupling deposits energy at 2f_a +/- f_b regardless of what |F(f)|
	// does there, so it fills part of the very band the half-sine's zero comb
	// had deleted, and it does that on the *prescribed* side. That is why the
	// 800 Hz advantage falls from 17.5 dB to 13.3 dB while 1500 and 2500 Hz
	// barely move — the coupling doing what P9/M1 predicted, not the contact
	// model losing ground.
	cases := []struct {
		frequencyHz           float64
		prescribedDB          float64
		hertzianDB            float64
		uncoupledPrescribedDB float64
		uncoupledHertzianDB   float64
	}{
		{400, -28.7, -14.6, -35.8, -27.5},
		{504, -27.7, -22.2, -39.7, -34.2},
		{635, -31.7, -26.4, -38.7, -26.1},
		{800, -40.8, -27.6, -50.2, -32.7},
		{1500, -61.8, -40.9, -62.0, -41.2},
		{2500, -70.8, -42.5, -71.2, -42.6},
		{4000, -76.8, -51.2, -77.2, -51.0},
	}

	frequencies := make([]float64, len(cases))
	for index, testCase := range cases {
		frequencies[index] = testCase.frequencyHz
	}

	for _, coupled := range []bool{true, false} {
		config := hertzianDrum()
		config.Attack.Enabled = false
		config.Nonlinearity.Coupling.Enabled = coupled

		prescribed := config
		prescribed.Strike.Contact.Model = ContactPrescribed

		hertzianLevels := renderedBandDB(t, config, frequencies)
		prescribedLevels := renderedBandDB(t, prescribed, frequencies)

		for index, testCase := range cases {
			wantPrescribed := testCase.prescribedDB
			wantHertzian := testCase.hertzianDB

			if !coupled {
				wantPrescribed = testCase.uncoupledPrescribedDB
				wantHertzian = testCase.uncoupledHertzianDB
			}

			t.Logf(
				"coupling=%v %.0f Hz: prescribed %.1f dB, Hertzian %.1f dB, delta %.1f",
				coupled, testCase.frequencyHz,
				prescribedLevels[index], hertzianLevels[index],
				hertzianLevels[index]-prescribedLevels[index],
			)

			for _, measurement := range []struct {
				name string
				got  float64
				want float64
			}{
				{"prescribed", prescribedLevels[index], wantPrescribed},
				{"Hertzian", hertzianLevels[index], wantHertzian},
			} {
				if math.Abs(measurement.got-measurement.want) > contactTableToleranceDB {
					t.Errorf(
						"coupling=%v, %s contact at %.0f Hz is %.1f dB relative to "+
							"%.0f Hz, against the %.1f dB recorded in "+
							"docs/physical-contact.md — re-derive that table's "+
							"\"What it does buy\" section rather than widening this",
						coupled, measurement.name, testCase.frequencyHz,
						measurement.got, contactReferenceHz(t, config),
						measurement.want,
					)
				}
			}
		}
	}
}

// contactTableToleranceDB is how far the measured table may drift from the one
// in docs/physical-contact.md before the document is considered stale. It is
// wide enough to absorb a compiler's floating-point liberties and narrow enough
// that any real change to the excitation trips it.
const contactTableToleranceDB = 1.5

// TestContactReferenceIsARenderedPartial checks the thing the retired 118 Hz
// constant could not: that the bin every table in docs/physical-contact.md is
// normalized by lands on the render's own fundamental rather than on some other
// partial's leakage skirt.
//
// It does not land on it exactly, and that is the measured result rather than a
// tolerance being nursed. contactReferenceHz returns the *uncoupled batter*
// (0,1) mode at 150.10 Hz; the render's fundamental is the lower branch of the
// cavity-coupled pair, and the enclosed air stiffens it to 154 Hz — **+3.9 Hz,
// 44 cents**. Attributed by elimination on this bank: disabling the Berger term
// leaves it at 154, disabling the mode coupling leaves it at 154, and disabling
// the resonant head moves it to 160 Hz, so it is the cavity load and nothing
// else.
//
// The reference is left config-derived anyway, because deriving it from the
// render instead would make it differ between the prescribed and the Hertzian
// column — and the two columns being normalized separately is exactly the defect
// this constant was re-pointed to fix. The cost of the 3.9 Hz offset is that the
// bin reads the partial's flank rather than its peak: 2.1 dB below it on the
// prescribed render and 1.3 dB on the Hertzian, so 0.8 dB of the Δ columns is
// this offset. Against a 13.6 dB skirt under the old 118 Hz that is the
// improvement, not a new problem.
//
// The gate is 6 Hz: enough for the measured cavity shift, not enough for a
// different partial (the next is (1,1) at 238.7 Hz). The band is deliberately
// 100-200 Hz and not the whole low end — the fundamental is *not* the strongest
// partial of this render, (2,1) at 320 Hz is on the prescribed side and (1,1) at
// 238 Hz on the Hertzian, so a wider band finds those instead.
func TestContactReferenceIsARenderedPartial(t *testing.T) {
	t.Parallel()

	const maximumOffsetHz = 6.0

	for _, contact := range []ContactModel{ContactPrescribed, ContactHertzian} {
		config := hertzianDrum()
		config.Attack.Enabled = false
		config.Strike.Contact.Model = contact

		model, err := NewDoubleHead(config)
		if err != nil {
			t.Fatalf("NewDoubleHead: %v", err)
		}

		if err := model.Trigger(1); err != nil {
			t.Fatalf("Trigger: %v", err)
		}

		rendered := make([]float64, int(config.SampleRateHz))
		model.Render(rendered)

		reference := contactReferenceHz(t, config)
		peak := strongestSpectralPeakHz(t, rendered, config.SampleRateHz, 100, 200)

		t.Logf(
			"contact=%s: reference (0,1) mode %.2f Hz, rendered fundamental "+
				"%.2f Hz, cavity shift %+.2f Hz (%+.0f cents)",
			contact, reference, peak, peak-reference,
			1200*math.Log2(peak/reference),
		)

		if math.Abs(peak-reference) > maximumOffsetHz {
			t.Errorf(
				"contact=%s: the levels in docs/physical-contact.md are referred "+
					"to the batter (0,1) mode at %.2f Hz, but the render's "+
					"fundamental is at %.2f Hz — more than the %.0f Hz the "+
					"cavity load accounts for, so the reference bin is no longer "+
					"reading that partial. Re-derive the tables and this offset "+
					"rather than widening the gate",
				contact, reference, peak, maximumOffsetHz,
			)
		}
	}
}

// contactReferenceHz is what the rendered levels are divided by: the (0,1) mode
// of the *configuration under test*, read out of the bank the render actually
// uses.
//
// Until 2026-08-02 this was a hard-coded 118.0 — the fundamental of
// reference/tom.wav, a recording deleted on 2026-08-01 (PLAN.md §"P10" item N8)
// and never this bank's fundamental in the first place. At 118 Hz the
// rectangular-window DFT read the leakage skirt of the 150 Hz partial, so it
// normalized overall level by accident rather than referring anything to a
// fundamental, and it could no longer be checked against anything.
//
// Deriving it here rather than writing 150.08 keeps it tied to the config: the
// fundamental is a consequence of TensionNPerM, RadiusM and SurfaceDensityKgPerM2
// (config.go's B.TUNE spans 300-3500 N/m, i.e. 75-251 Hz), so a hard-coded
// frequency would silently stop being the fundamental the moment any of those
// moved.
//
// generateHeadModes is used in preference to the shorter
// NaturalFrequencyHz(head, BesselZero(0, 1)) because it returns the mode as
// *retained*: same dispersion relation, but reached through the same selection
// and mode-building path the renderer walks, so a bank that stopped containing
// (0,1) fails here loudly instead of being normalized by a frequency it does not
// carry.
func contactReferenceHz(t *testing.T, config PhysicalDrum) float64 {
	t.Helper()

	modes, err := generateHeadModes(config, config.Batter)
	if err != nil {
		t.Fatalf("generateHeadModes: %v", err)
	}

	mode, ok := modeByOrder(modes, 0, 1)
	if !ok {
		t.Fatal("the batter bank has no (0,1) mode to refer levels to")
	}

	return mode.FrequencyHz
}

// renderedBandDB is one strike's level at each of several frequencies, referred
// to contactReferenceHz so the comparison survives a change of output gain.
//
// One render serves every frequency: the strike is deterministic, and the modal
// bank is expensive enough that re-rendering per frequency dominated the test.
//
// The reference bin is taken from *this* render, not shared between renders, so
// a Δ between two columns of the tables in docs/physical-contact.md does not
// cancel the normalization — each column carries its own. That is why moving the
// reference frequency moves the Δ columns too.
func renderedBandDB(
	t *testing.T, config PhysicalDrum, frequenciesHz []float64,
) []float64 {
	t.Helper()

	model, err := NewDoubleHead(config)
	if err != nil {
		t.Fatalf("NewDoubleHead: %v", err)
	}

	if err := model.Trigger(1); err != nil {
		t.Fatalf("Trigger: %v", err)
	}

	rendered := make([]float64, int(config.SampleRateHz))
	model.Render(rendered)

	level := func(hz float64) float64 {
		var real, imaginary float64

		for index, sample := range rendered {
			angle := 2 * math.Pi * hz * float64(index) / config.SampleRateHz
			real += sample * math.Cos(angle)
			imaginary -= sample * math.Sin(angle)
		}

		return 20 * math.Log10(math.Hypot(real, imaginary)+1e-30)
	}

	reference := level(contactReferenceHz(t, config))

	levels := make([]float64, len(frequenciesHz))
	for index, frequencyHz := range frequenciesHz {
		levels[index] = level(frequencyHz) - reference
	}

	return levels
}

// TestContactModelsShareNoState checks that a Reconfigure between the two models
// leaves nothing of the other behind, since they hold their pending force in
// entirely different places.
func TestContactModelsShareNoState(t *testing.T) {
	t.Parallel()

	model, err := NewDoubleHead(hertzianDrum())
	if err != nil {
		t.Fatal(err)
	}

	if err := model.Trigger(1); err != nil {
		t.Fatal(err)
	}

	for range 32 {
		model.Tick()
	}

	prescribed := hertzianDrum()
	prescribed.Strike.Contact.Model = ContactPrescribed
	if err := model.Reconfigure(prescribed); err != nil {
		t.Fatal(err)
	}

	if model.contact.isActive() {
		t.Error("a contact survived Reconfigure onto the other model")
	}

	if force := model.Tick().ContactForceN; force != 0 {
		t.Errorf("force after Reconfigure = %v, want 0", force)
	}
}

// TestDefaultContactIsPrescribed states the shipped choice as a decision rather
// than leaving it implicit. Switching it changes how the instrument sounds and
// needs the re-fit docs/physical-contact.md describes.
func TestDefaultContactIsPrescribed(t *testing.T) {
	t.Parallel()

	if model := DefaultPhysicalDrum().Strike.Contact.Model; model != ContactPrescribed {
		t.Errorf("default contact model = %q, want %q", model, ContactPrescribed)
	}
}

// contactPulseLevelDB is a force sequence's magnitude spectrum at one frequency,
// in dB relative to its own level at referenceHz. Evaluated straight from the
// sample buffer the modes are driven by, so it measures the excitation rather
// than an idealization of it.
func contactPulseLevelDB(pulse []float64, sampleRateHz, frequencyHz, referenceHz float64) float64 {
	magnitude := func(hz float64) float64 {
		var real, imaginary float64

		for index, sample := range pulse {
			angle := 2 * math.Pi * hz * float64(index) / sampleRateHz
			real += sample * math.Cos(angle)
			imaginary -= sample * math.Sin(angle)
		}

		return math.Hypot(real, imaginary)
	}

	return 20 * math.Log10(magnitude(frequencyHz)/magnitude(referenceHz))
}

// TestHertzianContactTimeIsSetByTheHeadNotTheTip is the finding this model
// exists to have produced, and the one that decides how much of the excitation
// problem it can solve.
//
// A drumstick tip is stiff and a drumhead is not. The head's driving-point mass
// under a 1 cm patch is about 0.3 g against a 15 g stick, so the stick is
// essentially unimpeded by the tip's compression and instead rides the head down
// and back up. The closed-form time for the same stick rebounding off a *rigid*
// Hertzian spring is a few tenths of a millisecond; the contact actually lasts
// twenty times that, and stays there while the stiffness moves by a factor of
// thirty.
//
// Two consequences follow, and both are load-bearing for
// docs/physical-contact.md. The contact duration is a prediction the model
// cannot be tuned out of, which is why it agreeing with Dahl and Wagner counts
// for something. And the tip is not the lever: the excitation bandwidth is set
// by a duration that HARD barely reaches, so the hardness control loses most of
// its authority under this model.
func TestHertzianContactTimeIsSetByTheHeadNotTheTip(t *testing.T) {
	t.Parallel()

	config := hertzianDrum()
	model, err := NewDoubleHead(config)
	if err != nil {
		t.Fatal(err)
	}

	rigidSeconds := hertzContactSeconds(
		hertzStiffnessNPerMAlpha(config.Strike),
		config.Strike.Contact.Exponent,
		reducedMassKg(
			config.Strike.MalletMassKg,
			strikePointMassKg(model.modes[:model.batterModeCount]),
		),
		config.Strike.VelocityMPerS,
	)

	_, metrics := contactForceSequence(t, config, 1)
	ratio := metrics.DwellSeconds / rigidSeconds

	t.Logf("rigid-target contact %.3f ms, coupled contact %.3f ms, ratio %.1f",
		rigidSeconds*1e3, metrics.DwellSeconds*1e3, ratio)

	if ratio < 10 {
		t.Errorf(
			"coupled contact is only %.1f times the rigid-target time — the head "+
				"has stopped dominating the contact, which changes what "+
				"docs/physical-contact.md concludes about the hardness control",
			ratio,
		)
	}

	soft, hard := config, config
	soft.Strike.Contact.StiffnessNPerMAlpha /= 30
	hard.Strike.Contact.StiffnessNPerMAlpha *= 30

	_, softMetrics := contactForceSequence(t, soft, 1)
	_, hardMetrics := contactForceSequence(t, hard, 1)
	span := softMetrics.DwellSeconds / hardMetrics.DwellSeconds

	t.Logf("stiffness / 30: %.3f ms, stiffness x 30: %.3f ms, span %.2f",
		softMetrics.DwellSeconds*1e3, hardMetrics.DwellSeconds*1e3, span)

	if span > 1.6 {
		t.Errorf(
			"a 900-fold stiffness range spans %.2f in contact duration, more than "+
				"the 1.6 recorded — the tip has regained authority over the contact "+
				"time and docs/physical-contact.md needs remeasuring",
			span,
		)
	}
}
