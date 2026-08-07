// Main-thread bridge to the drum engine.
//
// The Go WASM engine runs inside a dedicated Worker (audioWorker.ts); an
// AudioWorklet (public/worklet.js) pulls rendered chunks from it over a
// direct MessageChannel. This module only sends control commands and mirrors
// the audible sequencer step reported back by the worklet.

import { PROTOCOL_VERSION } from "./audioWorker";
import type {
  AlgoDrumApi,
  TransportSnapshot,
  TransportState,
  WorkerCommand,
  WorkerResponse,
} from "./audioWorker";
export type { TransportState } from "./audioWorker";
import {
  cloneEngineState,
  isConfigurationMethod,
  validateEngineState,
  type ConfigurationMethod,
  type EngineState,
  type PatternBankState,
} from "./engineState";
import { StateMirror } from "./stateMirror";
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

// Audio-lifecycle guard for a Play that is awaiting context startup or a
// worker acknowledgement. This is deliberately not exposed as transport
// state; only a successful worker acknowledgement may update that view.
let keepAudioAwake = false;

type TransportListener = (state: TransportState) => void;

let transportState: TransportState = "stopped";
let transportRevision = 0;
let playAttempt = 0;
const transportListeners = new Set<TransportListener>();

export interface BankPlayback {
  activeBank: number;
  queuedBank: number;
  chainPosition: number;
}

type BankPlaybackListener = (snapshot: BankPlayback) => void;

let bankPlayback: BankPlayback = {
  activeBank: 0,
  queuedBank: -1,
  chainPosition: -1,
};
const bankPlaybackListeners = new Set<BankPlaybackListener>();

type FailureListener = (error: Error) => void;

const failureListeners = new Set<FailureListener>();

// onFailure reports faults that invalidate a ready engine. Load failures are
// still returned by loadWasm(); this channel lets App offer the same Retry UI
// when an already-running worker or renderer dies.
export function onFailure(listener: FailureListener): () => void {
  failureListeners.add(listener);
  return () => failureListeners.delete(listener);
}

// onTransport subscribes to worker-confirmed transport changes and immediately
// replays the current state. The worker acknowledgement, not the button
// handler, is the authority for whether playback actually started.
export function onTransport(listener: TransportListener): () => void {
  transportListeners.add(listener);
  listener(transportState);

  return () => transportListeners.delete(listener);
}

// onBankPlayback mirrors the bank that is audible, a manually queued bank,
// and the current chain position independently from the transport state.
// Keeping this separate avoids rerendering transport-only consumers for bank
// changes while still replaying the complete current snapshot to new users.
export function onBankPlayback(listener: BankPlaybackListener): () => void {
  bankPlaybackListeners.add(listener);
  listener(bankPlayback);

  return () => bankPlaybackListeners.delete(listener);
}

function notifyBankPlayback(snapshot: BankPlayback): void {
  if (
    snapshot.activeBank === bankPlayback.activeBank &&
    snapshot.queuedBank === bankPlayback.queuedBank &&
    snapshot.chainPosition === bankPlayback.chainPosition
  ) {
    return;
  }

  bankPlayback = snapshot;
  bankPlaybackListeners.forEach((listener) => listener(snapshot));
}

function notifyTransport(state: TransportState): void {
  keepAudioAwake = state === "starting" || state === "playing";
  if (state === transportState) return;

  transportState = state;
  transportListeners.forEach((listener) => listener(state));
}

// Messages the worklet posts to the main thread over the node's port.
type WorkletMessage =
  | { type: "transport"; transport: TransportSnapshot }
  | { type: "underrun"; samples: number; count: number }
  | { type: "idle"; idle: boolean };

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

function validTransportSnapshot(value: TransportSnapshot): boolean {
  const inactive = value.state === "stopped" || value.state === "starting";
  return (
    (inactive || value.state === "playing" || value.state === "paused") &&
    Number.isSafeInteger(value.revision) &&
    value.revision >= 0 &&
    Number.isInteger(value.step) &&
    value.step >= -1 &&
    ((inactive && value.step === -1) || (!inactive && value.step >= 0)) &&
    Number.isInteger(value.activeBank) &&
    value.activeBank >= 0 &&
    value.activeBank < 4 &&
    Number.isInteger(value.queuedBank) &&
    value.queuedBank >= -1 &&
    value.queuedBank < 4 &&
    Number.isInteger(value.chainPosition) &&
    value.chainPosition >= -1 &&
    value.chainPosition < 16
  );
}

// acceptTransport arbitrates the worker's immediate engine snapshot and the
// same snapshot carried by audio that reaches the speakers later. Revisions
// make old queued chunks unambiguously stale, including a rapid Stop -> Play
// where both the old and new audio say "playing".
function acceptTransport(
  transport: TransportSnapshot,
  source: "engine" | "audible",
): void {
  if (!validTransportSnapshot(transport)) {
    console.error("Audio engine returned an invalid transport snapshot");
    return;
  }

  if (transport.revision < transportRevision) return;

  if (transport.revision > transportRevision) {
    transportRevision = transport.revision;

    // Starting and stopped have no active playhead. This is an explicit
    // engine-state rule, not an inference from currentStep's -1 sentinel.
    if (transport.state === "starting" || transport.state === "stopped") {
      if (audibleStep !== -1) notifyStep(-1);
    }
  } else if (transport.state !== transportState) {
    console.error("Audio engine reused a transport revision for a new state");
    return;
  }

  // Also refresh lifecycle guards when a failed command reads back the same
  // state and revision (notably a rejected Play returning stopped@0).
  notifyTransport(transport.state);
  const audible = source === "audible";
  const activeBankChanged =
    audible && transport.activeBank !== bankPlayback.activeBank;
  notifyBankPlayback({
    // While audio is flowing, only the worklet knows which bank has reached
    // the speakers. A stopped engine has no audible chunks, so its immediate
    // acknowledgement is authoritative instead.
    activeBank:
      audible || transport.state === "stopped"
        ? transport.activeBank
        : bankPlayback.activeBank,
    // An older chunk can share the transport revision of a later bank request.
    // Do not let its pre-request queuedBank value erase the worker's immediate
    // acknowledgement; the audible snapshot owns this field only when it also
    // announces that the requested bank has actually become active.
    queuedBank:
      source === "engine" || activeBankChanged
        ? transport.queuedBank
        : bankPlayback.queuedBank,
    chainPosition:
      audible || transport.state === "stopped"
        ? transport.chainPosition
        : bankPlayback.chainPosition,
  });

  if (source === "audible" && transport.step !== audibleStep) {
    notifyStep(transport.step);
  }
}

type UnderrunListener = (report: { samples: number; count: number }) => void;

const underrunListeners = new Set<UnderrunListener>();

// Timestamp of the last underrun warning, so a sustained dropout does not
// bury the console. The worklet already throttles its reports; this only
// guards against a second source of noise on top of them.
let lastUnderrunLog = 0;
const UNDERRUN_LOG_INTERVAL_MS = 2000;

// onUnderrun subscribes to audio dropout reports: the worklet ran out of
// rendered samples and emitted silence for `samples` frames across `count`
// render quanta since the previous report. Diagnostic only — nothing in the
// UI reacts to it, and the queue depth is deliberately not adaptive, so the
// documented ~43 ms of output latency stays put.
export function onUnderrun(listener: UnderrunListener): () => void {
  underrunListeners.add(listener);

  return () => underrunListeners.delete(listener);
}

function reportUnderrun(samples: number, count: number): void {
  const now = Date.now();
  if (now - lastUnderrunLog >= UNDERRUN_LOG_INTERVAL_MS) {
    lastUnderrunLog = now;
    console.warn(
      `Audio underrun: ${samples} samples of silence across ${count} render quanta.`,
    );
  }

  underrunListeners.forEach((listener) => listener({ samples, count }));
}

// reportIdle acts on the engine going quiet: with nothing left ringing there
// is no reason to keep the audio hardware and the render pull loop awake, so
// the context is suspended until the next play() or audition resumes it.
//
// The engine only reports idle once its output has actually gone silent, well
// after a Stop, so suspending here can never cut a tail short. The transport
// check is a second belt: a running sequencer must keep pulling even through a
// silent passage, or the playhead would freeze.
function reportIdle(idle: boolean): void {
  if (
    !idle ||
    keepAudioAwake ||
    transportState === "starting" ||
    transportState === "playing" ||
    !audioCtx
  )
    return;

  // A context that refuses to suspend simply keeps running, which is the
  // pre-existing behaviour. The promise is kept so resumeAudio can wait it
  // out rather than race it — see there.
  const pending = audioCtx.suspend().catch(() => undefined);
  suspending = pending;
  void pending.finally(() => {
    if (suspending === pending) suspending = null;
  });
}

// An idle suspend that has not resolved yet. resume() on a context whose
// suspend() is still in flight is not reliably ordered, and losing that race
// leaves a suspended context nothing will wake: the worklet is not called
// while suspended, so it cannot even notice.
let suspending: Promise<void> | null = null;

// resumeAudio undoes an idle suspend, and equally the suspended state
// autoplay policies (notably iOS Safari) leave a freshly created context in.
// Both callers reach it from a user gesture, which is what the policy wants.
async function resumeAudio(): Promise<void> {
  await suspending;

  if (audioCtx && audioCtx.state === "suspended") {
    await audioCtx.resume();
  }
}

// The Go engine owns all configuration. Full snapshots reconcile its clamps
// and cross-field semantics with optimistic edits still in flight.
const stateMirror = new StateMirror();

export function onState(listener: (state: EngineState) => void): () => void {
  return stateMirror.subscribe(listener);
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

function configurationCommand<K extends ConfigurationMethod>(
  name: K,
  ...args: Parameters<AlgoDrumApi[K]>
): void {
  stateMirror.beginMutation();
  command(name, ...args);
}

function isConfigurationCommand(cmd: WorkerCommand): boolean {
  return cmd.type === "cmd" && isConfigurationMethod(cmd.name);
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
  stateMirror.reset(pendingCommands.filter(isConfigurationCommand).length);
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
  let readyReported = false;

  const ready = new Promise<void>((resolve, reject) => {
    active.onmessage = (event: MessageEvent<WorkerResponse>) => {
      // terminate() cannot retract a message already queued on the main
      // thread. Never let a superseded worker overwrite its replacement's
      // transport or configuration mirror.
      if (worker !== active) return;

      const data = event.data;
      switch (data.type) {
        case "ready":
          readyReported = true;
          resolve();
          break;
        case "error":
          // Before readiness this settles the load and the app renders the
          // message. Afterwards ordinary command failures are diagnostic,
          // while a fatal render fault invalidates the whole engine.
          if (readyReported) {
            const error = new Error(data.error);
            if (data.fatal) {
              reportWorkerUnavailable(active, error);
            } else {
              console.error("Audio engine error:", data.error);
            }
          } else {
            reject(new Error(data.error));
          }
          break;
        case "stateSync":
          stateMirror.receiveSync(data.state);
          break;
        case "transportSync":
          acceptTransport(data.transport, "engine");
          break;
      }
    };

    // A worker can also fail without ever sending a message: a module that
    // 404s, or a throw at its top level. Without these handlers the load
    // promise would never settle and the UI would hang on "Loading engine…".
    active.onerror = (event) => {
      // message is empty for cross-origin failures.
      const error = new Error(
        event.message || "Audio engine worker failed to start",
      );
      if (readyReported) {
        reportWorkerUnavailable(active, error);
      } else {
        reject(error);
      }
    };
    active.onmessageerror = () => {
      const error = new Error(
        "Audio engine worker sent an undeliverable message",
      );
      if (readyReported) {
        reportWorkerUnavailable(active, error);
      } else {
        reject(error);
      }
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

// A worker-level error after readiness means the confirmed transport can no
// longer advance or answer commands. Force the public mirror back to Stop;
// late events from a superseded worker must not overwrite its replacement.
function reportWorkerUnavailable(active: Worker, error: Error): void {
  if (worker !== active) return;

  console.error("Audio engine worker failed:", error);
  teardownAudio();
  teardownWorker();
  wasmReady = false;
  playAttempt++;
  keepAudioAwake = false;
  notifyTransport("stopped");
  transportRevision = 0;
  notifyBankPlayback({ activeBank: 0, queuedBank: -1, chainPosition: -1 });
  notifyStep(-1);
  failureListeners.forEach((listener) => listener(error));
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

  // Any pending suspend belongs to the context that just went away.
  suspending = null;
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
  stateMirror.reset();

  // An in-flight load is waiting on a worker that no longer exists.
  cancelLoad?.(new Error("Audio engine was disposed"));
  cancelLoad = null;
  loadAttempt = null;

  wasmReady = false;
  playAttempt++;
  keepAudioAwake = false;
  notifyTransport("stopped");
  transportRevision = 0;
  notifyBankPlayback({ activeBank: 0, queuedBank: -1, chainPosition: -1 });
  notifyStep(-1);
}

async function startAudio(): Promise<void> {
  if (audioCtx || !worker) return;

  try {
    const ctx = new AudioContext({ sampleRate: SAMPLE_RATE });
    audioCtx = ctx;

    // worklet.js is copied from public/ without a content hash and can still
    // be served by the previous service worker on the first load after a
    // deploy. Its message shape is part of the live protocol, so version the
    // URL just like the WASM handshake rather than pairing incompatible code.
    await ctx.audioWorklet.addModule(
      `${import.meta.env.BASE_URL}worklet.js?v=${PROTOCOL_VERSION}`,
    );

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
    // Idle edges and dropouts come back over the same port.
    node.port.onmessage = (event: MessageEvent<WorkletMessage>) => {
      const message = event.data;

      switch (message.type) {
        case "transport":
          acceptTransport(message.transport, "audible");
          break;
        case "underrun":
          reportUnderrun(message.samples, message.count);
          break;
        case "idle":
          reportIdle(message.idle);
          break;
      }
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
  const attempt = ++playAttempt;

  // Set before the awaits below: an idle report delivered while they are
  // pending would otherwise suspend the context we are about to resume.
  keepAudioAwake = true;
  command("beginStart");

  try {
    await startAudio();
    await resumeAudio();
  } catch (error) {
    if (playAttempt === attempt) {
      keepAudioAwake = false;
      command("setRunning", false);
    }
    throw error;
  }

  // Stop or a newer Play issued while the browser was awaiting its audio graph
  // supersedes this attempt. Its engine-owned starting epoch has already been
  // replaced by the later command, so it must not commit Playing now.
  if (playAttempt !== attempt) return;

  command("setRunning", true);
}

// pause freezes sequencer time while allowing active voices and reverb to
// ring out. Unlike stop(), the engine keeps reporting the held step and a
// later play() resumes from the same fractional position.
export function pause(): void {
  playAttempt++;
  keepAudioAwake = false;
  command("pause");
}

export function stop(): void {
  playAttempt++;
  keepAudioAwake = false;
  command("setRunning", false);
}

export function setTempo(bpm: number): void {
  configurationCommand("setTempo", bpm);
}

export function setSwing(swing: number): void {
  configurationCommand("setSwing", swing);
}

// setStepCount sets the active pattern length (clamped to 1–16 in the
// engine); cells beyond it are kept, just not played.
export function setStepCount(bank: number, steps: number): void {
  configurationCommand("setStepCount", bank, steps);
}

export function setCell(
  bank: number,
  track: number,
  step: number,
  velocity: number,
): void {
  configurationCommand("setCell", bank, track, step, velocity);
}

export function setCellProbability(
  bank: number,
  track: number,
  step: number,
  probability: number,
): void {
  configurationCommand("setCellProbability", bank, track, step, probability);
}

export function setCellHumanize(
  bank: number,
  track: number,
  step: number,
  humanize: number,
): void {
  configurationCommand("setCellHumanize", bank, track, step, humanize);
}

export function setCellCondition(
  bank: number,
  track: number,
  step: number,
  condition: number,
): void {
  configurationCommand("setCellCondition", bank, track, step, condition);
}

export function setTrackLength(
  bank: number,
  track: number,
  length: number,
): void {
  configurationCommand("setTrackLength", bank, track, length);
}

export function setFillMode(enabled: boolean): void {
  configurationCommand("setFillMode", enabled);
}

// setPattern replaces the whole pattern: a flat track-major Float32Array of
// TrackCount×MaxSteps (7×16) velocities in [0, 1], index = track*16 + step.
export function setPattern(bank: number, pattern: Float32Array): void {
  configurationCommand("setPattern", bank, pattern.slice());
}

export function setPatternBank(bank: number, state: PatternBankState): void {
  configurationCommand("setPatternBank", bank, {
    stepCount: state.stepCount,
    pattern: state.pattern.slice(),
    cellProbabilities: state.cellProbabilities.slice(),
    cellHumanize: state.cellHumanize.slice(),
    cellConditions: state.cellConditions.slice(),
    trackLengths: state.trackLengths.slice(),
  });
}

export function requestBank(bank: number): void {
  configurationCommand("requestBank", bank);
}

export function setChain(chain: Uint8Array): void {
  configurationCommand("setChain", chain.slice());
}

export function setChainEnabled(enabled: boolean): void {
  configurationCommand("setChainEnabled", enabled);
}

// setState atomically seeds every configurable field. Clone before queueing so
// a caller cannot mutate typed-array storage while a worker is still loading.
export function setState(state: EngineState): void {
  const snapshot = cloneEngineState(validateEngineState(state));
  configurationCommand("setState", snapshot);
}

export function setVolume(track: number, vol: number): void {
  configurationCommand("setVolume", track, vol);
}

export function setDecay(track: number, amount: number): void {
  configurationCommand("setDecay", track, amount);
}

export function setMuted(track: number, muted: boolean): void {
  configurationCommand("setMuted", track, muted);
}

// setVoiceParam sets one of a voice's synthesis parameters from a normalized
// position in [0, 1]; the (track, index) addressing and the per-voice tables
// live in engine/voiceParams.ts (generated from the Go engine).
export function setVoiceParam(
  track: number,
  index: number,
  value: number,
): void {
  configurationCommand("setVoiceParam", track, index, value);
}

// setPhysicalTomParam addresses one Tom track's independent generated
// physical parameter bank, regardless of which model is currently selected.
export function setPhysicalTomParam(
  track: number,
  index: number,
  value: number,
): void {
  configurationCommand("setPhysicalTomParam", track, index, value);
}

// setTomModel switches either Tom track between its procedural voice and an
// independent experimental physical modal implementation.
export function setTomModel(track: number, model: TomModel): void {
  configurationCommand("setTomModel", track, model === "physical" ? 1 : 0);
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
  await resumeAudio();

  command("triggerVoice", track, velocity);
}

export function setReverb(amount: number): void {
  configurationCommand("setReverb", amount);
}

// setProbability sets the per-hit trigger chance in [0, 1] (1 = every hit).
export function setProbability(p: number): void {
  configurationCommand("setProbability", p);
}

// setHumanize sets the timing/velocity randomization amount in [0, 1].
export function setHumanize(h: number): void {
  configurationCommand("setHumanize", h);
}

// Vite swaps this module on save without unloading the old one, so every edit
// during development would otherwise strand another worker and AudioContext —
// audible as the same pattern playing several times over.
import.meta.hot?.dispose(() => {
  dispose();
});
