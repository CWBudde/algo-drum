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

import "math"

// maxVoiceParams bounds a voice's parameter list so the per-voice store is a
// fixed array (Render must not allocate, see TestHumanizeRenderDoesNotAllocate)
// and the persisted record has a fixed width. The widest voice is the Snare.
const maxVoiceParams = 6

// byteStep is one step of the UI's 8-bit persistence quantisation. Map snaps
// to Shipped within half a step of Default; see Map.
const byteStep = 1.0 / 255.0

type paramKind uint8

const (
	paramLinear paramKind = iota
	paramExp
)

func (k paramKind) String() string {
	if k == paramExp {
		return "exp"
	}

	return "lin"
}

// ParamSpec describes one tweakable synthesis parameter.
//
// Shipped is the constant the voice has always used and Default is its
// normalized position, derived by inverting the curve rather than typed by
// hand — so the table cannot drift from the sound it describes.
type ParamSpec struct {
	ID      string // stable persistence key; append-only, never reordered
	Label   string // knob face, kept short
	Name    string // accessible-name fragment, e.g. "pitch sweep start"
	Unit    string // "Hz", "s", "" …
	Choices []string
	Kind    paramKind
	Min     float64
	Max     float64
	Shipped float64
	Default float64
	Digits  int // display precision
}

// Map converts a normalized knob position to engineering units.
//
// Within half a persistence byte step of Default it returns Shipped exactly.
// Persistence quantises every scalar to one byte, so a default of 0.4648
// round-trips as 0.4667 — without this snap, saving and reloading would
// retune a 200 Hz body to 205 Hz on every untouched voice. The dead zone is
// ±0.2 %, i.e. sub-pixel on the Knob's 150 px full sweep, and reads as a
// detent at the default position.
func (s ParamSpec) Map(value01 float64) float64 {
	val := clamp01(value01)

	if math.Abs(val-s.Default) < byteStep/2 {
		return s.Shipped
	}

	if len(s.Choices) > 0 {
		return math.Round(val * float64(len(s.Choices)-1))
	}

	if s.Kind == paramExp {
		return s.Min * math.Pow(s.Max/s.Min, val)
	}

	return s.Min + (s.Max-s.Min)*val
}

// choiceSpec builds a discrete selector rendered by the same normalized Knob
// as continuous parameters. Shipped is the zero-based selected choice.
func choiceSpec(id, label, name string, choices []string, shipped int) ParamSpec {
	return ParamSpec{
		ID: id, Label: label, Name: name, Choices: choices,
		Kind: paramLinear, Min: 0, Max: float64(len(choices) - 1),
		Shipped: float64(shipped), Default: float64(shipped) / float64(len(choices)-1),
	}
}

// expSpec builds an exponentially mapped parameter. Frequencies and times are
// always exponential: the ear hears ratios, not differences.
func expSpec(id, label, name, unit string, minVal, maxVal, shipped float64, digits int) ParamSpec {
	return ParamSpec{
		ID: id, Label: label, Name: name, Unit: unit,
		Kind: paramExp, Min: minVal, Max: maxVal, Shipped: shipped,
		Default: math.Log(shipped/minVal) / math.Log(maxVal/minVal),
		Digits:  digits,
	}
}

// linSpec builds a linearly mapped parameter, used for levels and mixes.
func linSpec(id, label, name, unit string, minVal, maxVal, shipped float64, digits int) ParamSpec {
	return ParamSpec{
		ID: id, Label: label, Name: name, Unit: unit,
		Kind: paramLinear, Min: minVal, Max: maxVal, Shipped: shipped,
		Default: (shipped - minVal) / (maxVal - minVal),
		Digits:  digits,
	}
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

// Physical Tom parameters use their own persistence bank, separate from the
// procedural Tom table above. The application can therefore A/B models
// without one model reinterpreting or overwriting the other's settings.
const (
	physicalTomParamDiameter = iota
	physicalTomParamBatterTension
	physicalTomParamResonantTension
	physicalTomParamDamping
	physicalTomParamStrikeRadius
	physicalTomParamStrikeAngle
	physicalTomParamHardness
	physicalTomParamShellDepth
	physicalTomParamCavityCoupling
	physicalTomParamNonlinearity
	physicalTomParamPickupRadius
	physicalTomParamPickupAngle
	physicalTomParamQuality
)

var physicalTomSpecs = []ParamSpec{
	physicalTomParamDiameter:         expSpec("physicalTom.diameter", "SIZE", "head diameter", "m", 0.16, 0.50, 0.3048, 3),
	physicalTomParamBatterTension:    expSpec("physicalTom.batterTension", "B.TUNE", "batter head tension", "N/m", 150, 1400, 600, 0),
	physicalTomParamResonantTension:  expSpec("physicalTom.resonantTension", "R.TUNE", "resonant head tension", "N/m", 150, 1400, 500, 0),
	physicalTomParamDamping:          expSpec("physicalTom.damping", "DAMP", "head damping", "/s", 0.75, 12, 3, 2),
	physicalTomParamStrikeRadius:     linSpec("physicalTom.strikeRadius", "HIT.R", "strike radius", "", 0, 0.95, 0.45, 2),
	physicalTomParamStrikeAngle:      linSpec("physicalTom.strikeAngle", "HIT.A", "strike angle", "°", -180, 180, 0.2*180/math.Pi, 0),
	physicalTomParamHardness:         linSpec("physicalTom.hardness", "HARD", "mallet hardness", "", 0, 1, 0.7, 2),
	physicalTomParamShellDepth:       expSpec("physicalTom.shellDepth", "DEPTH", "shell depth", "m", 0.05, 0.60, 0.20, 3),
	physicalTomParamCavityCoupling:   linSpec("physicalTom.cavityCoupling", "AIR", "cavity coupling", "", 0, 1, 1, 2),
	physicalTomParamNonlinearity:     linSpec("physicalTom.nonlinearity", "NLIN", "nonlinear tension amount", "", 0, 2, 1, 2),
	physicalTomParamPickupRadius:     linSpec("physicalTom.pickupRadius", "MIC.R", "pickup radius", "", 0, 0.95, 0.32, 2),
	physicalTomParamPickupAngle:      linSpec("physicalTom.pickupAngle", "MIC.A", "pickup angle", "°", -180, 180, 0.6*180/math.Pi, 0),
	physicalTomParamQuality:          choiceSpec("physicalTom.quality", "QUAL", "quality tier", []string{"Draft", "Standard", "High"}, 1),
}

// voiceNames labels each track for the editor UI, in engine track order.
var voiceNames = [TrackCount]string{"Bass Drum", "Snare", "Hi-Hat", "Tom", "Cymbal"}

// voiceSpecs holds every voice's parameter table, in engine track order.
var voiceSpecs = [TrackCount][]ParamSpec{
	bassSpecs, snareSpecs, hatSpecs, tomSpecs, cymSpecs,
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
