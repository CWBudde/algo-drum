// Type declaration for the Go-exported global
declare global {
  interface Window {
    AlgoDrum: {
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
    };
  }
}

// Go runtime — loaded via <script src="wasm_exec.js"> in index.html
declare class Go {
  importObject: WebAssembly.Imports;
  run(instance: WebAssembly.Instance): Promise<void>;
}

const SAMPLE_RATE = 48000;

let audioCtx: AudioContext | null = null;
let processor: ScriptProcessorNode | null = null;
let wasmReady = false;

export async function loadWasm(): Promise<void> {
  if (wasmReady) return;

  const go = new (window as unknown as { Go: typeof Go }).Go();
  const result = await WebAssembly.instantiateStreaming(
    fetch(import.meta.env.BASE_URL + "algo_drum.wasm"),
    go.importObject,
  );
  go.run(result.instance); // keeps running via select{}

  // Create the engine immediately so pattern and parameter edits made
  // before the first Play are not lost. The AudioContext is created later,
  // on the first user gesture, at the same fixed sample rate.
  window.AlgoDrum.init(SAMPLE_RATE);
  wasmReady = true;
}

export function startAudio(): void {
  if (audioCtx) return;

  audioCtx = new AudioContext({ sampleRate: SAMPLE_RATE });

  const bufferSize = 4096;
  processor = audioCtx.createScriptProcessor(bufferSize, 0, 1);
  processor.onaudioprocess = (e) => {
    const output = e.outputBuffer.getChannelData(0);
    const samples = window.AlgoDrum.render(bufferSize);
    output.set(samples);
  };
  processor.connect(audioCtx.destination);
}

export async function play(): Promise<void> {
  startAudio();

  // Autoplay policies (notably iOS Safari) can leave a freshly created
  // context suspended; without an explicit resume there is no sound.
  if (audioCtx && audioCtx.state === "suspended") {
    await audioCtx.resume();
  }

  window.AlgoDrum.setRunning(true);
}

export function stop(): void {
  window.AlgoDrum.setRunning(false);
}

export function setTempo(bpm: number): void {
  if (wasmReady) window.AlgoDrum.setTempo(bpm);
}

export function setSwing(swing: number): void {
  if (wasmReady) window.AlgoDrum.setSwing(swing);
}

export function setCell(track: number, step: number, active: boolean): void {
  if (wasmReady) window.AlgoDrum.setCell(track, step, active);
}

export function setVolume(track: number, vol: number): void {
  if (wasmReady) window.AlgoDrum.setVolume(track, vol);
}

export function setDecay(track: number, amount: number): void {
  if (wasmReady) window.AlgoDrum.setDecay(track, amount);
}

export function setReverb(amount: number): void {
  if (wasmReady) window.AlgoDrum.setReverb(amount);
}

export function currentStep(): number {
  if (!wasmReady) return -1;
  return window.AlgoDrum.currentStep();
}
