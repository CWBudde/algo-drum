// Main-thread bridge to the drum engine.
//
// The Go WASM engine runs inside a dedicated Worker (audioWorker.ts); an
// AudioWorklet (public/worklet.js) pulls rendered chunks from it over a
// direct MessageChannel. This module only sends control commands and mirrors
// the audible sequencer step reported back by the worklet.

import type { AlgoDrumApi, WorkerCommand, WorkerResponse } from "./audioWorker";
import { PatternMirror } from "./patternMirror";
import type { TomModel } from "./tomModel";

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

// Set by stop() and cleared once the worklet has drained the samples it had
// already buffered when the stop was issued. See stop() and reportAudibleStep.
let awaitingStopDrain = false;

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

// reportAudibleStep publishes a step the worklet says has become audible,
// unless it belongs to a chunk rendered before a Stop that is still draining.
// The engine reports -1 for everything it renders while stopped, so the first
// such chunk is exactly the end of the drain.
function reportAudibleStep(step: number): void {
  if (awaitingStopDrain) {
    if (step !== -1) return;

    awaitingStopDrain = false;
  }

  // The -1 that ends a drain repeats the one stop() already published.
  if (step === audibleStep) return;

  notifyStep(step);
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

// Settles a load that is still waiting on the worker. dispose() terminates
// that worker, so without this its promise would never settle and the app
// would sit on "Loading engine…" forever.
let cancelLoad: ((error: Error) => void) | null = null;

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

    cancelLoad = reject;

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
  } catch (error) {
    // Don't let a failed attempt's worker outlive it. Teardown otherwise only
    // runs at the start of the *next* attempt, so a worker that errored or
    // timed out would keep running until the user hits Retry — or forever if
    // they never do. This also clears the module-level reference, so nothing
    // can post to it afterwards.
    //
    // Only our own worker, though: dispose() settles this promise and clears
    // loadAttempt synchronously, so a replacement load can already be under
    // way by the time this rejection is delivered.
    if (worker === active) teardownWorker();

    throw error;
  } finally {
    clearTimeout(timeout);
    cancelLoad = null;
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
  if (workletNode) {
    // Drop the step handler with the node: it closes over this module's
    // reporting state and would otherwise keep answering a graph we no
    // longer own.
    workletNode.port.onmessage = null;
    workletNode.disconnect();
  }

  workletNode = null;

  // close() is fire-and-forget here: we are already on an error path and a
  // context that refuses to close is being dropped anyway.
  void audioCtx?.close().catch(() => undefined);
  audioCtx = null;
}

// dispose tears the whole bridge down — audio graph, worker, and every piece
// of module state derived from them. The engine is a process-wide singleton
// with no owner in the React tree, so nothing else ever stops it: an unmount,
// a UI crash that swaps the machine out for a fault panel, or an HMR update
// would otherwise leave a worker rendering into a live AudioContext with no
// controls attached to it. A later loadWasm() rebuilds from scratch.
export function dispose(): void {
  teardownAudio();

  // Drop queued commands before the worker goes: they were meant for an
  // engine that is going away, and teardownWorker() would otherwise re-base
  // the mirror on edits that will never be replayed.
  pendingCommands.length = 0;
  teardownWorker();
  patternMirror.reset();

  // An in-flight load is waiting on a worker that no longer exists.
  cancelLoad?.(new Error("Audio engine was disposed"));
  cancelLoad = null;
  loadAttempt = null;

  wasmReady = false;
  awaitingStopDrain = false;
  notifyStep(-1);
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
      if (event.data.type === "step") reportAudibleStep(event.data.step);
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

  // A Play that beat the previous Stop's drain ends it here: the stopped
  // engine may never render the -1 chunk reportAudibleStep is waiting for,
  // and the playhead would then stay dark for the whole next pass.
  awaitingStopDrain = false;
  command("setRunning", true);
}

export function stop(): void {
  command("setRunning", false);

  // The worklet still holds ~2048 already-rendered samples (~43 ms) tagged
  // with the steps they were rendered on. Darkening the playhead now and
  // letting those chunks report would flash it backwards through the last
  // steps, so suppress them until the engine's first stopped chunk arrives.
  awaitingStopDrain = true;
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

// setVoiceParam sets one of a voice's synthesis parameters from a normalized
// position in [0, 1]; the (track, index) addressing and the per-voice tables
// live in engine/voiceParams.ts (generated from the Go engine).
export function setVoiceParam(
  track: number,
  index: number,
  value: number,
): void {
  command("setVoiceParam", track, index, value);
}

// setPhysicalTomParam addresses the physical model's independent generated
// parameter bank, regardless of which Tom model is currently selected.
export function setPhysicalTomParam(index: number, value: number): void {
  command("setPhysicalTomParam", index, value);
}

// setTomModel explicitly switches only the Tom track between the unchanged
// procedural voice and the experimental physical modal implementation.
export function setTomModel(model: TomModel): void {
  command("setTomModel", model === "physical" ? 1 : 0);
}

// triggerVoice fires one voice immediately, independent of the sequencer, so
// the voice editor can audition a sound while the transport is stopped.
//
// That is the case startAudio() would otherwise miss: the graph is only built
// inside play(), so with no context there is nothing pulling chunks and the
// hit would be silent. The audition click is itself the user gesture the
// autoplay policy requires, so building and resuming here is safe.
export async function triggerVoice(
  track: number,
  velocity: number,
): Promise<void> {
  await startAudio();

  if (audioCtx && audioCtx.state === "suspended") {
    await audioCtx.resume();
  }

  command("triggerVoice", track, velocity);
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

// Vite swaps this module on save without unloading the old one, so every edit
// during development would otherwise strand another worker and AudioContext —
// audible as the same pattern playing several times over.
import.meta.hot?.dispose(() => {
  dispose();
});
