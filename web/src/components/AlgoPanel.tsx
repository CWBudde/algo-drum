import { useCallback, useMemo, useRef, useState } from "react";
import { euclid } from "../algo/euclid";
import { mutate } from "../algo/mutate";
import { emptyPattern, PRESETS, presetToFlat } from "../algo/presets";
import { STEP_CAPACITY, VEL_NORMAL, VEL_OFF, index } from "../algo/pattern";
import "./AlgoPanel.css";

// Engine track order (0 Bass … 4 Cymbal), for the Euclid track selector.
const TRACK_NAMES = ["Bass", "Snare", "HiHat", "Tom", "Cymbal"];

interface Props {
  disabled: boolean;
  pattern: number[]; // current flat, engine-major pattern
  stepCount: number;
  onApplyPattern: (flat: number[]) => void;
  // Writes the share hash and returns the shareable URL to copy.
  getShareUrl: () => string;
}

export default function AlgoPanel({
  disabled,
  pattern,
  stepCount,
  onApplyPattern,
  getShareUrl,
}: Props) {
  const [presetIndex, setPresetIndex] = useState(-1);
  const [euclidTrack, setEuclidTrack] = useState(0);
  const [pulses, setPulses] = useState(4);
  const [rotation, setRotation] = useState(0);
  const [copied, setCopied] = useState(false);
  const copiedTimer = useRef<number | null>(null);

  // Clamp the Euclid controls to the active pattern length as it changes.
  const maxPulses = stepCount;
  const clampedPulses = useMemo(
    () => Math.max(0, Math.min(maxPulses, pulses)),
    [pulses, maxPulses],
  );
  const clampedRotation = useMemo(
    () => Math.max(0, Math.min(stepCount - 1, rotation)),
    [rotation, stepCount],
  );

  const applyPreset = useCallback(
    (value: string) => {
      const idx = Number(value);
      setPresetIndex(idx);
      if (idx >= 0 && PRESETS[idx]) onApplyPattern(presetToFlat(PRESETS[idx]));
    },
    [onApplyPattern],
  );

  const handleClear = useCallback(() => {
    setPresetIndex(-1);
    onApplyPattern(emptyPattern());
  }, [onApplyPattern]);

  const handleMutate = useCallback(() => {
    onApplyPattern(mutate(pattern, { stepCount }));
  }, [onApplyPattern, pattern, stepCount]);

  const handleEuclid = useCallback(() => {
    const hits = euclid(clampedPulses, stepCount, clampedRotation);
    const next = pattern.slice();
    for (let step = 0; step < STEP_CAPACITY; step++) {
      if (step < stepCount) {
        next[index(euclidTrack, step)] = hits[step] ? VEL_NORMAL : VEL_OFF;
      }
    }
    onApplyPattern(next);
  }, [
    clampedPulses,
    clampedRotation,
    euclidTrack,
    onApplyPattern,
    pattern,
    stepCount,
  ]);

  const handleShare = useCallback(() => {
    const url = getShareUrl();
    const clipboard = navigator.clipboard as Clipboard | undefined;
    if (clipboard) {
      void clipboard.writeText(url).catch(() => {
        // Clipboard may be blocked; the hash is still in the address bar.
      });
    }
    setCopied(true);
    if (copiedTimer.current) window.clearTimeout(copiedTimer.current);
    copiedTimer.current = window.setTimeout(() => setCopied(false), 1600);
  }, [getShareUrl]);

  return (
    <section className="dm-algo" aria-label="Algorithmic tools">
      <div className="dm-algo-group">
        <span className="dm-algo-legend">PATTERN</span>
        <div className="dm-algo-row">
          <label className="dm-algo-field">
            <span className="dm-algo-label">PRESET</span>
            <select
              className="dm-algo-select"
              value={presetIndex}
              disabled={disabled}
              aria-label="Load preset pattern"
              onChange={(e) => applyPreset(e.target.value)}
            >
              <option value={-1} disabled>
                choose…
              </option>
              {PRESETS.map((preset, i) => (
                <option key={preset.name} value={i}>
                  {preset.name}
                </option>
              ))}
            </select>
          </label>
          <button
            type="button"
            className="dm-algo-btn"
            disabled={disabled}
            onClick={handleMutate}
            aria-label="Mutate pattern"
            title="Random-walk the current pattern"
          >
            MUTATE
          </button>
          <button
            type="button"
            className="dm-algo-btn dm-algo-btn-warn"
            disabled={disabled}
            onClick={handleClear}
            aria-label="Clear pattern"
            title="Clear all steps"
          >
            CLEAR
          </button>
        </div>
      </div>

      <div className="dm-algo-group">
        <span className="dm-algo-legend">EUCLID</span>
        <div className="dm-algo-row">
          <label className="dm-algo-field">
            <span className="dm-algo-label">TRACK</span>
            <select
              className="dm-algo-select"
              value={euclidTrack}
              disabled={disabled}
              aria-label="Euclid target track"
              onChange={(e) => setEuclidTrack(Number(e.target.value))}
            >
              {TRACK_NAMES.map((name, i) => (
                <option key={name} value={i}>
                  {name}
                </option>
              ))}
            </select>
          </label>
          <label className="dm-algo-field">
            <span className="dm-algo-label">PULSES</span>
            <input
              className="dm-algo-num"
              type="number"
              min={0}
              max={maxPulses}
              value={clampedPulses}
              disabled={disabled}
              aria-label="Euclid pulses"
              onChange={(e) => setPulses(Number(e.target.value))}
            />
          </label>
          <label className="dm-algo-field">
            <span className="dm-algo-label">ROTATE</span>
            <input
              className="dm-algo-num"
              type="number"
              min={0}
              max={Math.max(0, stepCount - 1)}
              value={clampedRotation}
              disabled={disabled}
              aria-label="Euclid rotation"
              onChange={(e) => setRotation(Number(e.target.value))}
            />
          </label>
          <button
            type="button"
            className="dm-algo-btn"
            disabled={disabled}
            onClick={handleEuclid}
            aria-label={`Apply Euclid E(${clampedPulses}, ${stepCount}) to ${TRACK_NAMES[euclidTrack]}`}
            title="Fill the track with an even Euclidean rhythm"
          >
            FILL
          </button>
        </div>
      </div>

      <div className="dm-algo-group dm-algo-group-end">
        <span className="dm-algo-legend">SHARE</span>
        <div className="dm-algo-row">
          <button
            type="button"
            className="dm-algo-btn"
            disabled={disabled}
            onClick={handleShare}
            aria-label="Copy shareable link"
            title="Copy a link that restores this pattern"
          >
            {copied ? "COPIED ✓" : "SHARE"}
          </button>
          <span
            className={`dm-algo-toast ${copied ? "dm-algo-toast-show" : ""}`}
            role="status"
            aria-live="polite"
          >
            {copied ? "Link copied to clipboard" : ""}
          </span>
        </div>
      </div>
    </section>
  );
}
