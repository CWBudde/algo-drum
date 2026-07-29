import { useCallback, useRef, useState } from "react";
import { mutate } from "../algo/mutate";
import { PRESETS, presetToFlat } from "../algo/presets";
import "./AlgoPanel.css";

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
  const [copied, setCopied] = useState(false);
  const copiedTimer = useRef<number | null>(null);

  const applyPreset = useCallback(
    (value: string) => {
      const idx = Number(value);
      setPresetIndex(idx);
      if (idx >= 0 && PRESETS[idx]) onApplyPattern(presetToFlat(PRESETS[idx]));
    },
    [onApplyPattern],
  );

  const handleMutate = useCallback(() => {
    setPresetIndex(-1);
    onApplyPattern(mutate(pattern, { stepCount }));
  }, [onApplyPattern, pattern, stepCount]);

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
        className="dm-algo-btn dm-algo-share"
        disabled={disabled}
        onClick={handleShare}
        aria-label="Copy shareable link"
        title="Copy a link that restores this pattern"
      >
        {copied ? "COPIED ✓" : "SHARE"}
      </button>
      <span className="dm-sr-only" role="status" aria-live="polite">
        {copied ? "Link copied to clipboard" : ""}
      </span>
    </section>
  );
}
