package drum

import (
	"errors"
	"fmt"
	"math/bits"
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

	if e.transport > transportPaused {
		problems = append(problems, fmt.Errorf("invalid transport state: %d", e.transport))
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

	if e.currentStepDuration == 0 {
		problems = append(problems, errors.New("current step duration is zero"))
	} else if e.stepPhase >= e.currentStepDuration {
		problems = append(problems,
			fmt.Errorf("step phase %d outside current duration %d", e.stepPhase, e.currentStepDuration))
	}

	// A non-positive length makes Render advance a step per sample; lengths
	// are whole samples, so "finite" is inherent in the int64.
	for step, duration := range e.stepDuration {
		if duration == 0 {
			problems = append(problems, fmt.Errorf("step %d duration is zero", step))
		}
	}

	return problems
}

// checkPending verifies the humanize-delayed trigger slots: an active slot
// must still be counting down toward a real voice, since firePending clears it
// the moment it fires.
func (e *Engine) checkPending(problems []error) []error {
	for active := e.pendingMask; active != 0; {
		slot := bits.TrailingZeros32(active)
		active &^= uint32(1) << slot
		trigger := e.pending[slot]

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

	if !inRange(e.liveReverbAmount, 0, 1) {
		problems = append(problems,
			fmt.Errorf("live reverb amount out of contract: %v", e.liveReverbAmount))
	}

	if !inRange(e.volCoef, 0, 1) {
		problems = append(problems, fmt.Errorf("volume ramp coefficient out of contract: %v", e.volCoef))
	}

	// The idle window is derived from the sample rate in NewEngine and never
	// written again; a zero or negative one would make IsIdle true forever and
	// mute the engine. Both counters are whole samples, so "finite" is inherent
	// in the int64.
	if e.idleSamples < 1 {
		problems = append(problems, fmt.Errorf("idle confirm window %d below one sample", e.idleSamples))
	}

	// Render only ever increments silentRun toward the window and clears it
	// otherwise, so anything outside [0, idleSamples] means the counter was
	// corrupted rather than counted.
	if e.silentRun < 0 || e.silentRun > e.idleSamples {
		problems = append(problems,
			fmt.Errorf("silent run %d outside [0, %d]", e.silentRun, e.idleSamples))
	}

	for _, track := range [...]int{tomTrackIndex, tom2TrackIndex} {
		model := e.tomModels[track]
		switch model {
		case TomModelProcedural:
			if e.voices[track] != e.proceduralToms[track] {
				problems = append(problems,
					fmt.Errorf("procedural Tom track %d does not own its active voice", track))
			}
		case TomModelPhysical:
			if e.physicalToms[track] == nil {
				problems = append(problems,
					fmt.Errorf("physical Tom track %d has no physical voice", track))
			} else if e.voices[track] != e.physicalToms[track] {
				problems = append(problems,
					fmt.Errorf("physical Tom track %d does not own its active voice", track))
			}
		default:
			problems = append(problems,
				fmt.Errorf("invalid Tom model on track %d: %d", track, model))
		}
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

		if validTomTrack(track) {
			// The procedural bank is part of state even while the physical
			// implementation is active, so validate it independently of the
			// currently selected Voice.
			procedural := e.proceduralToms[track]
			if procedural == nil {
				problems = append(problems, fmt.Errorf("Tom track %d has no procedural voice", track))

				continue
			}

			voice = procedural
		}

		for index := range voice.ParamSpecs() {
			if param := voice.Param(index); !inRange(param, 0, 1) {
				problems = append(problems,
					fmt.Errorf("track %d param %d out of contract: %v", track, index, param))
			}
		}
	}

	for _, track := range [...]int{tomTrackIndex, tom2TrackIndex} {
		shadow := e.physicalTomParams[track]
		if len(shadow) != len(physicalTomSpecs) {
			problems = append(problems,
				fmt.Errorf("physical Tom track %d shadow parameter count %d, want %d",
					track, len(shadow), len(physicalTomSpecs)))
		} else {
			for index, param := range shadow {
				if !inRange(param, 0, 1) {
					problems = append(problems,
						fmt.Errorf("physical Tom track %d shadow param %d out of contract: %v",
							track, index, param))
				}
			}
		}

		physicalTom := e.physicalToms[track]
		if physicalTom == nil {
			continue
		}

		for index := range physicalTom.ParamSpecs() {
			param := physicalTom.Param(index)
			if !inRange(param, 0, 1) {
				problems = append(problems,
					fmt.Errorf("physical Tom track %d param %d out of contract: %v",
						track, index, param))
			}

			if index < len(shadow) && param != shadow[index] {
				problems = append(problems,
					fmt.Errorf("physical Tom track %d param %d = %v, shadow = %v",
						track, index, param, shadow[index]))
			}
		}
	}

	return problems
}
