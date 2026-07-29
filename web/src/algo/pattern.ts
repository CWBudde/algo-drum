// Shared constants for the algorithmic pattern modules (euclid apply, mutate,
// presets, persistence). Patterns are represented as a flat, engine-major array
// of length TRACK_COUNT × STEP_CAPACITY where index = track·STEP_CAPACITY + step
// and each value is a velocity in [0, 1]. This mirrors the Go engine's
// setPattern/getPattern layout so the flat form round-trips without conversion.

export const TRACK_COUNT = 7;
export const STEP_CAPACITY = 16;
export const PATTERN_SIZE = TRACK_COUNT * STEP_CAPACITY;

// Engine track index of the bass drum (see the track table in AGENTS.md).
export const BASS_TRACK = 0;

// The three velocity states a cell can hold, matching the UI's click cycle.
export const VEL_OFF = 0;
export const VEL_NORMAL = 0.7;
export const VEL_ACCENT = 1.0;

// index maps a (track, step) pair to its flat array index.
export function index(track: number, step: number): number {
  return track * STEP_CAPACITY + step;
}
