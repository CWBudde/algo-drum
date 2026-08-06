import {
  useCallback,
  useEffect,
  useMemo,
  useReducer,
  useRef,
  useState,
} from "react";
import Knob from "./Knob";
import AlgoPanel from "./AlgoPanel";
import VoiceEditor from "./VoiceEditor";
import * as engine from "../engine/wasmEngine";
import {
  defaultPhysicalTomParams,
  PHYSICAL_TOM_PARAMS,
  VOICE_NAMES,
  VOICE_PARAMS,
} from "../engine/voiceParams";
import { DEFAULT_TOM_MODEL, type TomModel } from "../engine/tomModel";
import {
  loadInitialState,
  replaceAddressBarWithShareUrl,
  saveLocal,
} from "../algo/persistence";
import {
  DEFAULT_TEMPO_BPM,
  defaultEngineState,
  reduceDrumState,
  type DrumStateAction,
} from "./drumState";
import "./DrumMachine.css";

// Visual order: Cymbal on top, Bass on bottom.
const TRACKS = ["Cymbal", "Perc", "Tom 2", "Tom", "HiHat", "Snare", "Bass"];
// Maps visual row index → engine track index.
const TRACK_INDEX = [4, 6, 5, 3, 2, 1, 0];
const COLS = 16;
const ROWS = 7;
const TOM_TRACK = 3;
const TOM2_TRACK = 5;

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

// This is the only place persistent engine-major state is reordered for the
// UI's reverse-ordered grid.
function flatToVisual(flat: ArrayLike<number>): number[][] {
  return TRACKS.map((_, row) =>
    Array.from(
      { length: COLS },
      (_, col) => flat[TRACK_INDEX[row] * COLS + col] ?? 0,
    ),
  );
}

// The UI represents the complete range accepted by the authoritative engine,
// so every clamped echo has an exact position on the tempo knob.
const BPM_MIN = 30;
const BPM_MAX = 300;
const BPM_RANGE = BPM_MAX - BPM_MIN;
const TAP_RESET_MS = 2000;
const TAP_WINDOW = 4;

interface Props {
  wasmLoaded: boolean;
}

// One exhaustive action-to-command mapping replaces the former collection of
// effects. Reducer replacement actions come only from authoritative echoes and
// therefore deliberately send nothing back to the engine.
function sendStateAction(action: DrumStateAction): void {
  switch (action.type) {
    case "replace":
      return;
    case "tempo":
      engine.setTempo(action.value);
      return;
    case "swing":
      engine.setSwing(action.value);
      return;
    case "stepCount":
      engine.setStepCount(action.value);
      return;
    case "reverb":
      engine.setReverb(action.value);
      return;
    case "probability":
      engine.setProbability(action.value);
      return;
    case "humanize":
      engine.setHumanize(action.value);
      return;
    case "cell":
      engine.setCell(action.track, action.step, action.value);
      return;
    case "pattern":
      engine.setPattern(action.value);
      return;
    case "volume":
      engine.setVolume(action.track, action.value);
      return;
    case "decay":
      engine.setDecay(action.track, action.value);
      return;
    case "muted":
      engine.setMuted(action.track, action.value);
      return;
    case "voiceParam":
      engine.setVoiceParam(action.track, action.index, action.value);
      return;
    case "voiceParams":
      action.value.forEach((value, index) => {
        engine.setVoiceParam(action.track, index, value);
      });
      return;
    case "tomModel":
      engine.setTomModel(action.track, action.value);
      return;
    case "physicalTomParam":
      engine.setPhysicalTomParam(action.track, action.index, action.value);
      return;
    case "physicalTomParams":
      action.value.forEach((value, index) => {
        engine.setPhysicalTomParam(action.track, index, value);
      });
      return;
  }

  const exhaustive: never = action;
  return exhaustive;
}

export default function DrumMachine({ wasmLoaded }: Props) {
  // Restore saved/shared state once on mount; a valid URL hash wins over
  // localStorage (see loadInitialState).
  const initial = useMemo(() => loadInitialState() ?? defaultEngineState(), []);
  const [drumState, dispatch] = useReducer(reduceDrumState, initial);
  const currentState = useRef(drumState);
  currentState.current = drumState;
  const applyStateAction = useCallback((action: DrumStateAction) => {
    dispatch(action);
    sendStateAction(action);
  }, []);
  const [transport, setTransport] = useState<engine.TransportState>("stopped");
  // Engine track whose editor is open, or null. Also used to hand the keyboard
  // over to the dialog (see the Space handler below).
  const [editorTrack, setEditorTrack] = useState<number | null>(null);
  const [currentStep, setCurrentStep] = useState(-1);

  const pattern = useMemo(
    () => flatToVisual(drumState.pattern),
    [drumState.pattern],
  );
  const bpm = drumState.tempoBpm;
  const tempo = (bpm - BPM_MIN) / BPM_RANGE;
  const swing = drumState.swing / 0.5;
  const stepCount = drumState.stepCount;
  const steps = (stepCount - 1) / (COLS - 1);
  const reverb = drumState.reverb;
  const prob = drumState.probability;
  const humanize = drumState.humanize;
  const volumes = TRACK_INDEX.map((track) => drumState.tracks[track].volume);
  const decays = TRACK_INDEX.map((track) => drumState.tracks[track].decay);
  const muted = TRACK_INDEX.map((track) => drumState.tracks[track].muted);
  const voiceParamsByEngineTrack = drumState.tracks.map(
    (track) => track.voiceParams,
  );
  const tomModel = drumState.tracks[TOM_TRACK].tom?.model ?? DEFAULT_TOM_MODEL;
  const tom2Model =
    drumState.tracks[TOM2_TRACK].tom?.model ?? DEFAULT_TOM_MODEL;
  const physicalTomParams =
    drumState.tracks[TOM_TRACK].tom?.physicalParams ??
    Float32Array.from(defaultPhysicalTomParams());
  const physicalTom2Params =
    drumState.tracks[TOM2_TRACK].tom?.physicalParams ??
    Float32Array.from(defaultPhysicalTomParams());

  // Seed all restored/default configuration in one command. Later reducer
  // actions use granular commands, while echoes reconcile the whole snapshot.
  useEffect(() => {
    engine.setState(initial);
    // Mount-only: intentionally seeds the engine with one complete state.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // A fatal worker error leaves this React tree mounted (App's fault shell is
  // hidden/inert). When Retry makes a replacement engine ready, seed it from
  // the retained current state rather than the mount-time persistence value.
  const hasLoaded = useRef(false);
  useEffect(() => {
    if (!wasmLoaded) return;

    if (hasLoaded.current) engine.setState(currentState.current);
    hasLoaded.current = true;
  }, [wasmLoaded]);

  // Playhead follows the audible step reported by the audio worklet
  useEffect(() => engine.onStep(setCurrentStep), []);

  // Transport changes only after the worker has successfully applied the
  // corresponding Go command. Worker failure also resets this view to Stop.
  useEffect(() => engine.onTransport(setTransport), []);

  // Every configuration echo is a complete authoritative snapshot. The
  // bridge suppresses stale intermediate echoes when newer edits are in flight.
  useEffect(
    () => engine.onState((state) => dispatch({ type: "replace", state })),
    [],
  );

  // Auto-save to localStorage, debounced so a knob sweep writes once it settles.
  useEffect(() => {
    const id = window.setTimeout(() => saveLocal(drumState), 300);
    return () => window.clearTimeout(id);
  }, [drumState]);

  const handleShare = useCallback(
    () => replaceAddressBarWithShareUrl(drumState),
    [drumState],
  );

  const flatPattern = useMemo(
    () => Array.from(drumState.pattern),
    [drumState.pattern],
  );

  // applyFlatPattern replaces the whole pattern (presets and mutation) in both
  // the UI and the engine.
  const applyFlatPattern = useCallback(
    (flat: number[]) => {
      const value = Float32Array.from(flat);
      applyStateAction({ type: "pattern", value });
    },
    [applyStateAction],
  );

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
      const value = Math.round(clampedBpm);
      applyStateAction({ type: "tempo", value });
    }
  }, [applyStateAction]);

  // Mouse clicks blur the button so Space stays free for play/pause;
  // keyboard activation (detail === 0) keeps focus for grid navigation.
  const blurOnMouseClick = (e: React.MouseEvent<HTMLButtonElement>) => {
    if (e.detail > 0) e.currentTarget.blur();
  };

  const cycleCell = useCallback(
    (row: number, col: number) => {
      const track = TRACK_INDEX[row];
      const value = cycleVelocity(pattern[row][col]);
      applyStateAction({ type: "cell", track, step: col, value });
    },
    [applyStateAction, pattern],
  );

  const handlePlayPause = useCallback(async () => {
    if (!wasmLoaded) return;
    if (transport === "stopped" || transport === "paused") {
      await engine.play();
    } else if (transport === "playing") {
      engine.pause();
    }
  }, [transport, wasmLoaded]);

  const handleStop = useCallback(() => {
    if (!wasmLoaded || transport === "stopped") return;
    engine.stop();
  }, [transport, wasmLoaded]);

  // Space toggles play/pause unless a control is focused
  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.code !== "Space") return;
      // The voice editor owns the keyboard while it is open. A state check
      // rather than target.closest("dialog"), so it also holds when focus has
      // drifted to <body>.
      if (editorTrack !== null) return;
      const target = e.target as HTMLElement;
      if (target.closest("button, [role='slider'], input, textarea")) return;
      e.preventDefault();
      void handlePlayPause();
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [handlePlayPause, editorTrack]);

  const setTrackVolume = (visualRow: number, value: number) => {
    const track = TRACK_INDEX[visualRow];
    applyStateAction({ type: "volume", track, value });
  };

  const setTrackDecay = (visualRow: number, value: number) => {
    const track = TRACK_INDEX[visualRow];
    applyStateAction({ type: "decay", track, value });
  };

  const toggleMute = (visualRow: number) => {
    const track = TRACK_INDEX[visualRow];
    const value = !drumState.tracks[track].muted;
    applyStateAction({ type: "muted", track, value });
  };

  // ── Voice editor ──────────────────────────────────────────────────────────

  // Where focus goes when the dialog closes, and whether it should: mouse users
  // get focus dropped so Space stays free for the transport, the same trade-off
  // blurOnMouseClick makes elsewhere.
  const editorOpener = useRef<HTMLButtonElement | null>(null);
  const editorOpenedByMouse = useRef(false);

  const openEditor = useCallback(
    (engineTrack: number, opener: HTMLButtonElement, byMouse: boolean) => {
      editorOpener.current = opener;
      editorOpenedByMouse.current = byMouse;
      setEditorTrack(engineTrack);
    },
    [],
  );

  const closeEditor = useCallback(() => {
    setEditorTrack(null);
    if (!editorOpenedByMouse.current) editorOpener.current?.focus();
    editorOpener.current = null;
  }, []);

  // One message per user event. A fan-out effect like the volume/decay ones
  // would re-send all ~25 parameters on every pointermove of a knob drag.
  const setVoiceParam = useCallback(
    (engineTrack: number, index: number, value: number) => {
      applyStateAction({
        type: "voiceParam",
        track: engineTrack,
        index,
        value,
      });
    },
    [applyStateAction],
  );

  const resetVoice = useCallback(
    (engineTrack: number) => {
      const specs = VOICE_PARAMS[engineTrack];
      const values = Float32Array.from(specs, (spec) => spec.default);
      applyStateAction({
        type: "voiceParams",
        track: engineTrack,
        value: values,
      });
    },
    [applyStateAction],
  );

  const setPhysicalTomParam = useCallback(
    (engineTrack: number, index: number, value: number) => {
      applyStateAction({
        type: "physicalTomParam",
        track: engineTrack,
        index,
        value,
      });
    },
    [applyStateAction],
  );

  const resetPhysicalTom = useCallback(
    (engineTrack: number) => {
      const defaults = Float32Array.from(defaultPhysicalTomParams());
      applyStateAction({
        type: "physicalTomParams",
        track: engineTrack,
        value: defaults,
      });
    },
    [applyStateAction],
  );

  const setTomModel = useCallback(
    (engineTrack: number, value: TomModel) => {
      applyStateAction({ type: "tomModel", track: engineTrack, value });
    },
    [applyStateAction],
  );

  const editorTomModel =
    editorTrack === TOM_TRACK
      ? tomModel
      : editorTrack === TOM2_TRACK
        ? tom2Model
        : undefined;
  const editorUsesPhysical = editorTomModel === "physical";
  const editorPhysicalParams =
    editorTrack === TOM2_TRACK ? physicalTom2Params : physicalTomParams;

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
        <AlgoPanel
          disabled={!wasmLoaded}
          pattern={flatPattern}
          stepCount={stepCount}
          onApplyPattern={applyFlatPattern}
          onShare={handleShare}
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
              onChange={(v) => setTrackVolume(row, v)}
              label={name.slice(0, 3).toUpperCase()}
              ariaLabel={`${name} volume`}
              defaultValue={0.75}
              size={42}
              color={AMBER}
            />
            <Knob
              value={decays[row]}
              onChange={(v) => setTrackDecay(row, v)}
              label="DEC"
              ariaLabel={`${name} decay`}
              defaultValue={0.5}
              size={42}
              color={BLUE}
            />
            {/* Deliberately no blurOnMouseClick: closeEditor needs the opener
                to still be focusable to hand focus back to it. */}
            <button
              type="button"
              className="dm-voice-open"
              // Named from VOICE_NAMES rather than the strip's short label so
              // the trigger and the dialog it opens announce the same voice.
              aria-label={`${VOICE_NAMES[TRACK_INDEX[row]]} voice settings`}
              aria-haspopup="dialog"
              title={`Edit the ${VOICE_NAMES[TRACK_INDEX[row]]} synth voice`}
              disabled={!wasmLoaded}
              onClick={(e) =>
                openEditor(TRACK_INDEX[row], e.currentTarget, e.detail > 0)
              }
            >
              {/* Three faders, not a gear: this opens a synth panel. */}
              <svg
                width={13}
                height={13}
                viewBox="0 0 14 14"
                aria-hidden="true"
              >
                <g
                  stroke="currentColor"
                  strokeWidth={1.3}
                  strokeLinecap="round"
                  opacity={0.55}
                >
                  <line x1={3} y1={2} x2={3} y2={12} />
                  <line x1={7} y1={2} x2={7} y2={12} />
                  <line x1={11} y1={2} x2={11} y2={12} />
                </g>
                <g fill="currentColor">
                  <rect x={1.6} y={4} width={2.8} height={2} rx={1} />
                  <rect x={5.6} y={8} width={2.8} height={2} rx={1} />
                  <rect x={9.6} y={5.5} width={2.8} height={2} rx={1} />
                </g>
              </svg>
            </button>
          </div>
        ))}
      </div>

      {editorTrack !== null && (
        <VoiceEditor
          name={VOICE_NAMES[editorTrack]}
          specs={
            editorUsesPhysical ? PHYSICAL_TOM_PARAMS : VOICE_PARAMS[editorTrack]
          }
          values={
            editorUsesPhysical
              ? editorPhysicalParams
              : voiceParamsByEngineTrack[editorTrack]
          }
          disabled={!wasmLoaded}
          model={editorTomModel}
          onModelChange={
            editorTrack === TOM_TRACK
              ? (model) => setTomModel(TOM_TRACK, model)
              : editorTrack === TOM2_TRACK
                ? (model) => setTomModel(TOM2_TRACK, model)
                : undefined
          }
          onChange={(index, value) =>
            editorUsesPhysical
              ? setPhysicalTomParam(editorTrack, index, value)
              : setVoiceParam(editorTrack, index, value)
          }
          onReset={() =>
            editorUsesPhysical
              ? resetPhysicalTom(editorTrack)
              : resetVoice(editorTrack)
          }
          onAudition={(amount) => void engine.triggerVoice(editorTrack, amount)}
          onRequestClose={closeEditor}
        />
      )}

      <footer className="dm-transport">
        <div className="dm-transport-buttons">
          <button
            type="button"
            className={`dm-play ${transport === "playing" ? "dm-play-active" : ""}`}
            onClick={(e) => {
              blurOnMouseClick(e);
              void handlePlayPause();
            }}
            disabled={!wasmLoaded || transport === "starting"}
            aria-label={
              transport === "starting"
                ? "Starting"
                : transport === "playing"
                  ? "Pause"
                  : "Play"
            }
            title={`${transport === "playing" ? "Pause" : transport === "starting" ? "Starting" : "Play"} (Space)`}
          >
            {transport === "playing" ? (
              <svg
                width={16}
                height={16}
                viewBox="0 0 18 18"
                aria-hidden="true"
              >
                <rect x={3} y={3} width={4} height={12} fill="white" rx={1} />
                <rect x={11} y={3} width={4} height={12} fill="white" rx={1} />
              </svg>
            ) : (
              <svg
                width={16}
                height={16}
                viewBox="0 0 18 18"
                aria-hidden="true"
              >
                <polygon points="5,3 15,9 5,15" fill="white" />
              </svg>
            )}
          </button>
          <button
            type="button"
            className="dm-stop"
            onClick={(e) => {
              blurOnMouseClick(e);
              handleStop();
            }}
            disabled={!wasmLoaded || transport === "stopped"}
            aria-label="Stop"
            title="Stop and return to step 1"
          >
            <span aria-hidden="true" />
          </button>
        </div>

        <div className="dm-tempo-group">
          <Knob
            value={tempo}
            onChange={(position) => {
              const value = Math.round(BPM_MIN + position * BPM_RANGE);
              applyStateAction({ type: "tempo", value });
            }}
            label={`${bpm} BPM`}
            ariaLabel="Tempo"
            valueText={() => `${bpm} BPM`}
            defaultValue={(DEFAULT_TEMPO_BPM - BPM_MIN) / BPM_RANGE}
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
          onChange={(position) => {
            const value = position * 0.5;
            applyStateAction({ type: "swing", value });
          }}
          label="SWING"
          defaultValue={0}
          size={54}
          color={AMBER}
        />
        <Knob
          value={steps}
          onChange={(position) => {
            const value = Math.round(1 + position * (COLS - 1));
            applyStateAction({ type: "stepCount", value });
          }}
          label="STEPS"
          ariaLabel="Pattern length"
          valueText={() => `${stepCount} steps`}
          defaultValue={1}
          size={54}
          color={AMBER}
        />
        <Knob
          value={prob}
          onChange={(value) => {
            applyStateAction({ type: "probability", value });
          }}
          label="PROB"
          ariaLabel="Trigger probability"
          defaultValue={1}
          size={54}
          color={AMBER}
        />
        <Knob
          value={humanize}
          onChange={(value) => {
            applyStateAction({ type: "humanize", value });
          }}
          label="HUMAN"
          ariaLabel="Humanize"
          defaultValue={0}
          size={54}
          color={AMBER}
        />

        <div className="dm-transport-spacer" />

        <Knob
          value={reverb}
          onChange={(value) => {
            applyStateAction({ type: "reverb", value });
          }}
          label="REVERB"
          defaultValue={0}
          size={54}
          color={AMBER}
        />
      </footer>
    </div>
  );
}
