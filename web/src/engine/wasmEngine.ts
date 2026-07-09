// Main-thread bridge to the drum engine.
//
// The Go WASM engine runs inside a dedicated Worker (audioWorker.ts); an
// AudioWorklet (public/worklet.js) pulls rendered chunks from it over a
// direct MessageChannel. This module only sends control commands and mirrors
// the audible sequencer step reported back by the worklet.

import type { WorkerCommand } from "./audioWorker";

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

function command(name: string, ...args: unknown[]): void {
  const cmd = { type: "cmd", name, args } as WorkerCommand;

  if (!wasmReady) {
    pendingCommands.push(cmd);
    return;
  }

  send(cmd);
}

export async function loadWasm(): Promise<void> {
  if (wasmReady) return;

  worker = new Worker(new URL("./audioWorker.ts", import.meta.url), {
    type: "module",
  });

  const ready = new Promise<void>((resolve, reject) => {
    if (!worker) return;
    worker.onmessage = (event: MessageEvent<{ type: string; error?: string }>) => {
      if (event.data.type === "ready") resolve();
      if (event.data.type === "error") reject(new Error(event.data.error));
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

export function setCell(track: number, step: number, active: boolean): void {
  command("setCell", track, step, active);
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

export function currentStep(): number {
  return audibleStep;
}
