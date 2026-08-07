// AudioWorkletProcessor for algo-drum.
//
// Audio is rendered by the Go WASM engine inside a Web Worker; this processor
// only consumes pre-rendered chunks from a queue. The worker talks to us
// directly over a MessagePort (handed over via this.port at setup), so no
// audio data ever touches the main thread.
//
// Each chunk carries an engine-owned transport snapshot; when playback reaches
// a new revision or step we notify the main thread, so the UI playhead tracks
// what is audible and can reject pre-transition chunks still in this queue.
// Chunks also carry
// the engine's idle flag, and reaching a chunk whose flag differs from the one
// playing is what tells the main thread to suspend (or stop suspending) the
// AudioContext — again keyed on audibility, so a tail still ringing in the
// queue is never cut off.

// How many samples we ask the worker for per chunk. The worker honours
// whatever size the request carries, so this is purely our own granularity
// trade-off: smaller chunks react to transport changes sooner, larger ones
// cost fewer messages. It only has to divide TARGET_QUEUE_SAMPLES evenly for
// the credit accounting below to land exactly on the target.
const CHUNK_SAMPLES = 512;
const TARGET_QUEUE_SAMPLES = 2048; // ~43 ms of buffer at 48 kHz

// Credit watchdog. pendingRequests is our outstanding-request credit, and it
// is only paid back when a chunk actually arrives, so a request the worker
// answers with nothing leaks a credit permanently; four leaks pin the request
// condition below false and audio stops for good. The worker is written to
// always reply, but it is a separate process that can be killed, reloaded or
// left suspended (the worklet is not called while the context is suspended,
// so a resume can find the credit stale). If we go this many consecutive
// quanta with nothing to play while still believing requests are in flight,
// we write the credit off and ask again. ~133 ms at 128 samples/quantum.
const MAX_STARVED_QUANTA = 50;

// Leading-edge throttle on underrun reports: the first underrun after a quiet
// period is posted immediately, further ones are aggregated until this many
// quanta have passed (~341 ms at 128 samples/quantum, so ~3 reports/s at
// worst). A sustained fault must not flood the main thread.
const UNDERRUN_REPORT_QUANTA = 128;

// Spent chunk buffers are transferred back to the worker to be refilled. The
// alternative — a SharedArrayBuffer ring with no messaging at all — is
// deliberately rejected: SAB requires cross-origin isolation (COOP/COEP
// headers), which this repository removed on purpose, and GitHub Pages cannot
// set them anyway. Do not reintroduce it without bringing those headers back.

class AlgoDrumProcessor extends AudioWorkletProcessor {
  constructor() {
    super();
    this.chunks = []; // { samples, transport: { state, step, revision }, idle }
    this.readOffset = 0;
    this.pendingRequests = 0;
    this.workerPort = null;
    this.lastTransport = null;

    // null, not a boolean: the first chunk must always report its idle state,
    // whichever way it points.
    this.lastIdle = null;

    // Consecutive quanta spent with an empty queue and outstanding requests.
    this.starvedQuanta = 0;

    // Underrun accounting since the last report to the main thread. Primed at
    // the throttle interval so the first underrun is reported immediately.
    this.underrunSamples = 0;
    this.underrunCount = 0;
    this.quantaSinceUnderrunReport = UNDERRUN_REPORT_QUANTA;

    this.port.onmessage = (event) => {
      if (event.data?.type === "workerPort" && event.ports[0]) {
        this.workerPort = event.ports[0];
        this.workerPort.onmessage = (msg) => {
          this.pendingRequests = Math.max(0, this.pendingRequests - 1);
          this.starvedQuanta = 0;
          this.chunks.push({
            samples: new Float32Array(msg.data.buffer),
            transport: msg.data.transport,
            idle: msg.data.idle === true,
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

  // recycle hands a drained chunk's storage back to the worker, which refills
  // it instead of allocating a fresh ~2 KB Float32Array per chunk (~94/s at
  // 48 kHz, every one of them transferred away and dropped).
  recycle(buffer) {
    if (!this.workerPort || buffer.byteLength === 0) return;
    this.workerPort.postMessage({ type: "recycle", buffer }, [buffer]);
  }

  // trackUnderrun accumulates the samples this quantum could not fill and
  // posts a throttled summary. Report only: the queue target is fixed, so the
  // documented ~43 ms latency does not move under load.
  trackUnderrun(missing) {
    this.quantaSinceUnderrunReport++;

    if (missing > 0) {
      this.underrunSamples += missing;
      this.underrunCount++;
    }

    if (
      this.underrunCount === 0 ||
      this.quantaSinceUnderrunReport < UNDERRUN_REPORT_QUANTA
    ) {
      return;
    }

    this.port.postMessage({
      type: "underrun",
      samples: this.underrunSamples,
      count: this.underrunCount,
    });
    this.underrunSamples = 0;
    this.underrunCount = 0;
    this.quantaSinceUnderrunReport = 0;
  }

  // checkCredit writes off outstanding requests that will never be answered.
  // See MAX_STARVED_QUANTA.
  checkCredit() {
    if (this.chunks.length > 0 || this.pendingRequests === 0) return;

    this.starvedQuanta++;
    if (this.starvedQuanta < MAX_STARVED_QUANTA) return;

    this.starvedQuanta = 0;
    this.pendingRequests = 0;
  }

  process(_inputs, outputs) {
    const out = outputs[0][0];
    let written = 0;

    while (written < out.length && this.chunks.length > 0) {
      const chunk = this.chunks[0];

      if (this.readOffset === 0) {
        const transport = chunk.transport;
        if (
          !this.lastTransport ||
          transport.state !== this.lastTransport.state ||
          transport.step !== this.lastTransport.step ||
          transport.revision !== this.lastTransport.revision
        ) {
          this.lastTransport = transport;
          this.port.postMessage({ type: "transport", transport });
        }

        // Changes only: every chunk carries the flag, but the main thread
        // only needs the edges.
        if (chunk.idle !== this.lastIdle) {
          this.lastIdle = chunk.idle;
          this.port.postMessage({ type: "idle", idle: chunk.idle });
        }
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
        this.recycle(chunk.samples.buffer);
      }
    }

    // Underrun leaves the tail at silence (output buffers arrive zeroed), so
    // it is inaudible as anything but a dropout — hence the report.
    this.trackUnderrun(out.length - written);
    this.checkCredit();
    this.requestChunks();
    return true;
  }
}

registerProcessor("algo-drum", AlgoDrumProcessor);
