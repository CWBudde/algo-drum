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
  PHYSICAL_TOM_PARAMS,
  VOICE_PARAM_CAPACITY,
} from "../engine/voiceParams";

const V1_BYTES = 38;
const LEGACY_TRACK_COUNT = 5;
const V2_BYTES = V1_BYTES + LEGACY_TRACK_COUNT * VOICE_PARAM_CAPACITY;
const V3_BYTES = V2_BYTES + 1;
const V4_PHYSICAL_TOM_PARAM_CAPACITY = 13;
const V4_BYTES = V3_BYTES + V4_PHYSICAL_TOM_PARAM_CAPACITY;
const V5_PHYSICAL_TOM_PARAM_CAPACITY = 15;
const V6_BYTES = V3_BYTES + V5_PHYSICAL_TOM_PARAM_CAPACITY;
const EXTRA_TRACK_COUNT = TRACK_COUNT - LEGACY_TRACK_COUNT;
const LEGACY_VISUAL_TO_CURRENT = [0, 3, 4, 5, 6] as const;
const V8_BYTES =
  V6_BYTES +
  EXTRA_TRACK_COUNT * 2 +
  1 +
  (EXTRA_TRACK_COUNT * 16) / 4 +
  EXTRA_TRACK_COUNT * VOICE_PARAM_CAPACITY;
const V9_BYTES = V8_BYTES + 1 + V5_PHYSICAL_TOM_PARAM_CAPACITY;
const V10_PHYSICAL_TOM_PARAM_CAPACITY = 16;
// The byte constants above describe the v9 layout. v10 widened the Tom 1 bank
// in the middle of the record, so anything after it sits one slot later.
const V10_TOM2_MODEL_OFFSET =
  V8_BYTES + (PHYSICAL_TOM_PARAM_CAPACITY - V5_PHYSICAL_TOM_PARAM_CAPACITY);
const TOTAL_BYTES =
  V9_BYTES + (PHYSICAL_TOM_PARAM_CAPACITY - V5_PHYSICAL_TOM_PARAM_CAPACITY) * 2;

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
  pattern[PATTERN_SIZE - 1] = VEL_ACCENT;
  return {
    pattern,
    steps: q(1.0),
    tempo: q(0.43),
    swing: q(0.2),
    reverb: q(0.6),
    prob: q(0.85),
    humanize: q(0.33),
    volumes: [q(0.2), q(0.4), q(0.75), q(0.5), q(1.0), q(0.0), q(0.9)],
    decays: [q(0.7), q(0.3), q(0.5), q(0.25), q(0.8), q(0.1), q(0.6)],
    muted: [true, false, false, true, false, false, true],
    voiceParams: Array.from({ length: TRACK_COUNT }, (_, track) =>
      Array.from({ length: VOICE_PARAM_CAPACITY }, (_, i) =>
        q((track * VOICE_PARAM_CAPACITY + i) / 60),
      ),
    ),
    tomModel: "physical",
    tom2Model: "physical",
    physicalTomParams: Array.from(
      { length: PHYSICAL_TOM_PARAM_CAPACITY },
      (_, i) => q((i + 1) / (PHYSICAL_TOM_PARAM_CAPACITY + 1)),
    ),
    physicalTom2Params: Array.from(
      { length: PHYSICAL_TOM_PARAM_CAPACITY },
      (_, i) => q((PHYSICAL_TOM_PARAM_CAPACITY - i) / 20),
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

  for (const visualIndex of LEGACY_VISUAL_TO_CURRENT)
    bytes[offset++] = toByte(state.volumes[visualIndex]);
  for (const visualIndex of LEGACY_VISUAL_TO_CURRENT)
    bytes[offset++] = toByte(state.decays[visualIndex]);

  let muteMask = 0;
  for (let i = 0; i < LEGACY_TRACK_COUNT; i++)
    if (state.muted[LEGACY_VISUAL_TO_CURRENT[i]]) muteMask |= 1 << i;
  bytes[offset++] = muteMask;

  for (let i = 0; i < (LEGACY_TRACK_COUNT * 16) / 4; i++) {
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

// Versions 2–9 are all prefixes of the v9 layout, so every fixture below is
// that layout truncated — which keeps them anchored to encodeState instead of
// duplicating it, and makes the truncation point the documentation of what each
// version added.
//
// v10 is the first release that is not a strict append: it widened the physical
// Tom bank sitting in the middle of the record, shifting every later offset, and
// v12 widened it again. So the reversal is parameterized by the target width
// rather than hard-coded to one slot — hard-coding it silently stops reversing
// anything the next time a bank grows.
function bytesAtBankWidth(state: PersistedState, width: number): Uint8Array {
  const current = toBytes(encodeState(state));
  const dropped = new Set<number>();
  for (let i = 0; i < PHYSICAL_TOM_PARAM_CAPACITY - width; i++) {
    dropped.add(V3_BYTES + PHYSICAL_TOM_PARAM_CAPACITY - 1 - i); // Tom 1 tail
    dropped.add(current.length - 1 - i); // Tom 2 tail
  }

  return current.filter((_, index) => !dropped.has(index));
}

function preV10Bytes(state: PersistedState): Uint8Array {
  return bytesAtBankWidth(state, V5_PHYSICAL_TOM_PARAM_CAPACITY);
}

// v11 changed no bytes at all, only the meaning of one stored position, so v10
// and v11 share a layout and differ only in their version byte.
function encodeAtV10Width(state: PersistedState, version: number): string {
  const bytes = bytesAtBankWidth(state, V10_PHYSICAL_TOM_PARAM_CAPACITY);
  bytes[0] = version;
  return toB64Url(bytes);
}

function encodeV10(state: PersistedState): string {
  return encodeAtV10Width(state, 10);
}

function encodeLegacy(
  state: PersistedState,
  version: number,
  length: number,
): string {
  const bytes = preV10Bytes(state).slice(0, length);
  bytes[0] = version;
  return toB64Url(bytes);
}

function encodeV2(state: PersistedState): string {
  return encodeLegacy(state, 2, V2_BYTES);
}

function encodeV3(state: PersistedState): string {
  return encodeLegacy(state, 3, V3_BYTES);
}

function encodeV4(state: PersistedState): string {
  return encodeLegacy(state, 4, V4_BYTES);
}

function encodeV5(state: PersistedState): string {
  return encodeLegacy(state, 5, V6_BYTES);
}

function encodeV6(state: PersistedState): string {
  return encodeLegacy(state, 6, V6_BYTES);
}

function encodeV7(state: PersistedState): string {
  return encodeLegacy(state, 7, V8_BYTES);
}

function encodeV8(state: PersistedState): string {
  return encodeLegacy(state, 8, V8_BYTES);
}

function encodeV9(state: PersistedState): string {
  return encodeLegacy(state, 9, V9_BYTES);
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

    it("roundtrips Tom 2 model selection independently", () => {
      const state = {
        ...makeState(),
        tomModel: "procedural" as const,
        tom2Model: "physical" as const,
      };
      const decoded = decodeState(encodeState(state));
      expect(decoded?.tomModel).toBe("procedural");
      expect(decoded?.tom2Model).toBe("physical");
    });

    it("rejects an unknown model code", () => {
      const bytes = toBytes(encodeState(makeState()));
      bytes[V2_BYTES] = 2;
      expect(decodeState(toB64Url(bytes))).toBeNull();
    });

    it("rejects an unknown Tom 2 model code", () => {
      const bytes = toBytes(encodeState(makeState()));
      bytes[V10_TOM2_MODEL_OFFSET] = 2;
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

    it("roundtrips Tom 2's independent physical bank", () => {
      const state = makeState();
      expect(decodeState(encodeState(state))?.physicalTom2Params).toEqual(
        state.physicalTom2Params,
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

  describe("backward compatibility with v1–v8 blobs", () => {
    it("decodes a v1 blob and leaves voiceParams unset", () => {
      const state = makeState();
      const decoded = decodeState(encodeV1(state));

      expect(decoded).not.toBeNull();
      expect(decoded?.pattern.slice(0, 80)).toEqual(state.pattern.slice(0, 80));
      expect(decoded?.pattern.slice(80)).toEqual(
        new Array<number>(32).fill(VEL_OFF),
      );
      expect(decoded?.tempo).toBe(state.tempo);
      expect(decoded?.swing).toBe(state.swing);
      expect(decoded?.volumes).toEqual([
        state.volumes[0],
        0.75,
        0.75,
        ...state.volumes.slice(3),
      ]);
      expect(decoded?.decays).toEqual([
        state.decays[0],
        0.5,
        0.5,
        ...state.decays.slice(3),
      ]);
      expect(decoded?.muted).toEqual([
        state.muted[0],
        false,
        false,
        ...state.muted.slice(3),
      ]);
      expect(decoded?.voiceParams).toBeUndefined();
      expect(decoded?.tomModel).toBeUndefined();
      expect(decoded?.physicalTomParams).toBeUndefined();
    });

    it("decodes a v2 blob and defaults the Tom model at the call site", () => {
      const state = makeState();
      const decoded = decodeState(encodeV2(state));

      expect(decoded?.voiceParams).toEqual(
        state.voiceParams?.slice(0, LEGACY_TRACK_COUNT),
      );
      expect(decoded?.tomModel).toBeUndefined();
      expect(decoded?.physicalTomParams).toBeUndefined();
    });

    it("decodes a v3 blob and leaves physical parameters unset", () => {
      const state = makeState();
      const decoded = decodeState(encodeV3(state));

      expect(decoded?.voiceParams).toEqual(
        state.voiceParams?.slice(0, LEGACY_TRACK_COUNT),
      );
      expect(decoded?.tomModel).toBe(state.tomModel);
      expect(decoded?.physicalTomParams).toBeUndefined();
    });

    it("decodes a v4 blob and leaves appended asymmetry controls unset", () => {
      const state = makeState();
      const decoded = decodeState(encodeV4(state));

      expect(decoded?.physicalTomParams).toEqual(
        state.physicalTomParams?.slice(0, V4_PHYSICAL_TOM_PARAM_CAPACITY),
      );
    });

    it("moves the old shipped strike radius to the corrected default", () => {
      const state = makeState();
      state.physicalTomParams![4] = q(0.45 / 0.95);

      for (const encoded of [encodeV4(state), encodeV5(state)]) {
        const decoded = decodeState(encoded);
        expect(decoded?.physicalTomParams?.[4]).toBe(
          PHYSICAL_TOM_PARAMS[4].default,
        );
      }
    });

    it("preserves an edited legacy strike radius", () => {
      const state = makeState();
      state.physicalTomParams![4] = q(0.8);

      expect(decodeState(encodeV5(state))?.physicalTomParams?.[4]).toBe(q(0.8));
    });

    it("moves the v6 central strike detent off centre in both banks", () => {
      const state = makeState();
      state.physicalTomParams![4] = q(0.12 / 0.95);
      state.physicalTom2Params![4] = q(0.12 / 0.95);

      // v9 is the first version with a Tom 2 bank, so it and v10 are the two
      // that can carry the detent there. The v6 rule deliberately skipped that
      // bank; this one must not.
      for (const encoded of [encodeV9(state), encodeV10(state)]) {
        const decoded = decodeState(encoded);
        expect(decoded?.physicalTomParams?.[4]).toBe(
          PHYSICAL_TOM_PARAMS[4].default,
        );
        expect(decoded?.physicalTom2Params?.[4]).toBe(
          PHYSICAL_TOM_PARAMS[4].default,
        );
      }
    });

    it("preserves a strike radius edited back to the v4 detent", () => {
      const state = makeState();
      state.physicalTomParams![4] = q(0.45 / 0.95);

      // A v6-or-later blob sitting at 0.45 is a deliberate edit, not the
      // shipped position, so the two detent rules must stay separately gated.
      expect(decodeState(encodeV10(state))?.physicalTomParams?.[4]).toBe(
        q(0.45 / 0.95),
      );
    });

    it("decodes v6 with the two added tracks at defaults", () => {
      const decoded = decodeState(encodeV6(makeState()));

      expect(decoded?.volumes).toEqual([
        makeState().volumes[0],
        0.75,
        0.75,
        ...makeState().volumes.slice(3),
      ]);
      expect(decoded?.decays).toEqual([
        makeState().decays[0],
        0.5,
        0.5,
        ...makeState().decays.slice(3),
      ]);
      expect(decoded?.muted).toEqual([
        makeState().muted[0],
        false,
        false,
        ...makeState().muted.slice(3),
      ]);
      expect(decoded?.pattern.slice(80)).toEqual(
        new Array<number>(32).fill(VEL_OFF),
      );
      expect(decoded?.voiceParams).toHaveLength(LEGACY_TRACK_COUNT);
    });

    it("keeps v7 mixer values attached to their voices after reordering", () => {
      const state = makeState();
      const decoded = decodeState(encodeV7(state));

      expect(decoded?.volumes).toEqual(state.volumes);
      expect(decoded?.decays).toEqual(state.decays);
      expect(decoded?.muted).toEqual(state.muted);
      expect(decoded?.tom2Model).toBeUndefined();
      expect(decoded?.physicalTom2Params).toBeUndefined();
    });

    it("decodes v8 with Tom 2 physical settings absent", () => {
      const decoded = decodeState(encodeV8(makeState()));

      expect(decoded?.tom2Model).toBeUndefined();
      expect(decoded?.physicalTom2Params).toBeUndefined();
    });

    it("decodes v9 with both physical banks one slot narrower", () => {
      const state = makeState();
      const decoded = decodeState(encodeV9(state));

      expect(decoded?.physicalTomParams).toEqual(
        state.physicalTomParams?.slice(0, V5_PHYSICAL_TOM_PARAM_CAPACITY),
      );
      expect(decoded?.physicalTom2Params).toEqual(
        state.physicalTom2Params?.slice(0, V5_PHYSICAL_TOM_PARAM_CAPACITY),
      );
      expect(decoded?.tom2Model).toBe(state.tom2Model);
    });

    it("re-encodes a decoded v1 state as v13 (one-way upgrade)", () => {
      const decoded = decodeState(encodeV1(makeState()));
      const bytes = toBytes(encodeState(decoded!));

      expect(bytes).toHaveLength(TOTAL_BYTES);
      expect(bytes[0]).toBe(13);
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
      bytes[0] = 14;
      expect(decodeState(toB64Url(bytes))).toBeNull();
    });

    // v13 changed no bytes, only how one slot is read, so a v12 blob is a v13
    // blob with a different version byte — which makes the migration itself the
    // only thing under test here.
    it("rescales a v12 attack level onto the narrowed range", () => {
      const state = makeState();
      // 0.4 of the old 0–0.3 range is 0.12, which is 0.8 of the new 0–0.15 one.
      const edited = Math.round(0.4 * 255) / 255;
      state.physicalTomParams = [...state.physicalTomParams!];
      state.physicalTomParams[16] = edited;
      state.physicalTom2Params = [...state.physicalTom2Params!];
      state.physicalTom2Params[16] = edited;

      const bytes = toBytes(encodeState(state));
      bytes[0] = 12;

      const decoded = decodeState(toB64Url(bytes));
      expect(decoded).not.toBeNull();
      expect(decoded?.physicalTomParams?.[16]).toBeCloseTo(edited * 2, 6);
      // Tom 2's bank is a separate call site and has been missed before.
      expect(decoded?.physicalTom2Params?.[16]).toBeCloseTo(edited * 2, 6);
    });

    it("moves an untouched v12 attack level onto the new default", () => {
      const state = makeState();
      state.physicalTomParams = [...state.physicalTomParams!];
      state.physicalTomParams[16] = Math.round((0.1 / 0.3) * 255) / 255;

      const bytes = toBytes(encodeState(state));
      bytes[0] = 12;

      const decoded = decodeState(toB64Url(bytes));
      expect(decoded?.physicalTomParams?.[16]).toBeCloseTo(
        PHYSICAL_TOM_PARAMS[16].default,
        6,
      );
    });

    // This is the case a version bump is most likely to break, and breaking it
    // loses every pattern in every user's localStorage and every share link
    // already sent: the previous version needs its own length and its own bank
    // width, and the fixtures only ever covered versions older than that.
    it.each([10, 11])("decodes v%i at its own bank width", (version) => {
      const state = makeState();
      const decoded = decodeState(encodeAtV10Width(state, version));

      expect(decoded).not.toBeNull();
      expect(decoded?.physicalTomParams).toHaveLength(
        V10_PHYSICAL_TOM_PARAM_CAPACITY,
      );
      expect(decoded?.physicalTom2Params).toHaveLength(
        V10_PHYSICAL_TOM_PARAM_CAPACITY,
      );
      // Everything after the Tom 1 bank decodes at the right offset only if the
      // bank width is right, so these are the desynchronization detectors.
      expect(decoded?.pattern).toEqual(state.pattern);
      expect(decoded?.muted).toEqual(state.muted);
      expect(decoded?.voiceParams).toEqual(state.voiceParams);
      expect(decoded?.tom2Model).toBe(state.tom2Model);
      expect(decoded?.physicalTom2Params).toEqual(
        state.physicalTom2Params?.slice(0, V10_PHYSICAL_TOM_PARAM_CAPACITY),
      );
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
