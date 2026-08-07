import { describe, expect, it, vi } from "vitest";
import { createDefaultEngineState } from "./engineState";
import { StateMirror } from "./stateMirror";

describe("StateMirror", () => {
  it("publishes a valid unsolicited snapshot", () => {
    const mirror = new StateMirror();
    const listener = vi.fn();
    const state = createDefaultEngineState();
    mirror.subscribe(listener);

    mirror.receiveSync(state);

    expect(listener).toHaveBeenCalledExactlyOnceWith(state);
  });

  it("suppresses intermediate echoes while newer edits are in flight", () => {
    const mirror = new StateMirror();
    const listener = vi.fn();
    mirror.subscribe(listener);
    mirror.beginMutation();
    mirror.beginMutation();

    const stale = createDefaultEngineState();
    stale.tempoBpm = 130;
    mirror.receiveSync(stale);
    expect(listener).not.toHaveBeenCalled();

    const current = createDefaultEngineState();
    current.tempoBpm = 140;
    mirror.receiveSync(current);
    expect(listener).toHaveBeenCalledExactlyOnceWith(current);
  });

  it("rejects malformed snapshots without stalling later state", () => {
    const mirror = new StateMirror();
    const listener = vi.fn();
    mirror.subscribe(listener);
    mirror.beginMutation();

    mirror.receiveSync({ tempoBpm: 120 });
    expect(listener).not.toHaveBeenCalled();

    const valid = createDefaultEngineState();
    mirror.receiveSync(valid);
    expect(listener).toHaveBeenCalledExactlyOnceWith(valid);
  });

  it("publishes engine clamp reconciliation", () => {
    const mirror = new StateMirror();
    const listener = vi.fn();
    mirror.subscribe(listener);
    mirror.beginMutation();

    const clamped = createDefaultEngineState();
    clamped.tempoBpm = 300;
    mirror.receiveSync(clamped);

    expect(listener).toHaveBeenCalledExactlyOnceWith(clamped);
  });

  it("re-bases retry accounting on queued mutations", () => {
    const mirror = new StateMirror();
    const listener = vi.fn();
    mirror.subscribe(listener);
    mirror.beginMutation(); // lost with the dead worker
    mirror.reset(2); // replayed to the replacement worker

    mirror.receiveSync(createDefaultEngineState());
    expect(listener).not.toHaveBeenCalled();

    const final = createDefaultEngineState();
    final.humanize = 0.25;
    mirror.receiveSync(final);
    expect(listener).toHaveBeenCalledExactlyOnceWith(final);
  });

  it("stops publishing after unsubscribe", () => {
    const mirror = new StateMirror();
    const listener = vi.fn();
    const unsubscribe = mirror.subscribe(listener);

    unsubscribe();
    mirror.receiveSync(createDefaultEngineState());

    expect(listener).not.toHaveBeenCalled();
  });
});
