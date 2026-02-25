# algo-drum Design Document

**Date:** 2026-02-25
**Status:** Approved

---

## Overview

algo-drum is a teaching-focused web drum computer with a wood-finish UI inspired by physical hardware (Tembo aesthetic). It runs entirely in the browser: a Go WebAssembly engine handles sequencing and synthesis, while a React/TypeScript frontend renders the interface on a Canvas element.

---

## Goals

- Teach drum machine concepts (step sequencing, swing, synthesis) via an approachable visual interface
- Demonstrate Go → WASM compilation using real DSP primitives from `github.com/cwbudde/algo-dsp`
- Deploy statically to GitHub Pages with no backend

---

## Architecture

```
┌─────────────────────────────────────────────┐
│  Browser                                    │
│                                             │
│  ┌─────────────┐     ┌──────────────────┐  │
│  │ React UI    │────▶│ wasmEngine.ts    │  │
│  │ (Canvas)    │◀────│ (WASM bridge)    │  │
│  └─────────────┘     └────────┬─────────┘  │
│                               │            │
│                    ┌──────────▼─────────┐  │
│                    │ AudioWorklet /     │  │
│                    │ ScriptProcessor    │  │
│                    └──────────┬─────────┘  │
│                               │            │
│                    ┌──────────▼─────────┐  │
│                    │ algo_drum.wasm     │  │
│                    │ (Go, 808-style syn)│  │
│                    └────────────────────┘  │
└─────────────────────────────────────────────┘
```

**Two main parts:**

1. **Go module** — the `algo-drum` Go module imports `github.com/cwbudde/algo-dsp` as a library. Compiled to `algo_drum.wasm` via `GOOS=js GOARCH=wasm`. Owns all synthesis and sequencing logic.

2. **React frontend** — Vite 7 + React + TypeScript, built with Bun. Renders the UI on a `<canvas>` element. Bridges to the WASM engine via the Web Audio API.

---

## Project Structure

```
algo-drum/
├── cmd/wasm/main.go          # WASM entry point — exports AlgoDrum JS API
├── internal/drum/
│   ├── engine.go             # Sequencer, mixer, transport
│   └── voices.go             # 5 drum voice synthesizers (808-style)
├── go.mod                    # module algo-drum; requires github.com/cwbudde/algo-dsp
├── go.sum
├── scripts/
│   └── build-wasm.sh         # copies wasm_exec.js, runs go build
├── web/                      # Vite 7 + React + TS + Bun
│   ├── src/
│   │   ├── main.tsx
│   │   ├── App.tsx
│   │   ├── components/
│   │   │   ├── DrumMachine.tsx   # top-level canvas component
│   │   │   └── Knob.tsx          # SVG rotary knob
│   │   └── engine/
│   │       └── wasmEngine.ts     # loads WASM, manages AudioContext
│   ├── public/
│   │   ├── algo_drum.wasm        # built by build-wasm.sh
│   │   └── wasm_exec.js          # copied from GOROOT
│   ├── index.html
│   ├── vite.config.ts
│   ├── tsconfig.json
│   └── package.json
└── .github/workflows/
    └── deploy.yml            # CI: build WASM → build Vite → deploy gh-pages
```

---

## WASM JS API

Exposed on `window.AlgoDrum`:

| Method | Description |
|--------|-------------|
| `init(sampleRate)` | Initialize engine at given sample rate |
| `setRunning(bool)` | Start / stop sequencer |
| `setTempo(bpm)` | Set BPM (range 60–200) |
| `setSwing(amount)` | Set swing 0.0–1.0 |
| `setCell(track, step, active)` | Toggle a grid cell |
| `render(n)` | Render n mono samples → Float32Array |
| `currentStep()` | Returns active step index (-1 if stopped) |

---

## Drum Synthesis (808-style analog modelling)

All synthesis uses algo-dsp primitives (oscillators, biquad filters, envelopes).

| Track | Voice | Technique |
|-------|-------|-----------|
| 0 — Bass | Sine osc + pitch envelope (200→50 Hz over 60ms) + amplitude envelope (fast attack, ~400ms decay) | Classic kick |
| 1 — Snare | Tone oscillator (200 Hz) + white noise generator, both through amplitude envelope; noise through highpass | 808 snare |
| 2 — Hi-Hat | 6 detuned square oscillators mixed, through bandpass (8–12 kHz), short decay (40ms closed, 300ms open) | Metallic hat |
| 3 — Tom | Sine + pitch envelope (120→60 Hz), medium decay (300ms) | Floor tom |
| 4 — Cymbal | White noise + bandpass cluster, long decay (1.2s) with shimmer LFO on filter freq | Ride/crash |

---

## UI Design

- **Canvas** occupies the full viewport with a warm wood-grain background (`#D4A853` gradient placeholder until real texture provided)
- **8×5 grid** — 8 steps (columns) × 5 tracks (rows); cells drawn with rounded rectangles
- **Pucks** — dark brown ellipses with a subtle highlight bevel, placed on active cells
- **Playhead** — active column highlighted in warm gold
- **Bottom bar** — Play/Stop button (teal circle with triangle icon), Tempo knob, Swing knob; all Canvas-drawn
- **Right panel** — 5 per-track volume knobs (one per row), labeled with track name
- **Typography** — "algo-drum" top-left in a warm serif/rounded font

Knobs are drawn as SVG components with mouse drag interaction (vertical drag = value change).

---

## Deployment (GitHub Pages)

GitHub Actions workflow on push to `main`:

1. `scripts/build-wasm.sh` — compiles Go to WASM, outputs to `web/public/`
2. `cd web && bun install && bun run build` — outputs static site to `web/dist/`
3. Deploy `web/dist/` to `gh-pages` branch via `peaceiris/actions-gh-pages`

Vite config sets `base` to the repo name for correct asset paths on GitHub Pages.

---

## Constraints & Scope

- **In scope:** 8×5 grid, 5 synthesis voices, tempo, swing, per-track volume, GitHub Pages deploy
- **Out of scope (v1):** pattern save/load, multiple patterns, per-step velocity, effects chain, mobile touch
- Wood texture image will be swapped in by user; placeholder gradient used until then
