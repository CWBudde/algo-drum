import { act, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import DrumMachine from "./DrumMachine";

const engineMock = vi.hoisted(() => ({
  transportListener: null as ((state: string) => void) | null,
  stepListener: null as ((step: number) => void) | null,
}));

vi.mock("../engine/wasmEngine", () => {
  const noop = vi.fn();
  return {
    setState: noop,
    onTransport: (listener: (state: string) => void) => {
      engineMock.transportListener = listener;
      listener("stopped");
      return () => {
        if (engineMock.transportListener === listener)
          engineMock.transportListener = null;
      };
    },
    onBankPlayback: (
      listener: (state: {
        activeBank: number;
        queuedBank: number;
        chainPosition: number;
      }) => void,
    ) => {
      listener({ activeBank: 0, queuedBank: -1, chainPosition: -1 });
      return noop;
    },
    onState: () => noop,
    onStep: (listener: (step: number) => void) => {
      engineMock.stepListener = listener;
      listener(-1);
      return () => {
        if (engineMock.stepListener === listener)
          engineMock.stepListener = null;
      };
    },
    play: vi.fn(() => Promise.resolve()),
    pause: noop,
    stop: noop,
    triggerVoice: noop,
    setTempo: noop,
    setSwing: noop,
    setStepCount: noop,
    setReverb: noop,
    setProbability: noop,
    setHumanize: noop,
    setCell: noop,
    setCellProbability: noop,
    setCellHumanize: noop,
    setCellCondition: noop,
    setCellRepeats: noop,
    setTrackLength: noop,
    setFillMode: noop,
    setMetronomeEnabled: noop,
    setCountInBars: noop,
    setPattern: noop,
    setPatternBank: noop,
    requestBank: noop,
    setChain: noop,
    setChainEnabled: noop,
    setVolume: noop,
    setDecay: noop,
    setMuted: noop,
    setVoiceParam: noop,
    setTomModel: noop,
    setPhysicalTomParam: noop,
  };
});

beforeEach(() => {
  localStorage.clear();
  window.history.replaceState(null, "", "/");
});

describe("DrumMachine live status", () => {
  it("announces engine readiness and transport transitions", () => {
    const { rerender } = render(<DrumMachine wasmLoaded={false} />);
    expect(screen.getByText("Audio engine loading.")).toBeInTheDocument();

    rerender(<DrumMachine wasmLoaded />);
    expect(
      screen.getByText("Audio engine ready. Transport stopped."),
    ).toBeInTheDocument();

    act(() => engineMock.transportListener?.("playing"));
    expect(
      screen.getByText("Audio engine ready. Transport playing."),
    ).toBeInTheDocument();
  });

  it("routes representative machine edits through reducer and engine sync", async () => {
    const user = userEvent.setup();
    render(<DrumMachine wasmLoaded />);

    const bass2 = screen.getByRole("button", { name: "Bass step 2" });
    await user.click(bass2);
    bass2.focus();
    await user.keyboard("{F2}");
    fireEvent.change(screen.getByRole("slider", { name: /^PROBABILITY/ }), {
      target: { value: "45" },
    });
    fireEvent.change(screen.getByRole("slider", { name: /^HUMANIZE/ }), {
      target: { value: "35" },
    });
    await user.selectOptions(
      screen.getByRole("combobox", { name: "CONDITION" }),
      "2",
    );
    await user.selectOptions(
      screen.getByRole("combobox", { name: "Ratchet repeats" }),
      "3",
    );
    await user.click(
      screen.getByRole("button", { name: "Close step settings" }),
    );

    await user.click(screen.getByRole("button", { name: "Mute Bass" }));
    await user.selectOptions(screen.getByLabelText("Bass pattern length"), "9");
    const bassVolume = screen.getByRole("slider", { name: "Bass volume" });
    bassVolume.focus();
    await user.keyboard("{ArrowDown}");
    const bassDecay = screen.getByRole("slider", { name: "Bass decay" });
    bassDecay.focus();
    await user.keyboard("{ArrowUp}");

    await user.click(screen.getByRole("button", { name: "Bass row actions" }));
    await user.click(screen.getByRole("button", { name: "Rotate right" }));

    const preset = screen.getByRole("combobox", {
      name: "Load preset pattern",
    });
    await user.selectOptions(preset, "0");
    await user.click(screen.getByRole("button", { name: "Mutate pattern" }));
    await user.click(screen.getByRole("button", { name: "Clear pattern" }));
    await user.click(
      screen.getByRole("button", { name: "Undo pattern action" }),
    );
    await user.click(
      screen.getByRole("button", { name: "Redo pattern action" }),
    );

    await user.click(screen.getByRole("button", { name: "B" }));
    await user.click(screen.getByText("COPY"));
    await user.click(
      screen.getByRole("button", { name: "Copy bank B to bank C" }),
    );
    await user.click(screen.getByRole("checkbox", { name: "CHAIN" }));

    await user.click(document.querySelector<HTMLButtonElement>(".dm-fill")!);
    await user.click(
      screen.getByRole("button", { name: "Playback metronome" }),
    );
    await user.selectOptions(
      screen.getByRole("combobox", { name: "Count-in bars" }),
      "1",
    );
    for (const name of [
      "Tempo",
      "SWING",
      "Pattern length",
      "Trigger probability",
      "Humanize",
      "REVERB",
    ]) {
      const slider = screen.getByRole("slider", { name });
      slider.focus();
      await user.keyboard("{PageDown}");
    }

    await user.click(
      screen.getByRole("button", { name: "Bass Drum voice settings" }),
    );
    const voiceDialog = screen.getByRole("dialog", { name: "Bass Drum voice" });
    const firstVoiceKnob = voiceDialog
      .getElementsByClassName("knob")[0]
      ?.querySelector<SVGSVGElement>("[role='slider']");
    expect(firstVoiceKnob).not.toBeNull();
    firstVoiceKnob?.focus();
    await user.keyboard("{ArrowRight}");
    await user.click(
      screen.getByRole("button", { name: "Audition Bass Drum" }),
    );
    await user.click(
      screen.getByRole("button", { name: "Reset Bass Drum voice to defaults" }),
    );
    await user.click(
      screen.getByRole("button", {
        name: "Close Bass Drum voice settings",
      }),
    );
  }, 30_000);
});
