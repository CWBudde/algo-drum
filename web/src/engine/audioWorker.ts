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
  setTempo: (bpm: number) => void;
  setSwing: (swing: number) => void;
  setStepCount: (steps: number) => void;
  setCell: (track: number, step: number, velocity: number) => void;
  setPattern: (pattern: Float32Array) => void;
  getPattern: () => Float32Array;
  setVolume: (track: number, vol: number) => void;
  setDecay: (track: number, amount: number) => void;
  setVoiceParam: (track: number, index: number, value: number) => void;
  setPhysicalTomParam: (index: number, value: number) => void;
  setTomModel: (model: number) => void;
  triggerVoice: (track: number, velocity: number) => void;
  setReverb: (amount: number) => void;
  setProbability: (p: number) => void;
  setHumanize: (h: number) => void;
  render: (n: number) => Float32Array;
  currentStep: () => number;
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

function handleWorkletPort(port: MessagePort): void {
  port.onmessage = (event: MessageEvent<{ type: string; samples: number }>) => {
    if (event.data.type !== "need" || !engineReady) return;

    // Capture the step BEFORE rendering: it's the step this chunk starts on.
    const step = workerScope.AlgoDrum.currentStep();
    const rendered = workerScope.AlgoDrum.render(event.data.samples);

    // Copy out of the engine's reused buffer into a transferable chunk.
    const chunk = new Float32Array(rendered);
    port.postMessage({ buffer: chunk.buffer, step }, [chunk.buffer]);
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
