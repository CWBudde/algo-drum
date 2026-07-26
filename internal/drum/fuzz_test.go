package drum

import (
	"encoding/binary"
	"math"
	"testing"
)

// A fuzz program is a byte string decoded into a sequence of setter calls.
// Each record is one opcode byte plus eight bytes reinterpreted as a float64,
// so the corpus reaches NaN, ±Inf and every subnormal for free — the exact
// inputs a hand-written test can only sample.
const (
	fuzzRecordLen = 9    // 1 opcode byte + 8 value bytes
	fuzzMaxOps    = 64   // keep a single fuzz iteration cheap
	fuzzRenderLen = 1024 // samples rendered after the program has run

	// fuzzIndexBias shifts decoded indices so they straddle the valid range
	// on both sides (negative as well as too large).
	fuzzIndexBias = 4
)

// Opcodes, held in the low nibble of the record's first byte; the high nibble
// carries the track index, so the two are independent.
const (
	opTempo byte = iota
	opSwing
	opVolume
	opDecay
	opReverb
	opProbability
	opHumanize
	opCell
	opPattern
	opStepCountRaw // whole value word as an int: reaches MinInt64/MaxInt64
	opStepCount    // small index-sized loop lengths around [1, MaxSteps]
	opRunning
	opVoiceParam
	opTriggerVoice
	fuzzOpCount
)

// fuzzOp encodes one setter call for the corpus.
func fuzzOp(op, index byte, value float64) []byte {
	record := make([]byte, fuzzRecordLen)
	record[0] = op&0x0f | index<<4
	binary.LittleEndian.PutUint64(record[1:], math.Float64bits(value))

	return record
}

// fuzzIndexValue builds a value word whose low bits decode to idx, for the
// opcodes that read a step or loop length out of them.
func fuzzIndexValue(idx int) float64 {
	return math.Float64frombits(uint64(idx + fuzzIndexBias))
}

// fuzzProgram concatenates encoded setter calls into one corpus entry.
func fuzzProgram(records ...[]byte) []byte {
	var program []byte
	for _, record := range records {
		program = append(program, record...)
	}

	return program
}

// applyFuzzProgram drives the public setter surface from an arbitrary byte
// string. A truncated trailing record is ignored.
func applyFuzzProgram(engine *Engine, program []byte) {
	for op := 0; op < fuzzMaxOps && len(program) >= fuzzRecordLen; op++ {
		opcode := program[0]
		bits := binary.LittleEndian.Uint64(program[1:fuzzRecordLen])
		program = program[fuzzRecordLen:]

		value := math.Float64frombits(bits)
		index := int(opcode>>4) - fuzzIndexBias
		small := int(bits&0x1f) - fuzzIndexBias

		switch (opcode & 0x0f) % fuzzOpCount {
		case opTempo:
			engine.SetTempo(value)
		case opSwing:
			engine.SetSwing(value)
		case opVolume:
			engine.SetVolume(index, value)
		case opDecay:
			engine.SetDecay(index, value)
		case opReverb:
			engine.SetReverb(value)
		case opProbability:
			engine.SetProbability(value)
		case opHumanize:
			engine.SetHumanize(value)
		case opCell:
			engine.SetCell(index, small, value)
		case opPattern:
			engine.SetPattern([]float64{value, -value, value * 2, math.NaN()})
		case opStepCountRaw:
			engine.SetStepCount(int(int64(bits)))
		case opStepCount:
			engine.SetStepCount(small)
		case opRunning:
			engine.SetRunning(bits&1 == 1)
		case opVoiceParam:
			engine.SetVoiceParam(index, small, value)
		case opTriggerVoice:
			engine.TriggerVoice(index, value)
		}
	}
}

// checkEngineInvariants asserts the state the render loop relies on. Output
// checks alone are not enough: the last-resort clamp in Render mutes a stray
// NaN, so corruption has to be caught where it actually lands — which is
// exactly what Engine.Validate reports (see validate.go).
func checkEngineInvariants(t *testing.T, engine *Engine) {
	t.Helper()

	if err := engine.Validate(); err != nil {
		t.Fatalf("engine invariants broken: %v", err)
	}
}

// FuzzEngineSetters drives an arbitrary sequence of setter calls with
// arbitrary arguments and then asserts the render contract: every sample
// finite and inside ±1.0, with the engine's own state still in range. The
// seed corpus runs as an ordinary test, so it stays in the default CI run.
func FuzzEngineSetters(f *testing.F) {
	nan := math.NaN()
	posInf := math.Inf(1)
	negInf := math.Inf(-1)

	// A NaN whose payload decodes to step 0, so this reproduction really is
	// SetCell(0, 0, NaN) and not an out-of-range no-op.
	cellNaN := math.Float64frombits(0x7ff8000000000000 | uint64(fuzzIndexBias))

	// The verified NaN reproductions, a swung odd-length loop, and a pass
	// over the out-of-range index contract.
	f.Add(fuzzProgram(fuzzOp(opCell, fuzzIndexBias, cellNaN)))
	f.Add(fuzzProgram(fuzzOp(opTempo, 0, nan)))
	f.Add(fuzzProgram(fuzzOp(opTempo, 0, posInf), fuzzOp(opSwing, 0, negInf)))
	f.Add(fuzzProgram(
		fuzzOp(opStepCount, 0, fuzzIndexValue(7)),
		fuzzOp(opSwing, 0, 0.5),
		fuzzOp(opCell, fuzzIndexBias, cellNaN),
		fuzzOp(opRunning, 0, math.Float64frombits(1)),
	))
	f.Add(fuzzProgram(
		fuzzOp(opVolume, 0, nan),
		fuzzOp(opDecay, 15, posInf),
		fuzzOp(opStepCountRaw, 0, negInf),
	))
	f.Add(fuzzProgram(
		fuzzOp(opPattern, 0, posInf),
		fuzzOp(opProbability, 0, nan),
		fuzzOp(opHumanize, 0, nan),
		fuzzOp(opReverb, 0, nan),
	))
	// Voice parameters at both extremes on every track, plus auditions mixed
	// with the strip decay trim on the same track: the filter-based voices
	// redesign their biquads from these values, so a bad range mutes or blows
	// up a voice rather than merely sounding wrong.
	f.Add(fuzzProgram(
		fuzzOp(opVoiceParam, fuzzIndexBias+2, fuzzIndexValue(0)),
		fuzzOp(opVoiceParam, fuzzIndexBias+2, math.Float64frombits(uint64(1+fuzzIndexBias))),
		fuzzOp(opVoiceParam, fuzzIndexBias+4, nan),
		fuzzOp(opVoiceParam, fuzzIndexBias, posInf),
		fuzzOp(opRunning, 0, math.Float64frombits(1)),
	))
	f.Add(fuzzProgram(
		fuzzOp(opTriggerVoice, fuzzIndexBias, 1),
		fuzzOp(opDecay, fuzzIndexBias, 1),
		fuzzOp(opVoiceParam, fuzzIndexBias, negInf),
		fuzzOp(opTriggerVoice, 15, nan),
	))
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, program []byte) {
		engine := NewEngine(testSampleRate)
		applyFuzzProgram(engine, program)
		engine.SetRunning(true)

		buf := make([]float32, fuzzRenderLen)
		engine.Render(buf)

		for i, sample := range buf {
			value := float64(sample)
			if math.IsNaN(value) || math.IsInf(value, 0) {
				t.Fatalf("sample %d is not finite: %v", i, value)
			}

			if math.Abs(value) > 1.0 {
				t.Fatalf("sample %d = %v exceeds ±1.0 output ceiling", i, value)
			}
		}

		checkEngineInvariants(t, engine)
	})
}
