package physical

import "math"

// The two level ceilings, named because they are the two this repository has
// admitted it never measured.
//
// Everything else in configBounds is a physical plausibility range — a head
// radius between 2 cm and 1 m, a speed of sound between 250 and 400 m/s — where
// the number carries its own justification. These two do not: they stand at
// roughly four orders of magnitude above the only values the product can
// produce (a knob maximum of 0.15 for the attack level, and a calibrated 0.0048
// output gain that no knob touches at all), which is headroom chosen rather
// than a bound derived. Naming them is the first step to deriving them; see
// PLAN.md N12b.
const (
	attackLevelRelativeCeiling = 1_000.0
	pickupOutputGainCeiling    = 100.0
)

// configBounds is the one place a validated *constant* range is written down.
//
// It exists because the range and the sweep that probes it used to be two
// separate copies of the same numbers. TestValidatedEndpointsRenderFinite drives
// every field to both ends of its validated range and skips endpoints Validate
// rejects — silently — so tightening a ceiling in validate.go without editing
// the test's own copy deleted that endpoint's coverage and left the test green.
// That is the failure shape N17 fixed for analysisSeconds/-duration, and for the
// same reason: a bound written twice is a bound that will eventually be written
// differently.
//
// Only ranges whose ends are *constants* live here. Several validated ranges are
// derived from the configuration under test, and those cannot desync in the same
// way because there is no second literal to drift from; they stay inline at
// their call sites and derivedBoundFields records that the omission was a
// decision rather than an oversight.
//
// The head entries are keyed once under "head." and checked against both the
// batter and the resonant head, which is why the key and the error's field name
// are separate arguments below.
var configBounds = map[string]struct{ low, high float64 }{
	"sampleRateHz": {minSampleRateHz, maxSampleRateHz},

	"strike.radius01":      {0, 1},
	"strike.angleRad":      {-2 * math.Pi, 2 * math.Pi},
	"strike.malletMassKg":  {1e-4, 1},
	"strike.velocityMPerS": {0, 20},
	"strike.hardness01":    {0, 1},

	"strike.contact.stiffnessNPerMAlpha": {1, 1e12},
	"strike.contact.exponent":            {1, 4},
	"strike.contact.hysteresisSPerM":     {0, 1},
	"strike.contact.maxDurationSeconds":  {1e-4, 0.5},

	"cavity.depthM":            {0.01, 2},
	"cavity.coupling01":        {0, 1},
	"cavity.stiffnessScale":    {0, 1},
	"cavity.airDensityKgPerM3": {0.5, 2},
	"cavity.soundSpeedMPerS":   {250, 400},
	"cavity.lossPerSecond":     {0, 10_000},

	"pickup.radius01":       {0, 1},
	"pickup.angleRad":       {-2 * math.Pi, 2 * math.Pi},
	"pickup.nearFieldScale": {0, 10},
	"pickup.distanceM":      {0.01, 10},
	"pickup.highpassHz":     {1, maxSampleRateHz / 2},
	"pickup.outputGain":     {0, pickupOutputGainCeiling},

	"head.radiusM":                                {0.02, 1},
	"head.surfaceDensityKgPerM2":                  {0.01, 10},
	"head.tensionNPerM":                           {1, 100_000},
	"head.bendingStiffnessNM":                     {0, 100},
	"head.loss0PerSecond":                         {0, 10_000},
	"head.loss1MPerSecond":                        {0, 1_000},
	"head.loss2M2PerSecond":                       {0, 10},
	"head.radiationLossPerSecond":                 {0, 10_000},
	"head.frequencyLimitFraction":                 {0.05, 0.49},
	"head.inactiveEnergyThresholdJ":               {0, 1},
	"head.tensionAsymmetry.splitRatio":            {0, 0.02},
	"head.tensionAsymmetry.principalAxisAngleRad": {-math.Pi, math.Pi},
	"head.modeDecayCorrection":                    {-10_000, 10_000},

	"nonlinearity.batterTensionCoefficientNPerM3":   {0, 1e9},
	"nonlinearity.resonantTensionCoefficientNPerM3": {0, 1e9},
	"nonlinearity.maximumTensionRatio":              {0, 1},

	"nonlinearity.coupling.coefficientNPerM": {0, maxCouplingCoefficientNPerM},
	"nonlinearity.coupling.aliasFraction":    {math.SmallestNonzeroFloat64, 0.5},

	"attack.levelRelative": {0, attackLevelRelativeCeiling},
	"attack.qualityFactor": {0.1, 20},
	"attack.decayScale":    {0, 100},
}

// derivedBoundFields names the validated ranges deliberately left out of
// configBounds because at least one end is computed from the configuration being
// validated. Listed rather than merely absent, so that "it is not in the table"
// is something someone decided and not a gap.
var derivedBoundFields = []string{
	"strike.contactRadiusM",                    // ceiling is Batter.RadiusM/2
	"pickup.lowpassHz",                         // floor is Pickup.HighpassHz
	"attack.centreHz",                          // ceiling is SampleRateHz/2
	"nonlinearity.coupling.pumpMaxFrequencyHz", // ceiling is a fraction of SampleRateHz
}

// boundOf returns a field's validated range. It panics on an unknown key rather
// than returning a zero range, because a zero range accepts almost nothing and
// would quietly turn a typo into a validator that rejects every configuration.
func boundOf(key string) (low, high float64) {
	bound, ok := configBounds[key]
	if !ok {
		panic("physical: no validated bound named " + key)
	}

	return bound.low, bound.high
}

// boundedRange checks value against the range recorded under key, reporting it
// as field. The two differ only for the head entries, which one table row
// covers for both heads.
func boundedRange(key, field string, value float64) error {
	low, high := boundOf(key)

	return finiteRange(field, value, low, high)
}
