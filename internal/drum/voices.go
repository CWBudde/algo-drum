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
}

// ── Bass Drum ──────────────────────────────────────────────────────────────

type BassDrum struct {
	sr        float64
	active    bool
	age       int
	phase     float64
	env       float64
	envDecay  float64
	pitchFrom float64
	pitchTo   float64
	pitchTC   float64 // pitch time-constant in samples
}

func NewBassDrum(sr float64) *BassDrum {
	return &BassDrum{
		sr:        sr,
		envDecay:  math.Exp(-1.0 / (sr * 0.45)),
		pitchFrom: 200.0,
		pitchTo:   50.0,
		pitchTC:   sr * 0.06,
	}
}

func (v *BassDrum) Trigger() {
	v.active = true
	v.age = 0
	v.env = 1.0
	v.phase = 0
}

func (v *BassDrum) IsActive() bool { return v.active }

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
	noiseEnv   float64
	noiseDecay float64
	hpFilter   biquad.Section
	rng        *rand.Rand
}

func NewSnare(sr float64) *Snare {
	hpCoeffs := design.Highpass(2000, 0.7, sr)

	return &Snare{
		sr:         sr,
		toneDecay:  math.Exp(-1.0 / (sr * 0.12)),
		noiseDecay: math.Exp(-1.0 / (sr * 0.18)),
		hpFilter:   *biquad.NewSection(hpCoeffs),
		rng:        rand.New(rand.NewSource(42)),
	}
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
	sr       float64
	active   bool
	age      int
	env      float64
	envDecay float64
	bpFilter biquad.Section
	rng      *rand.Rand
}

func NewHiHat(sr float64, closed bool) *HiHat {
	bpCoeffs := design.Bandpass(10000, 2.0, sr)

	decayS := 0.04
	if !closed {
		decayS = 0.4
	}

	return &HiHat{
		sr:       sr,
		envDecay: math.Exp(-1.0 / (sr * decayS)),
		bpFilter: *biquad.NewSection(bpCoeffs),
		rng:      rand.New(rand.NewSource(123)),
	}
}

func (v *HiHat) Trigger() {
	v.active = true
	v.age = 0
	v.env = 1.0
	v.bpFilter.Reset()
}

func (v *HiHat) IsActive() bool { return v.active }

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
	pitchFrom float64
	pitchTo   float64
	pitchTC   float64
}

func NewTom(sr float64) *Tom {
	return &Tom{
		sr:        sr,
		envDecay:  math.Exp(-1.0 / (sr * 0.35)),
		pitchFrom: 120.0,
		pitchTo:   60.0,
		pitchTC:   sr * 0.1,
	}
}

func (v *Tom) Trigger() {
	v.active = true
	v.age = 0
	v.env = 1.0
	v.phase = 0
}

func (v *Tom) IsActive() bool { return v.active }

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
	sr       float64
	active   bool
	age      int
	env      float64
	envDecay float64
	bpFilter biquad.Section
	rng      *rand.Rand
}

func NewCymbal(sr float64) *Cymbal {
	bpCoeffs := design.Bandpass(7000, 1.2, sr)

	return &Cymbal{
		sr:       sr,
		envDecay: math.Exp(-1.0 / (sr * 1.2)),
		bpFilter: *biquad.NewSection(bpCoeffs),
		rng:      rand.New(rand.NewSource(999)),
	}
}

func (v *Cymbal) Trigger() {
	v.active = true
	v.age = 0
	v.env = 1.0
	v.bpFilter.Reset()
}

func (v *Cymbal) IsActive() bool { return v.active }

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
