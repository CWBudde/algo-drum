// Dedicated worker that hosts the Go WASM drum engine.
//
// Running the engine here keeps audio rendering off the main thread: the
// AudioWorklet requests chunks over a direct MessagePort, and UI parameter
// changes arrive as command messages from the main thread.

interface AlgoDrumApi {
  init: (sampleRate: number) => void;
  setRunning: (playing: boolean) => void;
  setTempo: (bpm: number) => void;
  setSwing: (swing: number) => void;
  setCell: (track: number, step: number, active: boolean) => void;
  setVolume: (track: number, vol: number) => void;
  setDecay: (track: number, amount: number) => void;
  setReverb: (amount: number) => void;
  render: (n: number) => Float32Array;
  currentStep: () => number;
}

interface GoRuntime {
  importObject: WebAssembly.Imports;
  run(instance: WebAssembly.Instance): Promise<void>;
}

export type WorkerCommand =
  | { type: "load"; wasmExecUrl: string; wasmUrl: string; sampleRate: number }
  | { type: "connect" }
  | { type: "cmd"; name: keyof AlgoDrumApi; args: unknown[] };

const workerScope = globalThis as unknown as {
  Go: new () => GoRuntime;
  AlgoDrum: AlgoDrumApi;
  onmessage: ((event: MessageEvent) => void) | null;
  postMessage: (message: unknown) => void;
};

let engineReady = false;

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

  workerScope.AlgoDrum.init(sampleRate);
  engineReady = true;
  workerScope.postMessage({ type: "ready" });
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

workerScope.onmessage = (event: MessageEvent<WorkerCommand>) => {
  const message = event.data;

  switch (message.type) {
    case "load":
      void load(message.wasmExecUrl, message.wasmUrl, message.sampleRate).catch(
        (error: unknown) => {
          workerScope.postMessage({ type: "error", error: String(error) });
        },
      );
      break;
    case "connect":
      if (event.ports[0]) handleWorkletPort(event.ports[0]);
      break;
    case "cmd":
      if (engineReady) {
        const method = workerScope.AlgoDrum[message.name] as (
          ...args: unknown[]
        ) => unknown;
        method(...message.args);
      }
      break;
  }
};
