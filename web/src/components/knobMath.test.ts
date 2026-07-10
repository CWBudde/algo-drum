import { describe, expect, it } from "vitest";
import {
  DRAG_RANGE_PX,
  KEY_STEP,
  KEY_STEP_LARGE,
  MAX_ANGLE,
  MIN_ANGLE,
  WHEEL_STEP,
  clamp01,
  dragValue,
  keyValue,
  valueToAngle,
  wheelValue,
} from "./knobMath";

describe("clamp01", () => {
  it("passes through in-range values", () => {
    expect(clamp01(0)).toBe(0);
    expect(clamp01(0.5)).toBe(0.5);
    expect(clamp01(1)).toBe(1);
  });
  it("clamps out-of-range values", () => {
    expect(clamp01(-0.3)).toBe(0);
    expect(clamp01(1.7)).toBe(1);
  });
});

describe("valueToAngle", () => {
  it("maps 0/0.5/1 to the sweep endpoints and center", () => {
    expect(valueToAngle(0)).toBe(MIN_ANGLE);
    expect(valueToAngle(1)).toBe(MAX_ANGLE);
    expect(valueToAngle(0.5)).toBe(0);
  });
  it("is linear across the range", () => {
    expect(valueToAngle(0.25)).toBeCloseTo(-67.5, 6);
    expect(valueToAngle(0.75)).toBeCloseTo(67.5, 6);
  });
});

describe("dragValue", () => {
  it("dragging up (clientY decreasing) raises the value", () => {
    // A full DRAG_RANGE_PX of upward travel is a full 0→1 sweep.
    expect(dragValue(0, 200, 200 - DRAG_RANGE_PX, false)).toBeCloseTo(1, 6);
  });
  it("dragging down lowers the value", () => {
    expect(dragValue(1, 100, 100 + DRAG_RANGE_PX, false)).toBeCloseTo(0, 6);
  });
  it("no vertical movement keeps the start value", () => {
    expect(dragValue(0.4, 120, 120, false)).toBeCloseTo(0.4, 6);
  });
  it("fine (Shift) scales sensitivity by 0.25", () => {
    const coarse = dragValue(0.5, 200, 170, false) - 0.5; // +30px up
    const fine = dragValue(0.5, 200, 170, true) - 0.5;
    expect(fine).toBeCloseTo(coarse * 0.25, 6);
  });
  it("clamps at the ends", () => {
    expect(dragValue(0.9, 100, 100 - DRAG_RANGE_PX, false)).toBe(1);
    expect(dragValue(0.1, 100, 100 + DRAG_RANGE_PX, false)).toBe(0);
  });
});

describe("wheelValue", () => {
  it("scroll up (deltaY < 0) increases by WHEEL_STEP", () => {
    expect(wheelValue(0.5, -1)).toBeCloseTo(0.5 + WHEEL_STEP, 6);
  });
  it("scroll down (deltaY > 0) decreases by WHEEL_STEP", () => {
    expect(wheelValue(0.5, 1)).toBeCloseTo(0.5 - WHEEL_STEP, 6);
  });
  it("clamps at the extremes", () => {
    expect(wheelValue(1, -1)).toBe(1);
    expect(wheelValue(0, 1)).toBe(0);
  });
});

describe("keyValue", () => {
  it("arrow keys step by KEY_STEP", () => {
    expect(keyValue(0.5, "ArrowUp", false)).toBeCloseTo(0.5 + KEY_STEP, 6);
    expect(keyValue(0.5, "ArrowRight", false)).toBeCloseTo(0.5 + KEY_STEP, 6);
    expect(keyValue(0.5, "ArrowDown", false)).toBeCloseTo(0.5 - KEY_STEP, 6);
    expect(keyValue(0.5, "ArrowLeft", false)).toBeCloseTo(0.5 - KEY_STEP, 6);
  });
  it("Shift enlarges the arrow step", () => {
    expect(keyValue(0.5, "ArrowUp", true)).toBeCloseTo(0.5 + KEY_STEP_LARGE, 6);
  });
  it("PageUp/PageDown use the large step", () => {
    expect(keyValue(0.5, "PageUp", false)).toBeCloseTo(0.5 + KEY_STEP_LARGE, 6);
    expect(keyValue(0.5, "PageDown", false)).toBeCloseTo(
      0.5 - KEY_STEP_LARGE,
      6,
    );
  });
  it("Home/End jump to the extremes", () => {
    expect(keyValue(0.5, "Home", false)).toBe(0);
    expect(keyValue(0.5, "End", false)).toBe(1);
  });
  it("Escape resets to the default (clamped), or null without one", () => {
    expect(keyValue(0.8, "Escape", false, 0.43)).toBe(0.43);
    expect(keyValue(0.8, "Escape", false, 2)).toBe(1); // clamped
    expect(keyValue(0.8, "Escape", false)).toBeNull();
  });
  it("returns null for unhandled keys", () => {
    expect(keyValue(0.5, "a", false)).toBeNull();
    expect(keyValue(0.5, "Enter", false)).toBeNull();
  });
  it("clamps stepping past the ends", () => {
    expect(keyValue(1, "ArrowUp", false)).toBe(1);
    expect(keyValue(0, "ArrowDown", false)).toBe(0);
  });
});
