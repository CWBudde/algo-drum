import { describe, expect, it } from "vitest";
import { mutate } from "./mutate";
import {
  BASS_TRACK,
  PATTERN_SIZE,
  VEL_ACCENT,
  VEL_NORMAL,
  VEL_OFF,
  index,
} from "./pattern";

// Deterministic PRNG (mulberry32) so mutation is reproducible across seeds.
function rngFromSeed(seed: number): () => number {
  let a = seed >>> 0;
  return () => {
    a |= 0;
    a = (a + 0x6d2b79f5) | 0;
    let t = Math.imul(a ^ (a >>> 15), 1 | a);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

const countHits = (p: number[]): number => p.filter((v) => v > VEL_OFF).length;

function emptyPattern(): number[] {
  return new Array<number>(PATTERN_SIZE).fill(VEL_OFF);
}

// A representative full pattern: bass on the downbeats, snare on 2 & 4, hats.
function seedPattern(): number[] {
  const p = emptyPattern();
  p[index(0, 0)] = VEL_ACCENT;
  p[index(0, 8)] = VEL_NORMAL;
  p[index(1, 4)] = VEL_NORMAL;
  p[index(1, 12)] = VEL_ACCENT;
  for (let s = 0; s < 16; s += 2) p[index(2, s)] = VEL_NORMAL;
  return p;
}

const VALID = new Set([VEL_OFF, VEL_NORMAL, VEL_ACCENT]);

describe("mutate", () => {
  it("returns a new array without modifying the input", () => {
    const input = seedPattern();
    const snapshot = input.slice();
    const out = mutate(input, { rng: rngFromSeed(1) });
    expect(out).not.toBe(input);
    expect(input).toEqual(snapshot);
  });

  it("always outputs a full-length pattern of valid velocities", () => {
    for (let seed = 0; seed < 200; seed++) {
      const out = mutate(seedPattern(), { rng: rngFromSeed(seed) });
      expect(out).toHaveLength(PATTERN_SIZE);
      for (const v of out) expect(VALID.has(v)).toBe(true);
    }
  });

  it("never empties the pattern (even from a single hit)", () => {
    const single = emptyPattern();
    single[index(BASS_TRACK, 0)] = VEL_NORMAL;
    for (let seed = 0; seed < 300; seed++) {
      expect(
        countHits(mutate(single, { rng: rngFromSeed(seed) })),
      ).toBeGreaterThan(0);
    }
  });

  it("never empties a fuller pattern", () => {
    for (let seed = 0; seed < 300; seed++) {
      expect(
        countHits(mutate(seedPattern(), { rng: rngFromSeed(seed) })),
      ).toBeGreaterThan(0);
    }
  });

  it("preserves the lone downbeat bass hit", () => {
    // Bass at step 0 is the only bass hit; other tracks may be empty or busy.
    const loneBass = emptyPattern();
    loneBass[index(BASS_TRACK, 0)] = VEL_ACCENT;
    for (let seed = 0; seed < 300; seed++) {
      const out = mutate(loneBass, { rng: rngFromSeed(seed) });
      expect(out[index(BASS_TRACK, 0)]).toBeGreaterThan(VEL_OFF);
    }

    const withOthers = seedPattern();
    // Remove the second bass hit so step 0 is the lone bass downbeat again.
    withOthers[index(BASS_TRACK, 8)] = VEL_OFF;
    for (let seed = 0; seed < 300; seed++) {
      const out = mutate(withOthers, { rng: rngFromSeed(seed) });
      expect(out[index(BASS_TRACK, 0)]).toBeGreaterThan(VEL_OFF);
    }
  });

  it("keeps hits within the active step window", () => {
    const stepCount = 8;
    for (let seed = 0; seed < 100; seed++) {
      const out = mutate(seedPattern(), { rng: rngFromSeed(seed), stepCount });
      // Any hit the mutation *adds or moves* stays in [0, stepCount); seeded
      // hits already sit within 0..15 but the seed pattern here is all < 16,
      // and mutation only touches steps < stepCount, so no new hit appears at
      // step >= stepCount on an initially-clear column beyond the window.
      for (let track = 0; track < 5; track++) {
        for (let step = stepCount; step < 16; step++) {
          // seedPattern places hats up to step 14; only assert on tracks/steps
          // the seed leaves off within the beyond-window region.
          if (seedPattern()[index(track, step)] === VEL_OFF) {
            expect(out[index(track, step)]).toBe(VEL_OFF);
          }
        }
      }
    }
  });
});
