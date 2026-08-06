package drum

import (
	"math"
	"math/bits"
	"math/rand/v2"

	"github.com/cwbudde/algo-dsp/dsp/effects/reverb"
)

const (
	TrackCount = 7

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

	// Tempo bounds in BPM. The lower bound also bounds a step's length, which
	// keeps the swing arithmetic in recomputeStepDurations well away from
	// degenerate (sub-sample) steps.
	minTempoBPM = 30.0
	maxTempoBPM = 300.0

	// maxSwing is full shuffle: the long step of a pair runs at 1.5× the base
	// step length and the short one at 0.5×.
	maxSwing = 0.5

	// Sample-rate bounds. The rate arrives from JS, so a non-finite or
	// non-positive value falls back to defaultSampleRate and anything else is
	// clamped into a range the DSP is defined over.
	defaultSampleRate = 48000.0
	minSampleRate     = 8000.0
	maxSampleRate     = 768000.0

	tomTrackIndex  = 3
	tom2TrackIndex = 5

	// secondsPerMinute and stepsPerBeat convert BPM to samples per step;
	// steps are 16th notes, so there are four per quarter-note beat.
	secondsPerMinute = 60.0
	stepsPerBeat     = 4.0
	stepPhaseUnit    = uint64(1) << 32

	// Reverb mapping: SetReverb(1) means a 0.45 wet mix and a 4 s RT60.
	reverbMaxWet     = 0.45
	reverbMinRT60S   = 0.3
	reverbRangeRT60S = 3.7
)

// pendingTrigger is a voice hit scheduled to fire a few samples in the future
// (humanize timing jitter). The pending set is a fixed-size array so the render
// loop never allocates.
type pendingTrigger struct {
	countdown int     // samples remaining until the voice fires
	track     int     // voice index to trigger
	velocity  float64 // humanized velocity to trigger at
}

type transportState uint8

const (
	transportStopped transportState = iota
	transportPlaying
	transportPaused
)

// Engine is the drum machine sequencer and mixer.
type Engine struct {
	sr        float64
	transport transportState
	bpm       float64
	swing     float64 // 0.0 = no swing, 0.5 = full shuffle

	pattern [TrackCount][MaxSteps]float64 // per-cell velocity in [0, 1], 0 = off
	volumes [TrackCount]float64           // targets set by SetVolume
	liveVol [TrackCount]float64           // smoothed volumes applied in Render
	volCoef float64                       // per-sample one-pole ramp coefficient
	decays  [TrackCount]float64

	voices [TrackCount]Voice

	tomModels      [TrackCount]TomModel
	proceduralToms [TrackCount]*Tom
	physicalToms   [TrackCount]*physicalTom

	stepCount           int // active pattern length in [1, MaxSteps]
	currentStep         int
	stepPhase           uint64 // elapsed Q32.32 samples in the current step
	currentStepDuration uint64 // latched Q32.32 duration of the current step
	stepDuration        [MaxSteps]uint64
	stepTriggered       bool

	prob        float64 // per-hit trigger probability in [0, 1]
	humanize    float64 // timing/velocity randomization amount in [0, 1]
	rng         *rand.Rand
	pending     [maxPending]pendingTrigger
	pendingMask uint32

	reverb           *reverb.FDNReverb
	reverbAmount     float64
	liveReverbAmount float64
	limiter          *peakLimiter
	hardClipCount    uint64
}

// NewEngine creates a drum engine at the given sample rate. A non-finite or
// non-positive rate falls back to defaultSampleRate; any other value is
// clamped to [minSampleRate, maxSampleRate].
func NewEngine(sr float64) *Engine {
	sr = validSampleRate(sr)

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
	e.proceduralToms[tomTrackIndex] = NewTom(sr)
	e.voices[tomTrackIndex] = e.proceduralToms[tomTrackIndex]

	e.voices[4] = NewCymbal(sr)
	e.proceduralToms[tom2TrackIndex] = NewTom2(sr)
	e.voices[tom2TrackIndex] = e.proceduralToms[tom2TrackIndex]

	e.voices[6] = NewPercussion(sr)
	for i := range e.voices {
		e.voices[i].SetDecay(e.decays[i])
	}

	e.recomputeStepDurations()

	// The DSP constructors return (nil, err) for a rate they cannot work
	// with, so both handles stay nil-checked: a broken effect degrades to a
	// bypass instead of taking the engine down (see Render).
	rev, err := reverb.NewFDNReverb(sr)
	logErr("NewFDNReverb", err)

	if rev != nil {
		logErr("reverb.SetWet", rev.SetWet(0))
	}

	e.reverb = rev

	// The master limiter uses a peak-max lookahead detector, so even isolated
	// noise transients are held below the ceiling. The final clamp in Render is
	// retained only as a last-resort finite-output contract.
	e.limiter = newPeakLimiter(sr)

	return e
}

// logErr reports a discarded DSP configuration error; under wasm println
// lands in the JS console.
func logErr(context string, err error) {
	if err != nil {
		println("drum: " + context + ": " + err.Error())
	}
}

// validFloat is the single validation boundary for every float parameter that
// crosses into the engine. Non-finite input (NaN, ±Inf) is rejected — ok is
// false and the caller must leave its state untouched — while a finite value
// is clamped into [minVal, maxVal]. Plain comparisons cannot do this: every
// `<`/`>` test is false for NaN, so an unchecked NaN would walk through the
// clamp and poison the whole signal path.
func validFloat(val, minVal, maxVal float64) (float64, bool) {
	if math.IsNaN(val) || math.IsInf(val, 0) {
		return 0, false
	}

	if val < minVal {
		return minVal, true
	}

	if val > maxVal {
		return maxVal, true
	}

	return val, true
}

// validSampleRate sanitises the rate handed to NewEngine. Unlike the setters
// it cannot no-op — the engine has to exist — so a rejected rate falls back to
// defaultSampleRate.
func validSampleRate(sr float64) float64 {
	if math.IsNaN(sr) || math.IsInf(sr, 0) || sr <= 0 {
		return defaultSampleRate
	}

	rate, _ := validFloat(sr, minSampleRate, maxSampleRate)

	return rate
}

// validTrack reports whether track addresses a real voice. Out-of-range
// indices are a no-op for every indexed setter (see SetCell).
func validTrack(track int) bool {
	return track >= 0 && track < TrackCount
}

func validTomTrack(track int) bool {
	return track == tomTrackIndex || track == tom2TrackIndex
}

// validStep reports whether step addresses a real pattern cell. Steps are
// addressable up to MaxSteps regardless of the active step count.
func validStep(step int) bool {
	return step >= 0 && step < MaxSteps
}

// recomputeStepDurations recalculates Q32.32 step durations accounting for swing.
// Swing lengthens a step and shortens the one after it by the same amount, so
// each (long, short) pair still spans exactly two base steps and the loop
// keeps its tempo exactly in fixed point. The retained fractional phase crosses
// step and loop boundaries, so a non-integer samples-per-step value cannot
// accumulate tempo drift.
// An odd step count leaves the final step unpaired, so it keeps the plain base
// length rather than stretching the loop. Steps past the active length never
// play and are held at the base length too.
func (e *Engine) recomputeStepDurations() {
	base := e.sr * secondsPerMinute / e.bpm / stepsPerBeat // samples per 16th note
	plain := uint64(math.Round(base * float64(stepPhaseUnit)))
	delta := uint64(math.Round(base * e.swing * float64(stepPhaseUnit)))
	long := plain + delta
	short := plain - delta
	last := e.stepCount - 1

	for i := range e.stepDuration {
		unpaired := i > last || (i == last && e.stepCount%2 == 1)

		switch {
		case unpaired:
			e.stepDuration[i] = plain
		case i%2 == 0:
			e.stepDuration[i] = long
		default:
			e.stepDuration[i] = short
		}
	}

	// Parameter edits are latched at the next boundary while playing or
	// paused, avoiding a shortened step suddenly ending mid-buffer. A stopped
	// transport starts with the newly configured duration immediately.
	if e.transport == transportStopped || e.currentStepDuration == 0 {
		e.currentStepDuration = e.stepDuration[e.currentStep]
	}
}

func (e *Engine) SetRunning(running bool) {
	if running {
		e.transport = transportPlaying

		return
	}

	e.transport = transportStopped
	e.currentStep = 0
	e.stepPhase = 0
	e.currentStepDuration = e.stepDuration[0]
	e.stepTriggered = false

	// Drop any humanize-delayed hits so they don't fire after restart.
	e.pendingMask = 0
}

// Pause freezes sequencer time and delayed humanized hits while allowing
// already-triggered voices and effects to ring out. SetRunning(true) resumes
// from the held fractional position; SetRunning(false) performs a full stop.
func (e *Engine) Pause() {
	if e.transport == transportPlaying {
		e.transport = transportPaused
	}
}

// SetProbability sets the chance each scheduled hit actually fires, clamped to
// [0, 1]. 1 = every hit plays (default); 0 = silence. A non-finite value is
// rejected and leaves the current probability unchanged.
func (e *Engine) SetProbability(p float64) {
	prob, ok := validFloat(p, 0, 1)
	if !ok {
		return
	}

	e.prob = prob
}

// SetHumanize sets the humanize amount, clamped to [0, 1]. It jitters each
// hit's timing (delayed up to humanize·15 ms) and scales its velocity by up to
// ±humanize·20%. 0 = mechanical (default). A non-finite value is rejected and
// leaves the current amount unchanged.
func (e *Engine) SetHumanize(h float64) {
	humanize, ok := validFloat(h, 0, 1)
	if !ok {
		return
	}

	e.humanize = humanize
}

// SetTempo sets the tempo, clamped to [minTempoBPM, maxTempoBPM]. A non-finite
// value is rejected and leaves the current tempo unchanged.
func (e *Engine) SetTempo(bpm float64) {
	tempo, ok := validFloat(bpm, minTempoBPM, maxTempoBPM)
	if !ok {
		return
	}

	e.bpm = tempo
	e.recomputeStepDurations()
}

// SetSwing sets the swing amount, clamped to [0, maxSwing]. A non-finite value
// is rejected and leaves the current swing unchanged.
func (e *Engine) SetSwing(swing float64) {
	amount, ok := validFloat(swing, 0, maxSwing)
	if !ok {
		return
	}

	e.swing = amount
	e.recomputeStepDurations()
}

// SetStepCount sets the active pattern length, clamped to [1, MaxSteps].
// Cells beyond the new length keep their contents (see SetCell). Step durations
// are recomputed because swing pairs steps within the active loop.
func (e *Engine) SetStepCount(count int) {
	if count < 1 {
		count = 1
	}

	if count > MaxSteps {
		count = MaxSteps
	}

	e.stepCount = count
	if e.currentStep >= count {
		// The playhead lands inside the shortened loop; restart that step so
		// it plays out with its own length instead of inheriting the elapsed
		// samples of the step it replaced.
		e.currentStep %= count
		e.stepPhase = 0
		e.stepTriggered = false
		e.currentStepDuration = 0
	}

	e.recomputeStepDurations()
}

// SetCell sets a cell's velocity, clamped to [0, 1] (0 = off). Steps are
// addressable up to MaxSteps regardless of the active step count, so
// shrinking and re-growing the pattern is lossless.
//
// Out-of-range contract: an invalid track or step index is a silent no-op, as
// is a non-finite velocity — the cell keeps its previous value. Every indexed
// setter behaves this way (SetVolume, SetDecay), because the JS bridge feeds
// unvalidated arguments straight through and must never take the engine down.
func (e *Engine) SetCell(track, step int, velocity float64) {
	if !validTrack(track) || !validStep(step) {
		return
	}

	vel, ok := validFloat(velocity, 0, 1)
	if !ok {
		return
	}

	e.pattern[track][step] = vel
}

const PatternSize = TrackCount * MaxSteps

// SetPattern atomically replaces the full flat track-major pattern (index =
// track*MaxSteps + step). A wrong-sized snapshot or any non-finite entry is
// rejected as a whole; finite velocities are clamped to [0, 1].
func (e *Engine) SetPattern(velocities []float64) {
	if len(velocities) != PatternSize {
		return
	}

	for _, velocity := range velocities {
		if _, ok := validFloat(velocity, 0, 1); !ok {
			return
		}
	}

	for i, velocity := range velocities {
		vel, _ := validFloat(velocity, 0, 1)
		e.pattern[i/MaxSteps][i%MaxSteps] = vel
	}
}

// CopyPattern writes the full pattern into caller-owned storage without
// allocating. float32 matches the WASM wire format.
func (e *Engine) CopyPattern(dst *[PatternSize]float32) {
	for track := range e.pattern {
		for step, velocity := range e.pattern[track] {
			dst[track*MaxSteps+step] = float32(velocity)
		}
	}
}

// SetVolume sets per-track volume, clamped to [0, 1]. The change ramps in
// over ~volSmoothTauS inside Render to avoid zipper noise. An out-of-range
// track or a non-finite volume is a silent no-op (see SetCell).
func (e *Engine) SetVolume(track int, vol float64) {
	if !validTrack(track) {
		return
	}

	volume, ok := validFloat(vol, 0, 1)
	if !ok {
		return
	}

	e.volumes[track] = volume
}

// SetDecay sets per-track decay amount, clamped to [0, 1]. An out-of-range
// track or a non-finite amount is a silent no-op (see SetCell).
func (e *Engine) SetDecay(track int, amount float64) {
	if !validTrack(track) {
		return
	}

	decay, ok := validFloat(amount, 0, 1)
	if !ok {
		return
	}

	e.decays[track] = decay
	e.voices[track].SetDecay(decay)
}

// SetVoiceParam sets one of a voice's synthesis parameters from a normalized
// [0, 1] position; see params.go for the per-voice tables. An out-of-range
// track or index, or a non-finite value, is a silent no-op (see SetCell).
//
// Unlike SetVolume/SetDecay the engine keeps no mirror of the value — it lives
// in the voice, reachable via Voice.Param.
func (e *Engine) SetVoiceParam(track, index int, value01 float64) {
	if !validTrack(track) {
		return
	}

	// Each Tom's procedural parameter bank remains authoritative while its
	// physical model is selected, so saved settings survive A/B switching.
	if validTomTrack(track) {
		e.proceduralToms[track].SetParam(index, value01)

		return
	}

	e.voices[track].SetParam(index, value01)
}

// SetPhysicalTomParam updates one Tom's independent physical parameter bank.
// It is valid while either model is selected so A/B edits survive a model
// switch. Invalid tracks, indices, and non-finite values are ignored.
func (e *Engine) SetPhysicalTomParam(track, index int, value01 float64) {
	if !validTomTrack(track) || index < 0 || index >= len(physicalTomSpecs) {
		return
	}

	if _, ok := validFloat(value01, 0, 1); !ok {
		return
	}

	physicalVoice, ok := e.ensurePhysicalTom(track)
	if !ok {
		return
	}

	physicalVoice.SetParam(index, value01)
}

func (e *Engine) ensurePhysicalTom(track int) (*physicalTom, bool) {
	if !validTomTrack(track) {
		return nil, false
	}

	if e.physicalToms[track] != nil {
		return e.physicalToms[track], true
	}

	physicalVoice, err := newPhysicalTom(e.sr)
	if err != nil {
		logErr("NewPhysicalTom", err)

		return nil, false
	}

	physicalVoice.SetDecay(e.decays[track])
	e.physicalToms[track] = physicalVoice

	return physicalVoice, true
}

// SetTomModel explicitly selects one Tom track's implementation. Procedural
// remains the default, and invalid tracks or values are ignored. Switching
// resets both sides so a dormant tail cannot resume later.
func (e *Engine) SetTomModel(track int, model TomModel) {
	if !validTomTrack(track) || model == e.tomModels[track] {
		return
	}

	var next Voice

	switch model {
	case TomModelProcedural:
		next = e.proceduralToms[track]
	case TomModelPhysical:
		physicalVoice, ok := e.ensurePhysicalTom(track)
		if !ok {
			return
		}

		next = physicalVoice
	default:
		return
	}

	if current, ok := e.voices[track].(interface{ Reset() }); ok {
		current.Reset()
	}

	if reset, ok := next.(interface{ Reset() }); ok {
		reset.Reset()
	}

	next.SetDecay(e.decays[track])

	e.voices[track] = next
	e.tomModels[track] = model
}

// TomModel reports one Tom track's selected implementation. Invalid tracks
// report the procedural default.
func (e *Engine) TomModel(track int) TomModel {
	if !validTomTrack(track) {
		return TomModelProcedural
	}

	return e.tomModels[track]
}

// TriggerVoice fires one voice immediately, independent of the sequencer, so
// the UI can audition a voice while the transport is stopped. An out-of-range
// track, a non-finite velocity, or a velocity of 0 is a silent no-op.
//
// Triggering advances a noise voice's RNG stream, so an audition shifts the
// noise a later rendered hit will draw — see docs/voices.md.
func (e *Engine) TriggerVoice(track int, velocity float64) {
	if !validTrack(track) {
		return
	}

	vel, ok := validFloat(velocity, 0, 1)
	if !ok || vel <= 0 {
		return
	}

	e.voices[track].Trigger(vel)
}

// SetReverb sets the target reverb amount in [0, 1]. Render smooths the wet
// gain to that target; 0 = fully dry, 1 = maximum (wet=reverbMaxWet, RT60=4 s).
// A non-finite amount is rejected and leaves the current setting unchanged.
func (e *Engine) SetReverb(amount float64) {
	wet, ok := validFloat(amount, 0, 1)
	if !ok {
		return
	}

	e.reverbAmount = wet

	if e.reverb == nil {
		return
	}

	if wet <= 0 {
		return
	}

	logErr("reverb.SetRT60", e.reverb.SetRT60(reverbMinRT60S+wet*reverbRangeRT60S))
}

func (e *Engine) CurrentStep() int {
	if e.transport == transportStopped {
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
	free := ^e.pendingMask
	if free == 0 {
		e.voices[track].Trigger(velocity)

		return
	}

	slot := bits.TrailingZeros32(free)
	e.pending[slot] = pendingTrigger{countdown: delay, track: track, velocity: velocity}
	e.pendingMask |= uint32(1) << slot
}

// firePending advances every queued trigger by one sample and fires those that
// have reached their scheduled time.
func (e *Engine) firePending() {
	for active := e.pendingMask; active != 0; {
		slot := bits.TrailingZeros32(active)
		bit := uint32(1) << slot
		active &^= bit

		trigger := &e.pending[slot]

		trigger.countdown--
		if trigger.countdown <= 0 {
			e.voices[trigger.track].Trigger(trigger.velocity)
			e.pendingMask &^= bit
		}
	}
}

// Render fills buf with mono audio samples.
//
// The invariants the loop relies on — a positive duration for every step, the
// playhead inside the loop, no pending trigger past its deadline — are checked
// per buffer, on entry and on exit, only in builds tagged `drumassert`; the
// shipped build compiles assertValid away to nothing (see assert.go).
func (e *Engine) Render(buf []float32) {
	e.assertValid()

	for i := range buf {
		if e.transport == transportPlaying {
			if !e.stepTriggered {
				e.triggerStep()
				e.stepTriggered = true
			}

			e.stepPhase += stepPhaseUnit
			if e.stepPhase >= e.currentStepDuration {
				e.stepPhase -= e.currentStepDuration
				e.currentStep = (e.currentStep + 1) % e.stepCount
				e.currentStepDuration = e.stepDuration[e.currentStep]
				e.stepTriggered = false
			}

			e.firePending()
		}

		var out float64

		for t, v := range e.voices {
			// One-pole ramp toward the target volume so knob moves during
			// playback do not step the gain per-sample (zipper noise).
			e.liveVol[t] += (e.volumes[t] - e.liveVol[t]) * e.volCoef
			out += v.Tick() * e.liveVol[t]
		}

		out *= mixHeadroom

		if e.reverb != nil {
			e.liveReverbAmount += (e.reverbAmount - e.liveReverbAmount) * e.volCoef
			logErr("reverb.SetWet", e.reverb.SetWet(e.liveReverbAmount*reverbMaxWet))
			out = e.reverb.ProcessSample(out)
		}

		if e.limiter != nil {
			out = e.limiter.ProcessSample(out)
		}

		// Hard safety clamp — anything past ±1.0 would be clipped by the
		// browser's output stage anyway; this guarantees the contract. ±Inf
		// falls into the comparisons; NaN does not, and a single NaN sample
		// silences the Web Audio graph until reload, so it is caught last and
		// muted. Parameters are validated at the setters (see validFloat), so
		// this branch should never be reachable.
		switch {
		case out > 1:
			out = 1
			e.hardClipCount++
		case out < -1:
			out = -1
			e.hardClipCount++
		case math.IsNaN(out):
			out = 0
		}

		buf[i] = float32(out)
	}

	// Checked again on the way out: the loop advances the playhead and the
	// pending set itself, so this catches corruption Render caused rather
	// than inherited. Deliberately not deferred — the loop cannot return
	// early, and a defer would not compile away in the untagged build.
	e.assertValid()
}
