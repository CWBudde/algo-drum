package drum

import (
	"math"
	"math/bits"
	"math/rand/v2"

	"github.com/cwbudde/algo-dsp/dsp/effects/reverb"
)

const (
	TrackCount       = 7
	PatternBankCount = 4
	MaxChainLength   = 16
	NoBank           = -1

	// MaxSteps is the pattern capacity. SetStepCount sets the master/displayed
	// loop and initializes every track to that length; SetTrackLength then lets
	// tracks diverge for polymeter. Steps are 16th notes, so 16 steps span one
	// 4/4 bar.
	MaxSteps = 16

	// mixHeadroom scales the summed voice mix before the master chain. It does
	// not buy enough headroom to keep the limiter idle: measured at 48 kHz, a
	// solo hi-hat already peaks at +1.7 dBFS after this scaling and an ordinary
	// bass/snare/hat pattern reaches +5.1 dBFS, so the limiter does routine
	// transient reduction rather than only catching worst cases. That is the
	// shipped balance — the per-voice gains it comes from are user-facing
	// parameter defaults (hat ranges to 2.5), so no static scalar here can bound
	// the mix; only the limiter can. See PLAN.md E7.
	mixHeadroom = 0.5

	// volSmoothTauS is the per-track volume ramp time constant. ~8 ms feels
	// instant on a knob but is long enough to avoid zipper noise.
	volSmoothTauS = 0.008

	// engineSilence is the output magnitude at or below which a rendered sample
	// counts as silence for idle detection. 1e-6 is about −120 dBFS: two orders
	// of magnitude below the ±1 LSB of 16-bit audio and far below anything a
	// listener or an output device can resolve, so treating it as zero is
	// inaudible rather than merely quiet.
	engineSilence = 1e-6

	// idleConfirmS is how long the output must stay below engineSilence before
	// Render takes its idle path. 50 ms is long enough that a waveform passing
	// through a zero crossing, or a voice envelope dipping between partials,
	// cannot be mistaken for a decayed tail, and short enough that the CPU wakes
	// only for the tail end of a stop.
	idleConfirmS = 0.05

	// engineSeed seeds the probability/humanize randomness so renders stay
	// reproducible run-to-run (the voices are seeded the same way).
	engineSeed = 0x5eed

	// humanizeTimingMaxS is the largest absolute timing jitter at humanize=1.
	// Steady-state hits are scheduled with one-step lookahead in the centered
	// interval [-7.5 ms, +7.5 ms]. The first step after Play has no causal
	// pre-roll, so its negative offsets clamp to the start boundary.
	humanizeTimingMaxS = 0.0075

	// humanizeVelMax is the largest velocity deviation at humanize=1: each hit's
	// velocity is scaled by 1 ± a random fraction up to humanize·humanizeVelMax.
	humanizeVelMax = 0.20

	// maxPending caps in-flight humanize-delayed and ratcheted triggers. At the
	// shortest legal swung step, seven four-hit ratchets from the next step can
	// overlap the tail of the current step, so keep a full 64-bit fixed set.
	maxPending = 64

	minCellRepeats = 1
	maxCellRepeats = 4

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

// patternBank is the allocation-free render representation of one bank.
// PatternBankState is its owned, slice-backed snapshot at the state boundary.
type patternBank struct {
	stepCount       int
	pattern         [TrackCount][MaxSteps]float64
	cellProbability [TrackCount][MaxSteps]float64
	cellHumanize    [TrackCount][MaxSteps]float64
	cellCondition   [TrackCount][MaxSteps]TriggerCondition
	cellRepeats     [TrackCount][MaxSteps]uint8
	trackLength     [TrackCount]int
}

// TriggerCondition controls whether an active cell is eligible on a given
// pass through its track's independent loop. The numeric values are part of
// the EngineState/WASM wire contract; append new values rather than reordering
// these constants.
type TriggerCondition uint8

const (
	TriggerAlways TriggerCondition = iota
	TriggerEvery2
	TriggerEvery3
	TriggerEvery4
	TriggerFirstLoop
	TriggerFillOnly
	TriggerNotPreviousFired
	triggerConditionCount
)

type transportState uint8

const (
	transportStopped transportState = iota
	transportStarting
	transportPlaying
	transportPaused
)

// TransportState is the semantic transport state exposed at the WASM
// boundary. The sequencer remains the authority for this value; the worker and
// UI only mirror snapshots returned by TransportSnapshot.
type TransportState string

const (
	TransportStopped  TransportState = "stopped"
	TransportStarting TransportState = "starting"
	TransportPlaying  TransportState = "playing"
	TransportPaused   TransportState = "paused"
)

// TransportSnapshot identifies one transport epoch and its playhead. Revision
// changes at every state transition, which lets the main thread reject chunks
// rendered before a Stop, Pause or restart even when they reach the speakers
// later from the worklet's queue.
type TransportSnapshot struct {
	State    TransportState
	Step     int
	Revision uint64
}

// Engine is the drum machine sequencer and mixer.
type Engine struct {
	sr        float64
	transport transportState
	// transportRevision is monotonic for the lifetime of an engine. It is
	// deliberately runtime-only: loading a preset must not make buffered audio
	// from the current transport epoch stale.
	transportRevision uint64
	bpm               float64
	swing             float64 // 0.0 = no swing, 0.5 = full shuffle

	pattern         [TrackCount][MaxSteps]float64 // per-cell velocity in [0, 1], 0 = off
	cellProbability [TrackCount][MaxSteps]float64 // multiplied by the global probability
	cellHumanize    [TrackCount][MaxSteps]float64 // multiplied by the global humanize amount
	cellCondition   [TrackCount][MaxSteps]TriggerCondition
	cellRepeats     [TrackCount][MaxSteps]uint8 // evenly spaced hits per eligible cell, in [1, 4]
	volumes         [TrackCount]float64         // targets set by SetVolume
	muted           [TrackCount]bool            // mutes do not overwrite the stored volumes
	liveVol         [TrackCount]float64         // smoothed volumes applied in Render
	volCoef         float64                     // per-sample one-pole ramp coefficient
	decays          [TrackCount]float64

	voices [TrackCount]Voice

	tomModels      [TrackCount]TomModel
	proceduralToms [TrackCount]*Tom
	physicalToms   [TrackCount]*physicalTom
	// physicalTomParams is authoritative even before the comparatively heavy
	// physical voice is constructed. State snapshots therefore preserve both
	// Tom banks without turning a procedural-only session into two physical
	// model allocations.
	physicalTomParams [TrackCount][]float64

	stepCount           int // master/displayed loop length in [1, MaxSteps]
	currentStep         int
	clockStep           uint64 // absolute step clock since the last Stop/master reset
	stepPhase           uint64 // elapsed Q32.32 samples in the current step
	currentStepDuration uint64 // latched Q32.32 duration of the current step
	stepDuration        [MaxSteps]uint64
	stepTriggered       bool
	trackLength         [TrackCount]int
	trackStep           [TrackCount]int
	trackPass           [TrackCount]uint64
	previousFired       [TrackCount]bool
	nextFired           [TrackCount]bool
	nextStepScheduled   bool
	nextBank            int
	nextChainPosition   int

	banks          [PatternBankCount]patternBank
	standaloneBank int
	activeBank     int
	queuedBank     int
	chainEnabled   bool
	chain          [MaxChainLength]int
	chainLength    int
	chainPosition  int

	prob              float64 // per-hit trigger probability in [0, 1]
	humanize          float64 // timing/velocity randomization amount in [0, 1]
	humanizeLookahead int     // centered timing lookahead in whole samples
	fillMode          bool    // enables cells carrying TriggerFillOnly
	rng               *rand.Rand
	pending           [maxPending]pendingTrigger
	pendingMask       uint64

	reverb           *reverb.FDNReverb
	reverbAmount     float64
	liveReverbAmount float64
	limiter          *peakLimiter
	hardClipCount    uint64

	// silentRun counts consecutive rendered samples that were below
	// engineSilence while nothing could make the engine loud again; it
	// saturates at idleSamples, which is all IsIdle needs to know.
	silentRun   int64
	idleSamples int64 // idleConfirmS in samples, the confirm window for IsIdle
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
		// At the clamped rate floor this is still 400 samples, so the window
		// can never degenerate to "idle on the first quiet sample".
		idleSamples:       int64(math.Round(sr * idleConfirmS)),
		prob:              1,
		humanize:          0,
		humanizeLookahead: int(math.Ceil(sr * humanizeTimingMaxS)),
		rng:               rand.New(rand.NewPCG(engineSeed, engineSeed)),
		queuedBank:        NoBank,
		nextBank:          NoBank,
		chainLength:       1,
	}
	for i := range e.volumes {
		e.volumes[i] = 1.0
		e.liveVol[i] = 1.0

		e.decays[i] = 0.5
		for bank := range PatternBankCount {
			e.banks[bank].stepCount = MaxSteps

			e.banks[bank].trackLength[i] = MaxSteps
			for step := range MaxSteps {
				e.banks[bank].cellProbability[i][step] = 1
				e.banks[bank].cellHumanize[i][step] = 1
				e.banks[bank].cellRepeats[i][step] = minCellRepeats
			}
		}
	}

	e.chain[0] = 0
	e.loadBank(0)

	e.voices[0] = NewBassDrum(sr)
	e.voices[1] = NewSnare(sr)
	e.voices[2] = NewHiHat(sr)
	e.proceduralToms[tomTrackIndex] = NewTom(sr)
	e.voices[tomTrackIndex] = e.proceduralToms[tomTrackIndex]
	e.physicalTomParams[tomTrackIndex] = defaultParams(physicalTomSpecs)

	e.voices[4] = NewCymbal(sr)
	e.proceduralToms[tom2TrackIndex] = NewTom2(sr)
	e.voices[tom2TrackIndex] = e.proceduralToms[tom2TrackIndex]
	e.physicalTomParams[tom2TrackIndex] = defaultParams(physicalTomSpecs)

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
	for step := range e.stepDuration {
		e.stepDuration[step] = e.stepDurationFor(e.stepCount, step)
	}

	// Parameter edits are latched at the next boundary while playing or
	// paused, avoiding a shortened step suddenly ending mid-buffer. A stopped
	// transport starts with the newly configured duration immediately.
	if e.transport == transportStopped || e.currentStepDuration == 0 {
		e.currentStepDuration = e.stepDuration[e.currentStep]
	}
}

func (e *Engine) stepDurationFor(stepCount, step int) uint64 {
	base := e.sr * secondsPerMinute / e.bpm / stepsPerBeat // samples per 16th note
	plain := uint64(math.Round(base * float64(stepPhaseUnit)))
	delta := uint64(math.Round(base * e.swing * float64(stepPhaseUnit)))
	long := plain + delta
	short := plain - delta
	last := stepCount - 1
	unpaired := step > last || (step == last && stepCount%2 == 1)

	switch {
	case unpaired:
		return plain
	case step%2 == 0:
		return long
	default:
		return short
	}
}

func (e *Engine) setTransport(state transportState) {
	if e.transport == state {
		return
	}

	e.transport = state
	e.transportRevision++
}

// BeginStart records a requested start before the browser's asynchronous
// audio graph is ready. Render does not advance the sequencer in this state;
// SetRunning(true) commits the transition once audio output has resumed.
func (e *Engine) BeginStart() {
	if e.transport == transportPlaying || e.transport == transportStarting {
		return
	}

	e.setTransport(transportStarting)
}

func (e *Engine) SetRunning(running bool) {
	if running {
		e.setTransport(transportPlaying)
		e.wake()

		return
	}

	e.setTransport(transportStopped)
	e.queuedBank = NoBank
	e.chainPosition = 0

	target := e.standaloneBank
	if e.chainEnabled {
		target = e.chain[0]
	}

	if target != e.activeBank {
		e.loadBank(target)
		e.recomputeStepDurations()
	}

	e.resetSequencer()
}

func (e *Engine) resetSequencer() {
	e.currentStep = 0
	e.clockStep = 0
	e.stepPhase = 0
	e.currentStepDuration = e.stepDuration[0]
	e.stepTriggered = false
	e.nextStepScheduled = false
	e.nextBank = NoBank
	e.nextChainPosition = 0
	e.nextFired = [TrackCount]bool{}

	for track := range TrackCount {
		e.trackStep[track] = 0
		e.trackPass[track] = 0
		e.previousFired[track] = false
	}

	// Drop any humanize-delayed hits so they don't fire after restart.
	e.pendingMask = 0
}

// Pause freezes sequencer time and delayed humanized hits while allowing
// already-triggered voices and effects to ring out. SetRunning(true) resumes
// from the held fractional position; SetRunning(false) performs a full stop.
func (e *Engine) Pause() {
	if e.transport == transportPlaying {
		e.setTransport(transportPaused)
	}
}

// SetProbability sets the global probability multiplier, clamped to [0, 1].
// Each hit's effective chance is this value times its CellProbability. A
// global value of 1 leaves cell probabilities unscaled (the default), while 0
// silences every sequenced hit. A non-finite value is rejected and leaves the
// current probability unchanged.
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

// SetStepCount sets the master/displayed loop length, clamped to [1, MaxSteps],
// and applies it to every track as the backwards-compatible global-length
// operation. Call SetTrackLength afterward to create a polymeter. Cells beyond
// a new length keep their contents (see SetCell). Step durations are recomputed
// because swing pairs steps within the master loop.
func (e *Engine) SetStepCount(bank, count int) {
	if !validBank(bank) {
		return
	}

	if count < 1 {
		count = 1
	}

	if count > MaxSteps {
		count = MaxSteps
	}

	stored := &e.banks[bank]

	stored.stepCount = count
	for track := range TrackCount {
		stored.trackLength[track] = count
	}

	if bank != e.activeBank {
		return
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

	e.clockStep = uint64(e.currentStep)

	// The legacy master-length control is also the quick way to bring a
	// polymeter back into phase: all track loops adopt the master length and
	// align to its current step. Independent edits made afterward continue
	// across master wraps in SetTrackLength's polymetric mode.
	for track := range TrackCount {
		e.trackLength[track] = count
		e.trackStep[track] = e.currentStep
		e.trackPass[track] = 0
		e.previousFired[track] = false
	}

	e.syncActiveBank()

	e.recomputeStepDurations()
}

// SetTrackLength sets one track's independent loop length, clamped to
// [1, MaxSteps]. The master StepCount continues to define the displayed
// playhead and swing loop; track playheads advance continuously across master
// wraps, which is what makes non-dividing lengths a true polymeter.
func (e *Engine) SetTrackLength(bank, track, count int) {
	if !validBank(bank) || !validTrack(track) {
		return
	}

	if count < 1 {
		count = 1
	} else if count > MaxSteps {
		count = MaxSteps
	}

	e.banks[bank].trackLength[track] = count

	if bank != e.activeBank {
		return
	}

	e.setTrackLength(track, count)
	e.syncActiveBank()
}

func (e *Engine) setTrackLength(track, count int) {
	if count < 1 {
		count = 1
	} else if count > MaxSteps {
		count = MaxSteps
	}

	if e.trackLength[track] == count {
		return
	}

	e.trackLength[track] = count
	e.trackStep[track] = int(e.clockStep % uint64(count))
	// A length edit defines a new loop, so conditional-pass history starts
	// over instead of inheriting a pass number from a different cycle shape.
	e.trackPass[track] = 0
	e.previousFired[track] = false
}

// SetCell sets a cell's velocity, clamped to [0, 1] (0 = off). Steps are
// addressable up to MaxSteps regardless of the active step count, so
// shrinking and re-growing the pattern is lossless.
//
// Out-of-range contract: an invalid track or step index is a silent no-op, as
// is a non-finite velocity — the cell keeps its previous value. Every indexed
// setter behaves this way (SetVolume, SetDecay), because the JS bridge feeds
// unvalidated arguments straight through and must never take the engine down.
func (e *Engine) SetCell(bank, track, step int, velocity float64) {
	if !validBank(bank) || !validTrack(track) || !validStep(step) {
		return
	}

	vel, ok := validFloat(velocity, 0, 1)
	if !ok {
		return
	}

	e.banks[bank].pattern[track][step] = vel
	if bank == e.activeBank {
		e.pattern[track][step] = vel
	}
}

// SetCellProbability sets one cell's probability multiplier. It is combined
// with the global Probability control at trigger time. Defaults are 1, so old
// patterns and the mechanical render path remain sample-exact.
func (e *Engine) SetCellProbability(bank, track, step int, probability float64) {
	if !validBank(bank) || !validTrack(track) || !validStep(step) {
		return
	}

	value, ok := validFloat(probability, 0, 1)
	if !ok {
		return
	}

	e.banks[bank].cellProbability[track][step] = value
	if bank == e.activeBank {
		e.cellProbability[track][step] = value
	}
}

// SetCellHumanize sets one cell's multiplier for the global Humanize amount.
// Defaults are 1, so older patterns retain their existing global response.
func (e *Engine) SetCellHumanize(bank, track, step int, amount float64) {
	if !validBank(bank) || !validTrack(track) || !validStep(step) {
		return
	}

	value, ok := validFloat(amount, 0, 1)
	if !ok {
		return
	}

	e.banks[bank].cellHumanize[track][step] = value
	if bank == e.activeBank {
		e.cellHumanize[track][step] = value
	}
}

// SetCellCondition sets one cell's loop/fill condition. Unknown numeric codes
// are rejected so persisted state cannot silently acquire different semantics.
func (e *Engine) SetCellCondition(bank, track, step int, condition TriggerCondition) {
	if !validBank(bank) || !validTrack(track) || !validStep(step) || condition >= triggerConditionCount {
		return
	}

	e.banks[bank].cellCondition[track][step] = condition
	if bank == e.activeBank {
		e.cellCondition[track][step] = condition
	}
}

// SetCellRepeats sets the number of evenly spaced hits emitted by an eligible
// cell. Invalid indexes no-op and counts clamp to [1, 4].
func (e *Engine) SetCellRepeats(bank, track, step, repeats int) {
	if !validBank(bank) || !validTrack(track) || !validStep(step) {
		return
	}

	if repeats < minCellRepeats {
		repeats = minCellRepeats
	} else if repeats > maxCellRepeats {
		repeats = maxCellRepeats
	}

	value := uint8(repeats)

	e.banks[bank].cellRepeats[track][step] = value
	if bank == e.activeBank {
		e.cellRepeats[track][step] = value
	}
}

// SetFillMode enables or disables cells marked TriggerFillOnly. It is semantic
// configuration state rather than transport state so snapshots and shares can
// reproduce what the engine will play.
func (e *Engine) SetFillMode(enabled bool) {
	e.fillMode = enabled
}

const PatternSize = TrackCount * MaxSteps

// SetPattern atomically replaces the full flat track-major pattern (index =
// track*MaxSteps + step). A wrong-sized snapshot or any non-finite entry is
// rejected as a whole; finite velocities are clamped to [0, 1].
func (e *Engine) SetPattern(bank int, velocities []float64) {
	if !validBank(bank) || len(velocities) != PatternSize {
		return
	}

	for _, velocity := range velocities {
		if _, ok := validFloat(velocity, 0, 1); !ok {
			return
		}
	}

	for i, velocity := range velocities {
		vel, _ := validFloat(velocity, 0, 1)

		e.banks[bank].pattern[i/MaxSteps][i%MaxSteps] = vel

		if bank == e.activeBank {
			e.pattern[i/MaxSteps][i%MaxSteps] = vel
		}
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

// SetMuted changes one track's mute state without changing its stored volume.
// Render ramps toward zero while muted and back toward the stored volume when
// unmuted, so both transitions inherit the zipper-noise protection of the
// volume control.
func (e *Engine) SetMuted(track int, muted bool) {
	if !validTrack(track) {
		return
	}

	wasMuted := e.muted[track]

	e.muted[track] = muted
	if wasMuted && !muted {
		// A muted tail may have taken the engine into its frozen idle path.
		// Give it a chance to resume when its stored volume becomes audible.
		e.wake()
	}
}

// Muted reports one track's mute state. Invalid tracks report false.
func (e *Engine) Muted(track int) bool {
	if !validTrack(track) {
		return false
	}

	return e.muted[track]
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

	value, ok := validFloat(value01, 0, 1)
	if !ok {
		return
	}

	// The shadow is sufficient while the physical bank is inactive. This is
	// intentionally lazy: editing or restoring its controls must not construct
	// a DoubleHead until the user selects it.
	physicalVoice := e.physicalToms[track]
	if physicalVoice == nil {
		e.physicalTomParams[track][index] = value

		return
	}

	physicalVoice.SetParam(index, value)
	// SetParam rolls itself back if the derived physical configuration is not
	// valid. Mirror what it actually accepted rather than assuming success.
	e.physicalTomParams[track][index] = physicalVoice.Param(index)
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

	if err := physicalVoice.replaceParams(e.physicalTomParams[track]); err != nil {
		logErr("physical Tom shadow parameters", err)

		return nil, false
	}

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

	e.wake()
	e.voices[track].Trigger(vel)
}

// wake cancels an in-progress (or reached) idle window. Every path that can
// make a stopped engine loud again has to call it, because the idle fast path
// in Render writes zeros without ticking the voices and so cannot notice the
// new sound on its own. Starting the transport, auditioning a voice and
// unmuting a possibly frozen tail all pass through here; triggerStep and
// schedule only run while the transport is playing, which holds the counter at
// zero anyway.
func (e *Engine) wake() {
	e.silentRun = 0
}

// IsIdle reports whether Render is producing nothing but silence and can be
// stopped being called: the output has stayed below engineSilence for
// idleConfirmS while the transport was not playing and no humanize-delayed hit
// was armed. It is the engine's half of the "stop the audio graph when there is
// nothing to hear" contract — the worklet/worker side decides what to do with
// it.
//
// This truncates a decaying tail rather than rendering it to the last denormal:
// the reverb and the voice envelopes are exponential and never reach exactly
// zero, so waiting for a bit-exact zero would mean never idling at all. What is
// discarded is everything below −120 dBFS, which is over two orders of magnitude
// under the quantisation step of the 16-bit output it eventually reaches and far
// under the noise floor of any playback chain, so the cut is inaudible by
// construction. The consequence to be aware of is that renders are no longer
// bit-identical to a build without idling once a tail crosses the threshold —
// hence the tests hold idling to "nothing above engineSilence was lost" rather
// than to sample equality.
func (e *Engine) IsIdle() bool {
	return e.silentRun >= e.idleSamples
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
	if e.transport == transportStopped || e.transport == transportStarting {
		return -1
	}

	return e.currentStep
}

// TransportSnapshot returns the engine-owned transport state, its logical
// playhead and a revision that identifies the current transport epoch.
func (e *Engine) TransportSnapshot() TransportSnapshot {
	state := TransportStopped

	switch e.transport {
	case transportStarting:
		state = TransportStarting
	case transportPlaying:
		state = TransportPlaying
	case transportPaused:
		state = TransportPaused
	}

	return TransportSnapshot{
		State:    state,
		Step:     e.CurrentStep(),
		Revision: e.transportRevision,
	}
}

// triggerFirstStep evaluates the first step without a pre-roll. Positive
// offsets are delayed; negative offsets clamp to the start boundary because
// audio before Play cannot be rendered causally.
func (e *Engine) triggerFirstStep() {
	e.previousFired = e.scheduleBankStep(
		&e.banks[e.activeBank], e.trackStep, e.trackPass, e.previousFired,
		1, e.currentStepDuration, true,
	)
}

// scheduleNextStep commits the next rhythmic decision inside the centered
// timing lookahead. Bank requests arriving afterward intentionally wait for
// the next eligible master wrap: audio for this boundary has already been
// rendered into pending triggers.
func (e *Engine) scheduleNextStep(samplesToBoundary int) {
	targetBank := e.activeBank
	targetChainPosition := e.chainPosition
	targetMasterStep := (e.currentStep + 1) % e.stepCount

	masterWrap := e.currentStep+1 >= e.stepCount
	if masterWrap {
		switch {
		case e.chainEnabled:
			targetChainPosition = (e.chainPosition + 1) % e.chainLength
			targetBank = e.chain[targetChainPosition]
		case e.queuedBank != NoBank:
			targetBank = e.queuedBank
		}
	}

	var (
		steps  [TrackCount]int
		passes [TrackCount]uint64
	)

	previous := e.previousFired
	if targetBank == e.activeBank {
		for track := range TrackCount {
			steps[track] = e.trackStep[track] + 1

			passes[track] = e.trackPass[track]
			if steps[track] >= e.trackLength[track] {
				steps[track] = 0
				passes[track]++
			}
		}
	} else {
		previous = [TrackCount]bool{}
		targetMasterStep = 0
	}

	targetDuration := e.stepDuration[targetMasterStep]
	if targetBank != e.activeBank {
		targetDuration = e.stepDurationFor(e.banks[targetBank].stepCount, targetMasterStep)
	}

	e.nextFired = e.scheduleBankStep(
		&e.banks[targetBank], steps, passes, previous,
		samplesToBoundary+1, targetDuration, false,
	)
	e.nextBank = targetBank
	e.nextChainPosition = targetChainPosition
	e.nextStepScheduled = true
}

// scheduleBankStep decides condition/probability at the nominal step and
// queues its voices relative to a future boundary. It returns whether each
// gate was accepted; conditional history follows the grid, not the possibly
// early/late sample where a voice happens to sound.
func (e *Engine) scheduleBankStep(
	bank *patternBank,
	steps [TrackCount]int,
	passes [TrackCount]uint64,
	previous [TrackCount]bool,
	baseDelay int,
	stepDuration uint64,
	first bool,
) [TrackCount]bool {
	var fired [TrackCount]bool

	for track := range e.voices {
		step := steps[track]

		vel := bank.pattern[track][step]
		if vel <= 0 || !e.conditionAllows(bank.cellCondition[track][step], passes[track], previous[track]) {
			continue
		}

		probability := e.prob * bank.cellProbability[track][step]
		if probability < 1 && e.rng.Float64() >= probability {
			continue
		}

		fired[track] = true

		effective := e.humanize * bank.cellHumanize[track][step]
		velocity := vel
		jitter := 0
		// Keep the count=1, humanize=0 path free of RNG calls and sample-exact
		// with every persisted state predating ratchets.
		if effective > 0 {
			velocity = clamp01(vel * (1 + (e.rng.Float64()*2-1)*effective*humanizeVelMax))
			jitter = int((e.rng.Float64()*2 - 1) * effective * humanizeTimingMaxS * e.sr)
		}

		repeats := int(bank.cellRepeats[track][step])
		for repeat := range repeats {
			offset := ratchetOffsetSamples(stepDuration, repeat, repeats)

			delay := baseDelay + offset + jitter
			if first && delay <= 1 {
				e.voices[track].Trigger(velocity)
			} else if delay <= 0 {
				e.voices[track].Trigger(velocity)
			} else {
				e.schedule(track, velocity, delay)
			}
		}
	}

	return fired
}

func ratchetOffsetSamples(stepDuration uint64, repeat, repeats int) int {
	if repeat == 0 || repeats <= 1 {
		return 0
	}

	phase := stepDuration * uint64(repeat) / uint64(repeats)

	return int((phase + stepPhaseUnit/2) / stepPhaseUnit)
}

func (e *Engine) conditionAllows(condition TriggerCondition, pass uint64, previous bool) bool {
	switch condition {
	case TriggerAlways:
		return true
	case TriggerEvery2:
		return (pass+1)%2 == 0
	case TriggerEvery3:
		return (pass+1)%3 == 0
	case TriggerEvery4:
		return (pass+1)%4 == 0
	case TriggerFirstLoop:
		return pass == 0
	case TriggerFillOnly:
		return e.fillMode
	case TriggerNotPreviousFired:
		return !previous
	default:
		return false
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

	slot := bits.TrailingZeros64(free)
	e.pending[slot] = pendingTrigger{countdown: delay, track: track, velocity: velocity}
	e.pendingMask |= uint64(1) << slot
}

// firePending advances every queued trigger by one sample and fires those that
// have reached their scheduled time.
func (e *Engine) firePending() {
	for active := e.pendingMask; active != 0; {
		slot := bits.TrailingZeros64(active)
		bit := uint64(1) << slot
		active &^= bit

		trigger := &e.pending[slot]

		trigger.countdown--
		if trigger.countdown <= 0 {
			e.voices[trigger.track].Trigger(trigger.velocity)
			e.pendingMask &^= bit
		}
	}
}

func (e *Engine) commitScheduledBoundary() {
	targetBank := e.nextBank
	if targetBank != e.activeBank {
		// A different bank is a new rhythmic phrase: master and track phases,
		// pass history and fractional step phase all restart at step zero.
		e.loadBank(targetBank)
		e.recomputeStepDurations()
		e.currentStep = 0
		e.clockStep = 0
		e.stepPhase = 0
		e.currentStepDuration = e.stepDuration[0]

		for track := range TrackCount {
			e.trackStep[track] = 0
			e.trackPass[track] = 0
		}
	} else {
		e.clockStep++
		e.currentStep = (e.currentStep + 1) % e.stepCount
		e.currentStepDuration = e.stepDuration[e.currentStep]

		for track := range TrackCount {
			e.trackStep[track]++
			if e.trackStep[track] >= e.trackLength[track] {
				e.trackStep[track] = 0
				e.trackPass[track]++
			}
		}
	}

	// A duplicate bank in a chain still advances the song cursor while the
	// same-bank branch above deliberately preserves polymetric/pass phase.
	e.chainPosition = e.nextChainPosition
	if e.queuedBank == targetBank {
		e.queuedBank = NoBank
	}

	e.previousFired = e.nextFired
	e.nextFired = [TrackCount]bool{}
	e.stepTriggered = true
	e.nextStepScheduled = false
	e.nextBank = NoBank
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
				e.triggerFirstStep()
				e.stepTriggered = true
			}

			remainingPhase := e.currentStepDuration - e.stepPhase

			remainingSamples := int((remainingPhase + stepPhaseUnit - 1) / stepPhaseUnit)
			if !e.nextStepScheduled && remainingSamples <= e.humanizeLookahead {
				e.scheduleNextStep(remainingSamples)
			}

			e.stepPhase += stepPhaseUnit
			if e.stepPhase >= e.currentStepDuration {
				e.stepPhase -= e.currentStepDuration
				if e.nextStepScheduled {
					e.commitScheduledBoundary()
				} else {
					// Defensive fallback: current production bounds guarantee the
					// lookahead is shorter than every legal step duration.
					e.clockStep++
					e.currentStep = (e.currentStep + 1) % e.stepCount
					e.currentStepDuration = e.stepDuration[e.currentStep]
					e.stepTriggered = false
				}
			}

			e.firePending()
		}

		// Idle fast path: with the transport stopped and the tail decayed away
		// there is nothing for the voices, the reverb or the limiter to compute,
		// and running them anyway is what kept the CPU busy forever (PLAN.md
		// B10). Their state is left frozen rather than reset, so a later hit
		// resumes from exactly where the silence began. The check is per sample
		// so a buffer that goes idle halfway stops working halfway.
		if e.IsIdle() {
			buf[i] = 0

			continue
		}

		var out float64

		for t, v := range e.voices {
			// One-pole ramp toward the target volume so knob moves during
			// playback do not step the gain per-sample (zipper noise).
			targetVolume := e.volumes[t]
			if e.muted[t] {
				targetVolume = 0
			}

			e.liveVol[t] += (targetVolume - e.liveVol[t]) * e.volCoef
			out += v.Tick() * e.liveVol[t]
		}

		out *= mixHeadroom

		if e.reverb != nil {
			e.liveReverbAmount += (e.reverbAmount - e.liveReverbAmount) * e.volCoef

			// FDNReverb returns input*dry + tail*wet and its dry gain defaults
			// to 1, so setting only the wet gain would make REVERB a send that
			// raises the master level rather than a mix. Trading dry for wet
			// keeps the level flat across the sweep, which is what a 0–1
			// "amount" knob means and what keeps the limiter off the tail.
			// reverbMaxWet < 1, so the dry gain never goes negative.
			wet := e.liveReverbAmount * reverbMaxWet
			logErr("reverb.SetWet", e.reverb.SetWet(wet))
			logErr("reverb.SetDry", e.reverb.SetDry(1-wet))

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

		// Silence accounting, on the same value that reaches the buffer. A
		// playing transport or an armed pending hit means audio is coming
		// regardless of how quiet this sample is, so neither can be allowed to
		// accumulate a window. The counter saturates at the window length: it
		// has nothing left to prove past that point, and not growing keeps it
		// from running away over a long idle.
		if e.transport != transportPlaying && e.pendingMask == 0 && math.Abs(out) < engineSilence {
			if e.silentRun < e.idleSamples {
				e.silentRun++
			}
		} else {
			e.silentRun = 0
		}

		buf[i] = float32(out)
	}

	// Checked again on the way out: the loop advances the playhead and the
	// pending set itself, so this catches corruption Render caused rather
	// than inherited. Deliberately not deferred — the loop cannot return
	// early, and a defer would not compile away in the untagged build.
	e.assertValid()
}
