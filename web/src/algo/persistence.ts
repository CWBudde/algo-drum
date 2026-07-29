// Persistence + shareable links.
//
// The full user state (pattern + parameters) is serialized into a compact,
// versioned byte blob and base64url-encoded. The same blob backs both
// localStorage (auto-save/restore) and the URL hash (SHARE links). Encoding is
// pure and testable; the localStorage/hash glue below is a thin, fail-soft
// wrapper around it.

import {
  PHYSICAL_TOM_PARAM_CAPACITY,
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

// Bump when the byte layout changes. Every version is a strict append, so old
// offsets and meanings stay fixed: v2 added voice parameters and v3 adds the
// Tom model selector and v4 adds the physical Tom parameter bank. Older links
// still decode with their new fields unset.
const FORMAT_VERSION = 4;

// Byte layout: version, 6 scalar knobs, 5 volumes, 5 decays, 1 mute mask,
// then the 80-cell pattern packed 2 bits per cell (20 bytes)...
const HEADER_BYTES = 1 + 6 + 5 + 5 + 1;
const PATTERN_BYTES = PATTERN_SIZE / 4;
const V1_BYTES = HEADER_BYTES + PATTERN_BYTES;

// ...then, in v2, the per-voice synthesis parameters: one byte per slot,
// engine-major, at V1_BYTES + track*VOICE_PARAM_CAPACITY + index. Voices with
// fewer parameters than the capacity leave their trailing slots at 0.
const VOICE_PARAM_BYTES = TRACK_COUNT * VOICE_PARAM_CAPACITY;
const V2_BYTES = V1_BYTES + VOICE_PARAM_BYTES;

// v3 appends one byte for the explicitly selected Tom implementation.
const V3_BYTES = V2_BYTES + 1;

// v4 appends the independent physical-model parameter bank.
const TOTAL_BYTES = V3_BYTES + PHYSICAL_TOM_PARAM_CAPACITY;

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
  volumes: number[]; // length 5
  decays: number[]; // length 5
  muted: boolean[]; // length 5
  // Per-voice synthesis parameters, engine-major, TRACK_COUNT rows of
  // VOICE_PARAM_CAPACITY normalized positions. Absent from v1 blobs — callers
  // fall back to the per-voice defaults in engine/voiceParams.ts.
  voiceParams?: number[][];
  // Absent from v1/v2 blobs, which always used the procedural Tom.
  tomModel?: TomModel;
  // Absent from v1/v2/v3 blobs. The call site supplies generated defaults.
  physicalTomParams?: number[];
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

  for (let i = 0; i < 5; i++) bytes[offset++] = toByte(state.volumes[i] ?? 0);
  for (let i = 0; i < 5; i++) bytes[offset++] = toByte(state.decays[i] ?? 0);

  let muteMask = 0;
  for (let i = 0; i < 5; i++) if (state.muted[i]) muteMask |= 1 << i;
  bytes[offset++] = muteMask;

  // Pack four 2-bit cell codes into each pattern byte.
  for (let i = 0; i < PATTERN_BYTES; i++) {
    let packed = 0;
    for (let j = 0; j < 4; j++) {
      const code = velToCode(state.pattern[i * 4 + j] ?? VEL_OFF);
      packed |= code << (j * 2);
    }
    bytes[offset++] = packed;
  }

  // Rows are padded and truncated to the capacity so the record stays fixed
  // width no matter how many parameters a voice actually exposes.
  for (let track = 0; track < TRACK_COUNT; track++) {
    for (let i = 0; i < VOICE_PARAM_CAPACITY; i++) {
      bytes[offset++] = toByte(state.voiceParams?.[track]?.[i] ?? 0);
    }
  }

  bytes[offset++] = state.tomModel === "physical" ? 1 : 0;

  for (let i = 0; i < PHYSICAL_TOM_PARAM_CAPACITY; i++) {
    bytes[offset++] = toByte(state.physicalTomParams?.[i] ?? 0);
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

  const volumes: number[] = [];
  for (let i = 0; i < 5; i++) volumes.push(fromByte(bytes[offset++]));

  const decays: number[] = [];
  for (let i = 0; i < 5; i++) decays.push(fromByte(bytes[offset++]));

  const muteMask = bytes[offset++];
  const muted: boolean[] = [];
  for (let i = 0; i < 5; i++) muted.push((muteMask & (1 << i)) !== 0);

  const pattern = new Array<number>(PATTERN_SIZE).fill(VEL_OFF);
  for (let i = 0; i < PATTERN_BYTES; i++) {
    const packed = bytes[offset++];
    for (let j = 0; j < 4; j++) {
      pattern[i * 4 + j] = codeToVel((packed >> (j * 2)) & 0b11);
    }
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
  for (let track = 0; track < TRACK_COUNT; track++) {
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
  for (let i = 0; i < PHYSICAL_TOM_PARAM_CAPACITY; i++) {
    physicalTomParams.push(fromByte(bytes[offset++]));
  }

  return { ...stateWithModel, physicalTomParams };
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
