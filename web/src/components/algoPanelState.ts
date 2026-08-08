import { PRESETS, presetToFlat } from "../algo/presets";

// Engine snapshots carry the grid in Float32Array storage. Normalize both
// sides before comparing so a preset compiled from the source-level `0.7`
// detent still matches its exact engine representation without introducing a
// tolerance that could label a hand-edited pattern as a preset.
const NORMALIZED_PRESETS = PRESETS.map((preset) =>
  Float32Array.from(presetToFlat(preset)),
);

export function matchingPresetIndex(pattern: ArrayLike<number>): number {
  return NORMALIZED_PRESETS.findIndex((preset) => {
    if (preset.length !== pattern.length) return false;
    for (let index = 0; index < preset.length; index++) {
      if (preset[index] !== Math.fround(pattern[index] ?? Number.NaN)) {
        return false;
      }
    }
    return true;
  });
}

export function normalizeIntegerDraft(
  draft: string,
  minimum: number,
  maximum: number,
): number {
  const value = Number(draft);
  if (!Number.isFinite(value)) return minimum;
  return Math.max(minimum, Math.min(maximum, Math.round(value)));
}
