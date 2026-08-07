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
      bank: 1,
      track: 1,
      step: 3,
      value: 0.7,
    });

    expect(next.banks[1].pattern[19]).toBeCloseTo(0.7);
    expect(state.banks[1].pattern[19]).toBe(0);
    expect(next.banks[1].pattern).not.toBe(state.banks[1].pattern);
    expect(next.banks[0]).toBe(state.banks[0]);
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
      bank: 2,
      track: 2,
      step: 4,
      value: 0.35,
    });
    const condition = reduceDrumState(probability, {
      type: "cellCondition",
      bank: 2,
      track: 2,
      step: 4,
      value: 3,
    });
    const length = reduceDrumState(condition, {
      type: "trackLength",
      bank: 2,
      track: 2,
      value: 7,
    });
    const fill = reduceDrumState(length, { type: "fillMode", value: true });

    expect(fill.banks[2].cellProbabilities[36]).toBeCloseTo(0.35);
    expect(fill.banks[2].cellConditions[36]).toBe(3);
    expect(fill.banks[2].trackLengths[2]).toBe(7);
    expect(fill.fillMode).toBe(true);
    expect(state.banks[2].cellProbabilities[36]).toBe(1);
  });

  it("keeps per-track lengths aligned when the master length changes", () => {
    const state = defaultEngineState();
    const next = reduceDrumState(state, {
      type: "stepCount",
      bank: 3,
      value: 9,
    });

    expect(next.banks[3].stepCount).toBe(9);
    expect(Array.from(next.banks[3].trackLengths)).toEqual(
      new Array(7).fill(9),
    );
    expect(next.banks[0].stepCount).toBe(16);
  });

  it("updates local humanize, ratchets, and chain state without aliasing", () => {
    const state = defaultEngineState();
    const humanized = reduceDrumState(state, {
      type: "cellHumanize",
      bank: 1,
      track: 0,
      step: 2,
      value: 0.4,
    });
    const chained = reduceDrumState(humanized, {
      type: "chain",
      value: Uint8Array.from([0, 1, 1, 3]),
    });
    const ratcheted = reduceDrumState(chained, {
      type: "cellRepeats",
      bank: 1,
      track: 0,
      step: 2,
      value: 4,
    });

    expect(humanized.banks[1].cellHumanize[2]).toBeCloseTo(0.4);
    expect(state.banks[1].cellHumanize[2]).toBe(1);
    expect(Array.from(chained.chain)).toEqual([0, 1, 1, 3]);
    expect(chained.chain).not.toBe(state.chain);
    expect(ratcheted.banks[1].cellRepeats[2]).toBe(4);
    expect(chained.banks[1].cellRepeats[2]).toBe(1);
  });
});
