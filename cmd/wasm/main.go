//go:build js && wasm

package main

import (
	"math"
	"syscall/js"
	"unsafe"

	"github.com/cwbudde/algo-drum/internal/drum"
)

const (
	// defaultSampleRate is used when init() is called without a usable
	// sample rate.
	defaultSampleRate = 48000.0

	// maxRenderSamples caps a single render() call. The worklet asks for
	// 512-sample chunks, so this is a generous ceiling that still keeps a
	// hostile or buggy caller from requesting an unbounded allocation.
	maxRenderSamples = 65536
)

var (
	engine *drum.Engine
	funcs  []js.Func

	// warnedBeforeInit gates the "API used before init" console warning so
	// a burst of early calls logs once instead of spamming.
	warnedBeforeInit bool

	// warnedBadArg does the same for calls made with missing or
	// wrong-typed JS arguments.
	warnedBadArg bool

	// Persistent render buffers, grown on demand — render() is called for
	// every audio chunk, so per-call allocation would churn both GCs.
	renderBuf []float32
	jsBytes   js.Value // Uint8Array over the same allocation as jsFloats
	jsFloats  js.Value // Float32Array returned to the caller

	patternInput [drum.PatternSize]float64

	stateOut stateOutput
)

// stateOutput owns the reusable JS typed arrays returned by getState. The
// worker structured-clones the object synchronously before the next update, so
// reusing these views does not alias snapshots received on the main thread.
type stateOutput struct {
	value js.Value

	banks      js.Value
	bankValues [drum.PatternBankCount]patternBankOutput
	chain      []byte
	chainBytes js.Value

	tracks          js.Value
	trackValues     [drum.TrackCount]js.Value
	voiceParams     [drum.TrackCount][]float32
	voiceParamBytes [drum.TrackCount]js.Value
	tomValues       [drum.TrackCount]js.Value
	physicalParams  [drum.TrackCount][]float32
	physicalBytes   [drum.TrackCount]js.Value
}

type patternBankOutput struct {
	value js.Value

	pattern              []float32
	patternBytes         js.Value
	cellProbabilities    []float32
	cellProbabilityBytes js.Value
	cellHumanize         []float32
	cellHumanizeBytes    js.Value
	cellConditions       []byte
	cellConditionBytes   js.Value
	trackLengths         []byte
	trackLengthBytes     js.Value
}

func main() {
	api := js.Global().Get("Object").New()

	api.Set("init", export(func(args []js.Value) any {
		// A missing or unusable sample rate falls back to the default
		// rather than leaving the engine uninitialized.
		sr := defaultSampleRate

		if len(args) > 0 {
			if rate, ok := argFloat(args, 0, "init"); ok && rate > 0 {
				sr = rate
			}
		}

		engine = drum.NewEngine(sr)

		return js.Null()
	}))

	api.Set("setRunning", export(func(args []js.Value) any {
		if !ready() {
			return js.Null()
		}

		if running, ok := argBool(args, 0, "setRunning"); ok {
			engine.SetRunning(running)
		}

		return js.Null()
	}))

	api.Set("beginStart", export(func(args []js.Value) any {
		if ready() {
			engine.BeginStart()
		}

		return js.Null()
	}))

	api.Set("pause", export(func(args []js.Value) any {
		if ready() {
			engine.Pause()
		}

		return js.Null()
	}))

	api.Set("setTempo", export(func(args []js.Value) any {
		if !ready() {
			return js.Null()
		}

		if bpm, ok := argFloat(args, 0, "setTempo"); ok {
			engine.SetTempo(bpm)
		}

		return js.Null()
	}))

	api.Set("setSwing", export(func(args []js.Value) any {
		if !ready() {
			return js.Null()
		}

		if swing, ok := argFloat(args, 0, "setSwing"); ok {
			engine.SetSwing(swing)
		}

		return js.Null()
	}))

	api.Set("setStepCount", export(func(args []js.Value) any {
		if !ready() {
			return js.Null()
		}

		bank, bankOK := argInt(args, 0, "setStepCount")
		steps, stepsOK := argInt(args, 1, "setStepCount")

		if bankOK && stepsOK {
			engine.SetStepCount(bank, steps)
		}

		return js.Null()
	}))

	api.Set("setCell", export(func(args []js.Value) any {
		if !ready() {
			return js.Null()
		}

		bank, bankOK := argInt(args, 0, "setCell")
		track, trackOK := argInt(args, 1, "setCell")
		step, stepOK := argInt(args, 2, "setCell")
		velocity, velOK := argFloat(args, 3, "setCell")

		if bankOK && trackOK && stepOK && velOK {
			engine.SetCell(bank, track, step, velocity)
		}

		return js.Null()
	}))

	api.Set("setCellProbability", export(func(args []js.Value) any {
		if !ready() {
			return js.Null()
		}

		bank, bankOK := argInt(args, 0, "setCellProbability")
		track, trackOK := argInt(args, 1, "setCellProbability")
		step, stepOK := argInt(args, 2, "setCellProbability")
		probability, probabilityOK := argFloat(args, 3, "setCellProbability")

		if bankOK && trackOK && stepOK && probabilityOK {
			engine.SetCellProbability(bank, track, step, probability)
		}

		return js.Null()
	}))

	api.Set("setCellHumanize", export(func(args []js.Value) any {
		if !ready() {
			return js.Null()
		}

		bank, bankOK := argInt(args, 0, "setCellHumanize")
		track, trackOK := argInt(args, 1, "setCellHumanize")
		step, stepOK := argInt(args, 2, "setCellHumanize")
		humanize, humanizeOK := argFloat(args, 3, "setCellHumanize")

		if bankOK && trackOK && stepOK && humanizeOK {
			engine.SetCellHumanize(bank, track, step, humanize)
		}

		return js.Null()
	}))

	api.Set("setCellCondition", export(func(args []js.Value) any {
		if !ready() {
			return js.Null()
		}

		bank, bankOK := argInt(args, 0, "setCellCondition")
		track, trackOK := argInt(args, 1, "setCellCondition")
		step, stepOK := argInt(args, 2, "setCellCondition")
		condition, conditionOK := argInt(args, 3, "setCellCondition")

		if bankOK && trackOK && stepOK && conditionOK {
			engine.SetCellCondition(bank, track, step, drum.TriggerCondition(condition))
		}

		return js.Null()
	}))

	api.Set("setTrackLength", export(func(args []js.Value) any {
		if !ready() {
			return js.Null()
		}

		bank, bankOK := argInt(args, 0, "setTrackLength")
		track, trackOK := argInt(args, 1, "setTrackLength")
		length, lengthOK := argInt(args, 2, "setTrackLength")

		if bankOK && trackOK && lengthOK {
			engine.SetTrackLength(bank, track, length)
		}

		return js.Null()
	}))

	api.Set("setFillMode", export(func(args []js.Value) any {
		if !ready() {
			return js.Null()
		}

		if enabled, ok := argBool(args, 0, "setFillMode"); ok {
			engine.SetFillMode(enabled)
		}

		return js.Null()
	}))

	// setPattern takes a flat track-major Float32Array (index =
	// track*MaxSteps + step) of velocities in [0, 1].
	api.Set("setPattern", export(func(args []js.Value) any {
		if !ready() {
			return js.Null()
		}

		bank, bankOK := argInt(args, 0, "setPattern")
		if !bankOK || len(args) < 2 {
			return js.Null()
		}

		arr := args[1]
		if arr.Type() != js.TypeObject {
			warnBadArg("setPattern")

			return js.Null()
		}

		// Typed arrays and plain arrays both expose a numeric length;
		// anything else is not something we can read velocities from.
		length := arr.Get("length")
		if length.Type() != js.TypeNumber {
			warnBadArg("setPattern")

			return js.Null()
		}

		count := length.Int()
		if count < 0 {
			warnBadArg("setPattern")

			return js.Null()
		}

		if count != drum.PatternSize {
			warnBadArg("setPattern")

			return js.Null()
		}

		for i := range patternInput {
			elem := arr.Index(i)
			if elem.Type() != js.TypeNumber {
				warnBadArg("setPattern")

				return js.Null()
			}

			vel := elem.Float()
			if math.IsNaN(vel) || math.IsInf(vel, 0) {
				warnBadArg("setPattern")

				return js.Null()
			}

			patternInput[i] = vel
		}

		engine.SetPattern(bank, patternInput[:])

		return js.Null()
	}))

	api.Set("setPatternBank", export(func(args []js.Value) any {
		if !ready() {
			return js.Null()
		}

		bank, bankOK := argInt(args, 0, "setPatternBank")
		if !bankOK || len(args) < 2 {
			return js.Null()
		}

		state, stateOK := readPatternBankState(args[1])
		if !stateOK {
			warnBadArg("setPatternBank")

			return js.Null()
		}

		if err := engine.ReplacePatternBank(bank, state); err != nil {
			warnBadArg("setPatternBank")
		}

		return js.Null()
	}))

	api.Set("requestBank", export(func(args []js.Value) any {
		if !ready() {
			return js.Null()
		}

		if bank, ok := argInt(args, 0, "requestBank"); ok {
			engine.RequestBank(bank)
		}

		return js.Null()
	}))

	api.Set("setChain", export(func(args []js.Value) any {
		if !ready() || len(args) < 1 {
			return js.Null()
		}

		chain, ok := readVariableIntArray(args[0], 1, drum.MaxChainLength)
		if !ok {
			warnBadArg("setChain")

			return js.Null()
		}

		engine.SetChain(chain)

		return js.Null()
	}))

	api.Set("setChainEnabled", export(func(args []js.Value) any {
		if !ready() {
			return js.Null()
		}

		if enabled, ok := argBool(args, 0, "setChainEnabled"); ok {
			engine.SetChainEnabled(enabled)
		}

		return js.Null()
	}))

	api.Set("setState", export(func(args []js.Value) any {
		if !ready() {
			return js.Null()
		}

		state, ok := readEngineState(args, 0)
		if !ok {
			warnBadArg("setState")

			return js.Null()
		}

		if err := engine.ReplaceState(state); err != nil {
			warnBadArg("setState")
		}

		return js.Null()
	}))

	api.Set("getState", export(func(args []js.Value) any {
		if !ready() {
			return js.Null()
		}

		return writeEngineState(engine.State())
	}))

	api.Set("setVolume", export(func(args []js.Value) any {
		if !ready() {
			return js.Null()
		}

		track, trackOK := argInt(args, 0, "setVolume")
		volume, volOK := argFloat(args, 1, "setVolume")

		if trackOK && volOK {
			engine.SetVolume(track, volume)
		}

		return js.Null()
	}))

	api.Set("setMuted", export(func(args []js.Value) any {
		if !ready() {
			return js.Null()
		}

		track, trackOK := argInt(args, 0, "setMuted")

		muted, mutedOK := argBool(args, 1, "setMuted")
		if trackOK && mutedOK {
			engine.SetMuted(track, muted)
		}

		return js.Null()
	}))

	api.Set("setDecay", export(func(args []js.Value) any {
		if !ready() {
			return js.Null()
		}

		track, trackOK := argInt(args, 0, "setDecay")
		decay, decayOK := argFloat(args, 1, "setDecay")

		if trackOK && decayOK {
			engine.SetDecay(track, decay)
		}

		return js.Null()
	}))

	api.Set("setVoiceParam", export(func(args []js.Value) any {
		if !ready() {
			return js.Null()
		}

		track, trackOK := argInt(args, 0, "setVoiceParam")
		index, indexOK := argInt(args, 1, "setVoiceParam")
		value, valueOK := argFloat(args, 2, "setVoiceParam")

		if trackOK && indexOK && valueOK {
			engine.SetVoiceParam(track, index, value)
		}

		return js.Null()
	}))

	api.Set("setPhysicalTomParam", export(func(args []js.Value) any {
		if !ready() {
			return js.Null()
		}

		track, trackOK := argInt(args, 0, "setPhysicalTomParam")
		index, indexOK := argInt(args, 1, "setPhysicalTomParam")
		value, valueOK := argFloat(args, 2, "setPhysicalTomParam")

		if trackOK && indexOK && valueOK {
			engine.SetPhysicalTomParam(track, index, value)
		}

		return js.Null()
	}))

	api.Set("setTomModel", export(func(args []js.Value) any {
		if !ready() {
			return js.Null()
		}

		track, trackOK := argInt(args, 0, "setTomModel")

		model, modelOK := argInt(args, 1, "setTomModel")
		if trackOK && modelOK {
			engine.SetTomModel(track, drum.TomModel(model))
		}

		return js.Null()
	}))

	api.Set("triggerVoice", export(func(args []js.Value) any {
		if !ready() {
			return js.Null()
		}

		track, trackOK := argInt(args, 0, "triggerVoice")
		velocity, velOK := argFloat(args, 1, "triggerVoice")

		if trackOK && velOK {
			engine.TriggerVoice(track, velocity)
		}

		return js.Null()
	}))

	api.Set("setReverb", export(func(args []js.Value) any {
		if !ready() {
			return js.Null()
		}

		if amount, ok := argFloat(args, 0, "setReverb"); ok {
			engine.SetReverb(amount)
		}

		return js.Null()
	}))

	api.Set("setProbability", export(func(args []js.Value) any {
		if !ready() {
			return js.Null()
		}

		if prob, ok := argFloat(args, 0, "setProbability"); ok {
			engine.SetProbability(prob)
		}

		return js.Null()
	}))

	api.Set("setHumanize", export(func(args []js.Value) any {
		if !ready() {
			return js.Null()
		}

		if amount, ok := argFloat(args, 0, "setHumanize"); ok {
			engine.SetHumanize(amount)
		}

		return js.Null()
	}))

	api.Set("render", export(func(args []js.Value) any {
		if !ready() {
			return js.Global().Get("Float32Array").New(0)
		}

		sampleCount, ok := argInt(args, 0, "render")
		if !ok || sampleCount <= 0 {
			return js.Global().Get("Float32Array").New(0)
		}

		// Cap the request so a single call cannot ask for an arbitrarily
		// large Go slice plus JS ArrayBuffer.
		if sampleCount > maxRenderSamples {
			sampleCount = maxRenderSamples
		}

		ensureRenderBuffers(sampleCount)

		buf := renderBuf[:sampleCount]
		engine.Render(buf)

		// One bulk copy across the JS boundary instead of one SetIndex
		// call per sample.
		bytes := unsafe.Slice((*byte)(unsafe.Pointer(&buf[0])), sampleCount*4)
		js.CopyBytesToJS(jsBytes, bytes)

		return jsFloats.Call("subarray", 0, sampleCount)
	}))

	api.Set("currentStep", export(func(args []js.Value) any {
		if !ready() {
			return -1
		}

		return engine.CurrentStep()
	}))

	api.Set("transportState", export(func(args []js.Value) any {
		if !ready() {
			return string(drum.TransportStopped)
		}

		return string(engine.TransportSnapshot().State)
	}))

	api.Set("transportRevision", export(func(args []js.Value) any {
		if !ready() {
			return 0
		}

		// JavaScript numbers exactly represent integers through 2^53. A browser
		// session cannot produce enough user transport transitions to approach
		// that boundary, and float64 is an explicitly supported syscall/js value.
		return float64(engine.TransportSnapshot().Revision)
	}))

	api.Set("activeBank", export(func(args []js.Value) any {
		if !ready() {
			return 0
		}

		return engine.ActiveBank()
	}))

	api.Set("queuedBank", export(func(args []js.Value) any {
		if !ready() {
			return drum.NoBank
		}

		return engine.QueuedBank()
	}))

	api.Set("chainPosition", export(func(args []js.Value) any {
		if !ready() {
			return -1
		}

		return engine.ChainPosition()
	}))

	// isIdle reports that the engine has nothing left to render, so the caller
	// can stop pulling chunks (and suspend the AudioContext) instead of paying
	// for silence. An uninitialized engine has produced nothing at all, which
	// is the most idle state there is.
	api.Set("isIdle", export(func(args []js.Value) any {
		if !ready() {
			return true
		}

		return engine.IsIdle()
	}))

	// A plain data property, not an exported func: the worker reads it
	// during the load handshake and refuses to run on a mismatch.
	api.Set("protocolVersion", drum.ProtocolVersion)

	js.Global().Set("AlgoDrum", api)

	select {} // keep Go runtime alive
}

// ready reports whether init has run, logging a single warning the first
// time the API is used too early instead of silently ignoring the call.
func ready() bool {
	if engine != nil {
		return true
	}

	if !warnedBeforeInit {
		warnedBeforeInit = true

		println("algo-drum: API called before init — call ignored")
	}

	return false
}

// warnBadArg logs a single warning the first time the API is called with a
// missing or wrong-typed argument. JS is untyped at the call site, so a bad
// argument must never reach syscall/js conversions like Float() or Bool():
// those panic, which would tear down the whole engine.
func warnBadArg(name string) {
	if warnedBadArg {
		return
	}

	warnedBadArg = true

	println("algo-drum:", name, "called with an invalid argument — call ignored")
}

// argFloat reads a finite number from args[i], reporting false (and warning
// once) when the argument is missing, not a number, or NaN/Inf.
func argFloat(args []js.Value, i int, name string) (float64, bool) {
	if i >= len(args) || args[i].Type() != js.TypeNumber {
		warnBadArg(name)

		return 0, false
	}

	val := args[i].Float()
	if math.IsNaN(val) || math.IsInf(val, 0) {
		warnBadArg(name)

		return 0, false
	}

	return val, true
}

// argInt reads an integer from args[i] with the same guarantees as argFloat,
// additionally rejecting values that do not fit in an int32 and values with a
// fractional part. Truncating (7.9 -> 7) would silently reinterpret a caller
// bug as a valid step, track or sample count, so a non-integer is treated like
// any other invalid argument: warn once and leave engine state untouched.
func argInt(args []js.Value, i int, name string) (int, bool) {
	val, ok := argFloat(args, i, name)
	if !ok {
		return 0, false
	}

	if val > math.MaxInt32 || val < math.MinInt32 || math.Trunc(val) != val {
		warnBadArg(name)

		return 0, false
	}

	return int(val), true
}

// argBool reads a boolean from args[i]. JS truthiness is deliberately not
// applied: only an actual boolean is accepted.
func argBool(args []js.Value, i int, name string) (bool, bool) {
	if i >= len(args) || args[i].Type() != js.TypeBoolean {
		warnBadArg(name)

		return false, false
	}

	return args[i].Bool(), true
}

// ensureRenderBuffers grows the shared Go and JS render buffers so they can
// hold at least sampleCount float32 samples.
func ensureRenderBuffers(sampleCount int) {
	if sampleCount <= len(renderBuf) {
		return
	}

	renderBuf = make([]float32, sampleCount)
	arrayBuf := js.Global().Get("ArrayBuffer").New(sampleCount * 4)
	jsBytes = js.Global().Get("Uint8Array").New(arrayBuf)
	jsFloats = js.Global().Get("Float32Array").New(arrayBuf)
}

func readEngineState(args []js.Value, index int) (drum.EngineState, bool) {
	if index >= len(args) || args[index].Type() != js.TypeObject {
		return drum.EngineState{}, false
	}

	value := args[index]
	tempo, tempoOK := finiteJSNumber(value.Get("tempoBpm"))
	swing, swingOK := finiteJSNumber(value.Get("swing"))
	reverbAmount, reverbOK := finiteJSNumber(value.Get("reverb"))
	probability, probabilityOK := finiteJSNumber(value.Get("probability"))
	humanize, humanizeOK := finiteJSNumber(value.Get("humanize"))
	fillModeValue := value.Get("fillMode")
	standaloneBank, standaloneBankOK := integerJSNumber(value.Get("standaloneBank"))
	chainEnabledValue := value.Get("chainEnabled")
	chain, chainOK := readVariableIntArray(value.Get("chain"), 1, drum.MaxChainLength)
	banksValue := value.Get("banks")
	banksOK := banksValue.Type() == js.TypeObject &&
		js.Global().Get("Array").Call("isArray", banksValue).Bool() &&
		banksValue.Length() == drum.PatternBankCount

	tracksValue := value.Get("tracks")
	tracksOK := tracksValue.Type() == js.TypeObject &&
		js.Global().Get("Array").Call("isArray", tracksValue).Bool() &&
		tracksValue.Length() == drum.TrackCount

	if !tempoOK || !swingOK || !reverbOK || !probabilityOK || !humanizeOK ||
		fillModeValue.Type() != js.TypeBoolean || !standaloneBankOK ||
		chainEnabledValue.Type() != js.TypeBoolean || !chainOK || !banksOK || !tracksOK {
		return drum.EngineState{}, false
	}

	state := drum.EngineState{
		TempoBPM:       tempo,
		Swing:          swing,
		Reverb:         reverbAmount,
		Probability:    probability,
		Humanize:       humanize,
		FillMode:       fillModeValue.Bool(),
		Banks:          make([]drum.PatternBankState, drum.PatternBankCount),
		StandaloneBank: standaloneBank,
		ChainEnabled:   chainEnabledValue.Bool(),
		Chain:          chain,
		Tracks:         make([]drum.TrackState, drum.TrackCount),
	}

	for bank := range state.Banks {
		bankState, ok := readPatternBankState(banksValue.Index(bank))
		if !ok {
			return drum.EngineState{}, false
		}

		state.Banks[bank] = bankState
	}

	for track := range state.Tracks {
		trackValue := tracksValue.Index(track)
		if trackValue.Type() != js.TypeObject {
			return drum.EngineState{}, false
		}

		volume, volumeOK := finiteJSNumber(trackValue.Get("volume"))
		decay, decayOK := finiteJSNumber(trackValue.Get("decay"))
		mutedValue := trackValue.Get("muted")

		params, paramsOK := readFloatArray(
			trackValue.Get("voiceParams"), len(drum.SpecsForTrack(track)),
		)
		if !volumeOK || !decayOK || mutedValue.Type() != js.TypeBoolean || !paramsOK {
			return drum.EngineState{}, false
		}

		state.Tracks[track] = drum.TrackState{
			Volume:      volume,
			Decay:       decay,
			Muted:       mutedValue.Bool(),
			VoiceParams: params,
		}

		tomValue := trackValue.Get("tom")

		isTom := track == 3 || track == 5
		if !isTom {
			if tomValue.Type() != js.TypeUndefined && tomValue.Type() != js.TypeNull {
				return drum.EngineState{}, false
			}

			continue
		}

		if tomValue.Type() != js.TypeObject {
			return drum.EngineState{}, false
		}

		modelValue := tomValue.Get("model")
		if modelValue.Type() != js.TypeString {
			return drum.EngineState{}, false
		}

		var model drum.TomModel

		switch modelValue.String() {
		case "procedural":
			model = drum.TomModelProcedural
		case "physical":
			model = drum.TomModelPhysical
		default:
			return drum.EngineState{}, false
		}

		physicalParams, paramsOK := readFloatArray(
			tomValue.Get("physicalParams"), len(drum.PhysicalTomSpecs()),
		)
		if !paramsOK {
			return drum.EngineState{}, false
		}

		state.Tracks[track].Tom = &drum.TomState{
			Model:          model,
			PhysicalParams: physicalParams,
		}
	}

	return state, true
}

func readPatternBankState(value js.Value) (drum.PatternBankState, bool) {
	if value.Type() != js.TypeObject {
		return drum.PatternBankState{}, false
	}

	stepCount, stepCountOK := integerJSNumber(value.Get("stepCount"))
	pattern, patternOK := readFloatArray(value.Get("pattern"), drum.PatternSize)
	cellProbabilities, cellProbabilitiesOK := readFloatArray(
		value.Get("cellProbabilities"), drum.PatternSize,
	)
	cellHumanize, cellHumanizeOK := readFloatArray(
		value.Get("cellHumanize"), drum.PatternSize,
	)
	cellConditions, cellConditionsOK := readConditionArray(
		value.Get("cellConditions"), drum.PatternSize,
	)
	trackLengths, trackLengthsOK := readIntArray(value.Get("trackLengths"), drum.TrackCount)

	if !stepCountOK || !patternOK || !cellProbabilitiesOK || !cellHumanizeOK ||
		!cellConditionsOK || !trackLengthsOK {
		return drum.PatternBankState{}, false
	}

	return drum.PatternBankState{
		StepCount:         stepCount,
		Pattern:           pattern,
		CellProbabilities: cellProbabilities,
		CellHumanize:      cellHumanize,
		CellConditions:    cellConditions,
		TrackLengths:      trackLengths,
	}, true
}

func finiteJSNumber(value js.Value) (float64, bool) {
	if value.Type() != js.TypeNumber {
		return 0, false
	}

	number := value.Float()

	return number, !math.IsNaN(number) && !math.IsInf(number, 0)
}

func integerJSNumber(value js.Value) (int, bool) {
	number, ok := finiteJSNumber(value)
	if !ok || math.Trunc(number) != number || number < math.MinInt32 || number > math.MaxInt32 {
		return 0, false
	}

	return int(number), true
}

func readFloatArray(value js.Value, expected int) ([]float64, bool) {
	if value.Type() != js.TypeObject {
		return nil, false
	}

	length := value.Get("length")

	count, ok := integerJSNumber(length)
	if !ok || count != expected {
		return nil, false
	}

	result := make([]float64, expected)
	for i := range result {
		number, ok := finiteJSNumber(value.Index(i))
		if !ok {
			return nil, false
		}

		result[i] = number
	}

	return result, true
}

func readIntArray(value js.Value, expected int) ([]int, bool) {
	if value.Type() != js.TypeObject {
		return nil, false
	}

	count, ok := integerJSNumber(value.Get("length"))
	if !ok || count != expected {
		return nil, false
	}

	result := make([]int, expected)
	for i := range result {
		result[i], ok = integerJSNumber(value.Index(i))
		if !ok {
			return nil, false
		}
	}

	return result, true
}

func readVariableIntArray(value js.Value, minLength, maxLength int) ([]int, bool) {
	if value.Type() != js.TypeObject {
		return nil, false
	}

	count, ok := integerJSNumber(value.Get("length"))
	if !ok || count < minLength || count > maxLength {
		return nil, false
	}

	result := make([]int, count)
	for i := range result {
		result[i], ok = integerJSNumber(value.Index(i))
		if !ok {
			return nil, false
		}
	}

	return result, true
}

func readConditionArray(value js.Value, expected int) ([]drum.TriggerCondition, bool) {
	values, ok := readIntArray(value, expected)
	if !ok {
		return nil, false
	}

	result := make([]drum.TriggerCondition, expected)

	for i, value := range values {
		if value < 0 || value > int(drum.TriggerNotPreviousFired) {
			return nil, false
		}

		result[i] = drum.TriggerCondition(value)
	}

	return result, true
}

func writeEngineState(state drum.EngineState) js.Value {
	ensureStateOutput(state)

	stateOut.value.Set("tempoBpm", state.TempoBPM)
	stateOut.value.Set("swing", state.Swing)
	stateOut.value.Set("reverb", state.Reverb)
	stateOut.value.Set("probability", state.Probability)
	stateOut.value.Set("humanize", state.Humanize)
	stateOut.value.Set("fillMode", state.FillMode)
	stateOut.value.Set("standaloneBank", state.StandaloneBank)
	stateOut.value.Set("chainEnabled", state.ChainEnabled)

	if len(stateOut.chain) != len(state.Chain) {
		stateOut.chain = make([]byte, len(state.Chain))
		stateOut.chainBytes = js.Global().Get("Uint8Array").New(len(state.Chain))
		stateOut.value.Set("chain", stateOut.chainBytes)
	}

	for i, bank := range state.Chain {
		stateOut.chain[i] = byte(bank)
	}

	js.CopyBytesToJS(stateOut.chainBytes, stateOut.chain)

	for bank, bankState := range state.Banks {
		output := &stateOut.bankValues[bank]
		output.value.Set("stepCount", bankState.StepCount)
		copyFloatOutput(output.pattern, output.patternBytes, bankState.Pattern)
		copyFloatOutput(
			output.cellProbabilities, output.cellProbabilityBytes, bankState.CellProbabilities,
		)
		copyFloatOutput(
			output.cellHumanize, output.cellHumanizeBytes, bankState.CellHumanize,
		)

		for i, condition := range bankState.CellConditions {
			output.cellConditions[i] = byte(condition)
		}

		js.CopyBytesToJS(output.cellConditionBytes, output.cellConditions)

		for i, length := range bankState.TrackLengths {
			output.trackLengths[i] = byte(length)
		}

		js.CopyBytesToJS(output.trackLengthBytes, output.trackLengths)
	}

	for track, trackState := range state.Tracks {
		trackValue := stateOut.trackValues[track]
		trackValue.Set("volume", trackState.Volume)
		trackValue.Set("decay", trackState.Decay)
		trackValue.Set("muted", trackState.Muted)
		copyFloatOutput(
			stateOut.voiceParams[track], stateOut.voiceParamBytes[track], trackState.VoiceParams,
		)

		if trackState.Tom == nil {
			continue
		}

		tomValue := stateOut.tomValues[track]
		if trackState.Tom.Model == drum.TomModelPhysical {
			tomValue.Set("model", "physical")
		} else {
			tomValue.Set("model", "procedural")
		}

		copyFloatOutput(
			stateOut.physicalParams[track], stateOut.physicalBytes[track],
			trackState.Tom.PhysicalParams,
		)
	}

	return stateOut.value
}

func ensureStateOutput(state drum.EngineState) {
	if stateOut.value.Type() != js.TypeUndefined {
		return
	}

	stateOut.value = js.Global().Get("Object").New()
	stateOut.banks = js.Global().Get("Array").New(len(state.Banks))
	stateOut.value.Set("banks", stateOut.banks)

	stateOut.chain = make([]byte, len(state.Chain))
	stateOut.chainBytes = js.Global().Get("Uint8Array").New(len(state.Chain))
	stateOut.value.Set("chain", stateOut.chainBytes)

	for bank, bankState := range state.Banks {
		output := &stateOut.bankValues[bank]
		output.value = js.Global().Get("Object").New()
		stateOut.banks.SetIndex(bank, output.value)

		var patternView js.Value

		output.pattern, output.patternBytes, patternView = newFloatOutput(len(bankState.Pattern))
		output.value.Set("pattern", patternView)

		var cellProbabilityView js.Value

		output.cellProbabilities, output.cellProbabilityBytes, cellProbabilityView = newFloatOutput(len(bankState.CellProbabilities))
		output.value.Set("cellProbabilities", cellProbabilityView)

		var cellHumanizeView js.Value

		output.cellHumanize, output.cellHumanizeBytes, cellHumanizeView = newFloatOutput(len(bankState.CellHumanize))
		output.value.Set("cellHumanize", cellHumanizeView)

		output.cellConditions = make([]byte, len(bankState.CellConditions))
		cellConditionView := js.Global().Get("Uint8Array").New(len(bankState.CellConditions))
		output.cellConditionBytes = cellConditionView
		output.value.Set("cellConditions", cellConditionView)

		output.trackLengths = make([]byte, len(bankState.TrackLengths))
		trackLengthView := js.Global().Get("Uint8Array").New(len(bankState.TrackLengths))
		output.trackLengthBytes = trackLengthView
		output.value.Set("trackLengths", trackLengthView)
	}

	stateOut.tracks = js.Global().Get("Array").New(len(state.Tracks))
	stateOut.value.Set("tracks", stateOut.tracks)

	for track, trackState := range state.Tracks {
		trackValue := js.Global().Get("Object").New()
		stateOut.trackValues[track] = trackValue
		stateOut.tracks.SetIndex(track, trackValue)

		var paramsView js.Value

		stateOut.voiceParams[track], stateOut.voiceParamBytes[track], paramsView = newFloatOutput(len(trackState.VoiceParams))
		trackValue.Set("voiceParams", paramsView)

		if trackState.Tom == nil {
			continue
		}

		tomValue := js.Global().Get("Object").New()
		stateOut.tomValues[track] = tomValue
		trackValue.Set("tom", tomValue)

		var physicalView js.Value

		stateOut.physicalParams[track], stateOut.physicalBytes[track], physicalView = newFloatOutput(len(trackState.Tom.PhysicalParams))
		tomValue.Set("physicalParams", physicalView)
	}
}

func newFloatOutput(length int) ([]float32, js.Value, js.Value) {
	values := make([]float32, length)
	arrayBuf := js.Global().Get("ArrayBuffer").New(length * 4)
	bytes := js.Global().Get("Uint8Array").New(arrayBuf)
	floats := js.Global().Get("Float32Array").New(arrayBuf)

	return values, bytes, floats
}

func copyFloatOutput(dst []float32, jsBytes js.Value, src []float64) {
	for i, value := range src {
		dst[i] = float32(value)
	}

	if len(dst) == 0 {
		return
	}

	bytes := unsafe.Slice((*byte)(unsafe.Pointer(&dst[0])), len(dst)*4)
	js.CopyBytesToJS(jsBytes, bytes)
}

func export(fn func([]js.Value) any) js.Func {
	f := js.FuncOf(func(_ js.Value, args []js.Value) any {
		return fn(args)
	})
	funcs = append(funcs, f)

	return f
}
