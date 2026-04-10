//go:build js && wasm

package main

import (
	"syscall/js"

	"github.com/cwbudde/algo-drum/internal/drum"
)

var (
	engine *drum.Engine
	funcs  []js.Func
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
		if engine != nil && len(args) > 0 {
			engine.SetRunning(args[0].Bool())
		}

		return js.Null()
	}))

	api.Set("setTempo", export(func(args []js.Value) any {
		if engine != nil && len(args) > 0 {
			engine.SetTempo(args[0].Float())
		}

		return js.Null()
	}))

	api.Set("setSwing", export(func(args []js.Value) any {
		if engine != nil && len(args) > 0 {
			engine.SetSwing(args[0].Float())
		}

		return js.Null()
	}))

	api.Set("setCell", export(func(args []js.Value) any {
		if engine != nil && len(args) >= 3 {
			engine.SetCell(args[0].Int(), args[1].Int(), args[2].Bool())
		}

		return js.Null()
	}))

	api.Set("setVolume", export(func(args []js.Value) any {
		if engine != nil && len(args) >= 2 {
			engine.SetVolume(args[0].Int(), args[1].Float())
		}

		return js.Null()
	}))

	api.Set("setDecay", export(func(args []js.Value) any {
		if engine != nil && len(args) >= 2 {
			engine.SetDecay(args[0].Int(), args[1].Float())
		}

		return js.Null()
	}))

	api.Set("setReverb", export(func(args []js.Value) any {
		if engine != nil && len(args) > 0 {
			engine.SetReverb(args[0].Float())
		}

		return js.Null()
	}))

	api.Set("setKit", export(func(args []js.Value) any {
		if engine != nil && len(args) > 0 {
			engine.SetKit(args[0].Int())
		}

		return js.Null()
	}))

	api.Set("render", export(func(args []js.Value) any {
		if engine == nil || len(args) < 1 {
			return js.Global().Get("Float32Array").New(0)
		}

		n := args[0].Int()
		buf := make([]float32, n)
		engine.Render(buf)

		arr := js.Global().Get("Float32Array").New(n)
		for i, v := range buf {
			arr.SetIndex(i, v)
		}

		return arr
	}))

	api.Set("currentStep", export(func(args []js.Value) any {
		if engine == nil {
			return -1
		}

		return engine.CurrentStep()
	}))

	js.Global().Set("AlgoDrum", api)

	select {} // keep Go runtime alive
}

func export(fn func([]js.Value) any) js.Func {
	f := js.FuncOf(func(_ js.Value, args []js.Value) any {
		return fn(args)
	})
	funcs = append(funcs, f)

	return f
}
