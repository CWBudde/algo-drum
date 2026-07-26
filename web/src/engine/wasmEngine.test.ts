// Bridge tests for the load lifecycle. The module talks to a real Worker, so
// the tests stub the global with a fake one and re-import the module for each
// case (the bridge holds process-wide singletons: the worker, the ready flag,
// the command queue and the pattern mirror).

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { PATTERN_SIZE } from "../algo/pattern";
import type { WorkerCommand, WorkerResponse } from "./audioWorker";

class FakeWorker {
  static created: FakeWorker[] = [];

  onmessage: ((event: MessageEvent<WorkerResponse>) => void) | null = null;
  onerror: ((event: ErrorEvent) => void) | null = null;
  onmessageerror: ((event: MessageEvent) => void) | null = null;

  readonly posted: WorkerCommand[] = [];
  terminated = false;

  constructor() {
    FakeWorker.created.push(this);
  }

  postMessage(message: WorkerCommand): void {
    this.posted.push(message);
  }

  terminate(): void {
    this.terminated = true;
  }

  // emit delivers a worker response to the bridge.
  emit(response: WorkerResponse): void {
    this.onmessage?.({ data: response } as MessageEvent<WorkerResponse>);
  }

  fail(message: string): void {
    this.onerror?.({ message } as ErrorEvent);
  }

  // commands returns the engine calls this worker actually received.
  commands(): Extract<WorkerCommand, { type: "cmd" }>[] {
    return this.posted.filter(
      (message): message is Extract<WorkerCommand, { type: "cmd" }> =>
        message.type === "cmd",
    );
  }
}

// A full-size pattern echo whose leading cells hold the given velocities.
const pattern = (...values: number[]): Float32Array => {
  const flat = new Float32Array(PATTERN_SIZE);
  flat.set(values);
  return flat;
};

const importEngine = async () => {
  vi.resetModules();
  return await import("./wasmEngine");
};

const workers = () => FakeWorker.created;

beforeEach(() => {
  FakeWorker.created = [];
  vi.stubGlobal("Worker", FakeWorker);
  vi.stubGlobal("self", { location: { href: "https://example.test/app/" } });
  // The bridge logs engine errors; keep the expected ones out of the output.
  vi.spyOn(console, "error").mockImplementation(() => undefined);
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  vi.useRealTimers();
});

describe("loadWasm", () => {
  it("resolves once the worker reports the engine ready", async () => {
    const engine = await importEngine();

    const load = engine.loadWasm();
    workers()[0].emit({ type: "ready" });

    await expect(load).resolves.toBeUndefined();
    expect(workers()).toHaveLength(1);
  });

  it("shares one attempt between concurrent callers", async () => {
    const engine = await importEngine();

    // React StrictMode calls the mount effect twice; a second worker here
    // would terminate the first and strand its promise.
    const first = engine.loadWasm();
    const second = engine.loadWasm();
    expect(workers()).toHaveLength(1);

    workers()[0].emit({ type: "ready" });

    await expect(first).resolves.toBeUndefined();
    await expect(second).resolves.toBeUndefined();
    expect(workers()[0].terminated).toBe(false);
  });

  it("retries with a fresh worker after a failed attempt", async () => {
    const engine = await importEngine();

    const failed = engine.loadWasm();
    workers()[0].emit({ type: "error", error: "wasm fetch failed" });
    await expect(failed).rejects.toThrow("wasm fetch failed");

    const retry = engine.loadWasm();
    expect(workers()).toHaveLength(2);
    expect(workers()[0].terminated).toBe(true);

    workers()[1].emit({ type: "ready" });
    await expect(retry).resolves.toBeUndefined();
  });

  it("rejects when the worker errors without sending a message", async () => {
    const engine = await importEngine();

    // A module that 404s or throws at its top level never reaches the
    // worker's message handler, so only onerror can settle the load.
    const load = engine.loadWasm();
    workers()[0].fail("Failed to fetch worker module");

    await expect(load).rejects.toThrow("Failed to fetch worker module");
  });

  it("rejects on an undeliverable message", async () => {
    const engine = await importEngine();

    const load = engine.loadWasm();
    workers()[0].onmessageerror?.({} as MessageEvent);

    await expect(load).rejects.toThrow(/undeliverable/);
  });

  it("rejects when a silent worker never becomes ready", async () => {
    vi.useFakeTimers();
    const engine = await importEngine();

    const load = engine.loadWasm();
    vi.advanceTimersByTime(30000);

    await expect(load).rejects.toThrow(/did not start within/);
  });

  it("does not time out a worker that became ready", async () => {
    vi.useFakeTimers();
    const engine = await importEngine();

    const load = engine.loadWasm();
    workers()[0].emit({ type: "ready" });
    await load;

    // The pending timer must be cleared, or it would reject a settled
    // promise and (worse) keep the process awake.
    expect(vi.getTimerCount()).toBe(0);
  });
});

describe("command queue", () => {
  it("flushes commands issued before the engine was ready", async () => {
    const engine = await importEngine();

    engine.setTempo(140);
    const load = engine.loadWasm();
    expect(workers()[0].commands()).toHaveLength(0);

    workers()[0].emit({ type: "ready" });
    await load;

    expect(workers()[0].commands()).toEqual([
      { type: "cmd", name: "setTempo", args: [140] },
    ]);
  });

  it("replays queued edits to the retry worker without reverting newer ones", async () => {
    const engine = await importEngine();
    const listener = vi.fn();
    engine.onPattern(listener);

    // Edit A is queued against a worker that then fails to load.
    engine.setCell(0, 0, 0.7);
    const failed = engine.loadWasm();
    workers()[0].emit({ type: "error", error: "boom" });
    await expect(failed).rejects.toThrow("boom");

    // Retry, then edit B before the replacement worker is ready.
    const retry = engine.loadWasm();
    engine.setCell(1, 1, 1.0);
    workers()[1].emit({ type: "ready" });
    await retry;

    // Both edits are replayed, so both are echoed exactly once.
    expect(workers()[1].commands()).toEqual([
      { type: "cmd", name: "setCell", args: [0, 0, 0.7] },
      { type: "cmd", name: "setCell", args: [1, 1, 1.0] },
    ]);

    // The echo for A alone must not be published: it predates B.
    workers()[1].emit({ type: "patternSync", pattern: pattern(0.7) });
    expect(listener).not.toHaveBeenCalled();

    workers()[1].emit({ type: "patternSync", pattern: pattern(0.7, 1.0) });
    expect(listener).toHaveBeenCalledExactlyOnceWith(pattern(0.7, 1.0));
  });
});
