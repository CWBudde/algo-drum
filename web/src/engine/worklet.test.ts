// Tests for the AudioWorklet processor.
//
// worklet.js has to stay in public/ — it is loaded by addModule() at runtime,
// not bundled — so it is imported here by URL after AudioWorkletProcessor and
// registerProcessor have been stubbed on the global. It is a plain script with
// no imports, so an ESM import evaluates it exactly as the worklet scope does.

import { afterEach, describe, expect, it, vi } from "vitest";

const QUANTUM = 128; // samples per process() call
const CHUNK_SAMPLES = 512; // what the processor asks the worker for
const TARGET_CHUNKS = 4; // TARGET_QUEUE_SAMPLES / CHUNK_SAMPLES
const MAX_STARVED_QUANTA = 50;
const UNDERRUN_REPORT_QUANTA = 128;

interface Posted {
  data: Record<string, unknown>;
  transfer: Transferable[];
}

class FakePort {
  onmessage: ((event: MessageEvent) => void) | null = null;

  readonly posted: Posted[] = [];

  postMessage(data: Record<string, unknown>, transfer: Transferable[] = []) {
    this.posted.push({ data, transfer });
  }

  // of returns the messages of one type, oldest first.
  of(type: string): Record<string, unknown>[] {
    return this.posted.filter((m) => m.data.type === type).map((m) => m.data);
  }
}

// The base class the processor extends: all it contributes is the port to the
// main thread.
class FakeAudioWorkletProcessor {
  readonly port = new FakePort();
}

interface Processor {
  port: FakePort;
  pendingRequests: number;
  process(inputs: unknown[], outputs: Float32Array[][]): boolean;
}

// A connected processor plus the two ports it talks over.
interface Harness {
  proc: Processor;
  node: FakePort; // to the main thread
  worker: FakePort; // to the audio worker

  // deliver hands the processor one rendered chunk, as the worker would.
  deliver(
    step: number,
    idle?: boolean,
    state?: "stopped" | "starting" | "playing" | "paused",
    revision?: number,
  ): ArrayBuffer;

  // run calls process() the given number of times and returns the last
  // output block.
  run(quanta?: number): Float32Array;

  // needs counts the chunk requests issued so far.
  needs(): number;
}

async function harness(): Promise<Harness> {
  vi.resetModules();

  const registered: { ctor?: new () => Processor } = {};

  // Must be in place before the module body runs: the class declaration
  // extends AudioWorkletProcessor at evaluation time.
  vi.stubGlobal("AudioWorkletProcessor", FakeAudioWorkletProcessor);
  vi.stubGlobal(
    "registerProcessor",
    (_name: string, ctor: new () => Processor) => {
      registered.ctor = ctor;
    },
  );

  const url = new URL("../../public/worklet.js", import.meta.url).href;
  await import(/* @vite-ignore */ url);

  const Ctor = registered.ctor;
  if (!Ctor) throw new Error("worklet.js did not register a processor");

  const proc = new Ctor();
  const node = proc.port;
  const worker = new FakePort();

  node.onmessage?.({
    data: { type: "workerPort" },
    ports: [worker],
  } as unknown as MessageEvent);

  return {
    proc,
    node,
    worker,
    deliver(
      step: number,
      idle = false,
      state: "stopped" | "starting" | "playing" | "paused" = step < 0
        ? "stopped"
        : "playing",
      revision = state === "stopped" ? 0 : 1,
    ): ArrayBuffer {
      const buffer = new ArrayBuffer(CHUNK_SAMPLES * 4);
      new Float32Array(buffer).fill(0.25);
      worker.onmessage?.({
        data: { buffer, transport: { state, step, revision }, idle },
      } as unknown as MessageEvent);
      return buffer;
    },
    run(quanta = 1): Float32Array {
      let out = new Float32Array(QUANTUM);
      for (let i = 0; i < quanta; i++) {
        out = new Float32Array(QUANTUM);
        proc.process([], [[out]]);
      }
      return out;
    },
    needs(): number {
      return worker.of("need").length;
    },
  };
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("chunk requests", () => {
  it("fills the queue target on connect", async () => {
    const h = await harness();

    expect(h.needs()).toBe(TARGET_CHUNKS);
    expect(h.worker.of("need")[0]).toEqual({
      type: "need",
      samples: CHUNK_SAMPLES,
    });
  });

  it("plays delivered samples and asks for a replacement", async () => {
    const h = await harness();
    h.deliver(0);

    const out = h.run();

    expect(Array.from(out.slice(0, 3))).toEqual([0.25, 0.25, 0.25]);
    // The credit paid back by the delivery, minus what was just played, puts
    // the queue back under target, so exactly one chunk is re-requested.
    expect(h.needs()).toBe(TARGET_CHUNKS + 1);
    expect(h.proc.pendingRequests).toBe(TARGET_CHUNKS);
  });
});

describe("credit watchdog", () => {
  it("recovers when every request is dropped", async () => {
    const h = await harness();

    // Four requests, four replies that never come: the credit accounting
    // believes 2048 samples are on their way forever, so without the
    // watchdog nothing is ever requested again and audio stops for good.
    expect(h.needs()).toBe(TARGET_CHUNKS);
    expect(h.proc.pendingRequests).toBe(TARGET_CHUNKS);

    h.run(MAX_STARVED_QUANTA - 1);
    expect(h.needs()).toBe(TARGET_CHUNKS);

    h.run();
    expect(h.proc.pendingRequests).toBe(TARGET_CHUNKS);
    expect(h.needs()).toBe(TARGET_CHUNKS * 2);
  });

  it("does not write off credit while chunks are playing", async () => {
    const h = await harness();

    // A chunk lasts four quanta, so this outlives the watchdog window
    // several times over without ever starving.
    for (let i = 0; i < MAX_STARVED_QUANTA * 2; i++) {
      if (i % 4 === 0) h.deliver(i);
      h.run();
    }

    expect(h.proc.pendingRequests).toBeLessThanOrEqual(TARGET_CHUNKS);
    expect(h.node.of("underrun")).toHaveLength(0);
  });
});

describe("underrun reporting", () => {
  it("reports the first dropout immediately", async () => {
    const h = await harness();

    h.run();

    expect(h.node.of("underrun")).toEqual([
      { type: "underrun", samples: QUANTUM, count: 1 },
    ]);
  });

  it("aggregates the rest behind a throttle", async () => {
    const h = await harness();

    h.run(UNDERRUN_REPORT_QUANTA);
    expect(h.node.of("underrun")).toHaveLength(1);

    // The report at the throttle boundary carries everything since the first.
    h.run();
    expect(h.node.of("underrun")).toEqual([
      { type: "underrun", samples: QUANTUM, count: 1 },
      {
        type: "underrun",
        samples: QUANTUM * UNDERRUN_REPORT_QUANTA,
        count: UNDERRUN_REPORT_QUANTA,
      },
    ]);
  });

  it("stays quiet while the queue keeps up", async () => {
    const h = await harness();
    h.deliver(0);

    h.run(4); // exactly one chunk

    expect(h.node.of("underrun")).toHaveLength(0);
  });
});

describe("buffer recycling", () => {
  it("returns a drained chunk's storage to the worker", async () => {
    const h = await harness();
    const buffer = h.deliver(0);

    h.run(3);
    expect(h.worker.of("recycle")).toHaveLength(0);

    h.run();
    expect(h.worker.of("recycle")).toEqual([{ type: "recycle", buffer }]);

    // Transferred, not copied — the point is to stop allocating ~94 chunks a
    // second and throwing every one of them away.
    const recycle = h.worker.posted.find((m) => m.data.type === "recycle");
    expect(recycle?.transfer).toEqual([buffer]);
  });
});

describe("transport and idle reporting", () => {
  it("reports each new transport snapshot as it becomes audible", async () => {
    const h = await harness();
    h.deliver(0);
    h.deliver(0);
    h.deliver(1);

    h.run(12);

    expect(h.node.of("transport")).toEqual([
      {
        type: "transport",
        transport: { state: "playing", step: 0, revision: 1 },
      },
      {
        type: "transport",
        transport: { state: "playing", step: 1, revision: 1 },
      },
    ]);
  });

  it("reports a revision change even when state and step repeat", async () => {
    const h = await harness();
    h.deliver(0, false, "playing", 1);
    h.deliver(0, false, "playing", 3);

    h.run(8);

    expect(h.node.of("transport")).toEqual([
      {
        type: "transport",
        transport: { state: "playing", step: 0, revision: 1 },
      },
      {
        type: "transport",
        transport: { state: "playing", step: 0, revision: 3 },
      },
    ]);
  });

  it("reports idle edges only", async () => {
    const h = await harness();
    h.deliver(0, false);
    h.deliver(-1, true);
    h.deliver(-1, true);
    h.deliver(-1, false);

    h.run(16);

    expect(h.node.of("idle")).toEqual([
      { type: "idle", idle: false },
      { type: "idle", idle: true },
      { type: "idle", idle: false },
    ]);
  });
});
