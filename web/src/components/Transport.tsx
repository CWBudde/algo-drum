import { DEFAULT_TEMPO_BPM, type DrumStateAction } from "./drumState";
import type { TransportState } from "../engine/wasmEngine";
import { STEP_CAPACITY } from "../algo/pattern";
import Knob from "./Knob";

const BPM_MIN = 30;
const BPM_MAX = 300;
const BPM_RANGE = BPM_MAX - BPM_MIN;

interface Props {
  disabled: boolean;
  transport: TransportState;
  bpm: number;
  swing: number;
  bank: number;
  stepCount: number;
  probability: number;
  humanize: number;
  reverb: number;
  fillMode: boolean;
  metronomeEnabled: boolean;
  countInBars: number;
  onAction: (action: DrumStateAction) => void;
  onPlayPause: () => void;
  onStop: () => void;
  onTap: () => void;
}

export default function Transport({
  disabled,
  transport,
  bpm,
  swing,
  bank,
  stepCount,
  probability,
  humanize,
  reverb,
  fillMode,
  metronomeEnabled,
  countInBars,
  onAction,
  onPlayPause,
  onStop,
  onTap,
}: Props) {
  const starting = transport === "starting";
  const countingIn = transport === "counting-in";
  const playing = transport === "playing";
  const playLabel = countingIn
    ? "Counting in"
    : starting
      ? "Starting"
      : playing
        ? "Pause"
        : "Play";
  return (
    <footer id="dm-transport" className="dm-transport" tabIndex={-1}>
      <div className="dm-transport-buttons">
        <button
          type="button"
          className={`dm-play ${playing ? "dm-play-active" : ""}`}
          onClick={(event) => {
            if (event.detail > 0) event.currentTarget.blur();
            onPlayPause();
          }}
          disabled={disabled || starting || countingIn}
          aria-label={playLabel}
          aria-keyshortcuts="Space"
          title={`${playLabel} (Space)`}
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
        <button
          type="button"
          className="dm-stop"
          onClick={(event) => {
            if (event.detail > 0) event.currentTarget.blur();
            onStop();
          }}
          disabled={disabled || transport === "stopped"}
          aria-label="Stop"
          aria-keyshortcuts="S"
          title="Stop and return to step 1 (S)"
        >
          <span aria-hidden="true" />
        </button>
        <button
          type="button"
          className="dm-fill"
          aria-pressed={fillMode}
          disabled={disabled}
          onClick={() => {
            onAction({ type: "fillMode", value: !fillMode });
          }}
          title="Enable Fill-only conditional triggers"
        >
          FILL
        </button>
        <button
          type="button"
          className="dm-metronome"
          aria-pressed={metronomeEnabled}
          aria-label="Playback metronome"
          disabled={disabled}
          onClick={() => {
            onAction({
              type: "metronomeEnabled",
              value: !metronomeEnabled,
            });
          }}
          title="Toggle quarter-note guide clicks during playback; count-in clicks always sound"
        >
          CLICK
        </button>
        <label className="dm-count-in">
          <span>COUNT</span>
          <select
            aria-label="Count-in bars"
            value={countInBars}
            disabled={disabled}
            onChange={(event) => {
              onAction({
                type: "countInBars",
                value: Number(event.target.value),
              });
            }}
          >
            <option value={0}>OFF</option>
            <option value={1}>1 BAR</option>
            <option value={2}>2 BARS</option>
          </select>
        </label>
      </div>

      <div className="dm-tempo-group">
        <Knob
          value={(bpm - BPM_MIN) / BPM_RANGE}
          onChange={(position) => {
            onAction({
              type: "tempo",
              value: Math.round(BPM_MIN + position * BPM_RANGE),
            });
          }}
          label={`${bpm} BPM`}
          ariaLabel="Tempo"
          valueText={() => `${bpm} BPM`}
          defaultValue={(DEFAULT_TEMPO_BPM - BPM_MIN) / BPM_RANGE}
          size={54}
        />
        <button
          type="button"
          className="dm-tap"
          onClick={onTap}
          disabled={disabled}
          aria-label="Tap tempo"
          title="Tap to set tempo"
        >
          TAP
        </button>
      </div>
      <Knob
        value={swing / 0.5}
        onChange={(position) => {
          onAction({ type: "swing", value: position * 0.5 });
        }}
        label="SWING"
        defaultValue={0}
        size={54}
      />
      <Knob
        value={(stepCount - 1) / (STEP_CAPACITY - 1)}
        onChange={(position) => {
          onAction({
            type: "stepCount",
            bank,
            value: Math.round(1 + position * (STEP_CAPACITY - 1)),
          });
        }}
        label="STEPS"
        ariaLabel="Pattern length"
        valueText={() => `${stepCount} steps`}
        defaultValue={1}
        size={54}
      />
      <Knob
        value={probability}
        onChange={(value) => {
          onAction({ type: "probability", value });
        }}
        label="PROB"
        ariaLabel="Trigger probability"
        defaultValue={1}
        size={54}
      />
      <Knob
        value={humanize}
        onChange={(value) => {
          onAction({ type: "humanize", value });
        }}
        label="HUMAN"
        ariaLabel="Humanize"
        defaultValue={0}
        size={54}
      />

      <div className="dm-transport-spacer" />

      <Knob
        value={reverb}
        onChange={(value) => {
          onAction({ type: "reverb", value });
        }}
        label="REVERB"
        defaultValue={0}
        size={54}
      />
    </footer>
  );
}
