import { describe, expect, it, vi } from "vitest";
import { PatternMirror } from "./patternMirror";

const pattern = (...values: number[]): Float32Array =>
  Float32Array.from(values);

describe("PatternMirror", () => {
  it("notifies subscribers of an unsolicited sync", () => {
    const mirror = new PatternMirror();
    const listener = vi.fn();
    mirror.subscribe(listener);

    mirror.receiveSync(pattern(0.7, 0));

    expect(listener).toHaveBeenCalledExactlyOnceWith(pattern(0.7, 0));
  });

  it("skips echoes while newer edits are still in flight", () => {
    const mirror = new PatternMirror();
    const listener = vi.fn();
    mirror.subscribe(listener);

    // Two rapid edits: the first echo reflects only the first edit and must
    // not overwrite the newer optimistic state; the second echo settles it.
    mirror.beginMutation();
    mirror.beginMutation();

    mirror.receiveSync(pattern(0.7));
    expect(listener).not.toHaveBeenCalled();

    mirror.receiveSync(pattern(1.0));
    expect(listener).toHaveBeenCalledExactlyOnceWith(pattern(1.0));
  });

  it("ignores empty patterns from a not-yet-ready engine", () => {
    const mirror = new PatternMirror();
    const listener = vi.fn();
    mirror.subscribe(listener);

    mirror.beginMutation();
    mirror.receiveSync(pattern());

    expect(listener).not.toHaveBeenCalled();

    // The empty echo still settles the in-flight edit, so the next sync lands.
    mirror.receiveSync(pattern(0.7));
    expect(listener).toHaveBeenCalledExactlyOnceWith(pattern(0.7));
  });

  it("stops notifying after unsubscribe", () => {
    const mirror = new PatternMirror();
    const listener = vi.fn();
    const unsubscribe = mirror.subscribe(listener);

    unsubscribe();
    mirror.receiveSync(pattern(0.7));

    expect(listener).not.toHaveBeenCalled();
  });

  it("reset clears in-flight edits so future syncs are not gated", () => {
    const mirror = new PatternMirror();
    const listener = vi.fn();
    mirror.subscribe(listener);

    // Edits sent to a worker that then died: their echoes never arrive.
    mirror.beginMutation();
    mirror.beginMutation();
    mirror.reset();

    mirror.receiveSync(pattern(0.7));
    expect(listener).toHaveBeenCalledExactlyOnceWith(pattern(0.7));
  });
});
