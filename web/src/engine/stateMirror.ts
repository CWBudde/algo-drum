import { validateEngineState, type EngineState } from "./engineState";

export type StateListener = (state: EngineState) => void;

// The UI applies edits optimistically, while the Go engine owns the clamped
// state. StateMirror suppresses intermediate echoes until every newer edit is
// represented, then publishes the authoritative full snapshot.
export class StateMirror {
  private readonly listeners = new Set<StateListener>();
  private inFlight = 0;

  subscribe(listener: StateListener): () => void {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  }

  beginMutation(): void {
    this.inFlight++;
  }

  receiveSync(candidate: unknown): void {
    if (this.inFlight > 0) this.inFlight--;
    if (this.inFlight > 0) return;

    let state: EngineState;
    try {
      state = validateEngineState(candidate);
    } catch {
      return;
    }

    this.listeners.forEach((listener) => listener(state));
  }

  reset(queuedMutations = 0): void {
    this.inFlight = queuedMutations;
  }
}
