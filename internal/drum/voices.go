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

// ── Physically Inspired Kit ────────────────────────────────────────────────

type waveguideDrum struct {
	sr         float64
	active     bool
	env        float64
	envDecay   float64
	baseDecay  float64
	delay      []float64
	idx        int
	damp       float64
	dispersion float64
	rng        *rand.Rand
}

func newWaveguideDrum(sr, freq, baseDecay, damp, dispersion float64, seed int64) waveguideDrum {
	delayLen := int(sr / freq)
	if delayLen < 8 {
		delayLen = 8
	}

	v := waveguideDrum{
		sr:         sr,
		baseDecay:  baseDecay,
		delay:      make([]float64, delayLen),
		damp:       damp,
		dispersion: dispersion,
		rng:        rand.New(rand.NewSource(seed)),
	}
	v.SetDecay(0.5)

	return v
}

func (v *waveguideDrum) Trigger() {
	v.active = true
	v.env = 1.0
	v.idx = 0

	for i := range v.delay {
		pulse := 0.0
		if i < len(v.delay)/3 {
			pulse = (v.rng.Float64()*2 - 1) * (1 - float64(i)/float64(len(v.delay)/3))
		}
		v.delay[i] = pulse
	}
}

func (v *waveguideDrum) IsActive() bool { return v.active }

func (v *waveguideDrum) SetDecay(amount float64) {
	scale := 0.55 + clamp01(amount)
	v.envDecay = decayCoef(v.sr, v.baseDecay*scale)
}

func (v *waveguideDrum) Tick() float64 {
	if !v.active {
		return 0
	}

	n := len(v.delay)
	i0 := v.idx
	i1 := (i0 + 1) % n
	i2 := (i0 + 2) % n

	y := v.delay[i0]
	next := v.damp*(0.5*(v.delay[i1]+v.delay[i2])) + v.dispersion*(v.delay[i1]-v.delay[i2])
	v.delay[i0] = next
	v.idx = i1

	v.env *= v.envDecay
	if v.env < 1e-4 {
		v.active = false
	}

	return y * v.env
}

type PMKick struct {
	body  waveguideDrum
	phase float64
}

func NewPMKick(sr float64) *PMKick {
	return &PMKick{body: newWaveguideDrum(sr, 55, 0.65, 0.998, 0.003, 314)}
}

func (v *PMKick) Trigger() {
	v.body.Trigger()
	v.phase = 0
}
func (v *PMKick) IsActive() bool          { return v.body.IsActive() }
func (v *PMKick) SetDecay(amount float64) { v.body.SetDecay(amount) }
func (v *PMKick) Tick() float64 {
	res := v.body.Tick()
	v.phase += 2 * math.Pi * 48 / v.body.sr
	if v.phase > 2*math.Pi {
		v.phase -= 2 * math.Pi
	}
	return res*0.95 + math.Sin(v.phase)*math.Abs(res)*0.35
}

type PMSnare struct {
	body       waveguideDrum
	noiseEnv   float64
	noiseDecay float64
	hpFilter   biquad.Section
	rng        *rand.Rand
}

func NewPMSnare(sr float64) *PMSnare {
	hpCoeffs := design.Highpass(1800, 0.8, sr)
	v := &PMSnare{
		body:     newWaveguideDrum(sr, 170, 0.28, 0.992, 0.02, 2718),
		hpFilter: *biquad.NewSection(hpCoeffs),
		rng:      rand.New(rand.NewSource(777)),
	}
	v.SetDecay(0.5)
	return v
}

func (v *PMSnare) Trigger() {
	v.body.Trigger()
	v.noiseEnv = 1.0
	v.hpFilter.Reset()
}
func (v *PMSnare) IsActive() bool { return v.body.IsActive() || v.noiseEnv > 1e-4 }
func (v *PMSnare) SetDecay(amount float64) {
	v.body.SetDecay(amount)
	scale := 0.5 + clamp01(amount)
	v.noiseDecay = decayCoef(v.body.sr, 0.16*scale)
}
func (v *PMSnare) Tick() float64 {
	body := v.body.Tick()
	noise := v.hpFilter.ProcessSample((v.rng.Float64()*2 - 1) * v.noiseEnv)
	v.noiseEnv *= v.noiseDecay
	if !v.body.IsActive() && v.noiseEnv < 1e-4 {
		v.body.active = false
	}
	return body*0.9 + noise*0.6
}

type PMHat struct {
	res        waveguideDrum
	bpFilter   biquad.Section
	rng        *rand.Rand
	noiseEnv   float64
	noiseDecay float64
}

func NewPMHat(sr float64) *PMHat {
	bpCoeffs := design.Bandpass(9500, 2.4, sr)
	v := &PMHat{
		res:      newWaveguideDrum(sr, 320, 0.08, 0.985, 0.04, 99),
		bpFilter: *biquad.NewSection(bpCoeffs),
		rng:      rand.New(rand.NewSource(404)),
	}
	v.SetDecay(0.5)
	return v
}

func (v *PMHat) Trigger() {
	v.res.Trigger()
	v.noiseEnv = 0.8
	v.bpFilter.Reset()
}
func (v *PMHat) IsActive() bool { return v.res.IsActive() || v.noiseEnv > 1e-4 }
func (v *PMHat) SetDecay(amount float64) {
	v.res.SetDecay(amount)
	scale := 0.4 + clamp01(amount)
	v.noiseDecay = decayCoef(v.res.sr, 0.03*scale)
}
func (v *PMHat) Tick() float64 {
	exciter := (v.rng.Float64()*2 - 1) * v.noiseEnv
	noise := v.bpFilter.ProcessSample(exciter)
	v.noiseEnv *= v.noiseDecay
	return noise + v.res.Tick()*0.35
}

type PMTom struct{ body waveguideDrum }

func NewPMTom(sr float64) *PMTom {
	return &PMTom{body: newWaveguideDrum(sr, 95, 0.45, 0.996, 0.008, 505)}
}
func (v *PMTom) Trigger()                { v.body.Trigger() }
func (v *PMTom) IsActive() bool          { return v.body.IsActive() }
func (v *PMTom) SetDecay(amount float64) { v.body.SetDecay(amount) }
func (v *PMTom) Tick() float64           { return v.body.Tick() * 0.9 }

type PMCymbal struct {
	resA     waveguideDrum
	resB     waveguideDrum
	bp       biquad.Section
	rng      *rand.Rand
	env      float64
	envDecay float64
}

func NewPMCymbal(sr float64) *PMCymbal {
	bpCoeffs := design.Bandpass(7600, 1.1, sr)
	v := &PMCymbal{
		resA: newWaveguideDrum(sr, 420, 1.1, 0.997, 0.03, 8080),
		resB: newWaveguideDrum(sr, 510, 1.0, 0.996, 0.035, 9090),
		bp:   *biquad.NewSection(bpCoeffs),
		rng:  rand.New(rand.NewSource(1234)),
	}
	v.SetDecay(0.5)
	return v
}

func (v *PMCymbal) Trigger() {
	v.resA.Trigger()
	v.resB.Trigger()
	v.env = 1
	v.bp.Reset()
}
func (v *PMCymbal) IsActive() bool { return (v.resA.IsActive() || v.resB.IsActive()) && v.env > 1e-4 }
func (v *PMCymbal) SetDecay(amount float64) {
	v.resA.SetDecay(amount)
	v.resB.SetDecay(amount)
	scale := 0.5 + clamp01(amount)
	v.envDecay = decayCoef(v.resA.sr, 0.8*scale)
}
func (v *PMCymbal) Tick() float64 {
	metal := (v.resA.Tick() + v.resB.Tick()) * 0.7
	shimmer := v.bp.ProcessSample((v.rng.Float64()*2 - 1) * v.env)
	v.env *= v.envDecay
	return metal + shimmer*0.65
}
