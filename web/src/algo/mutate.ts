// Pattern mutate / evolve — a small musical random walk on the current pattern.
//
// Each call nudges the pattern a little: it shifts a hit to an adjacent step on
// one or two tracks, toggles a single cell, and occasionally flips a normal hit
// to an accent (or back). It is biased to stay musical — it never empties the
// pattern and protects the downbeat bass hit. Pure and dependency-free.

import {
  BASS_TRACK,
  PATTERN_SIZE,
  STEP_CAPACITY,
  TRACK_COUNT,
  VEL_ACCENT,
  VEL_NORMAL,
  VEL_OFF,
  index,
} from "./pattern";

export interface MutateOptions {
  // Active pattern length; mutation stays within [0, stepCount). Default 16.
  stepCount?: number;
  // Injectable RNG for deterministic tests. Default Math.random.
  rng?: () => number;
}

function countHits(pattern: number[]): number {
  let n = 0;
  for (const v of pattern) if (v > VEL_OFF) n++;
  return n;
}

// shiftOneHit moves a random hit on `track` to an adjacent free step.
function shiftOneHit(
  pattern: number[],
  track: number,
  stepCount: number,
  rng: () => number,
): void {
  const hits: number[] = [];
  for (let step = 0; step < stepCount; step++) {
    if (pattern[index(track, step)] > VEL_OFF) hits.push(step);
  }
  if (hits.length === 0) return;

  const step = hits[Math.floor(rng() * hits.length)];
  const dir = rng() < 0.5 ? -1 : 1;
  const target = (step + dir + stepCount) % stepCount;

  // Don't stomp an existing hit; skip the move if the neighbour is taken.
  if (pattern[index(track, target)] > VEL_OFF) return;

  // Protect the only downbeat bass hit from wandering off step 0.
  if (track === BASS_TRACK && step === 0 && countBassHits(pattern) === 1) {
    return;
  }

  pattern[index(track, target)] = pattern[index(track, step)];
  pattern[index(track, step)] = VEL_OFF;
}

function countBassHits(pattern: number[]): number {
  let n = 0;
  for (let step = 0; step < STEP_CAPACITY; step++) {
    if (pattern[index(BASS_TRACK, step)] > VEL_OFF) n++;
  }
  return n;
}

// mutate returns a new pattern one random-walk step away from the input; the
// input is not modified.
export function mutate(pattern: number[], opts: MutateOptions = {}): number[] {
  const stepCount = Math.max(1, Math.min(STEP_CAPACITY, opts.stepCount ?? 16));
  const rng = opts.rng ?? Math.random;
  const next = pattern.slice(0, PATTERN_SIZE);
  while (next.length < PATTERN_SIZE) next.push(VEL_OFF);

  // Shift a hit on one or two tracks.
  const moves = rng() < 0.5 ? 1 : 2;
  for (let m = 0; m < moves; m++) {
    shiftOneHit(next, Math.floor(rng() * TRACK_COUNT), stepCount, rng);
  }

  // Toggle a single cell, but never remove the last hit in the pattern nor the
  // lone downbeat bass hit.
  const track = Math.floor(rng() * TRACK_COUNT);
  const step = Math.floor(rng() * stepCount);
  const cell = index(track, step);
  if (next[cell] > VEL_OFF) {
    const isLoneBassDownbeat =
      track === BASS_TRACK && step === 0 && countBassHits(next) === 1;
    if (countHits(next) > 1 && !isLoneBassDownbeat) next[cell] = VEL_OFF;
  } else {
    next[cell] = VEL_NORMAL;
  }

  // Occasionally flip a random existing hit between normal and accent.
  if (rng() < 0.35) {
    const hits: number[] = [];
    for (let t = 0; t < TRACK_COUNT; t++) {
      for (let s = 0; s < stepCount; s++) {
        if (next[index(t, s)] > VEL_OFF) hits.push(index(t, s));
      }
    }
    if (hits.length > 0) {
      const cellToFlip = hits[Math.floor(rng() * hits.length)];
      next[cellToFlip] =
        next[cellToFlip] >= VEL_ACCENT ? VEL_NORMAL : VEL_ACCENT;
    }
  }

  // Safety net: never hand back an empty pattern.
  if (countHits(next) === 0) next[index(BASS_TRACK, 0)] = VEL_ACCENT;

  return next;
}
