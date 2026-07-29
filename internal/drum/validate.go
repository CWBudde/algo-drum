package drum

import (
	"errors"
	"fmt"
)

// Validate reports every engine invariant that is currently broken, joined
// into one error (nil when the state is sound). The setters already refuse
// out-of-contract input (see validFloat), so a violation here means the engine
// corrupted itself — the C11 swing bug rewrote step lengths from a valid
// SetStepCount call — and that class of defect is otherwise silent: Render's
// final clamp mutes a stray NaN and a wrong step length only sounds off.
//
// It is a pure read of the engine's own state and allocates nothing while the
// state is sound, so it is cheap enough for tests, the fuzz target and the
// per-render assertion built under the `drumassert` tag (see assert.go).
func (e *Engine) Validate() error {
	var problems []error

	problems = e.checkTransport(problems)
	problems = e.checkPending(problems)
	problems = e.checkMix(problems)

	return errors.Join(problems...)
}

// inRange reports whether val lies within [minVal, maxVal]. NaN and ±Inf fail
// both comparisons, so this is the predicate form of validFloat's contract:
// finite and clamped.
func inRange(val, minVal, maxVal float64) bool {
	return val >= minVal && val <= maxVal
}

// checkTransport verifies the sequencer state Render indexes with: the loop
// bounds, the playhead inside them, and a positive length for every step.
func (e *Engine) checkTransport(problems []error) []error {
	if !inRange(e.sr, minSampleRate, maxSampleRate) {
		problems = append(problems, fmt.Errorf("sample rate out of contract: %v", e.sr))
	}

	if !inRange(e.bpm, minTempoBPM, maxTempoBPM) {
		problems = append(problems, fmt.Errorf("tempo out of contract: %v", e.bpm))
	}

	if !inRange(e.swing, 0, maxSwing) {
		problems = append(problems, fmt.Errorf("swing out of contract: %v", e.swing))
	}

	if !inRange(e.prob, 0, 1) {
		problems = append(problems, fmt.Errorf("probability out of contract: %v", e.prob))
	}

	if !inRange(e.humanize, 0, 1) {
		problems = append(problems, fmt.Errorf("humanize out of contract: %v", e.humanize))
	}

	if e.stepCount < 1 || e.stepCount > MaxSteps {
		problems = append(problems, fmt.Errorf("step count %d outside [1, %d]", e.stepCount, MaxSteps))
	}

	// Checked against the raw field rather than stepCount so a broken count
	// cannot mask a playhead that would index outside the pattern.
	if e.currentStep < 0 || e.currentStep >= e.stepCount {
		problems = append(problems,
			fmt.Errorf("playhead %d outside loop of %d steps", e.currentStep, e.stepCount))
	}

	if e.stepSamples < 0 {
		problems = append(problems, fmt.Errorf("elapsed step samples %d is negative", e.stepSamples))
	}

	// A non-positive length makes Render advance a step per sample; lengths
	// are whole samples, so "finite" is inherent in the int64.
	for step, length := range e.stepLen {
		if length <= 0 {
			problems = append(problems, fmt.Errorf("step %d length %d is not positive", step, length))
		}
	}

	return problems
}

// checkPending verifies the humanize-delayed trigger slots: an active slot
// must still be counting down toward a real voice, since firePending clears it
// the moment it fires.
func (e *Engine) checkPending(problems []error) []error {
	for slot, trigger := range e.pending {
		if !trigger.active {
			continue
		}

		if trigger.countdown <= 0 {
			problems = append(problems,
				fmt.Errorf("pending trigger %d past its deadline (countdown %d)", slot, trigger.countdown))
		}

		if !validTrack(trigger.track) {
			problems = append(problems,
				fmt.Errorf("pending trigger %d targets invalid track %d", slot, trigger.track))
		}

		if !inRange(trigger.velocity, 0, 1) {
			problems = append(problems,
				fmt.Errorf("pending trigger %d velocity out of contract: %v", slot, trigger.velocity))
		}
	}

	return problems
}

// checkMix verifies everything that scales or shapes a sample: the pattern,
// the per-track gains and the voices' own parameters.
func (e *Engine) checkMix(problems []error) []error {
	if !inRange(e.reverbAmount, 0, 1) {
		problems = append(problems, fmt.Errorf("reverb amount out of contract: %v", e.reverbAmount))
	}

	if !inRange(e.volCoef, 0, 1) {
		problems = append(problems, fmt.Errorf("volume ramp coefficient out of contract: %v", e.volCoef))
	}

	if e.tomModel != TomModelProcedural && e.tomModel != TomModelPhysical {
		problems = append(problems, fmt.Errorf("invalid Tom model: %d", e.tomModel))
	}

	for track := range e.pattern {
		for step, velocity := range e.pattern[track] {
			if !inRange(velocity, 0, 1) {
				problems = append(problems,
					fmt.Errorf("cell (%d, %d) velocity out of contract: %v", track, step, velocity))
			}
		}
	}

	for track := range e.voices {
		if !inRange(e.volumes[track], 0, 1) {
			problems = append(problems,
				fmt.Errorf("track %d volume out of contract: %v", track, e.volumes[track]))
		}

		// The ramp is a convex blend of two in-range gains, so it can only
		// leave [0, 1] if one of its inputs already has.
		if !inRange(e.liveVol[track], 0, 1) {
			problems = append(problems,
				fmt.Errorf("track %d smoothed volume out of contract: %v", track, e.liveVol[track]))
		}

		if !inRange(e.decays[track], 0, 1) {
			problems = append(problems,
				fmt.Errorf("track %d decay out of contract: %v", track, e.decays[track]))
		}

		voice := e.voices[track]
		if voice == nil {
			problems = append(problems, fmt.Errorf("track %d has no voice", track))

			continue
		}

		for index := range voice.ParamSpecs() {
			if param := voice.Param(index); !inRange(param, 0, 1) {
				problems = append(problems,
					fmt.Errorf("track %d param %d out of contract: %v", track, index, param))
			}
		}
	}

	if e.physicalTom != nil {
		for index := range e.physicalTom.ParamSpecs() {
			if param := e.physicalTom.Param(index); !inRange(param, 0, 1) {
				problems = append(problems,
					fmt.Errorf("physical Tom param %d out of contract: %v", index, param))
			}
		}
	}

	return problems
}
