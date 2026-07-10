//go:build js && wasm

package main

import (
	"syscall/js"
	"unsafe"

	"github.com/cwbudde/algo-drum/internal/drum"
)

var (
	engine *drum.Engine
	funcs  []js.Func

	// warnedBeforeInit gates the "API used before init" console warning so
	// a burst of early calls logs once instead of spamming.
	warnedBeforeInit bool

	// Persistent render buffers, grown on demand — render() is called for
	// every audio chunk, so per-call allocation would churn both GCs.
	renderBuf []float32
	jsBytes   js.Value // Uint8Array over the same allocation as jsFloats
	jsFloats  js.Value // Float32Array returned to the caller
)

func main() {
	api := js.Global().Get("Object").New()

	api.Set("init", export(func(args []js.Value) any {
		sr := 48000.0
		if len(args) > 0 {
			sr = args[0].Float()
		}

		engine = drum.NewEngine(sr)

		return js.Null()
	}))

	api.Set("setRunning", export(func(args []js.Value) any {
		if ready() && len(args) > 0 {
			engine.SetRunning(args[0].Bool())
		}

		return js.Null()
	}))

	api.Set("setTempo", export(func(args []js.Value) any {
		if ready() && len(args) > 0 {
			engine.SetTempo(args[0].Float())
		}

		return js.Null()
	}))

	api.Set("setSwing", export(func(args []js.Value) any {
		if ready() && len(args) > 0 {
			engine.SetSwing(args[0].Float())
		}

		return js.Null()
	}))

	api.Set("setStepCount", export(func(args []js.Value) any {
		if ready() && len(args) > 0 {
			engine.SetStepCount(args[0].Int())
		}

		return js.Null()
	}))

	api.Set("setCell", export(func(args []js.Value) any {
		if ready() && len(args) >= 3 {
			engine.SetCell(args[0].Int(), args[1].Int(), args[2].Float())
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

		n := arr.Get("length").Int()
		if n > drum.TrackCount*drum.MaxSteps {
			n = drum.TrackCount * drum.MaxSteps
		}

		velocities := make([]float64, n)
		for i := range velocities {
			velocities[i] = arr.Index(i).Float()
		}

		engine.SetPattern(velocities)

		return js.Null()
	}))

	// getPattern returns the pattern in the same flat Float32Array layout
	// that setPattern accepts. Called rarely (state sync), so the per-call
	// allocation and element-wise copy are fine.
	api.Set("getPattern", export(func(args []js.Value) any {
		if !ready() {
			return js.Global().Get("Float32Array").New(0)
		}

		pattern := engine.Pattern()

		out := js.Global().Get("Float32Array").New(len(pattern))
		for i, vel := range pattern {
			out.SetIndex(i, vel)
		}

		return out
	}))

	api.Set("setVolume", export(func(args []js.Value) any {
		if ready() && len(args) >= 2 {
			engine.SetVolume(args[0].Int(), args[1].Float())
		}

		return js.Null()
	}))

	api.Set("setDecay", export(func(args []js.Value) any {
		if ready() && len(args) >= 2 {
			engine.SetDecay(args[0].Int(), args[1].Float())
		}

		return js.Null()
	}))

	api.Set("setReverb", export(func(args []js.Value) any {
		if ready() && len(args) > 0 {
			engine.SetReverb(args[0].Float())
		}

		return js.Null()
	}))

	api.Set("setProbability", export(func(args []js.Value) any {
		if ready() && len(args) > 0 {
			engine.SetProbability(args[0].Float())
		}

		return js.Null()
	}))

	api.Set("setHumanize", export(func(args []js.Value) any {
		if ready() && len(args) > 0 {
			engine.SetHumanize(args[0].Float())
		}

		return js.Null()
	}))

	api.Set("render", export(func(args []js.Value) any {
		if !ready() || len(args) < 1 {
			return js.Global().Get("Float32Array").New(0)
		}

		n := args[0].Int()
		if n <= 0 {
			return js.Global().Get("Float32Array").New(0)
		}

		ensureRenderBuffers(n)
		buf := renderBuf[:n]
		engine.Render(buf)

		// One bulk copy across the JS boundary instead of n SetIndex calls.
		bytes := unsafe.Slice((*byte)(unsafe.Pointer(&buf[0])), n*4)
		js.CopyBytesToJS(jsBytes, bytes)

		return jsFloats.Call("subarray", 0, n)
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

// ensureRenderBuffers grows the shared Go and JS render buffers so they can
// hold at least n float32 samples.
func ensureRenderBuffers(n int) {
	if n <= len(renderBuf) {
		return
	}

	renderBuf = make([]float32, n)
	arrayBuf := js.Global().Get("ArrayBuffer").New(n * 4)
	jsBytes = js.Global().Get("Uint8Array").New(arrayBuf)
	jsFloats = js.Global().Get("Float32Array").New(arrayBuf)
}

func export(fn func([]js.Value) any) js.Func {
	f := js.FuncOf(func(_ js.Value, args []js.Value) any {
		return fn(args)
	})
	funcs = append(funcs, f)

	return f
}
