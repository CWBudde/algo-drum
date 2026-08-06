// Dedicated worker that hosts the Go WASM drum engine.
//
// Running the engine here keeps audio rendering off the main thread: the
// AudioWorklet requests chunks over a direct MessagePort, and UI parameter
// changes arrive as command messages from the main thread.

// The engine's JS surface. Exported so the main-thread bridge can type its
// command sender against it (method name and argument tuple) instead of
// casting into the wire format.
export interface AlgoDrumApi {
  init: (sampleRate: number) => void;
  setRunning: (playing: boolean) => void;
  pause: () => void;
  setTempo: (bpm: number) => void;
  setSwing: (swing: number) => void;
  setStepCount: (steps: number) => void;
  setCell: (track: number, step: number, velocity: number) => void;
  setPattern: (pattern: Float32Array) => void;
  getPattern: () => Float32Array;
  setVolume: (track: number, vol: number) => void;
  setDecay: (track: number, amount: number) => void;
  setVoiceParam: (track: number, index: number, value: number) => void;
  setPhysicalTomParam: (track: number, index: number, value: number) => void;
  setTomModel: (track: number, model: number) => void;
  triggerVoice: (track: number, velocity: number) => void;
  setReverb: (amount: number) => void;
  setProbability: (p: number) => void;
  setHumanize: (h: number) => void;
  render: (n: number) => Float32Array;
  currentStep: () => number;
  isIdle: () => boolean;
}

// Every method this worker calls on the engine. AlgoDrumApi is only a
// compile-time contract, so without a runtime check a stale algo_drum.wasm
// (it is a gitignored, unhashed build artifact, and caches version it
// independently of the hashed JS bundle) loads happily and then throws a
// cryptic "AlgoDrum.<name> is not a function" on some later user action —
// e.g. an engine predating the bulk pattern API still has setCell, so the
// failure only surfaced as the pattern echo after the first cell edit.
const REQUIRED_METHODS = [
  "init",
  "setRunning",
  "pause",
  "setTempo",
  "setSwing",
  "setStepCount",
  "setCell",
  "setPattern",
  "getPattern",
  "setVolume",
  "setDecay",
  "setVoiceParam",
  "setPhysicalTomParam",
  "setTomModel",
  "triggerVoice",
  "setReverb",
  "setProbability",
  "setHumanize",
  "render",
  "currentStep",
  "isIdle",
] as const satisfies readonly (keyof AlgoDrumApi)[];

// Compile error if a method is added to AlgoDrumApi but not to
// REQUIRED_METHODS, so the runtime check can never silently fall behind.
type AssertNever<T extends never> = T;
export type _AllMethodsListed = AssertNever<
  Exclude<keyof AlgoDrumApi, (typeof REQUIRED_METHODS)[number]>
>;

// assertEngineApi fails the load when the instantiated WASM does not expose
// the API this bundle was built against, turning a silent version skew into
// an actionable error surfaced by the app's load-fault UI.
function assertEngineApi(api: AlgoDrumApi | undefined): void {
  if (!api) {
    throw new Error(
      "WASM engine loaded but did not register the AlgoDrum API — " +
        "algo_drum.wasm is not an algo-drum build.",
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
  | { type: "ready" }
  | { type: "error"; error: string }
  | { type: "patternSync"; pattern: Float32Array };

const workerScope = globalThis as unknown as {
  Go: new () => GoRuntime;
  AlgoDrum: AlgoDrumApi;
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
  respond({ type: "ready" });
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
function fillChunk(buf: Float32Array): { step: number; idle: boolean } {
  if (!engineReady) {
    buf.fill(0);
    // Not idle: the main thread suspends the AudioContext on idle, and a
    // context that never runs cannot recover on its own.
    return { step: -1, idle: false };
  }

  try {
    // Capture the step BEFORE rendering: it's the step this chunk starts on.
    const step = workerScope.AlgoDrum.currentStep();

    // Copy out of the engine's reused buffer into the transferable chunk.
    const rendered = workerScope.AlgoDrum.render(buf.length);
    buf.set(rendered.subarray(0, Math.min(rendered.length, buf.length)));
    if (rendered.length < buf.length) buf.fill(0, rendered.length);

    const idle = workerScope.AlgoDrum.isIdle();
    renderFaulted = false;
    return { step, idle };
  } catch (error) {
    buf.fill(0);

    if (!renderFaulted) {
      renderFaulted = true;
      respond({
        type: "error",
        error: `Audio rendering failed: ${String(error)}`,
      });
    }

    return { step: -1, idle: false };
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
    const { step, idle } = fillChunk(chunk);
    port.postMessage({ buffer: chunk.buffer, step, idle }, [chunk.buffer]);
  };
}

// invokeEngine calls one engine method. AlgoDrum is a foreign object built by
// Go, so neither the method's existence nor its success is guaranteed at
// runtime: an uncaught throw here would escape the message handler as an
// unhandled worker error that the main thread cannot attribute to anything.
// Reporting it as an error response keeps the failure diagnosable.
function invokeEngine(name: keyof AlgoDrumApi, args: unknown[]): void {
  const method: unknown = workerScope.AlgoDrum[name];

  if (typeof method !== "function") {
    respond({ type: "error", error: `AlgoDrum.${name} is not callable` });
    return;
  }

  try {
    (method as (...callArgs: unknown[]) => unknown)(...args);
  } catch (error) {
    respond({
      type: "error",
      error: `AlgoDrum.${name} failed: ${String(error)}`,
    });
  }
}

// readPattern reads the engine's authoritative pattern for an echo, falling
// back to an empty array if it cannot. The mirror counts exactly one echo per
// edit, so a swallowed echo would stall it permanently; an empty one is
// ignored by the mirror but still balances the books. The catch also covers
// the engine not being ready (AlgoDrum undefined), which the main thread's
// command queue already prevents.
function readPattern(): Float32Array {
  try {
    return workerScope.AlgoDrum.getPattern();
  } catch (error) {
    respond({
      type: "error",
      error: `AlgoDrum.getPattern failed: ${String(error)}`,
    });
    return new Float32Array(0);
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
      if (engineReady) invokeEngine(message.name, message.args);

      // The engine owns the pattern: after every pattern edit, echo its
      // authoritative copy so the main-thread mirror can reconcile. Exactly
      // one echo per edit, so the mirror's in-flight accounting stays
      // balanced even when the read fails.
      if (message.name === "setCell" || message.name === "setPattern") {
        respond({ type: "patternSync", pattern: readPattern() });
      }
      break;
  }
};
