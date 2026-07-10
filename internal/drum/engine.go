package drum

import (
	"math"
	"math/rand/v2"

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

	// engineSeed seeds the probability/humanize randomness so renders stay
	// reproducible run-to-run (the voices are seeded the same way).
	engineSeed = 0x5eed

	// humanizeTimingMaxS is the largest timing jitter at humanize=1: each hit
	// is delayed by a random offset in [0, humanize·humanizeTimingMaxS]. A hit
	// can only be pushed later — the step boundary has already elapsed — so the
	// jitter is one-sided rather than centered.
	humanizeTimingMaxS = 0.015

	// humanizeVelMax is the largest velocity deviation at humanize=1: each hit's
	// velocity is scaled by 1 ± a random fraction up to humanize·humanizeVelMax.
	humanizeVelMax = 0.20

	// maxPending caps in-flight humanize-delayed triggers; sized well above the
	// handful that can overlap so scheduling stays allocation-free in Render.
	maxPending = 32
)

// pendingTrigger is a voice hit scheduled to fire a few samples in the future
// (humanize timing jitter). The pending set is a fixed-size array so the render
// loop never allocates.
type pendingTrigger struct {
	countdown int     // samples remaining until the voice fires
	track     int     // voice index to trigger
	velocity  float64 // humanized velocity to trigger at
	active    bool    // slot in use
}

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

	prob     float64 // per-hit trigger probability in [0, 1]
	humanize float64 // timing/velocity randomization amount in [0, 1]
	rng      *rand.Rand
	pending  [maxPending]pendingTrigger

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
		prob:      1,
		humanize:  0,
		rng:       rand.New(rand.NewPCG(engineSeed, engineSeed)),
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

		// Drop any humanize-delayed hits so they don't fire after restart.
		for i := range e.pending {
			e.pending[i].active = false
		}
	}

	e.running = running
}

// SetProbability sets the chance each scheduled hit actually fires, clamped to
// [0, 1]. 1 = every hit plays (default); 0 = silence.
func (e *Engine) SetProbability(p float64) {
	e.prob = clamp01(p)
}

// SetHumanize sets the humanize amount, clamped to [0, 1]. It jitters each
// hit's timing (delayed up to humanize·15 ms) and scales its velocity by up to
// ±humanize·20%. 0 = mechanical (default).
func (e *Engine) SetHumanize(h float64) {
	e.humanize = clamp01(h)
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
func (e *Engine) SetStepCount(count int) {
	if count < 1 {
		count = 1
	}

	if count > MaxSteps {
		count = MaxSteps
	}

	e.stepCount = count
	if e.currentStep >= count {
		e.currentStep %= count
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

// triggerStep fires (or schedules) the voices whose cell is active on the
// current step, applying probability and humanize.
func (e *Engine) triggerStep() {
	for t := range e.voices {
		vel := e.pattern[t][e.currentStep]
		if vel <= 0 {
			continue
		}

		// Probability gate: drop the hit with chance 1-prob. At prob=1 the
		// rng is left untouched so mechanical renders stay deterministic.
		if e.prob < 1 && e.rng.Float64() >= e.prob {
			continue
		}

		if e.humanize <= 0 {
			e.voices[t].Trigger(vel)
			continue
		}

		// Velocity humanize: scale by 1 ± up to humanize·20%.
		vel = clamp01(vel * (1 + (e.rng.Float64()*2-1)*e.humanize*humanizeVelMax))

		// Timing humanize: delay by 0..humanize·15 ms worth of samples.
		delay := int(e.rng.Float64() * e.humanize * humanizeTimingMaxS * e.sr)
		if delay <= 0 {
			e.voices[t].Trigger(vel)
			continue
		}

		e.schedule(t, vel, delay)
	}
}

// schedule queues a delayed voice trigger; if the fixed pending buffer is full
// the hit fires immediately rather than being dropped.
func (e *Engine) schedule(track int, velocity float64, delay int) {
	for i := range e.pending {
		if !e.pending[i].active {
			e.pending[i] = pendingTrigger{
				countdown: delay,
				track:     track,
				velocity:  velocity,
				active:    true,
			}

			return
		}
	}

	e.voices[track].Trigger(velocity)
}

// firePending advances every queued trigger by one sample and fires those that
// have reached their scheduled time.
func (e *Engine) firePending() {
	for i := range e.pending {
		if !e.pending[i].active {
			continue
		}

		e.pending[i].countdown--
		if e.pending[i].countdown <= 0 {
			e.voices[e.pending[i].track].Trigger(e.pending[i].velocity)
			e.pending[i].active = false
		}
	}
}

// Render fills buf with mono audio samples.
func (e *Engine) Render(buf []float32) {
	for i := range buf {
		if e.running {
			if e.stepSamples == 0 {
				e.triggerStep()
			}

			e.stepSamples++
			if e.stepSamples >= e.stepLen[e.currentStep] {
				e.stepSamples = 0
				e.currentStep = (e.currentStep + 1) % e.stepCount
			}
		}

		e.firePending()

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
