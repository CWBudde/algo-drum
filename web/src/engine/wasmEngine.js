let audioCtx = null;
let processor = null;
let wasmReady = false;
export async function loadWasm() {
    if (wasmReady)
        return;
    const go = new window.Go();
    const result = await WebAssembly.instantiateStreaming(fetch(import.meta.env.BASE_URL + "algo_drum.wasm"), go.importObject);
    go.run(result.instance); // keeps running via select{}
    wasmReady = true;
}
export function startAudio() {
    if (audioCtx)
        return;
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
export function play() {
    startAudio();
    window.AlgoDrum.setRunning(true);
}
export function stop() {
    window.AlgoDrum.setRunning(false);
}
export function setTempo(bpm) {
    if (wasmReady)
        window.AlgoDrum.setTempo(bpm);
}
export function setSwing(swing) {
    if (wasmReady)
        window.AlgoDrum.setSwing(swing);
}
export function setCell(track, step, active) {
    if (wasmReady)
        window.AlgoDrum.setCell(track, step, active);
}
export function setVolume(track, vol) {
    if (wasmReady)
        window.AlgoDrum.setVolume(track, vol);
}
export function setDecay(track, amount) {
    if (wasmReady)
        window.AlgoDrum.setDecay(track, amount);
}
export function setReverb(amount) {
    if (wasmReady)
        window.AlgoDrum.setReverb(amount);
}
export function currentStep() {
    if (!wasmReady)
        return -1;
    return window.AlgoDrum.currentStep();
}
