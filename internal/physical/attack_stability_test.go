package physical

import (
	"math"
	"testing"
)

// renderAttack builds and renders one second from a configuration that has
// already been through Validate, which is the contract under test: everything
// Validate accepts must render finite audio.
func renderAttack(t *testing.T, config PhysicalDrum) (*DoubleHead, []float64) {
	t.Helper()

	if err := config.Validate(); err != nil {
		t.Fatalf("configuration must validate: %v", err)
	}

	model, err := NewDoubleHead(config)
	if err != nil {
		t.Fatalf("NewDoubleHead: %v", err)
	}

	if err := model.Trigger(1); err != nil {
		t.Fatalf("Trigger: %v", err)
	}

	buffer := make([]float64, int(config.SampleRateHz))
	model.Render(buffer)

	return model, buffer
}

// losslessHeadConfig is the configuration that used to render every sample NaN:
// all three structural loss coefficients at their validated lower bound of zero,
// which makes the loss law the attack layer reads return exactly zero.
func losslessHeadConfig() PhysicalDrum {
	config := DefaultPhysicalDrum()
	config.Batter.Loss0PerSecond = 0
	config.Batter.Loss1MPerSecond = 0
	config.Batter.Loss2M2PerSecond = 0
	config.Batter.RadiationLossPerSecond = 0

	return config
}

// TestAttackLayerSurvivesALosslessHead is the defect itself. The head validates
// — every loss knob's lower bound is zero — and before the release was clamped,
// DecayScale/0 was +Inf and the per-sample factor Inf/(Inf+1) = NaN.
func TestAttackLayerSurvivesALosslessHead(t *testing.T) {
	t.Parallel()

	config := losslessHeadConfig()

	_, buffer := renderAttack(t, config)

	if index := firstNonFinite(buffer); index >= 0 {
		t.Fatalf("sample %d is not finite on a lossless head", index)
	}
}

// TestAttackReleaseIsAlwaysFiniteAndDecaying states the property directly, over
// the whole validated corner of the loss law rather than only its zero. A
// per-sample factor of exactly 1 is as bad as a NaN in the one way that matters
// here: the envelope never falls under attackInactiveEnvelope, so the voice never
// goes inactive.
func TestAttackReleaseIsAlwaysFiniteAndDecaying(t *testing.T) {
	t.Parallel()

	losses := []float64{0, 1e-12, 1e-9, 1e-3, 1, 10}
	rates := []float64{8_000, 48_000, 384_000}

	for _, loss := range losses {
		for _, rate := range rates {
			for _, scale := range []float64{0, 1, 100} {
				head := DefaultPhysicalDrum().Batter
				head.Loss0PerSecond = loss
				head.Loss1MPerSecond = 0
				head.Loss2M2PerSecond = 0

				attack := DefaultPhysicalDrum().Attack
				attack.DecayScale = scale

				layer := newAttackLayer(attack, head, rate)

				for index, band := range layer.bands {
					if math.IsNaN(band.decayFactor) ||
						math.IsInf(band.decayFactor, 0) {
						t.Fatalf(
							"loss %v rate %v scale %v: band %d factor %v",
							loss, rate, scale, index, band.decayFactor,
						)
					}

					if band.decayFactor < 0 || band.decayFactor >= 1 {
						t.Fatalf(
							"loss %v rate %v scale %v: band %d factor %v is not a decay",
							loss, rate, scale, index, band.decayFactor,
						)
					}
				}
			}
		}
	}
}

// TestAttackClampIsInertOnShippedConfigurations is the other half of the
// contract, and the reason the shipped render digest does not move: on everything
// that ships the clamp never binds, so the release is the one the loss law gives
// and the arithmetic is the arithmetic that was there before.
func TestAttackClampIsInertOnShippedConfigurations(t *testing.T) {
	t.Parallel()

	qualities := []Quality{QualityDraft, QualityStandard, QualityHigh}
	rates := []float64{44_100, 48_000, 96_000}

	for _, quality := range qualities {
		for _, rate := range rates {
			config := DefaultPhysicalDrum()
			config.Quality = quality
			config.SampleRateHz = rate

			speed := WaveSpeedMPerS(config.Batter)

			for _, ratio := range attackBandRatios {
				centreHz := min(config.Attack.CentreHz*ratio, rate*0.45)
				decayRate := ModalDecayRatePerSecond(
					config.Batter,
					2*math.Pi*centreHz/speed,
				)
				release := config.Attack.DecayScale / decayRate

				if release >= maxAttackReleaseSeconds {
					t.Fatalf(
						"%s %.0f Hz: shipped release %v s reaches the clamp %v s",
						quality, rate, release, float64(maxAttackReleaseSeconds),
					)
				}

				if got := attackReleaseSeconds(
					config.Attack.DecayScale,
					decayRate,
				); got != release {
					t.Fatalf(
						"%s %.0f Hz: clamped release %v, want the unclamped %v",
						quality, rate, got, release,
					)
				}
			}

			_, buffer := renderAttack(t, config)

			if index := firstNonFinite(buffer); index >= 0 {
				t.Fatalf("%s %.0f Hz: sample %d is not finite", quality, rate, index)
			}
		}
	}
}

// validatedScalarField is one config field whose validated range is a pair of
// constants, paired with the setter that drives it to either end.
//
// It is package-level so that TestValidatedEndpointsRenderFinite and
// TestTheEndpointSweepCoversEveryConstantBound read the same list. Two lists
// would reintroduce, one level up, exactly the duplication configBounds exists
// to remove.
type validatedScalarField struct {
	key string
	set func(*PhysicalDrum, float64)
}

// validatedHeadFields and validatedContactFields are the same idea for the two
// tables that are swept more than once — the head fields under both the batter
// and the resonant head, the contact fields under both contact models.
func validatedHeadFields() []struct {
	field string
	set   func(*Head, float64)
} {
	return []struct {
		field string
		set   func(*Head, float64)
	}{
		{"radiusM", func(h *Head, v float64) { h.RadiusM = v }},
		{"surfaceDensityKgPerM2", func(h *Head, v float64) { h.SurfaceDensityKgPerM2 = v }},
		{"tensionNPerM", func(h *Head, v float64) { h.TensionNPerM = v }},
		{"bendingStiffnessNM", func(h *Head, v float64) { h.BendingStiffnessNM = v }},
		{"loss0PerSecond", func(h *Head, v float64) { h.Loss0PerSecond = v }},
		{"loss1MPerSecond", func(h *Head, v float64) { h.Loss1MPerSecond = v }},
		{"loss2M2PerSecond", func(h *Head, v float64) { h.Loss2M2PerSecond = v }},
		{"radiationLossPerSecond", func(h *Head, v float64) { h.RadiationLossPerSecond = v }},
		{"frequencyLimitFraction", func(h *Head, v float64) { h.FrequencyLimitFraction = v }},
		{"inactiveEnergyThresholdJ", func(h *Head, v float64) { h.InactiveEnergyThresholdJ = v }},
		{"tensionAsymmetry.splitRatio", func(h *Head, v float64) {
			h.TensionAsymmetry.SplitRatio = v
		}},
		{"tensionAsymmetry.principalAxisAngleRad", func(h *Head, v float64) {
			h.TensionAsymmetry.PrincipalAxisAngleRad = v
		}},
	}
}

func validatedContactFields() []struct {
	field string
	set   func(*Contact, float64)
} {
	return []struct {
		field string
		set   func(*Contact, float64)
	}{
		{"stiffnessNPerMAlpha", func(c *Contact, v float64) { c.StiffnessNPerMAlpha = v }},
		{"exponent", func(c *Contact, v float64) { c.Exponent = v }},
		{"hysteresisSPerM", func(c *Contact, v float64) { c.HysteresisSPerM = v }},
		{"maxDurationSeconds", func(c *Contact, v float64) { c.MaxDurationSeconds = v }},
	}
}

// Every scalar whose validated range is a pair of constants. Nothing here
// restates a number from validate.go, which is the whole point: this sweep used
// to carry its own copy of every bound, and a ceiling tightened on one side
// without the other silently stopped probing the endpoint it was named after.
func validatedScalarFields() []validatedScalarField {
	return []validatedScalarField{
		{"sampleRateHz", func(c *PhysicalDrum, v float64) { c.SampleRateHz = v }},
		{"strike.radius01", func(c *PhysicalDrum, v float64) { c.Strike.Radius01 = v }},
		{"strike.angleRad", func(c *PhysicalDrum, v float64) { c.Strike.AngleRad = v }},
		{"strike.malletMassKg", func(c *PhysicalDrum, v float64) { c.Strike.MalletMassKg = v }},
		{"strike.velocityMPerS", func(c *PhysicalDrum, v float64) { c.Strike.VelocityMPerS = v }},
		{"strike.hardness01", func(c *PhysicalDrum, v float64) { c.Strike.Hardness01 = v }},
		{"cavity.depthM", func(c *PhysicalDrum, v float64) { c.Cavity.DepthM = v }},
		{"cavity.coupling01", func(c *PhysicalDrum, v float64) { c.Cavity.Coupling01 = v }},
		{"cavity.stiffnessScale", func(c *PhysicalDrum, v float64) { c.Cavity.StiffnessScale = v }},
		{"cavity.airDensityKgPerM3", func(c *PhysicalDrum, v float64) {
			c.Cavity.AirDensityKgPerM3 = v
		}},
		{"cavity.soundSpeedMPerS", func(c *PhysicalDrum, v float64) { c.Cavity.SoundSpeedMPerS = v }},
		{"cavity.lossPerSecond", func(c *PhysicalDrum, v float64) { c.Cavity.LossPerSecond = v }},
		{"attack.levelRelative", func(c *PhysicalDrum, v float64) { c.Attack.LevelRelative = v }},
		{"attack.qualityFactor", func(c *PhysicalDrum, v float64) { c.Attack.QualityFactor = v }},
		{"attack.decayScale", func(c *PhysicalDrum, v float64) { c.Attack.DecayScale = v }},
		{"pickup.radius01", func(c *PhysicalDrum, v float64) { c.Pickup.Radius01 = v }},
		{"pickup.angleRad", func(c *PhysicalDrum, v float64) { c.Pickup.AngleRad = v }},
		{"pickup.distanceM", func(c *PhysicalDrum, v float64) { c.Pickup.DistanceM = v }},
		{"pickup.nearFieldScale", func(c *PhysicalDrum, v float64) { c.Pickup.NearFieldScale = v }},
		{"pickup.outputGain", func(c *PhysicalDrum, v float64) { c.Pickup.OutputGain = v }},
		{"pickup.highpassHz", func(c *PhysicalDrum, v float64) {
			c.Pickup.HighpassHz = v
			// The ceiling case would otherwise leave the lowpass below the
			// highpass and be rejected rather than rendered.
			c.Pickup.LowpassHz = max(c.Pickup.LowpassHz, v)
		}},
	}
}

// TestValidatedEndpointsRenderFinite is the claim Validate implicitly makes,
// swept over the fields the coupling work did not cover: the loss coefficients,
// the head geometry, the cavity, the strike and the pickup, each driven to both
// ends of its validated range.
//
// Endpoints that Validate rejects are skipped rather than failed — a rejected
// combination is the validator doing its job, and which combinations those are
// is a property of the other bounds, not of this test. The *count* of skipped
// endpoints is asserted, because a skip is coverage quietly withdrawn.
func TestValidatedEndpointsRenderFinite(t *testing.T) {
	t.Parallel()

	type endpoint struct {
		name  string
		apply func(*PhysicalDrum)
	}

	headEndpoints := func(
		name string,
		get func(*PhysicalDrum) *Head,
	) []endpoint {
		fields := validatedHeadFields()

		endpoints := make([]endpoint, 0, 2*len(fields)+1)
		for _, field := range fields {
			low, high := boundOf("head." + field.field)
			endpoints = append(
				endpoints,
				endpoint{name + "." + field.field + "=low", func(c *PhysicalDrum) {
					field.set(get(c), low)
				}},
				endpoint{name + "." + field.field + "=high", func(c *PhysicalDrum) {
					field.set(get(c), high)
				}},
			)
		}

		return append(endpoints, endpoint{
			name + " lossless",
			func(c *PhysicalDrum) {
				head := get(c)
				head.Loss0PerSecond = 0
				head.Loss1MPerSecond = 0
				head.Loss2M2PerSecond = 0
				head.RadiationLossPerSecond = 0
			},
		})
	}

	scalarFields := validatedScalarFields()

	endpoints := make([]endpoint, 0, 2*len(scalarFields)+24)
	for _, field := range scalarFields {
		low, high := boundOf(field.key)
		endpoints = append(
			endpoints,
			endpoint{field.key + "=low", func(c *PhysicalDrum) { field.set(c, low) }},
			endpoint{field.key + "=high", func(c *PhysicalDrum) { field.set(c, high) }},
		)
	}

	// The rest: integer counts, on/off flags, and the four ranges whose ends are
	// computed from the configuration under test rather than written down as
	// constants. See derivedBoundFields for why those four are not in the table.
	endpoints = append(endpoints, []endpoint{
		{"resonantModeLimit=low", func(c *PhysicalDrum) { c.ResonantModeLimit = 1 }},
		{"resonantModeLimit=high", func(c *PhysicalDrum) {
			c.ResonantModeLimit = maxResonantModeLimit
		}},
		{"strike.contactRadiusM=low", func(c *PhysicalDrum) { c.Strike.ContactRadiusM = 1e-4 }},
		{"strike.contactRadiusM=high", func(c *PhysicalDrum) {
			c.Strike.ContactRadiusM = c.Batter.RadiusM / 2
		}},
		{"cavity.disabled", func(c *PhysicalDrum) { c.Cavity.Enabled = false }},
		{"cavity.modeCount=low", func(c *PhysicalDrum) { c.Cavity.ModeCount = 1 }},
		{"cavity.modeCount=high", func(c *PhysicalDrum) { c.Cavity.ModeCount = maxCavityModes }},
		{"attack.disabled", func(c *PhysicalDrum) { c.Attack.Enabled = false }},
		{"attack.centreHz=low", func(c *PhysicalDrum) { c.Attack.CentreHz = 20 }},
		{"attack.centreHz=high", func(c *PhysicalDrum) {
			c.Attack.CentreHz = c.SampleRateHz / 2
		}},
		{"pickup.lowpassHz=low", func(c *PhysicalDrum) {
			c.Pickup.LowpassHz = c.Pickup.HighpassHz
		}},
		{"pickup.lowpassHz=high", func(c *PhysicalDrum) {
			c.Pickup.LowpassHz = maxSampleRateHz / 2
		}},
	}...)

	endpoints = append(endpoints, headEndpoints("batter", func(c *PhysicalDrum) *Head {
		return &c.Batter
	})...)
	endpoints = append(endpoints, headEndpoints("resonant", func(c *PhysicalDrum) *Head {
		return &c.Resonant
	})...)

	for _, contactModel := range []ContactModel{ContactPrescribed, ContactHertzian} {
		contactFields := validatedContactFields()

		for _, field := range contactFields {
			name := "strike.contact." + string(contactModel) + "." + field.field
			low, high := boundOf("strike.contact." + field.field)
			endpoints = append(
				endpoints,
				endpoint{name + "=low", func(c *PhysicalDrum) {
					c.Strike.Contact.Model = contactModel
					field.set(&c.Strike.Contact, low)
				}},
				endpoint{name + "=high", func(c *PhysicalDrum) {
					c.Strike.Contact.Model = contactModel
					field.set(&c.Strike.Contact, high)
				}},
			)
		}
	}

	// A rejected endpoint is coverage this test silently stops providing, and
	// the count is asserted because that silence is exactly how a tightened
	// ceiling deletes a case without anyone noticing. If this number moves,
	// either a bound moved or a combination that used to validate no longer
	// does, and both are things to look at rather than absorb.
	//
	// Six today, and every one of them is the same joint constraint: the
	// anti-alias limit on the nonlinearity. Driving sampleRateHz to its floor,
	// the batter radius to its floor, the batter tension or bending stiffness to
	// its ceiling, or either head's frequencyLimitFraction to its ceiling moves
	// the top of the modal bank far enough that the coupling's sum frequencies
	// or the Berger tension ratio cross the alias bound, and Validate rejects
	// the pair rather than the field. That is the validator doing its job — but
	// it does mean these six single-field endpoints are *not* covered by the
	// render check below, which nothing said until now.
	const wantSkipped = 6

	skipped := make([]string, 0, len(endpoints))

	for _, testCase := range endpoints {
		config := DefaultPhysicalDrum()
		testCase.apply(&config)

		if err := config.Validate(); err != nil {
			skipped = append(skipped, testCase.name+": "+err.Error())

			continue
		}

		model, err := NewDoubleHead(config)
		if err != nil {
			t.Errorf("%s: NewDoubleHead: %v", testCase.name, err)

			continue
		}

		if err := model.Trigger(1); err != nil {
			t.Errorf("%s: Trigger: %v", testCase.name, err)

			continue
		}

		buffer := make([]float64, int(config.SampleRateHz))
		model.Render(buffer)

		if index := firstNonFinite(buffer); index >= 0 {
			t.Errorf("%s: sample %d is not finite", testCase.name, index)
		}
	}

	t.Logf("%d endpoints probed, %d rejected by Validate", len(endpoints), len(skipped))

	if len(skipped) != wantSkipped {
		t.Errorf(
			"%d endpoints were rejected by Validate and so not rendered, want %d: %v",
			len(skipped), wantSkipped, skipped,
		)
	}
}

// TestTheEndpointSweepCoversEveryConstantBound is the other half of the same
// defence.
//
// Reading the ends out of configBounds stops the sweep probing a stale number,
// but it does not stop a *new* bound being added with no endpoint at all. This
// asserts the two lists are the same set, so adding a row to configBounds
// without adding a setter fails here rather than quietly going unswept.
func TestTheEndpointSweepCoversEveryConstantBound(t *testing.T) {
	t.Parallel()

	swept := map[string]bool{}
	for _, field := range validatedScalarFields() {
		swept[field.key] = true
	}

	for _, field := range validatedHeadFields() {
		swept["head."+field.field] = true
	}

	for _, field := range validatedContactFields() {
		swept["strike.contact."+field.field] = true
	}

	// Swept through the head loop rather than through validatedHeadFields, so
	// named here rather than left looking uncovered.
	swept["head.modeDecayCorrection"] = true

	// Reached only through the coupling's own endpoint coverage, which lives in
	// the nonlinear-coupling tests rather than in this sweep.
	swept["nonlinearity.batterTensionCoefficientNPerM3"] = true
	swept["nonlinearity.resonantTensionCoefficientNPerM3"] = true
	swept["nonlinearity.maximumTensionRatio"] = true
	swept["nonlinearity.coupling.coefficientNPerM"] = true
	swept["nonlinearity.coupling.aliasFraction"] = true

	for key := range configBounds {
		if !swept[key] {
			t.Errorf(
				"configBounds has %q but nothing drives it to either end; add a "+
					"setter or say here where it is covered",
				key,
			)
		}
	}

	for key := range swept {
		if _, ok := configBounds[key]; !ok {
			t.Errorf("the sweep probes %q, which is not a validated bound", key)
		}
	}

	for _, field := range derivedBoundFields {
		if _, ok := configBounds[field]; ok {
			t.Errorf(
				"%q is listed as a derived bound but also appears in configBounds",
				field,
			)
		}
	}
}
