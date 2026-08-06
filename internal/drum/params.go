package drum

// Per-voice synthesis parameters (PLAN.md G20).
//
// Every tuning value in voices.go used to be a compile-time const. This file
// turns them into runtime parameters addressed by (track, index) and
// normalized to [0, 1], so the UI can drive them with the same Knob it uses
// for everything else and the same one-byte-per-scalar persistence format.
//
// The const blocks in voices.go stay: they are now the specs' Shipped values,
// which is exactly what pins today's sound as the default.

import "github.com/cwbudde/algo-tom/tomparams"

// maxVoiceParams bounds a voice's parameter list so the per-voice store is a
// fixed array (Render must not allocate, see TestHumanizeRenderDoesNotAllocate)
// and the persisted record has a fixed width. The widest voice is the Snare.
const maxVoiceParams = 6

// The spec type and its normalized→engineering mapping live in
// github.com/cwbudde/algo-tom/tomparams, because the physical Tom's mapping had
// to move there — an offline fitter that reused a copy would measure a different
// instrument than the one that ships — and the five procedural voices are
// described by the very same curve machinery.
//
// These are deliberately *aliases*, not defined types. A defined type would give
// this package its own Map, its own byte-step snap and its own Default
// derivation, and a drift between the two copies would silently retune the
// shipped sound with nothing but ears to catch it. As aliases there is exactly
// one of each in existence, cmd/gen-voiceparams needs no source change, and
// web/src/engine/voiceParams.generated.ts comes out byte-identical.
type ParamSpec = tomparams.Spec

const (
	paramExp = tomparams.KindExp

	// byteStep is one step of the UI's 8-bit persistence quantisation. Map
	// snaps to Shipped within half a step of Default; see tomparams.Spec.Map.
	byteStep = tomparams.ByteStep
)

// tomparams.Choice has no wrapper here on purpose: the physical Tom's QUAL
// selector was the only discrete parameter this package ever built, and it moved
// with the rest of the bank. A voice that needs one should call tomparams.Choice
// directly rather than reviving a forwarder for a single caller.

// expSpec builds an exponentially mapped parameter. Frequencies and times are
// always exponential: the ear hears ratios, not differences.
func expSpec(id, label, name, unit string, minVal, maxVal, shipped float64, digits int) ParamSpec {
	return tomparams.Exp(id, label, name, unit, minVal, maxVal, shipped, digits)
}

// linSpec builds a linearly mapped parameter, used for levels and mixes.
func linSpec(id, label, name, unit string, minVal, maxVal, shipped float64, digits int) ParamSpec {
	return tomparams.Lin(id, label, name, unit, minVal, maxVal, shipped, digits)
}

// paramBank is the shared per-voice parameter store, embedded by every voice.
// It supplies Param and ParamSpecs; each voice implements only SetParam, which
// is the genuinely heterogeneous part.
type paramBank struct {
	specs []ParamSpec
	vals  []float64
}

func newParamBank(specs []ParamSpec) paramBank {
	bank := paramBank{
		specs: specs,
		vals:  make([]float64, len(specs)),
	}
	for i, spec := range specs {
		bank.vals[i] = spec.Default
	}

	return bank
}

func defaultParams(specs []ParamSpec) []float64 {
	values := make([]float64, len(specs))
	for index, spec := range specs {
		values[index] = spec.Default
	}

	return values
}

// ParamSpecs returns the voice's parameter descriptors, in index order.
func (b *paramBank) ParamSpecs() []ParamSpec { return b.specs }

// Param returns the normalized position of one parameter, or 0 for an
// out-of-range index.
func (b *paramBank) Param(index int) float64 {
	if index < 0 || index >= len(b.specs) {
		return 0
	}

	return b.vals[index]
}

// set stores a normalized position, reporting whether the caller must
// re-derive its cached values. An out-of-range index or a non-finite value is
// a silent no-op — the same contract as every other indexed setter (SetCell).
func (b *paramBank) set(index int, value01 float64) bool {
	if index < 0 || index >= len(b.specs) {
		return false
	}

	val, ok := validFloat(value01, 0, 1)
	if !ok {
		return false
	}

	b.vals[index] = val

	return true
}

// value returns one parameter in engineering units.
func (b *paramBank) value(index int) float64 {
	return b.specs[index].Map(b.vals[index])
}

// clampDesignHz keeps a filter frequency inside the range design.Bandpass and
// design.Highpass are defined over: they return all-zero coefficients — a
// silent voice — at or above Nyquist. At the app's 48 kHz this never binds,
// but the engine accepts rates down to 8 kHz, where even the shipped 10 kHz
// hi-hat centre is already above Nyquist.
func clampDesignHz(freq, sampleRate float64) float64 {
	maxHz := 0.45 * sampleRate
	if freq > maxHz {
		return maxHz
	}

	if freq < 10 {
		return 10
	}

	return freq
}

// ── Per-voice parameter tables ─────────────────────────────────────────────
//
// Index constants are the persistence addresses: append only, never reorder.

const (
	bassParamPitchFrom = iota
	bassParamPitchTo
	bassParamSweepTime
	bassParamSweepRate
	bassParamDecay
)

var bassSpecs = []ParamSpec{
	bassParamPitchFrom: expSpec("bass.pitchFrom", "ATK", "attack pitch", "Hz", 60, 800, bassPitchFromHz, 0),
	bassParamPitchTo:   expSpec("bass.pitchTo", "TUNE", "body pitch", "Hz", 25, 120, bassPitchToHz, 1),
	bassParamSweepTime: expSpec("bass.sweepTime", "SWP", "pitch sweep time", "s", 0.005, 0.5, bassPitchTCS, 3),
	bassParamSweepRate: expSpec("bass.sweepRate", "SNAP", "pitch sweep rate", "", 1, 20, pitchSweepRate, 2),
	bassParamDecay:     expSpec("bass.decay", "TIME", "base decay time", "s", 0.05, 2.0, bassBaseDecayS, 3),
}

const (
	snareParamToneHz = iota
	snareParamToneLevel
	snareParamToneDecay
	snareParamNoiseDecay
	snareParamHPHz
	snareParamHPQ
)

var snareSpecs = []ParamSpec{
	snareParamToneHz:     expSpec("snare.toneHz", "BODY", "body pitch", "Hz", 100, 500, snareToneHz, 0),
	snareParamToneLevel:  linSpec("snare.toneLevel", "MIX", "body level", "", 0, 1, snareToneLevel, 2),
	snareParamToneDecay:  expSpec("snare.toneDecay", "B.DEC", "body decay time", "s", 0.02, 1.0, snareBaseToneS, 3),
	snareParamNoiseDecay: expSpec("snare.noiseDecay", "S.DEC", "snap decay time", "s", 0.02, 1.5, snareBaseNoiseS, 3),
	snareParamHPHz:       expSpec("snare.hpHz", "SNAP", "snap highpass", "Hz", 200, 8000, snareHPHz, 0),
	snareParamHPQ:        expSpec("snare.hpQ", "RES", "snap resonance", "", 0.3, 4, snareHPQ, 2),
}

const (
	hatParamBPHz = iota
	hatParamBPQ
	hatParamDecay
	hatParamGain
)

var hatSpecs = []ParamSpec{
	hatParamBPHz:  expSpec("hat.bpHz", "TONE", "bandpass centre", "Hz", 2000, 16000, hatBPHz, 0),
	hatParamBPQ:   expSpec("hat.bpQ", "RES", "bandpass resonance", "", 0.5, 8, hatBPQ, 2),
	hatParamDecay: expSpec("hat.decay", "TIME", "base decay time", "s", 0.005, 0.4, hatBaseDecayS, 3),
	hatParamGain:  linSpec("hat.gain", "LVL", "make-up gain", "", 0, 2.5, hatGain, 2),
}

const (
	tomParamPitchFrom = iota
	tomParamPitchTo
	tomParamSweepTime
	tomParamSweepRate
	tomParamDecay
	tomParamGain
)

var tomSpecs = []ParamSpec{
	tomParamPitchFrom: expSpec("tom.pitchFrom", "ATK", "attack pitch", "Hz", 60, 600, tomPitchFromHz, 0),
	tomParamPitchTo:   expSpec("tom.pitchTo", "TUNE", "body pitch", "Hz", 30, 300, tomPitchToHz, 1),
	tomParamSweepTime: expSpec("tom.sweepTime", "SWP", "pitch sweep time", "s", 0.005, 0.5, tomPitchTCS, 3),
	tomParamSweepRate: expSpec("tom.sweepRate", "SNAP", "pitch sweep rate", "", 1, 20, pitchSweepRate, 2),
	tomParamDecay:     expSpec("tom.decay", "TIME", "base decay time", "s", 0.05, 2.0, tomBaseDecayS, 3),
	tomParamGain:      linSpec("tom.gain", "LVL", "output level", "", 0, 2, tomGain, 2),
}

var tom2Specs = []ParamSpec{
	tomParamPitchFrom: expSpec("tom2.pitchFrom", "ATK", "attack pitch", "Hz", 60, 600, tom2PitchFromHz, 0),
	tomParamPitchTo:   expSpec("tom2.pitchTo", "TUNE", "body pitch", "Hz", 30, 300, tom2PitchToHz, 1),
	tomParamSweepTime: expSpec("tom2.sweepTime", "SWP", "pitch sweep time", "s", 0.005, 0.5, tom2PitchTCS, 3),
	tomParamSweepRate: expSpec("tom2.sweepRate", "SNAP", "pitch sweep rate", "", 1, 20, pitchSweepRate, 2),
	tomParamDecay:     expSpec("tom2.decay", "TIME", "base decay time", "s", 0.05, 2.0, tom2BaseDecayS, 3),
	tomParamGain:      linSpec("tom2.gain", "LVL", "output level", "", 0, 2, tom2Gain, 2),
}

const (
	cymParamBPHz = iota
	cymParamBPQ
	cymParamDecay
	cymParamGain
)

var cymSpecs = []ParamSpec{
	cymParamBPHz:  expSpec("cym.bpHz", "TONE", "bandpass centre", "Hz", 1000, 14000, cymBPHz, 0),
	cymParamBPQ:   expSpec("cym.bpQ", "RES", "bandpass resonance", "", 0.3, 6, cymBPQ, 2),
	cymParamDecay: expSpec("cym.decay", "TIME", "base decay time", "s", 0.1, 4.0, cymBaseDecayS, 3),
	cymParamGain:  linSpec("cym.gain", "LVL", "make-up gain", "", 0, 2, cymGain, 2),
}

const (
	percParamPitch = iota
	percParamRatio
	percParamDecay
	percParamClick
	percParamGain
)

var percSpecs = []ParamSpec{
	percParamPitch: expSpec("perc.pitch", "TUNE", "body pitch", "Hz", 120, 1600, percPitchHz, 0),
	percParamRatio: expSpec("perc.ratio", "METAL", "oscillator ratio", "", 1.05, 3.0, percRatio, 2),
	percParamDecay: expSpec("perc.decay", "TIME", "base decay time", "s", 0.02, 1.0, percBaseDecay, 3),
	percParamClick: linSpec("perc.click", "CLICK", "attack noise", "", 0, 1, percClick, 2),
	percParamGain:  linSpec("perc.gain", "LVL", "output level", "", 0, 2, percGain, 2),
}

// The physical Tom's parameter table and index constants come from tomparams,
// which owns them along with the normalized→SI mapping they address. They are
// bound to local names here so the rest of this package — and its tests — read
// the same way as the procedural voices around them.
//
// The indices are persistence and WASM command addresses, so params_test.go
// pins both the width and the ID list. Those assertions now guard an *imported*
// table, which is the point: they are what would catch algo-tom reordering the
// bank underneath a stored preset.
var physicalTomSpecs = tomparams.Specs()

const (
	physicalTomParamDiameter        = tomparams.ParamDiameter
	physicalTomParamBatterTension   = tomparams.ParamBatterTension
	physicalTomParamResonantTension = tomparams.ParamResonantTension
	physicalTomParamDamping         = tomparams.ParamDamping
	physicalTomParamStrikeRadius    = tomparams.ParamStrikeRadius
	physicalTomParamStrikeAngle     = tomparams.ParamStrikeAngle
	physicalTomParamHardness        = tomparams.ParamHardness
	physicalTomParamShellDepth      = tomparams.ParamShellDepth
	physicalTomParamCavityCoupling  = tomparams.ParamCavityCoupling
	physicalTomParamNonlinearity    = tomparams.ParamNonlinearity
	physicalTomParamPickupRadius    = tomparams.ParamPickupRadius
	physicalTomParamPickupAngle     = tomparams.ParamPickupAngle
	physicalTomParamQuality         = tomparams.ParamQuality
	physicalTomParamAsymmetry       = tomparams.ParamAsymmetry
	physicalTomParamAsymmetryAxis   = tomparams.ParamAsymmetryAxis
	physicalTomParamDampingTilt     = tomparams.ParamDampingTilt
	physicalTomParamAttackLevel     = tomparams.ParamAttackLevel
	physicalTomParamAttackTone      = tomparams.ParamAttackTone
)

// voiceNames labels each track for the editor UI, in engine track order.
var voiceNames = [TrackCount]string{
	"Bass Drum", "Snare", "Hi-Hat", "Tom", "Cymbal", "Tom 2", "Percussion",
}

// voiceSpecs holds every voice's parameter table, in engine track order.
var voiceSpecs = [TrackCount][]ParamSpec{
	bassSpecs, snareSpecs, hatSpecs, tomSpecs, cymSpecs, tom2Specs, percSpecs,
}

// SpecsForTrack returns the parameter descriptors of one voice, or nil for an
// out-of-range track. Used by cmd/gen-voiceparams to generate the TypeScript
// mirror the UI renders from.
func SpecsForTrack(track int) []ParamSpec {
	if !validTrack(track) {
		return nil
	}

	return voiceSpecs[track]
}

// PhysicalTomSpecs returns the generated descriptor source for the physical
// Tom editor. Its indices are stable persistence and WASM command addresses.
func PhysicalTomSpecs() []ParamSpec {
	return physicalTomSpecs
}

// VoiceName returns the display name of one voice, or "" for a bad track.
func VoiceName(track int) string {
	if !validTrack(track) {
		return ""
	}

	return voiceNames[track]
}
