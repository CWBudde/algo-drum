// Persistence + shareable links.
//
// The full user state (pattern + parameters) is serialized into a compact,
// versioned byte blob and base64url-encoded. The same blob backs both
// localStorage (auto-save/restore) and the URL hash (SHARE links). Encoding is
// pure and testable; the localStorage/hash glue below is a thin, fail-soft
// wrapper around it.

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
import type { TomModel } from "../engine/tomModel";

// Bump when the byte layout or its UI interpretation changes. Layout changes
// through v7 are strict appends, so old offsets and meanings stay fixed: v2
// added voice parameters, v3 the Tom model selector, v4 the original 13-slot
// physical Tom bank, v5 the two tension-asymmetry controls, v6 the corrected
// central-hit default, and v7 the second Tom and Percussion tracks. V8 keeps
// the same bytes but migrates the reordered mixer strips; v9 appends Tom 2's
// physical model choice and independent parameter bank. V10 widens both
// physical banks by one slot for the damping tilt. V11 keeps v10's bytes and
// migrates the former shipped strike-radius position again, this time off the
// near-centre 0.12 detent. V12 widens both physical banks by two more slots for
// the attack layer's level and tone. V13 keeps v12's bytes and rescales the
// attack level, whose range narrowed from 0–0.3 to 0–0.15 once the layer became
// three bands instead of one. Older links still decode with values attached to
// the same voices.
const FORMAT_VERSION = 13;

// Byte layout: version, 6 scalar knobs, 5 volumes, 5 decays, 1 mute mask,
// then the 80-cell pattern packed 2 bits per cell (20 bytes)...
const LEGACY_TRACK_COUNT = 5;
const LEGACY_PATTERN_SIZE = LEGACY_TRACK_COUNT * 16;
const LEGACY_HEADER_BYTES = 1 + 6 + 5 + 5 + 1;
const LEGACY_PATTERN_BYTES = LEGACY_PATTERN_SIZE / 4;
const V1_BYTES = LEGACY_HEADER_BYTES + LEGACY_PATTERN_BYTES;

// ...then, in v2, the per-voice synthesis parameters: one byte per slot,
// engine-major, at V1_BYTES + track*VOICE_PARAM_CAPACITY + index. Voices with
// fewer parameters than the capacity leave their trailing slots at 0.
const LEGACY_VOICE_PARAM_BYTES = LEGACY_TRACK_COUNT * VOICE_PARAM_CAPACITY;
const V2_BYTES = V1_BYTES + LEGACY_VOICE_PARAM_BYTES;

// v3 appends one byte for the explicitly selected Tom implementation.
const V3_BYTES = V2_BYTES + 1;

// v4 appended the original independent physical-model parameter bank. Keep
// its width explicit: the generated capacity grows append-only, while old v4
// links must continue to decode at their original length.
const V4_PHYSICAL_TOM_PARAM_CAPACITY = 13;
const V4_BYTES = V3_BYTES + V4_PHYSICAL_TOM_PARAM_CAPACITY;

// v5 extends that bank with the P6 asymmetry amount and principal axis. v6 has
// the same width and migrates only the former shipped strike-radius position.
// Pinned for the same reason as v4's width: the generated capacity grows
// append-only, while a v5–v9 link must keep decoding at the length it was
// written with.
const V5_PHYSICAL_TOM_PARAM_CAPACITY = 15;
const V6_BYTES = V3_BYTES + V5_PHYSICAL_TOM_PARAM_CAPACITY;

// v7 appends all state belonging to engine tracks 5 and 6. The original five
// tracks keep byte-for-byte v1–v6 offsets, so existing links remain decodable.
const EXTRA_TRACK_COUNT = TRACK_COUNT - LEGACY_TRACK_COUNT;
const EXTRA_PATTERN_BYTES = (PATTERN_SIZE - LEGACY_PATTERN_SIZE) / 4;
const V7_EXTRA_BYTES =
  EXTRA_TRACK_COUNT * 2 + // volumes and decays
  1 + // mute mask
  EXTRA_PATTERN_BYTES +
  EXTRA_TRACK_COUNT * VOICE_PARAM_CAPACITY;
const V8_BYTES = V6_BYTES + V7_EXTRA_BYTES;

// v9 gives Tom 2 the same independently persisted physical-model controls.
const V9_BYTES = V8_BYTES + 1 + V5_PHYSICAL_TOM_PARAM_CAPACITY;

// v10 adds the damping tilt to both physical banks. The Tom 1 bank sits in the
// middle of the record, so widening it moves every offset after it — hence a
// version bump rather than an append. Pinned like v4's and v5's widths: v10 and
// v11 links must keep decoding at 16 even after the generated capacity grows.
const V10_PHYSICAL_TOM_PARAM_CAPACITY = 16;
const V10_BYTES =
  V3_BYTES +
  V10_PHYSICAL_TOM_PARAM_CAPACITY +
  V7_EXTRA_BYTES +
  1 +
  V10_PHYSICAL_TOM_PARAM_CAPACITY;

// v11 changed no bytes, only the meaning of one stored position, so it shares
// v10's length. v12 widens both banks again, for the attack layer, and v13 in
// turn changes only the interpretation of one of v12's slots, so those two share
// a length the same way.
const V11_BYTES = V10_BYTES;
const V12_PHYSICAL_TOM_PARAM_CAPACITY = 18;
const V12_BYTES =
  V3_BYTES +
  V12_PHYSICAL_TOM_PARAM_CAPACITY +
  V7_EXTRA_BYTES +
  1 +
  V12_PHYSICAL_TOM_PARAM_CAPACITY;
const TOTAL_BYTES =
  V3_BYTES +
  PHYSICAL_TOM_PARAM_CAPACITY +
  V7_EXTRA_BYTES +
  1 +
  PHYSICAL_TOM_PARAM_CAPACITY;

// migrateStrikeRadius moves the *exact* shipped strike-radius detent onto the
// current default, twice over, and leaves every edited position alone.
//
// The two rules stay separately gated. v4/v5 shipped a peripheral 45% hit; v6
// corrected that to a near-central 0.12, which turned out to be the other
// extreme — a centre hit excites the axisymmetric modes and almost nothing else
// — and v11 moves it to 0.30. Merging the gates would overwrite a v6–v10 blob
// that a user had deliberately dragged back to 0.45.
//
// One accepted false positive: a v4/v5 blob edited to exactly 0.1255 is treated
// as the shipped detent. There is no way to tell those apart from one byte.
function migrateStrikeRadius(version: number, bank: number[]): void {
  const shipped = PHYSICAL_TOM_PARAMS[PHYSICAL_STRIKE_RADIUS_INDEX].default;
  if (
    version < 6 &&
    bank[PHYSICAL_STRIKE_RADIUS_INDEX] === OLD_PHYSICAL_STRIKE_RADIUS_DEFAULT
  ) {
    bank[PHYSICAL_STRIKE_RADIUS_INDEX] = shipped;
  }
  if (
    version < 11 &&
    bank[PHYSICAL_STRIKE_RADIUS_INDEX] ===
      CENTRAL_PHYSICAL_STRIKE_RADIUS_DEFAULT
  ) {
    bank[PHYSICAL_STRIKE_RADIUS_INDEX] = shipped;
  }
}

// physicalBankWidth is a table rather than a "below the current version" test on
// purpose. The old form read every earlier version at the v5 width, which is
// correct only until a bump makes the previous version wider than that — and then
// it misreads the bank and desynchronizes the offset of everything after it: the
// extra tracks, the mute mask, the pattern and the whole Tom 2 record.
function physicalBankWidth(version: number): number {
  if (version === 4) return V4_PHYSICAL_TOM_PARAM_CAPACITY;
  if (version <= 9) return V5_PHYSICAL_TOM_PARAM_CAPACITY;
  if (version <= 11) return V10_PHYSICAL_TOM_PARAM_CAPACITY;
  if (version <= 13) return V12_PHYSICAL_TOM_PARAM_CAPACITY;

  return PHYSICAL_TOM_PARAM_CAPACITY;
}

// migrateAttackLevel rescales the stored attack level onto its narrowed range.
//
// v12 mapped the slot linearly onto 0–0.3 with 0.1 shipped. The layer became
// three summed bands in v13, which is about three times as loud for the same
// number, so the useful range narrowed to 0–0.15 with 0.05 shipped. A stored
// position therefore has to double to keep meaning the same level.
//
// The untouched detent is handled first and separately, exactly as
// migrateStrikeRadius does: a bank still sitting on v12's default gets v13's
// default rather than v12's level, because the refit *is* the new default.
function migrateAttackLevel(version: number, bank: number[]): void {
  if (version >= 13 || bank.length <= PHYSICAL_ATTACK_LEVEL_INDEX) return;

  const stored = bank[PHYSICAL_ATTACK_LEVEL_INDEX];
  if (stored === OLD_PHYSICAL_ATTACK_LEVEL_DEFAULT) {
    bank[PHYSICAL_ATTACK_LEVEL_INDEX] =
      PHYSICAL_TOM_PARAMS[PHYSICAL_ATTACK_LEVEL_INDEX].default;

    return;
  }

  bank[PHYSICAL_ATTACK_LEVEL_INDEX] = Math.min(1, stored * 2);
}

const PHYSICAL_ATTACK_LEVEL_INDEX = 16;
// v12's shipped position: 0.1 on a 0–0.3 linear range, as one byte.
const OLD_PHYSICAL_ATTACK_LEVEL_DEFAULT = Math.round((0.1 / 0.3) * 255) / 255;

const PHYSICAL_STRIKE_RADIUS_INDEX = 4;
const OLD_PHYSICAL_STRIKE_RADIUS_DEFAULT =
  Math.round((0.45 / 0.95) * 255) / 255;
// The v6–v10 shipped detent, corrected again by v11. Encoded exactly as
// toByte(0.12 / 0.95) so only the untouched default matches it.
const CENTRAL_PHYSICAL_STRIKE_RADIUS_DEFAULT =
  Math.round((0.12 / 0.95) * 255) / 255;

// Names the storage slot, not the blob version — it never tracked
// FORMAT_VERSION, and the decoder is version-tolerant. Bumping it would orphan
// every pattern users have saved, which is exactly what v2 avoids.
export const STORAGE_KEY = "algo-drum.state.v1";

// PersistedState mirrors the DrumMachine's serializable UI state. Scalar knob
// values and per-track volumes/decays are normalized positions in [0, 1].
export interface PersistedState {
  pattern: number[]; // flat, engine-major velocities, length PATTERN_SIZE
  steps: number;
  tempo: number;
  swing: number;
  reverb: number;
  prob: number;
  humanize: number;
  volumes: number[]; // visual row order, length TRACK_COUNT
  decays: number[]; // visual row order, length TRACK_COUNT
  muted: boolean[]; // visual row order, length TRACK_COUNT
  // Per-voice synthesis parameters, engine-major, TRACK_COUNT rows of
  // VOICE_PARAM_CAPACITY normalized positions. Absent from v1 blobs — callers
  // fall back to the per-voice defaults in engine/voiceParams.ts.
  voiceParams?: number[][];
  // Absent from v1/v2 blobs, which always used the procedural Tom.
  tomModel?: TomModel;
  // Absent from v1/v2/v3 blobs. The call site supplies generated defaults.
  physicalTomParams?: number[];
  // Absent before v9; Tom 2 used its procedural model and fresh defaults.
  tom2Model?: TomModel;
  physicalTom2Params?: number[];
}

function clamp01(v: number): number {
  if (!Number.isFinite(v)) return 0;
  return v < 0 ? 0 : v > 1 ? 1 : v;
}

function toByte(v: number): number {
  return Math.round(clamp01(v) * 255);
}

function fromByte(b: number): number {
  return b / 255;
}

function velToCode(v: number): number {
  if (v >= VEL_ACCENT) return 2;
  return v > VEL_OFF ? 1 : 0;
}

function codeToVel(code: number): number {
  if (code === 2) return VEL_ACCENT;
  return code === 1 ? VEL_NORMAL : VEL_OFF;
}

// Persisted mixer values are split between the original five visual rows
// (Cymbal, Tom, HiHat, Snare, Bass) and the two appended engine tracks. These
// mappings keep that byte layout independent from later UI reordering.
const LEGACY_VISUAL_TO_CURRENT = [0, 3, 4, 5, 6] as const;
const ENGINE_TO_VISUAL = [6, 5, 4, 3, 0, 2, 1] as const;

function visualIndexForEngineTrack(track: number): number {
  return ENGINE_TO_VISUAL[track] ?? -1;
}

function bytesToBase64Url(bytes: Uint8Array): string {
  let binary = "";
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return btoa(binary)
    .replace(/\+/g, "-")
    .replace(/\//g, "_")
    .replace(/=+$/, "");
}

function base64UrlToBytes(text: string): Uint8Array | null {
  try {
    const base64 = text.replace(/-/g, "+").replace(/_/g, "/");
    // Restore the "=" padding stripped by encoding — not every atob
    // implementation accepts unpadded input.
    const padded = base64.padEnd(Math.ceil(base64.length / 4) * 4, "=");
    const binary = atob(padded);
    const bytes = new Uint8Array(binary.length);
    for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
    return bytes;
  } catch {
    return null;
  }
}

// encodeState serializes state into a compact base64url string.
export function encodeState(state: PersistedState): string {
  const bytes = new Uint8Array(TOTAL_BYTES);
  let offset = 0;

  bytes[offset++] = FORMAT_VERSION;
  bytes[offset++] = toByte(state.steps);
  bytes[offset++] = toByte(state.tempo);
  bytes[offset++] = toByte(state.swing);
  bytes[offset++] = toByte(state.reverb);
  bytes[offset++] = toByte(state.prob);
  bytes[offset++] = toByte(state.humanize);

  for (const visualIndex of LEGACY_VISUAL_TO_CURRENT)
    bytes[offset++] = toByte(state.volumes[visualIndex] ?? 0);
  for (const visualIndex of LEGACY_VISUAL_TO_CURRENT)
    bytes[offset++] = toByte(state.decays[visualIndex] ?? 0);

  let muteMask = 0;
  for (let i = 0; i < LEGACY_TRACK_COUNT; i++) {
    if (state.muted[LEGACY_VISUAL_TO_CURRENT[i]]) muteMask |= 1 << i;
  }
  bytes[offset++] = muteMask;

  // Pack four 2-bit cell codes into each pattern byte.
  for (let i = 0; i < LEGACY_PATTERN_BYTES; i++) {
    let packed = 0;
    for (let j = 0; j < 4; j++) {
      const code = velToCode(state.pattern[i * 4 + j] ?? VEL_OFF);
      packed |= code << (j * 2);
    }
    bytes[offset++] = packed;
  }

  // Rows are padded and truncated to the capacity so the record stays fixed
  // width no matter how many parameters a voice actually exposes.
  for (let track = 0; track < LEGACY_TRACK_COUNT; track++) {
    for (let i = 0; i < VOICE_PARAM_CAPACITY; i++) {
      bytes[offset++] = toByte(state.voiceParams?.[track]?.[i] ?? 0);
    }
  }

  bytes[offset++] = state.tomModel === "physical" ? 1 : 0;

  for (let i = 0; i < PHYSICAL_TOM_PARAM_CAPACITY; i++) {
    bytes[offset++] = toByte(state.physicalTomParams?.[i] ?? 0);
  }

  for (let track = LEGACY_TRACK_COUNT; track < TRACK_COUNT; track++) {
    bytes[offset++] = toByte(
      state.volumes[visualIndexForEngineTrack(track)] ?? 0,
    );
  }
  for (let track = LEGACY_TRACK_COUNT; track < TRACK_COUNT; track++) {
    bytes[offset++] = toByte(
      state.decays[visualIndexForEngineTrack(track)] ?? 0,
    );
  }

  let extraMuteMask = 0;
  for (let track = LEGACY_TRACK_COUNT; track < TRACK_COUNT; track++) {
    if (state.muted[visualIndexForEngineTrack(track)])
      extraMuteMask |= 1 << (track - LEGACY_TRACK_COUNT);
  }
  bytes[offset++] = extraMuteMask;

  for (let i = 0; i < EXTRA_PATTERN_BYTES; i++) {
    let packed = 0;
    for (let j = 0; j < 4; j++) {
      const patternIndex = LEGACY_PATTERN_SIZE + i * 4 + j;
      packed |= velToCode(state.pattern[patternIndex] ?? VEL_OFF) << (j * 2);
    }
    bytes[offset++] = packed;
  }

  for (let track = LEGACY_TRACK_COUNT; track < TRACK_COUNT; track++) {
    for (let i = 0; i < VOICE_PARAM_CAPACITY; i++) {
      bytes[offset++] = toByte(state.voiceParams?.[track]?.[i] ?? 0);
    }
  }

  bytes[offset++] = state.tom2Model === "physical" ? 1 : 0;
  for (let i = 0; i < PHYSICAL_TOM_PARAM_CAPACITY; i++) {
    bytes[offset++] = toByte(state.physicalTom2Params?.[i] ?? 0);
  }

  return bytesToBase64Url(bytes);
}

// decodeState parses a base64url string back into state, returning null on any
// version/length/garbage mismatch so callers can fall back to a fresh start.
export function decodeState(text: string): PersistedState | null {
  const bytes = base64UrlToBytes(text);
  if (!bytes || bytes.length === 0) return null;

  // The expected length is version-specific: a v1 blob is not a truncated v2
  // blob, and a full-length blob claiming v1 is corrupt rather than v1 with
  // junk appended.
  const version = bytes[0];
  const expected =
    version === 1
      ? V1_BYTES
      : version === 2
        ? V2_BYTES
        : version === 3
          ? V3_BYTES
          : version === 4
            ? V4_BYTES
            : version === 5 || version === 6
              ? V6_BYTES
              : version === 7 || version === 8
                ? V8_BYTES
                : version === 9
                  ? V9_BYTES
                  : version === 10
                    ? V10_BYTES
                    : version === 11
                      ? V11_BYTES
                      : version === 12
                        ? V12_BYTES
                        : version === FORMAT_VERSION
                          ? TOTAL_BYTES
                          : -1;
  if (bytes.length !== expected) return null;

  let offset = 1;

  const steps = fromByte(bytes[offset++]);
  const tempo = fromByte(bytes[offset++]);
  const swing = fromByte(bytes[offset++]);
  const reverb = fromByte(bytes[offset++]);
  const prob = fromByte(bytes[offset++]);
  const humanize = fromByte(bytes[offset++]);

  const legacyVolumes: number[] = [];
  for (let i = 0; i < LEGACY_TRACK_COUNT; i++)
    legacyVolumes.push(fromByte(bytes[offset++]));

  const legacyDecays: number[] = [];
  for (let i = 0; i < LEGACY_TRACK_COUNT; i++)
    legacyDecays.push(fromByte(bytes[offset++]));

  const muteMask = bytes[offset++];
  const legacyMuted: boolean[] = [];
  for (let i = 0; i < LEGACY_TRACK_COUNT; i++)
    legacyMuted.push((muteMask & (1 << i)) !== 0);

  const pattern = new Array<number>(PATTERN_SIZE).fill(VEL_OFF);
  for (let i = 0; i < LEGACY_PATTERN_BYTES; i++) {
    const packed = bytes[offset++];
    for (let j = 0; j < 4; j++) {
      pattern[i * 4 + j] = codeToVel((packed >> (j * 2)) & 0b11);
    }
  }

  const volumes = new Array<number>(TRACK_COUNT).fill(0.75);
  const decays = new Array<number>(TRACK_COUNT).fill(0.5);
  const muted = new Array<boolean>(TRACK_COUNT).fill(false);
  for (let i = 0; i < LEGACY_TRACK_COUNT; i++) {
    const visualIndex = LEGACY_VISUAL_TO_CURRENT[i];
    volumes[visualIndex] = legacyVolumes[i];
    decays[visualIndex] = legacyDecays[i];
    muted[visualIndex] = legacyMuted[i];
  }

  const state: PersistedState = {
    pattern,
    steps,
    tempo,
    swing,
    reverb,
    prob,
    humanize,
    volumes,
    decays,
    muted,
  };

  if (version === 1) return state;

  const voiceParams: number[][] = [];
  for (let track = 0; track < LEGACY_TRACK_COUNT; track++) {
    const row: number[] = [];
    for (let i = 0; i < VOICE_PARAM_CAPACITY; i++)
      row.push(fromByte(bytes[offset++]));
    voiceParams.push(row);
  }

  if (version === 2) return { ...state, voiceParams };

  const tomModelCode = bytes[offset++];
  if (tomModelCode > 1) return null;

  const stateWithModel: PersistedState = {
    ...state,
    voiceParams,
    tomModel: tomModelCode === 1 ? "physical" : "procedural",
  };

  if (version === 3) return stateWithModel;

  const physicalTomParams: number[] = [];
  const physicalParamCount = physicalBankWidth(version);
  for (let i = 0; i < physicalParamCount; i++) {
    physicalTomParams.push(fromByte(bytes[offset++]));
  }

  migrateStrikeRadius(version, physicalTomParams);
  migrateAttackLevel(version, physicalTomParams);

  const stateWithPhysical = { ...stateWithModel, physicalTomParams };
  if (version < 7) return stateWithPhysical;

  for (let track = LEGACY_TRACK_COUNT; track < TRACK_COUNT; track++) {
    stateWithPhysical.volumes[visualIndexForEngineTrack(track)] = fromByte(
      bytes[offset++],
    );
  }
  for (let track = LEGACY_TRACK_COUNT; track < TRACK_COUNT; track++) {
    stateWithPhysical.decays[visualIndexForEngineTrack(track)] = fromByte(
      bytes[offset++],
    );
  }

  const extraMuteMask = bytes[offset++];
  for (let track = LEGACY_TRACK_COUNT; track < TRACK_COUNT; track++) {
    stateWithPhysical.muted[visualIndexForEngineTrack(track)] =
      (extraMuteMask & (1 << (track - LEGACY_TRACK_COUNT))) !== 0;
  }

  for (let i = 0; i < EXTRA_PATTERN_BYTES; i++) {
    const packed = bytes[offset++];
    for (let j = 0; j < 4; j++) {
      pattern[LEGACY_PATTERN_SIZE + i * 4 + j] = codeToVel(
        (packed >> (j * 2)) & 0b11,
      );
    }
  }

  for (let track = LEGACY_TRACK_COUNT; track < TRACK_COUNT; track++) {
    const row: number[] = [];
    for (let i = 0; i < VOICE_PARAM_CAPACITY; i++)
      row.push(fromByte(bytes[offset++]));
    voiceParams.push(row);
  }

  if (version < 9) return stateWithPhysical;

  const tom2ModelCode = bytes[offset++];
  if (tom2ModelCode > 1) return null;

  // Both banks were widened together, so they share physicalParamCount.
  const physicalTom2Params: number[] = [];
  for (let i = 0; i < physicalParamCount; i++) {
    physicalTom2Params.push(fromByte(bytes[offset++]));
  }

  // Tom 2's bank needs the same correction. The v6 rule above deliberately did
  // not touch it, which was right at the time — this bank did not exist before
  // v9, so it could never hold a pre-v6 detent — but v9 and v10 blobs do carry
  // the 0.12 one, and leaving them alone would strand Tom 2 on a centre hit.
  migrateStrikeRadius(version, physicalTom2Params);
  migrateAttackLevel(version, physicalTom2Params);

  return {
    ...stateWithPhysical,
    tom2Model: tom2ModelCode === 1 ? "physical" : "procedural",
    physicalTom2Params,
  };
}

// ── localStorage + URL hash glue (fail-soft) ────────────────────────────────

// saveLocal persists state to localStorage; storage errors are swallowed.
export function saveLocal(state: PersistedState): void {
  try {
    localStorage.setItem(STORAGE_KEY, encodeState(state));
  } catch {
    // Ignore quota / disabled-storage errors — persistence is best-effort.
  }
}

// loadLocal restores state from localStorage, or null if absent/invalid.
export function loadLocal(): PersistedState | null {
  try {
    const text = localStorage.getItem(STORAGE_KEY);
    return text ? decodeState(text) : null;
  } catch {
    return null;
  }
}

// readHash decodes state from the current URL hash (if present and valid).
export function readHash(): PersistedState | null {
  const hash = window.location.hash.replace(/^#/, "");
  return hash ? decodeState(hash) : null;
}

// shareUrl encodes state into the URL hash, updates the address bar without a
// navigation, and returns the full shareable link.
export function shareUrl(state: PersistedState): string {
  const encoded = encodeState(state);
  const url = `${window.location.origin}${window.location.pathname}${window.location.search}#${encoded}`;
  window.history.replaceState(null, "", url);
  return url;
}

// loadInitialState prefers a valid URL hash over localStorage, so shared links
// always win.
export function loadInitialState(): PersistedState | null {
  return readHash() ?? loadLocal();
}
