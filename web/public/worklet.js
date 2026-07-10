// AudioWorkletProcessor for algo-drum.
//
// Audio is rendered by the Go WASM engine inside a Web Worker; this processor
// only consumes pre-rendered chunks from a queue. The worker talks to us
// directly over a MessagePort (handed over via this.port at setup), so no
// audio data ever touches the main thread.
//
// Each chunk carries the sequencer step it starts on; when playback reaches a
// chunk with a new step we notify the main thread, so the UI playhead tracks
// what is audible instead of what has been rendered ahead.

const CHUNK_SAMPLES = 512; // must match the worker's render chunk size
const TARGET_QUEUE_SAMPLES = 2048; // ~43 ms of buffer at 48 kHz

class AlgoDrumProcessor extends AudioWorkletProcessor {
  constructor() {
    super();
    this.chunks = []; // { samples: Float32Array, step: number }
    this.readOffset = 0;
    this.pendingRequests = 0;
    this.workerPort = null;
    this.lastStep = -2;

    this.port.onmessage = (event) => {
      if (event.data?.type === "workerPort" && event.ports[0]) {
        this.workerPort = event.ports[0];
        this.workerPort.onmessage = (msg) => {
          this.pendingRequests = Math.max(0, this.pendingRequests - 1);
          this.chunks.push({
            samples: new Float32Array(msg.data.buffer),
            step: msg.data.step,
          });
        };
        this.requestChunks();
      }
    };
  }

  queuedSamples() {
    let total = -this.readOffset;
    for (const chunk of this.chunks) total += chunk.samples.length;
    return total;
  }

  requestChunks() {
    if (!this.workerPort) return;
    while (
      this.queuedSamples() + this.pendingRequests * CHUNK_SAMPLES <
      TARGET_QUEUE_SAMPLES
    ) {
      this.pendingRequests++;
      this.workerPort.postMessage({ type: "need", samples: CHUNK_SAMPLES });
    }
  }

  process(_inputs, outputs) {
    const out = outputs[0][0];
    let written = 0;

    while (written < out.length && this.chunks.length > 0) {
      const chunk = this.chunks[0];

      if (this.readOffset === 0 && chunk.step !== this.lastStep) {
        this.lastStep = chunk.step;
        this.port.postMessage({ type: "step", step: chunk.step });
      }

      const available = chunk.samples.length - this.readOffset;
      const n = Math.min(out.length - written, available);
      out.set(
        chunk.samples.subarray(this.readOffset, this.readOffset + n),
        written,
      );
      written += n;
      this.readOffset += n;

      if (this.readOffset >= chunk.samples.length) {
        this.chunks.shift();
        this.readOffset = 0;
      }
    }

    // Underrun leaves the tail at silence (output buffers arrive zeroed).
    this.requestChunks();
    return true;
  }
}

registerProcessor("algo-drum", AlgoDrumProcessor);
