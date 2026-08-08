package drum

import "math"

const (
	metronomeDecayS       = 0.012
	metronomeRegularHz    = 1100.0
	metronomeAccentHz     = 1760.0
	metronomeRegularLevel = 0.16
	metronomeAccentLevel  = 0.24
)

// metronomeClick is a deliberately small, deterministic click generator. It
// is separate from the seven user voices so the guide pulse cannot consume a
// noise RNG stream or inherit mixer, probability, humanize or voice settings.
type metronomeClick struct {
	sr       float64
	phase    float64
	phaseInc float64
	envelope float64
	decay    float64
}

func newMetronomeClick(sr float64) metronomeClick {
	return metronomeClick{
		sr:    sr,
		decay: math.Exp(-1 / (sr * metronomeDecayS)),
	}
}

func (c *metronomeClick) Trigger(accent bool) {
	frequency := metronomeRegularHz
	level := metronomeRegularLevel

	if accent {
		frequency = metronomeAccentHz
		level = metronomeAccentLevel
	}

	c.phase = math.Pi / 2
	c.phaseInc = 2 * math.Pi * frequency / c.sr
	c.envelope = level
}

func (c *metronomeClick) Tick() float64 {
	if c.envelope < envSilence {
		c.envelope = 0

		return 0
	}

	out := math.Sin(c.phase) * c.envelope

	c.phase += c.phaseInc
	if c.phase >= 2*math.Pi {
		c.phase -= 2 * math.Pi
	}

	c.envelope *= c.decay

	return out
}

// SetMetronomeEnabled controls quarter-note clicks during ordinary playback.
// Count-in clicks are always audible regardless of this setting.
func (e *Engine) SetMetronomeEnabled(enabled bool) {
	e.metronomeEnabled = enabled
}

// SetCountInBars selects a stopped-start count-in of zero, one or two fixed
// 4/4 bars. A count already in progress keeps the length latched at its start.
func (e *Engine) SetCountInBars(bars int) {
	if bars < 0 {
		bars = 0
	} else if bars > MaxCountInBars {
		bars = MaxCountInBars
	}

	e.countInBars = bars
}

func (e *Engine) startCountIn() {
	e.countInStep = 0
	e.countInTotalSteps = e.countInBars * MaxSteps
	e.countInPhase = 0
	e.countInDuration = e.stepDurationFor(MaxSteps, 0)
	e.countInTriggered = false
	e.setTransport(transportCountingIn)
	e.wake()
}

func (e *Engine) resetCountIn() {
	e.countInStep = 0
	e.countInTotalSteps = 0
	e.countInPhase = 0
	e.countInDuration = 0
	e.countInTriggered = false
}

// triggerFirstStep evaluates the first step without a pre-roll. Positive
// offsets are delayed; negative offsets clamp to the start boundary because
// audio before Play cannot be rendered causally.
func (e *Engine) triggerFirstStep() {
	if e.metronomeEnabled {
		e.click.Trigger(true)
	}

	e.previousFired = e.scheduleBankStep(
		&e.banks[e.activeBank], e.trackStep, e.trackPass, e.previousFired,
		1, e.currentStepDuration, true,
	)
}

func (e *Engine) renderCountInSample() {
	if !e.countInTriggered {
		if e.countInStep%int(stepsPerBeat) == 0 {
			e.click.Trigger(e.countInStep%MaxSteps == 0)
		}

		e.countInTriggered = true
	}

	e.countInPhase += stepPhaseUnit
	if e.countInPhase < e.countInDuration {
		return
	}

	e.countInPhase -= e.countInDuration
	e.countInStep++

	if e.countInStep >= e.countInTotalSteps {
		e.resetCountIn()
		e.setTransport(transportPlaying)

		return
	}

	e.countInDuration = e.stepDurationFor(MaxSteps, e.countInStep%MaxSteps)
	e.countInTriggered = false
}
