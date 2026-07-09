---
name: verify
description: Build, launch, and drive algo-drum end-to-end (Go WASM + Vite + headless Chromium) to verify changes at the running app.
---

# Verifying algo-drum

## Build & launch

```bash
bash scripts/build-wasm.sh                 # WASM → web/public/ (required first)
cd web && bun install && bun run dev --port 5173 --strictPort   # app at http://localhost:5173/algo-drum/
```

## Drive headless

Playwright works with the pre-installed browser, but the npm package's pinned
revision may not match — pass `executablePath: "/opt/pw-browsers/chromium"` to
`chromium.launch()`, plus `--autoplay-policy=no-user-gesture-required` so the
AudioContext starts in headless.

Useful hooks (post AudioWorklet migration — the WASM engine runs inside a
Web Worker; there is NO `window.AlgoDrum` on the main thread):

- Wait for readiness: the "Loading engine" text disappears once the worker
  reports the engine ready (regression check for the
  pattern-lost-before-play bug: program cells before Play, then expect audio).
- Capture the audio node by wrapping the `AudioWorkletNode` constructor in an
  init script (stash `this` and the ctx on `window`); connect an
  `AnalyserNode` to it for output peaks. The worklet also posts
  `{type:"step", step}` on its port as each chunk becomes audible — add a
  `port.addEventListener("message", ...)` in the wrapper to observe the
  playhead (works alongside the app's `onmessage`).
- Grid cells are canvas-drawn; click coordinates from constants in
  `DrumMachine.tsx`: CW=1020, CH=700, GRID_X=120, GRID_Y=80, GRID_W=656,
  GRID_H=440, 8 cols × 5 rows. Cell center:
  `x = box.x + (GRID_X + col*CELL_W + CELL_W/2)/CW * box.width` (same for y).
  Visual rows top→bottom: Cymbal, Tom, HiHat, Snare, Bass.
- Play/Stop button: `button[title="Play"]` / `button[title="Stop"]`.
- Sequencer motion: step messages arrive every 250 ms at 120 BPM; −1 when
  stopped.
- Production-build check matters here: worker bundling differs dev vs prod —
  also drive `bunx vite build && bunx vite preview --port 4173` at
  `http://localhost:4173/algo-drum/`.

## Gotchas

- Vite `base` is `/algo-drum/` — the dev URL includes that path.
- The knob BPM label (`/\d+ BPM/` text) sits next to its SVG; drag the SVG
  vertically with mouse down/move/up to change values.
- Known issue (PLAN.md E7): master peaks can exceed ±1.0 — don't treat
  peak > 1 as a harness bug.
