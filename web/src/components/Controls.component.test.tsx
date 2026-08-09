import { useState } from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { PHYSICAL_TOM_PARAMS, VOICE_PARAMS } from "../engine/voiceParams";
import CellInspector from "./CellInspector";
import type { DrumStateAction } from "./drumState";
import Knob from "./Knob";
import PatternBanks from "./PatternBanks";
import ShortcutHelp from "./ShortcutHelp";
import Transport from "./Transport";
import VoiceEditor from "./VoiceEditor";

describe("Knob", () => {
  it("connects keyboard, wheel, pointer, and reset gestures to its value", async () => {
    const user = userEvent.setup();
    function Harness() {
      const [value, setValue] = useState(0.5);
      return (
        <Knob
          value={value}
          onChange={setValue}
          label="TEST"
          defaultValue={0.25}
        />
      );
    }
    render(<Harness />);
    const slider = screen.getByRole("slider", { name: "TEST" });

    slider.focus();
    await user.keyboard("{ArrowUp}");
    expect(slider).toHaveAttribute("aria-valuenow", "52");
    fireEvent.wheel(slider, { deltaY: -1 });
    expect(slider).toHaveAttribute("aria-valuenow", "55");

    fireEvent.pointerDown(slider, {
      pointerId: 4,
      pointerType: "mouse",
      button: 0,
      clientY: 200,
    });
    fireEvent.pointerMove(window, { pointerId: 4, clientY: 125 });
    expect(slider).toHaveAttribute("aria-valuenow", "100");
    fireEvent.pointerUp(window, { pointerId: 4, clientY: 125 });

    await user.keyboard("{Escape}");
    expect(slider).toHaveAttribute("aria-valuenow", "25");
    await user.dblClick(slider);
    expect(slider).toHaveAttribute("aria-valuenow", "25");
  });
});

describe("Transport", () => {
  it("dispatches every transport and master-control action", async () => {
    const user = userEvent.setup();
    const onAction = vi.fn<(action: DrumStateAction) => void>();
    const onPlayPause = vi.fn();
    const onStop = vi.fn();
    const onTap = vi.fn();
    const props = {
      disabled: false,
      transport: "playing" as const,
      bpm: 120,
      swing: 0,
      bank: 2,
      stepCount: 16,
      probability: 1,
      humanize: 0,
      reverb: 0,
      fillMode: false,
      metronomeEnabled: false,
      countInBars: 0,
      onAction,
      onPlayPause,
      onStop,
      onTap,
    };
    const { rerender } = render(<Transport {...props} />);

    await user.click(screen.getByRole("button", { name: "Pause" }));
    await user.click(screen.getByRole("button", { name: "Stop" }));
    await user.click(screen.getByRole("button", { name: "FILL" }));
    await user.click(
      screen.getByRole("button", { name: "Playback metronome" }),
    );
    await user.selectOptions(
      screen.getByRole("combobox", { name: "Count-in bars" }),
      "2",
    );
    await user.click(screen.getByRole("button", { name: "Tap tempo" }));
    expect(onPlayPause).toHaveBeenCalledOnce();
    expect(onStop).toHaveBeenCalledOnce();
    expect(onTap).toHaveBeenCalledOnce();
    expect(onAction).toHaveBeenCalledWith({ type: "fillMode", value: true });
    expect(onAction).toHaveBeenCalledWith({
      type: "metronomeEnabled",
      value: true,
    });
    expect(onAction).toHaveBeenCalledWith({ type: "countInBars", value: 2 });

    const controls = [
      ["Tempo", "tempo"],
      ["SWING", "swing"],
      ["Pattern length", "stepCount"],
      ["Trigger probability", "probability"],
      ["Humanize", "humanize"],
      ["REVERB", "reverb"],
    ] as const;
    for (const [name] of controls) {
      const slider = screen.getByRole("slider", { name });
      slider.focus();
      await user.keyboard("{Home}{ArrowRight}");
    }
    for (const [, type] of controls) {
      expect(onAction.mock.calls.some(([action]) => action.type === type)).toBe(
        true,
      );
    }

    rerender(<Transport {...props} transport="counting-in" />);
    expect(screen.getByRole("button", { name: "Counting in" })).toBeDisabled();
    rerender(<Transport {...props} transport="starting" />);
    expect(screen.getByRole("button", { name: "Starting" })).toBeDisabled();
    rerender(<Transport {...props} transport="stopped" />);
    expect(screen.getByRole("button", { name: "Play" })).toBeEnabled();
  });
});

describe("CellInspector", () => {
  it("commits all trigger controls and closes by button or Escape", async () => {
    const user = userEvent.setup();
    const onProbabilityChange = vi.fn();
    const onHumanizeChange = vi.fn();
    const onConditionChange = vi.fn();
    const onRepeatsChange = vi.fn();
    const onClose = vi.fn();
    render(
      <CellInspector
        track={1}
        step={4}
        probability={1}
        humanize={0}
        condition={0}
        repeats={1}
        onProbabilityChange={onProbabilityChange}
        onHumanizeChange={onHumanizeChange}
        onConditionChange={onConditionChange}
        onRepeatsChange={onRepeatsChange}
        onClose={onClose}
      />,
    );

    fireEvent.change(screen.getByRole("slider", { name: /^PROBABILITY/ }), {
      target: { value: "40" },
    });
    fireEvent.change(screen.getByRole("slider", { name: /^HUMANIZE/ }), {
      target: { value: "25" },
    });
    await user.selectOptions(
      screen.getByRole("combobox", { name: "CONDITION" }),
      "3",
    );
    await user.selectOptions(
      screen.getByRole("combobox", { name: "Ratchet repeats" }),
      "4",
    );
    expect(onProbabilityChange).toHaveBeenCalledWith(0.4);
    expect(onHumanizeChange).toHaveBeenCalledWith(0.25);
    expect(onConditionChange).toHaveBeenCalledWith(3);
    expect(onRepeatsChange).toHaveBeenCalledWith(4);

    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    await user.click(
      screen.getByRole("button", { name: "Close step settings" }),
    );
    expect(onClose).toHaveBeenCalledTimes(2);
  });
});

describe("PatternBanks", () => {
  it("selects, copies, toggles, and edits a stopped chain", async () => {
    const user = userEvent.setup();
    const onSelectBank = vi.fn();
    const onCopyBank = vi.fn();
    const onChainEnabledChange = vi.fn();
    const onChainChange = vi.fn<(chain: Uint8Array) => void>();
    render(
      <PatternBanks
        disabled={false}
        transport="stopped"
        selectedBank={0}
        activeBank={0}
        queuedBank={-1}
        chainPosition={-1}
        chainEnabled={false}
        chain={Uint8Array.of(0, 1)}
        onSelectBank={onSelectBank}
        onCopyBank={onCopyBank}
        onChainEnabledChange={onChainEnabledChange}
        onChainChange={onChainChange}
      />,
    );

    await user.click(screen.getByRole("button", { name: "B" }));
    await user.click(screen.getByText("COPY"));
    await user.click(
      screen.getByRole("button", { name: "Copy bank A to bank C" }),
    );
    await user.click(screen.getByRole("checkbox", { name: "CHAIN" }));
    await user.click(screen.getByText("EDIT CHAIN"));
    await user.selectOptions(
      screen.getByRole("combobox", { name: "Chain entry 1" }),
      "3",
    );
    await user.click(
      screen.getByRole("button", { name: "Remove chain entry 2" }),
    );
    await user.click(screen.getByRole("button", { name: "ADD ENTRY" }));

    expect(onSelectBank).toHaveBeenCalledWith(1);
    expect(onCopyBank).toHaveBeenCalledWith(2);
    expect(onChainEnabledChange).toHaveBeenCalledWith(true);
    expect(onChainChange).toHaveBeenCalledTimes(3);
    expect(Array.from(onChainChange.mock.calls[0]![0])).toEqual([3, 1]);
    expect(Array.from(onChainChange.mock.calls[1]![0])).toEqual([0]);
    expect(Array.from(onChainChange.mock.calls[2]![0])).toEqual([0, 1, 1]);
  });
});

describe("dialogs", () => {
  it("opens and closes shortcut help through its native dialog events", async () => {
    const user = userEvent.setup();
    const onRequestClose = vi.fn();
    render(<ShortcutHelp onRequestClose={onRequestClose} />);
    const dialog = screen.getByRole("dialog", { name: "Keyboard shortcuts" });
    expect(dialog).toHaveAttribute("open");
    fireEvent(
      dialog,
      new Event("cancel", { bubbles: false, cancelable: true }),
    );
    await user.click(
      screen.getByRole("button", { name: "Close keyboard shortcuts" }),
    );
    expect(onRequestClose).toHaveBeenCalledTimes(2);
  });

  it("edits, auditions, resets, switches model, and closes a voice", async () => {
    const user = userEvent.setup();
    const specs = VOICE_PARAMS[0]!;
    const values = Float32Array.from(specs, (spec) => spec.default);
    const onModelChange = vi.fn();
    const onChange = vi.fn();
    const onReset = vi.fn();
    const onAudition = vi.fn();
    const onRequestClose = vi.fn();
    const props = {
      name: "Bass Drum",
      specs,
      values,
      disabled: false,
      model: "procedural" as const,
      onModelChange,
      onChange,
      onReset,
      onAudition,
      onRequestClose,
    };
    const { rerender } = render(<VoiceEditor {...props} />);

    await user.click(screen.getByRole("radio", { name: /Physical/ }));
    const firstKnob = screen.getAllByRole("slider")[0]!;
    firstKnob.focus();
    await user.keyboard("{ArrowRight}");
    await user.click(
      screen.getByRole("button", { name: "Audition Bass Drum" }),
    );
    await user.click(
      screen.getByRole("button", { name: "Reset Bass Drum voice to defaults" }),
    );
    expect(onModelChange).toHaveBeenCalledWith("physical");
    expect(onChange).toHaveBeenCalled();
    expect(onAudition).toHaveBeenCalledWith(1);
    expect(onReset).toHaveBeenCalledOnce();
    expect(screen.getByRole("status")).toHaveTextContent(
      "Bass Drum voice reset to defaults",
    );

    const physicalValues = Float32Array.from(
      PHYSICAL_TOM_PARAMS,
      (spec) => spec.default,
    );
    rerender(
      <VoiceEditor
        {...props}
        name="Tom"
        specs={PHYSICAL_TOM_PARAMS}
        values={physicalValues}
        model="physical"
      />,
    );
    await user.selectOptions(
      screen.getByRole("combobox", { name: "Preset" }),
      "0",
    );
    expect(onChange).toHaveBeenCalled();
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    expect(onRequestClose).toHaveBeenCalled();
  });
});
