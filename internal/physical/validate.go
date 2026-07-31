// Validation of the physical-drum configuration.
//
// Split out of config.go, which had grown to hold the struct tree, the shipped
// defaults, the migration chain and this. Validation is the natural unit to
// separate: it is the same division internal/drum draws between params.go and
// validate.go, and it reads top-down as one contract — Validate names every
// persisted field in order and the helpers below it do the work.

package physical

import (
	"fmt"
	"math"
)

// Validate checks every persisted field.
func (d PhysicalDrum) Validate() error {
	if d.Version != ConfigVersion {
		return fmt.Errorf("%w: got %d, want %d", ErrConfigVersion, d.Version, ConfigVersion)
	}

	if err := finiteRange("sampleRateHz", d.SampleRateHz, minSampleRateHz, maxSampleRateHz); err != nil {
		return err
	}

	if d.Quality.ModeLimit() == 0 {
		return fmt.Errorf("%w: unknown quality %q", ErrInvalidConfig, d.Quality)
	}

	// The floor is one oscillator rather than zero: a reduced head with an empty
	// bank is a disabled head, and Head.Enabled already says that. The ceiling is
	// the largest bank the reduction can select out of, above which the cap is
	// inert.
	if d.ResonantModeLimit < 1 || d.ResonantModeLimit > maxResonantModeLimit {
		return fmt.Errorf(
			"%w: resonantModeLimit=%d outside [1,%d]",
			ErrInvalidConfig,
			d.ResonantModeLimit,
			maxResonantModeLimit,
		)
	}

	if err := validateHead("batter", d.Batter, true); err != nil {
		return err
	}

	if err := validateHead("resonant", d.Resonant, false); err != nil {
		return err
	}

	if err := finiteRange("strike.radius01", d.Strike.Radius01, 0, 1); err != nil {
		return err
	}

	if err := finiteRange("strike.angleRad", d.Strike.AngleRad, -2*math.Pi, 2*math.Pi); err != nil {
		return err
	}

	if err := finiteRange("strike.contactRadiusM", d.Strike.ContactRadiusM, 1e-4, d.Batter.RadiusM/2); err != nil {
		return err
	}

	if err := finiteRange("strike.malletMassKg", d.Strike.MalletMassKg, 1e-4, 1); err != nil {
		return err
	}

	if err := finiteRange("strike.velocityMPerS", d.Strike.VelocityMPerS, 0, 20); err != nil {
		return err
	}

	if err := finiteRange("strike.hardness01", d.Strike.Hardness01, 0, 1); err != nil {
		return err
	}

	if err := validateContact(d.Strike.Contact); err != nil {
		return err
	}

	if err := finiteRange("cavity.depthM", d.Cavity.DepthM, 0.01, 2); err != nil {
		return err
	}

	if err := finiteRange("cavity.coupling01", d.Cavity.Coupling01, 0, 1); err != nil {
		return err
	}

	if err := finiteRange("cavity.stiffnessScale", d.Cavity.StiffnessScale, 0, 1); err != nil {
		return err
	}

	if err := finiteRange("cavity.airDensityKgPerM3", d.Cavity.AirDensityKgPerM3, 0.5, 2); err != nil {
		return err
	}

	if err := finiteRange("cavity.soundSpeedMPerS", d.Cavity.SoundSpeedMPerS, 250, 400); err != nil {
		return err
	}

	if err := finiteRange("cavity.lossPerSecond", d.Cavity.LossPerSecond, 0, 10_000); err != nil {
		return err
	}

	if err := validateCavityModes(d); err != nil {
		return err
	}

	if err := validateNonlinearity(d); err != nil {
		return err
	}

	if err := finiteRange("pickup.radius01", d.Pickup.Radius01, 0, 1); err != nil {
		return err
	}

	if err := finiteRange("pickup.angleRad", d.Pickup.AngleRad, -2*math.Pi, 2*math.Pi); err != nil {
		return err
	}

	if err := validateAttack(d); err != nil {
		return err
	}

	if err := finiteRange("pickup.nearFieldScale", d.Pickup.NearFieldScale, 0, 10); err != nil {
		return err
	}

	if err := finiteRange("pickup.distanceM", d.Pickup.DistanceM, 0.01, 10); err != nil {
		return err
	}

	if err := finiteRange("pickup.highpassHz", d.Pickup.HighpassHz, 1, maxSampleRateHz/2); err != nil {
		return err
	}

	if err := finiteRange("pickup.lowpassHz", d.Pickup.LowpassHz, d.Pickup.HighpassHz, maxSampleRateHz/2); err != nil {
		return err
	}

	if err := finiteRange("pickup.outputGain", d.Pickup.OutputGain, 0, 100); err != nil {
		return err
	}

	// Last, because the alias bound is checked against the generated batter bank
	// and mode generation reads the strike and pickup geometry above it.
	return validateNonlinearCoupling(d)
}

func validateContact(contact Contact) error {
	switch contact.Model {
	case ContactPrescribed, ContactHertzian:
	default:
		return fmt.Errorf(
			"%w: unknown strike.contact.model %q",
			ErrInvalidConfig,
			contact.Model,
		)
	}

	if err := finiteRange(
		"strike.contact.stiffnessNPerMAlpha",
		contact.StiffnessNPerMAlpha,
		1,
		1e12,
	); err != nil {
		return err
	}

	// The lower bound is not cosmetic. Contact duration scales as
	// v^(-(alpha-1)/(alpha+1)), so alpha = 1 is a linear spring whose contact
	// time does not depend on how hard the drum is hit at all, and below it the
	// dependence inverts and a loud stroke would dwell longer than a quiet one.
	if err := finiteRange(
		"strike.contact.exponent",
		contact.Exponent,
		1,
		4,
	); err != nil {
		return err
	}

	// The ceiling is the model's validity limit, not a taste bound. The
	// Hunt-Crossley force is K*d^alpha*(1 + h*ddot), so once h exceeds the
	// reciprocal of the separation speed the bracket goes negative and the force
	// is clipped to zero part-way through the release. That is a step
	// discontinuity, and it costs the pulse the smooth spectrum it exists to
	// have. One second per metre is above any strike this model admits and well
	// below where the clipping starts to matter.
	if err := finiteRange(
		"strike.contact.hysteresisSPerM",
		contact.HysteresisSPerM,
		0,
		1,
	); err != nil {
		return err
	}

	return finiteRange(
		"strike.contact.maxDurationSeconds",
		contact.MaxDurationSeconds,
		1e-4,
		0.5,
	)
}

// validateCavityModes bounds the enclosed-air state count and keeps the retained
// transverse modes inside the same anti-alias band the heads are held to.
//
// The count is capped because the midpoint elimination is a k x k dense solve in
// the audio path. The frequency bound is the cavity's version of the head's
// FrequencyLimitFraction: a cavity mode is an oscillator like any other, and one
// placed near Nyquist would be as badly resolved as a head mode there. It cannot
// trip on any shipped geometry — the eighth mode of a 12-inch shell is near
// 1.6 kHz — but a small shell or a low sample rate can reach it.
func validateCavityModes(config PhysicalDrum) error {
	if config.Cavity.ModeCount < 1 || config.Cavity.ModeCount > maxCavityModes {
		return fmt.Errorf(
			"%w: cavity.modeCount=%d outside [1,%d]",
			ErrInvalidConfig,
			config.Cavity.ModeCount,
			maxCavityModes,
		)
	}

	modes, err := generateCavityModes(config)
	if err != nil {
		return err
	}

	if len(modes) != config.Cavity.ModeCount {
		return fmt.Errorf(
			"%w: cavity.modeCount=%d admits only %d modes",
			ErrInvalidConfig,
			config.Cavity.ModeCount,
			len(modes),
		)
	}

	limitHz := config.Batter.FrequencyLimitFraction * config.SampleRateHz
	for _, mode := range modes {
		if mode.FrequencyHz > limitHz {
			return fmt.Errorf(
				"%w: cavity mode (%d,%d) at %.1f Hz exceeds the anti-alias limit %.1f Hz",
				ErrInvalidConfig,
				mode.AzimuthalOrder,
				mode.RadialOrder,
				mode.FrequencyHz,
				limitHz,
			)
		}
	}

	return nil
}

func validateNonlinearity(config PhysicalDrum) error {
	nonlinearity := config.Nonlinearity
	if err := finiteRange(
		"nonlinearity.batterTensionCoefficientNPerM3",
		nonlinearity.BatterTensionCoefficientNPerM3,
		0,
		1e9,
	); err != nil {
		return err
	}

	if err := finiteRange(
		"nonlinearity.resonantTensionCoefficientNPerM3",
		nonlinearity.ResonantTensionCoefficientNPerM3,
		0,
		1e9,
	); err != nil {
		return err
	}

	if err := finiteRange(
		"nonlinearity.maximumTensionRatio",
		nonlinearity.MaximumTensionRatio,
		0,
		1,
	); err != nil {
		return err
	}

	if !nonlinearity.Enabled {
		return nil
	}

	if nonlinearity.BatterTensionCoefficientNPerM3 == 0 &&
		(!config.Resonant.Enabled ||
			nonlinearity.ResonantTensionCoefficientNPerM3 == 0) {
		return fmt.Errorf(
			"%w: enabled nonlinearity has no positive tension coefficient",
			ErrInvalidConfig,
		)
	}

	if nonlinearity.MaximumTensionRatio == 0 {
		return fmt.Errorf(
			"%w: enabled nonlinearity has zero maximum tension ratio",
			ErrInvalidConfig,
		)
	}

	safeRatio := maximumSafeTensionRatio(config.Batter)
	if config.Resonant.Enabled {
		safeRatio = min(safeRatio, maximumSafeTensionRatio(config.Resonant))
	}

	if nonlinearity.MaximumTensionRatio >= safeRatio {
		return fmt.Errorf(
			"%w: nonlinearity.maximumTensionRatio %v reaches anti-alias bound %v",
			ErrInvalidConfig,
			nonlinearity.MaximumTensionRatio,
			safeRatio,
		)
	}

	return nil
}

// validateNonlinearCoupling bounds the coupling's settings and holds the cubic
// force it generates inside the anti-alias band.
//
// The existing bound r < 1/(4*alpha^2) - 1 does not cover this and never did: it
// bounds a *uniform* detune, which moves every retained mode by the same
// relative amount, and says nothing about a force that is a product of three
// sampled modal signals and therefore reaches the sum of three modal
// frequencies. Since the truncation puts a pump index in every retained entry
// the worst case is not 3*f_top; it is bounded by max_P f + 2*f_top, which is
// the quantity checked here. The table the model actually builds is usually
// tighter still, and DoubleHead records that one.
//
// It does not bind on any shipped tier — the shipped default clears it by about
// 6x — but it bites if Quality rises, if SampleRateHz falls toward 8 kHz, or if
// PumpMaxFrequencyHz widens, and it is three lines.
//
// maxCouplingCoefficientNPerM is a weaker claim than the rest of this and cannot
// be sufficient alone; see coupling.go and DoubleHead.solveNonlinearStep.
func validateNonlinearCoupling(config PhysicalDrum) error {
	coupling := config.Nonlinearity.Coupling
	// The zero struct is "no coupling field", which is what a hand-built
	// PhysicalDrum literal and any pre-v11 document carry. It is inert by
	// construction, so the settings below are not held to their live ranges.
	if coupling == (NonlinearCoupling{}) {
		return nil
	}

	if err := finiteRange(
		"nonlinearity.coupling.coefficientNPerM",
		coupling.CoefficientNPerM,
		0,
		maxCouplingCoefficientNPerM,
	); err != nil {
		return err
	}

	if coupling.PumpCount < 0 || coupling.PumpCount > maxCouplingPumps {
		return fmt.Errorf(
			"%w: nonlinearity.coupling.pumpCount=%d outside [0,%d]",
			ErrInvalidConfig,
			coupling.PumpCount,
			maxCouplingPumps,
		)
	}

	if coupling.MaxCoefficients < 0 ||
		coupling.MaxCoefficients > maxCouplingCoefficients {
		return fmt.Errorf(
			"%w: nonlinearity.coupling.maxCoefficients=%d outside [0,%d]",
			ErrInvalidConfig,
			coupling.MaxCoefficients,
			maxCouplingCoefficients,
		)
	}

	pumpLimitHz := config.Batter.FrequencyLimitFraction * config.SampleRateHz
	if err := finiteRange(
		"nonlinearity.coupling.pumpMaxFrequencyHz",
		coupling.PumpMaxFrequencyHz,
		math.SmallestNonzeroFloat64,
		pumpLimitHz,
	); err != nil {
		return err
	}

	if err := finiteRange(
		"nonlinearity.coupling.aliasFraction",
		coupling.AliasFraction,
		math.SmallestNonzeroFloat64,
		0.5,
	); err != nil {
		return err
	}

	// Subordinate to the parent flag rather than in conflict with it. The
	// coupling is a second channel set of the same potential, so switching the
	// nonlinearity off switches it off too — the way Resonant.Enabled already
	// governs the resonant tension coefficient. Erroring here instead would make
	// `config.Nonlinearity.Enabled = false`, which is how every linear-reference
	// test and every A/B experiment reaches the linear model, a hard failure on
	// a default configuration.
	if !coupling.Enabled || !config.Nonlinearity.Enabled {
		return nil
	}

	if coupling.CoefficientNPerM == 0 {
		return fmt.Errorf(
			"%w: enabled nonlinearity.coupling has a zero coefficient",
			ErrInvalidConfig,
		)
	}

	if coupling.MaxCoefficients == 0 {
		return fmt.Errorf(
			"%w: enabled nonlinearity.coupling retains no coefficients",
			ErrInvalidConfig,
		)
	}

	if coupling.PumpCount < 2 {
		return fmt.Errorf(
			"%w: nonlinearity.coupling.pumpCount=%d is below 2; the coupling "+
				"force is cubic and odd, so a single pump deposits only at f_p "+
				"and 3f_p and can reach nothing between them",
			ErrInvalidConfig,
			coupling.PumpCount,
		)
	}

	modes, err := generateHeadModes(config, config.Batter)
	if err != nil {
		return fmt.Errorf("nonlinearity.coupling: batter modes: %w", err)
	}

	topHz := 0.0
	pumpHz := 0.0

	for index := range modes {
		frequency := modes[index].FrequencyHz

		topHz = max(topHz, frequency)
		if frequency <= coupling.PumpMaxFrequencyHz {
			pumpHz = max(pumpHz, frequency)
		}
	}

	detune := math.Sqrt(1 + config.Nonlinearity.MaximumTensionRatio)
	worstHz := (pumpHz + 2*topHz) * detune

	limitHz := coupling.AliasFraction * config.SampleRateHz
	if worstHz >= limitHz {
		return fmt.Errorf(
			"%w: nonlinear coupling reaches %.1f Hz (pump %.1f Hz, top mode "+
				"%.1f Hz, detune %.4f) against the alias bound %.1f Hz",
			ErrInvalidConfig,
			worstHz,
			pumpHz,
			topHz,
			detune,
			limitHz,
		)
	}

	return nil
}

func maximumSafeTensionRatio(head Head) float64 {
	frequencyLimit := head.FrequencyLimitFraction

	return 1/(4*frequencyLimit*frequencyLimit) - 1
}

func validateHead(name string, head Head, required bool) error {
	if required && !head.Enabled {
		return fmt.Errorf("%w: %s must be enabled", ErrInvalidConfig, name)
	}

	checks := []struct {
		field    string
		value    float64
		minValue float64
		maxValue float64
	}{
		{"radiusM", head.RadiusM, 0.02, 1},
		{"surfaceDensityKgPerM2", head.SurfaceDensityKgPerM2, 0.01, 10},
		{"tensionNPerM", head.TensionNPerM, 1, 100_000},
		{"bendingStiffnessNM", head.BendingStiffnessNM, 0, 100},
		{"loss0PerSecond", head.Loss0PerSecond, 0, 10_000},
		{"loss1MPerSecond", head.Loss1MPerSecond, 0, 1_000},
		{"loss2M2PerSecond", head.Loss2M2PerSecond, 0, 10},
		{"radiationLossPerSecond", head.RadiationLossPerSecond, 0, 10_000},
		{"frequencyLimitFraction", head.FrequencyLimitFraction, 0.05, 0.49},
		{"inactiveEnergyThresholdJ", head.InactiveEnergyThresholdJ, 0, 1},
	}
	for _, check := range checks {
		if err := finiteRange(name+"."+check.field, check.value, check.minValue, check.maxValue); err != nil {
			return err
		}
	}

	if err := finiteRange(
		name+".tensionAsymmetry.splitRatio",
		head.TensionAsymmetry.SplitRatio,
		0,
		0.02,
	); err != nil {
		return err
	}

	if err := finiteRange(
		name+".tensionAsymmetry.principalAxisAngleRad",
		head.TensionAsymmetry.PrincipalAxisAngleRad,
		-math.Pi,
		math.Pi,
	); err != nil {
		return err
	}

	seenCorrections := make(map[[2]int]struct{}, len(head.ModeDecayCorrections))
	for _, correction := range head.ModeDecayCorrections {
		if correction.AzimuthalOrder < 0 || correction.AzimuthalOrder > maxModeOrder ||
			correction.RadialOrder < 1 || correction.RadialOrder > maxModeOrder {
			return fmt.Errorf(
				"%w: %s mode correction index=(%d,%d)",
				ErrInvalidConfig,
				name,
				correction.AzimuthalOrder,
				correction.RadialOrder,
			)
		}

		if err := finiteRange(
			name+".modeDecayCorrection",
			correction.DecayRatePerSecond,
			-10_000,
			10_000,
		); err != nil {
			return err
		}

		key := [2]int{correction.AzimuthalOrder, correction.RadialOrder}
		if _, exists := seenCorrections[key]; exists {
			return fmt.Errorf(
				"%w: duplicate %s mode correction index=(%d,%d)",
				ErrInvalidConfig,
				name,
				correction.AzimuthalOrder,
				correction.RadialOrder,
			)
		}

		seenCorrections[key] = struct{}{}
	}

	return nil
}

func finiteRange(name string, value, minValue, maxValue float64) error {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < minValue || value > maxValue {
		return fmt.Errorf(
			"%w: %s=%v outside [%v,%v]",
			ErrInvalidConfig,
			name,
			value,
			minValue,
			maxValue,
		)
	}

	return nil
}

func validateAttack(config PhysicalDrum) error {
	attack := config.Attack
	if err := finiteRange(
		"attack.levelRelative",
		attack.LevelRelative,
		0,
		1_000,
	); err != nil {
		return err
	}

	if err := finiteRange(
		"attack.centreHz",
		attack.CentreHz,
		20,
		config.SampleRateHz/2,
	); err != nil {
		return err
	}

	if err := finiteRange(
		"attack.qualityFactor",
		attack.QualityFactor,
		0.1,
		20,
	); err != nil {
		return err
	}

	return finiteRange("attack.decayScale", attack.DecayScale, 0, 100)
}
