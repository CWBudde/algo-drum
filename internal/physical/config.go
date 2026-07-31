// Package physical implements reduced physical models of acoustic drums.
//
// The package is deliberately independent of the sequencer and the existing
// procedural voices. Physical parameters use SI units; UI mapping and
// persistence integration belong at the application boundary.
package physical

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
)

const (
	legacyConfigVersion           = 1
	linearDoubleHeadConfigVersion = 2
	nonlinearConfigVersion        = 3
	fullCouplingConfigVersion     = 4
	asymmetryConfigVersion        = 5
	tiltedDampingConfigVersion    = 6
	fittedCavityConfigVersion     = 7
	radiatedAccelerationVersion   = 8
	multiBandAttackConfigVersion  = 9
	selectableContactVersion      = 10
	// ConfigVersion is the physical-drum JSON schema emitted by EncodeConfig.
	ConfigVersion = 11

	minSampleRateHz = 8_000.0
	maxSampleRateHz = 384_000.0
)

var (
	// ErrConfigVersion reports an unsupported persisted configuration version.
	ErrConfigVersion = errors.New("unsupported physical drum config version")
	// ErrInvalidConfig reports a non-finite or out-of-range physical parameter.
	ErrInvalidConfig = errors.New("invalid physical drum config")
)

// Quality selects a maximum real-time modal-state budget.
type Quality string

const (
	QualityDraft    Quality = "draft"
	QualityStandard Quality = "standard"
	QualityHigh     Quality = "high"
)

// ModeLimit returns the modal-oscillator budget one head selects from.
// Non-axisymmetric eigenmodes consume two slots (cosine and sine orientation).
//
// This is the *batter* head's budget, and since P9/M2 only the batter head's.
// A head with AxisymmetricOnly set runs the same selection and then keeps only
// the modes the enclosed air can reach, and how many that leaves depends on the
// cavity rather than on this number: with a lumped cavity it is the 6 m = 0 modes
// out of Standard's 96, but with the shipped transverse basis the reachable set is
// {0,1,2} and the same selection leaves 28. So one number was sizing two banks
// that are excited by different things and want different sizes, and the size of
// the second was a side effect of a cavity setting. PhysicalDrum.ResonantModeLimit
// gives the reduced head its own; see DefaultResonantModeLimit for what sets it,
// and generateHeadModes for where it applies.
//
// The tiers doubled when the resonant head stopped being computed and discarded.
// Bandwidth grows only as the square root of the count, because a membrane's mode
// count grows as f²: 48 to 96 slots moves the top retained mode from 646 Hz to
// about 914 Hz, which is 0.6 of an octave, not one. Reaching several kHz modally
// would need thousands of oscillators, which is why the attack layer exists.
func (q Quality) ModeLimit() int {
	switch q {
	case QualityDraft:
		return 48
	case QualityStandard:
		return 96
	case QualityHigh:
		return 160
	default:
		return 0
	}
}

// DefaultResonantModeLimit is the oscillator budget of a head that is reduced to
// what the enclosed air can reach — in practice the resonant head.
//
// It is a fixed number rather than a per-tier one, because the two budgets answer
// different questions. Quality.ModeLimit() buys *bandwidth* on the head the strike
// drives and the microphone hears, so it belongs on a quality tier. The reduced
// head is never struck; the cavity is its only excitation, so the span worth
// covering is the cavity's, and the cavity's basis is a property of the shell
// rather than of a quality setting. Sharing the tier's number was accidental — it
// was the budget of the *only* head that had one, back when the reduction left
// half a dozen modes and the choice could not matter — and it stopped being
// harmless when the modal cavity widened the reachable set from {0} to {0,1,2}.
//
// The value is the smallest that straddles every transverse cavity resonance,
// which is the criterion the coupling mechanism actually rests on: a cavity mode
// drives the resonant modes of its own azimuthal order, and it is represented
// faithfully only if that family has partials on both sides of it. On the shipped
// shell the cavity's (1,1) pair sits at 660 Hz, between the resonant (1,2) at
// 472 Hz and (1,3) at 685 Hz; its (2,1) pair sits at 1094 Hz, between (2,4) at
// 1001 Hz and (2,5) at 1213 Hz. The (2,5) pair is the 23rd and 24th oscillator of
// the frequency-ordered reachable bank.
//
// Measured against the uncapped Standard bank, as the difference between the
// modal cavity and the lumped one that P9/M2 introduced — the 13.1 dB feature at
// 657.5 Hz — the budget behaves like this:
//
//	 8  -10.22 dB   the (1,3) straddle is missing and the mechanism is 3 dB wrong
//	12  -13.60 dB
//	16  -13.46 dB
//	20  -13.24 dB
//	24  -13.16 dB
//	28  -13.13 dB   the whole reachable bank
//
// The sharp knee is at 12 and the residual past it is small, but the (2,1) region
// converges more slowly: 4.7 dB of whole-band transfer-function error at 12,
// 2.1 dB at 20, 1.1 dB at 24, and 0.1 dB at 26. 24 is where the second knee is,
// and where both transverse pairs are straddled rather than only the first.
//
// It is also far above every count a lumped cavity can produce — 4, 6 and 8
// axisymmetric modes at Draft, Standard and High — so it never binds on a migrated
// v1–v10 document and those still render bit-identically.
const DefaultResonantModeLimit = 24

// maxResonantModeLimit bounds the field at the largest budget the batter head can
// have, since the reduced head selects out of a bank of that size and a cap above
// it could never bind.
const maxResonantModeLimit = 160

// PhysicalDrum is the versioned, serializable physical-model configuration.
//
// Quality and ResonantModeLimit are two separate oscillator budgets and are meant
// to be: the tier sizes the batter head, and the second sizes whatever the
// reachability reduction leaves of the resonant head. They were one number until
// the cavity became modal, at which point widening the cavity basis silently
// quadrupled the resonant bank. See DefaultResonantModeLimit.
type PhysicalDrum struct {
	Version           int     `json:"version"`
	SampleRateHz      float64 `json:"sampleRateHz"`
	Quality           Quality `json:"quality"`
	ResonantModeLimit int     `json:"resonantModeLimit"`
	Batter            Head    `json:"batter"`
	Resonant          Head         `json:"resonant"`
	Strike            Strike       `json:"strike"`
	Cavity            Cavity       `json:"cavity"`
	Nonlinearity      Nonlinearity `json:"nonlinearity"`
	Attack            Attack       `json:"attack"`
	Pickup            Pickup       `json:"pickup"`
}

// Head describes one circular membrane/plate.
//
// The three loss coefficients form the structural decay law
// γ(k) = Loss0 + Loss1·k + Loss2·k², evaluated by ModalDecayRatePerSecond. The
// k¹ term is the one that expresses constant Q: with ω ≈ c·k on a membrane,
// Loss1 = ζ·c holds the fraction of critical damping ζ fixed across the mode
// series, which is the measured behaviour above the fundamental. Loss0 is a
// frequency-independent floor and Loss2 an excess high-frequency loss; neither
// can produce constant Q on its own, so both stay small.
//
// AxisymmetricOnly retains only the modes the enclosed air can reach. On the
// resonant head that is free rather than approximate: nothing can excite a
// resonant mode the cavity does not couple to. The strike force reaches only
// batter modes, and the cavity — the sole path between the heads — couples
// through an overlap integral whose azimuthal factor is exactly zero unless the
// head mode's azimuthal order matches a cavity mode's. Their displacement, their
// strain contribution to the tension law and their stored energy are therefore
// all exactly zero for all time, so dropping them is bit-exact. On the batter
// head it would silence most of the instrument.
//
// With a one-state cavity the reachable set is m = 0 alone and the field is
// literally "axisymmetric only". With transverse cavity modes it widens to the
// azimuthal orders they carry — see retainCavityReachable for why the field is
// widened rather than rejected.
type Head struct {
	Enabled                  bool                  `json:"enabled"`
	AxisymmetricOnly         bool                  `json:"axisymmetricOnly"`
	RadiusM                  float64               `json:"radiusM"`
	SurfaceDensityKgPerM2    float64               `json:"surfaceDensityKgPerM2"`
	TensionNPerM             float64               `json:"tensionNPerM"`
	TensionAsymmetry         TensionAsymmetry      `json:"tensionAsymmetry"`
	BendingStiffnessNM       float64               `json:"bendingStiffnessNM"`
	Loss0PerSecond           float64               `json:"loss0PerSecond"`
	Loss1MPerSecond          float64               `json:"loss1MPerSecond"`
	Loss2M2PerSecond         float64               `json:"loss2M2PerSecond"`
	RadiationLossPerSecond   float64               `json:"radiationLossPerSecond"`
	ModeDecayCorrections     []ModeDecayCorrection `json:"modeDecayCorrections,omitempty"`
	FrequencyLimitFraction   float64               `json:"frequencyLimitFraction"`
	InactiveEnergyThresholdJ float64               `json:"inactiveEnergyThresholdJ"`
}

// TensionAsymmetry is a reduced, deterministic representation of non-uniform
// rim tension. SplitRatio is the full relative frequency separation of each
// non-axisymmetric cosine/sine pair around its ideal circular-head frequency.
// PrincipalAxisAngleRad rotates the pair's mode shapes into the measured or
// deliberately selected tension axis. Axisymmetric modes are unchanged.
type TensionAsymmetry struct {
	SplitRatio            float64 `json:"splitRatio"`
	PrincipalAxisAngleRad float64 `json:"principalAxisAngleRad"`
}

// ModeDecayCorrection adds a measured residual to the two-parameter loss law.
// A correction applies to both orientations of a non-axisymmetric mode.
type ModeDecayCorrection struct {
	AzimuthalOrder     int     `json:"azimuthalOrder"`
	RadialOrder        int     `json:"radialOrder"`
	DecayRatePerSecond float64 `json:"decayRatePerSecond"`
}

// Mode describes one retained circular-head oscillator.
//
// Three of these fields are easy to confuse, so they are named for what they
// are rather than for where they are used:
//
//   - SweptAreaM2 is the signed *net* area the mode sweeps, and is exactly zero
//     for every m > 0 mode. It is the cavity's coupling coefficient and nothing
//     else.
//   - RadiatingMomentM2 is the far-field geometric factor: the exact Rayleigh
//     integral of the mode shape against the observation direction. It equals
//     SweptAreaM2 when the microphone is on axis and is non-zero for m > 0.
//   - PickupShape is the mode shape at a point on the head. It belongs to the
//     near-field contact diagnostics and to strike weighting, and must not
//     appear in a far-field weight.
type Mode struct {
	AzimuthalOrder           int
	RadialOrder              int
	Orientation              Orientation
	BesselZero               float64
	WavenumberPerM           float64
	FrequencyHz              float64
	AngularFrequency         float64
	StructuralDecayPerSecond float64
	RadiationDecayPerSecond  float64
	DecayCorrectionPerSecond float64
	DecayRatePerSecond       float64
	ModalMassKg              float64
	StrikeAccelerationPerN   float64
	PickupShape              float64
	RadiationWeight          float64
	RadiatingMomentM2        float64
	RadiationDirectivity     float64
	SweptAreaM2              float64
}

// Strike describes the mallet and its finite contact footprint.
type Strike struct {
	Radius01       float64 `json:"radius01"`
	AngleRad       float64 `json:"angleRad"`
	ContactRadiusM float64 `json:"contactRadiusM"`
	MalletMassKg   float64 `json:"malletMassKg"`
	VelocityMPerS  float64 `json:"velocityMPerS"`
	Hardness01     float64 `json:"hardness01"`
	Contact        Contact `json:"contact"`
}

// ContactModel selects how the strike force is produced.
type ContactModel string

const (
	// ContactPrescribed writes a half-sine of the measured contact duration
	// into the force buffer at trigger time. The head never influences it.
	ContactPrescribed ContactModel = "prescribed"
	// ContactHertzian integrates the stick as a free mass against a
	// Hunt-Crossley contact spring, so the force follows from where the head
	// is. Duration, shape and re-contact are outputs rather than inputs.
	ContactHertzian ContactModel = "hertzian"
)

// Contact parameterizes the stick/head interaction.
//
// StiffnessNPerMAlpha is the tip's contact stiffness at the reference hardness,
// in N/m^alpha — the units carry the exponent, which is why it cannot be read as
// a spring constant. Exponent is Hertz's alpha.
//
// Exponent is fixed by measurement rather than assumed. A Hertzian contact time
// scales as v^(-(alpha-1)/(alpha+1)), and Wagner's Fig. 4.7 crescendo runs
// 7.5 ms at piano to 5.9 ms at forte; over the three- to fourfold striking
// velocity that spans, the implied alpha is 1.42 to 1.56. So the canonical
// spherical-contact 3/2 is not a convenient assumption here, it is what the
// measured velocity dependence says — which is worth stating plainly, because it
// means the prescribed model's velocity law is not discarded by this change but
// reproduced by it.
//
// HysteresisSPerM is the Hunt-Crossley coefficient: the elastic force is scaled
// by (1 + h*compression rate), so the loss vanishes with the compression instead
// of stepping at impact and at separation the way a linear dashpot does. It sets
// the tip's restitution, and with it how much of the stick's energy is left to
// be caught by the returning head.
//
// MaxDurationSeconds bounds how long one strike is tracked. It is not a contact
// time — the stick separates long before it — but the window inside which the
// head is allowed to rise back into the stick. Past it the player has lifted.
type Contact struct {
	Model               ContactModel `json:"model"`
	StiffnessNPerMAlpha float64      `json:"stiffnessNPerMAlpha"`
	Exponent            float64      `json:"exponent"`
	HysteresisSPerM     float64      `json:"hysteresisSPerM"`
	MaxDurationSeconds  float64      `json:"maxDurationSeconds"`
}

// Cavity describes the lumped enclosed-air spring and its pressure loss.
//
// StiffnessScale multiplies the rigid-enclosure bulk stiffness rho*c^2/V. A real
// shell is not rigid and its heads are not pistons, so the rigid formula badly
// over-predicts how much the air stiffens the axisymmetric modes; the scale is
// the one place that discrepancy is absorbed. The vent is a third candidate and a
// small one: a port is a Helmholtz high-pass, and a 10 mm vent in this shell
// tunes at 32 Hz and diverts under 5 % of the flow at the 150 Hz fundamental,
// where the fitted scale is a factor of twelve. See docs/physical-cavity.md. It is a
// fraction rather than a free gain because the rigid, sealed, piston-driven
// enclosure is the stiffest case there is — 1 is the physical ceiling, not a
// neutral setting. It multiplies every cavity mode's stiffness alike, so the
// ceiling keeps its meaning once the air carries more than one state.
//
// ModeCount is how many enclosed-air pressure states the cavity carries, chosen
// in frequency order from the rigid-walled cylinder's axially uniform family; an
// m > 0 mode costs two states for its cosine and sine members. 1 is the single
// uniform compliance this model had before transverse modes existed, and is the
// exact reproduction of it. See cavity.go for the basis and docs/physical-cavity.md
// for what the transverse modes buy.
type Cavity struct {
	Enabled           bool    `json:"enabled"`
	DepthM            float64 `json:"depthM"`
	Coupling01        float64 `json:"coupling01"`
	StiffnessScale    float64 `json:"stiffnessScale"`
	AirDensityKgPerM3 float64 `json:"airDensityKgPerM3"`
	SoundSpeedMPerS   float64 `json:"soundSpeedMPerS"`
	LossPerSecond     float64 `json:"lossPerSecond"`
	ModeCount         int     `json:"modeCount"`
}

// Nonlinearity controls the Berger-style tension increase shared by every
// retained mode of each head. TensionCoefficientNPerM3 is the small-strain
// slope dT/dS, where S is the integral of the squared head gradient in m².
// MaximumTensionRatio caps the tension increase relative to each head's static
// tension so retained modes remain below Nyquist.
type Nonlinearity struct {
	Enabled                          bool    `json:"enabled"`
	BatterTensionCoefficientNPerM3   float64 `json:"batterTensionCoefficientNPerM3"`
	ResonantTensionCoefficientNPerM3 float64 `json:"resonantTensionCoefficientNPerM3"`
	MaximumTensionRatio              float64 `json:"maximumTensionRatio"`
}

// Attack is the stochastic high-band layer that stands in for the modes this
// model cannot afford to resolve. LevelRelative is fitted against the modal
// layer; CentreHz places the band group and QualityFactor sets the width of each
// of its bands.
//
// DecayScale is a dimensionless multiplier on releases that are otherwise
// *derived*: each band decays at the batter head's own structural loss law
// evaluated at that band's centre, so the layer extrapolates the mode series
// instead of ringing at a rate of its own. It replaced an absolute DecaySeconds,
// which was a fitted 20 ms — a 138 ms T60 held flat across 1-8 kHz, against a
// loss law that puts that band between 75 ms and 18 ms. The scale exists at all
// only because the law is being read past the range it was fitted in.
type Attack struct {
	Enabled       bool    `json:"enabled"`
	LevelRelative float64 `json:"levelRelative"`
	CentreHz      float64 `json:"centreHz"`
	QualityFactor float64 `json:"qualityFactor"`
	DecayScale    float64 `json:"decayScale"`
}

// Pickup places the microphone and sets the balance of the two mechanisms it
// hears. Radius01 and AngleRad locate it over the head and DistanceM sets its
// height, and between them they decide both the observation direction used by
// the far-field weight and how much of the non-propagating near field survives.
//
// NearFieldScale sets how much of that near field the microphone picks up. It is
// fitted, not derived — the effective area of an evanescent patch is outside
// what this reduced model can compute — and it matters more than any other
// number here: in the far field a drum this size is nearly a monopole, so with
// the scale at zero the output is very nearly the axisymmetric modes alone.
type Pickup struct {
	Radius01       float64 `json:"radius01"`
	AngleRad       float64 `json:"angleRad"`
	DistanceM      float64 `json:"distanceM"`
	NearFieldScale float64 `json:"nearFieldScale"`
	HighpassHz     float64 `json:"highpassHz"`
	LowpassHz      float64 `json:"lowpassHz"`
	OutputGain     float64 `json:"outputGain"`
}

// DefaultContact returns the calibrated stick contact.
//
// Model is ContactPrescribed, which is the shipped sound and its known defect,
// not a judgement that it is the better of the two. Switching the default is a
// change to how the instrument sounds and needs its own re-fit; see
// docs/physical-contact.md for what ContactHertzian measures against it.
func DefaultContact() Contact {
	return Contact{
		Model: ContactPrescribed,
		// Chosen for the shipped 15 g mallet, against which it predicts a 7.4 ms
		// contact — inside the 5.5-8 ms Dahl and Wagner measure, and predicted
		// rather than prescribed.
		//
		// It is not fitted to that number, because it cannot be: the contact time
		// here is set by the head, not by the tip. Over the four decades from 1e4
		// to 3e6 the contact runs 14.5 ms down to 7.1 ms and then stops moving,
		// because the stick is riding the head's own return rather than
		// rebounding off the tip's compression. 1e6 sits on that plateau, where
		// the peak indentation is 0.4 mm — a plausible figure for a tip on a
		// coated head, and the reason to prefer it over any other point on a
		// plateau the sound cannot distinguish.
		StiffnessNPerMAlpha: 1e6,
		// Hertz's 3/2, which here is a measured number rather than an assumed
		// one: see the note on Contact.
		Exponent: 1.5,
		// Tip loss. It buys little here and that is itself informative: it takes
		// the rebound from 0.91 of the striking speed to 0.90, because the tip
		// barely compresses. Almost all of the bounce is the head's elasticity
		// rather than the tip's, which is the same fact as the contact time being
		// head-dominated, and it is why a stick can be pressed into a roll.
		//
		// Larger values were tried and are not admissible: past about 1/v the
		// Hunt-Crossley factor turns negative and the force is truncated to zero
		// mid-release, which puts a step into the pulse and a 46 dB notch at
		// 460 Hz back into its spectrum — reintroducing, by a different route,
		// exactly the defect this model removes.
		HysteresisSPerM: 0.3,
		// 20 ms, against a contact that ends by 8. It bounds the work per strike
		// and leaves room for a head that rises back into the stick; it does not
		// shape the result.
		MaxDurationSeconds: 0.02,
	}
}

// DefaultPhysicalDrum returns a conservative 12-inch double-headed tom
// configuration.
func DefaultPhysicalDrum() PhysicalDrum {
	head := Head{
		Enabled:               true,
		RadiusM:               0.1524,
		SurfaceDensityKgPerM2: 0.35,
		// 1250 N/m, not the 600 this shipped with. At 600 the 12-inch batter
		// head's fundamental is 104.00 Hz, which is a floor tom; the drum only
		// began to read as a rack tom near the old ceiling of 1400. This puts
		// the fundamental at 150.08 Hz, and the range around it now runs
		// 300-3500 N/m (75-251 Hz) so that pitch sits mid-travel instead of
		// against the stop. See RetuneTension for the other half of that fix.
		TensionNPerM: 1250,
		TensionAsymmetry: TensionAsymmetry{
			SplitRatio:            0.004,
			PrincipalAxisAngleRad: 0,
		},
		BendingStiffnessNM: 0.001,
		// ζ = 0.72 % at c = √(T/σ) = 59.76 m/s. Loss0 is a small floor rather
		// than the dominant term it used to be: it flattens ζ toward low
		// frequencies, which is the shape the measurements contradict.
		//
		// 0.72 %, not the 1.1 % this shipped with, because 0.72 % is what the
		// old coefficients happened to give at the old ceiling of 1400 N/m —
		// the tuning that sounded right. See RetuneTension: ζ used to drift
		// with the tuning knob, so "high B.TUNE sounds better" was partly a
		// report about decay length rather than pitch.
		Loss0PerSecond:         0.8,
		Loss1MPerSecond:        0.4303,
		Loss2M2PerSecond:       1.9e-5,
		RadiationLossPerSecond: 1.5,
		// The (0,1) loses energy fastest of all, into the cavity and the
		// opposite head. Held at the rate that keeps its T60 at 0.21 s, the
		// value the retuned default inherits from the point that sounded
		// right.
		ModeDecayCorrections: []ModeDecayCorrection{
			{AzimuthalOrder: 0, RadialOrder: 1, DecayRatePerSecond: 24.6},
		},
		FrequencyLimitFraction:   0.45,
		InactiveEnergyThresholdJ: 1e-12,
	}

	return PhysicalDrum{
		Version:           ConfigVersion,
		SampleRateHz:      48_000,
		Quality:           QualityStandard,
		ResonantModeLimit: DefaultResonantModeLimit,
		Batter:            head,
		Resonant: Head{
			Enabled: true,
			// Free: see the note on Head. The 44 oscillators this reclaims are
			// what pays for the batter head's wider band.
			AxisymmetricOnly:      true,
			RadiusM:               head.RadiusM,
			SurfaceDensityKgPerM2: 0.25,
			// Retuned by the same factor as the batter head, so the two heads
			// keep the relationship the cavity fit was made against.
			TensionNPerM: 1040,
			TensionAsymmetry: TensionAsymmetry{
				SplitRatio:            0.003,
				PrincipalAxisAngleRad: 0,
			},
			BendingStiffnessNM: 0.0007,
			// Same ζ, but the thinner, slacker head carries waves at
			// c = 64.50 m/s, so its k¹ coefficient and its (0,1) correction
			// both differ from the batter's.
			Loss0PerSecond:         1.0,
			Loss1MPerSecond:        0.4644,
			Loss2M2PerSecond:       1.9e-5,
			RadiationLossPerSecond: 1.5,
			ModeDecayCorrections: []ModeDecayCorrection{
				{AzimuthalOrder: 0, RadialOrder: 1, DecayRatePerSecond: 26.4},
			},
			FrequencyLimitFraction:   head.FrequencyLimitFraction,
			InactiveEnergyThresholdJ: head.InactiveEnergyThresholdJ,
		},
		Strike: Strike{
			// 0.30 of the radius, not the near-centre 0.12 it shipped with. A
			// centre hit excites the axisymmetric modes and very little else, so
			// it is a tuned thump rather than a tom; at 0.30 the (1,1) family is
			// properly excited (J1(1.15) = 0.47) while the fundamental still is
			// (J0(0.72) = 0.87). Off-centre is also where a tom is actually
			// struck.
			Radius01:       0.30,
			AngleRad:       0.2,
			ContactRadiusM: 0.01,
			MalletMassKg:   0.015,
			VelocityMPerS:  3,
			Hardness01:     0.7,
			Contact:        DefaultContact(),
		},
		Cavity: Cavity{
			Enabled:    true,
			DepthM:     0.20,
			Coupling01: 1,
			// Fitted, not derived. The rigid formula puts the stiffened (0,1)
			// branch at 219.5 Hz against an unstiffened 107.5 — a doublet ratio
			// of 2.04, where a measured drum separates its two (0,1) branches by
			// 10–20 %. This scale gives 106.6/124.2 Hz, a ratio of 1.16.
			StiffnessScale:    0.083,
			AirDensityKgPerM3: 1.204,
			SoundSpeedMPerS:   343,
			LossPerSecond:     5,
			// Six states: the uniform compliance, the (1,1) and (2,1)
			// transverse pairs, and the axisymmetric (0,1). On the shipped
			// 12-inch shell those sit at 0, 660, 1094 and 1373 Hz — the same
			// j'_11, j'_21, j'_01 series docs/physical-excitation-gap.md states
			// its hypothesis at, evaluated here at the shipped radius rather
			// than the reference recording's unknown one. The count stops there
			// because the next candidate, (3,1) at 1505 Hz, is above the top
			// retained head mode and costs two more states to reach nothing.
			// 1 reproduces the lumped model this replaced, exactly.
			ModeCount: 6,
		},
		Nonlinearity: Nonlinearity{
			Enabled: true,
			// Four times the coefficients this shipped with. Every published tom
			// analysis treats the downward glide as the characteristic feature,
			// and at the old value it was 38 cents on the loudest hit — real, but
			// below what anyone hears as a bend. Measured against the multiplier:
			//
			//	x1   37.9 cents loud, 1.5 quiet
			//	x2   65.7 cents loud, 2.3 quiet
			//	x4  102.8 cents loud, 3.0 quiet
			//	x8  135.7 cents loud, 7.5 quiet
			//	x16 152.0 cents loud, 14.3 quiet
			//
			// x4 is an audible semitone that still leaves the tanh cap well
			// clear: past x8 the loud hit sits on the plateau, which flattens the
			// glide into a hold-then-drop and erodes the velocity dependence that
			// makes it expressive. The discretization error against a 4x
			// oversampled reference goes 0.0773 % to 0.0833 % against a 1.5 %
			// ceiling, and the solve costs nothing extra — see the note in
			// nonlinear.go.
			BatterTensionCoefficientNPerM3:   9.6e6,
			ResonantTensionCoefficientNPerM3: 6.4e6,
			MaximumTensionRatio:              0.2,
		},
		Attack: Attack{
			Enabled: true,
			// Fitted by spectral balance, like the near-field scale. Measured in
			// the 43 ms attack window, against the strongest low partial:
			//
			//	level   1-2 kHz   2-5 kHz   5-10 kHz
			//	0        -66.9     -83.9     -99.8
			//	0.05     -38.3     -33.0     -33.9
			//	0.1      -32.3     -27.0     -27.9
			//	0.2      -26.3     -20.9     -21.9
			//
			// The first row is the defect: with modal synthesis alone there is
			// nothing above 1 kHz at all.
			//
			// Refitted for the three-band layer, which sums three envelopes where
			// there used to be one, and against the retuned head. Measured in the
			// 43 ms attack window, against the strongest low partial:
			//
			//	level   1-2 kHz   2-5 kHz   5-10 kHz   peak
			//	0        -45.5     -77.7     -94.5     0.435
			//	0.02     -37.9     -39.1     -40.6     0.503
			//	0.05     -31.6     -31.1     -32.7     0.616
			//	0.10     -26.1     -25.0     -26.6     0.803
			//
			// 0.05 is a stick clearly present and still well under the low band.
			LevelRelative: 0.05,
			// 4 kHz, not 3: the bands sit at 0.4, 1 and 2.5 times this, and at
			// 3 kHz the lowest of them landed at 1.2 kHz — *below* the top retained
			// mode, which the wider Standard budget now puts at 1310 Hz. Noise
			// where the model already has resolved modes is both a double count and
			// the wrong texture: that region is heard as pitch, not as hiss. At
			// 4 kHz the group spans 1.6-10 kHz and starts just above the modal
			// ceiling.
			CentreHz: 4000,
			// Broad on purpose. This band stands in for a dense thicket of
			// unresolved modes, so a resonant peak would be a worse lie than a
			// gentle hump. At Q = 0.7 each band is about an octave wide, so the
			// three of them tile the group's 1-8 kHz span without a gap.
			QualityFactor: 0.7,
			// 1: take the loss law at its word. The releases it produces are much
			// shorter than any resolved mode's, because what this layer stands for
			// genuinely decays faster — constant Q means the absolute rate rises
			// with frequency.
			DecayScale: 1,
		},
		Pickup: Pickup{
			// A tom microphone is a close one, a few centimetres off the head
			// and out toward the rim, and this model is unusually sensitive to
			// that: the far-field term alone leaves every m > 0 mode at least
			// 23 dB down, because a 12-inch head below 600 Hz really is nearly a
			// monopole. Fitted here, the partial structure is (0,1) 0 dB, (1,1)
			// -7.1 and -10.4, (0,2) -8.5, (2,1) -9.3 and -17.5, falling to
			// -34.5 dB at the top of the retained band.
			Radius01:       0.65,
			AngleRad:       0.6,
			DistanceM:      0.03,
			NearFieldScale: 1,
			HighpassHz:     35,
			LowpassHz:      12_000,
			// Fitted so a velocity-1 hit peaks at 0.9 with no compensating gain
			// in the voice, with the attack layer included. Small because the
			// radiated sum is now a volume acceleration in m³/s² rather than a
			// bare modal velocity.
			OutputGain: 0.0048,
		},
	}
}

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

	return nil
}

// EncodeConfig validates and serializes a physical-drum configuration.
func EncodeConfig(config PhysicalDrum) ([]byte, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	return json.Marshal(config)
}

// DecodeConfig decodes the current schema, migrates supported older schemas,
// and validates every field.
func DecodeConfig(data []byte) (PhysicalDrum, error) {
	var config PhysicalDrum
	if err := json.Unmarshal(data, &config); err != nil {
		return PhysicalDrum{}, fmt.Errorf("%w: decode: %v", ErrInvalidConfig, err)
	}

	if config.Version == legacyConfigVersion {
		migrateV1Config(&config)
	}

	if config.Version == linearDoubleHeadConfigVersion {
		migrateV2Config(&config)
	}

	if config.Version == nonlinearConfigVersion {
		migrateV3Config(&config)
	}

	if config.Version == fullCouplingConfigVersion {
		migrateV4Config(&config)
	}

	if config.Version == asymmetryConfigVersion {
		migrateV5Config(&config)
	}

	if config.Version == tiltedDampingConfigVersion {
		migrateV6Config(&config)
	}

	if config.Version == fittedCavityConfigVersion {
		migrateV7Config(&config)
	}

	if config.Version == radiatedAccelerationVersion {
		migrateV8Config(&config)
	}

	if config.Version == multiBandAttackConfigVersion {
		migrateV9Config(&config)
	}

	if config.Version == selectableContactVersion {
		migrateV10Config(&config)
	}

	if err := config.Validate(); err != nil {
		return PhysicalDrum{}, err
	}

	return config, nil
}

func migrateV1Config(config *PhysicalDrum) {
	defaults := DefaultPhysicalDrum()

	config.Version = linearDoubleHeadConfigVersion
	config.Batter.RadiationLossPerSecond = defaults.Batter.RadiationLossPerSecond
	config.Resonant.RadiationLossPerSecond = defaults.Resonant.RadiationLossPerSecond
	config.Pickup.DistanceM = defaults.Pickup.DistanceM
	config.Pickup.HighpassHz = defaults.Pickup.HighpassHz
	config.Pickup.LowpassHz = defaults.Pickup.LowpassHz

	// P1's scalar gain preceded distance attenuation and radiation filtering.
	// Move legacy configs onto the P2 output level.
	config.Pickup.OutputGain = defaults.Pickup.OutputGain
}

func migrateV2Config(config *PhysicalDrum) {
	config.Version = nonlinearConfigVersion
	// Version 2 was the linear double-head model. Preserve its sound exactly;
	// newly created version-3 configs opt into the nonlinear extension.
	config.Nonlinearity = Nonlinearity{}
}

func migrateV3Config(config *PhysicalDrum) {
	config.Version = fullCouplingConfigVersion
	// Version 3 coupled the full analytic swept head area into the cavity.
	config.Cavity.Coupling01 = 1
}

func migrateV4Config(config *PhysicalDrum) {
	config.Version = asymmetryConfigVersion
	// Version 4 was the ideal circular-head model. Zero-valued asymmetry is an
	// exact compatibility mode, so decoding an old configuration does not
	// introduce beating or rotate its mode shapes.
	config.Batter.TensionAsymmetry = TensionAsymmetry{}
	config.Resonant.TensionAsymmetry = TensionAsymmetry{}
}

func migrateV5Config(config *PhysicalDrum) {
	config.Version = tiltedDampingConfigVersion
	// Version 5 had no k¹ loss term, so its damping was flat in frequency and
	// its absent field decodes to the exact zero that reproduces it. Leaving it
	// there — and leaving the decay corrections alone — keeps an old
	// configuration sounding as it did, including its ringing fundamental.
	// Newly created version-6 configurations get the calibrated law from
	// DefaultPhysicalDrum instead.
}

func migrateV6Config(config *PhysicalDrum) {
	// Named, not ConfigVersion: assigning the latest here would let a version-6
	// document skip every migration added after this one, silently and without
	// failing this migration's own test.
	config.Version = fittedCavityConfigVersion
	// Version 6 derived the cavity stiffness from the rigid rho*c²/V, so the
	// unscaled value is what reproduces it. Unlike the other compatibility
	// migrations this one cannot rely on the zero value: an absent
	// stiffnessScale decodes to 0, which is the uncoupled limit rather than the
	// old sound, so it has to be written explicitly.
	config.Cavity.StiffnessScale = 1
}

// migrateV7Config carries a version-7 document onto the corrected microphone
// model. Unlike the migrations above it cannot promise the old sound: version 7
// summed modal velocity weighted by a far-field radiation efficiency times a
// near-field point mode shape, which is not a physical quantity and is not
// reconstructible from a scale factor. The radiated sum is corrected for old and
// new configurations alike; only the mixture of the two mechanisms is
// migratable, and zero — pure far field — is its exact absence.
func migrateV7Config(config *PhysicalDrum) {
	config.Version = radiatedAccelerationVersion
	// Version 7 had no separate near-field term, so its absent field decodes to
	// the zero that means "propagating part only". That is a meaningful setting
	// rather than a broken one: it is what a distant microphone hears.
	//
	// Its output gain, however, was fitted against the old sum and would be
	// roughly two orders of magnitude hot against this one, so it moves to the
	// calibrated default. Note this migration has no production caller —
	// EncodeConfig/DecodeConfig are used by tests, and the voice rebuilds from
	// DefaultPhysicalDrum on every edit — so it is for internal consistency, not
	// for user state.
	config.Pickup.OutputGain = DefaultPhysicalDrum().Pickup.OutputGain
}

// migrateV8Config carries a version-8 document onto the multi-band attack layer.
//
// Like the migration above it cannot promise the old sound, and here that is the
// point: version 8's attack was a single band held at an absolute 20 ms release,
// which is the defect this version exists to remove. Its decaySeconds has no
// image in a set of rates derived from the head's loss law, so the field is
// dropped and the scale that reads that law verbatim takes its place.
//
// The tuning defaults moved in the same version, but they are not migrated:
// tension, and the loss coefficients quoted against it, are the document's own
// measured content. A version-8 drum keeps the pitch it was saved with, and only
// new drums start from the retuned default.
func migrateV8Config(config *PhysicalDrum) {
	config.Version = multiBandAttackConfigVersion
	config.Attack.DecayScale = DefaultPhysicalDrum().Attack.DecayScale
}

// migrateV9Config carries a version-9 document onto the selectable contact
// model.
//
// This one *can* promise the old sound, and does: version 9 had only the
// prescribed half-sine, so naming it explicitly reproduces the document exactly.
// The Hertzian coefficients come along because the model is selectable at any
// time and a document with a zero stiffness would fail validation the moment it
// was switched — but they are inert until it is.
func migrateV9Config(config *PhysicalDrum) {
	// Named, not ConfigVersion: see the note on migrateV6Config.
	config.Version = selectableContactVersion
	config.Strike.Contact = DefaultContact()
	config.Strike.Contact.Model = ContactPrescribed
}

// migrateV10Config carries a version-10 document onto the modal cavity.
//
// This one can promise the old sound and does. Version 10's cavity was a single
// uniform-pressure state, which is exactly the m = 0, j' = 0 member of the modal
// basis: its coupling coefficient is the swept area, its natural frequency is
// zero, and the k x k midpoint elimination collapses to the same single division
// the rank-one form performed. So a count of 1 is a bit-exact reproduction rather
// than an approximation of one, and a regression test renders both to prove it.
//
// Neither new field can be left at its zero value: an absent modeCount decodes to
// 0, which is no cavity at all rather than the old one, and an absent
// resonantModeLimit decodes to a bank of no oscillators. Both are written
// explicitly. Newly created version-11 documents get the transverse modes from
// DefaultPhysicalDrum instead.
//
// The old sound survives the resonant cap too, and for a reason rather than by
// luck: with one uniform cavity state the reachable set is {0}, and the number of
// axisymmetric modes in a bank of N slots grows only as about (2/pi)*sqrt(N) — 4,
// 6 and 8 at the three tiers. DefaultResonantModeLimit is well above all of them,
// so the cap cannot bind on any document this migration can produce, and the
// render stays bit-identical rather than merely close.
func migrateV10Config(config *PhysicalDrum) {
	config.Version = ConfigVersion
	config.Cavity.ModeCount = 1
	config.ResonantModeLimit = DefaultResonantModeLimit
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
