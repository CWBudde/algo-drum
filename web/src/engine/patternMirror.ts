// Engine-owned pattern mirroring (the single source of truth lives in Go).
//
// The UI applies pattern edits optimistically for immediate feedback, but the
// Go engine's copy is authoritative: the worker echoes the full pattern back
// after every mutation (setCell/setPattern), and this mirror decides when an
// echo is current enough to hand to the UI. Echoes that arrive while newer
// edits are still in flight are skipped — applying them would briefly revert
// the newer optimistic state — so listeners only ever see a pattern that
// reflects every edit sent so far.

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
  // notified only when no newer edits are outstanding and the engine actually
  // produced a pattern (an empty array means it was not ready yet).
  receiveSync(pattern: Float32Array): void {
    if (this.inFlight > 0) this.inFlight--;
    if (this.inFlight > 0 || pattern.length === 0) return;
    this.listeners.forEach((listener) => listener(pattern));
  }

  // reset drops the in-flight count when the worker is torn down — edits sent
  // to a dead worker will never be echoed and must not gate future syncs.
  reset(): void {
    this.inFlight = 0;
  }
}
