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
	patternBuf   [drum.PatternSize]float32
	patternBytes js.Value
	patternJS    js.Value
)

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

		if steps, ok := argInt(args, 0, "setStepCount"); ok {
			engine.SetStepCount(steps)
		}

		return js.Null()
	}))

	api.Set("setCell", export(func(args []js.Value) any {
		if !ready() {
			return js.Null()
		}

		track, trackOK := argInt(args, 0, "setCell")
		step, stepOK := argInt(args, 1, "setCell")
		velocity, velOK := argFloat(args, 2, "setCell")

		if trackOK && stepOK && velOK {
			engine.SetCell(track, step, velocity)
		}

		return js.Null()
	}))

	// setPattern takes a flat track-major Float32Array (index =
	// track*MaxSteps + step) of velocities in [0, 1].
	api.Set("setPattern", export(func(args []js.Value) any {
		if !ready() || len(args) < 1 {
			return js.Null()
		}

		arr := args[0]
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

		engine.SetPattern(patternInput[:])

		return js.Null()
	}))

	// getPattern returns the pattern in a persistent JS-owned Float32Array.
	// postMessage structured-clones it in audioWorker.ts; transferring it there
	// would detach this reusable buffer and violate the allocation-free path.
	api.Set("getPattern", export(func(args []js.Value) any {
		if !ready() {
			return js.Global().Get("Float32Array").New(0)
		}

		ensurePatternBuffer()
		engine.CopyPattern(&patternBuf)
		bytes := unsafe.Slice((*byte)(unsafe.Pointer(&patternBuf[0])), drum.PatternSize*4)
		js.CopyBytesToJS(patternBytes, bytes)

		return patternJS
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

func ensurePatternBuffer() {
	if patternJS.Type() != js.TypeUndefined {
		return
	}

	arrayBuf := js.Global().Get("ArrayBuffer").New(drum.PatternSize * 4)
	patternBytes = js.Global().Get("Uint8Array").New(arrayBuf)
	patternJS = js.Global().Get("Float32Array").New(arrayBuf)
}

func export(fn func([]js.Value) any) js.Func {
	f := js.FuncOf(func(_ js.Value, args []js.Value) any {
		return fn(args)
	})
	funcs = append(funcs, f)

	return f
}
