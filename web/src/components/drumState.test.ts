import { describe, expect, it } from "vitest";
import { defaultEngineState, reduceDrumState } from "./drumState";

describe("reduceDrumState", () => {
  it("updates engine-major track state without mutating the previous snapshot", () => {
    const state = defaultEngineState();
    const next = reduceDrumState(state, {
      type: "volume",
      track: 4,
      value: 0.25,
    });

    expect(next.tracks[4].volume).toBe(0.25);
    expect(state.tracks[4].volume).toBe(0.75);
    expect(next.tracks[3]).toBe(state.tracks[3]);
  });

  it("copies typed arrays for optimistic edits", () => {
    const state = defaultEngineState();
    const next = reduceDrumState(state, {
      type: "cell",
      track: 1,
      step: 3,
      value: 0.7,
    });

    expect(next.pattern[19]).toBeCloseTo(0.7);
    expect(state.pattern[19]).toBe(0);
    expect(next.pattern).not.toBe(state.pattern);
  });

  it("keeps the physical parameter banks attached to their Tom track", () => {
    const state = defaultEngineState();
    const next = reduceDrumState(state, {
      type: "physicalTomParam",
      track: 5,
      index: 2,
      value: 0.9,
    });

    expect(next.tracks[5].tom?.physicalParams[2]).toBeCloseTo(0.9);
    expect(state.tracks[5].tom?.physicalParams[2]).not.toBeCloseTo(0.9);
    expect(next.tracks[3].tom).toBe(state.tracks[3].tom);
  });

  it("replaces optimistic state with an authoritative snapshot", () => {
    const state = defaultEngineState();
    const authoritative = { ...state, tempoBpm: 300 };

    expect(
      reduceDrumState(state, { type: "replace", state: authoritative }),
    ).toBe(authoritative);
  });
});
