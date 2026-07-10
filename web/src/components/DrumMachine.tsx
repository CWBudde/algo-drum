import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import Knob from "./Knob";
import AlgoPanel from "./AlgoPanel";
import * as engine from "../engine/wasmEngine";
import {
  loadInitialState,
  saveLocal,
  shareUrl,
  type PersistedState,
} from "../algo/persistence";
import "./DrumMachine.css";

// Visual order: Cymbal on top, Bass on bottom
const TRACKS = ["Cymbal", "Tom", "HiHat", "Snare", "Bass"];
// Maps visual row index → engine track index (engine: 0=Bass,1=Snare,2=HiHat,3=Tom,4=Cymbal)
const TRACK_INDEX = [4, 3, 2, 1, 0];
const COLS = 16;
const ROWS = 5;

// Clicking a cell cycles off → normal hit → accent → off.
const VEL_NORMAL = 0.7;
const VEL_ACCENT = 1.0;

const AMBER = "#C87828";
const BLUE = "#6D95C8";

function cycleVelocity(velocity: number): number {
  if (velocity === 0) return VEL_NORMAL;
  if (velocity < VEL_ACCENT) return VEL_ACCENT;
  return 0;
}

function velocityName(velocity: number): string {
  if (velocity === 0) return "off";
  return velocity < VEL_ACCENT ? "on" : "accent";
}

// visualToFlat / flatToVisual bridge the UI's reverse-ordered visual grid and
// the engine-major flat pattern (index = engineTrack·COLS + step) used by the
// bulk pattern API and every algo module.
function visualToFlat(visual: number[][]): number[] {
  const flat = new Array<number>(ROWS * COLS).fill(0);
  for (let row = 0; row < ROWS; row++) {
    for (let col = 0; col < COLS; col++) {
      flat[TRACK_INDEX[row] * COLS + col] = visual[row][col];
    }
  }
  return flat;
}

function flatToVisual(flat: number[]): number[][] {
  return TRACKS.map((_, row) =>
    Array.from(
      { length: COLS },
      (_, col) => flat[TRACK_INDEX[row] * COLS + col] ?? 0,
    ),
  );
}

// snapVelocity undoes float32 rounding on velocities echoed back from the
// engine (0.7 stored as float32 reads back as 0.699999988…) so the mirror
// stays strictly equal to what the UI wrote.
function snapVelocity(velocity: number): number {
  return Math.round(velocity * 1000) / 1000;
}

function visualPatternsEqual(a: number[][], b: number[][]): boolean {
  return a.every((row, r) => row.every((vel, c) => vel === b[r][c]));
}

// Tap tempo maps between BPM and the tempo knob position (see bpm below).
const BPM_MIN = 60;
const BPM_MAX = 200;
const BPM_RANGE = 140; // BPM_MAX - BPM_MIN
const TAP_RESET_MS = 2000;
const TAP_WINDOW = 4;

interface Props {
  wasmLoaded: boolean;
}

export default function DrumMachine({ wasmLoaded }: Props) {
  // Restore saved/shared state once on mount; a valid URL hash wins over
  // localStorage (see loadInitialState).
  const initial = useMemo<PersistedState | null>(() => loadInitialState(), []);

  const [pattern, setPattern] = useState<number[][]>(() =>
    initial
      ? flatToVisual(initial.pattern)
      : Array.from({ length: ROWS }, () => Array<number>(COLS).fill(0)),
  );
  const [playing, setPlaying] = useState(false);
  const [tempo, setTempoState] = useState(initial?.tempo ?? 0.43); // ~120 BPM
  const [swing, setSwingState] = useState(initial?.swing ?? 0.0);
  const [steps, setStepsState] = useState(initial?.steps ?? 1.0); // 1.0 = 16 steps
  const [reverb, setReverbState] = useState(initial?.reverb ?? 0.0);
  const [prob, setProbState] = useState(initial?.prob ?? 1.0);
  const [humanize, setHumanizeState] = useState(initial?.humanize ?? 0.0);
  const [volumes, setVolumes] = useState<number[]>(
    () => initial?.volumes ?? Array<number>(ROWS).fill(0.75),
  );
  const [decays, setDecays] = useState<number[]>(
    () => initial?.decays ?? Array<number>(ROWS).fill(0.5),
  );
  const [muted, setMuted] = useState<boolean[]>(
    () => initial?.muted ?? Array<boolean>(ROWS).fill(false),
  );
  const [currentStep, setCurrentStep] = useState(-1);

  const bpm = Math.round(BPM_MIN + tempo * BPM_RANGE);
  const stepCount = Math.round(1 + steps * (COLS - 1));

  // Push parameters to the engine (queued by the bridge until it's ready)
  useEffect(() => {
    engine.setTempo(bpm);
  }, [bpm]);
  useEffect(() => {
    engine.setSwing(swing * 0.5);
  }, [swing]);
  useEffect(() => {
    engine.setStepCount(stepCount);
  }, [stepCount]);
  useEffect(() => {
    engine.setReverb(reverb);
  }, [reverb]);
  useEffect(() => {
    engine.setProbability(prob);
  }, [prob]);
  useEffect(() => {
    engine.setHumanize(humanize);
  }, [humanize]);
  useEffect(() => {
    volumes.forEach((v, i) => {
      engine.setVolume(TRACK_INDEX[i], muted[i] ? 0 : v);
    });
  }, [volumes, muted]);
  useEffect(() => {
    decays.forEach((d, i) => {
      engine.setDecay(TRACK_INDEX[i], d);
    });
  }, [decays]);

  // Push the (possibly restored) initial pattern to the engine once — per-cell
  // edits sync in cycleCell, but a bulk restore needs one setPattern.
  useEffect(() => {
    engine.setPattern(Float32Array.from(visualToFlat(pattern)));
    // Mount-only: intentionally seeds the engine with the initial pattern.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Playhead follows the audible step reported by the audio worklet
  useEffect(() => engine.onStep(setCurrentStep), []);

  // The engine owns the pattern: edits above apply optimistically for instant
  // feedback, and the authoritative copy the engine echoes back after each
  // edit replaces the mirror (once no newer edits are in flight). Bail out on
  // equality so confirming echoes don't re-render the grid.
  useEffect(
    () =>
      engine.onPattern((flat) => {
        setPattern((prev) => {
          const next = flatToVisual(Array.from(flat, snapVelocity));
          return visualPatternsEqual(prev, next) ? prev : next;
        });
      }),
    [],
  );

  // Snapshot the full serializable UI state for persistence + share links.
  const buildState = useCallback(
    (): PersistedState => ({
      pattern: visualToFlat(pattern),
      steps,
      tempo,
      swing,
      reverb,
      prob,
      humanize,
      volumes,
      decays,
      muted,
    }),
    [
      pattern,
      steps,
      tempo,
      swing,
      reverb,
      prob,
      humanize,
      volumes,
      decays,
      muted,
    ],
  );

  // Auto-save to localStorage, debounced so a knob sweep writes once it settles.
  useEffect(() => {
    const id = window.setTimeout(() => saveLocal(buildState()), 300);
    return () => window.clearTimeout(id);
  }, [buildState]);

  const getShareUrl = useCallback(() => shareUrl(buildState()), [buildState]);

  const flatPattern = useMemo(() => visualToFlat(pattern), [pattern]);

  // applyFlatPattern replaces the whole pattern (presets, clear, mutate,
  // Euclid) in both the UI and the engine.
  const applyFlatPattern = useCallback((flat: number[]) => {
    setPattern(flatToVisual(flat));
    engine.setPattern(Float32Array.from(flat));
  }, []);

  // Tap tempo: average the intervals of the last few taps, reset after a gap.
  const tapTimes = useRef<number[]>([]);
  const handleTap = useCallback(() => {
    const now = performance.now();
    const times = tapTimes.current;
    if (times.length > 0 && now - times[times.length - 1] > TAP_RESET_MS) {
      times.length = 0;
    }
    times.push(now);
    if (times.length > TAP_WINDOW) times.splice(0, times.length - TAP_WINDOW);

    if (times.length >= 2) {
      let sum = 0;
      for (let i = 1; i < times.length; i++) sum += times[i] - times[i - 1];
      const avgMs = sum / (times.length - 1);
      const clampedBpm = Math.max(BPM_MIN, Math.min(BPM_MAX, 60000 / avgMs));
      setTempoState((clampedBpm - BPM_MIN) / BPM_RANGE);
    }
  }, []);

  // Mouse clicks blur the button so Space stays free for play/stop;
  // keyboard activation (detail === 0) keeps focus for grid navigation.
  const blurOnMouseClick = (e: React.MouseEvent<HTMLButtonElement>) => {
    if (e.detail > 0) e.currentTarget.blur();
  };

  const cycleCell = useCallback((row: number, col: number) => {
    setPattern((prev) => {
      const next = prev.map((cells) => [...cells]);
      next[row][col] = cycleVelocity(next[row][col]);
      engine.setCell(TRACK_INDEX[row], col, next[row][col]);
      return next;
    });
  }, []);

  const handlePlayStop = useCallback(async () => {
    if (!wasmLoaded) return;
    if (!playing) {
      await engine.play();
      setPlaying(true);
    } else {
      engine.stop();
      setPlaying(false);
    }
  }, [playing, wasmLoaded]);

  // Space toggles play/stop unless a control is focused
  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.code !== "Space") return;
      const target = e.target as HTMLElement;
      if (target.closest("button, [role='slider'], input, textarea")) return;
      e.preventDefault();
      void handlePlayStop();
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [handlePlayStop]);

  const setTrackValue = (
    setter: React.Dispatch<React.SetStateAction<number[]>>,
    track: number,
    value: number,
  ) => {
    setter((prev) => {
      const next = [...prev];
      next[track] = value;
      return next;
    });
  };

  const toggleMute = (track: number) => {
    setMuted((prev) => {
      const next = [...prev];
      next[track] = !next[track];
      return next;
    });
  };

  return (
    <div className="dm-machine">
      <header className="dm-header">
        <h1 className="dm-title">
          <span className="dm-title-algo">algo</span>
          <span className="dm-title-drum">-drum</span>
        </h1>
        <span
          className={`dm-status ${wasmLoaded ? "dm-status-ready" : ""}`}
          title={wasmLoaded ? "Engine ready" : "Loading engine"}
          aria-hidden="true"
        />
      </header>

      <div className="dm-board">
        {/* Recessed screen behind the step grid */}
        <div className="dm-screen" aria-hidden="true" />

        {TRACKS.map((name, row) => (
          <span
            key={name}
            className="dm-track-label"
            style={{ gridRow: row + 1, gridColumn: 1 }}
            id={`dm-track-${name}`}
          >
            {name.toUpperCase()}
          </span>
        ))}

        {TRACKS.map((name, row) =>
          Array.from({ length: COLS }, (_, col) => (
            <button
              key={`${name}-${col}`}
              type="button"
              className="dm-cell"
              style={{ gridRow: row + 1, gridColumn: col + 2 }}
              data-active={pattern[row][col] > 0 || undefined}
              data-accent={pattern[row][col] >= VEL_ACCENT || undefined}
              data-beyond={col >= stepCount || undefined}
              data-playhead={col === currentStep || undefined}
              data-bar-start={col % 4 === 0 || undefined}
              data-bar-odd={Math.floor(col / 4) % 2 === 1 || undefined}
              aria-pressed={pattern[row][col] > 0}
              aria-label={`${name} step ${col + 1}: ${velocityName(pattern[row][col])}`}
              onClick={(e) => {
                blurOnMouseClick(e);
                cycleCell(row, col);
              }}
            >
              <span className="dm-led" aria-hidden="true" />
            </button>
          )),
        )}

        {Array.from({ length: COLS }, (_, col) => (
          <span
            key={col}
            className="dm-step-number"
            style={{ gridRow: ROWS + 1, gridColumn: col + 2 }}
            data-playhead={col === currentStep || undefined}
            data-beyond={col >= stepCount || undefined}
            aria-hidden="true"
          >
            {col + 1}
          </span>
        ))}

        {TRACKS.map((name, row) => (
          <div
            key={name}
            className="dm-track-controls"
            style={{ gridRow: row + 1, gridColumn: COLS + 3 }}
          >
            <button
              type="button"
              className="dm-mute"
              aria-pressed={muted[row]}
              aria-label={`Mute ${name}`}
              title={muted[row] ? `Unmute ${name}` : `Mute ${name}`}
              onClick={(e) => {
                blurOnMouseClick(e);
                toggleMute(row);
              }}
            >
              <span className="dm-mute-led" aria-hidden="true" />
            </button>
            <Knob
              value={volumes[row]}
              onChange={(v) => setTrackValue(setVolumes, row, v)}
              label={name.slice(0, 3).toUpperCase()}
              ariaLabel={`${name} volume`}
              defaultValue={0.75}
              size={42}
              color={AMBER}
            />
            <Knob
              value={decays[row]}
              onChange={(v) => setTrackValue(setDecays, row, v)}
              label="DEC"
              ariaLabel={`${name} decay`}
              defaultValue={0.5}
              size={42}
              color={BLUE}
            />
          </div>
        ))}
      </div>

      <AlgoPanel
        disabled={!wasmLoaded}
        pattern={flatPattern}
        stepCount={stepCount}
        onApplyPattern={applyFlatPattern}
        getShareUrl={getShareUrl}
      />

      <footer className="dm-transport">
        <button
          type="button"
          className={`dm-play ${playing ? "dm-play-active" : ""}`}
          onClick={() => void handlePlayStop()}
          disabled={!wasmLoaded}
          aria-label={playing ? "Stop" : "Play"}
          title={`${playing ? "Stop" : "Play"} (Space)`}
        >
          {playing ? (
            <svg width={16} height={16} viewBox="0 0 18 18" aria-hidden="true">
              <rect x={3} y={3} width={4} height={12} fill="white" rx={1} />
              <rect x={11} y={3} width={4} height={12} fill="white" rx={1} />
            </svg>
          ) : (
            <svg width={16} height={16} viewBox="0 0 18 18" aria-hidden="true">
              <polygon points="5,3 15,9 5,15" fill="white" />
            </svg>
          )}
        </button>

        <div className="dm-tempo-group">
          <Knob
            value={tempo}
            onChange={setTempoState}
            label={`${bpm} BPM`}
            ariaLabel="Tempo"
            valueText={() => `${bpm} BPM`}
            defaultValue={0.43}
            size={54}
            color={AMBER}
          />
          <button
            type="button"
            className="dm-tap"
            onClick={handleTap}
            disabled={!wasmLoaded}
            aria-label="Tap tempo"
            title="Tap to set tempo"
          >
            TAP
          </button>
        </div>
        <Knob
          value={swing}
          onChange={setSwingState}
          label="SWING"
          defaultValue={0}
          size={54}
          color={AMBER}
        />
        <Knob
          value={steps}
          onChange={setStepsState}
          label="STEPS"
          ariaLabel="Pattern length"
          valueText={() => `${stepCount} steps`}
          defaultValue={1}
          size={54}
          color={AMBER}
        />
        <Knob
          value={prob}
          onChange={setProbState}
          label="PROB"
          ariaLabel="Trigger probability"
          defaultValue={1}
          size={54}
          color={AMBER}
        />
        <Knob
          value={humanize}
          onChange={setHumanizeState}
          label="HUMAN"
          ariaLabel="Humanize"
          defaultValue={0}
          size={54}
          color={AMBER}
        />

        <div className="dm-transport-spacer" />

        <Knob
          value={reverb}
          onChange={setReverbState}
          label="REVERB"
          defaultValue={0}
          size={54}
          color={AMBER}
        />
      </footer>
    </div>
  );
}
