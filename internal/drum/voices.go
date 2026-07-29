package drum

import (
	"math"
	"math/rand/v2"

	"github.com/cwbudde/algo-dsp/dsp/filter/biquad"
	"github.com/cwbudde/algo-dsp/dsp/filter/design"
)

// Voice is a single-shot drum synthesizer voice.
type Voice interface {
	// Trigger starts (or restarts) the voice; velocity in [0, 1] scales
	// the level of the whole hit.
	Trigger(velocity float64)
	Tick() float64
	IsActive() bool
	// SetDecay trims the voice's base decay time by decayScaleMin + amount.
	// The base itself is a synthesis parameter (see params.go), so the
	// effective decay is base × (decayScaleMin + amount).
	SetDecay(amount float64)
	// SetParam sets one synthesis parameter from a normalized [0, 1]
	// position; an out-of-range index or non-finite value is a no-op.
	SetParam(index int, value01 float64)
	Param(index int) float64
	ParamSpecs() []ParamSpec
}

// Shared voice tuning. These constants are the shipped defaults of the
// parameter table in params.go — changing one changes a default, not a
// hard-coded value in the signal path.
const (
	// envSilence is the envelope level below which a voice deactivates.
	// Deliberately not exposed as a parameter: raising it would stop a voice
	// ever deactivating, leaving it stuck and burning CPU forever.
	envSilence = 1e-4

	// decayScaleMin: SetDecay(amount) scales a voice's base decay time by
	// decayScaleMin + amount, i.e. 0.5×–1.5× of the base. Not exposed
	// either — it defines what the persisted setDecay byte means, so
	// changing it would silently reinterpret every existing share link.
	decayScaleMin = 0.5

	// pitchSweepRate shapes the exponential pitch drop of the tonal drums;
	// higher settles onto the target pitch faster. Bass and Tom each get
	// their own runtime copy so they can be swept independently.
	pitchSweepRate = 5.0
)

// Bass drum tuning.
const (
	bassPitchFromHz = 200.0 // pitch sweep start
	bassPitchToHz   = 50.0  // pitch sweep target
	bassPitchTCS    = 0.06  // pitch sweep time constant, seconds
	bassBaseDecayS  = 0.45  // base envelope decay, seconds
)

// Snare tuning.
const (
	snareToneHz     = 200.0  // body oscillator frequency
	snareToneLevel  = 0.7    // tone level relative to noise at trigger
	snareBaseToneS  = 0.12   // base tone decay, seconds
	snareBaseNoiseS = 0.18   // base noise decay, seconds
	snareHPHz       = 2000.0 // noise highpass cutoff
	snareHPQ        = 0.7
	snareSeed       = 42
)

// Hi-hat tuning (closed hat; an open-hat track is deferred, see PLAN.md G7).
const (
	hatBPHz       = 10000.0 // metallic bandpass center
	hatBPQ        = 2.0
	hatBaseDecayS = 0.04 // base envelope decay, seconds
	hatGain       = 1.5  // make-up gain after the bandpass
	hatSeed       = 123
)

// Tom tuning.
const (
	tomPitchFromHz = 120.0
	tomPitchToHz   = 60.0
	tomPitchTCS    = 0.1
	tomBaseDecayS  = 0.35
	tomGain        = 0.9
)

// Cymbal tuning.
const (
	cymBPHz       = 7000.0
	cymBPQ        = 1.2
	cymBaseDecayS = 1.2
	cymGain       = 1.2
	cymSeed       = 999
)

// clamp01 limits a voice-level value to [0, 1]. Non-finite input becomes 0:
// bare `<`/`>` comparisons are false for NaN, so it would otherwise pass
// straight through into an envelope and turn the voice permanently silent-but-
// NaN. Engine parameters are already rejected at the setters (validFloat);
// this is the same policy applied inside the voices.
func clamp01(v float64) float64 {
	val, ok := validFloat(v, 0, 1)
	if !ok {
		return 0
	}

	return val
}

func decayCoef(sr, decayS float64) float64 {
	if decayS < 0.005 {
		decayS = 0.005
	}

	return math.Exp(-1.0 / (sr * decayS))
}

// newVoiceRng returns a deterministic per-voice noise source; fixed seeds
// keep renders reproducible across runs.
func newVoiceRng(seed uint64) *rand.Rand {
	return rand.New(rand.NewPCG(seed, seed))
}

// ── Bass Drum ──────────────────────────────────────────────────────────────

type BassDrum struct {
	paramBank

	sr       float64
	active   bool
	age      int
	phase    float64
	env      float64
	envDecay float64

	// Derived from paramBank whenever a parameter changes; read per sample.
	pitchFrom  float64
	pitchTo    float64
	pitchTC    float64
	sweepRate  float64
	baseDecayS float64

	// decayAmount is the strip knob's [0, 1] position, kept so SetDecay and
	// the base-decay parameter can be recombined independently.
	decayAmount float64
}

func NewBassDrum(sr float64) *BassDrum {
	v := &BassDrum{
		paramBank:   newParamBank(bassSpecs),
		sr:          sr,
		decayAmount: 0.5,
	}
	v.applyParams()

	return v
}

func (v *BassDrum) Trigger(velocity float64) {
	v.active = true
	v.age = 0
	v.env = clamp01(velocity)
	v.phase = 0
}

func (v *BassDrum) IsActive() bool { return v.active }

func (v *BassDrum) SetDecay(amount float64) {
	v.decayAmount = clamp01(amount)
	v.applyDecay()
}

func (v *BassDrum) SetParam(index int, value01 float64) {
	if !v.set(index, value01) {
		return
	}

	v.applyParams()
}

// applyParams re-derives every cached engineering value. Recomputing all of
// them on any change costs a handful of flops on a knob move and keeps the
// per-voice code to one function instead of a switch per parameter.
func (v *BassDrum) applyParams() {
	v.pitchFrom = v.value(bassParamPitchFrom)
	v.pitchTo = v.value(bassParamPitchTo)
	v.pitchTC = v.value(bassParamSweepTime)
	v.sweepRate = v.value(bassParamSweepRate)
	v.baseDecayS = v.value(bassParamDecay)
	v.applyDecay()
}

func (v *BassDrum) applyDecay() {
	v.envDecay = decayCoef(v.sr, v.baseDecayS*(decayScaleMin+v.decayAmount))
}

func (v *BassDrum) Tick() float64 {
	if !v.active {
		return 0
	}

	// pitchFrom may sit below pitchTo once a user inverts the sweep; the
	// arithmetic handles it and the pitch simply rises instead of falling.
	t := float64(v.age) / (v.sr * v.pitchTC)
	freq := v.pitchTo + (v.pitchFrom-v.pitchTo)*math.Exp(-t*v.sweepRate)

	v.phase += 2 * math.Pi * freq / v.sr
	if v.phase > 2*math.Pi {
		v.phase -= 2 * math.Pi
	}

	sample := math.Sin(v.phase) * v.env

	v.env *= v.envDecay
	if v.env < envSilence {
		v.active = false
	}

	v.age++

	return sample
}

// ── Snare ──────────────────────────────────────────────────────────────────

type Snare struct {
	paramBank

	sr         float64
	active     bool
	phase      float64
	toneEnv    float64
	toneDecay  float64
	noiseEnv   float64
	noiseDecay float64
	hpFilter   biquad.Section
	rng        *rand.Rand

	toneHz     float64
	toneLevel  float64
	baseToneS  float64
	baseNoiseS float64

	decayAmount float64
}

func NewSnare(sr float64) *Snare {
	v := &Snare{
		paramBank:   newParamBank(snareSpecs),
		sr:          sr,
		rng:         newVoiceRng(snareSeed),
		decayAmount: 0.5,
	}
	v.applyParams()

	return v
}

func (v *Snare) Trigger(velocity float64) {
	vel := clamp01(velocity)
	v.active = true
	v.toneEnv = v.toneLevel * vel
	v.noiseEnv = vel
	v.phase = 0
	v.hpFilter.Reset()
}

func (v *Snare) IsActive() bool { return v.active }

func (v *Snare) SetDecay(amount float64) {
	v.decayAmount = clamp01(amount)
	v.applyDecay()
}

func (v *Snare) SetParam(index int, value01 float64) {
	if !v.set(index, value01) {
		return
	}

	v.applyParams()
}

func (v *Snare) applyParams() {
	v.toneHz = v.value(snareParamToneHz)
	v.toneLevel = v.value(snareParamToneLevel)
	v.baseToneS = v.value(snareParamToneDecay)
	v.baseNoiseS = v.value(snareParamNoiseDecay)
	v.applyDecay()
	v.redesign()
}

// applyDecay scales both layers by the same strip-knob factor, keeping the
// tone/noise relationship the voice was designed with.
func (v *Snare) applyDecay() {
	scale := decayScaleMin + v.decayAmount
	v.toneDecay = decayCoef(v.sr, v.baseToneS*scale)
	v.noiseDecay = decayCoef(v.sr, v.baseNoiseS*scale)
}

// redesign recomputes the highpass coefficients in place. See HiHat.redesign
// for why the filter state is deliberately left alone.
func (v *Snare) redesign() {
	hz := clampDesignHz(v.value(snareParamHPHz), v.sr)
	v.hpFilter.Coefficients = design.Highpass(hz, v.value(snareParamHPQ), v.sr)
}

func (v *Snare) Tick() float64 {
	if !v.active {
		return 0
	}

	v.phase += 2 * math.Pi * v.toneHz / v.sr
	if v.phase > 2*math.Pi {
		v.phase -= 2 * math.Pi
	}

	tone := math.Sin(v.phase) * v.toneEnv
	noise := (v.rng.Float64()*2 - 1) * v.noiseEnv
	noise = v.hpFilter.ProcessSample(noise)
	v.toneEnv *= v.toneDecay

	v.noiseEnv *= v.noiseDecay
	if v.toneEnv < envSilence && v.noiseEnv < envSilence {
		v.active = false
	}

	return tone + noise
}

// ── Hi-Hat ─────────────────────────────────────────────────────────────────

type HiHat struct {
	paramBank

	sr       float64
	active   bool
	env      float64
	envDecay float64
	bpFilter biquad.Section
	rng      *rand.Rand

	baseDecayS float64
	gain       float64

	decayAmount float64
}

func NewHiHat(sr float64) *HiHat {
	v := &HiHat{
		paramBank:   newParamBank(hatSpecs),
		sr:          sr,
		rng:         newVoiceRng(hatSeed),
		decayAmount: 0.5,
	}
	v.applyParams()

	return v
}

func (v *HiHat) Trigger(velocity float64) {
	v.active = true
	v.env = clamp01(velocity)
	v.bpFilter.Reset()
}

func (v *HiHat) IsActive() bool { return v.active }

func (v *HiHat) SetDecay(amount float64) {
	v.decayAmount = clamp01(amount)
	v.applyDecay()
}

func (v *HiHat) SetParam(index int, value01 float64) {
	if !v.set(index, value01) {
		return
	}

	v.applyParams()
}

func (v *HiHat) applyParams() {
	v.baseDecayS = v.value(hatParamDecay)
	v.gain = v.value(hatParamGain)
	v.applyDecay()
	v.redesign()
}

func (v *HiHat) applyDecay() {
	v.envDecay = decayCoef(v.sr, v.baseDecayS*(decayScaleMin+v.decayAmount))
}

// redesign recomputes the bandpass coefficients in place.
//
// Only the coefficients are replaced: biquad.Section embeds them alongside its
// delay line, so assigning to .Coefficients leaves the filter state intact and
// a hit already ringing keeps ringing through the change. Calling Reset() here
// would zero the delay line mid-tail — an audible click, and exactly what
// Trigger's Reset() is for.
func (v *HiHat) redesign() {
	hz := clampDesignHz(v.value(hatParamBPHz), v.sr)
	v.bpFilter.Coefficients = design.Bandpass(hz, v.value(hatParamBPQ), v.sr)
}

func (v *HiHat) Tick() float64 {
	if !v.active {
		return 0
	}

	noise := (v.rng.Float64()*2 - 1) * v.env
	sample := v.bpFilter.ProcessSample(noise)

	v.env *= v.envDecay
	if v.env < envSilence {
		v.active = false
	}

	return sample * v.gain
}

// ── Tom ────────────────────────────────────────────────────────────────────

type Tom struct {
	paramBank

	sr       float64
	active   bool
	age      int
	phase    float64
	env      float64
	envDecay float64

	pitchFrom  float64
	pitchTo    float64
	pitchTC    float64
	sweepRate  float64
	baseDecayS float64
	gain       float64

	decayAmount float64
}

func NewTom(sr float64) *Tom {
	v := &Tom{
		paramBank:   newParamBank(tomSpecs),
		sr:          sr,
		decayAmount: 0.5,
	}
	v.applyParams()

	return v
}

func (v *Tom) Trigger(velocity float64) {
	v.active = true
	v.age = 0
	v.env = clamp01(velocity)
	v.phase = 0
}

func (v *Tom) IsActive() bool { return v.active }

func (v *Tom) Reset() {
	v.active = false
	v.age = 0
	v.phase = 0
	v.env = 0
}

func (v *Tom) SetDecay(amount float64) {
	v.decayAmount = clamp01(amount)
	v.applyDecay()
}

func (v *Tom) SetParam(index int, value01 float64) {
	if !v.set(index, value01) {
		return
	}

	v.applyParams()
}

func (v *Tom) applyParams() {
	v.pitchFrom = v.value(tomParamPitchFrom)
	v.pitchTo = v.value(tomParamPitchTo)
	v.pitchTC = v.value(tomParamSweepTime)
	v.sweepRate = v.value(tomParamSweepRate)
	v.baseDecayS = v.value(tomParamDecay)
	v.gain = v.value(tomParamGain)
	v.applyDecay()
}

func (v *Tom) applyDecay() {
	v.envDecay = decayCoef(v.sr, v.baseDecayS*(decayScaleMin+v.decayAmount))
}

func (v *Tom) Tick() float64 {
	if !v.active {
		return 0
	}

	t := float64(v.age) / (v.sr * v.pitchTC)
	freq := v.pitchTo + (v.pitchFrom-v.pitchTo)*math.Exp(-t*v.sweepRate)

	v.phase += 2 * math.Pi * freq / v.sr
	if v.phase > 2*math.Pi {
		v.phase -= 2 * math.Pi
	}

	sample := math.Sin(v.phase) * v.env

	v.env *= v.envDecay
	if v.env < envSilence {
		v.active = false
	}

	v.age++

	return sample * v.gain
}

// ── Cymbal ─────────────────────────────────────────────────────────────────

type Cymbal struct {
	paramBank

	sr       float64
	active   bool
	env      float64
	envDecay float64
	bpFilter biquad.Section
	rng      *rand.Rand

	baseDecayS float64
	gain       float64

	decayAmount float64
}

func NewCymbal(sr float64) *Cymbal {
	v := &Cymbal{
		paramBank:   newParamBank(cymSpecs),
		sr:          sr,
		rng:         newVoiceRng(cymSeed),
		decayAmount: 0.5,
	}
	v.applyParams()

	return v
}

func (v *Cymbal) Trigger(velocity float64) {
	v.active = true
	v.env = clamp01(velocity)
	v.bpFilter.Reset()
}

func (v *Cymbal) IsActive() bool { return v.active }

func (v *Cymbal) SetDecay(amount float64) {
	v.decayAmount = clamp01(amount)
	v.applyDecay()
}

func (v *Cymbal) SetParam(index int, value01 float64) {
	if !v.set(index, value01) {
		return
	}

	v.applyParams()
}

func (v *Cymbal) applyParams() {
	v.baseDecayS = v.value(cymParamDecay)
	v.gain = v.value(cymParamGain)
	v.applyDecay()
	v.redesign()
}

func (v *Cymbal) applyDecay() {
	v.envDecay = decayCoef(v.sr, v.baseDecayS*(decayScaleMin+v.decayAmount))
}

// redesign recomputes the bandpass coefficients in place; see HiHat.redesign.
func (v *Cymbal) redesign() {
	hz := clampDesignHz(v.value(cymParamBPHz), v.sr)
	v.bpFilter.Coefficients = design.Bandpass(hz, v.value(cymParamBPQ), v.sr)
}

func (v *Cymbal) Tick() float64 {
	if !v.active {
		return 0
	}

	noise := (v.rng.Float64()*2 - 1) * v.env
	sample := v.bpFilter.ProcessSample(noise)

	v.env *= v.envDecay
	if v.env < envSilence {
		v.active = false
	}

	return sample * v.gain
}
