import { describe, expect, it, vi } from "vitest";
import { PATTERN_SIZE } from "../algo/pattern";
import { PatternMirror } from "./patternMirror";

// A full-size echo whose leading cells hold the given velocities — the mirror
// only accepts complete snapshots, so every valid echo has PATTERN_SIZE cells.
const pattern = (...values: number[]): Float32Array => {
  const flat = new Float32Array(PATTERN_SIZE);
  flat.set(values);
  return flat;
};

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
    mirror.receiveSync(new Float32Array(0));

    expect(listener).not.toHaveBeenCalled();

    // The empty echo still settles the in-flight edit, so the next sync lands.
    mirror.receiveSync(pattern(0.7));
    expect(listener).toHaveBeenCalledExactlyOnceWith(pattern(0.7));
  });

  it("ignores a short echo from a version-skewed engine", () => {
    const mirror = new PatternMirror();
    const listener = vi.fn();
    mirror.subscribe(listener);

    // Publishing this would wipe every cell past the truncation, because the
    // consumer pads missing cells with 0.
    mirror.receiveSync(new Float32Array(PATTERN_SIZE - 1).fill(1));

    expect(listener).not.toHaveBeenCalled();
  });

  it("ignores an over-long echo from a version-skewed engine", () => {
    const mirror = new PatternMirror();
    const listener = vi.fn();
    mirror.subscribe(listener);

    mirror.receiveSync(new Float32Array(PATTERN_SIZE + 1).fill(1));

    expect(listener).not.toHaveBeenCalled();
  });

  it("keeps counting edits after a malformed echo", () => {
    const mirror = new PatternMirror();
    const listener = vi.fn();
    mirror.subscribe(listener);

    // A rejected echo must still settle its edit, or the mirror would gate
    // every later sync forever.
    mirror.beginMutation();
    mirror.receiveSync(new Float32Array(3));

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

  it("reset re-bases on edits that will be replayed to a new worker", () => {
    const mirror = new PatternMirror();
    const listener = vi.fn();
    mirror.subscribe(listener);

    // One edit died with the old worker; two more are queued and will be
    // replayed (and echoed) once the replacement worker is ready.
    mirror.beginMutation();
    mirror.reset(2);

    mirror.receiveSync(pattern(0.7));
    expect(listener).not.toHaveBeenCalled();

    mirror.receiveSync(pattern(0.7, 1.0));
    expect(listener).toHaveBeenCalledExactlyOnceWith(pattern(0.7, 1.0));
  });
});
