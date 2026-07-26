// Engine-owned pattern mirroring (the single source of truth lives in Go).
//
// The UI applies pattern edits optimistically for immediate feedback, but the
// Go engine's copy is authoritative: the worker echoes the full pattern back
// after every mutation (setCell/setPattern), and this mirror decides when an
// echo is current enough to hand to the UI. Echoes that arrive while newer
// edits are still in flight are skipped — applying them would briefly revert
// the newer optimistic state — so listeners only ever see a pattern that
// reflects every edit sent so far.

import { PATTERN_SIZE } from "../algo/pattern";

export type PatternListener = (pattern: Float32Array) => void;

export class PatternMirror {
  private readonly listeners = new Set<PatternListener>();
  private inFlight = 0;

  // subscribe registers a listener for authoritative pattern snapshots and
  // returns an unsubscribe function.
  subscribe(listener: PatternListener): () => void {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  }

  // beginMutation records that a pattern edit was sent to the engine and its
  // echo is still outstanding.
  beginMutation(): void {
    this.inFlight++;
  }

  // receiveSync handles a pattern echoed back by the engine. Listeners are
  // notified only when no newer edits are outstanding and the echo is a full
  // snapshot.
  receiveSync(pattern: Float32Array): void {
    if (this.inFlight > 0) this.inFlight--;
    if (this.inFlight > 0) return;

    // Anything but a full snapshot is unusable: an empty array means the
    // engine was not ready, and a short one means version skew (a differently
    // sized grid). Consumers pad missing cells with 0, so publishing either
    // would silently wipe tracks instead of failing loudly.
    if (pattern.length !== PATTERN_SIZE) return;

    this.listeners.forEach((listener) => listener(pattern));
  }

  // reset re-bases the in-flight count when the worker is torn down: edits
  // already sent to the dead worker will never be echoed, while edits still
  // queued on the main thread will be replayed to the replacement worker and
  // echoed exactly once each, so the caller passes how many of those it holds.
  reset(queuedEdits = 0): void {
    this.inFlight = queuedEdits;
  }
}
