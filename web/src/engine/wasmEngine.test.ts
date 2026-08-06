// Bridge tests for the load lifecycle. The module talks to a real Worker, so
// the tests stub the global with a fake one and re-import the module for each
// case (the bridge holds process-wide singletons: the worker, the ready flag,
// the command queue and the pattern mirror).

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { WorkerCommand, WorkerResponse } from "./audioWorker";
import { createDefaultEngineState } from "./engineState";

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

// Minimal stand-ins for the Web Audio graph the bridge builds on the first
// play(): enough surface for it to wire the worklet up, plus the hooks the
// tests need to emit steps and to observe teardown.
class FakePort {
  onmessage:
    ((event: MessageEvent<{ type: string; step: number }>) => void) | null =
    null;

  postMessage(): void {}
}

class FakeMessageChannel {
  readonly port1 = new FakePort();
  readonly port2 = new FakePort();
}

class FakeWorkletNode {
  static created: FakeWorkletNode[] = [];

  readonly port = new FakePort();
  disconnected = 0;

  constructor() {
    FakeWorkletNode.created.push(this);
  }

  connect(): void {}

  disconnect(): void {
    this.disconnected++;
  }

  // emit delivers one worklet report to the bridge.
  emit(data: unknown): void {
    this.port.onmessage?.({ data } as MessageEvent<{
      type: string;
      step: number;
    }>);
  }

  // step delivers one audible-step report from the worklet.
  step(step: number): void {
    this.emit({ type: "step", step });
  }
}

class FakeAudioContext {
  static created: FakeAudioContext[] = [];

  state = "running";
  closed = false;
  suspends = 0;
  resumes = 0;
  readonly destination = {};
  readonly audioWorklet = { addModule: () => Promise.resolve() };

  constructor() {
    FakeAudioContext.created.push(this);
  }

  resume(): Promise<void> {
    this.resumes++;
    this.state = "running";
    return Promise.resolve();
  }

  // Suspension takes effect when the promise settles, not when it is asked
  // for — that gap is exactly what a resume racing it has to survive.
  suspend(): Promise<void> {
    this.suspends++;

    // Several ticks deep on purpose: a resume that merely reads .state
    // without waiting the suspend out still sees "running", skips the
    // resume, and is then overtaken by the suspension.
    return Promise.resolve()
      .then(() => undefined)
      .then(() => undefined)
      .then(() => undefined)
      .then(() => {
        this.state = "suspended";
      });
  }

  close(): Promise<void> {
    this.closed = true;
    return Promise.resolve();
  }
}

const state = (tempoBpm = 120) => {
  const snapshot = createDefaultEngineState();
  snapshot.tempoBpm = tempoBpm;
  return snapshot;
};

const importEngine = async () => {
  vi.resetModules();
  return await import("./wasmEngine");
};

const workers = () => FakeWorker.created;

beforeEach(() => {
  FakeWorker.created = [];
  FakeWorkletNode.created = [];
  FakeAudioContext.created = [];
  vi.stubGlobal("Worker", FakeWorker);
  vi.stubGlobal("AudioContext", FakeAudioContext);
  vi.stubGlobal("AudioWorkletNode", FakeWorkletNode);
  vi.stubGlobal("MessageChannel", FakeMessageChannel);
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

  it.each([
    [
      "an error response",
      (w: FakeWorker) => w.emit({ type: "error", error: "wasm fetch failed" }),
    ],
    ["onerror", (w: FakeWorker) => w.fail("boom")],
    [
      "onmessageerror",
      (w: FakeWorker) => w.onmessageerror?.({} as MessageEvent),
    ],
  ])("terminates the worker when a load fails via %s", async (_label, fail) => {
    const engine = await importEngine();

    const load = engine.loadWasm();
    fail(workers()[0]);
    await expect(load).rejects.toThrow();

    // Teardown must happen with the failure, not lazily at the start of the
    // next attempt: a user who never retries would otherwise leave a broken
    // worker running for the life of the page.
    expect(workers()[0].terminated).toBe(true);
  });

  it("terminates the worker when the load times out", async () => {
    vi.useFakeTimers();
    const engine = await importEngine();

    const load = engine.loadWasm();
    vi.advanceTimersByTime(30000);
    await expect(load).rejects.toThrow(/did not start within/);

    expect(workers()[0].terminated).toBe(true);
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
    engine.onState(listener);

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
    workers()[1].emit({ type: "stateSync", state: state(130) });
    expect(listener).not.toHaveBeenCalled();

    const current = state(140);
    workers()[1].emit({ type: "stateSync", state: current });
    expect(listener).toHaveBeenCalledExactlyOnceWith(current);
  });

  it("reconciles every configuration setter but not operational commands", async () => {
    const engine = await loaded();
    const listener = vi.fn();
    engine.onState(listener);

    engine.setTempo(999);
    const clamped = state(300);
    workers()[0].emit({ type: "stateSync", state: clamped });
    expect(listener).toHaveBeenCalledExactlyOnceWith(clamped);

    await engine.play();
    workers()[0].emit({ type: "stateSync", state: state(200) });
    // Play is operational and therefore does not create in-flight state that
    // could suppress an unsolicited authoritative sync.
    expect(listener).toHaveBeenCalledTimes(2);
  });

  it("rejects invalid bulk state before counting or queueing it", async () => {
    const engine = await loaded();
    const invalid = state();
    invalid.pattern = new Float32Array(1);

    expect(() => engine.setState(invalid)).toThrow(TypeError);
    expect(workers()[0].commands()).toHaveLength(0);

    const unsolicited = state(150);
    const listener = vi.fn();
    engine.onState(listener);
    workers()[0].emit({ type: "stateSync", state: unsolicited });
    expect(listener).toHaveBeenCalledExactlyOnceWith(unsolicited);
  });

  it("owns the typed arrays queued by setState", async () => {
    const engine = await loaded();
    const supplied = state();

    engine.setState(supplied);
    const posted = workers()[0].commands()[0];
    const snapshot = posted.args[0] as typeof supplied;

    expect(snapshot).not.toBe(supplied);
    expect(snapshot.pattern).not.toBe(supplied.pattern);
    expect(snapshot.tracks[0].voiceParams).not.toBe(
      supplied.tracks[0].voiceParams,
    );
    expect(snapshot.tracks[3].tom.physicalParams).not.toBe(
      supplied.tracks[3].tom.physicalParams,
    );
  });

  it("maps the named Tom model to the WASM model code", async () => {
    const engine = await loaded();

    engine.setTomModel(3, "physical");
    engine.setTomModel(5, "procedural");

    expect(workers()[0].commands()).toEqual([
      { type: "cmd", name: "setTomModel", args: [3, 1] },
      { type: "cmd", name: "setTomModel", args: [5, 0] },
    ]);
  });

  it("sends physical Tom parameter edits to the independent bank", async () => {
    const engine = await loaded();

    engine.setPhysicalTomParam(5, 4, 0.625);

    expect(workers()[0].commands()).toEqual([
      { type: "cmd", name: "setPhysicalTomParam", args: [5, 4, 0.625] },
    ]);
  });
});

// A ready engine. The audio graph is built lazily by the first play().
const loaded = async () => {
  const engine = await importEngine();
  const load = engine.loadWasm();
  workers()[0].emit({ type: "ready" });
  await load;
  return engine;
};

const node = () => FakeWorkletNode.created[0];

describe("playhead", () => {
  // Recorded steps, minus the -1 onStep replays to every new subscriber.
  const record = (engine: { onStep: (fn: (step: number) => void) => void }) => {
    const steps: number[] = [];
    engine.onStep((step) => steps.push(step));
    return steps;
  };

  it("ignores steps still draining from a stop", async () => {
    const engine = await loaded();
    const steps = record(engine);

    await engine.play();
    node().step(0);
    node().step(1);
    engine.stop();

    // ~43 ms of already-rendered audio is still queued in the worklet, tagged
    // with the steps it was rendered on. Reporting those would flash the
    // playhead backwards after the user pressed Stop.
    node().step(2);
    node().step(3);

    expect(steps).toEqual([-1, 0, 1, -1]);
  });

  it("pauses without entering the stop-drain state", async () => {
    const engine = await loaded();
    const steps = record(engine);

    await engine.play();
    node().step(2);
    engine.pause();
    node().step(3);

    expect(workers()[0].commands().slice(-2)).toEqual([
      { type: "cmd", name: "setRunning", args: [true] },
      { type: "cmd", name: "pause", args: [] },
    ]);
    expect(steps).toEqual([-1, 2, 3]);
  });

  it("resumes reporting after the engine's first stopped chunk", async () => {
    const engine = await loaded();
    const steps = record(engine);

    await engine.play();
    node().step(0);
    engine.stop();
    node().step(1); // stale
    node().step(-1); // rendered after the stop: the drain is over

    await engine.play();
    node().step(0);

    expect(steps).toEqual([-1, 0, -1, 0]);
  });

  it("does not strand the playhead when play beats the drain", async () => {
    const engine = await loaded();
    const steps = record(engine);

    await engine.play();
    node().step(4);
    engine.stop();

    // Restarted inside the drain window, so the engine never renders the -1
    // chunk that would otherwise end the suppression.
    await engine.play();
    node().step(0);
    node().step(1);

    expect(steps).toEqual([-1, 4, -1, 0, 1]);
  });
});

const ctx = () => FakeAudioContext.created[0];

// flush lets the suspend/resume promise chains settle.
const flush = async () => {
  for (let i = 0; i < 10; i++) await Promise.resolve();
};

describe("idle suspend", () => {
  it("suspends the context once the engine has gone quiet", async () => {
    const engine = await loaded();
    await engine.play();
    engine.stop();

    // The engine only reports idle after its output has actually fallen
    // silent, so the reverb and voice tails are already through the worklet
    // by the time this arrives.
    node().emit({ type: "idle", idle: true });
    await flush();

    expect(ctx().suspends).toBe(1);
    expect(ctx().state).toBe("suspended");
  });

  it("keeps the context awake while the transport is running", async () => {
    const engine = await loaded();
    await engine.play();

    // A silent passage mid-pattern must not stop the pull loop: the playhead
    // would freeze with it.
    node().emit({ type: "idle", idle: true });
    await flush();

    expect(ctx().suspends).toBe(0);
  });

  it("resumes on the next play", async () => {
    const engine = await loaded();
    await engine.play();
    engine.stop();
    node().emit({ type: "idle", idle: true });
    await flush();

    await engine.play();

    expect(ctx().state).toBe("running");
    expect(ctx().resumes).toBe(1);
  });

  it("resumes for an audition", async () => {
    const engine = await loaded();
    await engine.play();
    engine.stop();
    node().emit({ type: "idle", idle: true });
    await flush();

    await engine.triggerVoice(0, 1);

    expect(ctx().state).toBe("running");
    expect(workers()[0].commands().slice(-1)).toEqual([
      {
        type: "cmd",
        name: "triggerVoice",
        args: [0, 1],
      },
    ]);
  });

  it("does not let an in-flight suspend outlive the play that resumes it", async () => {
    const engine = await loaded();
    await engine.play();
    engine.stop();

    // Idle report and Play in the same tick: without waiting the suspend out,
    // the resume can land first and leave a suspended context that nothing
    // will ever wake — the worklet is not called while suspended.
    node().emit({ type: "idle", idle: true });
    await engine.play();

    // Let the suspend land wherever it was going to: a resume that skipped
    // it because the state still read "running" is overtaken right here.
    await flush();

    expect(ctx().state).toBe("running");
  });
});

describe("underruns", () => {
  it("publishes dropout reports to subscribers", async () => {
    const engine = await loaded();
    const reports: { samples: number; count: number }[] = [];
    engine.onUnderrun((report) => reports.push(report));
    vi.spyOn(console, "warn").mockImplementation(() => undefined);

    await engine.play();
    node().emit({ type: "underrun", samples: 384, count: 3 });

    expect(reports).toEqual([{ samples: 384, count: 3 }]);
  });
});

describe("dispose", () => {
  it("tears down the audio graph and the worker", async () => {
    const engine = await loaded();
    await engine.play();

    engine.dispose();

    expect(workers()[0].terminated).toBe(true);
    expect(node().disconnected).toBe(1);
    expect(node().port.onmessage).toBeNull();
    expect(FakeAudioContext.created[0].closed).toBe(true);
  });

  it("darkens the playhead", async () => {
    const engine = await loaded();
    await engine.play();
    node().step(3);

    const steps: number[] = [];
    engine.onStep((step) => steps.push(step));
    engine.dispose();

    expect(steps).toEqual([3, -1]);
  });

  it("lets a later load start a fresh engine", async () => {
    const engine = await loaded();
    engine.dispose();

    const reload = engine.loadWasm();
    expect(workers()).toHaveLength(2);

    workers()[1].emit({ type: "ready" });
    await expect(reload).resolves.toBeUndefined();
  });

  it("settles a load that was still in flight", async () => {
    const engine = await importEngine();

    // The worker dispose() terminates can never answer, so without an
    // explicit rejection the app would sit on "Loading engine…" forever.
    const load = engine.loadWasm();
    engine.dispose();

    await expect(load).rejects.toThrow(/disposed/);
    expect(workers()[0].terminated).toBe(true);
  });

  it("does not let a cancelled load tear down its replacement", async () => {
    const engine = await importEngine();

    const cancelled = engine.loadWasm();
    engine.dispose();

    // Started synchronously, before the rejection above is delivered.
    const reload = engine.loadWasm();
    await expect(cancelled).rejects.toThrow(/disposed/);

    expect(workers()).toHaveLength(2);
    expect(workers()[1].terminated).toBe(false);

    workers()[1].emit({ type: "ready" });
    await expect(reload).resolves.toBeUndefined();
  });

  it("drops queued commands instead of replaying them to a new engine", async () => {
    const engine = await importEngine();

    engine.setTempo(140);
    engine.dispose();

    const load = engine.loadWasm();
    workers()[0].emit({ type: "ready" });
    await load;

    expect(workers()[0].commands()).toHaveLength(0);
  });
});
