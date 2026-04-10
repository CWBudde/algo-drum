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
      setKit: (kit: number) => void;
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
  wasmReady = true;
}

export function startAudio(): void {
  if (audioCtx) return;

  audioCtx = new AudioContext({ sampleRate: 48000 });
  window.AlgoDrum.init(audioCtx.sampleRate);

  const bufferSize = 4096;
  processor = audioCtx.createScriptProcessor(bufferSize, 0, 1);
  processor.onaudioprocess = (e) => {
    const output = e.outputBuffer.getChannelData(0);
    const samples = window.AlgoDrum.render(bufferSize);
    output.set(samples);
  };
  processor.connect(audioCtx.destination);
}

export function play(): void {
  startAudio();
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


export function setKit(kit: number): void {
  if (wasmReady) window.AlgoDrum.setKit(kit);
}
