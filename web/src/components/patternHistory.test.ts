import { describe, expect, it } from "vitest";
import { createDefaultPatternBankState } from "../engine/engineState";
import {
  EMPTY_PATTERN_HISTORY,
  recordPatternBank,
  redoPatternBank,
  undoPatternBank,
} from "./patternHistory";

describe("pattern bank history", () => {
  it("records complete rhythm snapshots and clears redo", () => {
    const current = createDefaultPatternBankState();
    const next = createDefaultPatternBankState();
    next.pattern[0] = 1;
    next.cellProbabilities[0] = 0.5;
    const history = recordPatternBank(EMPTY_PATTERN_HISTORY, current, next);
    expect(history.past[0].pattern[0]).toBe(0);
    expect(history.future).toEqual([]);
    expect(recordPatternBank(history, next, next)).toBe(history);
  });

  it("undoes and redoes without aliasing typed arrays", () => {
    const before = createDefaultPatternBankState();
    const after = createDefaultPatternBankState();
    after.pattern[0] = 1;
    const recorded = recordPatternBank(EMPTY_PATTERN_HISTORY, before, after);
    const undone = undoPatternBank(recorded, after);
    expect(undone.bank?.pattern[0]).toBe(0);
    expect(undone.history.future[0].pattern[0]).toBe(1);

    undone.bank!.pattern[0] = 0.25;
    expect(recorded.past[0].pattern[0]).toBe(0);

    const redone = redoPatternBank(undone.history, undone.bank!);
    expect(redone.bank?.pattern[0]).toBe(1);
    expect(redone.history.future).toEqual([]);
  });

  it("is a no-op at either end of history", () => {
    const bank = createDefaultPatternBankState();
    expect(undoPatternBank(EMPTY_PATTERN_HISTORY, bank).bank).toBeNull();
    expect(redoPatternBank(EMPTY_PATTERN_HISTORY, bank).bank).toBeNull();
  });
});
