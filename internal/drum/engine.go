package drum

const (
	TrackCount = 5
	StepCount  = 8
)

// Engine is the drum machine sequencer and mixer.
type Engine struct {
	sr      float64
	running bool
	bpm     float64
	swing   float64 // 0.0 = no swing, 0.5 = full shuffle

	pattern [TrackCount][StepCount]bool
	volumes [TrackCount]float64

	voices [TrackCount]Voice

	currentStep int
	stepSamples int64
	stepLen     [StepCount]int64 // pre-computed step lengths
}

// NewEngine creates a drum engine at the given sample rate.
func NewEngine(sr float64) *Engine {
	e := &Engine{
		sr:  sr,
		bpm: 120,
	}
	for i := range e.volumes {
		e.volumes[i] = 1.0
	}
	e.voices[0] = NewBassDrum(sr)
	e.voices[1] = NewSnare(sr)
	e.voices[2] = NewHiHat(sr, true)
	e.voices[3] = NewTom(sr)
	e.voices[4] = NewCymbal(sr)
	e.recomputeStepLengths()
	return e
}

// recomputeStepLengths recalculates step durations accounting for swing.
// swing=0: all equal. swing=0.5: even steps get 1.5× base, odd get 0.5× base.
func (e *Engine) recomputeStepLengths() {
	base := e.sr * 60.0 / e.bpm / 2.0 // samples per 8th note
	s := e.swing * 0.5
	for i := range e.stepLen {
		if i%2 == 0 {
			e.stepLen[i] = int64(base * (1.0 + s))
		} else {
			e.stepLen[i] = int64(base * (1.0 - s))
		}
	}
}

func (e *Engine) SetRunning(r bool) {
	if !r {
		e.currentStep = 0
		e.stepSamples = 0
	}
	e.running = r
}

func (e *Engine) SetTempo(bpm float64) {
	e.bpm = bpm
	e.recomputeStepLengths()
}

func (e *Engine) SetSwing(swing float64) {
	e.swing = swing
	e.recomputeStepLengths()
}

func (e *Engine) SetCell(track, step int, active bool) {
	if track < 0 || track >= TrackCount || step < 0 || step >= StepCount {
		return
	}
	e.pattern[track][step] = active
}

func (e *Engine) SetVolume(track int, vol float64) {
	if track >= 0 && track < TrackCount {
		e.volumes[track] = vol
	}
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
					if e.pattern[t][e.currentStep] {
						e.voices[t].Trigger()
					}
				}
			}
			e.stepSamples++
			if e.stepSamples >= e.stepLen[e.currentStep] {
				e.stepSamples = 0
				e.currentStep = (e.currentStep + 1) % StepCount
			}
		}

		var out float64
		for t, v := range e.voices {
			out += v.Tick() * e.volumes[t]
		}
		buf[i] = float32(softClip(out * 0.8))
	}
}

// softClip applies tanh-like saturation to prevent hard clipping.
func softClip(x float64) float64 {
	if x > 1 {
		return 1
	}
	if x < -1 {
		return -1
	}
	return x * (1.5 - 0.5*x*x)
}
