// Pure math for the rotary Knob: value↔angle mapping and the drag / wheel /
// keyboard interaction deltas. Extracted from Knob.tsx so the behaviour can be
// unit-tested without rendering. Knob.tsx imports these; the arithmetic here is
// exactly what the component used inline.

export const MIN_ANGLE = -135;
export const MAX_ANGLE = 135;
export const KEY_STEP = 0.02;
export const KEY_STEP_LARGE = 0.1;
export const WHEEL_STEP = 0.03;

// Vertical pixels of travel for a full 0→1 sweep, and the fine-adjust factor
// applied while Shift is held during a drag.
export const DRAG_RANGE_PX = 150;
export const FINE_FACTOR = 0.25;

export function clamp01(v: number): number {
  return Math.max(0, Math.min(1, v));
}

// valueToAngle maps a normalized value in [0, 1] to the knob's sweep angle.
export function valueToAngle(v: number): number {
  return MIN_ANGLE + v * (MAX_ANGLE - MIN_ANGLE);
}

// dragValue returns the new value for a vertical drag: dragging up (clientY
// decreasing) raises the value. `fine` (Shift) scales the sensitivity down.
export function dragValue(
  startVal: number,
  startY: number,
  currentY: number,
  fine: boolean,
): number {
  const factor = fine ? FINE_FACTOR : 1;
  const delta = ((startY - currentY) / DRAG_RANGE_PX) * factor;
  return clamp01(startVal + delta);
}

// wheelValue nudges the value one WHEEL_STEP per notch; scroll up increases.
export function wheelValue(value: number, deltaY: number): number {
  const direction = deltaY < 0 ? 1 : -1;
  return clamp01(value + direction * WHEEL_STEP);
}

// keyValue returns the value a key press should set, or null if the key isn't
// a knob control. Shift enlarges the arrow step; Escape resets to defaultValue
// (or null when no default is provided).
export function keyValue(
  value: number,
  key: string,
  shiftKey: boolean,
  defaultValue?: number,
): number | null {
  const step = shiftKey ? KEY_STEP_LARGE : KEY_STEP;
  switch (key) {
    case "ArrowUp":
    case "ArrowRight":
      return clamp01(value + step);
    case "ArrowDown":
    case "ArrowLeft":
      return clamp01(value - step);
    case "PageUp":
      return clamp01(value + KEY_STEP_LARGE);
    case "PageDown":
      return clamp01(value - KEY_STEP_LARGE);
    case "Home":
      return 0;
    case "End":
      return 1;
    case "Escape":
      return defaultValue === undefined ? null : clamp01(defaultValue);
    default:
      return null;
  }
}
