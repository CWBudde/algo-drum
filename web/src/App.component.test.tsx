import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import App from "./App";
import ErrorBoundary from "./components/ErrorBoundary";

const engineMock = vi.hoisted(() => ({
  loadWasm: vi.fn<() => Promise<void>>(),
  failureListener: null as ((error: Error) => void) | null,
  dispose: vi.fn(),
}));

vi.mock("./engine/wasmEngine", () => ({
  loadWasm: engineMock.loadWasm,
  dispose: engineMock.dispose,
  onFailure: (listener: (error: Error) => void) => {
    engineMock.failureListener = listener;
    return () => {
      if (engineMock.failureListener === listener)
        engineMock.failureListener = null;
    };
  },
}));

vi.mock("./components/DrumMachine", () => ({
  default: ({ wasmLoaded }: { wasmLoaded: boolean }) => (
    <div>{wasmLoaded ? "Machine ready" : "Machine waiting"}</div>
  ),
}));

beforeEach(() => {
  engineMock.loadWasm.mockReset();
  engineMock.dispose.mockReset();
  engineMock.failureListener = null;
});

describe("App engine lifecycle", () => {
  it("moves from loading to ready, handles a runtime failure, and retries", async () => {
    const user = userEvent.setup();
    engineMock.loadWasm.mockResolvedValue(undefined);
    render(<App />);

    expect(screen.getByRole("status")).toHaveTextContent(
      "Loading audio engine",
    );
    expect(await screen.findByText("Machine ready")).toBeInTheDocument();

    act(() => engineMock.failureListener?.(new Error("worker stopped")));
    expect(screen.getByRole("alert")).toHaveTextContent("worker stopped");
    await user.click(screen.getByRole("button", { name: "Retry" }));
    expect(await screen.findByText("Machine ready")).toBeInTheDocument();
  });

  it("renders a useful fallback for an unhelpful rejected value", async () => {
    engineMock.loadWasm.mockRejectedValue({});
    render(<App />);
    expect(await screen.findByRole("alert")).toHaveTextContent("Unknown error");
  });
});

describe("ErrorBoundary", () => {
  it("replaces a crashed child and reports the captured error", () => {
    const onError = vi.fn();
    const consoleError = vi
      .spyOn(console, "error")
      .mockImplementation(() => {});
    function Crash(): never {
      throw new Error("render exploded");
    }

    render(
      <ErrorBoundary onError={onError}>
        <Crash />
      </ErrorBoundary>,
    );
    expect(screen.getByRole("alert")).toHaveTextContent("render exploded");
    expect(screen.getByRole("button", { name: "Reload" })).toBeEnabled();
    expect(onError).toHaveBeenCalledOnce();
    consoleError.mockRestore();
  });
});
