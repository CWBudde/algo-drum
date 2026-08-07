import {
  STEP_CAPACITY,
  TRACK_COUNT,
  VEL_ACCENT,
  VEL_NORMAL,
} from "../algo/pattern";

// The hardware-style grid is intentionally the reverse of engine track order.
export const TRACKS = [
  "Cymbal",
  "Perc",
  "Tom 2",
  "Tom",
  "HiHat",
  "Snare",
  "Bass",
] as const;

export const TRACK_INDEX = [4, 6, 5, 3, 2, 1, 0] as const;

export interface PlayheadClock {
  masterStep: number;
  clockStep: number;
}

export const STOPPED_PLAYHEAD: PlayheadClock = {
  masterStep: -1,
  clockStep: -1,
};

// The engine reports the master playhead modulo stepCount. Preserve the
// elapsed clock across that wrap so a 3-step track keeps moving 0,1,2 while a
// 5-step master wraps 4→0. Worklet reports are frequent enough that a normal
// update cannot skip a complete master cycle.
export function advancePlayheadClock(
  previous: PlayheadClock,
  masterStep: number,
  masterLength: number,
): PlayheadClock {
  if (masterStep < 0) return STOPPED_PLAYHEAD;
  if (previous.masterStep < 0) {
    return { masterStep, clockStep: masterStep };
  }

  const length = Math.max(1, Math.round(masterLength));
  const delta = (masterStep - previous.masterStep + length) % length;
  if (delta === 0) {
    // A different raw step with the same modulo position means the master
    // length changed underneath the audible snapshot. Rebase: SetStepCount
    // intentionally synchronizes every track to that master position.
    return masterStep === previous.masterStep
      ? previous
      : { masterStep, clockStep: masterStep };
  }

  return { masterStep, clockStep: previous.clockStep + delta };
}

export function cycleVelocity(velocity: number): number {
  if (velocity === 0) return VEL_NORMAL;
  if (velocity < VEL_ACCENT) return VEL_ACCENT;
  return 0;
}

export function velocityName(velocity: number): string {
  if (velocity === 0) return "off";
  if (velocity >= VEL_ACCENT) return "accent";
  // The v15 share format stores one byte, so the 0.7 detent returns as
  // 179/255. Treat that half-byte neighbourhood as the named normal hit.
  if (Math.abs(velocity - VEL_NORMAL) <= 0.5 / 255) return "on";
  return `${Math.round(velocity * 100)}%`;
}

export function flatToVisual(flat: ArrayLike<number>): number[][] {
  return TRACKS.map((_, row) =>
    Array.from(
      { length: STEP_CAPACITY },
      (_, col) => flat[TRACK_INDEX[row] * STEP_CAPACITY + col] ?? 0,
    ),
  );
}

export function patternsEqual(
  left: ArrayLike<number>,
  right: ArrayLike<number>,
): boolean {
  if (left.length !== right.length) return false;
  for (let index = 0; index < left.length; index++) {
    if (left[index] !== right[index]) return false;
  }
  return true;
}

export function velocityFromPointer(
  clientY: number,
  top: number,
  height: number,
): number {
  if (!Number.isFinite(height) || height <= 0) return 0;
  const value = 1 - (clientY - top) / height;
  return Math.max(0, Math.min(1, Math.round(value * 100) / 100));
}

export function nudgeVelocity(velocity: number, direction: 1 | -1): number {
  const next = Math.round((velocity + direction * 0.05) * 100) / 100;
  return Math.max(0, Math.min(1, next));
}

export function fillEuclidTrack(
  pattern: ArrayLike<number>,
  track: number,
  rhythm: readonly boolean[],
): number[] {
  const next = Array.from(pattern);
  if (track < 0 || track >= TRACK_COUNT) return next;

  const offset = track * STEP_CAPACITY;
  for (let step = 0; step < STEP_CAPACITY; step++) {
    next[offset + step] = rhythm[step] ? VEL_NORMAL : 0;
  }
  return next;
}

export function cellIndex(track: number, step: number): number {
  return track * STEP_CAPACITY + step;
}
