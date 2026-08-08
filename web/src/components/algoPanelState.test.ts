import { describe, expect, it } from "vitest";
import { PRESETS, presetToFlat } from "../algo/presets";
import { matchingPresetIndex, normalizeIntegerDraft } from "./algoPanelState";

describe("algorithm panel state", () => {
  it("matches presets after exact Float32 normalization", () => {
    const funkIndex = PRESETS.findIndex((preset) => preset.name === "Funk");
    const funk = PRESETS[funkIndex];
    expect(funk).toBeDefined();

    const sourcePattern = presetToFlat(funk!);
    expect(matchingPresetIndex(sourcePattern)).toBe(funkIndex);
    expect(
      matchingPresetIndex(Array.from(Float32Array.from(sourcePattern))),
    ).toBe(funkIndex);
  });

  it("follows external edits, bank changes, and exact restoration", () => {
    const rock = presetToFlat(PRESETS[0]!);
    const house = presetToFlat(PRESETS[1]!);
    const edited = Float32Array.from(rock);
    edited[0] = 0.5;

    expect(matchingPresetIndex(rock)).toBe(0);
    expect(matchingPresetIndex(edited)).toBe(-1);
    expect(matchingPresetIndex(house)).toBe(1);
    expect(matchingPresetIndex(rock)).toBe(0);
  });

  it("normalizes integer drafts only when asked to commit", () => {
    expect(normalizeIntegerDraft("", 0, 16)).toBe(0);
    expect(normalizeIntegerDraft("-", 0, 15)).toBe(0);
    expect(normalizeIntegerDraft("3.6", 0, 16)).toBe(4);
    expect(normalizeIntegerDraft("99", 0, 16)).toBe(16);
    expect(normalizeIntegerDraft("-3", 0, 15)).toBe(0);
  });
});
