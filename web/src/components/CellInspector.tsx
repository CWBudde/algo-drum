import { TRIGGER_CONDITION_LABELS } from "../engine/engineState";
import { TRACKS, TRACK_INDEX } from "./patternView";

interface Props {
  track: number;
  step: number;
  probability: number;
  humanize: number;
  condition: number;
  repeats: number;
  onProbabilityChange: (value: number) => void;
  onHumanizeChange: (value: number) => void;
  onConditionChange: (value: number) => void;
  onRepeatsChange: (value: number) => void;
  onClose: () => void;
}

export default function CellInspector({
  track,
  step,
  probability,
  humanize,
  condition,
  repeats,
  onProbabilityChange,
  onHumanizeChange,
  onConditionChange,
  onRepeatsChange,
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
        HUMANIZE
        <input
          type="range"
          min={0}
          max={100}
          step={1}
          value={Math.round(humanize * 100)}
          aria-valuetext={`${Math.round(humanize * 100)} percent`}
          onChange={(event) =>
            onHumanizeChange(Number(event.target.value) / 100)
          }
        />
        <output>{Math.round(humanize * 100)}%</output>
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
      <label>
        RATCHET
        <select
          value={repeats}
          aria-label="Ratchet repeats"
          onChange={(event) => onRepeatsChange(Number(event.target.value))}
        >
          <option value={1}>1 hit</option>
          <option value={2}>2 hits</option>
          <option value={3}>3 hits</option>
          <option value={4}>4 hits</option>
        </select>
      </label>
      <button type="button" onClick={onClose} aria-label="Close step settings">
        ×
      </button>
    </aside>
  );
}
