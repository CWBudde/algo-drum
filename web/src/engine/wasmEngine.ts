// Main-thread bridge to the drum engine.
//
// The Go WASM engine runs inside a dedicated Worker (audioWorker.ts); an
// AudioWorklet (public/worklet.js) pulls rendered chunks from it over a
// direct MessageChannel. This module only sends control commands and mirrors
// the audible sequencer step reported back by the worklet.

import type { AlgoDrumApi, WorkerCommand, WorkerResponse } from "./audioWorker";
import { PatternMirror } from "./patternMirror";

const SAMPLE_RATE = 48000;

// Upper bound on how long the worker may take to report the engine ready.
// A worker that neither answers nor errors (a stalled fetch, a runtime that
// never reaches its entry point) would otherwise leave the app on "Loading
// engine…" forever, with no path to the Retry button.
const LOAD_TIMEOUT_MS = 30000;

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

// The engine owns the pattern: the worker echoes the authoritative copy back
// after every edit, and the mirror reconciles those echoes with edits still
// in flight before notifying the UI.
const patternMirror = new PatternMirror();

// onPattern subscribes to authoritative pattern snapshots (flat track-major
// Float32Array, see setPattern) and returns an unsubscribe function.
export function onPattern(
  listener: (pattern: Float32Array) => void,
): () => void {
  return patternMirror.subscribe(listener);
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

// command sends one engine call. Typing the name against AlgoDrumApi (and the
// arguments against that method's signature) makes a typo or a wrong argument
// a compile error here rather than a silent no-op in the worker.
function command<K extends keyof AlgoDrumApi>(
  name: K,
  ...args: Parameters<AlgoDrumApi[K]>
): void {
  post({ type: "cmd", name, args });
}

// isPatternEdit reports whether a queued command mutates the engine's pattern
// and will therefore be echoed back once it is finally sent.
function isPatternEdit(cmd: WorkerCommand): boolean {
  return (
    cmd.type === "cmd" && (cmd.name === "setCell" || cmd.name === "setPattern")
  );
}

// teardownWorker drops a worker from a failed attempt so a retry starts clean
// and we don't leak workers.
function teardownWorker(): void {
  if (!worker) return;

  worker.terminate();
  worker = null;

  // Edits already sent to the dead worker will never be echoed, but the ones
  // still queued here will be replayed to the replacement worker. Re-base the
  // mirror on that count: any mismatch makes it publish a stale echo as
  // authoritative and revert newer edits.
  patternMirror.reset(pendingCommands.filter(isPatternEdit).length);
}

// One shared load attempt. React StrictMode invokes the mount effect twice and
// the Retry button can be double-clicked; without this, the second call would
// terminate the first call's worker and leave its promise pending forever.
let loadAttempt: Promise<void> | null = null;

export function loadWasm(): Promise<void> {
  if (wasmReady) return Promise.resolve();

  if (!loadAttempt) {
    const attempt = attemptLoad();
    loadAttempt = attempt;

    // Forget a settled attempt so a failure can be retried; a success is
    // short-circuited by wasmReady above. The extra catch only keeps this
    // bookkeeping chain from surfacing as a second, unhandled rejection —
    // callers still see the original one.
    void attempt
      .catch(() => undefined)
      .finally(() => {
        if (loadAttempt === attempt) loadAttempt = null;
      });
  }

  return loadAttempt;
}

async function attemptLoad(): Promise<void> {
  // A previous attempt may have left a worker running (e.g. the WASM fetch
  // failed after the worker spawned).
  teardownWorker();

  const active = new Worker(new URL("./audioWorker.ts", import.meta.url), {
    type: "module",
  });
  worker = active;

  let timeout: ReturnType<typeof setTimeout> | undefined;

  const ready = new Promise<void>((resolve, reject) => {
    active.onmessage = (event: MessageEvent<WorkerResponse>) => {
      const data = event.data;
      switch (data.type) {
        case "ready":
          resolve();
          break;
        case "error":
          // Before readiness this settles the load and the app renders the
          // message. Afterwards it is a per-command failure and this reject
          // is a no-op, so log it or it would vanish silently.
          if (wasmReady) console.error("Audio engine error:", data.error);
          reject(new Error(data.error));
          break;
        case "patternSync":
          patternMirror.receiveSync(data.pattern);
          break;
      }
    };

    // A worker can also fail without ever sending a message: a module that
    // 404s, or a throw at its top level. Without these handlers the load
    // promise would never settle and the UI would hang on "Loading engine…".
    active.onerror = (event) => {
      // message is empty for cross-origin failures.
      reject(new Error(event.message || "Audio engine worker failed to start"));
    };
    active.onmessageerror = () => {
      reject(new Error("Audio engine worker sent an undeliverable message"));
    };

    timeout = setTimeout(() => {
      reject(
        new Error(
          `Audio engine did not start within ${LOAD_TIMEOUT_MS / 1000}s`,
        ),
      );
    }, LOAD_TIMEOUT_MS);
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

  try {
    await ready;
  } finally {
    clearTimeout(timeout);
  }

  wasmReady = true;

  for (const cmd of pendingCommands.splice(0)) {
    send(cmd);
  }
}

function describeError(error: unknown): string {
  return error instanceof Error && error.message
    ? error.message
    : String(error);
}

// teardownAudio tears the audio graph down so the next play() can rebuild it.
function teardownAudio(): void {
  workletNode?.disconnect();
  workletNode = null;

  // close() is fire-and-forget here: we are already on an error path and a
  // context that refuses to close is being dropped anyway.
  void audioCtx?.close().catch(() => undefined);
  audioCtx = null;
}

async function startAudio(): Promise<void> {
  if (audioCtx || !worker) return;

  try {
    const ctx = new AudioContext({ sampleRate: SAMPLE_RATE });
    audioCtx = ctx;

    await ctx.audioWorklet.addModule(import.meta.env.BASE_URL + "worklet.js");

    const node = new AudioWorkletNode(ctx, "algo-drum", {
      numberOfInputs: 0,
      outputChannelCount: [1],
    });
    workletNode = node;

    // Direct worker <-> worklet channel for audio chunks.
    const channel = new MessageChannel();
    send({ type: "connect" }, [channel.port1]);
    node.port.postMessage({ type: "workerPort" }, [channel.port2]);

    // The worklet reports the step each playing chunk starts on, so the UI
    // playhead follows what is audible rather than what was rendered ahead.
    node.port.onmessage = (
      event: MessageEvent<{ type: string; step: number }>,
    ) => {
      if (event.data.type === "step") notifyStep(event.data.step);
    };

    node.connect(ctx.destination);
  } catch (error) {
    // Constructing the context or loading the worklet module can fail (an
    // exhausted/blocked AudioContext, a worklet.js that 404s or throws). Drop
    // the half-built graph so a later play() retries from scratch instead of
    // returning early on a dead context, and reject with something the caller
    // can show the user.
    teardownAudio();

    // tsconfig targets the ES2020 lib, which predates the Error options bag,
    // so the cause is attached after construction.
    const failure: Error & { cause?: unknown } = new Error(
      `Audio output could not be started: ${describeError(error)}`,
    );
    failure.cause = error;
    throw failure;
  }
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

// setCell sets one cell's velocity in [0, 1]; 0 turns the cell off. The
// engine echoes its authoritative pattern back to onPattern subscribers.
export function setCell(track: number, step: number, velocity: number): void {
  patternMirror.beginMutation();
  command("setCell", track, step, velocity);
}

// setPattern replaces the whole pattern: a flat track-major Float32Array of
// TrackCount×MaxSteps (5×16) velocities in [0, 1], index = track*16 + step.
// The engine echoes its authoritative pattern back to onPattern subscribers.
export function setPattern(pattern: Float32Array): void {
  patternMirror.beginMutation();
  command("setPattern", pattern);
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
