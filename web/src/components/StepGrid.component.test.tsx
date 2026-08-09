import { act, fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { STEP_CAPACITY, VEL_ACCENT, VEL_NORMAL } from "../algo/pattern";
import StepGrid from "./StepGrid";

const engineMock = vi.hoisted(() => ({
  stepListener: null as ((step: number) => void) | null,
}));

vi.mock("../engine/wasmEngine", () => ({
  onStep: (listener: (step: number) => void) => {
    engineMock.stepListener = listener;
    listener(-1);
    return () => {
      if (engineMock.stepListener === listener) engineMock.stepListener = null;
    };
  },
}));

function makePattern(): number[][] {
  return Array.from({ length: 7 }, () =>
    new Array<number>(STEP_CAPACITY).fill(0),
  );
}

function gridProps(pattern = makePattern()) {
  return {
    disabled: false,
    showPlayhead: true,
    pattern,
    stepCount: STEP_CAPACITY,
    volumes: new Array<number>(7).fill(0.75),
    decays: new Array<number>(7).fill(0.5),
    muted: new Array<boolean>(7).fill(false),
    trackLengths: new Array<number>(7).fill(STEP_CAPACITY),
    onCellChange: vi.fn(),
    onPaintComplete: vi.fn(),
    onClearRow: vi.fn(),
    onRotateRow: vi.fn(),
    onCopyRow: vi.fn(),
    onCopyBar: vi.fn(),
    onVolumeChange: vi.fn(),
    onDecayChange: vi.fn(),
    onToggleMute: vi.fn(),
    onTrackLengthChange: vi.fn(),
    onInspectCell: vi.fn(),
    onOpenEditor: vi.fn(),
  };
}

describe("StepGrid accessibility contract", () => {
  it("keeps one roving tab stop and moves focus in two dimensions", () => {
    const { container } = render(<StepGrid {...gridProps()} />);
    const cymbal1 = screen.getByRole("button", { name: "Cymbal step 1" });
    expect(container.querySelectorAll(".dm-cell[tabindex='0']")).toHaveLength(
      1,
    );
    expect(cymbal1).toHaveAttribute("tabindex", "0");

    cymbal1.focus();
    fireEvent.keyDown(cymbal1, { key: "ArrowRight" });
    const cymbal2 = screen.getByRole("button", { name: "Cymbal step 2" });
    expect(cymbal2).toHaveFocus();
    expect(cymbal2).toHaveAttribute("tabindex", "0");

    fireEvent.keyDown(cymbal2, { key: "ArrowDown" });
    const perc2 = screen.getByRole("button", { name: "Perc step 2" });
    expect(perc2).toHaveFocus();

    fireEvent.keyDown(perc2, { key: "End", ctrlKey: true });
    expect(screen.getByRole("button", { name: "Bass step 16" })).toHaveFocus();
    expect(container.querySelectorAll(".dm-cell[tabindex='0']")).toHaveLength(
      1,
    );
  });

  it("exposes off, normal, and accent without changing cell names", () => {
    const pattern = makePattern();
    pattern[0]![1] = VEL_NORMAL;
    pattern[0]![2] = VEL_ACCENT;
    render(<StepGrid {...gridProps(pattern)} />);

    const off = screen.getByRole("button", { name: "Cymbal step 1" });
    const normal = screen.getByRole("button", { name: "Cymbal step 2" });
    const accent = screen.getByRole("button", { name: "Cymbal step 3" });
    expect(off).toHaveAttribute("aria-pressed", "false");
    expect(normal).toHaveAttribute("aria-pressed", "true");
    expect(accent).toHaveAttribute("aria-pressed", "mixed");
    expect(off).toHaveAccessibleDescription(
      expect.stringContaining("off velocity"),
    );
    expect(normal).toHaveAccessibleDescription(
      expect.stringContaining("on velocity"),
    );
    expect(accent).toHaveAccessibleDescription(
      expect.stringContaining("accent velocity"),
    );
  });

  it("marks only audible playhead cells as the current step", () => {
    const props = gridProps();
    const { container, rerender } = render(<StepGrid {...props} />);

    act(() => engineMock.stepListener?.(3));
    const current = container.querySelectorAll(".dm-cell[aria-current='step']");
    expect(current).toHaveLength(7);
    expect(
      screen.getByRole("button", { name: "Snare step 4" }),
    ).toHaveAttribute("aria-current", "step");

    act(() => engineMock.stepListener?.(-1));
    expect(container.querySelectorAll(".dm-cell[aria-current]")).toHaveLength(
      0,
    );

    rerender(<StepGrid {...props} showPlayhead={false} />);
    act(() => engineMock.stepListener?.(4));
    expect(container.querySelectorAll(".dm-cell[aria-current]")).toHaveLength(
      0,
    );
  });

  it("routes cell, inspector, and track-strip edits to semantic callbacks", () => {
    const props = gridProps();
    render(<StepGrid {...props} />);
    const cymbal1 = screen.getByRole("button", { name: "Cymbal step 1" });

    fireEvent.click(cymbal1);
    expect(props.onCellChange).toHaveBeenCalledWith(4, 0, VEL_NORMAL);
    fireEvent.keyDown(cymbal1, { key: "ArrowUp", shiftKey: true });
    expect(props.onCellChange).toHaveBeenLastCalledWith(4, 0, 0.05);
    fireEvent.keyDown(cymbal1, { key: "F2" });
    fireEvent.contextMenu(cymbal1);
    expect(props.onInspectCell).toHaveBeenCalledTimes(2);
    expect(props.onInspectCell).toHaveBeenCalledWith(4, 0);

    fireEvent.click(screen.getByRole("button", { name: "Mute Cymbal" }));
    expect(props.onToggleMute).toHaveBeenCalledWith(0);
    fireEvent.change(screen.getByLabelText("Cymbal pattern length"), {
      target: { value: "7" },
    });
    expect(props.onTrackLengthChange).toHaveBeenCalledWith(0, 7);

    const volume = screen.getByRole("slider", { name: "Cymbal volume" });
    fireEvent.keyDown(volume, { key: "ArrowUp" });
    expect(props.onVolumeChange).toHaveBeenCalledWith(0, 0.77);
    const decay = screen.getByRole("slider", { name: "Cymbal decay" });
    fireEvent.keyDown(decay, { key: "ArrowDown" });
    expect(props.onDecayChange).toHaveBeenCalledWith(0, 0.48);

    const voice = screen.getByRole("button", {
      name: "Cymbal voice settings",
    });
    fireEvent.click(voice);
    expect(props.onOpenEditor).toHaveBeenCalledWith(4, voice, false);
  });

  it("executes every row action and restores focus to its opener", () => {
    const props = gridProps();
    render(<StepGrid {...props} />);
    const opener = screen.getByRole("button", { name: "Cymbal row actions" });

    fireEvent.click(opener);
    fireEvent.click(screen.getByRole("button", { name: "Clear row" }));
    expect(props.onClearRow).toHaveBeenCalledWith(0);
    expect(opener).toHaveFocus();

    fireEvent.click(opener);
    fireEvent.click(screen.getByRole("button", { name: "Rotate left" }));
    fireEvent.click(opener);
    fireEvent.click(screen.getByRole("button", { name: "Rotate right" }));
    expect(props.onRotateRow).toHaveBeenNthCalledWith(1, 0, "left");
    expect(props.onRotateRow).toHaveBeenNthCalledWith(2, 0, "right");

    fireEvent.click(opener);
    fireEvent.change(screen.getByLabelText("Copy row to"), {
      target: { value: "2" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Copy row" }));
    expect(props.onCopyRow).toHaveBeenCalledWith(0, 2);

    fireEvent.click(opener);
    fireEvent.change(screen.getByLabelText("Source bar"), {
      target: { value: "1" },
    });
    fireEvent.change(screen.getByLabelText("Destination bar"), {
      target: { value: "3" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Copy bar" }));
    expect(props.onCopyBar).toHaveBeenCalledWith(0, 1, 3);
  });

  it("commits pointer painting and continuous shift-drag velocity", () => {
    const props = gridProps();
    render(<StepGrid {...props} />);
    const first = screen.getByRole("button", { name: "Cymbal step 1" });
    const second = screen.getByRole("button", { name: "Cymbal step 2" });
    const elementFromPoint = vi.fn(() => second);
    Object.defineProperty(document, "elementFromPoint", {
      configurable: true,
      value: elementFromPoint,
    });

    fireEvent.pointerDown(first, {
      pointerId: 8,
      pointerType: "mouse",
      isPrimary: true,
      button: 0,
    });
    fireEvent.pointerMove(first, { pointerId: 8, clientX: 10, clientY: 10 });
    fireEvent.pointerUp(first, { pointerId: 8 });
    expect(props.onPaintComplete).toHaveBeenCalledWith([
      { track: 4, step: 0, value: VEL_NORMAL },
      { track: 4, step: 1, value: VEL_NORMAL },
    ]);

    vi.spyOn(second, "getBoundingClientRect").mockReturnValue({
      x: 0,
      y: 100,
      top: 100,
      left: 0,
      right: 40,
      bottom: 140,
      width: 40,
      height: 40,
      toJSON: () => ({}),
    });
    fireEvent.pointerDown(second, {
      pointerId: 9,
      pointerType: "mouse",
      isPrimary: true,
      button: 0,
      shiftKey: true,
      clientY: 110,
    });
    fireEvent.pointerMove(second, {
      pointerId: 9,
      shiftKey: true,
      clientY: 120,
    });
    fireEvent.pointerUp(second, { pointerId: 9 });
    expect(props.onCellChange).toHaveBeenCalledWith(4, 1, 0.75);
    expect(props.onCellChange).toHaveBeenCalledWith(4, 1, 0.5);
    Reflect.deleteProperty(document, "elementFromPoint");
  });
});
