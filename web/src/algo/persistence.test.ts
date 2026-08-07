import { afterEach, describe, expect, it, vi } from "vitest";
import {
  createDefaultEngineState,
  type EngineState,
} from "../engine/engineState";
import {
  PHYSICAL_TOM_PARAM_CAPACITY,
  PHYSICAL_TOM_PARAMS,
  VOICE_PARAM_CAPACITY,
} from "../engine/voiceParams";
import {
  PATTERN_SIZE,
  TRACK_COUNT,
  VEL_ACCENT,
  VEL_NORMAL,
  VEL_OFF,
} from "./pattern";
import {
  buildShareUrl,
  decodeState,
  encodeState,
  replaceAddressBarWithShareUrl,
  shareUrl,
} from "./persistence";

const LEGACY_TRACK_COUNT = 5;
const LEGACY_PATTERN_SIZE = LEGACY_TRACK_COUNT * 16;
const V1_BYTES = 38;
const V2_BYTES = V1_BYTES + LEGACY_TRACK_COUNT * VOICE_PARAM_CAPACITY;
const V3_BYTES = V2_BYTES + 1;
const V4_PHYSICAL_CAPACITY = 13;
const V4_BYTES = V3_BYTES + V4_PHYSICAL_CAPACITY;
const V5_PHYSICAL_CAPACITY = 15;
const V6_BYTES = V3_BYTES + V5_PHYSICAL_CAPACITY;
const EXTRA_TRACK_COUNT = TRACK_COUNT - LEGACY_TRACK_COUNT;
const V7_EXTRA_BYTES =
  EXTRA_TRACK_COUNT * 2 +
  1 +
  (PATTERN_SIZE - LEGACY_PATTERN_SIZE) / 4 +
  EXTRA_TRACK_COUNT * VOICE_PARAM_CAPACITY;
const V8_BYTES = V6_BYTES + V7_EXTRA_BYTES;
const V9_BYTES = V8_BYTES + 1 + V5_PHYSICAL_CAPACITY;
const V10_PHYSICAL_CAPACITY = 16;
const V10_BYTES =
  V3_BYTES + V10_PHYSICAL_CAPACITY + V7_EXTRA_BYTES + 1 + V10_PHYSICAL_CAPACITY;
const V13_BYTES =
  V3_BYTES +
  PHYSICAL_TOM_PARAM_CAPACITY +
  V7_EXTRA_BYTES +
  1 +
  PHYSICAL_TOM_PARAM_CAPACITY;

const V14_PATTERN_OFFSET = 8;
const V14_PATTERN_BYTES = PATTERN_SIZE / 4;
const V14_MIXER_OFFSET = V14_PATTERN_OFFSET + V14_PATTERN_BYTES;
const V14_MUTE_OFFSET = V14_MIXER_OFFSET + TRACK_COUNT * 2;
const V14_VOICE_OFFSET = V14_MUTE_OFFSET + 1;
const V14_MODEL_OFFSET = V14_VOICE_OFFSET + TRACK_COUNT * VOICE_PARAM_CAPACITY;
const V14_PHYSICAL_OFFSET = V14_MODEL_OFFSET + 1;
const V14_BYTES = V14_PHYSICAL_OFFSET + 2 * PHYSICAL_TOM_PARAM_CAPACITY;
const V15_VELOCITY_OFFSET = V14_BYTES;
const V15_PROBABILITY_OFFSET = V15_VELOCITY_OFFSET + PATTERN_SIZE;
const V15_CONDITION_OFFSET = V15_PROBABILITY_OFFSET + PATTERN_SIZE;
const V15_TRACK_LENGTH_OFFSET = V15_CONDITION_OFFSET + PATTERN_SIZE / 2;
const V15_FLAGS_OFFSET = V15_TRACK_LENGTH_OFFSET + TRACK_COUNT;
const V15_BYTES = V15_FLAGS_OFFSET + 1;

const LEGACY_VISUAL_TO_CURRENT = [0, 3, 4, 5, 6] as const;
const ENGINE_TO_VISUAL = [6, 5, 4, 3, 0, 2, 1] as const;

const q = (x: number): number => Math.round(x * 255) / 255;

function toBytes(text: string): Uint8Array {
  const base64 = text.replace(/-/g, "+").replace(/_/g, "/");
  const binary = atob(base64.padEnd(Math.ceil(base64.length / 4) * 4, "="));
  return Uint8Array.from(binary, (char) => char.charCodeAt(0));
}

function toB64Url(bytes: Uint8Array): string {
  let binary = "";
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return btoa(binary)
    .replace(/\+/g, "-")
    .replace(/\//g, "_")
    .replace(/=+$/, "");
}

function makeState(): EngineState {
  const state = createDefaultEngineState();
  state.tempoBpm = 173;
  state.stepCount = 7;
  state.swing = q(0.4) * 0.5;
  state.reverb = q(0.6);
  state.probability = q(0.85);
  state.humanize = q(0.33);

  for (let i = 0; i < PATTERN_SIZE; i++) {
    state.pattern[i] = q([VEL_OFF, VEL_NORMAL, VEL_ACCENT][i % 3]);
    state.cellProbabilities[i] = q((i % 11) / 10);
    state.cellConditions[i] = i % 7;
  }

  state.trackLengths.set([16, 15, 14, 13, 12, 11, 10]);
  state.fillMode = true;

  for (let track = 0; track < TRACK_COUNT; track++) {
    const row = state.tracks[track];
    row.volume = q((track + 1) / 9);
    row.decay = q((8 - track) / 10);
    row.muted = track % 3 === 1;
    for (let i = 0; i < row.voiceParams.length; i++) {
      row.voiceParams[i] = q((track * VOICE_PARAM_CAPACITY + i + 1) / 50);
    }
  }

  state.tracks[3].tom.model = "physical";
  state.tracks[5].tom.model = "procedural";
  for (let i = 0; i < PHYSICAL_TOM_PARAM_CAPACITY; i++) {
    state.tracks[3].tom.physicalParams[i] = q((i + 1) / 20);
    state.tracks[5].tom.physicalParams[i] = q(
      (PHYSICAL_TOM_PARAM_CAPACITY - i) / 20,
    );
  }

  return state;
}

interface LegacyFixtureState {
  pattern: number[];
  steps: number;
  tempo: number;
  swing: number;
  reverb: number;
  prob: number;
  humanize: number;
  volumes: number[];
  decays: number[];
  muted: boolean[];
  voiceParams: number[][];
  tomModel: "procedural" | "physical";
  physicalTomParams: number[];
  tom2Model: "procedural" | "physical";
  physicalTom2Params: number[];
}

function makeLegacyState(): LegacyFixtureState {
  const pattern = new Array<number>(PATTERN_SIZE).fill(VEL_OFF);
  pattern[0] = VEL_ACCENT;
  pattern[3] = VEL_NORMAL;
  pattern[16] = VEL_NORMAL;
  pattern[PATTERN_SIZE - 1] = VEL_ACCENT;

  return {
    pattern,
    steps: q(0.6),
    tempo: q(0.43),
    swing: q(0.2),
    reverb: q(0.6),
    prob: q(0.85),
    humanize: q(0.33),
    volumes: [q(0.2), q(0.4), q(0.75), q(0.5), q(1), q(0), q(0.9)],
    decays: [q(0.7), q(0.3), q(0.5), q(0.25), q(0.8), q(0.1), q(0.6)],
    muted: [true, false, false, true, false, false, true],
    voiceParams: Array.from({ length: TRACK_COUNT }, (_, track) =>
      Array.from({ length: VOICE_PARAM_CAPACITY }, (_, i) =>
        q((track * VOICE_PARAM_CAPACITY + i + 1) / 50),
      ),
    ),
    tomModel: "physical",
    physicalTomParams: Array.from(
      { length: PHYSICAL_TOM_PARAM_CAPACITY },
      (_, i) => q((i + 1) / 20),
    ),
    tom2Model: "physical",
    physicalTom2Params: Array.from(
      { length: PHYSICAL_TOM_PARAM_CAPACITY },
      (_, i) => q((PHYSICAL_TOM_PARAM_CAPACITY - i) / 20),
    ),
  };
}

function legacyV13Bytes(state: LegacyFixtureState): Uint8Array {
  const bytes = new Uint8Array(V13_BYTES);
  const toByte = (value: number) =>
    Math.round(
      Math.min(1, Math.max(0, Number.isFinite(value) ? value : 0)) * 255,
    );
  let offset = 0;
  bytes[offset++] = 13;
  for (const scalar of [
    state.steps,
    state.tempo,
    state.swing,
    state.reverb,
    state.prob,
    state.humanize,
  ]) {
    bytes[offset++] = toByte(scalar);
  }

  for (const visual of LEGACY_VISUAL_TO_CURRENT)
    bytes[offset++] = toByte(state.volumes[visual]);
  for (const visual of LEGACY_VISUAL_TO_CURRENT)
    bytes[offset++] = toByte(state.decays[visual]);
  let muteMask = 0;
  for (let i = 0; i < LEGACY_TRACK_COUNT; i++) {
    if (state.muted[LEGACY_VISUAL_TO_CURRENT[i]]) muteMask |= 1 << i;
  }
  bytes[offset++] = muteMask;

  const packPattern = (start: number, byteCount: number) => {
    for (let i = 0; i < byteCount; i++) {
      let packed = 0;
      for (let j = 0; j < 4; j++) {
        const velocity = state.pattern[start + i * 4 + j] ?? VEL_OFF;
        const code = velocity >= VEL_ACCENT ? 2 : velocity > VEL_OFF ? 1 : 0;
        packed |= code << (j * 2);
      }
      bytes[offset++] = packed;
    }
  };
  packPattern(0, LEGACY_PATTERN_SIZE / 4);

  const writeVoiceRows = (from: number, to: number) => {
    for (let track = from; track < to; track++) {
      for (let i = 0; i < VOICE_PARAM_CAPACITY; i++)
        bytes[offset++] = toByte(state.voiceParams[track][i]);
    }
  };
  writeVoiceRows(0, LEGACY_TRACK_COUNT);
  bytes[offset++] = state.tomModel === "physical" ? 1 : 0;
  for (const value of state.physicalTomParams) bytes[offset++] = toByte(value);

  for (let track = LEGACY_TRACK_COUNT; track < TRACK_COUNT; track++) {
    bytes[offset++] = toByte(state.volumes[ENGINE_TO_VISUAL[track]]);
  }
  for (let track = LEGACY_TRACK_COUNT; track < TRACK_COUNT; track++) {
    bytes[offset++] = toByte(state.decays[ENGINE_TO_VISUAL[track]]);
  }
  let extraMuteMask = 0;
  for (let track = LEGACY_TRACK_COUNT; track < TRACK_COUNT; track++) {
    if (state.muted[ENGINE_TO_VISUAL[track]])
      extraMuteMask |= 1 << (track - LEGACY_TRACK_COUNT);
  }
  bytes[offset++] = extraMuteMask;
  packPattern(LEGACY_PATTERN_SIZE, (PATTERN_SIZE - LEGACY_PATTERN_SIZE) / 4);
  writeVoiceRows(LEGACY_TRACK_COUNT, TRACK_COUNT);
  bytes[offset++] = state.tom2Model === "physical" ? 1 : 0;
  for (const value of state.physicalTom2Params) bytes[offset++] = toByte(value);

  expect(offset).toBe(bytes.length);
  return bytes;
}

function bytesAtPhysicalWidth(
  state: LegacyFixtureState,
  width: number,
): Uint8Array {
  const current = legacyV13Bytes(state);
  const dropped = new Set<number>();
  for (let i = 0; i < PHYSICAL_TOM_PARAM_CAPACITY - width; i++) {
    dropped.add(V3_BYTES + PHYSICAL_TOM_PARAM_CAPACITY - 1 - i);
    dropped.add(current.length - 1 - i);
  }
  return current.filter((_, index) => !dropped.has(index));
}

function legacyBlob(state: LegacyFixtureState, version: number): string {
  let bytes: Uint8Array;
  switch (version) {
    case 1:
      bytes = bytesAtPhysicalWidth(state, V5_PHYSICAL_CAPACITY).slice(
        0,
        V1_BYTES,
      );
      break;
    case 2:
      bytes = bytesAtPhysicalWidth(state, V5_PHYSICAL_CAPACITY).slice(
        0,
        V2_BYTES,
      );
      break;
    case 3:
      bytes = bytesAtPhysicalWidth(state, V5_PHYSICAL_CAPACITY).slice(
        0,
        V3_BYTES,
      );
      break;
    case 4:
      bytes = bytesAtPhysicalWidth(state, V5_PHYSICAL_CAPACITY).slice(
        0,
        V4_BYTES,
      );
      break;
    case 5:
    case 6:
      bytes = bytesAtPhysicalWidth(state, V5_PHYSICAL_CAPACITY).slice(
        0,
        V6_BYTES,
      );
      break;
    case 7:
    case 8:
      bytes = bytesAtPhysicalWidth(state, V5_PHYSICAL_CAPACITY).slice(
        0,
        V8_BYTES,
      );
      break;
    case 9:
      bytes = bytesAtPhysicalWidth(state, V5_PHYSICAL_CAPACITY).slice(
        0,
        V9_BYTES,
      );
      break;
    case 10:
    case 11:
      bytes = bytesAtPhysicalWidth(state, V10_PHYSICAL_CAPACITY).slice(
        0,
        V10_BYTES,
      );
      break;
    case 12:
    case 13:
      bytes = legacyV13Bytes(state);
      break;
    default:
      throw new Error(`unsupported fixture version ${version}`);
  }
  bytes[0] = version;
  return toB64Url(bytes);
}

describe("v15 canonical persistence", () => {
  it("roundtrips a representative EngineState", () => {
    const state = makeState();
    expect(decodeState(encodeState(state))).toEqual(state);
  });

  it("has a stable fixed-width engine-major layout", () => {
    const state = makeState();
    const bytes = toBytes(encodeState(state));

    expect(bytes).toHaveLength(V15_BYTES);
    expect(bytes[0]).toBe(15);
    expect(bytes[1] | (bytes[2] << 8)).toBe(state.tempoBpm);
    expect(bytes[3]).toBe(state.stepCount);
    expect(bytes[V14_MIXER_OFFSET]).toBe(
      Math.round(state.tracks[0].volume * 255),
    );
    expect(bytes[V14_MIXER_OFFSET + 2]).toBe(
      Math.round(state.tracks[1].volume * 255),
    );
    expect(bytes[V14_MUTE_OFFSET]).toBe(0b0010010);
    expect(bytes[V14_VOICE_OFFSET]).toBe(
      Math.round(state.tracks[0].voiceParams[0] * 255),
    );
    expect(bytes[V14_VOICE_OFFSET + VOICE_PARAM_CAPACITY]).toBe(
      Math.round(state.tracks[1].voiceParams[0] * 255),
    );
    expect(bytes[V14_MODEL_OFFSET]).toBe(0b01);
    expect(bytes[V14_PHYSICAL_OFFSET]).toBe(
      Math.round(state.tracks[3].tom.physicalParams[0] * 255),
    );
    expect(bytes[V15_VELOCITY_OFFSET + 1]).toBe(
      Math.round(state.pattern[1] * 255),
    );
    expect(bytes[V15_PROBABILITY_OFFSET + 10]).toBe(255);
    expect(bytes[V15_CONDITION_OFFSET]).toBe(0x10);
    expect(bytes[V15_CONDITION_OFFSET + 3]).toBe(0x06);
    expect(
      Array.from(
        bytes.slice(V15_TRACK_LENGTH_OFFSET, V15_TRACK_LENGTH_OFFSET + 7),
      ),
    ).toEqual([16, 15, 14, 13, 12, 11, 10]);
    expect(bytes[V15_FLAGS_OFFSET]).toBe(1);
  });

  it("stores BPM and step count semantically and exactly", () => {
    for (const [tempoBpm, stepCount] of [
      [30, 1],
      [137, 7],
      [300, 16],
    ] as const) {
      const state = makeState();
      state.tempoBpm = tempoBpm;
      state.stepCount = stepCount;
      const decoded = decodeState(encodeState(state));
      expect(decoded?.tempoBpm).toBe(tempoBpm);
      expect(decoded?.stepCount).toBe(stepCount);
    }
  });

  it("roundtrips continuous cell velocities at byte precision", () => {
    const state = makeState();
    state.pattern.set([0.23, 0.51, 0.88]);

    const decoded = decodeState(encodeState(state))!;
    expect(decoded.pattern[0]).toBeCloseTo(q(0.23), 6);
    expect(decoded.pattern[1]).toBeCloseTo(q(0.51), 6);
    expect(decoded.pattern[2]).toBeCloseTo(q(0.88), 6);
  });

  it("quantizes, clamps, and sanitizes normalized fields", () => {
    const state = makeState();
    state.swing = Infinity;
    state.reverb = -1;
    state.probability = 2;
    state.humanize = NaN;
    state.tracks[0].voiceParams.set([NaN, -1, 2]);
    state.cellProbabilities.set([NaN, -1, 2]);
    state.cellConditions.set([255, 6]);
    state.trackLengths.set([0, 255]);

    const decoded = decodeState(encodeState(state));
    expect(decoded?.swing).toBe(0);
    expect(decoded?.reverb).toBe(0);
    expect(decoded?.probability).toBe(1);
    expect(decoded?.humanize).toBe(0);
    expect(Array.from(decoded!.tracks[0].voiceParams.slice(0, 3))).toEqual([
      0, 0, 1,
    ]);
    expect(Array.from(decoded!.cellProbabilities.slice(0, 3))).toEqual([
      0, 0, 1,
    ]);
    expect(Array.from(decoded!.cellConditions.slice(0, 2))).toEqual([0, 6]);
    expect(Array.from(decoded!.trackLengths.slice(0, 2))).toEqual([1, 16]);
  });

  it("emits URL-safe base64 and is a stable fixed point", () => {
    const encoded = encodeState(makeState());
    expect(encoded).toMatch(/^[A-Za-z0-9_-]+$/);
    expect(encodeState(decodeState(encoded)!)).toBe(encoded);
  });

  it("rejects corrupt reserved codes and invalid semantic fields", () => {
    const pattern = toBytes(encodeState(makeState()));
    pattern[V14_PATTERN_OFFSET] |= 0b11;
    expect(decodeState(toB64Url(pattern))).toBeNull();

    const tempo = toBytes(encodeState(makeState()));
    tempo[1] = 0;
    tempo[2] = 0;
    expect(decodeState(toB64Url(tempo))).toBeNull();

    const stepCount = toBytes(encodeState(makeState()));
    stepCount[3] = 0;
    expect(decodeState(toB64Url(stepCount))).toBeNull();

    const model = toBytes(encodeState(makeState()));
    model[V14_MODEL_OFFSET] = 4;
    expect(decodeState(toB64Url(model))).toBeNull();

    const condition = toBytes(encodeState(makeState()));
    condition[V15_CONDITION_OFFSET] = 0x70;
    expect(decodeState(toB64Url(condition))).toBeNull();

    const trackLength = toBytes(encodeState(makeState()));
    trackLength[V15_TRACK_LENGTH_OFFSET] = 0;
    expect(decodeState(toB64Url(trackLength))).toBeNull();

    const flags = toBytes(encodeState(makeState()));
    flags[V15_FLAGS_OFFSET] = 2;
    expect(decodeState(toB64Url(flags))).toBeNull();
  });

  it("keeps the complete v14 record as an immutable prefix", () => {
    const state = makeState();
    const v15 = toBytes(encodeState(state));
    const v14 = v15.slice(0, V14_BYTES);
    v14[0] = 14;

    const decoded = decodeState(toB64Url(v14))!;
    expect(decoded.tempoBpm).toBe(state.tempoBpm);
    expect(decoded.pattern).toEqual(
      Float32Array.from(state.pattern, (velocity) =>
        velocity >= VEL_ACCENT
          ? VEL_ACCENT
          : velocity > VEL_OFF
            ? VEL_NORMAL
            : VEL_OFF,
      ),
    );
    expect(decoded.tracks).toEqual(state.tracks);
    expect(Array.from(decoded.cellProbabilities)).toEqual(
      new Array(PATTERN_SIZE).fill(1),
    );
    expect(Array.from(decoded.cellConditions)).toEqual(
      new Array(PATTERN_SIZE).fill(0),
    );
    expect(Array.from(decoded.trackLengths)).toEqual(
      new Array(TRACK_COUNT).fill(state.stepCount),
    );
    expect(decoded.fillMode).toBe(false);
  });
});

describe("legacy v1-v13 migration", () => {
  it.each(Array.from({ length: 13 }, (_, index) => index + 1))(
    "decodes v%i into semantic EngineState coordinates",
    (version) => {
      const legacy = makeLegacyState();
      const decoded = decodeState(legacyBlob(legacy, version));
      expect(decoded).not.toBeNull();
      expect(decoded?.tempoBpm).toBe(Math.round(60 + legacy.tempo * 140));
      expect(decoded?.stepCount).toBe(Math.round(1 + legacy.steps * 15));
      expect(decoded?.swing).toBe(legacy.swing * 0.5);
      expect(decoded?.reverb).toBe(legacy.reverb);
      expect(decoded?.probability).toBe(legacy.prob);
      expect(decoded?.humanize).toBe(legacy.humanize);
      expect(Array.from(decoded!.cellProbabilities)).toEqual(
        new Array(PATTERN_SIZE).fill(1),
      );
      expect(Array.from(decoded!.cellConditions)).toEqual(
        new Array(PATTERN_SIZE).fill(0),
      );
      expect(Array.from(decoded!.trackLengths)).toEqual(
        new Array(TRACK_COUNT).fill(decoded!.stepCount),
      );
      expect(decoded!.fillMode).toBe(false);
    },
  );

  it("maps the historical visual mixer rows back to engine tracks", () => {
    const legacy = makeLegacyState();
    const decoded = decodeState(legacyBlob(legacy, 13))!;

    for (let track = 0; track < TRACK_COUNT; track++) {
      const visual = ENGINE_TO_VISUAL[track];
      expect(decoded.tracks[track].volume).toBe(legacy.volumes[visual]);
      expect(decoded.tracks[track].decay).toBe(legacy.decays[visual]);
      expect(decoded.tracks[track].muted).toBe(legacy.muted[visual]);
    }
  });

  it("fills state absent from early versions with canonical defaults", () => {
    const legacy = makeLegacyState();
    const defaults = createDefaultEngineState();
    const v1 = decodeState(legacyBlob(legacy, 1))!;

    expect(Array.from(v1.pattern.slice(LEGACY_PATTERN_SIZE))).toEqual(
      new Array(PATTERN_SIZE - LEGACY_PATTERN_SIZE).fill(VEL_OFF),
    );
    for (const track of [5, 6]) {
      expect(v1.tracks[track]).toEqual(defaults.tracks[track]);
    }
    for (let track = 0; track < TRACK_COUNT; track++) {
      expect(v1.tracks[track].voiceParams).toEqual(
        defaults.tracks[track].voiceParams,
      );
    }
    expect(v1.tracks[3].tom.model).toBe("procedural");
    expect(v1.tracks[5].tom.model).toBe("procedural");
  });

  it("restores engine-major voice rows and both Tom banks", () => {
    const legacy = makeLegacyState();
    const decoded = decodeState(legacyBlob(legacy, 13))!;

    for (let track = 0; track < TRACK_COUNT; track++) {
      const length = decoded.tracks[track].voiceParams.length;
      expect(Array.from(decoded.tracks[track].voiceParams)).toEqual(
        legacy.voiceParams[track].slice(0, length).map(Math.fround),
      );
    }
    expect(decoded.tracks[3].tom.model).toBe("physical");
    expect(decoded.tracks[5].tom.model).toBe("physical");
    expect(Array.from(decoded.tracks[3].tom.physicalParams)).toEqual(
      legacy.physicalTomParams.map(Math.fround),
    );
    expect(Array.from(decoded.tracks[5].tom.physicalParams)).toEqual(
      legacy.physicalTom2Params.map(Math.fround),
    );
  });

  it("preserves the two historical strike-radius migrations", () => {
    const legacy = makeLegacyState();
    legacy.physicalTomParams[4] = q(0.45 / 0.95);
    expect(
      decodeState(legacyBlob(legacy, 5))?.tracks[3].tom.physicalParams[4],
    ).toBe(Math.fround(PHYSICAL_TOM_PARAMS[4].default));

    legacy.physicalTomParams[4] = q(0.12 / 0.95);
    legacy.physicalTom2Params[4] = q(0.12 / 0.95);
    const v10 = decodeState(legacyBlob(legacy, 10))!;
    expect(v10.tracks[3].tom.physicalParams[4]).toBe(
      Math.fround(PHYSICAL_TOM_PARAMS[4].default),
    );
    expect(v10.tracks[5].tom.physicalParams[4]).toBe(
      Math.fround(PHYSICAL_TOM_PARAMS[4].default),
    );
  });

  it("rescales v12 attack level in both independent banks", () => {
    const legacy = makeLegacyState();
    const edited = q(0.4);
    legacy.physicalTomParams[16] = edited;
    legacy.physicalTom2Params[16] = edited;
    const decoded = decodeState(legacyBlob(legacy, 12))!;

    expect(decoded.tracks[3].tom.physicalParams[16]).toBeCloseTo(edited * 2, 6);
    expect(decoded.tracks[5].tom.physicalParams[16]).toBeCloseTo(edited * 2, 6);
  });

  it("upgrades legacy blobs to v15 on re-encode", () => {
    const decoded = decodeState(legacyBlob(makeLegacyState(), 1))!;
    const bytes = toBytes(encodeState(decoded));
    expect(bytes[0]).toBe(15);
    expect(bytes).toHaveLength(V15_BYTES);
  });
});

describe("invalid input", () => {
  it("rejects garbage, wrong lengths, and unknown versions", () => {
    expect(decodeState("@@@not-valid@@@")).toBeNull();
    expect(decodeState("")).toBeNull();
    expect(decodeState("AAAA")).toBeNull();

    const future = toBytes(encodeState(makeState()));
    future[0] = 99;
    expect(decodeState(toB64Url(future))).toBeNull();

    const truncatedV15 = toBytes(encodeState(makeState())).slice(0, -1);
    expect(decodeState(toB64Url(truncatedV15))).toBeNull();

    const extendedV15 = new Uint8Array(V15_BYTES + 1);
    extendedV15.set(toBytes(encodeState(makeState())));
    expect(decodeState(toB64Url(extendedV15))).toBeNull();

    const truncatedV14 = toBytes(encodeState(makeState())).slice(
      0,
      V14_BYTES - 1,
    );
    truncatedV14[0] = 14;
    expect(decodeState(toB64Url(truncatedV14))).toBeNull();
  });
});

describe("share URLs", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("builds a URL purely from state and an explicit location", () => {
    const state = makeState();
    const currentHref =
      "https://example.test/algo-drum/?theme=dark#previous-state";

    const first = buildShareUrl(state, currentHref);
    const second = buildShareUrl(state, currentHref);

    expect(first).toBe(second);
    expect(first).toBe(
      `https://example.test/algo-drum/?theme=dark#${encodeState(state)}`,
    );
    expect(currentHref).toBe(
      "https://example.test/algo-drum/?theme=dark#previous-state",
    );
  });

  it("gets the current-page share URL without touching history", () => {
    const replaceState = vi.fn();
    vi.stubGlobal("window", {
      location: { href: "https://example.test/drums?preset=house#old" },
      history: { replaceState },
    });

    expect(shareUrl(makeState())).toMatch(
      /^https:\/\/example\.test\/drums\?preset=house#[A-Za-z0-9_-]+$/,
    );
    expect(replaceState).not.toHaveBeenCalled();
  });

  it("mutates history only through the explicitly named operation", () => {
    const replaceState = vi.fn();
    const historyState = { route: "drums", scroll: 42 };
    vi.stubGlobal("window", {
      location: { href: "https://example.test/drums" },
      history: { state: historyState, replaceState },
    });

    const state = makeState();
    const url = replaceAddressBarWithShareUrl(state);

    expect(url).toBe(`https://example.test/drums#${encodeState(state)}`);
    expect(replaceState).toHaveBeenCalledOnce();
    expect(replaceState).toHaveBeenCalledWith(historyState, "", url);
  });

  it("returns the share URL when the address-bar mutation is unavailable", () => {
    const replaceState = vi.fn(() => {
      throw new DOMException("History access denied", "SecurityError");
    });
    vi.stubGlobal("window", {
      location: { href: "https://example.test/drums" },
      history: { state: { route: "drums" }, replaceState },
    });

    const state = makeState();
    expect(replaceAddressBarWithShareUrl(state)).toBe(
      `https://example.test/drums#${encodeState(state)}`,
    );
    expect(replaceState).toHaveBeenCalledOnce();
  });
});
