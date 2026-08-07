package drum

func validBank(bank int) bool {
	return bank >= 0 && bank < PatternBankCount
}

// loadBank makes one stored bank the render-time bank. Copying these small,
// fixed arrays happens only on an explicit selection or a master-loop wrap;
// Render otherwise reads the flat active fields directly.
func (e *Engine) loadBank(bank int) {
	stored := &e.banks[bank]

	e.activeBank = bank
	e.stepCount = stored.stepCount
	e.pattern = stored.pattern
	e.cellProbability = stored.cellProbability
	e.cellHumanize = stored.cellHumanize
	e.cellCondition = stored.cellCondition
	e.cellRepeats = stored.cellRepeats
	e.trackLength = stored.trackLength
}

func (e *Engine) syncActiveBank() {
	stored := &e.banks[e.activeBank]

	stored.stepCount = e.stepCount
	stored.pattern = e.pattern
	stored.cellProbability = e.cellProbability
	stored.cellHumanize = e.cellHumanize
	stored.cellCondition = e.cellCondition
	stored.cellRepeats = e.cellRepeats
	stored.trackLength = e.trackLength
}

// ActiveBank reports the bank currently driving the sequencer.
func (e *Engine) ActiveBank() int { return e.activeBank }

// QueuedBank reports the standalone bank requested for a future master wrap,
// or NoBank when there is no outstanding request.
func (e *Engine) QueuedBank() int { return e.queuedBank }

// ChainPosition reports the active entry within Chain. It is runtime state and
// resets to zero on Stop.
func (e *Engine) ChainPosition() int { return e.chainPosition }

// RequestBank selects a standalone bank. Stopped requests take effect
// immediately; playing and paused requests are last-write-wins at a master
// wrap. Chain mode owns selection and therefore ignores manual requests.
func (e *Engine) RequestBank(bank int) {
	if !validBank(bank) || e.chainEnabled {
		return
	}

	e.standaloneBank = bank
	if e.transport == transportStopped {
		if bank != e.activeBank {
			e.loadBank(bank)
			e.recomputeStepDurations()
		}

		e.queuedBank = NoBank
		e.resetSequencer()

		return
	}

	// Once lookahead committed a different bank for the imminent boundary,
	// requesting the still-active bank means "return on the following wrap",
	// not "undo audio that has already been rendered".
	if bank == e.activeBank && (!e.nextStepScheduled || e.nextBank == e.activeBank) {
		e.queuedBank = NoBank

		return
	}

	e.queuedBank = bank
}

// SetChain atomically replaces the 1..MaxChainLength bank sequence. It is a
// stopped-only edit so the cursor and already-rendered lookahead cannot drift
// from the persisted configuration.
func (e *Engine) SetChain(chain []int) {
	if e.transport != transportStopped || len(chain) < 1 || len(chain) > MaxChainLength {
		return
	}

	for _, bank := range chain {
		if !validBank(bank) {
			return
		}
	}

	e.chainLength = len(chain)
	copy(e.chain[:], chain)
	clear(e.chain[len(chain):])
	e.chainPosition = 0

	if e.chainEnabled {
		e.loadBank(e.chain[0])
		e.recomputeStepDurations()
		e.resetSequencer()
	}
}

// SetChainEnabled toggles chain playback while stopped. Enabling selects the
// first chain entry; disabling returns to the standalone selection.
func (e *Engine) SetChainEnabled(enabled bool) {
	if e.transport != transportStopped || e.chainEnabled == enabled {
		return
	}

	e.chainEnabled = enabled
	e.chainPosition = 0

	target := e.standaloneBank
	if enabled {
		target = e.chain[0]
	}

	e.loadBank(target)
	e.recomputeStepDurations()
	e.resetSequencer()
}
