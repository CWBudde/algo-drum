import { TRIGGER_CONDITION_LABELS } from "../engine/engineState";
import { TRACKS, TRACK_INDEX } from "./patternView";

interface Props {
  track: number;
  step: number;
  probability: number;
  condition: number;
  onProbabilityChange: (value: number) => void;
  onConditionChange: (value: number) => void;
  onClose: () => void;
}

export default function CellInspector({
  track,
  step,
  probability,
  condition,
  onProbabilityChange,
  onConditionChange,
  onClose,
}: Props) {
  const visualRow = TRACK_INDEX.indexOf(track as (typeof TRACK_INDEX)[number]);
  const name = TRACKS[visualRow] ?? `Track ${track + 1}`;
  return (
    <aside
      className="dm-cell-inspector"
      aria-label="Step trigger settings"
      onKeyDown={(event) => {
        if (event.key !== "Escape") return;
        event.preventDefault();
        onClose();
      }}
    >
      <strong>
        {name.toUpperCase()} · {step + 1}
      </strong>
      <label>
        PROBABILITY
        <input
          autoFocus
          type="range"
          min={0}
          max={100}
          step={1}
          value={Math.round(probability * 100)}
          aria-valuetext={`${Math.round(probability * 100)} percent`}
          onChange={(event) =>
            onProbabilityChange(Number(event.target.value) / 100)
          }
        />
        <output>{Math.round(probability * 100)}%</output>
      </label>
      <label>
        CONDITION
        <select
          value={condition}
          onChange={(event) => onConditionChange(Number(event.target.value))}
        >
          {TRIGGER_CONDITION_LABELS.map((label, value) => (
            <option key={label} value={value}>
              {label}
            </option>
          ))}
        </select>
      </label>
      <button type="button" onClick={onClose} aria-label="Close step settings">
        ×
      </button>
    </aside>
  );
}
