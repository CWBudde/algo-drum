import { describe, expect, it } from "vitest";
import { STEP_CAPACITY } from "../algo/pattern";
import { createDefaultPatternBankState } from "../engine/engineState";
import { clearRow, copyBar, copyRow, rotateRow } from "./rowOperations";

function seedTrigger(
  bank: ReturnType<typeof createDefaultPatternBankState>,
  track: number,
  step: number,
  value: number,
): void {
  const index = track * STEP_CAPACITY + step;
  bank.pattern[index] = value;
  bank.cellProbabilities[index] = value / 2;
  bank.cellHumanize[index] = value / 3;
  bank.cellConditions[index] = Math.round(value * 4);
  bank.cellRepeats[index] = Math.max(1, Math.round(value * 3));
}

function triggerAt(
  bank: ReturnType<typeof createDefaultPatternBankState>,
  track: number,
  step: number,
): number[] {
  const index = track * STEP_CAPACITY + step;
  return [
    bank.pattern[index]!,
    bank.cellProbabilities[index]!,
    bank.cellHumanize[index]!,
    bank.cellConditions[index]!,
    bank.cellRepeats[index]!,
  ];
}

describe("row operations", () => {
  it("clears complete trigger records while preserving row length", () => {
    const bank = createDefaultPatternBankState();
    bank.trackLengths[2] = 7;
    seedTrigger(bank, 2, 3, 1);

    const cleared = clearRow(bank, 2);
    expect(triggerAt(cleared, 2, 3)).toEqual([0, 1, 1, 0, 1]);
    expect(cleared.trackLengths[2]).toBe(7);
    expect(triggerAt(bank, 2, 3)[0]).toBe(1);
  });

  it("rotates complete records only inside the active loop", () => {
    const bank = createDefaultPatternBankState();
    bank.trackLengths[1] = 3;
    seedTrigger(bank, 1, 0, 0.3);
    seedTrigger(bank, 1, 1, 0.6);
    seedTrigger(bank, 1, 2, 0.9);
    seedTrigger(bank, 1, 3, 1);

    const left = rotateRow(bank, 1, "left");
    expect(triggerAt(left, 1, 0)).toEqual(triggerAt(bank, 1, 1));
    expect(triggerAt(left, 1, 1)).toEqual(triggerAt(bank, 1, 2));
    expect(triggerAt(left, 1, 2)).toEqual(triggerAt(bank, 1, 0));
    expect(triggerAt(left, 1, 3)).toEqual(triggerAt(bank, 1, 3));

    const right = rotateRow(bank, 1, "right");
    expect(triggerAt(right, 1, 0)).toEqual(triggerAt(bank, 1, 2));
    expect(triggerAt(right, 1, 1)).toEqual(triggerAt(bank, 1, 0));
  });

  it("copies every row field and the source loop length", () => {
    const bank = createDefaultPatternBankState();
    bank.trackLengths[0] = 5;
    bank.trackLengths[4] = 12;
    for (let step = 0; step < STEP_CAPACITY; step++) {
      seedTrigger(bank, 0, step, (step + 1) / STEP_CAPACITY);
    }

    const copied = copyRow(bank, 0, 4);
    expect(copied.trackLengths[4]).toBe(5);
    for (let step = 0; step < STEP_CAPACITY; step++) {
      expect(triggerAt(copied, 4, step)).toEqual(triggerAt(bank, 0, step));
    }
  });

  it("uses the neutral humanize multiplier when a source record is missing", () => {
    const bank = createDefaultPatternBankState();
    bank.cellHumanize = new Float32Array(STEP_CAPACITY).fill(0.25);

    const copied = copyRow(bank, 6, 0);
    expect(copied.cellHumanize[0]).toBe(1);
  });

  it("copies a four-step bar as complete records without changing length", () => {
    const bank = createDefaultPatternBankState();
    bank.trackLengths[3] = 11;
    for (let step = 0; step < 4; step++) {
      seedTrigger(bank, 3, step, (step + 1) / 4);
    }

    const copied = copyBar(bank, 3, 0, 2);
    for (let step = 0; step < 4; step++) {
      expect(triggerAt(copied, 3, 8 + step)).toEqual(triggerAt(bank, 3, step));
    }
    expect(copied.trackLengths[3]).toBe(11);
  });
});
