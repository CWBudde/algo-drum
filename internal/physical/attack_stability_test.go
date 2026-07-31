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

// TestValidatedEndpointsRenderFinite is the claim Validate implicitly makes,
// swept over the fields the coupling work did not cover: the loss coefficients,
// the head geometry, the cavity, the strike and the pickup, each driven to both
// ends of its validated range.
//
// Endpoints that Validate rejects are skipped rather than failed — a rejected
// combination is the validator doing its job, and which combinations those are
// is a property of the other bounds, not of this test.
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
		fields := []struct {
			field string
			set   func(*Head, float64)
			low   float64
			high  float64
		}{
			{"radiusM", func(h *Head, v float64) { h.RadiusM = v }, 0.02, 1},
			{"surfaceDensityKgPerM2", func(h *Head, v float64) {
				h.SurfaceDensityKgPerM2 = v
			}, 0.01, 10},
			{"tensionNPerM", func(h *Head, v float64) { h.TensionNPerM = v }, 1, 100_000},
			{"bendingStiffnessNM", func(h *Head, v float64) {
				h.BendingStiffnessNM = v
			}, 0, 100},
			{"loss0PerSecond", func(h *Head, v float64) { h.Loss0PerSecond = v }, 0, 10_000},
			{"loss1MPerSecond", func(h *Head, v float64) { h.Loss1MPerSecond = v }, 0, 1_000},
			{"loss2M2PerSecond", func(h *Head, v float64) { h.Loss2M2PerSecond = v }, 0, 10},
			{"radiationLossPerSecond", func(h *Head, v float64) {
				h.RadiationLossPerSecond = v
			}, 0, 10_000},
			{"frequencyLimitFraction", func(h *Head, v float64) {
				h.FrequencyLimitFraction = v
			}, 0.05, 0.49},
			{"inactiveEnergyThresholdJ", func(h *Head, v float64) {
				h.InactiveEnergyThresholdJ = v
			}, 0, 1},
			{"tensionAsymmetry.splitRatio", func(h *Head, v float64) {
				h.TensionAsymmetry.SplitRatio = v
			}, 0, 0.02},
			{"tensionAsymmetry.principalAxisAngleRad", func(h *Head, v float64) {
				h.TensionAsymmetry.PrincipalAxisAngleRad = v
			}, -math.Pi, math.Pi},
		}

		endpoints := make([]endpoint, 0, 2*len(fields)+1)
		for _, field := range fields {
			endpoints = append(
				endpoints,
				endpoint{name + "." + field.field + "=low", func(c *PhysicalDrum) {
					field.set(get(c), field.low)
				}},
				endpoint{name + "." + field.field + "=high", func(c *PhysicalDrum) {
					field.set(get(c), field.high)
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

	endpoints := []endpoint{
		{"sampleRateHz=low", func(c *PhysicalDrum) { c.SampleRateHz = minSampleRateHz }},
		{"sampleRateHz=high", func(c *PhysicalDrum) { c.SampleRateHz = maxSampleRateHz }},
		{"resonantModeLimit=low", func(c *PhysicalDrum) { c.ResonantModeLimit = 1 }},
		{"resonantModeLimit=high", func(c *PhysicalDrum) {
			c.ResonantModeLimit = maxResonantModeLimit
		}},
		{"strike.radius01=low", func(c *PhysicalDrum) { c.Strike.Radius01 = 0 }},
		{"strike.radius01=high", func(c *PhysicalDrum) { c.Strike.Radius01 = 1 }},
		{"strike.angleRad=low", func(c *PhysicalDrum) { c.Strike.AngleRad = -2 * math.Pi }},
		{"strike.angleRad=high", func(c *PhysicalDrum) { c.Strike.AngleRad = 2 * math.Pi }},
		{"strike.contactRadiusM=low", func(c *PhysicalDrum) { c.Strike.ContactRadiusM = 1e-4 }},
		{"strike.contactRadiusM=high", func(c *PhysicalDrum) {
			c.Strike.ContactRadiusM = c.Batter.RadiusM / 2
		}},
		{"strike.malletMassKg=low", func(c *PhysicalDrum) { c.Strike.MalletMassKg = 1e-4 }},
		{"strike.malletMassKg=high", func(c *PhysicalDrum) { c.Strike.MalletMassKg = 1 }},
		{"strike.velocityMPerS=low", func(c *PhysicalDrum) { c.Strike.VelocityMPerS = 0 }},
		{"strike.velocityMPerS=high", func(c *PhysicalDrum) { c.Strike.VelocityMPerS = 20 }},
		{"strike.hardness01=low", func(c *PhysicalDrum) { c.Strike.Hardness01 = 0 }},
		{"strike.hardness01=high", func(c *PhysicalDrum) { c.Strike.Hardness01 = 1 }},
		{"cavity.disabled", func(c *PhysicalDrum) { c.Cavity.Enabled = false }},
		{"cavity.depthM=low", func(c *PhysicalDrum) { c.Cavity.DepthM = 0.01 }},
		{"cavity.depthM=high", func(c *PhysicalDrum) { c.Cavity.DepthM = 2 }},
		{"cavity.coupling01=low", func(c *PhysicalDrum) { c.Cavity.Coupling01 = 0 }},
		{"cavity.coupling01=high", func(c *PhysicalDrum) { c.Cavity.Coupling01 = 1 }},
		{"cavity.stiffnessScale=low", func(c *PhysicalDrum) { c.Cavity.StiffnessScale = 0 }},
		{"cavity.stiffnessScale=high", func(c *PhysicalDrum) { c.Cavity.StiffnessScale = 1 }},
		{"cavity.airDensityKgPerM3=low", func(c *PhysicalDrum) {
			c.Cavity.AirDensityKgPerM3 = 0.5
		}},
		{"cavity.airDensityKgPerM3=high", func(c *PhysicalDrum) {
			c.Cavity.AirDensityKgPerM3 = 2
		}},
		{"cavity.soundSpeedMPerS=low", func(c *PhysicalDrum) { c.Cavity.SoundSpeedMPerS = 250 }},
		{"cavity.soundSpeedMPerS=high", func(c *PhysicalDrum) { c.Cavity.SoundSpeedMPerS = 400 }},
		{"cavity.lossPerSecond=low", func(c *PhysicalDrum) { c.Cavity.LossPerSecond = 0 }},
		{"cavity.lossPerSecond=high", func(c *PhysicalDrum) { c.Cavity.LossPerSecond = 10_000 }},
		{"cavity.modeCount=low", func(c *PhysicalDrum) { c.Cavity.ModeCount = 1 }},
		{"cavity.modeCount=high", func(c *PhysicalDrum) { c.Cavity.ModeCount = maxCavityModes }},
		{"attack.disabled", func(c *PhysicalDrum) { c.Attack.Enabled = false }},
		{"attack.levelRelative=low", func(c *PhysicalDrum) { c.Attack.LevelRelative = 0 }},
		{"attack.levelRelative=high", func(c *PhysicalDrum) { c.Attack.LevelRelative = 1_000 }},
		{"attack.centreHz=low", func(c *PhysicalDrum) { c.Attack.CentreHz = 20 }},
		{"attack.centreHz=high", func(c *PhysicalDrum) {
			c.Attack.CentreHz = c.SampleRateHz / 2
		}},
		{"attack.qualityFactor=low", func(c *PhysicalDrum) { c.Attack.QualityFactor = 0.1 }},
		{"attack.qualityFactor=high", func(c *PhysicalDrum) { c.Attack.QualityFactor = 20 }},
		{"attack.decayScale=low", func(c *PhysicalDrum) { c.Attack.DecayScale = 0 }},
		{"attack.decayScale=high", func(c *PhysicalDrum) { c.Attack.DecayScale = 100 }},
		{"pickup.radius01=low", func(c *PhysicalDrum) { c.Pickup.Radius01 = 0 }},
		{"pickup.radius01=high", func(c *PhysicalDrum) { c.Pickup.Radius01 = 1 }},
		{"pickup.angleRad=low", func(c *PhysicalDrum) { c.Pickup.AngleRad = -2 * math.Pi }},
		{"pickup.angleRad=high", func(c *PhysicalDrum) { c.Pickup.AngleRad = 2 * math.Pi }},
		{"pickup.distanceM=low", func(c *PhysicalDrum) { c.Pickup.DistanceM = 0.01 }},
		{"pickup.distanceM=high", func(c *PhysicalDrum) { c.Pickup.DistanceM = 10 }},
		{"pickup.nearFieldScale=low", func(c *PhysicalDrum) { c.Pickup.NearFieldScale = 0 }},
		{"pickup.nearFieldScale=high", func(c *PhysicalDrum) { c.Pickup.NearFieldScale = 10 }},
		{"pickup.highpassHz=low", func(c *PhysicalDrum) { c.Pickup.HighpassHz = 1 }},
		{"pickup.highpassHz=high", func(c *PhysicalDrum) {
			c.Pickup.HighpassHz = maxSampleRateHz / 2
			c.Pickup.LowpassHz = maxSampleRateHz / 2
		}},
		{"pickup.lowpassHz=low", func(c *PhysicalDrum) {
			c.Pickup.LowpassHz = c.Pickup.HighpassHz
		}},
		{"pickup.lowpassHz=high", func(c *PhysicalDrum) {
			c.Pickup.LowpassHz = maxSampleRateHz / 2
		}},
		{"pickup.outputGain=low", func(c *PhysicalDrum) { c.Pickup.OutputGain = 0 }},
		{"pickup.outputGain=high", func(c *PhysicalDrum) { c.Pickup.OutputGain = 100 }},
	}

	endpoints = append(endpoints, headEndpoints("batter", func(c *PhysicalDrum) *Head {
		return &c.Batter
	})...)
	endpoints = append(endpoints, headEndpoints("resonant", func(c *PhysicalDrum) *Head {
		return &c.Resonant
	})...)

	for _, contactModel := range []ContactModel{ContactPrescribed, ContactHertzian} {
		contactFields := []struct {
			field string
			set   func(*Contact, float64)
			low   float64
			high  float64
		}{
			{"stiffnessNPerMAlpha", func(c *Contact, v float64) {
				c.StiffnessNPerMAlpha = v
			}, 1, 1e12},
			{"exponent", func(c *Contact, v float64) { c.Exponent = v }, 1, 4},
			{"hysteresisSPerM", func(c *Contact, v float64) { c.HysteresisSPerM = v }, 0, 1},
			{"maxDurationSeconds", func(c *Contact, v float64) {
				c.MaxDurationSeconds = v
			}, 1e-4, 0.5},
		}

		for _, field := range contactFields {
			name := "strike.contact." + string(contactModel) + "." + field.field
			endpoints = append(
				endpoints,
				endpoint{name + "=low", func(c *PhysicalDrum) {
					c.Strike.Contact.Model = contactModel
					field.set(&c.Strike.Contact, field.low)
				}},
				endpoint{name + "=high", func(c *PhysicalDrum) {
					c.Strike.Contact.Model = contactModel
					field.set(&c.Strike.Contact, field.high)
				}},
			)
		}
	}

	for _, testCase := range endpoints {
		config := DefaultPhysicalDrum()
		testCase.apply(&config)

		if err := config.Validate(); err != nil {
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
}
