package drum

import (
	"math"
	"math/rand"

	"github.com/cwbudde/algo-dsp/dsp/filter/biquad"
	"github.com/cwbudde/algo-dsp/dsp/filter/design"
)

// Voice is a single-shot drum synthesizer voice.
type Voice interface {
	Trigger()
	Tick() float64
	IsActive() bool
	SetDecay(amount float64)
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func decayCoef(sr, decayS float64) float64 {
	if decayS < 0.005 {
		decayS = 0.005
	}
	return math.Exp(-1.0 / (sr * decayS))
}

// ── Bass Drum ──────────────────────────────────────────────────────────────

type BassDrum struct {
	sr        float64
	active    bool
	age       int
	phase     float64
	env       float64
	envDecay  float64
	baseDecay float64
	pitchFrom float64
	pitchTo   float64
	pitchTC   float64 // pitch time-constant in samples
}

func NewBassDrum(sr float64) *BassDrum {
	v := &BassDrum{
		sr:        sr,
		baseDecay: 0.45,
		pitchFrom: 200.0,
		pitchTo:   50.0,
		pitchTC:   sr * 0.06,
	}
	v.SetDecay(0.5)
	return v
}

func (v *BassDrum) Trigger() {
	v.active = true
	v.age = 0
	v.env = 1.0
	v.phase = 0
}

func (v *BassDrum) IsActive() bool { return v.active }

func (v *BassDrum) SetDecay(amount float64) {
	scale := 0.5 + clamp01(amount)
	v.envDecay = decayCoef(v.sr, v.baseDecay*scale)
}

func (v *BassDrum) Tick() float64 {
	if !v.active {
		return 0
	}

	t := float64(v.age) / v.pitchTC
	freq := v.pitchTo + (v.pitchFrom-v.pitchTo)*math.Exp(-t*5)

	v.phase += 2 * math.Pi * freq / v.sr
	if v.phase > 2*math.Pi {
		v.phase -= 2 * math.Pi
	}

	sample := math.Sin(v.phase) * v.env

	v.env *= v.envDecay
	if v.env < 1e-4 {
		v.active = false
	}

	v.age++

	return sample
}

// ── Snare ──────────────────────────────────────────────────────────────────

type Snare struct {
	sr         float64
	active     bool
	age        int
	phase      float64
	toneEnv    float64
	toneDecay  float64
	baseTone   float64
	noiseEnv   float64
	noiseDecay float64
	baseNoise  float64
	hpFilter   biquad.Section
	rng        *rand.Rand
}

func NewSnare(sr float64) *Snare {
	hpCoeffs := design.Highpass(2000, 0.7, sr)

	v := &Snare{
		sr:        sr,
		baseTone:  0.12,
		baseNoise: 0.18,
		hpFilter:  *biquad.NewSection(hpCoeffs),
		rng:       rand.New(rand.NewSource(42)),
	}
	v.SetDecay(0.5)
	return v
}

func (v *Snare) Trigger() {
	v.active = true
	v.age = 0
	v.toneEnv = 0.7
	v.noiseEnv = 1.0
	v.phase = 0
	v.hpFilter.Reset()
}

func (v *Snare) IsActive() bool { return v.active }

func (v *Snare) SetDecay(amount float64) {
	scale := 0.5 + clamp01(amount)
	v.toneDecay = decayCoef(v.sr, v.baseTone*scale)
	v.noiseDecay = decayCoef(v.sr, v.baseNoise*scale)
}

func (v *Snare) Tick() float64 {
	if !v.active {
		return 0
	}

	v.phase += 2 * math.Pi * 200 / v.sr
	if v.phase > 2*math.Pi {
		v.phase -= 2 * math.Pi
	}

	tone := math.Sin(v.phase) * v.toneEnv
	noise := (v.rng.Float64()*2 - 1) * v.noiseEnv
	noise = v.hpFilter.ProcessSample(noise)
	v.toneEnv *= v.toneDecay

	v.noiseEnv *= v.noiseDecay
	if v.toneEnv < 1e-4 && v.noiseEnv < 1e-4 {
		v.active = false
	}

	v.age++

	return tone + noise
}

// ── Hi-Hat ─────────────────────────────────────────────────────────────────

type HiHat struct {
	sr        float64
	active    bool
	age       int
	env       float64
	envDecay  float64
	baseDecay float64
	bpFilter  biquad.Section
	rng       *rand.Rand
}

func NewHiHat(sr float64, closed bool) *HiHat {
	bpCoeffs := design.Bandpass(10000, 2.0, sr)

	decayS := 0.04
	if !closed {
		decayS = 0.4
	}

	v := &HiHat{
		sr:        sr,
		baseDecay: decayS,
		bpFilter:  *biquad.NewSection(bpCoeffs),
		rng:       rand.New(rand.NewSource(123)),
	}
	v.SetDecay(0.5)
	return v
}

func (v *HiHat) Trigger() {
	v.active = true
	v.age = 0
	v.env = 1.0
	v.bpFilter.Reset()
}

func (v *HiHat) IsActive() bool { return v.active }

func (v *HiHat) SetDecay(amount float64) {
	scale := 0.5 + clamp01(amount)
	v.envDecay = decayCoef(v.sr, v.baseDecay*scale)
}

func (v *HiHat) Tick() float64 {
	if !v.active {
		return 0
	}

	noise := (v.rng.Float64()*2 - 1) * v.env
	sample := v.bpFilter.ProcessSample(noise)

	v.env *= v.envDecay
	if v.env < 1e-4 {
		v.active = false
	}

	v.age++

	return sample * 1.5
}

// ── Tom ────────────────────────────────────────────────────────────────────

type Tom struct {
	sr        float64
	active    bool
	age       int
	phase     float64
	env       float64
	envDecay  float64
	baseDecay float64
	pitchFrom float64
	pitchTo   float64
	pitchTC   float64
}

func NewTom(sr float64) *Tom {
	v := &Tom{
		sr:        sr,
		baseDecay: 0.35,
		pitchFrom: 120.0,
		pitchTo:   60.0,
		pitchTC:   sr * 0.1,
	}
	v.SetDecay(0.5)
	return v
}

func (v *Tom) Trigger() {
	v.active = true
	v.age = 0
	v.env = 1.0
	v.phase = 0
}

func (v *Tom) IsActive() bool { return v.active }

func (v *Tom) SetDecay(amount float64) {
	scale := 0.5 + clamp01(amount)
	v.envDecay = decayCoef(v.sr, v.baseDecay*scale)
}

func (v *Tom) Tick() float64 {
	if !v.active {
		return 0
	}

	t := float64(v.age) / v.pitchTC
	freq := v.pitchTo + (v.pitchFrom-v.pitchTo)*math.Exp(-t*5)

	v.phase += 2 * math.Pi * freq / v.sr
	if v.phase > 2*math.Pi {
		v.phase -= 2 * math.Pi
	}

	sample := math.Sin(v.phase) * v.env

	v.env *= v.envDecay
	if v.env < 1e-4 {
		v.active = false
	}

	v.age++

	return sample * 0.9
}

// ── Cymbal ─────────────────────────────────────────────────────────────────

type Cymbal struct {
	sr        float64
	active    bool
	age       int
	env       float64
	envDecay  float64
	baseDecay float64
	bpFilter  biquad.Section
	rng       *rand.Rand
}

func NewCymbal(sr float64) *Cymbal {
	bpCoeffs := design.Bandpass(7000, 1.2, sr)

	v := &Cymbal{
		sr:        sr,
		baseDecay: 1.2,
		bpFilter:  *biquad.NewSection(bpCoeffs),
		rng:       rand.New(rand.NewSource(999)),
	}
	v.SetDecay(0.5)
	return v
}

func (v *Cymbal) Trigger() {
	v.active = true
	v.age = 0
	v.env = 1.0
	v.bpFilter.Reset()
}

func (v *Cymbal) IsActive() bool { return v.active }

func (v *Cymbal) SetDecay(amount float64) {
	scale := 0.5 + clamp01(amount)
	v.envDecay = decayCoef(v.sr, v.baseDecay*scale)
}

func (v *Cymbal) Tick() float64 {
	if !v.active {
		return 0
	}

	noise := (v.rng.Float64()*2 - 1) * v.env
	sample := v.bpFilter.ProcessSample(noise)

	v.env *= v.envDecay
	if v.env < 1e-4 {
		v.active = false
	}

	v.age++

	return sample * 1.2
}
