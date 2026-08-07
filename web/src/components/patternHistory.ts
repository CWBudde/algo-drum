import { useCallback, useRef, useState } from "react";
import type { EngineState, PatternBankState } from "../engine/engineState";

const HISTORY_LIMIT = 50;
const BANK_COUNT = 4;

export interface PatternHistory {
  past: PatternBankState[];
  future: PatternBankState[];
}

export interface PatternHistoryTransition {
  history: PatternHistory;
  bank: PatternBankState | null;
}

export const EMPTY_PATTERN_HISTORY: PatternHistory = { past: [], future: [] };

function cloneBank(bank: PatternBankState): PatternBankState {
  return {
    stepCount: bank.stepCount,
    pattern: bank.pattern.slice(),
    cellProbabilities: bank.cellProbabilities.slice(),
    cellHumanize: bank.cellHumanize.slice(),
    cellConditions: bank.cellConditions.slice(),
    trackLengths: bank.trackLengths.slice(),
  };
}

function arraysEqual(
  left: ArrayLike<number>,
  right: ArrayLike<number>,
): boolean {
  if (left.length !== right.length) return false;
  for (let index = 0; index < left.length; index++) {
    if (left[index] !== right[index]) return false;
  }
  return true;
}

export function patternBanksEqual(
  left: PatternBankState,
  right: PatternBankState,
): boolean {
  return (
    left.stepCount === right.stepCount &&
    arraysEqual(left.pattern, right.pattern) &&
    arraysEqual(left.cellProbabilities, right.cellProbabilities) &&
    arraysEqual(left.cellHumanize, right.cellHumanize) &&
    arraysEqual(left.cellConditions, right.cellConditions) &&
    arraysEqual(left.trackLengths, right.trackLengths)
  );
}

export function recordPatternBank(
  history: PatternHistory,
  current: PatternBankState,
  next: PatternBankState,
): PatternHistory {
  if (patternBanksEqual(current, next)) return history;
  const past = [...history.past, cloneBank(current)];
  return { past: past.slice(-HISTORY_LIMIT), future: [] };
}

export function undoPatternBank(
  history: PatternHistory,
  current: PatternBankState,
): PatternHistoryTransition {
  const bank = history.past[history.past.length - 1];
  if (!bank) return { history, bank: null };
  return {
    history: {
      past: history.past.slice(0, -1),
      future: [cloneBank(current), ...history.future],
    },
    bank: cloneBank(bank),
  };
}

export function redoPatternBank(
  history: PatternHistory,
  current: PatternBankState,
): PatternHistoryTransition {
  const bank = history.future[0];
  if (!bank) return { history, bank: null };
  return {
    history: {
      past: [...history.past, cloneBank(current)],
      future: history.future.slice(1),
    },
    bank: cloneBank(bank),
  };
}

export function usePatternHistory(
  selectedBank: number,
  banks: EngineState["banks"],
  applyBank: (bank: number, state: PatternBankState) => void,
) {
  const banksRef = useRef(banks);
  banksRef.current = banks;
  const selectedRef = useRef(selectedBank);
  selectedRef.current = selectedBank;
  const histories = useRef<PatternHistory[]>(
    Array.from({ length: BANK_COUNT }, () => EMPTY_PATTERN_HISTORY),
  );
  const [, render] = useState(0);

  const updateHistory = useCallback((bank: number, history: PatternHistory) => {
    histories.current[bank] = history;
    render((revision) => revision + 1);
  }, []);

  const replaceBank = useCallback(
    (bank: number, next: PatternBankState) => {
      const current = banksRef.current[bank];
      if (!current) return;
      const history = recordPatternBank(histories.current[bank], current, next);
      if (history === histories.current[bank]) return;
      updateHistory(bank, history);
      applyBank(bank, cloneBank(next));
    },
    [applyBank, updateHistory],
  );

  const applyDestructivePattern = useCallback(
    (next: ArrayLike<number>) => {
      const bank = selectedRef.current;
      const current = banksRef.current[bank];
      if (!current) return;
      replaceBank(bank, {
        ...cloneBank(current),
        pattern: Float32Array.from(next),
      });
    },
    [replaceBank],
  );

  const copyBank = useCallback(
    (source: number, destination: number) => {
      const sourceBank = banksRef.current[source];
      if (!sourceBank || source === destination) return;
      replaceBank(destination, sourceBank);
    },
    [replaceBank],
  );

  const undo = useCallback(() => {
    const bank = selectedRef.current;
    const current = banksRef.current[bank];
    if (!current) return;
    const transition = undoPatternBank(histories.current[bank], current);
    if (!transition.bank) return;
    updateHistory(bank, transition.history);
    applyBank(bank, transition.bank);
  }, [applyBank, updateHistory]);

  const redo = useCallback(() => {
    const bank = selectedRef.current;
    const current = banksRef.current[bank];
    if (!current) return;
    const transition = redoPatternBank(histories.current[bank], current);
    if (!transition.bank) return;
    updateHistory(bank, transition.history);
    applyBank(bank, transition.bank);
  }, [applyBank, updateHistory]);

  const currentHistory = histories.current[selectedBank];
  return {
    applyDestructivePattern,
    copyBank,
    undo,
    redo,
    canUndo: (currentHistory?.past.length ?? 0) > 0,
    canRedo: (currentHistory?.future.length ?? 0) > 0,
  };
}
