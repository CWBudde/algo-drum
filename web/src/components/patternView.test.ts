import { describe, expect, it } from "vitest";
import { PATTERN_SIZE, STEP_CAPACITY } from "../algo/pattern";
import {
  STOPPED_PLAYHEAD,
  advancePlayheadClock,
  cycleVelocity,
  fillEuclidTrack,
  flatToVisual,
  moveGridFocus,
  nudgeVelocity,
  patternsEqual,
  velocityFromPointer,
  velocityName,
} from "./patternView";

describe("pattern view helpers", () => {
  it("preserves the tri-state click cycle", () => {
    expect(cycleVelocity(0)).toBe(0.7);
    expect(cycleVelocity(0.37)).toBe(1);
    expect(cycleVelocity(1)).toBe(0);
  });

  it("names continuous velocities without losing accent/off semantics", () => {
    expect(velocityName(0)).toBe("off");
    expect(velocityName(0.7)).toBe("on");
    expect(velocityName(0.35)).toBe("35%");
    expect(velocityName(1)).toBe("accent");
  });

  it("moves grid focus by cell, row, and row or grid edge", () => {
    expect(moveGridFocus({ row: 3, col: 7 }, "ArrowLeft", false)).toEqual({
      row: 3,
      col: 6,
    });
    expect(moveGridFocus({ row: 3, col: 7 }, "ArrowDown", false)).toEqual({
      row: 4,
      col: 7,
    });
    expect(moveGridFocus({ row: 3, col: 7 }, "Home", false)).toEqual({
      row: 3,
      col: 0,
    });
    expect(moveGridFocus({ row: 3, col: 7 }, "End", true)).toEqual({
      row: 6,
      col: 15,
    });
  });

  it("keeps grid focus inside the pattern", () => {
    expect(moveGridFocus({ row: 0, col: 0 }, "ArrowLeft", false)).toEqual({
      row: 0,
      col: 0,
    });
    expect(moveGridFocus({ row: 6, col: 15 }, "ArrowDown", false)).toEqual({
      row: 6,
      col: 15,
    });
    expect(moveGridFocus({ row: 0, col: 0 }, "PageDown", false)).toBeNull();
  });

  it("maps engine-major tracks into the reversed visual order", () => {
    const flat = Array.from({ length: PATTERN_SIZE }, (_, index) => index);
    const visual = flatToVisual(flat);
    expect(visual[0][0]).toBe(4 * STEP_CAPACITY);
    expect(visual[6][15]).toBe(15);
  });

  it("maps and nudges continuous velocities at bounded precision", () => {
    expect(velocityFromPointer(100, 100, 40)).toBe(1);
    expect(velocityFromPointer(120, 100, 40)).toBe(0.5);
    expect(velocityFromPointer(150, 100, 40)).toBe(0);
    expect(nudgeVelocity(0.7, 1)).toBe(0.75);
    expect(nudgeVelocity(0.02, -1)).toBe(0);
  });

  it("fills only the selected Euclidean track", () => {
    const pattern = new Array(PATTERN_SIZE).fill(0.25);
    const next = fillEuclidTrack(pattern, 1, [true, false, true]);
    expect(next.slice(STEP_CAPACITY, STEP_CAPACITY + 4)).toEqual([
      0.7, 0, 0.7, 0,
    ]);
    expect(next[0]).toBe(0.25);
    expect(pattern[STEP_CAPACITY]).toBe(0.25);
  });

  it("compares typed and ordinary arrays", () => {
    expect(patternsEqual([0, 0.5], Float32Array.from([0, 0.5]))).toBe(true);
    expect(patternsEqual([0, 0.5], [0, 0.7])).toBe(false);
  });

  it("keeps an absolute clock across master wraps for polymetric rows", () => {
    let playhead = STOPPED_PLAYHEAD;
    for (const masterStep of [0, 1, 2, 3, 4, 0, 1]) {
      playhead = advancePlayheadClock(playhead, masterStep, 5);
    }

    expect(playhead).toEqual({ masterStep: 1, clockStep: 6 });
    expect(playhead.clockStep % 3).toBe(0);
    expect(advancePlayheadClock(playhead, -1, 5)).toBe(STOPPED_PLAYHEAD);
  });

  it("rebases the clock when a master-length edit changes the raw step", () => {
    expect(
      advancePlayheadClock({ masterStep: 10, clockStep: 26 }, 2, 4),
    ).toEqual({ masterStep: 2, clockStep: 2 });
  });
});
