import { describe, expect, it } from "vitest";
import { decodeState, encodeState, type PersistedState } from "./persistence";
import {
  PATTERN_SIZE,
  TRACK_COUNT,
  VEL_ACCENT,
  VEL_NORMAL,
  VEL_OFF,
} from "./pattern";
import {
  PHYSICAL_TOM_PARAM_CAPACITY,
  VOICE_PARAM_CAPACITY,
} from "../engine/voiceParams";

const V1_BYTES = 38;
const V2_BYTES = V1_BYTES + TRACK_COUNT * VOICE_PARAM_CAPACITY;
const V3_BYTES = V2_BYTES + 1;
const TOTAL_BYTES = V3_BYTES + PHYSICAL_TOM_PARAM_CAPACITY;

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
    voiceParams: Array.from({ length: TRACK_COUNT }, (_, track) =>
      Array.from({ length: VOICE_PARAM_CAPACITY }, (_, i) =>
        q((track * VOICE_PARAM_CAPACITY + i) / 40),
      ),
    ),
    tomModel: "physical",
    physicalTomParams: Array.from(
      { length: PHYSICAL_TOM_PARAM_CAPACITY },
      (_, i) => q((i + 1) / (PHYSICAL_TOM_PARAM_CAPACITY + 1)),
    ),
  };
}

// encodeV1 builds a pre-voice-editor blob by hand, so the backward-compat
// tests document the v1 layout instead of depending on a captured golden.
function encodeV1(state: PersistedState): string {
  const bytes = new Uint8Array(V1_BYTES);
  const toByte = (v: number) => Math.round(Math.min(1, Math.max(0, v)) * 255);
  let offset = 0;

  bytes[offset++] = 1; // format version
  bytes[offset++] = toByte(state.steps);
  bytes[offset++] = toByte(state.tempo);
  bytes[offset++] = toByte(state.swing);
  bytes[offset++] = toByte(state.reverb);
  bytes[offset++] = toByte(state.prob);
  bytes[offset++] = toByte(state.humanize);

  for (let i = 0; i < 5; i++) bytes[offset++] = toByte(state.volumes[i]);
  for (let i = 0; i < 5; i++) bytes[offset++] = toByte(state.decays[i]);

  let muteMask = 0;
  for (let i = 0; i < 5; i++) if (state.muted[i]) muteMask |= 1 << i;
  bytes[offset++] = muteMask;

  for (let i = 0; i < PATTERN_SIZE / 4; i++) {
    let packed = 0;
    for (let j = 0; j < 4; j++) {
      const vel = state.pattern[i * 4 + j] ?? VEL_OFF;
      const code = vel >= VEL_ACCENT ? 2 : vel > VEL_OFF ? 1 : 0;
      packed |= code << (j * 2);
    }
    bytes[offset++] = packed;
  }

  return toB64Url(bytes);
}

// v3 is a strict one-byte append to v2, so trimming the selector documents
// and exercises the exact previous layout.
function encodeV2(state: PersistedState): string {
  const bytes = toBytes(encodeState(state)).slice(0, V2_BYTES);
  bytes[0] = 2;
  return toB64Url(bytes);
}

function encodeV3(state: PersistedState): string {
  const bytes = toBytes(encodeState(state)).slice(0, V3_BYTES);
  bytes[0] = 3;
  return toB64Url(bytes);
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

  it("encodes to the expected fixed width", () => {
    // A layout slip anywhere shows up here before it shows up as a null decode.
    expect(toBytes(encodeState(makeState()))).toHaveLength(TOTAL_BYTES);
  });

  describe("voice parameters", () => {
    it("roundtrips every slot exactly", () => {
      const state = makeState();
      const decoded = decodeState(encodeState(state));
      expect(decoded?.voiceParams).toEqual(state.voiceParams);
    });

    it("pads short rows and truncates long ones to the capacity", () => {
      const state = makeState();
      state.voiceParams = [
        [q(0.1), q(0.2), q(0.3)],
        ...Array.from({ length: TRACK_COUNT - 2 }, () =>
          new Array<number>(VOICE_PARAM_CAPACITY).fill(0),
        ),
        new Array<number>(VOICE_PARAM_CAPACITY + 2).fill(q(0.4)),
      ];

      const decoded = decodeState(encodeState(state));
      expect(decoded?.voiceParams?.[0]).toEqual([
        q(0.1),
        q(0.2),
        q(0.3),
        ...new Array<number>(VOICE_PARAM_CAPACITY - 3).fill(0),
      ]);
      expect(decoded?.voiceParams?.[TRACK_COUNT - 1]).toHaveLength(
        VOICE_PARAM_CAPACITY,
      );
    });

    it("clamps and sanitizes out-of-range values", () => {
      const state = makeState();
      state.voiceParams = state.voiceParams!.map(() => [
        NaN,
        -1,
        2,
        ...new Array<number>(VOICE_PARAM_CAPACITY - 3).fill(0),
      ]);

      const decoded = decodeState(encodeState(state));
      expect(decoded?.voiceParams?.[0].slice(0, 3)).toEqual([0, 0, 1]);
    });
  });

  describe("Tom model", () => {
    it("roundtrips both model selections", () => {
      for (const tomModel of ["procedural", "physical"] as const) {
        const state = { ...makeState(), tomModel };
        expect(decodeState(encodeState(state))?.tomModel).toBe(tomModel);
      }
    });

    it("rejects an unknown model code", () => {
      const bytes = toBytes(encodeState(makeState()));
      bytes[V2_BYTES] = 2;
      expect(decodeState(toB64Url(bytes))).toBeNull();
    });
  });

  describe("physical Tom parameters", () => {
    it("roundtrips every slot exactly", () => {
      const state = makeState();
      expect(decodeState(encodeState(state))?.physicalTomParams).toEqual(
        state.physicalTomParams,
      );
    });

    it("pads, truncates, clamps, and sanitizes the independent bank", () => {
      const state = makeState();
      state.physicalTomParams = [
        NaN,
        -1,
        2,
        ...new Array<number>(PHYSICAL_TOM_PARAM_CAPACITY + 2).fill(q(0.4)),
      ];

      const decoded = decodeState(encodeState(state))?.physicalTomParams;
      expect(decoded).toHaveLength(PHYSICAL_TOM_PARAM_CAPACITY);
      expect(decoded?.slice(0, 3)).toEqual([0, 0, 1]);
    });
  });

  describe("backward compatibility with v1/v2/v3 blobs", () => {
    it("decodes a v1 blob and leaves voiceParams unset", () => {
      const state = makeState();
      const decoded = decodeState(encodeV1(state));

      expect(decoded).not.toBeNull();
      expect(decoded?.pattern).toEqual(state.pattern);
      expect(decoded?.tempo).toBe(state.tempo);
      expect(decoded?.swing).toBe(state.swing);
      expect(decoded?.volumes).toEqual(state.volumes);
      expect(decoded?.decays).toEqual(state.decays);
      expect(decoded?.muted).toEqual(state.muted);
      expect(decoded?.voiceParams).toBeUndefined();
      expect(decoded?.tomModel).toBeUndefined();
      expect(decoded?.physicalTomParams).toBeUndefined();
    });

    it("decodes a v2 blob and defaults the Tom model at the call site", () => {
      const state = makeState();
      const decoded = decodeState(encodeV2(state));

      expect(decoded?.voiceParams).toEqual(state.voiceParams);
      expect(decoded?.tomModel).toBeUndefined();
      expect(decoded?.physicalTomParams).toBeUndefined();
    });

    it("decodes a v3 blob and leaves physical parameters unset", () => {
      const state = makeState();
      const decoded = decodeState(encodeV3(state));

      expect(decoded?.voiceParams).toEqual(state.voiceParams);
      expect(decoded?.tomModel).toBe(state.tomModel);
      expect(decoded?.physicalTomParams).toBeUndefined();
    });

    it("re-encodes a decoded v1 state as v4 (one-way upgrade)", () => {
      const decoded = decodeState(encodeV1(makeState()));
      const bytes = toBytes(encodeState(decoded!));

      expect(bytes).toHaveLength(TOTAL_BYTES);
      expect(bytes[0]).toBe(4);
    });

    it("rejects a v1-length blob whose version byte claims v2", () => {
      const bytes = toBytes(encodeV1(makeState()));
      bytes[0] = 2;
      expect(decodeState(toB64Url(bytes))).toBeNull();
    });

    it("rejects a v2-length blob whose version byte claims v1", () => {
      const bytes = toBytes(encodeV2(makeState()));
      bytes[0] = 1;
      expect(decodeState(toB64Url(bytes))).toBeNull();
    });

    it("rejects an unknown future version at the right length", () => {
      const bytes = toBytes(encodeState(makeState()));
      bytes[0] = 5;
      expect(decodeState(toB64Url(bytes))).toBeNull();
    });
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
