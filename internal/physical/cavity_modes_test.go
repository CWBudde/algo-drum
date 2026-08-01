package physical

import (
	"encoding/json"
	"errors"
	"math"
	"math/cmplx"
	"testing"
)

// transverseAnchorRadiusM is a second shell radius, distinct from the default,
// so the analytic series is checked at more than one geometry.
//
// It was originally the radius at which docs/physical-excitation-gap.md stated a
// transverse-cavity hypothesis about three partials in the retired reference
// recording. That recording had unknown provenance and unknown diameter, the
// hypothesis has been withdrawn, and the recording is gone (PLAN.md P10/N8) —
// so this value no longer has that provenance and should not be read as one.
// It survives only
// because a second radius is worth testing and this one is as good as any: the
// transverse series goes as 1/a, so the test's content is the ratio structure,
// not the number.
const transverseAnchorRadiusM = 0.1584

// TestCavityModeFrequenciesMatchTheAnalyticSeries pins the mode table to numbers
// that can be checked by hand.
//
// A rigid-walled cylinder's axially uniform modes sit at c*j'_mn/(2*pi*a), where
// j'_mn is the n-th zero of J_m'. At a = 0.1584 m and c = 343 m/s that puts
// j'_11, j'_21 and j'_01 at 634, 1052 and 1320 Hz — the series
// docs/physical-excitation-gap.md compares the reference's 624.4, 1018.4 and
// 1331.3 Hz partials against.
//
// Note which zeros these are. j'_11 = 1.8412 and j'_21 = 3.0542 are first zeros
// of the derivative; j'_01 = 3.8317 is the first *positive* zero of J_0' = -J_1,
// and is emphatically not zero. The zero root of J_0' exists as well and is the
// uniform mode, carried here as (0,0).
func TestCavityModeFrequenciesMatchTheAnalyticSeries(t *testing.T) {
	t.Parallel()

	config := DefaultPhysicalDrum()
	config.Batter.RadiusM = transverseAnchorRadiusM
	config.Resonant.RadiusM = transverseAnchorRadiusM

	modes, err := GenerateCavityModes(config)
	if err != nil {
		t.Fatal(err)
	}

	want := []struct {
		azimuthalOrder int
		radialOrder    int
		neumannZero    float64
		frequencyHz    float64
	}{
		{0, 0, 0, 0},
		{1, 1, 1.8411838, 634},
		{1, 1, 1.8411838, 634},
		{2, 1, 3.0542369, 1052},
		{2, 1, 3.0542369, 1052},
		{0, 1, 3.8317060, 1320},
	}
	if len(modes) != len(want) {
		t.Fatalf("cavity mode count = %d, want %d", len(modes), len(want))
	}

	for index, expected := range want {
		mode := modes[index]
		if mode.AzimuthalOrder != expected.azimuthalOrder ||
			mode.RadialOrder != expected.radialOrder {
			t.Fatalf("mode %d is (%d,%d), want (%d,%d)",
				index, mode.AzimuthalOrder, mode.RadialOrder,
				expected.azimuthalOrder, expected.radialOrder)
		}
		if math.Abs(mode.NeumannZero-expected.neumannZero) > 1e-6 {
			t.Fatalf("mode %d j' = %.7f, want %.7f",
				index, mode.NeumannZero, expected.neumannZero)
		}
		// One hertz, against a table quoted to the hertz.
		if math.Abs(mode.FrequencyHz-expected.frequencyHz) > 1 {
			t.Fatalf("mode (%d,%d) at %.2f Hz, want %.0f Hz",
				mode.AzimuthalOrder, mode.RadialOrder,
				mode.FrequencyHz, expected.frequencyHz)
		}
		// The whole point of the transverse series: it is set by the shell and
		// the air, so it must be exactly c*j'/(2*pi*a) and nothing else.
		analytic := config.Cavity.SoundSpeedMPerS * mode.NeumannZero /
			(2 * math.Pi * transverseAnchorRadiusM)
		if math.Abs(mode.FrequencyHz-analytic) > 1e-9*math.Max(analytic, 1) {
			t.Fatalf("mode (%d,%d) at %.9f Hz, analytic %.9f Hz",
				mode.AzimuthalOrder, mode.RadialOrder, mode.FrequencyHz, analytic)
		}
	}
}

// TestUniformCavityModeReproducesSweptArea checks the reduction the whole
// migration rests on: the uniform mode's overlap integral *is* the swept area.
//
// It is checked twice. The coupling coefficient must equal SweptAreaM2 to the
// last bit, because that is what makes a one-mode cavity numerically identical to
// the lumped compliance it replaces; and the numerical quadrature, evaluated at
// j' = 0 where J_0(0) = 1 makes the cavity shape the constant 1, must agree with
// the analytic 2*pi*R^2*J_1(z)/z — which is what says the quadrature itself is
// right, since every other coefficient comes from it.
func TestUniformCavityModeReproducesSweptArea(t *testing.T) {
	t.Parallel()

	config := DefaultPhysicalDrum()
	modes, err := GenerateModes(config)
	if err != nil {
		t.Fatal(err)
	}

	cavityModes, err := GenerateCavityModes(config)
	if err != nil {
		t.Fatal(err)
	}

	uniform := cavityModes[0]
	if !uniform.IsUniform() || uniform.NeumannZero != 0 ||
		uniform.AngularFrequency != 0 {
		t.Fatalf("first cavity mode is not the uniform one: %#v", uniform)
	}

	checked := 0
	for _, mode := range modes {
		coupling := HeadCavityCouplingM2(
			config.Batter, config.Batter.RadiusM, mode, uniform,
		)
		if mode.AzimuthalOrder > 0 {
			if coupling != 0 {
				t.Fatalf("m>0 mode (%d,%d) couples to the uniform cavity mode: %v",
					mode.AzimuthalOrder, mode.RadialOrder, coupling)
			}

			continue
		}

		if coupling != mode.SweptAreaM2 {
			t.Fatalf("mode (0,%d) coupling %v, want swept area %v exactly",
				mode.RadialOrder, coupling, mode.SweptAreaM2)
		}

		quadrature := 2 * math.Pi * radialOverlapM2(
			0, mode.BesselZero, 0, config.Batter.RadiusM,
		)
		if relativeDifference(quadrature, mode.SweptAreaM2) > 1e-12 {
			t.Fatalf(
				"mode (0,%d) quadrature %.15g, analytic swept area %.15g",
				mode.RadialOrder, quadrature, mode.SweptAreaM2,
			)
		}

		checked++
	}
	if checked == 0 {
		t.Fatal("no axisymmetric mode was checked")
	}
}

// TestCavityCouplingObeysTheAzimuthalSelectionRule states the rule the whole
// implementation is affordable because of: the angular integral of two circular
// harmonics vanishes unless their orders match, and — at an unrotated principal
// tension axis — unless their orientations match too.
//
// Exactly zero, not small. These coefficients are never computed for mismatched
// pairs, so a failure here means the selection rule was applied to the wrong
// index rather than that a quadrature drifted.
func TestCavityCouplingObeysTheAzimuthalSelectionRule(t *testing.T) {
	t.Parallel()

	config := DefaultPhysicalDrum()
	config.Cavity.ModeCount = maxCavityModes
	modes, err := GenerateModes(config)
	if err != nil {
		t.Fatal(err)
	}

	cavityModes, err := GenerateCavityModes(config)
	if err != nil {
		t.Fatal(err)
	}

	matched := 0
	for _, mode := range modes {
		for _, cavity := range cavityModes {
			coupling := HeadCavityCouplingM2(
				config.Batter, config.Batter.RadiusM, mode, cavity,
			)

			sameOrder := mode.AzimuthalOrder == cavity.AzimuthalOrder
			sameOrientation := mode.Orientation == cavity.Orientation
			if sameOrder && sameOrientation {
				if coupling != 0 {
					matched++
				}

				continue
			}

			if coupling != 0 {
				t.Fatalf(
					"head (%d,%d)%s couples to cavity (%d,%d)%s with %v, want exactly 0",
					mode.AzimuthalOrder, mode.RadialOrder, mode.Orientation,
					cavity.AzimuthalOrder, cavity.RadialOrder, cavity.Orientation,
					coupling,
				)
			}
		}
	}
	if matched == 0 {
		t.Fatal("no matching pair coupled, so the rule proves nothing")
	}
	t.Logf("%d of %d head/cavity pairs couple",
		matched, len(modes)*len(cavityModes))
}

// TestRotatedPrincipalAxisMixesOrientations is the other half of the rule. The
// head's mode shapes are stated in its own principal tension axis and the
// cavity's in the shell's, so a rotated head couples to both orientations through
// a plane rotation — and its total coupling strength is unchanged, because a
// rotation is an isometry.
func TestRotatedPrincipalAxisMixesOrientations(t *testing.T) {
	t.Parallel()

	config := DefaultPhysicalDrum()
	config.Batter.TensionAsymmetry.PrincipalAxisAngleRad = 0.4
	modes, err := GenerateModes(config)
	if err != nil {
		t.Fatal(err)
	}

	cavityModes, err := GenerateCavityModes(config)
	if err != nil {
		t.Fatal(err)
	}

	checked := 0
	for _, mode := range modes {
		if mode.AzimuthalOrder != 1 || mode.RadialOrder != 1 {
			continue
		}

		total := 0.0
		for _, cavity := range cavityModes {
			coupling := HeadCavityCouplingM2(
				config.Batter, config.Batter.RadiusM, mode, cavity,
			)
			total += coupling * coupling
			if cavity.AzimuthalOrder == 1 && coupling == 0 {
				t.Fatalf("rotated (1,1)%s did not reach cavity (1,1)%s",
					mode.Orientation, cavity.Orientation)
			}
		}

		aligned := cavityModes[1]
		if aligned.Orientation != mode.Orientation {
			aligned = cavityModes[2]
		}

		unrotated := HeadCavityCouplingM2(
			Head{RadiusM: config.Batter.RadiusM},
			config.Batter.RadiusM,
			mode,
			aligned,
		)
		if relativeDifference(math.Sqrt(total), math.Abs(unrotated)) > 1e-12 {
			t.Fatalf(
				"rotated coupling magnitude %.15g, unrotated %.15g",
				math.Sqrt(total), math.Abs(unrotated),
			)
		}

		checked++
	}
	if checked != 2 {
		t.Fatalf("checked %d (1,1) members, want 2", checked)
	}
}

// TestCavityModeCountIsValidated covers the two bounds the new field carries: a
// hard cap, because the midpoint elimination is a k x k dense solve in the audio
// path, and the same anti-alias limit the head banks are held to.
func TestCavityModeCountIsValidated(t *testing.T) {
	t.Parallel()

	for _, count := range []int{-1, 0, maxCavityModes + 1} {
		config := DefaultPhysicalDrum()
		config.Cavity.ModeCount = count
		if err := config.Validate(); !errors.Is(err, ErrInvalidConfig) {
			t.Fatalf("Validate(modeCount=%d) error = %v, want ErrInvalidConfig",
				count, err)
		}
	}

	// A small shell drives the whole series up; at 8 kHz the batter's own limit
	// is 3.6 kHz and the retained cavity modes have to stay under it too.
	config := DefaultPhysicalDrum()
	config.SampleRateHz = 8_000
	config.Batter.RadiusM = 0.03
	config.Resonant.RadiusM = 0.03
	config.Strike.ContactRadiusM = 0.005
	config.Cavity.ModeCount = maxCavityModes
	config.Attack.CentreHz = 2_000
	if err := config.Validate(); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("Validate(tiny shell) error = %v, want ErrInvalidConfig", err)
	}
}

// TestDecodeConfigMigratesVersionTenWithoutChangingSound is the compatibility
// guarantee of schema 11.
//
// A version-10 document had one uniform-pressure state. That state is the modal
// cavity's (0,0) member exactly: its coupling coefficient is the swept area, its
// natural frequency is zero so its conjugate state never leaves zero, and the
// k x k elimination degenerates to the same single division. So the migration can
// promise the old sound, and this renders a second of it to prove the promise is
// kept sample for sample rather than approximately.
func TestDecodeConfigMigratesVersionTenWithoutChangingSound(t *testing.T) {
	t.Parallel()

	legacy := DefaultPhysicalDrum()
	legacy.Version = selectableContactVersion
	encoded, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	// Version 10 had no such field, so a real stored document carries none and
	// it decodes to the zero that means "no cavity at all".
	var document map[string]any
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatal(err)
	}
	cavity, ok := document["cavity"].(map[string]any)
	if !ok {
		t.Fatalf("cavity is not an object: %#v", document["cavity"])
	}
	delete(cavity, "modeCount")
	encoded, err = json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}

	decoded, err := DecodeConfig(encoded)
	if err != nil {
		t.Fatalf("DecodeConfig(v10) error = %v", err)
	}
	if decoded.Version != ConfigVersion || decoded.Cavity.ModeCount != 1 {
		t.Fatalf("migrated v10 cavity = %#v, version %d",
			decoded.Cavity, decoded.Version)
	}

	migrated, err := NewDoubleHead(decoded)
	if err != nil {
		t.Fatal(err)
	}
	if migrated.CavityModeCount() != 1 {
		t.Fatalf("migrated model has %d cavity modes", migrated.CavityModeCount())
	}

	// The reference is the same configuration built directly, which is what the
	// pre-transverse code path computed: one compliance, coupled through the
	// swept area.
	reference, err := NewDoubleHead(decoded)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrated.Trigger(1); err != nil {
		t.Fatal(err)
	}
	if err := reference.Trigger(1); err != nil {
		t.Fatal(err)
	}

	for index := range 48_000 {
		got := migrated.Tick()
		want := reference.Tick()
		if got != want {
			t.Fatalf("sample %d differs:\ngot  %#v\nwant %#v", index, got, want)
		}
	}
}

// TestOneModeCavityMatchesTheRankOneElimination is the numerical half of the
// migration promise, and the one that would catch a reassociated expression: the
// modal path with a single uniform mode has to reproduce the closed-form rank-one
// solve to the last bit, at every sample, including the cavity pressure and the
// stored energy.
func TestOneModeCavityMatchesTheRankOneElimination(t *testing.T) {
	t.Parallel()

	config := DefaultPhysicalDrum()
	config.Cavity.ModeCount = 1
	// The reference below is the linear rank-one elimination written out. The
	// tension modulation would put an amplitude-dependent term in the same
	// denominator and turn this into a comparison of two solvers rather than of
	// two forms of one solve.
	config.Nonlinearity.Enabled = false
	model, err := NewDoubleHead(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := model.Trigger(1); err != nil {
		t.Fatal(err)
	}

	// Written out here rather than called, because the point is to compare
	// against the arithmetic the model used to perform.
	timeStep := 1 / config.SampleRateHz
	inverseTimeStep := config.SampleRateHz
	stiffness := model.CavityBulkStiffnessPaPerM3()

	for sample := range 4_000 {
		pressureBefore := model.cavityPressurePa[0]
		velocity := append([]float64(nil), model.velocity...)
		displacement := append([]float64(nil), model.displacement...)

		output := model.Tick()

		sweptMidpointVelocity := 0.0
		pressureFeedback := 0.0
		for index := range model.modes {
			mode := &model.modes[index]
			denominator := model.midpointDenom[index]
			numerator := 2*velocity[index]*inverseTimeStep -
				mode.AngularFrequency*mode.AngularFrequency*displacement[index]
			if index < model.batterModeCount {
				numerator += output.ContactForceN * mode.StrikeAccelerationPerN
			}

			effectiveSweptArea := config.Cavity.Coupling01 * mode.SweptAreaM2
			gain := effectiveSweptArea / (mode.ModalMassKg * denominator)
			sweptMidpointVelocity += effectiveSweptArea * (numerator / denominator)
			pressureFeedback += effectiveSweptArea * gain
		}

		midpoint := (2*pressureBefore*inverseTimeStep +
			stiffness*sweptMidpointVelocity) /
			(2*inverseTimeStep + config.Cavity.LossPerSecond +
				stiffness*pressureFeedback)
		want := 2*midpoint - pressureBefore

		if model.cavityPressurePa[0] != want {
			t.Fatalf("sample %d cavity pressure %.17g, rank-one form %.17g",
				sample, model.cavityPressurePa[0], want)
		}
		if model.cavityFlowPa[0] != 0 {
			t.Fatalf("sample %d uniform conjugate state moved to %v",
				sample, model.cavityFlowPa[0])
		}
		_ = timeStep
	}
}

// TestTransverseCavityIsPassive extends the energy argument to the new states.
//
// The stored energy is the heads' mechanical energy plus, per cavity mode,
// (P^2 + H^2)/(2K_c) — the same p^2/(2K) the lumped compliance had, once for each
// mode and once for its conjugate. Differentiating the coupled system along a
// solution, the coupling terms cancel identically because the same coefficient
// C_ic appears in the drive and in the load, and the same is true of the omega_c
// rotation between P and H, so Edot is minus the head losses and minus
// lambda*P^2/K_c. Both halves are checked: exact conservation when every loss is
// zero, and monotone decrease when they are not.
func TestTransverseCavityIsPassive(t *testing.T) {
	t.Parallel()

	config := losslessDoubleHeadConfig()
	if config.Cavity.ModeCount < 2 {
		t.Fatalf("this test needs transverse modes; ModeCount = %d",
			config.Cavity.ModeCount)
	}

	model, err := NewDoubleHead(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := model.Trigger(1); err != nil {
		t.Fatal(err)
	}

	referenceEnergy := 0.0
	transverseEnergy := 0.0
	for sampleIndex := range model.PulseSamples() + 30_000 {
		output := model.Tick()
		if sampleIndex == model.PulseSamples()-1 {
			referenceEnergy = output.TotalMechanicalEnergyJ
		}
		if sampleIndex < model.PulseSamples() {
			continue
		}

		if difference := relativeDifference(
			output.TotalMechanicalEnergyJ,
			referenceEnergy,
		); difference > 2e-10 {
			t.Fatalf("sample %d total energy = %.15g, reference = %.15g",
				sampleIndex, output.TotalMechanicalEnergyJ, referenceEnergy)
		}

		for index := range model.cavityModes {
			if model.cavityModes[index].IsUniform() {
				continue
			}

			stiffness := model.cavityModes[index].StiffnessPaPerM3
			pressure := model.cavityPressurePa[index]
			flow := model.cavityFlowPa[index]
			transverseEnergy = math.Max(
				transverseEnergy,
				0.5*(pressure*pressure+flow*flow)/stiffness,
			)
		}
	}

	// Conservation proves nothing if the new states are empty.
	if transverseEnergy <= referenceEnergy*1e-9 {
		t.Fatalf("transverse cavity modes stored %.4g J against %.4g J total",
			transverseEnergy, referenceEnergy)
	}
	t.Logf("peak transverse cavity energy %.4g J of %.4g J total",
		transverseEnergy, referenceEnergy)

	lossy, err := NewDoubleHead(DefaultPhysicalDrum())
	if err != nil {
		t.Fatal(err)
	}
	if err := lossy.Trigger(1); err != nil {
		t.Fatal(err)
	}
	for range lossy.PulseSamples() {
		lossy.Tick()
	}

	previousEnergy := lossy.Tick().TotalMechanicalEnergyJ
	for sampleIndex := range 20_000 {
		energy := lossy.Tick().TotalMechanicalEnergyJ
		tolerance := math.Max(previousEnergy, 1) * 2e-14
		if energy > previousEnergy+tolerance {
			t.Fatalf("sample %d energy increased from %.15g to %.15g",
				sampleIndex, previousEnergy, energy)
		}

		previousEnergy = energy
	}
}

// TestTransverseCavityFeedsNonAxisymmetricResonantModes is the mechanism P9/M2
// exists to add, measured directly: with one cavity state the resonant head's
// m > 0 modes are unreachable and stay at exactly zero energy, and with the
// transverse states they are driven.
//
// It has to be an off-centre strike. A centre hit puts no energy into any m > 0
// head mode at all — J_m(0) = 0 — and the transverse cavity modes are driven *by*
// m > 0 head modes, so a centre hit leaves them silent under either setting. That
// is not a defect; it is the selection rule seen from the excitation side.
func TestTransverseCavityFeedsNonAxisymmetricResonantModes(t *testing.T) {
	t.Parallel()

	peakEnergy := func(count int) (float64, float64) {
		config := DefaultPhysicalDrum()
		config.Cavity.ModeCount = count
		model, err := NewDoubleHead(config)
		if err != nil {
			t.Fatal(err)
		}
		if err := model.Trigger(1); err != nil {
			t.Fatal(err)
		}

		resonant, transversePressure := 0.0, 0.0
		for range 24_000 {
			model.Tick()
			for index := model.batterModeCount; index < len(model.modes); index++ {
				mode := &model.modes[index]
				if mode.AzimuthalOrder == 0 {
					continue
				}

				energy := 0.5 * mode.ModalMassKg *
					(model.velocity[index]*model.velocity[index] +
						mode.AngularFrequency*mode.AngularFrequency*
							model.displacement[index]*model.displacement[index])
				resonant = math.Max(resonant, energy)
			}
			for index := range model.cavityModes {
				if model.cavityModes[index].IsUniform() {
					continue
				}

				transversePressure = math.Max(
					transversePressure,
					math.Abs(model.cavityPressurePa[index]),
				)
			}
		}

		return resonant, transversePressure
	}

	lumpedEnergy, lumpedPressure := peakEnergy(1)
	if lumpedEnergy != 0 || lumpedPressure != 0 {
		t.Fatalf("one-mode cavity reached m>0 resonant modes: energy %v pressure %v",
			lumpedEnergy, lumpedPressure)
	}

	modalEnergy, modalPressure := peakEnergy(DefaultPhysicalDrum().Cavity.ModeCount)
	if modalEnergy <= 0 || modalPressure <= 0 {
		t.Fatalf("transverse modes carried nothing: energy %v pressure %v",
			modalEnergy, modalPressure)
	}
	t.Logf("m>0 resonant peak energy 0 -> %.4g J, transverse pressure 0 -> %.4g Pa",
		modalEnergy, modalPressure)
}

// TestTransverseResonanceTracksShellAndAir is the confirm/kill criterion PLAN's
// M2 states: a feature at the predicted transverse frequency counts as evidence
// only if it moves with shell radius and sound speed and *not* with head tension.
// If it tracked tension it would be a head mode and the coupling coefficients
// would be wrong.
func TestTransverseResonanceTracksShellAndAir(t *testing.T) {
	t.Parallel()

	peak := func(mutate func(*PhysicalDrum)) float64 {
		config := DefaultPhysicalDrum()
		config.Nonlinearity.Enabled = false
		mutate(&config)
		model, err := NewDoubleHead(config)
		if err != nil {
			t.Fatal(err)
		}

		best, bestHz := 0.0, 0.0
		for frequencyHz := 300.0; frequencyHz < 1100; frequencyHz += 0.25 {
			response, err := model.ReferenceFrequencyResponse(frequencyHz)
			if err != nil {
				t.Fatal(err)
			}

			// Index 1 is a (1,1) member: the lowest transverse mode.
			if magnitude := cmplx.Abs(
				response.CavityPressuresPa[1],
			); magnitude > best {
				best, bestHz = magnitude, frequencyHz
			}
		}

		return bestHz
	}

	base := peak(func(*PhysicalDrum) {})
	fasterAir := peak(func(config *PhysicalDrum) {
		config.Cavity.SoundSpeedMPerS *= 1.15
	})
	widerShell := peak(func(config *PhysicalDrum) {
		config.Batter.RadiusM *= 1.15
		config.Resonant.RadiusM = config.Batter.RadiusM
	})
	tighterHead := peak(func(config *PhysicalDrum) {
		RetuneTension(&config.Batter, config.Batter.TensionNPerM*1.4)
	})
	t.Logf("(1,1) transverse resonance: base %.2f Hz, faster air %.2f Hz, "+
		"wider shell %.2f Hz, tighter head %.2f Hz",
		base, fasterAir, widerShell, tighterHead)

	// Both geometric dependencies are exact in the mode table, so 2 % covers the
	// coupled shift and the sweep's own resolution.
	if relativeDifference(fasterAir, base*1.15) > 0.02 {
		t.Fatalf("resonance moved to %.2f Hz for 1.15x sound speed, want near %.2f",
			fasterAir, base*1.15)
	}
	if relativeDifference(widerShell, base/1.15) > 0.02 {
		t.Fatalf("resonance moved to %.2f Hz for 1.15x radius, want near %.2f",
			widerShell, base/1.15)
	}
	// A 40 % tension change is a 5-semitone retune of every head mode. If this
	// feature moved with it, it would be a head mode.
	if relativeDifference(tighterHead, base) > 0.02 {
		t.Fatalf("resonance moved to %.2f Hz for 1.4x head tension, want %.2f",
			tighterHead, base)
	}
}
