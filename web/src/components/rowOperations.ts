import { STEP_CAPACITY, TRACK_COUNT } from "../algo/pattern";
import type { PatternBankState } from "../engine/engineState";

const BAR_LENGTH = 4;
const BAR_COUNT = STEP_CAPACITY / BAR_LENGTH;

function validTrack(track: number): boolean {
  return Number.isInteger(track) && track >= 0 && track < TRACK_COUNT;
}

function validBar(bar: number): boolean {
  return Number.isInteger(bar) && bar >= 0 && bar < BAR_COUNT;
}

function cloneBank(bank: PatternBankState): PatternBankState {
  return {
    stepCount: bank.stepCount,
    pattern: bank.pattern.slice(),
    cellProbabilities: bank.cellProbabilities.slice(),
    cellHumanize: bank.cellHumanize.slice(),
    cellConditions: bank.cellConditions.slice(),
    cellRepeats: bank.cellRepeats.slice(),
    trackLengths: bank.trackLengths.slice(),
  };
}

function cellIndex(track: number, step: number): number {
  return track * STEP_CAPACITY + step;
}

function copyTrigger(
  source: PatternBankState,
  destination: PatternBankState,
  sourceTrack: number,
  sourceStep: number,
  destinationTrack: number,
  destinationStep: number,
): void {
  const from = cellIndex(sourceTrack, sourceStep);
  const to = cellIndex(destinationTrack, destinationStep);
  destination.pattern[to] = source.pattern[from] ?? 0;
  destination.cellProbabilities[to] = source.cellProbabilities[from] ?? 1;
  destination.cellHumanize[to] = source.cellHumanize[from] ?? 1;
  destination.cellConditions[to] = source.cellConditions[from] ?? 0;
  destination.cellRepeats[to] = source.cellRepeats[from] ?? 1;
}

/** Clear every trigger field in a row without changing its loop length. */
export function clearRow(
  bank: PatternBankState,
  track: number,
): PatternBankState {
  const next = cloneBank(bank);
  if (!validTrack(track)) return next;

  for (let step = 0; step < STEP_CAPACITY; step++) {
    const index = cellIndex(track, step);
    next.pattern[index] = 0;
    next.cellProbabilities[index] = 1;
    next.cellHumanize[index] = 1;
    next.cellConditions[index] = 0;
    next.cellRepeats[index] = 1;
  }
  return next;
}

/** Rotate complete trigger records inside the row's active loop. */
export function rotateRow(
  bank: PatternBankState,
  track: number,
  direction: "left" | "right",
): PatternBankState {
  const next = cloneBank(bank);
  if (!validTrack(track)) return next;
  const length = bank.trackLengths[track] ?? bank.stepCount;
  if (length <= 1) return next;

  for (let destinationStep = 0; destinationStep < length; destinationStep++) {
    const sourceStep =
      direction === "left"
        ? (destinationStep + 1) % length
        : (destinationStep - 1 + length) % length;
    copyTrigger(bank, next, track, sourceStep, track, destinationStep);
  }
  return next;
}

/** Copy a complete row, including its independent loop length. */
export function copyRow(
  bank: PatternBankState,
  sourceTrack: number,
  destinationTrack: number,
): PatternBankState {
  const next = cloneBank(bank);
  if (
    !validTrack(sourceTrack) ||
    !validTrack(destinationTrack) ||
    sourceTrack === destinationTrack
  ) {
    return next;
  }

  for (let step = 0; step < STEP_CAPACITY; step++) {
    copyTrigger(bank, next, sourceTrack, step, destinationTrack, step);
  }
  next.trackLengths[destinationTrack] =
    bank.trackLengths[sourceTrack] ?? bank.stepCount;
  return next;
}

/** Copy one four-step bar to another bar in the same row. */
export function copyBar(
  bank: PatternBankState,
  track: number,
  sourceBar: number,
  destinationBar: number,
): PatternBankState {
  const next = cloneBank(bank);
  if (
    !validTrack(track) ||
    !validBar(sourceBar) ||
    !validBar(destinationBar) ||
    sourceBar === destinationBar
  ) {
    return next;
  }

  const sourceStart = sourceBar * BAR_LENGTH;
  const destinationStart = destinationBar * BAR_LENGTH;
  for (let offset = 0; offset < BAR_LENGTH; offset++) {
    copyTrigger(
      bank,
      next,
      track,
      sourceStart + offset,
      track,
      destinationStart + offset,
    );
  }
  return next;
}
