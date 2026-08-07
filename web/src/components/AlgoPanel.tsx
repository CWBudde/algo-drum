import { useCallback, useEffect, useRef, useState } from "react";
import { euclid } from "../algo/euclid";
import { mutate } from "../algo/mutate";
import { PRESETS, presetToFlat } from "../algo/presets";
import { fillEuclidTrack, TRACK_INDEX, TRACKS } from "./patternView";
import "./AlgoPanel.css";

interface Props {
  disabled: boolean;
  pattern: number[]; // current flat, engine-major pattern
  stepCount: number;
  onApplyPattern: (flat: number[]) => void;
  canUndo: boolean;
  canRedo: boolean;
  onUndo: () => void;
  onRedo: () => void;
  // Explicit share action: publishes the state in the address bar and returns
  // the same URL for the clipboard.
  onShare: () => string;
}

export default function AlgoPanel({
  disabled,
  pattern,
  stepCount,
  onApplyPattern,
  canUndo,
  canRedo,
  onUndo,
  onRedo,
  onShare,
}: Props) {
  const [presetIndex, setPresetIndex] = useState(-1);
  const [copied, setCopied] = useState(false);
  const [announcement, setAnnouncement] = useState("");
  const copiedTimer = useRef<number | null>(null);
  const [euclidTrack, setEuclidTrack] = useState(0);
  const [euclidPulses, setEuclidPulses] = useState(4);
  const [euclidRotation, setEuclidRotation] = useState(0);

  useEffect(
    () => () => {
      if (copiedTimer.current) window.clearTimeout(copiedTimer.current);
    },
    [],
  );

  const applyPreset = useCallback(
    (value: string) => {
      const idx = Number(value);
      setPresetIndex(idx);
      const preset = PRESETS[idx];
      if (idx >= 0 && preset) {
        onApplyPattern(presetToFlat(preset));
        setAnnouncement(`${preset.name} preset loaded.`);
      }
    },
    [onApplyPattern],
  );

  const handleMutate = useCallback(() => {
    setPresetIndex(-1);
    onApplyPattern(mutate(pattern, { stepCount }));
    setAnnouncement("Pattern mutated.");
  }, [onApplyPattern, pattern, stepCount]);

  const handleClear = useCallback(() => {
    setPresetIndex(-1);
    onApplyPattern(new Array<number>(pattern.length).fill(0));
    setAnnouncement("Pattern cleared.");
  }, [onApplyPattern, pattern.length]);

  const handleEuclid = useCallback(() => {
    setPresetIndex(-1);
    const pulses = Number.isFinite(euclidPulses)
      ? Math.max(0, Math.min(stepCount, euclidPulses))
      : 0;
    const rotation = Number.isFinite(euclidRotation) ? euclidRotation : 0;
    onApplyPattern(
      fillEuclidTrack(
        pattern,
        euclidTrack,
        euclid(pulses, stepCount, rotation),
      ),
    );
    const visualRow = TRACK_INDEX.findIndex((track) => track === euclidTrack);
    setAnnouncement(
      `${TRACKS[visualRow] ?? "Track"} filled with ${pulses} Euclidean hits.`,
    );
  }, [
    euclidPulses,
    euclidRotation,
    euclidTrack,
    onApplyPattern,
    pattern,
    stepCount,
  ]);

  const handleShare = useCallback(() => {
    const url = onShare();
    const clipboard = navigator.clipboard as Clipboard | undefined;
    if (clipboard) {
      void clipboard.writeText(url).catch(() => {
        // Clipboard may be blocked; the hash is still in the address bar.
      });
    }
    setCopied(true);
    if (copiedTimer.current) window.clearTimeout(copiedTimer.current);
    copiedTimer.current = window.setTimeout(() => setCopied(false), 1600);
  }, [onShare]);

  return (
    <section className="dm-algo" aria-label="Pattern tools">
      <select
        className="dm-algo-select"
        value={presetIndex}
        disabled={disabled}
        aria-label="Load preset pattern"
        title="Load preset pattern"
        onChange={(e) => applyPreset(e.target.value)}
      >
        <option value={-1} disabled>
          PRESET…
        </option>
        {PRESETS.map((preset, i) => (
          <option key={preset.name} value={i}>
            {preset.name}
          </option>
        ))}
      </select>
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
        title="Clear every track"
      >
        CLEAR
      </button>
      <details className="dm-euclid">
        <summary className="dm-algo-btn">EUCLID</summary>
        <div className="dm-euclid-panel">
          <label>
            TRACK
            <select
              value={euclidTrack}
              onChange={(event) => setEuclidTrack(Number(event.target.value))}
              disabled={disabled}
            >
              {TRACKS.map((name, row) => (
                <option key={name} value={TRACK_INDEX[row]}>
                  {name}
                </option>
              ))}
            </select>
          </label>
          <label>
            HITS
            <input
              type="number"
              min={0}
              max={stepCount}
              value={euclidPulses}
              onChange={(event) => setEuclidPulses(Number(event.target.value))}
              disabled={disabled}
            />
          </label>
          <label>
            ROTATE
            <input
              type="number"
              min={0}
              max={Math.max(0, stepCount - 1)}
              value={euclidRotation}
              onChange={(event) =>
                setEuclidRotation(Number(event.target.value))
              }
              disabled={disabled}
            />
          </label>
          <button
            type="button"
            className="dm-algo-btn"
            onClick={handleEuclid}
            disabled={disabled}
            title="Replace this track with the Euclidean rhythm"
          >
            FILL
          </button>
        </div>
      </details>
      <div className="dm-history" role="group" aria-label="Pattern history">
        <button
          type="button"
          className="dm-algo-btn dm-history-btn"
          disabled={disabled || !canUndo}
          onClick={() => {
            onUndo();
            setAnnouncement("Pattern action undone.");
          }}
          aria-label="Undo pattern action"
          title="Undo pattern action (Ctrl/Cmd+Z)"
        >
          ↶
        </button>
        <button
          type="button"
          className="dm-algo-btn dm-history-btn"
          disabled={disabled || !canRedo}
          onClick={() => {
            onRedo();
            setAnnouncement("Pattern action redone.");
          }}
          aria-label="Redo pattern action"
          title="Redo pattern action (Ctrl/Cmd+Shift+Z)"
        >
          ↷
        </button>
      </div>
      <button
        type="button"
        className="dm-algo-btn dm-algo-share"
        disabled={disabled}
        onClick={handleShare}
        aria-label="Copy shareable link"
        title="Copy a link that restores this pattern"
      >
        {copied ? "COPIED ✓" : "SHARE"}
      </button>
      <span className="dm-sr-only" role="status" aria-live="polite">
        {copied ? "Link copied to clipboard" : announcement}
      </span>
    </section>
  );
}
