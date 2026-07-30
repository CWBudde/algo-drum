import { describe, expect, it } from "vitest";
import { PHYSICAL_TOM_PARAMS } from "../engine/voiceParams";
import {
  DEFAULT_PHYSICAL_TOM_PRESET,
  PHYSICAL_TOM_PRESETS,
  presetValues,
} from "./physicalTomPresets";

describe("physical tom presets", () => {
  it("expands the default preset to every knob's own default", () => {
    expect(
      presetValues(DEFAULT_PHYSICAL_TOM_PRESET, PHYSICAL_TOM_PARAMS),
    ).toEqual(PHYSICAL_TOM_PARAMS.map((spec) => spec.default));
  });

  it("names only parameters that exist", () => {
    const ids = new Set(PHYSICAL_TOM_PARAMS.map((spec) => spec.id));
    for (const preset of PHYSICAL_TOM_PRESETS) {
      for (const id of Object.keys(preset.values)) {
        expect(ids, `${preset.name} names ${id}`).toContain(id);
      }
    }
  });

  it("keeps every preset inside the normalized range", () => {
    for (const preset of PHYSICAL_TOM_PRESETS) {
      for (const value of Object.values(preset.values)) {
        expect(value).toBeGreaterThanOrEqual(0);
        expect(value).toBeLessThanOrEqual(1);
      }
    }
  });

  it("returns one value per spec, in spec order", () => {
    const values = presetValues(
      {
        name: "t",
        description: "t",
        values: { [PHYSICAL_TOM_PARAMS[2].id]: 0.25 },
      },
      PHYSICAL_TOM_PARAMS,
    );

    expect(values).toHaveLength(PHYSICAL_TOM_PARAMS.length);
    expect(values[2]).toBe(0.25);
    expect(values[0]).toBe(PHYSICAL_TOM_PARAMS[0].default);
  });

  it("ignores an unknown id rather than throwing", () => {
    // A share link or a stale table naming a parameter this build does not
    // have must degrade to the parameters it does have.
    const values = presetValues(
      { name: "t", description: "t", values: { "physicalTom.notAThing": 0.5 } },
      PHYSICAL_TOM_PARAMS,
    );

    expect(values).toEqual(PHYSICAL_TOM_PARAMS.map((spec) => spec.default));
  });

  it("clamps an out-of-range value", () => {
    const id = PHYSICAL_TOM_PARAMS[0].id;

    expect(
      presetValues(
        { name: "t", description: "t", values: { [id]: 4 } },
        PHYSICAL_TOM_PARAMS,
      )[0],
    ).toBe(1);
    expect(
      presetValues(
        { name: "t", description: "t", values: { [id]: -4 } },
        PHYSICAL_TOM_PARAMS,
      )[0],
    ).toBe(0);
  });
});
