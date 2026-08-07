import { useCallback, useRef, useState } from "react";
import { patternsEqual } from "./patternView";

const HISTORY_LIMIT = 50;

export interface PatternHistory {
  past: Float32Array[];
  future: Float32Array[];
}

export interface PatternHistoryTransition {
  history: PatternHistory;
  pattern: Float32Array | null;
}

export const EMPTY_PATTERN_HISTORY: PatternHistory = { past: [], future: [] };

export function recordPattern(
  history: PatternHistory,
  current: ArrayLike<number>,
  next: ArrayLike<number>,
): PatternHistory {
  const canonicalNext = Float32Array.from(next);
  if (patternsEqual(current, canonicalNext)) return history;
  const past = [...history.past, Float32Array.from(current)];
  return { past: past.slice(-HISTORY_LIMIT), future: [] };
}

export function undoPattern(
  history: PatternHistory,
  current: ArrayLike<number>,
): PatternHistoryTransition {
  const pattern = history.past[history.past.length - 1];
  if (!pattern) return { history, pattern: null };
  return {
    history: {
      past: history.past.slice(0, -1),
      future: [Float32Array.from(current), ...history.future],
    },
    pattern: pattern.slice(),
  };
}

export function redoPattern(
  history: PatternHistory,
  current: ArrayLike<number>,
): PatternHistoryTransition {
  const pattern = history.future[0];
  if (!pattern) return { history, pattern: null };
  return {
    history: {
      past: [...history.past, Float32Array.from(current)],
      future: history.future.slice(1),
    },
    pattern: pattern.slice(),
  };
}

export function usePatternHistory(
  current: Float32Array,
  applyPattern: (pattern: Float32Array) => void,
) {
  const currentRef = useRef(current);
  currentRef.current = current;
  const historyRef = useRef<PatternHistory>(EMPTY_PATTERN_HISTORY);
  const [, render] = useState(0);

  const updateHistory = useCallback((history: PatternHistory) => {
    historyRef.current = history;
    render((revision) => revision + 1);
  }, []);

  const applyDestructivePattern = useCallback(
    (next: ArrayLike<number>) => {
      const history = recordPattern(
        historyRef.current,
        currentRef.current,
        next,
      );
      if (history === historyRef.current) return;
      updateHistory(history);
      applyPattern(Float32Array.from(next));
    },
    [applyPattern, updateHistory],
  );

  const undo = useCallback(() => {
    const transition = undoPattern(historyRef.current, currentRef.current);
    if (!transition.pattern) return;
    updateHistory(transition.history);
    applyPattern(transition.pattern);
  }, [applyPattern, updateHistory]);

  const redo = useCallback(() => {
    const transition = redoPattern(historyRef.current, currentRef.current);
    if (!transition.pattern) return;
    updateHistory(transition.history);
    applyPattern(transition.pattern);
  }, [applyPattern, updateHistory]);

  return {
    applyDestructivePattern,
    undo,
    redo,
    canUndo: historyRef.current.past.length > 0,
    canRedo: historyRef.current.future.length > 0,
  };
}
