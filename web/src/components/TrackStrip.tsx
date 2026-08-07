import { VOICE_NAMES } from "../engine/voiceParams";
import { STEP_CAPACITY } from "../algo/pattern";
import Knob from "./Knob";
import { TRACK_INDEX } from "./patternView";

const AMBER = "#C87828";
const BLUE = "#6D95C8";

interface Props {
  name: string;
  row: number;
  disabled: boolean;
  volume: number;
  decay: number;
  muted: boolean;
  trackLength: number;
  onVolumeChange: (value: number) => void;
  onDecayChange: (value: number) => void;
  onToggleMute: () => void;
  onTrackLengthChange: (value: number) => void;
  onOpenEditor: (
    track: number,
    opener: HTMLButtonElement,
    byMouse: boolean,
  ) => void;
}

export default function TrackStrip({
  name,
  row,
  disabled,
  volume,
  decay,
  muted,
  trackLength,
  onVolumeChange,
  onDecayChange,
  onToggleMute,
  onTrackLengthChange,
  onOpenEditor,
}: Props) {
  const track = TRACK_INDEX[row];
  return (
    <div
      className="dm-track-controls"
      style={{ gridRow: row + 1, gridColumn: STEP_GRID_CONTROL_COLUMN }}
    >
      <button
        type="button"
        className="dm-mute"
        aria-pressed={muted}
        aria-label={`Mute ${name}`}
        title={muted ? `Unmute ${name}` : `Mute ${name}`}
        onClick={(event) => {
          if (event.detail > 0) event.currentTarget.blur();
          onToggleMute();
        }}
      >
        <span className="dm-mute-led" aria-hidden="true" />
      </button>
      <label className="dm-track-length-label">
        <span className="dm-sr-only">{name} pattern length</span>
        <select
          className="dm-track-length"
          value={trackLength}
          onChange={(event) => onTrackLengthChange(Number(event.target.value))}
          title={`${name} pattern length: ${trackLength} steps`}
        >
          {Array.from({ length: STEP_CAPACITY }, (_, index) => index + 1).map(
            (length) => (
              <option key={length} value={length}>
                {length}
              </option>
            ),
          )}
        </select>
      </label>
      <Knob
        value={volume}
        onChange={onVolumeChange}
        label={name.slice(0, 3).toUpperCase()}
        ariaLabel={`${name} volume`}
        defaultValue={0.75}
        size={42}
        color={AMBER}
      />
      <Knob
        value={decay}
        onChange={onDecayChange}
        label="DEC"
        ariaLabel={`${name} decay`}
        defaultValue={0.5}
        size={42}
        color={BLUE}
      />
      <button
        type="button"
        className="dm-voice-open"
        aria-label={`${VOICE_NAMES[track]} voice settings`}
        aria-haspopup="dialog"
        title={`Edit the ${VOICE_NAMES[track]} synth voice`}
        disabled={disabled}
        onClick={(event) =>
          onOpenEditor(track, event.currentTarget, event.detail > 0)
        }
      >
        <svg width={13} height={13} viewBox="0 0 14 14" aria-hidden="true">
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
  );
}

// labels | 16 cells | gap | controls
const STEP_GRID_CONTROL_COLUMN = STEP_CAPACITY + 3;
