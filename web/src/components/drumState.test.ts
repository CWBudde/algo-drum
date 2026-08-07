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

  it("updates conditional-step and polymeter controls immutably", () => {
    const state = defaultEngineState();
    const probability = reduceDrumState(state, {
      type: "cellProbability",
      track: 2,
      step: 4,
      value: 0.35,
    });
    const condition = reduceDrumState(probability, {
      type: "cellCondition",
      track: 2,
      step: 4,
      value: 3,
    });
    const length = reduceDrumState(condition, {
      type: "trackLength",
      track: 2,
      value: 7,
    });
    const fill = reduceDrumState(length, { type: "fillMode", value: true });

    expect(fill.cellProbabilities[36]).toBeCloseTo(0.35);
    expect(fill.cellConditions[36]).toBe(3);
    expect(fill.trackLengths[2]).toBe(7);
    expect(fill.fillMode).toBe(true);
    expect(state.cellProbabilities[36]).toBe(1);
  });

  it("keeps per-track lengths aligned when the master length changes", () => {
    const state = defaultEngineState();
    const next = reduceDrumState(state, { type: "stepCount", value: 9 });

    expect(next.stepCount).toBe(9);
    expect(Array.from(next.trackLengths)).toEqual(new Array(7).fill(9));
  });
});
