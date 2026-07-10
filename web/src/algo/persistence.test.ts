import { describe, expect, it } from "vitest";
import { decodeState, encodeState, type PersistedState } from "./persistence";
import { PATTERN_SIZE, VEL_ACCENT, VEL_NORMAL, VEL_OFF } from "./pattern";

// Scalars are stored as one byte, so only multiples of 1/255 survive a
// roundtrip exactly. Quantize test inputs so equality is bit-exact.
const q = (x: number): number => Math.round(x * 255) / 255;

// base64url <-> bytes helpers for tampering with encoded blobs in tests.
function toBytes(s: string): Uint8Array {
  const b = atob(s.replace(/-/g, "+").replace(/_/g, "/"));
  return Uint8Array.from(b, (c) => c.charCodeAt(0));
}
function toB64Url(bytes: Uint8Array): string {
  let bin = "";
  for (const byte of bytes) bin += String.fromCharCode(byte);
  return btoa(bin).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

function makeState(): PersistedState {
  const pattern = new Array<number>(PATTERN_SIZE).fill(VEL_OFF);
  pattern[0] = VEL_ACCENT;
  pattern[3] = VEL_NORMAL;
  pattern[16] = VEL_NORMAL; // snare track, step 0
  pattern[79] = VEL_ACCENT; // last cell
  return {
    pattern,
    steps: q(1.0),
    tempo: q(0.43),
    swing: q(0.2),
    reverb: q(0.6),
    prob: q(0.85),
    humanize: q(0.33),
    volumes: [q(0.75), q(0.5), q(1.0), q(0.0), q(0.9)],
    decays: [q(0.5), q(0.25), q(0.8), q(0.1), q(0.6)],
    muted: [false, true, false, false, true],
  };
}

describe("persistence encode/decode", () => {
  it("roundtrips a representative state exactly", () => {
    const state = makeState();
    const decoded = decodeState(encodeState(state));
    expect(decoded).toEqual(state);
  });

  it("roundtrips pattern velocities exactly (3-state quantization)", () => {
    const state = makeState();
    // Fill every cell with a rotating off/normal/accent value.
    state.pattern = Array.from(
      { length: PATTERN_SIZE },
      (_, i) => [VEL_OFF, VEL_NORMAL, VEL_ACCENT][i % 3],
    );
    const decoded = decodeState(encodeState(state));
    expect(decoded?.pattern).toEqual(state.pattern);
  });

  it("is a stable fixed point (re-encoding decoded state is identical)", () => {
    const encoded = encodeState(makeState());
    const decoded = decodeState(encoded);
    expect(decoded).not.toBeNull();
    expect(encodeState(decoded!)).toBe(encoded);
  });

  it("emits URL-safe base64 (no +, /, or = padding)", () => {
    const encoded = encodeState(makeState());
    expect(encoded).toMatch(/^[A-Za-z0-9_-]+$/);
  });

  describe("rejects bad input with null", () => {
    it("garbage / non-base64 strings", () => {
      expect(decodeState("@@@not-valid@@@")).toBeNull();
      expect(decodeState("")).toBeNull();
    });

    it("too-short (wrong length) blobs", () => {
      expect(decodeState("AAAA")).toBeNull(); // decodes to 3 bytes
    });

    it("a wrong format version", () => {
      const bytes = toBytes(encodeState(makeState()));
      bytes[0] = 99; // corrupt the version byte, keep correct length
      expect(decodeState(toB64Url(bytes))).toBeNull();
    });
  });
});
