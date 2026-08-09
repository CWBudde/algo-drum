import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { euclid } from "../algo/euclid";
import { PATTERN_SIZE } from "../algo/pattern";
import { mutate } from "../algo/mutate";
import { PRESETS, presetToFlat } from "../algo/presets";
import AlgoPanel from "./AlgoPanel";
import { fillEuclidTrack } from "./patternView";

function panelProps(pattern = new Array<number>(PATTERN_SIZE).fill(0)) {
  return {
    disabled: false,
    pattern,
    stepCount: 16,
    onApplyPattern: vi.fn(),
    canUndo: true,
    canRedo: true,
    onUndo: vi.fn(),
    onRedo: vi.fn(),
    onShare: vi.fn(() => "https://example.test/algo-drum/#state"),
  };
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe("AlgoPanel actions", () => {
  it("applies a selected preset and announces its name", async () => {
    const user = userEvent.setup();
    const props = panelProps();
    render(<AlgoPanel {...props} />);

    await user.selectOptions(
      screen.getByRole("combobox", { name: "Load preset pattern" }),
      "0",
    );

    expect(props.onApplyPattern).toHaveBeenCalledWith(
      presetToFlat(PRESETS[0]!),
    );
    expect(screen.getByRole("status")).toHaveTextContent("Rock preset loaded.");
  });

  it("mutates and clears with deterministic payloads and announcements", async () => {
    const user = userEvent.setup();
    const pattern = presetToFlat(PRESETS[0]!);
    const props = panelProps(pattern);
    vi.spyOn(Math, "random").mockReturnValue(0.9);
    const expectedMutation = mutate(pattern, {
      stepCount: 16,
      rng: () => 0.9,
    });
    render(<AlgoPanel {...props} />);

    await user.click(screen.getByRole("button", { name: "Mutate pattern" }));
    expect(props.onApplyPattern).toHaveBeenLastCalledWith(expectedMutation);
    expect(screen.getByRole("status")).toHaveTextContent("Pattern mutated.");

    await user.click(screen.getByRole("button", { name: "Clear pattern" }));
    expect(props.onApplyPattern).toHaveBeenLastCalledWith(
      new Array<number>(PATTERN_SIZE).fill(0),
    );
    expect(screen.getByRole("status")).toHaveTextContent("Pattern cleared.");
  });

  it("fills the chosen Euclidean track and announces history actions", async () => {
    const user = userEvent.setup();
    const props = panelProps();
    render(<AlgoPanel {...props} />);

    await user.click(screen.getByText("EUCLID", { exact: true }));
    await user.selectOptions(screen.getByLabelText("TRACK"), "1");
    await user.clear(screen.getByLabelText("HITS"));
    await user.type(screen.getByLabelText("HITS"), "4");
    await user.clear(screen.getByLabelText("ROTATE"));
    await user.type(screen.getByLabelText("ROTATE"), "0");
    await user.click(screen.getByRole("button", { name: "FILL" }));

    expect(props.onApplyPattern).toHaveBeenLastCalledWith(
      fillEuclidTrack(props.pattern, 1, euclid(4, 16, 0)),
    );
    expect(screen.getByRole("status")).toHaveTextContent(
      "Snare filled with 4 Euclidean hits.",
    );

    await user.click(
      screen.getByRole("button", { name: "Undo pattern action" }),
    );
    expect(props.onUndo).toHaveBeenCalledOnce();
    expect(screen.getByRole("status")).toHaveTextContent(
      "Pattern action undone.",
    );

    await user.click(
      screen.getByRole("button", { name: "Redo pattern action" }),
    );
    expect(props.onRedo).toHaveBeenCalledOnce();
    expect(screen.getByRole("status")).toHaveTextContent(
      "Pattern action redone.",
    );
  });
});
