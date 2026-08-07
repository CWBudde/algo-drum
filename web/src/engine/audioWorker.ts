// Dedicated worker that hosts the Go WASM drum engine.
//
// Running the engine here keeps audio rendering off the main thread: the
// AudioWorklet requests chunks over a direct MessagePort, and UI parameter
// changes arrive as command messages from the main thread.

import {
  CONFIGURATION_METHODS,
  isConfigurationMethod,
  validateEngineState,
  type EngineState,
  type PatternBankState,
} from "./engineState";

// The engine's JS surface. Exported so the main-thread bridge can type its
// command sender against it (method name and argument tuple) instead of
// casting into the wire format.
export interface AlgoDrumApi {
  init: (sampleRate: number) => void;
  beginStart: () => void;
  setRunning: (playing: boolean) => void;
  pause: () => void;
  setTempo: (bpm: number) => void;
  setSwing: (swing: number) => void;
  setStepCount: (bank: number, steps: number) => void;
  setCell: (
    bank: number,
    track: number,
    step: number,
    velocity: number,
  ) => void;
  setCellProbability: (
    bank: number,
    track: number,
    step: number,
    probability: number,
  ) => void;
  setCellHumanize: (
    bank: number,
    track: number,
    step: number,
    humanize: number,
  ) => void;
  setCellCondition: (
    bank: number,
    track: number,
    step: number,
    condition: number,
  ) => void;
  setCellRepeats: (
    bank: number,
    track: number,
    step: number,
    repeats: number,
  ) => void;
  setTrackLength: (bank: number, track: number, length: number) => void;
  setFillMode: (enabled: boolean) => void;
  setPattern: (bank: number, pattern: Float32Array) => void;
  setPatternBank: (bank: number, state: PatternBankState) => void;
  requestBank: (bank: number) => void;
  setChain: (chain: Uint8Array) => void;
  setChainEnabled: (enabled: boolean) => void;
  setState: (state: EngineState) => void;
  getState: () => EngineState;
  setVolume: (track: number, vol: number) => void;
  setDecay: (track: number, amount: number) => void;
  setMuted: (track: number, muted: boolean) => void;
  setVoiceParam: (track: number, index: number, value: number) => void;
  setPhysicalTomParam: (track: number, index: number, value: number) => void;
  setTomModel: (track: number, model: number) => void;
  triggerVoice: (track: number, velocity: number) => void;
  setReverb: (amount: number) => void;
  setProbability: (p: number) => void;
  setHumanize: (h: number) => void;
  render: (n: number) => Float32Array;
  currentStep: () => number;
  transportState: () => TransportState;
  transportRevision: () => number;
  activeBank: () => number;
  queuedBank: () => number;
  chainPosition: () => number;
  isIdle: () => boolean;
}

// Transport is runtime state rather than persisted EngineState, but it follows
// the same ownership rule: Go is authoritative and every other layer mirrors
// a validated snapshot. The revision distinguishes queued chunks from before
// and after a transition even when their state names happen to match.
export type TransportState = "stopped" | "starting" | "playing" | "paused";

export interface TransportSnapshot {
  state: TransportState;
  step: number;
  revision: number;
  activeBank: number;
  queuedBank: number;
  chainPosition: number;
}

// The semantics of the API above, not just its shape. REQUIRED_METHODS below
// checks that method *names* exist; it cannot see a changed argument order, a
// changed unit, or a changed EngineState — an engine that drifted that way
// would load happily and play wrong. The engine publishes this number as
// AlgoDrum.protocolVersion (internal/drum/protocol.go, the peer constant) and
// the load refuses to proceed on a mismatch. Bump both together, or neither.
export const PROTOCOL_VERSION = 5;

// The engine as it appears on the worker's global scope: the callable API plus
// the version property. The property is deliberately kept out of AlgoDrumApi —
// that interface is the command surface the bridge types itself against, and a
// non-function member would break `keyof AlgoDrumApi` for REQUIRED_METHODS and
// its exhaustiveness guard. `unknown` because a stale engine has no such
// property at all.
type AlgoDrumGlobal = AlgoDrumApi & { protocolVersion?: unknown };

// Every method this worker calls on the engine. AlgoDrumApi is only a
// compile-time contract, so without a runtime check a stale algo_drum.wasm
// (it is a gitignored, unhashed build artifact, and caches version it
// independently of the hashed JS bundle) loads happily and then throws a
// cryptic "AlgoDrum.<name> is not a function" on some later user action —
// e.g. an engine predating the bulk pattern API still has setCell, so the
// failure only surfaced as the pattern echo after the first cell edit.
const REQUIRED_METHODS = [
  "init",
  "beginStart",
  "setRunning",
  "pause",
  "setTempo",
  "setSwing",
  "setStepCount",
  "setCell",
  "setCellProbability",
  "setCellHumanize",
  "setCellCondition",
  "setCellRepeats",
  "setTrackLength",
  "setFillMode",
  "setPattern",
  "setPatternBank",
  "requestBank",
  "setChain",
  "setChainEnabled",
  "setState",
  "getState",
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
  "render",
  "currentStep",
  "transportState",
  "transportRevision",
  "activeBank",
  "queuedBank",
  "chainPosition",
  "isIdle",
] as const satisfies readonly (keyof AlgoDrumApi)[];

// Compile error if a method is added to AlgoDrumApi but not to
// REQUIRED_METHODS, so the runtime check can never silently fall behind.
type AssertNever<T extends never> = T;
export type _AllMethodsListed = AssertNever<
  Exclude<keyof AlgoDrumApi, (typeof REQUIRED_METHODS)[number]>
>;

export const OPERATIONAL_METHODS = [
  "init",
  "getState",
  "beginStart",
  "setRunning",
  "pause",
  "triggerVoice",
  "render",
  "currentStep",
  "transportState",
  "transportRevision",
  "activeBank",
  "queuedBank",
  "chainPosition",
  "isIdle",
] as const satisfies readonly (keyof AlgoDrumApi)[];

type ClassifiedMethod =
  (typeof CONFIGURATION_METHODS)[number] | (typeof OPERATIONAL_METHODS)[number];
export type _ConfigurationMethodsExist = AssertNever<
  Exclude<(typeof CONFIGURATION_METHODS)[number], keyof AlgoDrumApi>
>;
export type _AllMethodsClassified = AssertNever<
  Exclude<keyof AlgoDrumApi, ClassifiedMethod>
>;

// assertEngineApi fails the load when the instantiated WASM does not expose
// the API this bundle was built against, turning a silent version skew into
// an actionable error surfaced by the app's load-fault UI.
function assertEngineApi(api: AlgoDrumGlobal | undefined): void {
  if (!api) {
    throw new Error(
      "WASM engine loaded but did not register the AlgoDrum API — " +
        "algo_drum.wasm is not an algo-drum build.",
    );
  }

  // Before the method list: if the engine speaks a different protocol, which
  // names it happens to expose says nothing useful. A pre-versioning engine
  // has no property at all, hence the <none>.
  if (api.protocolVersion !== PROTOCOL_VERSION) {
    const reported =
      typeof api.protocolVersion === "number"
        ? String(api.protocolVersion)
        : "<none>";

    throw new Error(
      `WASM engine is out of date: algo_drum.wasm speaks protocol version ${reported}, ` +
        `this build requires ${PROTOCOL_VERSION}. ` +
        "Rebuild it with `bash scripts/build-wasm.sh` (or hard-reload to bypass a cached engine).",
    );
  }

  const missing = REQUIRED_METHODS.filter(
    (name) => typeof api[name] !== "function",
  );

  if (missing.length > 0) {
    throw new Error(
      `WASM engine is out of date: algo_drum.wasm is missing ${missing.join(", ")}. ` +
        "Rebuild it with `bash scripts/build-wasm.sh` (or hard-reload to bypass a cached engine).",
    );
  }
}

interface GoRuntime {
  importObject: WebAssembly.Imports;
  run(instance: WebAssembly.Instance): Promise<void>;
}

export type WorkerCommand =
  | { type: "load"; wasmExecUrl: string; wasmUrl: string; sampleRate: number }
  | { type: "connect" }
  | { type: "cmd"; name: keyof AlgoDrumApi; args: unknown[] };

export type WorkerResponse =
  | { type: "ready"; protocolVersion: number }
  | { type: "error"; error: string; fatal?: boolean }
  | { type: "stateSync"; state: unknown }
  | { type: "transportSync"; transport: TransportSnapshot };

const workerScope = globalThis as unknown as {
  Go: new () => GoRuntime;
  AlgoDrum: AlgoDrumGlobal;
  onmessage: ((event: MessageEvent) => void) | null;
  postMessage: (message: unknown) => void;
};

let engineReady = false;

function respond(message: WorkerResponse): void {
  workerScope.postMessage(message);
}

async function instantiate(
  wasmUrl: string,
  importObject: WebAssembly.Imports,
): Promise<WebAssembly.WebAssemblyInstantiatedSource> {
  const request = fetch(wasmUrl);
  try {
    return await WebAssembly.instantiateStreaming(request, importObject);
  } catch {
    // Fallback for servers that don't serve application/wasm.
    const response = await fetch(wasmUrl);
    const bytes = await response.arrayBuffer();
    return WebAssembly.instantiate(bytes, importObject);
  }
}

async function load(
  wasmExecUrl: string,
  wasmUrl: string,
  sampleRate: number,
): Promise<void> {
  // Side-effect import of the Go runtime (defines `Go` on globalThis).
  // wasm_exec.js is served from public/; @vite-ignore keeps the bundler
  // from trying to resolve the runtime-computed URL.
  await import(/* @vite-ignore */ wasmExecUrl);

  const go = new workerScope.Go();
  const result = await instantiate(wasmUrl, go.importObject);
  void go.run(result.instance); // runs forever via select{}

  // go.run() executes Go's main synchronously up to its select{}, so the
  // API registration has already happened by the time it returns.
  assertEngineApi(workerScope.AlgoDrum);

  workerScope.AlgoDrum.init(sampleRate);
  engineReady = true;
  respond({ type: "ready", protocolVersion: PROTOCOL_VERSION });
  respond({ type: "transportSync", transport: readTransport() });
}

// Messages the worklet sends over the direct audio port: a pull request, and
// the storage of a chunk it has finished playing.
type WorkletRequest =
  { type: "need"; samples: number } | { type: "recycle"; buffer: ArrayBuffer };

// Free pool of spent chunk buffers returned by the worklet. Without it the
// worker allocates a fresh ~2 KB Float32Array per chunk (~94/s at 48 kHz) and
// transfers its storage away, so every one of them is garbage. The bound is
// small on purpose: the worklet only ever holds TARGET_QUEUE_SAMPLES /
// CHUNK_SAMPLES chunks plus the ones in flight, so a deeper pool would only
// retain memory nothing is going to ask for.
//
// The zero-message alternative — a SharedArrayBuffer ring — is deliberately
// rejected: SAB requires cross-origin isolation (COOP/COEP headers), which
// this repository removed on purpose and GitHub Pages cannot set. Do not
// relitigate it without bringing those headers back first.
const MAX_POOLED_BUFFERS = 8;
const bufferPool: ArrayBuffer[] = [];

function recycleBuffer(buffer: ArrayBuffer): void {
  if (bufferPool.length >= MAX_POOLED_BUFFERS) return;

  bufferPool.push(buffer);
}

// takeChunk returns a Float32Array of the requested length, reusing a pooled
// buffer when one of exactly the right size is available. Pooled storage holds
// the previous chunk's samples, so every caller must fill it completely.
function takeChunk(length: number): Float32Array {
  const bytes = length * Float32Array.BYTES_PER_ELEMENT;

  for (let i = bufferPool.length - 1; i >= 0; i--) {
    if (bufferPool[i].byteLength !== bytes) continue;

    return new Float32Array(bufferPool.splice(i, 1)[0]);
  }

  return new Float32Array(length);
}

// True while render is failing. The worklet keeps pulling at ~94 chunks/s, so
// reporting per chunk would flood the main thread with the same message; only
// the transition into a failing state is worth an error response.
let renderFaulted = false;

// fillChunk renders one chunk's worth of audio into buf and returns the step
// it starts on plus the engine's idle state. It never throws: the worklet
// spends a request credit per pull and only gets it back when a chunk
// arrives, so a pull that returns nothing leaks a credit — four of those and
// the request condition is false forever and audio stops permanently, with no
// error anywhere. Silence is the failure mode, not starvation.
function fillChunk(buf: Float32Array): {
  transport: TransportSnapshot;
  idle: boolean;
} {
  if (!engineReady) {
    buf.fill(0);
    // Not idle: the main thread suspends the AudioContext on idle, and a
    // context that never runs cannot recover on its own.
    return {
      transport: {
        state: "stopped",
        step: -1,
        revision: 0,
        activeBank: 0,
        queuedBank: -1,
        chainPosition: -1,
      },
      idle: false,
    };
  }

  try {
    // Capture the complete epoch BEFORE rendering: this is the state and step
    // whose samples start the chunk, not the step Render advances to.
    const transport = readTransport();

    // Copy out of the engine's reused buffer into the transferable chunk.
    const rendered = workerScope.AlgoDrum.render(buf.length);
    buf.set(rendered.subarray(0, Math.min(rendered.length, buf.length)));
    if (rendered.length < buf.length) buf.fill(0, rendered.length);

    const idle = workerScope.AlgoDrum.isIdle();
    renderFaulted = false;
    return { transport, idle };
  } catch (error) {
    buf.fill(0);

    if (!renderFaulted) {
      renderFaulted = true;

      // Rendering has failed, so keeping the sequencer running would only
      // advance an inaudible transport. Best-effort Stop; the fatal response
      // tears the bridge down even if the engine can no longer report state.
      try {
        workerScope.AlgoDrum.setRunning(false);
        respond({ type: "transportSync", transport: readTransport() });
      } catch {
        // The original render failure is the useful diagnostic.
      }
      respond({
        type: "error",
        error: `Audio rendering failed: ${String(error)}`,
        fatal: true,
      });
    }

    return {
      transport: {
        state: "stopped",
        step: -1,
        revision: 0,
        activeBank: 0,
        queuedBank: -1,
        chainPosition: -1,
      },
      idle: false,
    };
  }
}

function handleWorkletPort(port: MessagePort): void {
  port.onmessage = (event: MessageEvent<WorkletRequest>) => {
    const request = event.data;

    if (request.type === "recycle") {
      recycleBuffer(request.buffer);
      return;
    }

    // Exactly one reply per request, always — see fillChunk.
    const chunk = takeChunk(request.samples);
    const { transport, idle } = fillChunk(chunk);
    port.postMessage({ buffer: chunk.buffer, transport, idle }, [chunk.buffer]);
  };
}

// invokeEngine calls one engine method. AlgoDrum is a foreign object built by
// Go, so neither the method's existence nor its success is guaranteed at
// runtime: an uncaught throw here would escape the message handler as an
// unhandled worker error that the main thread cannot attribute to anything.
// Reporting it as an error response keeps the failure diagnosable.
function invokeEngine(name: keyof AlgoDrumApi, args: unknown[]): boolean {
  const method: unknown = workerScope.AlgoDrum[name];

  if (typeof method !== "function") {
    respond({ type: "error", error: `AlgoDrum.${name} is not callable` });
    return false;
  }

  try {
    (method as (...callArgs: unknown[]) => unknown)(...args);
    return true;
  } catch (error) {
    respond({
      type: "error",
      error: `AlgoDrum.${name} failed: ${String(error)}`,
    });
    return false;
  }
}

function isTransportCommand(name: keyof AlgoDrumApi): boolean {
  return (
    name === "beginStart" ||
    name === "setRunning" ||
    name === "pause" ||
    name === "requestBank" ||
    name === "setChain" ||
    name === "setChainEnabled"
  );
}

function readTransport(): TransportSnapshot {
  const state = workerScope.AlgoDrum.transportState();
  const step = workerScope.AlgoDrum.currentStep();
  const revision = workerScope.AlgoDrum.transportRevision();
  const activeBank = workerScope.AlgoDrum.activeBank();
  const queuedBank = workerScope.AlgoDrum.queuedBank();
  const chainPosition = workerScope.AlgoDrum.chainPosition();

  if (
    state !== "stopped" &&
    state !== "starting" &&
    state !== "playing" &&
    state !== "paused"
  ) {
    throw new TypeError(`invalid transport state ${String(state)}`);
  }

  if (!Number.isSafeInteger(revision) || revision < 0) {
    throw new TypeError(`invalid transport revision ${String(revision)}`);
  }

  if (!Number.isInteger(step) || step < -1) {
    throw new TypeError(`invalid transport step ${String(step)}`);
  }

  if (!Number.isInteger(activeBank) || activeBank < 0 || activeBank > 3) {
    throw new TypeError(`invalid active bank ${String(activeBank)}`);
  }

  if (!Number.isInteger(queuedBank) || queuedBank < -1 || queuedBank > 3) {
    throw new TypeError(`invalid queued bank ${String(queuedBank)}`);
  }

  if (
    !Number.isInteger(chainPosition) ||
    chainPosition < -1 ||
    chainPosition > 15
  ) {
    throw new TypeError(`invalid chain position ${String(chainPosition)}`);
  }

  const inactive = state === "stopped" || state === "starting";
  if ((inactive && step !== -1) || (!inactive && step < 0)) {
    throw new TypeError(`transport state ${state} cannot report step ${step}`);
  }

  return { state, step, revision, activeBank, queuedBank, chainPosition };
}

// A failed read still returns an invalid sentinel: the mirror spends one
// in-flight mutation per echo, so swallowing it would gate every later state.
function readState(): unknown {
  try {
    return validateEngineState(workerScope.AlgoDrum.getState());
  } catch (error) {
    respond({
      type: "error",
      error: `AlgoDrum.getState failed: ${String(error)}`,
    });
    return null;
  }
}

workerScope.onmessage = (event: MessageEvent<WorkerCommand>) => {
  const message = event.data;

  switch (message.type) {
    case "load":
      void load(message.wasmExecUrl, message.wasmUrl, message.sampleRate).catch(
        (error: unknown) => {
          respond({ type: "error", error: String(error) });
        },
      );
      break;
    case "connect":
      if (event.ports[0]) handleWorkletPort(event.ports[0]);
      break;
    case "cmd":
      if (engineReady) {
        invokeEngine(message.name, message.args);
        if (isTransportCommand(message.name)) {
          // Read back Go's state on success and failure alike. Setters may be
          // no-ops, and inferring their result here would create a second
          // transport owner in the worker.
          try {
            respond({ type: "transportSync", transport: readTransport() });
          } catch (error) {
            respond({
              type: "error",
              error: `AlgoDrum transport read failed: ${String(error)}`,
              fatal: true,
            });
          }
        }
      }

      // One full authoritative echo follows every configuration mutation.
      // Transport has its own small acknowledgement above; audition and audio
      // rendering deliberately do not echo persistent configuration.
      if (isConfigurationMethod(message.name)) {
        respond({ type: "stateSync", state: readState() });
      }
      break;
  }
};
