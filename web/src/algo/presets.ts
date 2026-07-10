// Classic 16-step preset patterns.
//
// Each preset is written as one 16-character string per voice: '.' = off,
// 'x' = normal hit, 'X' = accent. Voices are keyed by engine track index
// (0 Bass, 1 Snare, 2 HiHat, 3 Tom, 4 Cymbal). Patterns compile to the flat,
// engine-major velocity array the engine consumes. Pure and dependency-free.

import {
  PATTERN_SIZE,
  STEP_CAPACITY,
  VEL_ACCENT,
  VEL_NORMAL,
  VEL_OFF,
  index,
} from "./pattern";

type VoiceRows = Partial<Record<number, string>>;

export interface Preset {
  name: string;
  rows: VoiceRows;
}

// Track order per row string: Bass, Snare, HiHat, Tom, Cymbal.
export const PRESETS: Preset[] = [
  {
    name: "Rock",
    rows: {
      0: "X.......X.......",
      1: "....X.......X...",
      2: "x.x.x.x.x.x.x.x.",
    },
  },
  {
    name: "House",
    rows: {
      0: "X...X...X...X...",
      1: "....X.......X...",
      2: "..x...x...x...x.",
    },
  },
  {
    name: "Breakbeat",
    rows: {
      0: "X.....x...X.....",
      1: "....X..x....X.x.",
      2: "x.xxx.x.x.xxx.x.",
    },
  },
  {
    name: "Hip-Hop",
    rows: {
      0: "X......x..X.....",
      1: "....X.......X...",
      2: "x.x.x.x.x.x.x.x.",
    },
  },
  {
    name: "Techno",
    rows: {
      0: "X...X...X...X...",
      1: "....x.......x...",
      2: "..x...x...x...x.",
      4: "........X.......",
    },
  },
  {
    name: "Funk",
    rows: {
      0: "X..x..X...x..X..",
      1: "....X.......X..x",
      2: "xxxxxxxxxxxxxxxx",
    },
  },
];

function charToVelocity(ch: string): number {
  if (ch === "X") return VEL_ACCENT;
  if (ch === "x") return VEL_NORMAL;
  return VEL_OFF;
}

// presetToFlat compiles a preset into a flat, engine-major velocity array of
// length PATTERN_SIZE (cells not covered by the preset are left off).
export function presetToFlat(preset: Preset): number[] {
  const flat = new Array<number>(PATTERN_SIZE).fill(VEL_OFF);

  for (const [trackKey, row] of Object.entries(preset.rows)) {
    const track = Number(trackKey);
    if (!row) continue;

    for (let step = 0; step < STEP_CAPACITY && step < row.length; step++) {
      flat[index(track, step)] = charToVelocity(row[step]);
    }
  }

  return flat;
}

// emptyPattern returns an all-off flat pattern (for the CLEAR button).
export function emptyPattern(): number[] {
  return new Array<number>(PATTERN_SIZE).fill(VEL_OFF);
}
