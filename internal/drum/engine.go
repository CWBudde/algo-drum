package drum

import (
	"math"

	"github.com/cwbudde/algo-dsp/dsp/effects/dynamics"
	"github.com/cwbudde/algo-dsp/dsp/effects/reverb"
)

const (
	TrackCount = 5

	// MaxSteps is the pattern capacity; the active length is set at runtime
	// via SetStepCount (1–MaxSteps). Steps are 16th notes, so 16 steps span
	// one 4/4 bar.
	MaxSteps = 16

	// mixHeadroom scales the summed voice mix so simultaneous hits do not
	// slam the limiter; the limiter then only catches rare worst cases.
	mixHeadroom = 0.5

	// volSmoothTauS is the per-track volume ramp time constant. ~8 ms feels
	// instant on a knob but is long enough to avoid zipper noise.
	volSmoothTauS = 0.008
)

// Engine is the drum machine sequencer and mixer.
type Engine struct {
	sr      float64
	running bool
	bpm     float64
	swing   float64 // 0.0 = no swing, 0.5 = full shuffle

	pattern [TrackCount][MaxSteps]float64 // per-cell velocity in [0, 1], 0 = off
	volumes [TrackCount]float64           // targets set by SetVolume
	liveVol [TrackCount]float64           // smoothed volumes applied in Render
	volCoef float64                       // per-sample one-pole ramp coefficient
	decays  [TrackCount]float64

	voices [TrackCount]Voice

	stepCount   int // active pattern length in [1, MaxSteps]
	currentStep int
	stepSamples int64
	stepLen     [MaxSteps]int64 // pre-computed step lengths

	reverb       *reverb.FDNReverb
	reverbAmount float64
	limiter      *dynamics.LookaheadLimiter
}

// NewEngine creates a drum engine at the given sample rate.
func NewEngine(sr float64) *Engine {
	e := &Engine{
		sr:        sr,
		bpm:       120,
		stepCount: MaxSteps,
		volCoef:   1 - math.Exp(-1.0/(sr*volSmoothTauS)),
	}
	for i := range e.volumes {
		e.volumes[i] = 1.0
		e.liveVol[i] = 1.0
		e.decays[i] = 0.5
	}

	e.voices[0] = NewBassDrum(sr)
	e.voices[1] = NewSnare(sr)
	e.voices[2] = NewHiHat(sr)
	e.voices[3] = NewTom(sr)

	e.voices[4] = NewCymbal(sr)
	for i := range e.voices {
		e.voices[i].SetDecay(e.decays[i])
	}

	e.recomputeStepLengths()

	rev, err := reverb.NewFDNReverb(sr)
	logErr("NewFDNReverb", err)
	logErr("reverb.SetWet", rev.SetWet(0))
	e.reverb = rev

	// Lookahead limiter controls the sustained level (dense patterns, long
	// reverb tails). Its smoothed detector still under-reacts to
	// single-sample noise transients, so the hard clamp in Render is the
	// actual brick wall for those rare (~inaudible) peaks.
	lim, err := dynamics.NewLookaheadLimiter(sr)
	logErr("NewLookaheadLimiter", err)
	logErr("limiter.SetThreshold", lim.SetThreshold(-1.0))
	e.limiter = lim

	return e
}

// logErr reports a discarded DSP configuration error; under wasm println
// lands in the JS console.
func logErr(context string, err error) {
	if err != nil {
		println("drum: " + context + ": " + err.Error())
	}
}

// recomputeStepLengths recalculates step durations accounting for swing.
// swing=0: all equal. swing=0.5: even steps get 1.5× base, odd get 0.5× base.
func (e *Engine) recomputeStepLengths() {
	base := e.sr * 60.0 / e.bpm / 4.0 // samples per 16th note

	s := e.swing
	for i := range e.stepLen {
		if i%2 == 0 {
			e.stepLen[i] = int64(base * (1.0 + s))
		} else {
			e.stepLen[i] = int64(base * (1.0 - s))
		}
	}
}

func (e *Engine) SetRunning(running bool) {
	if !running {
		e.currentStep = 0
		e.stepSamples = 0
	}

	e.running = running
}

// SetTempo sets the tempo, clamped to [30, 300] BPM.
func (e *Engine) SetTempo(bpm float64) {
	if bpm < 30 {
		bpm = 30
	}

	if bpm > 300 {
		bpm = 300
	}

	e.bpm = bpm
	e.recomputeStepLengths()
}

// SetSwing sets the swing amount, clamped to [0, 0.5].
func (e *Engine) SetSwing(swing float64) {
	if swing < 0 {
		swing = 0
	}

	if swing > 0.5 {
		swing = 0.5
	}

	e.swing = swing
	e.recomputeStepLengths()
}

// SetStepCount sets the active pattern length, clamped to [1, MaxSteps].
// Cells beyond the new length keep their contents (see SetCell).
func (e *Engine) SetStepCount(n int) {
	if n < 1 {
		n = 1
	}

	if n > MaxSteps {
		n = MaxSteps
	}

	e.stepCount = n
	if e.currentStep >= n {
		e.currentStep %= n
	}
}

// SetCell sets a cell's velocity, clamped to [0, 1] (0 = off). Steps are
// addressable up to MaxSteps regardless of the active step count, so
// shrinking and re-growing the pattern is lossless.
func (e *Engine) SetCell(track, step int, velocity float64) {
	if track < 0 || track >= TrackCount || step < 0 || step >= MaxSteps {
		return
	}

	e.pattern[track][step] = clamp01(velocity)
}

// SetPattern replaces cells from a flat track-major slice (index =
// track*MaxSteps + step) of velocities, each clamped to [0, 1]. Values past
// TrackCount×MaxSteps are ignored; a shorter slice leaves the rest untouched.
func (e *Engine) SetPattern(velocities []float64) {
	for i, vel := range velocities {
		if i >= TrackCount*MaxSteps {
			return
		}

		e.pattern[i/MaxSteps][i%MaxSteps] = clamp01(vel)
	}
}

// Pattern returns a flat track-major copy of the full pattern (see
// SetPattern for the layout).
func (e *Engine) Pattern() []float64 {
	out := make([]float64, 0, TrackCount*MaxSteps)
	for t := range e.pattern {
		out = append(out, e.pattern[t][:]...)
	}

	return out
}

// SetVolume sets per-track volume, clamped to [0, 1]. The change ramps in
// over ~volSmoothTauS inside Render to avoid zipper noise.
func (e *Engine) SetVolume(track int, vol float64) {
	if track >= 0 && track < TrackCount {
		e.volumes[track] = clamp01(vol)
	}
}

// SetDecay sets per-track decay amount, clamped to [0, 1].
func (e *Engine) SetDecay(track int, amount float64) {
	if track < 0 || track >= TrackCount {
		return
	}

	amount = clamp01(amount)
	e.decays[track] = amount
	e.voices[track].SetDecay(amount)
}

// SetReverb sets the reverb amount in [0, 1].
// 0 = fully dry, 1 = maximum reverb (wet=0.45, RT60=4 s).
func (e *Engine) SetReverb(amount float64) {
	amount = clamp01(amount)
	e.reverbAmount = amount

	if amount <= 0 {
		logErr("reverb.SetWet", e.reverb.SetWet(0))
		return
	}

	logErr("reverb.SetWet", e.reverb.SetWet(amount*0.45))
	rt60 := 0.3 + amount*3.7
	logErr("reverb.SetRT60", e.reverb.SetRT60(rt60))
}

func (e *Engine) CurrentStep() int {
	if !e.running {
		return -1
	}

	return e.currentStep
}

// Render fills buf with mono audio samples.
func (e *Engine) Render(buf []float32) {
	for i := range buf {
		if e.running {
			if e.stepSamples == 0 {
				for t := range e.voices {
					if vel := e.pattern[t][e.currentStep]; vel > 0 {
						e.voices[t].Trigger(vel)
					}
				}
			}

			e.stepSamples++
			if e.stepSamples >= e.stepLen[e.currentStep] {
				e.stepSamples = 0
				e.currentStep = (e.currentStep + 1) % e.stepCount
			}
		}

		var out float64
		for t, v := range e.voices {
			// One-pole ramp toward the target volume so knob moves during
			// playback do not step the gain per-sample (zipper noise).
			e.liveVol[t] += (e.volumes[t] - e.liveVol[t]) * e.volCoef
			out += v.Tick() * e.liveVol[t]
		}

		out *= mixHeadroom

		if e.reverbAmount > 0 {
			out = e.reverb.ProcessSample(out)
		}

		out = e.limiter.ProcessSample(out)

		// Hard safety clamp — anything past ±1.0 would be clipped by the
		// browser's output stage anyway; this guarantees the contract.
		if out > 1 {
			out = 1
		} else if out < -1 {
			out = -1
		}

		buf[i] = float32(out)
	}
}
