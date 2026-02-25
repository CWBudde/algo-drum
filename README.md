# algo-drum

An algorithmic drum machine running entirely in your browser.
[**Try it live →**](https://meko-tech.github.io/algo-drum/)

Built with a **Go audio engine** compiled to WebAssembly and a React UI.
No plugins, no backend — just a `.wasm` file and a browser.

## Features

- 5 voices × 8 steps: Bass Drum, Snare, Hi-Hat, Tom, Cymbal
- Adjustable tempo (BPM) and swing
- Per-track volume knobs
- Global reverb control
- Runs entirely client-side

## How it works

The audio engine is written in Go and compiled to WebAssembly. It exposes a `window.AlgoDrum` API that the React frontend calls to toggle steps, update parameters, and pull rendered audio samples on each buffer callback.

```
Go engine (WASM)  ──render()──►  ScriptProcessorNode  ──►  AudioContext  ──►  speakers
     ▲
     │  setCell / setTempo / setSwing / setVolume / setReverb
     │
React UI (TypeScript)
```

The synthesizer voices are purely procedural — no samples. Each voice uses an exponential amplitude envelope; tonal voices (Bass Drum, Tom) add pitch sweep, and noise voices (Snare, Hi-Hat, Cymbal) pass filtered white noise through biquad filters from the [algo-dsp](https://github.com/cwbudde/algo-dsp) library. The mix passes through a global FDN reverb and brick-wall limiter before reaching the browser.

## Building locally

**Prerequisites:** Go 1.25+, [Bun](https://bun.sh/)

```bash
# 1. Build the WASM binary (outputs to web/public/)
bash scripts/build-wasm.sh

# 2. Start the dev server
cd web && bun install && bun run dev
```

The dev server runs at `http://localhost:5173`. WASM must be built before starting the frontend — the dev server serves `web/public/` as static assets.

```bash
# Production build → web/dist/
cd web && bun run build
```

## Deployment

GitHub Actions builds WASM + frontend on every push to `main` and deploys `web/dist/` to GitHub Pages automatically.
