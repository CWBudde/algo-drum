package drum

import (
	"fmt"

	"github.com/cwbudde/algo-tom/tomparams"
)

// EngineState is the complete user-controlled engine state. Each bank owns its
// rhythmic configuration; mixer, voice and global performance controls remain
// shared. Active/queued bank and chain cursor are runtime-only.
// The transport/playheads, conditional pass history, live smoothing values,
// active tails and RNG position are deliberately runtime state rather than
// preset state.
type EngineState struct {
	TempoBPM       float64
	Swing          float64
	Reverb         float64
	Probability    float64
	Humanize       float64
	FillMode       bool
	Banks          []PatternBankState
	StandaloneBank int
	ChainEnabled   bool
	Chain          []int
	Tracks         []TrackState
}

// PatternBankState is one owned rhythmic snapshot. Cell arrays are flat
// track-major with PatternSize entries; TrackLengths is engine-major.
type PatternBankState struct {
	StepCount         int
	Pattern           []float64
	CellProbabilities []float64
	CellHumanize      []float64
	CellConditions    []TriggerCondition
	TrackLengths      []int
}

// TrackState is one engine-major track. Volume remains meaningful while Muted
// is true; muting changes only the smoothed render target. VoiceParams always
// describes the procedural/ordinary voice bank, including on a Tom whose
// physical model is currently selected.
type TrackState struct {
	Volume      float64
	Decay       float64
	Muted       bool
	VoiceParams []float64
	Tom         *TomState
}

// TomState carries the state unique to the two Tom tracks. PhysicalParams is
// present even for a procedural Tom, so switching models never discards the
// inactive bank and a snapshot does not need to instantiate the physical model.
type TomState struct {
	Model          TomModel
	PhysicalParams []float64
}

// State returns a deep snapshot of every user-controlled value. Callers own
// all returned slices and may modify them without aliasing the live engine.
func (e *Engine) State() EngineState {
	state := EngineState{
		TempoBPM:       e.bpm,
		Swing:          e.swing,
		Reverb:         e.reverbAmount,
		Probability:    e.prob,
		Humanize:       e.humanize,
		FillMode:       e.fillMode,
		Banks:          make([]PatternBankState, PatternBankCount),
		StandaloneBank: e.standaloneBank,
		ChainEnabled:   e.chainEnabled,
		Chain:          append([]int(nil), e.chain[:e.chainLength]...),
		Tracks:         make([]TrackState, TrackCount),
	}

	for bank := range PatternBankCount {
		stored := &e.banks[bank]

		bankState := PatternBankState{
			StepCount:         stored.stepCount,
			Pattern:           make([]float64, PatternSize),
			CellProbabilities: make([]float64, PatternSize),
			CellHumanize:      make([]float64, PatternSize),
			CellConditions:    make([]TriggerCondition, PatternSize),
			TrackLengths:      make([]int, TrackCount),
		}
		for track := range TrackCount {
			start := track * MaxSteps
			end := start + MaxSteps
			copy(bankState.Pattern[start:end], stored.pattern[track][:])
			copy(bankState.CellProbabilities[start:end], stored.cellProbability[track][:])
			copy(bankState.CellHumanize[start:end], stored.cellHumanize[track][:])
			copy(bankState.CellConditions[start:end], stored.cellCondition[track][:])
			bankState.TrackLengths[track] = stored.trackLength[track]
		}

		state.Banks[bank] = bankState
	}

	for track := range e.voices {
		voice := e.voices[track]
		if validTomTrack(track) {
			voice = e.proceduralToms[track]
		}

		trackState := TrackState{
			Volume:      e.volumes[track],
			Decay:       e.decays[track],
			Muted:       e.muted[track],
			VoiceParams: make([]float64, len(voice.ParamSpecs())),
		}
		for index := range trackState.VoiceParams {
			trackState.VoiceParams[index] = voice.Param(index)
		}

		if validTomTrack(track) {
			trackState.Tom = &TomState{
				Model:          e.tomModels[track],
				PhysicalParams: append([]float64(nil), e.physicalTomParams[track]...),
			}
		}

		state.Tracks[track] = trackState
	}

	return state
}

// ReplaceState atomically validates and normalizes a full user-state snapshot,
// then replaces the engine's corresponding values. Structural mistakes and
// every NaN/Inf reject the whole snapshot before mutation. Finite numeric input
// follows the setters' existing contract and is clamped into its valid range.
// Runtime state (transport position, active tails and smoothing positions) is
// preserved so applying a preset during playback does not restart the machine.
func (e *Engine) ReplaceState(state EngineState) error {
	normalized, err := e.normalizeState(state)
	if err != nil {
		return err
	}

	e.SetTempo(normalized.TempoBPM)
	e.SetSwing(normalized.Swing)
	e.SetReverb(normalized.Reverb)
	e.SetProbability(normalized.Probability)
	e.SetHumanize(normalized.Humanize)
	e.SetFillMode(normalized.FillMode)

	for bank, bankState := range normalized.Banks {
		e.storePatternBank(bank, bankState)
	}

	e.standaloneBank = normalized.StandaloneBank
	e.chainEnabled = normalized.ChainEnabled
	e.chainLength = len(normalized.Chain)
	copy(e.chain[:], normalized.Chain)
	clear(e.chain[e.chainLength:])
	e.chainPosition = 0
	e.queuedBank = NoBank

	target := e.activeBank
	if e.transport == transportStopped {
		target = e.standaloneBank
		if e.chainEnabled {
			target = e.chain[0]
		}
	}

	e.loadBank(target)
	e.recomputeStepDurations()

	if e.transport == transportStopped {
		e.resetSequencer()
	}

	for track, trackState := range normalized.Tracks {
		e.SetVolume(track, trackState.Volume)
		e.SetMuted(track, trackState.Muted)
		e.SetDecay(track, trackState.Decay)

		for index, value := range trackState.VoiceParams {
			e.SetVoiceParam(track, index, value)
		}

		if !validTomTrack(track) {
			continue
		}

		copy(e.physicalTomParams[track], trackState.Tom.PhysicalParams)

		if physicalVoice := e.physicalToms[track]; physicalVoice != nil {
			if err := physicalVoice.replaceParams(e.physicalTomParams[track]); err != nil {
				// normalizeState has already derived and validated the same
				// configuration, so reaching this means the live model rejected a
				// configuration its constructor accepts.
				return fmt.Errorf("replace physical Tom track %d: %w", track, err)
			}
		}
	}

	for _, track := range [...]int{tomTrackIndex, tom2TrackIndex} {
		e.SetTomModel(track, normalized.Tracks[track].Tom.Model)

		if e.tomModels[track] != normalized.Tracks[track].Tom.Model {
			return fmt.Errorf("replace Tom track %d: model %d unavailable",
				track, normalized.Tracks[track].Tom.Model)
		}
	}

	return nil
}

// ReplacePatternBank atomically validates and replaces one complete rhythmic
// bank. Invalid indexes are silent no-ops, matching the indexed setter
// contract; malformed state is reported without mutating any bank.
func (e *Engine) ReplacePatternBank(bank int, state PatternBankState) error {
	if !validBank(bank) {
		return nil
	}

	normalized, err := normalizePatternBank(state)
	if err != nil {
		return err
	}

	e.storePatternBank(bank, normalized)

	if bank == e.activeBank {
		e.loadBank(bank)
		e.recomputeStepDurations()

		if e.currentStep >= e.stepCount {
			e.currentStep = 0
			e.stepPhase = 0
			e.stepTriggered = false
		}

		for track := range TrackCount {
			if e.trackStep[track] >= e.trackLength[track] {
				e.trackStep[track] = 0
				e.trackPass[track] = 0
				e.previousFired[track] = false
			}
		}
	}

	return nil
}

func (e *Engine) storePatternBank(bank int, state PatternBankState) {
	target := &e.banks[bank]
	target.stepCount = state.StepCount

	for index := range PatternSize {
		track := index / MaxSteps
		step := index % MaxSteps
		target.pattern[track][step] = state.Pattern[index]
		target.cellProbability[track][step] = state.CellProbabilities[index]
		target.cellHumanize[track][step] = state.CellHumanize[index]
		target.cellCondition[track][step] = state.CellConditions[index]
	}

	copy(target.trackLength[:], state.TrackLengths)
}

func normalizePatternBank(state PatternBankState) (PatternBankState, error) {
	if len(state.Pattern) != PatternSize {
		return PatternBankState{}, fmt.Errorf("pattern length %d, want %d", len(state.Pattern), PatternSize)
	}

	if len(state.CellProbabilities) != PatternSize {
		return PatternBankState{}, fmt.Errorf("cell probability length %d, want %d",
			len(state.CellProbabilities), PatternSize)
	}

	if len(state.CellHumanize) != PatternSize {
		return PatternBankState{}, fmt.Errorf("cell humanize length %d, want %d",
			len(state.CellHumanize), PatternSize)
	}

	if len(state.CellConditions) != PatternSize {
		return PatternBankState{}, fmt.Errorf("cell condition length %d, want %d",
			len(state.CellConditions), PatternSize)
	}

	if len(state.TrackLengths) != TrackCount {
		return PatternBankState{}, fmt.Errorf("track length count %d, want %d",
			len(state.TrackLengths), TrackCount)
	}

	normalized := PatternBankState{
		StepCount:         state.StepCount,
		Pattern:           make([]float64, PatternSize),
		CellProbabilities: make([]float64, PatternSize),
		CellHumanize:      make([]float64, PatternSize),
		CellConditions:    make([]TriggerCondition, PatternSize),
		TrackLengths:      make([]int, TrackCount),
	}
	if normalized.StepCount < 1 {
		normalized.StepCount = 1
	} else if normalized.StepCount > MaxSteps {
		normalized.StepCount = MaxSteps
	}

	for index, value := range state.Pattern {
		clamped, valid := validFloat(value, 0, 1)
		if !valid {
			return PatternBankState{}, fmt.Errorf("pattern value %d is not finite: %v", index, value)
		}

		normalized.Pattern[index] = clamped
	}

	for index, value := range state.CellProbabilities {
		clamped, valid := validFloat(value, 0, 1)
		if !valid {
			return PatternBankState{}, fmt.Errorf("cell probability %d is not finite: %v", index, value)
		}

		normalized.CellProbabilities[index] = clamped
	}

	for index, value := range state.CellHumanize {
		clamped, valid := validFloat(value, 0, 1)
		if !valid {
			return PatternBankState{}, fmt.Errorf("cell humanize %d is not finite: %v", index, value)
		}

		normalized.CellHumanize[index] = clamped
	}

	for index, condition := range state.CellConditions {
		if condition >= triggerConditionCount {
			return PatternBankState{}, fmt.Errorf("cell condition %d has invalid code %d", index, condition)
		}

		normalized.CellConditions[index] = condition
	}

	for track, length := range state.TrackLengths {
		if length < 1 {
			length = 1
		} else if length > MaxSteps {
			length = MaxSteps
		}

		normalized.TrackLengths[track] = length
	}

	return normalized, nil
}

// normalizeState performs the complete rejection pass and returns an owned,
// clamped copy. It must remain a pure read of e: ReplaceState relies on that
// property for its reject-before-mutation guarantee.
func (e *Engine) normalizeState(state EngineState) (EngineState, error) {
	if len(state.Banks) != PatternBankCount {
		return EngineState{}, fmt.Errorf("bank count %d, want %d", len(state.Banks), PatternBankCount)
	}

	if !validBank(state.StandaloneBank) {
		return EngineState{}, fmt.Errorf("standalone bank %d outside [0, %d]",
			state.StandaloneBank, PatternBankCount-1)
	}

	if len(state.Chain) < 1 || len(state.Chain) > MaxChainLength {
		return EngineState{}, fmt.Errorf("chain length %d outside [1, %d]",
			len(state.Chain), MaxChainLength)
	}

	for position, bank := range state.Chain {
		if !validBank(bank) {
			return EngineState{}, fmt.Errorf("chain position %d has invalid bank %d", position, bank)
		}
	}

	if len(state.Tracks) != TrackCount {
		return EngineState{}, fmt.Errorf("track count %d, want %d", len(state.Tracks), TrackCount)
	}

	normalized := EngineState{
		FillMode:       state.FillMode,
		Banks:          make([]PatternBankState, PatternBankCount),
		StandaloneBank: state.StandaloneBank,
		ChainEnabled:   state.ChainEnabled,
		Chain:          append([]int(nil), state.Chain...),
		Tracks:         make([]TrackState, TrackCount),
	}

	var ok bool
	if normalized.TempoBPM, ok = validFloat(state.TempoBPM, minTempoBPM, maxTempoBPM); !ok {
		return EngineState{}, fmt.Errorf("tempo is not finite: %v", state.TempoBPM)
	}

	if normalized.Swing, ok = validFloat(state.Swing, 0, maxSwing); !ok {
		return EngineState{}, fmt.Errorf("swing is not finite: %v", state.Swing)
	}

	if normalized.Reverb, ok = validFloat(state.Reverb, 0, 1); !ok {
		return EngineState{}, fmt.Errorf("reverb is not finite: %v", state.Reverb)
	}

	if normalized.Probability, ok = validFloat(state.Probability, 0, 1); !ok {
		return EngineState{}, fmt.Errorf("probability is not finite: %v", state.Probability)
	}

	if normalized.Humanize, ok = validFloat(state.Humanize, 0, 1); !ok {
		return EngineState{}, fmt.Errorf("humanize is not finite: %v", state.Humanize)
	}

	for bank, source := range state.Banks {
		bankState, bankErr := normalizePatternBank(source)
		if bankErr != nil {
			return EngineState{}, fmt.Errorf("bank %d: %w", bank, bankErr)
		}

		normalized.Banks[bank] = bankState
	}

	for track, source := range state.Tracks {
		wantParams := len(SpecsForTrack(track))
		if len(source.VoiceParams) != wantParams {
			return EngineState{}, fmt.Errorf("track %d voice parameter count %d, want %d",
				track, len(source.VoiceParams), wantParams)
		}

		target := TrackState{
			Muted:       source.Muted,
			VoiceParams: make([]float64, wantParams),
		}
		if target.Volume, ok = validFloat(source.Volume, 0, 1); !ok {
			return EngineState{}, fmt.Errorf("track %d volume is not finite: %v", track, source.Volume)
		}

		if target.Decay, ok = validFloat(source.Decay, 0, 1); !ok {
			return EngineState{}, fmt.Errorf("track %d decay is not finite: %v", track, source.Decay)
		}

		for index, value := range source.VoiceParams {
			clamped, valid := validFloat(value, 0, 1)
			if !valid {
				return EngineState{}, fmt.Errorf(
					"track %d voice parameter %d is not finite: %v", track, index, value,
				)
			}

			target.VoiceParams[index] = clamped
		}

		if validTomTrack(track) {
			if source.Tom == nil {
				return EngineState{}, fmt.Errorf("Tom track %d has no Tom state", track)
			}

			if source.Tom.Model != TomModelProcedural && source.Tom.Model != TomModelPhysical {
				return EngineState{}, fmt.Errorf("Tom track %d has invalid model %d",
					track, source.Tom.Model)
			}

			if len(source.Tom.PhysicalParams) != len(physicalTomSpecs) {
				return EngineState{}, fmt.Errorf(
					"Tom track %d physical parameter count %d, want %d",
					track, len(source.Tom.PhysicalParams), len(physicalTomSpecs),
				)
			}

			target.Tom = &TomState{
				Model:          source.Tom.Model,
				PhysicalParams: make([]float64, len(physicalTomSpecs)),
			}
			for index, value := range source.Tom.PhysicalParams {
				clamped, valid := validFloat(value, 0, 1)
				if !valid {
					return EngineState{}, fmt.Errorf(
						"Tom track %d physical parameter %d is not finite: %v",
						track, index, value,
					)
				}

				target.Tom.PhysicalParams[index] = clamped
			}

			if _, err := tomparams.Config(target.Tom.PhysicalParams, target.Decay, e.sr); err != nil {
				return EngineState{}, fmt.Errorf("Tom track %d physical parameters: %w", track, err)
			}
		} else if source.Tom != nil {
			return EngineState{}, fmt.Errorf("non-Tom track %d has Tom state", track)
		}

		normalized.Tracks[track] = target
	}

	return normalized, nil
}
