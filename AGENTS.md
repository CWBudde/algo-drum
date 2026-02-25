# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

algo-drum is an algorithmic drum machine built with a **Go WASM audio engine** and a **React/TypeScript frontend**. The Go code compiles to WebAssembly, exposes a global `window.AlgoDrum` API, and React renders the UI while feeding audio through the Web Audio API.

## Build Commands

### WASM Engine (Go)

```bash
# Build the WASM binary + copy wasm_exec.js to web/public/
bash scripts/build-wasm.sh

# Verify the WASM build compiles
GOOS=js GOARCH=wasm go build ./cmd/wasm/
```

### Frontend (React/TypeScript)

```bash
cd web

bun install          # Install dependencies
bun run dev          # Dev server (Vite) — requires WASM built first
bun run build        # Type-check + production build → web/dist/
bun run preview      # Preview the production build
```

### Full Development Workflow

```bash
# Step 1: build WASM (output goes to web/public/)
bash scripts/build-wasm.sh

# Step 2: start frontend dev server
cd web && bun run dev
```

Vite serves `web/public/` as static assets, so `algo_drum.wasm` and `wasm_exec.js` are available at runtime.

## Architecture

```
cmd/wasm/main.go          — WASM entry point; registers window.AlgoDrum JS API
internal/drum/engine.go   — Sequencer: pattern grid, tempo/swing, per-track volumes, Render()
internal/drum/voices.go   — Drum synthesizer voices (BassDrum, Snare, HiHat, Tom, Cymbal)
web/src/engine/wasmEngine.ts — TypeScript bridge: loads WASM, wraps AlgoDrum calls, manages AudioContext
web/src/components/DrumMachine.tsx — Main UI: 5×8 step grid, playback controls, per-track volume knobs
web/src/components/Knob.tsx        — Reusable rotary knob (SVG, vertical drag)
web/src/App.tsx           — Root: loads WASM on mount, renders DrumMachine
```

### Audio Signal Flow

`Engine.Render(buf)` → Go voices mix mono samples → soft-clip → `Float32Array` returned to JS → `ScriptProcessorNode` feeds `AudioContext` at 48 kHz, buffer size 4096.

### Track Order (index 0–4)

| Index | Voice           |
| ----- | --------------- |
| 0     | Bass Drum       |
| 1     | Snare           |
| 2     | Hi-Hat (closed) |
| 3     | Tom             |
| 4     | Cymbal          |

UI displays tracks in **reverse order** (Cymbal on top, Bass on bottom).

### WASM JS API (`window.AlgoDrum`)

| Method                       | Description                               |
| ---------------------------- | ----------------------------------------- |
| `init(sampleRate)`           | Initialize engine                         |
| `setRunning(bool)`           | Play / stop (stop resets to step 0)       |
| `setTempo(bpm)`              | Set tempo (BPM)                           |
| `setSwing(0–0.5)`            | Set swing amount                          |
| `setCell(track, step, bool)` | Toggle grid cell                          |
| `setVolume(track, 0–1)`      | Set track volume                          |
| `render(n)`                  | Render n samples → Float32Array           |
| `currentStep()`              | Returns active step index (-1 if stopped) |

## Key Dependencies

- **[algo-dsp](https://github.com/cwbudde/algo-dsp) v0.2.0** — DSP library used for biquad filters in voices (`biquad.Section`, `design.Highpass`, `design.Bandpass`)
- **bun** — package manager and script runner for the frontend
- **Vite 7** — frontend bundler; configured with `@vitejs/plugin-react`

## Deployment

GitHub Actions (`.github/workflows/deploy.yml`) builds WASM + frontend and deploys `web/dist/` to GitHub Pages on every push to `main`. The Vite build must use correct `base` URL for asset paths in production.
