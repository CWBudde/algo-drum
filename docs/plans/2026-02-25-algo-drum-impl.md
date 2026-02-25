# algo-drum Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a browser-based drum computer with a wood-finish canvas UI, Go/WASM synthesis engine, and GitHub Pages deployment.

**Architecture:** Go module (`github.com/MeKo-Tech/algo-drum`) imports `github.com/cwbudde/algo-dsp v0.1.0` for biquad filters; compiled to `algo_drum.wasm`. React 18 + TypeScript + Vite 7 frontend (Bun) drives the UI and audio via `ScriptProcessorNode`.

**Tech Stack:** Go 1.25, algo-dsp v0.1.0, React 18, TypeScript 5, Vite 7, Bun, GitHub Actions, GitHub Pages

---

## Task 1: Go Module Scaffold

**Files:**
- Create: `go.mod`
- Create: `cmd/wasm/main.go` (stub)
- Create: `internal/drum/voices.go` (stub)
- Create: `internal/drum/engine.go` (stub)

**Step 1: Create go.mod**

```
module github.com/MeKo-Tech/algo-drum

go 1.25

require github.com/cwbudde/algo-dsp v0.1.0
```

Save to `go.mod`.

**Step 2: Create stub files**

`internal/drum/voices.go`:
```go
package drum
```

`internal/drum/engine.go`:
```go
package drum
```

`cmd/wasm/main.go`:
```go
//go:build js && wasm

package main

func main() {}
```

**Step 3: Fetch dependency**

```bash
go mod tidy
```

Expected: `go.sum` created, no errors.

**Step 4: Verify build**

```bash
GOOS=js GOARCH=wasm go build ./cmd/wasm/
```

Expected: no output (success). Temporary binary created; delete it.

**Step 5: Commit**

```bash
git add go.mod go.sum cmd/ internal/
git commit -m "feat: scaffold Go module with algo-dsp dependency"
```

---

## Task 2: Drum Voice Synthesis

**Files:**
- Modify: `internal/drum/voices.go`

Five 808-style voices. Uses `biquad.Section.ProcessSample` (sample-by-sample) and `design.Bandpass`/`design.Highpass` for filter coefficients from algo-dsp.

**Step 1: Write `internal/drum/voices.go`**

```go
package drum

import (
	"math"
	"math/rand"

	"github.com/cwbudde/algo-dsp/dsp/filter/biquad"
	"github.com/cwbudde/algo-dsp/dsp/filter/design"
)

// Voice is a single-shot drum synthesizer voice.
type Voice interface {
	Trigger()
	Tick() float64
	IsActive() bool
}

// ── Bass Drum ──────────────────────────────────────────────────────────────

type BassDrum struct {
	sr        float64
	active    bool
	age       int
	phase     float64
	env       float64
	envDecay  float64
	pitchFrom float64
	pitchTo   float64
	pitchTC   float64 // pitch time-constant in samples
}

func NewBassDrum(sr float64) *BassDrum {
	return &BassDrum{
		sr:        sr,
		envDecay:  math.Exp(-1.0 / (sr * 0.45)),
		pitchFrom: 200.0,
		pitchTo:   50.0,
		pitchTC:   sr * 0.06,
	}
}

func (v *BassDrum) Trigger() {
	v.active = true
	v.age = 0
	v.env = 1.0
	v.phase = 0
}

func (v *BassDrum) IsActive() bool { return v.active }

func (v *BassDrum) Tick() float64 {
	if !v.active {
		return 0
	}
	t := float64(v.age) / v.pitchTC
	freq := v.pitchTo + (v.pitchFrom-v.pitchTo)*math.Exp(-t*5)
	v.phase += 2 * math.Pi * freq / v.sr
	if v.phase > 2*math.Pi {
		v.phase -= 2 * math.Pi
	}
	sample := math.Sin(v.phase) * v.env
	v.env *= v.envDecay
	if v.env < 1e-4 {
		v.active = false
	}
	v.age++
	return sample
}

// ── Snare ──────────────────────────────────────────────────────────────────

type Snare struct {
	sr         float64
	active     bool
	age        int
	phase      float64
	toneEnv    float64
	toneDecay  float64
	noiseEnv   float64
	noiseDecay float64
	hpFilter   biquad.Section
	rng        *rand.Rand
}

func NewSnare(sr float64) *Snare {
	hpCoeffs := design.Highpass(2000, 0.7, sr)
	return &Snare{
		sr:         sr,
		toneDecay:  math.Exp(-1.0 / (sr * 0.12)),
		noiseDecay: math.Exp(-1.0 / (sr * 0.18)),
		hpFilter:   *biquad.NewSection(hpCoeffs),
		rng:        rand.New(rand.NewSource(42)),
	}
}

func (v *Snare) Trigger() {
	v.active = true
	v.age = 0
	v.toneEnv = 0.7
	v.noiseEnv = 1.0
	v.phase = 0
	v.hpFilter.Reset()
}

func (v *Snare) IsActive() bool { return v.active }

func (v *Snare) Tick() float64 {
	if !v.active {
		return 0
	}
	v.phase += 2 * math.Pi * 200 / v.sr
	if v.phase > 2*math.Pi {
		v.phase -= 2 * math.Pi
	}
	tone := math.Sin(v.phase) * v.toneEnv
	noise := (v.rng.Float64()*2 - 1) * v.noiseEnv
	noise = v.hpFilter.ProcessSample(noise)
	v.toneEnv *= v.toneDecay
	v.noiseEnv *= v.noiseDecay
	if v.toneEnv < 1e-4 && v.noiseEnv < 1e-4 {
		v.active = false
	}
	v.age++
	return tone + noise
}

// ── Hi-Hat ─────────────────────────────────────────────────────────────────

type HiHat struct {
	sr       float64
	active   bool
	age      int
	env      float64
	envDecay float64
	bpFilter biquad.Section
	rng      *rand.Rand
}

func NewHiHat(sr float64, closed bool) *HiHat {
	bpCoeffs := design.Bandpass(10000, 2.0, sr)
	decayS := 0.04
	if !closed {
		decayS = 0.4
	}
	return &HiHat{
		sr:       sr,
		envDecay: math.Exp(-1.0 / (sr * decayS)),
		bpFilter: *biquad.NewSection(bpCoeffs),
		rng:      rand.New(rand.NewSource(123)),
	}
}

func (v *HiHat) Trigger() {
	v.active = true
	v.age = 0
	v.env = 1.0
	v.bpFilter.Reset()
}

func (v *HiHat) IsActive() bool { return v.active }

func (v *HiHat) Tick() float64 {
	if !v.active {
		return 0
	}
	noise := (v.rng.Float64()*2 - 1) * v.env
	sample := v.bpFilter.ProcessSample(noise)
	v.env *= v.envDecay
	if v.env < 1e-4 {
		v.active = false
	}
	v.age++
	return sample * 1.5
}

// ── Tom ────────────────────────────────────────────────────────────────────

type Tom struct {
	sr        float64
	active    bool
	age       int
	phase     float64
	env       float64
	envDecay  float64
	pitchFrom float64
	pitchTo   float64
	pitchTC   float64
}

func NewTom(sr float64) *Tom {
	return &Tom{
		sr:        sr,
		envDecay:  math.Exp(-1.0 / (sr * 0.35)),
		pitchFrom: 120.0,
		pitchTo:   60.0,
		pitchTC:   sr * 0.1,
	}
}

func (v *Tom) Trigger() {
	v.active = true
	v.age = 0
	v.env = 1.0
	v.phase = 0
}

func (v *Tom) IsActive() bool { return v.active }

func (v *Tom) Tick() float64 {
	if !v.active {
		return 0
	}
	t := float64(v.age) / v.pitchTC
	freq := v.pitchTo + (v.pitchFrom-v.pitchTo)*math.Exp(-t*5)
	v.phase += 2 * math.Pi * freq / v.sr
	if v.phase > 2*math.Pi {
		v.phase -= 2 * math.Pi
	}
	sample := math.Sin(v.phase) * v.env
	v.env *= v.envDecay
	if v.env < 1e-4 {
		v.active = false
	}
	v.age++
	return sample * 0.9
}

// ── Cymbal ─────────────────────────────────────────────────────────────────

type Cymbal struct {
	sr       float64
	active   bool
	age      int
	env      float64
	envDecay float64
	bpFilter biquad.Section
	rng      *rand.Rand
}

func NewCymbal(sr float64) *Cymbal {
	bpCoeffs := design.Bandpass(7000, 1.2, sr)
	return &Cymbal{
		sr:       sr,
		envDecay: math.Exp(-1.0 / (sr * 1.2)),
		bpFilter: *biquad.NewSection(bpCoeffs),
		rng:      rand.New(rand.NewSource(999)),
	}
}

func (v *Cymbal) Trigger() {
	v.active = true
	v.age = 0
	v.env = 1.0
	v.bpFilter.Reset()
}

func (v *Cymbal) IsActive() bool { return v.active }

func (v *Cymbal) Tick() float64 {
	if !v.active {
		return 0
	}
	noise := (v.rng.Float64()*2 - 1) * v.env
	sample := v.bpFilter.ProcessSample(noise)
	v.env *= v.envDecay
	if v.env < 1e-4 {
		v.active = false
	}
	v.age++
	return sample * 1.2
}
```

**Step 2: Verify it compiles**

```bash
go build ./internal/drum/
```

Expected: no errors.

**Step 3: Commit**

```bash
git add internal/drum/voices.go
git commit -m "feat: add 808-style drum voice synthesizers"
```

---

## Task 3: Sequencer Engine

**Files:**
- Modify: `internal/drum/engine.go`

**Step 1: Write `internal/drum/engine.go`**

```go
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
	stepSamples int64 // samples elapsed in current step
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
// Steps are 8th notes. Swing delays odd-numbered steps relative to even ones.
// swing=0: all equal. swing=0.5: even steps get 1.5× base, odd get 0.5× base.
func (e *Engine) recomputeStepLengths() {
	base := e.sr * 60.0 / e.bpm / 2.0 // samples per 8th note
	s := e.swing * 0.5                 // clamp effective swing factor
	for i := range e.stepLen {
		if i%2 == 0 {
			e.stepLen[i] = int64(base * (1.0 + s))
		} else {
			e.stepLen[i] = int64(base * (1.0 - s))
		}
	}
}

func (e *Engine) SetRunning(r bool) { e.running = r }

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
				// Trigger voices for this step
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

		// Mix all voices
		var out float64
		for t, v := range e.voices {
			out += v.Tick() * e.volumes[t]
		}
		// Soft clip
		buf[i] = float32(softClip(out * 0.8))
	}
}

// softClip applies a tanh-like soft saturation to prevent hard clipping.
func softClip(x float64) float64 {
	if x > 1 {
		return 1
	}
	if x < -1 {
		return -1
	}
	return x * (1.5 - 0.5*x*x)
}
```

**Step 2: Verify it compiles**

```bash
go build ./internal/drum/
```

Expected: no errors.

**Step 3: Commit**

```bash
git add internal/drum/engine.go
git commit -m "feat: add drum sequencer engine with swing"
```

---

## Task 4: WASM Entry Point

**Files:**
- Modify: `cmd/wasm/main.go`

**Step 1: Write `cmd/wasm/main.go`**

```go
//go:build js && wasm

package main

import (
	"syscall/js"

	"github.com/MeKo-Tech/algo-drum/internal/drum"
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
```

**Step 2: Verify WASM compiles**

```bash
GOOS=js GOARCH=wasm go build -o /tmp/test.wasm ./cmd/wasm/
```

Expected: `/tmp/test.wasm` created (will be ~5–15 MB). No errors.

**Step 3: Commit**

```bash
git add cmd/wasm/main.go
git commit -m "feat: add WASM entry point with AlgoDrum JS API"
```

---

## Task 5: WASM Build Script

**Files:**
- Create: `scripts/build-wasm.sh`

**Step 1: Write the script**

```bash
#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
OUT_DIR="$ROOT_DIR/web/public"

mkdir -p "$OUT_DIR"

# Copy wasm_exec.js from Go installation
WASM_EXEC=""
for candidate in \
    "$(go env GOROOT)/lib/wasm/wasm_exec.js" \
    "$(go env GOROOT)/misc/wasm/wasm_exec.js"; do
    if [[ -f "$candidate" ]]; then
        WASM_EXEC="$candidate"
        break
    fi
done

if [[ -z "$WASM_EXEC" ]]; then
    echo "ERROR: wasm_exec.js not found under GOROOT=$(go env GOROOT)" >&2
    exit 1
fi

cp "$WASM_EXEC" "$OUT_DIR/wasm_exec.js"
GOOS=js GOARCH=wasm go build -o "$OUT_DIR/algo_drum.wasm" "$ROOT_DIR/cmd/wasm/"

echo "Built $OUT_DIR/algo_drum.wasm"
echo "Copied $OUT_DIR/wasm_exec.js"
```

**Step 2: Make executable and run**

```bash
chmod +x scripts/build-wasm.sh
bash scripts/build-wasm.sh
```

Expected:
```
Built .../web/public/algo_drum.wasm
Copied .../web/public/wasm_exec.js
```

**Step 3: Add wasm artifacts to .gitignore**

Create `.gitignore`:
```
web/public/algo_drum.wasm
web/public/wasm_exec.js
web/node_modules/
web/dist/
```

**Step 4: Commit**

```bash
git add scripts/build-wasm.sh .gitignore
git commit -m "feat: add WASM build script"
```

---

## Task 6: Vite + React Frontend Scaffold

**Files:**
- Create: `web/package.json`
- Create: `web/vite.config.ts`
- Create: `web/tsconfig.json`
- Create: `web/index.html`
- Create: `web/src/main.tsx`

**Step 1: Write `web/package.json`**

```json
{
  "name": "algo-drum",
  "version": "0.1.0",
  "private": true,
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "tsc -b && vite build",
    "preview": "vite preview"
  },
  "dependencies": {
    "react": "^18.3.1",
    "react-dom": "^18.3.1"
  },
  "devDependencies": {
    "@types/react": "^18.3.12",
    "@types/react-dom": "^18.3.1",
    "@vitejs/plugin-react": "^4.3.4",
    "typescript": "^5.6.3",
    "vite": "^7.0.0"
  }
}
```

**Step 2: Write `web/vite.config.ts`**

```ts
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  base: '/algo-drum/',   // GitHub Pages repo name
  server: {
    headers: {
      // Required for SharedArrayBuffer (future AudioWorklet upgrade)
      'Cross-Origin-Opener-Policy': 'same-origin',
      'Cross-Origin-Embedder-Policy': 'require-corp',
    },
  },
  optimizeDeps: {
    exclude: [], // don't pre-bundle WASM
  },
})
```

**Step 3: Write `web/tsconfig.json`**

```json
{
  "compilerOptions": {
    "target": "ES2020",
    "lib": ["ES2020", "DOM", "DOM.Iterable"],
    "module": "ESNext",
    "moduleResolution": "bundler",
    "jsx": "react-jsx",
    "strict": true,
    "noUnusedLocals": true,
    "noUnusedParameters": true,
    "noFallthroughCasesInSwitch": true,
    "skipLibCheck": true
  },
  "include": ["src"]
}
```

**Step 4: Write `web/index.html`**

```html
<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>algo-drum</title>
    <script src="/algo-drum/wasm_exec.js"></script>
    <style>
      * { margin: 0; padding: 0; box-sizing: border-box; }
      body { background: #2a1f14; display: flex; justify-content: center; align-items: center; min-height: 100vh; }
    </style>
  </head>
  <body>
    <div id="root"></div>
    <script type="module" src="/src/main.tsx"></script>
  </body>
</html>
```

**Step 5: Write `web/src/main.tsx`**

```tsx
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import App from './App'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
```

**Step 6: Install dependencies**

```bash
cd web && bun install
```

Expected: `node_modules/` created, no errors.

**Step 7: Commit**

```bash
cd ..
git add web/package.json web/vite.config.ts web/tsconfig.json web/index.html web/src/main.tsx web/bun.lock
git commit -m "feat: scaffold Vite 7 React TypeScript frontend"
```

---

## Task 7: WASM Bridge

**Files:**
- Create: `web/src/engine/wasmEngine.ts`

This module loads `algo_drum.wasm`, sets up `AudioContext` + `ScriptProcessorNode`, and exposes a typed API to React.

**Step 1: Write `web/src/engine/wasmEngine.ts`**

```ts
// Type declaration for the Go-exported global
declare global {
  interface Window {
    AlgoDrum: {
      init: (sampleRate: number) => void
      setRunning: (playing: boolean) => void
      setTempo: (bpm: number) => void
      setSwing: (swing: number) => void
      setCell: (track: number, step: number, active: boolean) => void
      setVolume: (track: number, vol: number) => void
      render: (n: number) => Float32Array
      currentStep: () => number
    }
  }
}

// Go runtime — loaded via <script> in index.html
declare class Go {
  importObject: WebAssembly.Imports
  run(instance: WebAssembly.Instance): Promise<void>
}

let audioCtx: AudioContext | null = null
// eslint-disable-next-line @typescript-eslint/no-explicit-any
let processor: ScriptProcessorNode | null = null
let wasmReady = false

export async function loadWasm(): Promise<void> {
  if (wasmReady) return

  const go = new (window as unknown as { Go: typeof Go }).Go()
  const result = await WebAssembly.instantiateStreaming(
    fetch(import.meta.env.BASE_URL + 'algo_drum.wasm'),
    go.importObject,
  )
  go.run(result.instance) // keeps running via select{}
  wasmReady = true
}

export function startAudio(): void {
  if (audioCtx) return

  audioCtx = new AudioContext({ sampleRate: 48000 })
  window.AlgoDrum.init(audioCtx.sampleRate)

  const bufferSize = 4096
  // ScriptProcessorNode: deprecated but works without COOP/COEP on all browsers
  processor = audioCtx.createScriptProcessor(bufferSize, 0, 1)
  processor.onaudioprocess = (e) => {
    const output = e.outputBuffer.getChannelData(0)
    const samples = window.AlgoDrum.render(bufferSize)
    output.set(samples)
  }
  processor.connect(audioCtx.destination)
}

export function stopAudio(): void {
  window.AlgoDrum.setRunning(false)
}

export function play(): void {
  startAudio()
  window.AlgoDrum.setRunning(true)
}

export function stop(): void {
  window.AlgoDrum.setRunning(false)
}

export function setTempo(bpm: number): void {
  if (wasmReady) window.AlgoDrum.setTempo(bpm)
}

export function setSwing(swing: number): void {
  if (wasmReady) window.AlgoDrum.setSwing(swing)
}

export function setCell(track: number, step: number, active: boolean): void {
  if (wasmReady) window.AlgoDrum.setCell(track, step, active)
}

export function setVolume(track: number, vol: number): void {
  if (wasmReady) window.AlgoDrum.setVolume(track, vol)
}

export function currentStep(): number {
  if (!wasmReady) return -1
  return window.AlgoDrum.currentStep()
}
```

**Step 2: Commit**

```bash
git add web/src/engine/wasmEngine.ts
git commit -m "feat: add WASM bridge with AudioContext integration"
```

---

## Task 8: Knob Component

**Files:**
- Create: `web/src/components/Knob.tsx`

SVG rotary knob. Drag vertically to change value. Range 0–1, displayed as arc.

**Step 1: Write `web/src/components/Knob.tsx`**

```tsx
import { useCallback, useEffect, useRef, useState } from 'react'

interface KnobProps {
  value: number           // 0.0 – 1.0
  onChange: (v: number) => void
  label: string
  size?: number           // diameter in px, default 48
  color?: string
}

const MIN_ANGLE = -135   // degrees from 12 o'clock, counter-clockwise
const MAX_ANGLE = 135    // degrees from 12 o'clock, clockwise

function valueToAngle(v: number) {
  return MIN_ANGLE + v * (MAX_ANGLE - MIN_ANGLE)
}

function polarToXY(cx: number, cy: number, r: number, angleDeg: number) {
  const rad = ((angleDeg - 90) * Math.PI) / 180
  return {
    x: cx + r * Math.cos(rad),
    y: cy + r * Math.sin(rad),
  }
}

function describeArc(cx: number, cy: number, r: number, startDeg: number, endDeg: number) {
  const start = polarToXY(cx, cy, r, startDeg)
  const end = polarToXY(cx, cy, r, endDeg)
  const largeArc = endDeg - startDeg > 180 ? 1 : 0
  return `M ${start.x} ${start.y} A ${r} ${r} 0 ${largeArc} 1 ${end.x} ${end.y}`
}

export default function Knob({ value, onChange, label, size = 48, color = '#5C8A6A' }: KnobProps) {
  const dragRef = useRef<{ startY: number; startVal: number } | null>(null)
  const [dragging, setDragging] = useState(false)

  const handleMouseDown = useCallback((e: React.MouseEvent) => {
    e.preventDefault()
    dragRef.current = { startY: e.clientY, startVal: value }
    setDragging(true)
  }, [value])

  useEffect(() => {
    if (!dragging) return
    const onMove = (e: MouseEvent) => {
      if (!dragRef.current) return
      const delta = (dragRef.current.startY - e.clientY) / 150
      const next = Math.max(0, Math.min(1, dragRef.current.startVal + delta))
      onChange(next)
    }
    const onUp = () => {
      setDragging(false)
      dragRef.current = null
    }
    window.addEventListener('mousemove', onMove)
    window.addEventListener('mouseup', onUp)
    return () => {
      window.removeEventListener('mousemove', onMove)
      window.removeEventListener('mouseup', onUp)
    }
  }, [dragging, onChange])

  const cx = size / 2
  const cy = size / 2
  const r = size * 0.38
  const indicatorAngle = valueToAngle(value)
  const indicator = polarToXY(cx, cy, r * 0.7, indicatorAngle)
  const trackArc = describeArc(cx, cy, r, MIN_ANGLE, MAX_ANGLE)
  const valueArc = describeArc(cx, cy, r, MIN_ANGLE, indicatorAngle)

  return (
    <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 4, userSelect: 'none' }}>
      <svg
        width={size}
        height={size}
        onMouseDown={handleMouseDown}
        style={{ cursor: 'ns-resize' }}
      >
        {/* Knob body */}
        <circle cx={cx} cy={cy} r={r} fill="#3D2B1F" stroke="#6B4C38" strokeWidth={1.5} />
        {/* Track arc */}
        <path d={trackArc} fill="none" stroke="#5A3F2E" strokeWidth={3} strokeLinecap="round" />
        {/* Value arc */}
        <path d={valueArc} fill="none" stroke={color} strokeWidth={3} strokeLinecap="round" />
        {/* Indicator dot */}
        <circle cx={indicator.x} cy={indicator.y} r={2.5} fill={color} />
      </svg>
      <span style={{ color: '#C4A07A', fontSize: 10, fontFamily: 'Georgia, serif', letterSpacing: '0.05em' }}>
        {label}
      </span>
    </div>
  )
}
```

**Step 2: Commit**

```bash
git add web/src/components/Knob.tsx
git commit -m "feat: add SVG rotary knob component with drag interaction"
```

---

## Task 9: DrumMachine Canvas Component

**Files:**
- Create: `web/src/components/DrumMachine.tsx`

The main canvas component. Draws wood background, 8×5 grid, pucks, playhead, labels, play button, tempo/swing knobs using React + Canvas 2D.

**Step 1: Write `web/src/components/DrumMachine.tsx`**

```tsx
import { useCallback, useEffect, useRef, useState } from 'react'
import Knob from './Knob'
import * as engine from '../engine/wasmEngine'

const TRACKS = ['Bass', 'Snare', 'HiHat', 'Tom', 'Cymbal']
const COLS = 8
const ROWS = 5

// Canvas layout constants (logical pixels; scaled to fit container)
const CANVAS_W = 920
const CANVAS_H = 680
const GRID_X = 90          // left edge of grid
const GRID_Y = 80          // top edge of grid
const GRID_W = 620         // total grid width
const GRID_H = 480         // total grid height
const CELL_W = GRID_W / COLS
const CELL_H = GRID_H / ROWS

// Wood colors
const WOOD_LIGHT = '#D4A853'
const WOOD_MID = '#C08838'
const WOOD_DARK = '#8B5E2A'
const PUCK_COLOR = '#3D2B1F'
const PUCK_HIGHLIGHT = '#5C4030'
const GRID_LINE = '#9B6B2A'

function drawWoodBackground(ctx: CanvasRenderingContext2D) {
  const grad = ctx.createLinearGradient(0, 0, CANVAS_W, CANVAS_H)
  grad.addColorStop(0, WOOD_LIGHT)
  grad.addColorStop(0.4, WOOD_MID)
  grad.addColorStop(1, '#B07428')
  ctx.fillStyle = grad
  ctx.fillRect(0, 0, CANVAS_W, CANVAS_H)

  // Subtle wood grain lines
  ctx.strokeStyle = 'rgba(160,100,40,0.12)'
  ctx.lineWidth = 1
  for (let y = 0; y < CANVAS_H; y += 18) {
    ctx.beginPath()
    ctx.moveTo(0, y + Math.sin(y * 0.05) * 3)
    ctx.bezierCurveTo(
      CANVAS_W * 0.3, y + Math.sin(y * 0.08) * 5,
      CANVAS_W * 0.7, y + Math.sin(y * 0.06) * 4,
      CANVAS_W, y + Math.sin(y * 0.04) * 3,
    )
    ctx.stroke()
  }
}

function drawGrid(
  ctx: CanvasRenderingContext2D,
  pattern: boolean[][],
  activeStep: number,
) {
  // Grid border (rounded rect)
  const padding = 8
  ctx.strokeStyle = WOOD_DARK
  ctx.lineWidth = 2
  ctx.beginPath()
  ctx.roundRect(
    GRID_X - padding, GRID_Y - padding,
    GRID_W + padding * 2, GRID_H + padding * 2,
    12,
  )
  ctx.stroke()

  // Playhead highlight
  if (activeStep >= 0) {
    ctx.fillStyle = 'rgba(210, 160, 50, 0.25)'
    ctx.fillRect(GRID_X + activeStep * CELL_W, GRID_Y, CELL_W, GRID_H)
  }

  // Grid cells and pucks
  for (let row = 0; row < ROWS; row++) {
    for (let col = 0; col < COLS; col++) {
      const x = GRID_X + col * CELL_W
      const y = GRID_Y + row * CELL_H

      // Cell background
      ctx.strokeStyle = GRID_LINE
      ctx.lineWidth = 1
      ctx.beginPath()
      ctx.roundRect(x + 4, y + 4, CELL_W - 8, CELL_H - 8, 6)
      ctx.stroke()

      if (pattern[row][col]) {
        drawPuck(ctx, x + CELL_W / 2, y + CELL_H / 2, Math.min(CELL_W, CELL_H) * 0.38)
      }
    }
  }

  // Track labels
  ctx.fillStyle = WOOD_DARK
  ctx.font = '13px Georgia, serif'
  ctx.textAlign = 'right'
  for (let row = 0; row < ROWS; row++) {
    const y = GRID_Y + row * CELL_H + CELL_H / 2 + 5
    ctx.fillText(TRACKS[row], GRID_X - 16, y)
  }
}

function drawPuck(ctx: CanvasRenderingContext2D, cx: number, cy: number, r: number) {
  // Shadow
  ctx.shadowColor = 'rgba(0,0,0,0.4)'
  ctx.shadowBlur = 8
  ctx.shadowOffsetY = 3

  // Body
  const grad = ctx.createRadialGradient(cx - r * 0.2, cy - r * 0.2, r * 0.05, cx, cy, r)
  grad.addColorStop(0, PUCK_HIGHLIGHT)
  grad.addColorStop(1, PUCK_COLOR)
  ctx.fillStyle = grad
  ctx.beginPath()
  ctx.ellipse(cx, cy, r, r * 0.88, 0, 0, Math.PI * 2)
  ctx.fill()

  ctx.shadowColor = 'transparent'
  ctx.shadowBlur = 0
  ctx.shadowOffsetY = 0
}

function drawTitle(ctx: CanvasRenderingContext2D) {
  ctx.fillStyle = '#2A1A0A'
  ctx.font = 'bold 28px Georgia, serif'
  ctx.textAlign = 'left'
  ctx.fillText('algo-drum', 24, 48)
}

interface Props {
  wasmLoaded: boolean
}

export default function DrumMachine({ wasmLoaded }: Props) {
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const containerRef = useRef<HTMLDivElement>(null)
  const [pattern, setPattern] = useState<boolean[][]>(
    () => Array.from({ length: ROWS }, () => Array(COLS).fill(false)),
  )
  const [playing, setPlaying] = useState(false)
  const [tempo, setTempo] = useState(0.5)     // 0→60bpm, 1→200bpm
  const [swing, setSwing] = useState(0.0)
  const [volumes, setVolumes] = useState<number[]>(Array(ROWS).fill(0.75))
  const activeStepRef = useRef(-1)
  const animFrameRef = useRef<number>(0)

  // Convert normalized tempo to BPM
  const bpm = Math.round(60 + tempo * 140)

  // Canvas scale for hit-testing
  const scaleRef = useRef(1)

  // Sync engine params on change
  useEffect(() => {
    if (wasmLoaded) engine.setTempo(bpm)
  }, [bpm, wasmLoaded])

  useEffect(() => {
    if (wasmLoaded) engine.setSwing(swing * 0.5)
  }, [swing, wasmLoaded])

  useEffect(() => {
    volumes.forEach((v, i) => {
      if (wasmLoaded) engine.setVolume(i, v)
    })
  }, [volumes, wasmLoaded])

  // Draw loop
  const draw = useCallback(() => {
    const canvas = canvasRef.current
    if (!canvas) return
    const ctx = canvas.getContext('2d')
    if (!ctx) return

    if (playing) {
      activeStepRef.current = engine.currentStep()
    } else {
      activeStepRef.current = -1
    }

    drawWoodBackground(ctx)
    drawTitle(ctx)
    drawGrid(ctx, pattern, activeStepRef.current)

    animFrameRef.current = requestAnimationFrame(draw)
  }, [pattern, playing])

  useEffect(() => {
    animFrameRef.current = requestAnimationFrame(draw)
    return () => cancelAnimationFrame(animFrameRef.current)
  }, [draw])

  // Resize observer to scale canvas
  useEffect(() => {
    const container = containerRef.current
    if (!container) return
    const obs = new ResizeObserver(([entry]) => {
      const { width } = entry.contentRect
      const scale = width / CANVAS_W
      scaleRef.current = scale
    })
    obs.observe(container)
    return () => obs.disconnect()
  }, [])

  // Hit test: canvas coordinates → grid cell
  const canvasToCell = useCallback((clientX: number, clientY: number) => {
    const canvas = canvasRef.current
    if (!canvas) return null
    const rect = canvas.getBoundingClientRect()
    const scale = rect.width / CANVAS_W
    const lx = (clientX - rect.left) / scale
    const ly = (clientY - rect.top) / scale
    const col = Math.floor((lx - GRID_X) / CELL_W)
    const row = Math.floor((ly - GRID_Y) / CELL_H)
    if (col < 0 || col >= COLS || row < 0 || row >= ROWS) return null
    return { row, col }
  }, [])

  const handleCanvasClick = useCallback((e: React.MouseEvent<HTMLCanvasElement>) => {
    const cell = canvasToCell(e.clientX, e.clientY)
    if (!cell) return
    const { row, col } = cell
    const next = !pattern[row][col]
    setPattern(prev => {
      const updated = prev.map(r => [...r])
      updated[row][col] = next
      return updated
    })
    if (wasmLoaded) engine.setCell(row, col, next)
  }, [canvasToCell, pattern, wasmLoaded])

  const handlePlayStop = useCallback(async () => {
    if (!wasmLoaded) return
    if (!playing) {
      await engine.loadWasm().catch(() => {}) // no-op if already loaded
      engine.play()
      setPlaying(true)
    } else {
      engine.stop()
      setPlaying(false)
    }
  }, [playing, wasmLoaded])

  const handleVolumeChange = useCallback((track: number, v: number) => {
    setVolumes(prev => {
      const next = [...prev]
      next[track] = v
      return next
    })
  }, [])

  return (
    <div ref={containerRef} style={{ width: '100%', maxWidth: 920, position: 'relative' }}>
      <canvas
        ref={canvasRef}
        width={CANVAS_W}
        height={CANVAS_H}
        style={{ width: '100%', height: 'auto', display: 'block', borderRadius: 12, boxShadow: '0 8px 40px rgba(0,0,0,0.6)' }}
        onClick={handleCanvasClick}
      />
      {/* Overlaid controls — positioned absolutely over canvas */}
      <div style={{
        position: 'absolute',
        bottom: '8%',
        left: '8%',
        display: 'flex',
        alignItems: 'center',
        gap: 24,
      }}>
        {/* Play/Stop button */}
        <button
          onClick={handlePlayStop}
          disabled={!wasmLoaded}
          style={{
            width: 48, height: 48,
            borderRadius: '50%',
            background: playing ? '#C44' : '#2A8A6A',
            border: 'none',
            cursor: 'pointer',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            boxShadow: '0 3px 10px rgba(0,0,0,0.5)',
          }}
        >
          {playing
            ? <svg width={16} height={16} viewBox="0 0 16 16"><rect x={3} y={3} width={4} height={10} fill="white" /><rect x={9} y={3} width={4} height={10} fill="white" /></svg>
            : <svg width={16} height={16} viewBox="0 0 16 16"><polygon points="4,3 13,8 4,13" fill="white" /></svg>
          }
        </button>
        <Knob value={tempo} onChange={setTempo} label={`${bpm} BPM`} size={52} color="#2A8A6A" />
        <Knob value={swing} onChange={setSwing} label="SWING" size={52} color="#C4903A" />
      </div>

      {/* Per-track volume knobs — right side */}
      <div style={{
        position: 'absolute',
        right: '2%',
        top: '10%',
        display: 'flex',
        flexDirection: 'column',
        gap: 16,
      }}>
        {volumes.map((v, i) => (
          <Knob
            key={i}
            value={v}
            onChange={(val) => handleVolumeChange(i, val)}
            label={TRACKS[i].slice(0, 3).toUpperCase()}
            size={40}
            color="#9B6B2A"
          />
        ))}
      </div>
    </div>
  )
}
```

**Step 2: Commit**

```bash
git add web/src/components/DrumMachine.tsx
git commit -m "feat: add canvas-based drum machine UI component"
```

---

## Task 10: App.tsx and WASM Loading

**Files:**
- Create: `web/src/App.tsx`

**Step 1: Write `web/src/App.tsx`**

```tsx
import { useEffect, useState } from 'react'
import DrumMachine from './components/DrumMachine'
import { loadWasm } from './engine/wasmEngine'

export default function App() {
  const [wasmLoaded, setWasmLoaded] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    loadWasm()
      .then(() => setWasmLoaded(true))
      .catch((e: unknown) => setError(String(e)))
  }, [])

  return (
    <div style={{
      minHeight: '100vh',
      display: 'flex',
      flexDirection: 'column',
      alignItems: 'center',
      justifyContent: 'center',
      padding: 16,
      background: '#1A1008',
    }}>
      {error && (
        <div style={{ color: '#ff6b6b', marginBottom: 12, fontFamily: 'monospace' }}>
          Failed to load engine: {error}
        </div>
      )}
      {!wasmLoaded && !error && (
        <div style={{ color: '#C4A07A', marginBottom: 16, fontFamily: 'Georgia, serif' }}>
          Loading engine...
        </div>
      )}
      <DrumMachine wasmLoaded={wasmLoaded} />
    </div>
  )
}
```

**Step 2: Verify dev build starts**

```bash
cd web && bun run dev
```

Expected: Vite dev server starts. Open browser at `http://localhost:5173/algo-drum/`. The drum machine renders (WASM load will fail until wasm files are in `public/` — run `bash scripts/build-wasm.sh` first).

**Step 3: Full test: build WASM then serve**

```bash
# from repo root
bash scripts/build-wasm.sh && cd web && bun run dev
```

Expected: Browser shows the drum machine. Click Play — beats are audible.

**Step 4: Commit**

```bash
cd ..
git add web/src/App.tsx
git commit -m "feat: add App.tsx with WASM loading"
```

---

## Task 11: GitHub Actions CI/CD

**Files:**
- Create: `.github/workflows/deploy.yml`

**Step 1: Write `.github/workflows/deploy.yml`**

```yaml
name: Deploy to GitHub Pages

on:
  push:
    branches: [main]
  workflow_dispatch:

permissions:
  contents: write

jobs:
  build-and-deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: '1.25'
          cache: true

      - name: Build WASM
        run: bash scripts/build-wasm.sh

      - uses: oven-sh/setup-bun@v2
        with:
          bun-version: latest

      - name: Install frontend dependencies
        run: cd web && bun install --frozen-lockfile

      - name: Build frontend
        run: cd web && bun run build

      - name: Deploy to GitHub Pages
        uses: peaceiris/actions-gh-pages@v4
        with:
          github_token: ${{ secrets.GITHUB_TOKEN }}
          publish_dir: ./web/dist
          force_orphan: true
```

**Step 2: Commit**

```bash
git add .github/workflows/deploy.yml
git commit -m "ci: add GitHub Actions workflow for GitHub Pages deployment"
```

**Step 3: Push to GitHub**

```bash
git remote add origin https://github.com/MeKo-Tech/algo-drum.git
git push -u origin main
```

Expected: GitHub Actions workflow triggers. After ~3 minutes, site is live at `https://meko-tech.github.io/algo-drum/`.

---

## Summary

| Task | Output |
|------|--------|
| 1 | Go module with algo-dsp dependency |
| 2 | 5 drum voices (bass, snare, hihat, tom, cymbal) |
| 3 | Sequencer engine with swing |
| 4 | WASM JS API (`AlgoDrum`) |
| 5 | WASM build script |
| 6 | Vite 7 + React + TS + Bun scaffold |
| 7 | WASM bridge + AudioContext |
| 8 | SVG rotary Knob component |
| 9 | Canvas DrumMachine with wood UI |
| 10 | App.tsx wiring |
| 11 | GitHub Actions → GitHub Pages deploy |
