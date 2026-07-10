import { describe, expect, it } from "vitest";
import { PRESETS, emptyPattern, presetToFlat } from "./presets";
import { PATTERN_SIZE, VEL_ACCENT, VEL_NORMAL, VEL_OFF } from "./pattern";

const VALID = new Set([VEL_OFF, VEL_NORMAL, VEL_ACCENT]);

describe("presets", () => {
  it("ships a non-empty catalogue with unique names", () => {
    expect(PRESETS.length).toBeGreaterThan(0);
    const names = PRESETS.map((p) => p.name);
    expect(new Set(names).size).toBe(names.length);
  });

  it.each(PRESETS.map((p) => [p.name, p] as const))(
    "%s compiles to a full-length pattern of valid velocities",
    (_name, preset) => {
      const flat = presetToFlat(preset);
      expect(flat).toHaveLength(PATTERN_SIZE);
      for (const v of flat) expect(VALID.has(v)).toBe(true);
      // A preset should actually place some hits.
      expect(flat.some((v) => v > VEL_OFF)).toBe(true);
    },
  );

  it.each(PRESETS.map((p) => [p.name, p] as const))(
    "%s row strings are all 16 steps long",
    (_name, preset) => {
      for (const row of Object.values(preset.rows)) {
        expect(row).toHaveLength(16);
      }
    },
  );

  it("maps row characters to the right velocities (Rock)", () => {
    const rock = PRESETS.find((p) => p.name === "Rock");
    expect(rock).toBeDefined();
    const flat = presetToFlat(rock!);
    // Bass row "X.......X.......": accent at steps 0 and 8, off elsewhere.
    expect(flat[0]).toBe(VEL_ACCENT);
    expect(flat[8]).toBe(VEL_ACCENT);
    expect(flat[1]).toBe(VEL_OFF);
    // HiHat row (track 2) "x.x.x..." normal hits on even steps.
    expect(flat[2 * 16 + 0]).toBe(VEL_NORMAL);
    expect(flat[2 * 16 + 1]).toBe(VEL_OFF);
  });

  it("emptyPattern is all-off and full length", () => {
    const empty = emptyPattern();
    expect(empty).toHaveLength(PATTERN_SIZE);
    expect(empty.every((v) => v === VEL_OFF)).toBe(true);
  });
});
