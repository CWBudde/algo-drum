// Main-thread bridge to the drum engine.
//
// The Go WASM engine runs inside a dedicated Worker (audioWorker.ts); an
// AudioWorklet (public/worklet.js) pulls rendered chunks from it over a
// direct MessageChannel. This module only sends control commands and mirrors
// the audible sequencer step reported back by the worklet.

import type { WorkerCommand, WorkerResponse } from "./audioWorker";

const SAMPLE_RATE = 48000;

let worker: Worker | null = null;
let audioCtx: AudioContext | null = null;
let workletNode: AudioWorkletNode | null = null;
let wasmReady = false;
let audibleStep = -1;

type StepListener = (step: number) => void;

const stepListeners = new Set<StepListener>();

// onStep subscribes to audible-step changes (-1 while stopped) and returns
// an unsubscribe function.
export function onStep(listener: StepListener): () => void {
  stepListeners.add(listener);
  listener(audibleStep);

  return () => stepListeners.delete(listener);
}

function notifyStep(step: number): void {
  audibleStep = step;
  stepListeners.forEach((listener) => listener(step));
}

function send(command: WorkerCommand, transfer: Transferable[] = []): void {
  worker?.postMessage(command, transfer);
}

// Commands issued before the worker reports the engine ready are queued and
// flushed on readiness, so nothing a fast user does during load is dropped.
const pendingCommands: WorkerCommand[] = [];

function post(command: WorkerCommand): void {
  if (!wasmReady) {
    pendingCommands.push(command);
    return;
  }

  send(command);
}

function command(name: string, ...args: unknown[]): void {
  post({ type: "cmd", name, args } as WorkerCommand);
}

// getPattern is fire-and-forget on the command channel, so pattern reads go
// through a small request/reply keyed by id.
let nextRequestId = 1;
const patternResolvers = new Map<number, (pattern: Float32Array) => void>();

// settlePendingPatternRequests resolves every in-flight getPattern() with an
// empty pattern so callers never hang when the worker dies or fails to load.
function settlePendingPatternRequests(): void {
  for (const resolve of patternResolvers.values()) {
    resolve(new Float32Array(0));
  }
  patternResolvers.clear();
}

export async function loadWasm(): Promise<void> {
  if (wasmReady) return;

  // A previous attempt may have left a worker running (e.g. the WASM fetch
  // failed after the worker spawned). Tear it down so Retry starts clean and
  // we don't leak workers.
  if (worker) {
    worker.terminate();
    worker = null;
    settlePendingPatternRequests();
  }

  worker = new Worker(new URL("./audioWorker.ts", import.meta.url), {
    type: "module",
  });

  const ready = new Promise<void>((resolve, reject) => {
    if (!worker) return;
    worker.onmessage = (event: MessageEvent<WorkerResponse>) => {
      const data = event.data;
      switch (data.type) {
        case "ready":
          resolve();
          break;
        case "error":
          settlePendingPatternRequests();
          reject(new Error(data.error));
          break;
        case "pattern": {
          const resolvePattern = patternResolvers.get(data.id);
          patternResolvers.delete(data.id);
          resolvePattern?.(data.pattern);
          break;
        }
      }
    };
  });

  // Create the engine immediately so pattern and parameter edits made before
  // the first Play are not lost. The AudioContext is created later, on the
  // first user gesture, at the same fixed sample rate.
  const base = new URL(import.meta.env.BASE_URL, self.location.href);
  send({
    type: "load",
    wasmExecUrl: new URL("wasm_exec.js", base).toString(),
    wasmUrl: new URL("algo_drum.wasm", base).toString(),
    sampleRate: SAMPLE_RATE,
  });

  await ready;
  wasmReady = true;

  for (const cmd of pendingCommands.splice(0)) {
    send(cmd);
  }
}

async function startAudio(): Promise<void> {
  if (audioCtx || !worker) return;

  audioCtx = new AudioContext({ sampleRate: SAMPLE_RATE });
  await audioCtx.audioWorklet.addModule(
    import.meta.env.BASE_URL + "worklet.js",
  );

  workletNode = new AudioWorkletNode(audioCtx, "algo-drum", {
    numberOfInputs: 0,
    outputChannelCount: [1],
  });

  // Direct worker <-> worklet channel for audio chunks.
  const channel = new MessageChannel();
  send({ type: "connect" }, [channel.port1]);
  workletNode.port.postMessage({ type: "workerPort" }, [channel.port2]);

  // The worklet reports the step each playing chunk starts on, so the UI
  // playhead follows what is audible rather than what was rendered ahead.
  workletNode.port.onmessage = (
    event: MessageEvent<{ type: string; step: number }>,
  ) => {
    if (event.data.type === "step") notifyStep(event.data.step);
  };

  workletNode.connect(audioCtx.destination);
}

export async function play(): Promise<void> {
  await startAudio();

  // Autoplay policies (notably iOS Safari) can leave a freshly created
  // context suspended; without an explicit resume there is no sound.
  if (audioCtx && audioCtx.state === "suspended") {
    await audioCtx.resume();
  }

  command("setRunning", true);
}

export function stop(): void {
  command("setRunning", false);
  notifyStep(-1);
}

export function setTempo(bpm: number): void {
  command("setTempo", bpm);
}

export function setSwing(swing: number): void {
  command("setSwing", swing);
}

// setStepCount sets the active pattern length (clamped to 1–16 in the
// engine); cells beyond it are kept, just not played.
export function setStepCount(steps: number): void {
  command("setStepCount", steps);
}

// setCell sets one cell's velocity in [0, 1]; 0 turns the cell off.
export function setCell(track: number, step: number, velocity: number): void {
  command("setCell", track, step, velocity);
}

// setPattern replaces the whole pattern: a flat track-major Float32Array of
// TrackCount×MaxSteps (5×16) velocities in [0, 1], index = track*16 + step.
export function setPattern(pattern: Float32Array): void {
  command("setPattern", pattern);
}

// getPattern resolves with the engine's pattern in the same flat layout
// setPattern accepts (empty before the engine has loaded).
export function getPattern(): Promise<Float32Array> {
  const id = nextRequestId++;
  const result = new Promise<Float32Array>((resolve) => {
    patternResolvers.set(id, resolve);
  });

  post({ type: "getPattern", id });

  return result;
}

export function setVolume(track: number, vol: number): void {
  command("setVolume", track, vol);
}

export function setDecay(track: number, amount: number): void {
  command("setDecay", track, amount);
}

export function setReverb(amount: number): void {
  command("setReverb", amount);
}

// setProbability sets the per-hit trigger chance in [0, 1] (1 = every hit).
export function setProbability(p: number): void {
  command("setProbability", p);
}

// setHumanize sets the timing/velocity randomization amount in [0, 1].
export function setHumanize(h: number): void {
  command("setHumanize", h);
}

export function currentStep(): number {
  return audibleStep;
}
