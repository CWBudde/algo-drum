# algo-drum

An algorithmic drum machine running entirely in your browser.
[**Try it live →**](https://cwbudde.github.io/algo-drum/)

Built with a **Go audio engine** compiled to WebAssembly and a React UI.
No plugins, no backend — just a `.wasm` file and a browser.

## Features

- 7 voices × up to 16 steps: Bass Drum, Snare, Hi-Hat, two independently
  tunable Toms, Cymbal, and metallic Percussion, with a runtime-adjustable
  pattern length (STEPS knob, 1–16)
- Per-step velocity: click a cell to cycle off → hit → accent
- Per-track volume (smoothed, zipper-free) and decay knobs, plus per-track mute
- Tempo (BPM) and swing, with tap tempo
- Trigger probability and humanize (timing jitter) knobs for less mechanical
  patterns
- Pattern mutate (random-walk the current pattern) and 6 built-in presets
- Global reverb control
- Shareable patterns: state round-trips through a URL hash and
  `localStorage`, so reloading or sending a link restores the pattern, tempo,
  and knobs
- Keyboard-accessible: Space toggles play/stop, every grid cell and button is
  focusable and activates with Enter/Space, and the knobs are ARIA sliders that
  respond to arrow keys, Page Up/Down, Home/End, and Escape to reset
- Installable as a PWA: a web app manifest plus a service worker that caches
  the app shell, the WASM engine, and the audio worklet (a fully offline reload
  isn't guaranteed yet — the hashed JS/CSS bundle is not precached)
- Runs entirely client-side

## How it works

The audio engine is written in Go and compiled to WebAssembly. It runs inside a
Web Worker, so rendering never blocks the UI; an `AudioWorklet` pulls rendered
chunks from the worker over a direct `MessageChannel` and reports the audible
sequencer step back to the UI playhead.

```
Go engine (WASM, in a Web Worker)  ──512-sample chunks──►  AudioWorklet  ──►  AudioContext  ──►  speakers
     ▲                                                          │
     │  setCell / setTempo / setSwing / setVolume / setReverb   │ audible step
     │                                                          ▼
React UI (TypeScript) ◄─────────────────────────────────── playhead
```

The synthesizer voices are purely procedural — no samples. Each voice uses an
exponential amplitude envelope; tonal voices (Bass Drum and both Toms) add
pitch sweep, the Snare, Hi-Hat, and Cymbal use filtered noise, and Percussion
combines inharmonic oscillators with a short noise transient. The mix passes
through a global FDN reverb and brick-wall limiter before reaching the browser.
See [`docs/voices.md`](docs/voices.md) for the full per-voice synthesis recipe
and parameter reference.

### Experimental physical model

An independent, work-in-progress physical path contains a double-headed,
cavity-coupled modal tom. It uses circular Fourier–Bessel modes, a passive
damped/nonlinear state update, measured-range velocity-dependent stick
contact, frequency-dependent loss, mode-dependent radiation, and a
batter-side filtered microphone response. In the web demo, open either Tom
voice’s settings and select **Physical — Experimental** to A/B it against that
track’s algorithmic model. The two Toms keep independent model choices and
physical parameter banks. Algorithmic remains the default, and older saved
patterns and share links continue to select it.

Render the default model to a normalized mono PCM WAV file for auditioning:

```bash
go run ./cmd/render-physical -o renders/physical-drum.wav
```

Use `-duration`, `-velocity`, `-strike-radius`, and `-hardness` to compare the
prototype's response. WAV encoding uses the maintained
[`CWBudde/wav`](https://github.com/CWBudde/wav) fork. The research basis and
staged implementation plan are in
[`docs/physical-model-research.md`](docs/physical-model-research.md) and
[`PLAN.md`](PLAN.md).

Generate modal targets, decay estimates, spectra, spectral peaks, and a
fundamental-frequency track as JSON:

```bash
go run ./cmd/analyze-physical
just gen-physical-reference # refresh the committed multi-condition suite
```

The equations, calibration workflow, and provenance of the synthetic reference
set are documented in
[`docs/physical-calibration.md`](docs/physical-calibration.md).

## Browser requirements

algo-drum needs a browser with:

- **WebAssembly** support (to run the Go audio engine)
- **Web Audio API** with **AudioWorklet** support (the engine renders audio
  in a Web Worker; an AudioWorklet consumes the rendered chunks on the audio
  thread)
- A **user gesture** (e.g. pressing Play) to start the `AudioContext` — this
  is a standard browser autoplay restriction, not an algo-drum limitation

All current major desktop and mobile browsers (Chrome, Firefox, Safari, Edge)
satisfy these requirements.

## Building locally

**Prerequisites:** Go 1.25+, [Bun](https://bun.sh/)

```bash
# 1. Build the WASM binary (outputs to web/public/)
bash scripts/build-wasm.sh

# 2. Start the dev server
cd web && bun install && bun run dev
```

The dev server serves the app at `http://localhost:5173/algo-drum/` — Vite's `base` is `/algo-drum/` to match the GitHub Pages path, and opening `http://localhost:5173/` redirects there. WASM must be built before starting the frontend — the dev server serves `web/public/` as static assets.

```bash
# Production build → web/dist/
cd web && bun run build
```

## Deployment

GitHub Actions builds WASM + frontend on every push to `main` and deploys `web/dist/` to GitHub Pages automatically.
In repository settings, GitHub Pages should be configured with **Source: GitHub Actions**.
