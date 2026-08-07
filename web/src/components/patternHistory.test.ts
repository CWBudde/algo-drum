import { describe, expect, it } from "vitest";
import {
  EMPTY_PATTERN_HISTORY,
  recordPattern,
  redoPattern,
  undoPattern,
} from "./patternHistory";

describe("pattern history", () => {
  it("records destructive replacements and clears redo", () => {
    const history = recordPattern(EMPTY_PATTERN_HISTORY, [0, 0], [1, 0]);
    expect(Array.from(history.past[0])).toEqual([0, 0]);
    expect(history.future).toEqual([]);
    expect(recordPattern(history, [1, 0], [1, 0])).toBe(history);
  });

  it("undoes and redoes complete pattern snapshots", () => {
    const recorded = recordPattern(EMPTY_PATTERN_HISTORY, [0, 0], [1, 0]);
    const undone = undoPattern(recorded, [1, 0.5]);
    expect(Array.from(undone.pattern ?? [])).toEqual([0, 0]);
    expect(Array.from(undone.history.future[0])).toEqual([1, 0.5]);

    const redone = redoPattern(undone.history, [0, 0]);
    expect(Array.from(redone.pattern ?? [])).toEqual([1, 0.5]);
    expect(redone.history.future).toEqual([]);
  });

  it("is a no-op at either end of history", () => {
    expect(undoPattern(EMPTY_PATTERN_HISTORY, [1]).pattern).toBeNull();
    expect(redoPattern(EMPTY_PATTERN_HISTORY, [1]).pattern).toBeNull();
  });
});
