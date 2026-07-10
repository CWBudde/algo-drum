import { useCallback, useEffect, useState } from "react";
import Knob from "./Knob";
import * as engine from "../engine/wasmEngine";
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

interface Props {
  wasmLoaded: boolean;
}

export default function DrumMachine({ wasmLoaded }: Props) {
  const [pattern, setPattern] = useState<number[][]>(() =>
    Array.from({ length: ROWS }, () => Array<number>(COLS).fill(0)),
  );
  const [playing, setPlaying] = useState(false);
  const [tempo, setTempoState] = useState(0.43); // ~120 BPM
  const [swing, setSwingState] = useState(0.0);
  const [steps, setStepsState] = useState(1.0); // knob position; 1.0 = 16 steps
  const [reverb, setReverbState] = useState(0.0);
  const [volumes, setVolumes] = useState(() => Array<number>(ROWS).fill(0.75));
  const [decays, setDecays] = useState(() => Array<number>(ROWS).fill(0.5));
  const [muted, setMuted] = useState(() => Array<boolean>(ROWS).fill(false));
  const [currentStep, setCurrentStep] = useState(-1);

  const bpm = Math.round(60 + tempo * 140);
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
    volumes.forEach((v, i) => {
      engine.setVolume(TRACK_INDEX[i], muted[i] ? 0 : v);
    });
  }, [volumes, muted]);
  useEffect(() => {
    decays.forEach((d, i) => {
      engine.setDecay(TRACK_INDEX[i], d);
    });
  }, [decays]);

  // Playhead follows the audible step reported by the audio worklet
  useEffect(() => engine.onStep(setCurrentStep), []);

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
