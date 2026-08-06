// Tests for the audio worker's pull path.
//
// The worker is a module with side effects on its global scope (it installs
// onmessage and answers through postMessage), so the tests stub those globals
// and re-import it per case. There is no DOM and no WebAssembly engine here:
// the load handshake is driven through hand-built stand-ins for Go, fetch and
// WebAssembly, and AlgoDrum is a plain object.

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { AlgoDrumApi, WorkerCommand, WorkerResponse } from "./audioWorker";
import { createDefaultEngineState } from "./engineState";

const CHUNK_SAMPLES = 512;

// A MessagePort stand-in for the direct worker <-> worklet audio channel.
// Nothing is really transferred, which is what lets a recycled buffer be
// compared by identity with the one that was handed out.
class FakePort {
  onmessage: ((event: MessageEvent) => void) | null = null;

  readonly posted: { data: ChunkReply; transfer: Transferable[] }[] = [];

  postMessage(data: ChunkReply, transfer: Transferable[] = []): void {
    this.posted.push({ data, transfer });
  }

  // send delivers a worklet request to the worker.
  send(data: unknown): void {
    this.onmessage?.({ data } as MessageEvent);
  }

  // need issues one chunk request and returns the reply it produced.
  need(samples = CHUNK_SAMPLES): ChunkReply {
    const before = this.posted.length;
    this.send({ type: "need", samples });
    expect(this.posted).toHaveLength(before + 1);
    return this.posted[before].data;
  }
}

interface ChunkReply {
  buffer: ArrayBuffer;
  step: number;
  idle: boolean;
}

// The engine surface the worker calls. Only the pull path matters here; the
// rest exists so assertEngineApi accepts the object.
const OTHER_METHODS: (keyof AlgoDrumApi)[] = [
  "init",
  "setRunning",
  "pause",
  "setTempo",
  "setSwing",
  "setStepCount",
  "setCell",
  "setPattern",
  "setState",
  "setVolume",
  "setDecay",
  "setMuted",
  "setVoiceParam",
  "setPhysicalTomParam",
  "setTomModel",
  "triggerVoice",
  "setReverb",
  "setProbability",
  "setHumanize",
];

interface FakeEngine {
  step: number;
  idle: boolean;
  renders: number;
  render: (n: number) => Float32Array;
}

function fakeEngine(): FakeEngine {
  const engine: FakeEngine = {
    step: 0,
    idle: false,
    renders: 0,
    render(n: number) {
      engine.renders++;
      // Advancing here is what makes the "step captured before rendering"
      // assertion meaningful.
      engine.step++;
      return new Float32Array(n).fill(0.5);
    },
  };

  const api = engine as unknown as Record<string, unknown>;
  api.currentStep = () => engine.step;
  api.isIdle = () => engine.idle;
  api.getState = () => createDefaultEngineState();
  for (const name of OTHER_METHODS) api[name] = () => undefined;

  return engine;
}

// Responses the worker posted to the main thread.
let responses: WorkerResponse[];

function errors(): string[] {
  return responses
    .filter((r): r is Extract<WorkerResponse, { type: "error" }> => {
      return r.type === "error";
    })
    .map((r) => r.error);
}

// dispatch delivers a main-thread command to the worker.
function dispatch(command: WorkerCommand, ports: FakePort[] = []): void {
  const scope = globalThis as unknown as {
    onmessage: ((event: MessageEvent) => void) | null;
  };
  scope.onmessage?.({ data: command, ports } as unknown as MessageEvent);
}

// importWorker loads a fresh copy of the worker module. It installs its
// onmessage handler as an import side effect.
async function importWorker(): Promise<void> {
  vi.resetModules();
  await import("./audioWorker");
}

// A module the worker can dynamically import in place of wasm_exec.js: the Go
// runtime is stubbed on the global instead, so all this has to do is resolve.
const WASM_EXEC_STUB = new URL("./tomModel.ts", import.meta.url).href;

// connect brings the worker to readiness and returns the audio port.
async function connect(engine: FakeEngine): Promise<FakePort> {
  vi.stubGlobal("AlgoDrum", engine);
  await importWorker();

  dispatch({
    type: "load",
    wasmExecUrl: WASM_EXEC_STUB,
    wasmUrl: "https://example.test/algo_drum.wasm",
    sampleRate: 48000,
  });

  // Readiness is reached across the instantiate promise chain.
  await vi.waitFor(() => expect(responses).toContainEqual({ type: "ready" }));

  const port = new FakePort();
  dispatch({ type: "connect" }, [port]);
  return port;
}

beforeEach(() => {
  responses = [];
  vi.stubGlobal("postMessage", (message: WorkerResponse) => {
    responses.push(message);
  });
  vi.stubGlobal("onmessage", null);
  vi.stubGlobal(
    "Go",
    class {
      importObject = {};
      run(): Promise<void> {
        // The real runtime never returns; nothing awaits this.
        return new Promise<void>(() => undefined);
      }
    },
  );
  vi.stubGlobal("fetch", () =>
    Promise.resolve({ arrayBuffer: () => Promise.resolve(new ArrayBuffer(8)) }),
  );
  vi.stubGlobal("WebAssembly", {
    instantiateStreaming: () => Promise.reject(new Error("not streamable")),
    instantiate: () => Promise.resolve({ instance: {}, module: {} }),
  });
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("chunk requests", () => {
  it("answers a request that arrives before the engine is ready", async () => {
    // The worklet spends a request credit per pull and only gets it back when
    // a chunk arrives. Four unanswered pulls pin its credit at the maximum
    // and audio stops for good, so silence is the only acceptable failure.
    await importWorker();

    const port = new FakePort();
    dispatch({ type: "connect" }, [port]);

    const reply = port.need();

    expect(reply.buffer.byteLength).toBe(CHUNK_SAMPLES * 4);
    expect(new Float32Array(reply.buffer).some((v) => v !== 0)).toBe(false);
    expect(reply.step).toBe(-1);
    // Idle would suspend the AudioContext, and the worklet is not called
    // while suspended, so nothing could wake it again.
    expect(reply.idle).toBe(false);
  });

  it("answers with silence and reports once when rendering throws", async () => {
    const engine = fakeEngine();
    const port = await connect(engine);

    engine.render = () => {
      throw new Error("engine exploded");
    };

    const first = port.need();
    const second = port.need();

    expect(first.buffer.byteLength).toBe(CHUNK_SAMPLES * 4);
    expect(new Float32Array(second.buffer).some((v) => v !== 0)).toBe(false);
    expect(second.step).toBe(-1);

    // One report for the fault, not one per chunk: the worklet pulls ~94
    // times a second and would drown the main thread.
    expect(errors()).toEqual([
      expect.stringContaining("engine exploded") as unknown as string,
    ]);
  });

  it("tags the chunk with the step it starts on, not the one after", async () => {
    const engine = fakeEngine();
    const port = await connect(engine);
    engine.step = 3;

    expect(port.need().step).toBe(3);
    expect(port.need().step).toBe(4);
  });

  it("carries the engine's idle state", async () => {
    const engine = fakeEngine();
    const port = await connect(engine);

    expect(port.need().idle).toBe(false);

    engine.idle = true;
    expect(port.need().idle).toBe(true);
  });
});

describe("buffer recycling", () => {
  it("refills a returned buffer instead of allocating", async () => {
    const engine = fakeEngine();
    const port = await connect(engine);

    const first = port.need().buffer;
    port.send({ type: "recycle", buffer: first });

    expect(port.need().buffer).toBe(first);
  });

  it("allocates when the pool holds a different size", async () => {
    const engine = fakeEngine();
    const port = await connect(engine);

    const small = port.need(128).buffer;
    port.send({ type: "recycle", buffer: small });

    const large = port.need(CHUNK_SAMPLES).buffer;
    expect(large).not.toBe(small);
    expect(large.byteLength).toBe(CHUNK_SAMPLES * 4);
  });

  it("zeroes a reused buffer on the failure path", async () => {
    const engine = fakeEngine();
    const port = await connect(engine);

    // Fill a buffer with real audio, hand it back, then fail the next render:
    // pooled storage still holds the previous chunk's samples.
    const used = port.need().buffer;
    expect(new Float32Array(used).every((v) => v === 0.5)).toBe(true);
    port.send({ type: "recycle", buffer: used });

    engine.render = () => {
      throw new Error("engine exploded");
    };

    const reply = port.need();
    expect(reply.buffer).toBe(used);
    expect(new Float32Array(reply.buffer).some((v) => v !== 0)).toBe(false);
  });
});

describe("state echoes", () => {
  it.each([
    ["setState", [createDefaultEngineState()]],
    ["setTempo", [140]],
    ["setSwing", [0.2]],
    ["setStepCount", [12]],
    ["setCell", [0, 0, 1]],
    ["setPattern", [createDefaultEngineState().pattern]],
    ["setVolume", [0, 0.5]],
    ["setDecay", [0, 0.5]],
    ["setMuted", [0, true]],
    ["setVoiceParam", [0, 0, 0.5]],
    ["setPhysicalTomParam", [3, 0, 0.5]],
    ["setTomModel", [3, 1]],
    ["setReverb", [0.5]],
    ["setProbability", [0.5]],
    ["setHumanize", [0.5]],
  ] as const)("echoes one full state after %s", async (name, args) => {
    const engine = fakeEngine();
    await connect(engine);
    responses = [];

    dispatch({ type: "cmd", name, args: [...args] });

    expect(responses).toHaveLength(1);
    expect(responses[0]).toMatchObject({ type: "stateSync" });
  });

  it.each([
    ["setRunning", [true]],
    ["pause", []],
    ["triggerVoice", [0, 1]],
    ["render", [16]],
  ] as const)("does not echo operational command %s", async (name, args) => {
    const engine = fakeEngine();
    await connect(engine);
    responses = [];

    dispatch({ type: "cmd", name, args: [...args] });

    expect(responses).toEqual([]);
  });

  it("balances a mutation with an invalid sentinel when getState fails", async () => {
    const engine = fakeEngine();
    const api = engine as unknown as Record<string, unknown>;
    api.getState = () => ({ tempoBpm: 120 });
    await connect(engine);
    responses = [];

    dispatch({ type: "cmd", name: "setTempo", args: [140] });

    expect(errors()).toEqual([expect.stringContaining("getState failed")]);
    expect(responses).toContainEqual({ type: "stateSync", state: null });
  });
});
