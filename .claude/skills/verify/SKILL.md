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
- The UI is DOM/CSS (no canvas). Grid cells are real buttons:
  `getByRole("button", { name: "Bass step 1" })` etc. (tracks: Cymbal, Tom,
  HiHat, Snare, Bass; steps 1–8). Active cells have `aria-pressed="true"`;
  the playhead column carries `data-playhead` (5 cells while playing).
- Play/Stop: `getByRole("button", { name: "Play" })` (aria-label toggles to
  "Stop"), or press Space with focus on the body. Knobs are
  `role="slider"` with `aria-valuetext` (arrow keys adjust, Escape resets).
- Sequencer motion: step messages arrive every 250 ms at 120 BPM; −1 when
  stopped.
- Production-build check matters here: worker bundling differs dev vs prod —
  also drive `bunx vite build && bunx vite preview --port 4173` at
  `http://localhost:4173/algo-drum/`.

## Gotchas

- Vite `base` is `/algo-drum/` — the dev URL includes that path.
- Mouse-clicking a cell blurs it (so Space stays free for the transport);
  keyboard activation keeps focus. Knobs still drag vertically with the mouse.
- Known issue (PLAN.md E7): master peaks can exceed ±1.0 — don't treat
  peak > 1 as a harness bug.
