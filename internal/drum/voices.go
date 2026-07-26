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
	SetDecay(amount float64)
}

// Shared voice tuning.
const (
	// envSilence is the envelope level below which a voice deactivates.
	envSilence = 1e-4

	// decayScaleMin: SetDecay(amount) scales a voice's base decay time by
	// decayScaleMin + amount, i.e. 0.5×–1.5× of the base.
	decayScaleMin = 0.5

	// pitchSweepRate shapes the exponential pitch drop of the tonal drums;
	// higher settles onto the target pitch faster.
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
	sr       float64
	active   bool
	age      int
	phase    float64
	env      float64
	envDecay float64
}

func NewBassDrum(sr float64) *BassDrum {
	v := &BassDrum{sr: sr}
	v.SetDecay(0.5)

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
	scale := decayScaleMin + clamp01(amount)
	v.envDecay = decayCoef(v.sr, bassBaseDecayS*scale)
}

func (v *BassDrum) Tick() float64 {
	if !v.active {
		return 0
	}

	t := float64(v.age) / (v.sr * bassPitchTCS)
	freq := bassPitchToHz + (bassPitchFromHz-bassPitchToHz)*math.Exp(-t*pitchSweepRate)

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
	sr         float64
	active     bool
	phase      float64
	toneEnv    float64
	toneDecay  float64
	noiseEnv   float64
	noiseDecay float64
	hpFilter   biquad.Section
	rng        *rand.Rand
}

func NewSnare(sr float64) *Snare {
	hpCoeffs := design.Highpass(snareHPHz, snareHPQ, sr)

	v := &Snare{
		sr:       sr,
		hpFilter: *biquad.NewSection(hpCoeffs),
		rng:      newVoiceRng(snareSeed),
	}
	v.SetDecay(0.5)

	return v
}

func (v *Snare) Trigger(velocity float64) {
	vel := clamp01(velocity)
	v.active = true
	v.toneEnv = snareToneLevel * vel
	v.noiseEnv = vel
	v.phase = 0
	v.hpFilter.Reset()
}

func (v *Snare) IsActive() bool { return v.active }

func (v *Snare) SetDecay(amount float64) {
	scale := decayScaleMin + clamp01(amount)
	v.toneDecay = decayCoef(v.sr, snareBaseToneS*scale)
	v.noiseDecay = decayCoef(v.sr, snareBaseNoiseS*scale)
}

func (v *Snare) Tick() float64 {
	if !v.active {
		return 0
	}

	v.phase += 2 * math.Pi * snareToneHz / v.sr
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
	sr       float64
	active   bool
	env      float64
	envDecay float64
	bpFilter biquad.Section
	rng      *rand.Rand
}

func NewHiHat(sr float64) *HiHat {
	bpCoeffs := design.Bandpass(hatBPHz, hatBPQ, sr)

	v := &HiHat{
		sr:       sr,
		bpFilter: *biquad.NewSection(bpCoeffs),
		rng:      newVoiceRng(hatSeed),
	}
	v.SetDecay(0.5)

	return v
}

func (v *HiHat) Trigger(velocity float64) {
	v.active = true
	v.env = clamp01(velocity)
	v.bpFilter.Reset()
}

func (v *HiHat) IsActive() bool { return v.active }

func (v *HiHat) SetDecay(amount float64) {
	scale := decayScaleMin + clamp01(amount)
	v.envDecay = decayCoef(v.sr, hatBaseDecayS*scale)
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

	return sample * hatGain
}

// ── Tom ────────────────────────────────────────────────────────────────────

type Tom struct {
	sr       float64
	active   bool
	age      int
	phase    float64
	env      float64
	envDecay float64
}

func NewTom(sr float64) *Tom {
	v := &Tom{sr: sr}
	v.SetDecay(0.5)

	return v
}

func (v *Tom) Trigger(velocity float64) {
	v.active = true
	v.age = 0
	v.env = clamp01(velocity)
	v.phase = 0
}

func (v *Tom) IsActive() bool { return v.active }

func (v *Tom) SetDecay(amount float64) {
	scale := decayScaleMin + clamp01(amount)
	v.envDecay = decayCoef(v.sr, tomBaseDecayS*scale)
}

func (v *Tom) Tick() float64 {
	if !v.active {
		return 0
	}

	t := float64(v.age) / (v.sr * tomPitchTCS)
	freq := tomPitchToHz + (tomPitchFromHz-tomPitchToHz)*math.Exp(-t*pitchSweepRate)

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

	return sample * tomGain
}

// ── Cymbal ─────────────────────────────────────────────────────────────────

type Cymbal struct {
	sr       float64
	active   bool
	env      float64
	envDecay float64
	bpFilter biquad.Section
	rng      *rand.Rand
}

func NewCymbal(sr float64) *Cymbal {
	bpCoeffs := design.Bandpass(cymBPHz, cymBPQ, sr)

	v := &Cymbal{
		sr:       sr,
		bpFilter: *biquad.NewSection(bpCoeffs),
		rng:      newVoiceRng(cymSeed),
	}
	v.SetDecay(0.5)

	return v
}

func (v *Cymbal) Trigger(velocity float64) {
	v.active = true
	v.env = clamp01(velocity)
	v.bpFilter.Reset()
}

func (v *Cymbal) IsActive() bool { return v.active }

func (v *Cymbal) SetDecay(amount float64) {
	scale := decayScaleMin + clamp01(amount)
	v.envDecay = decayCoef(v.sr, cymBaseDecayS*scale)
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

	return sample * cymGain
}
