// Persistence + shareable links.
//
// The full user state (pattern + parameters) is serialized into a compact,
// versioned byte blob and base64url-encoded. The same blob backs both
// localStorage (auto-save/restore) and the URL hash (SHARE links). Encoding is
// pure and testable; the localStorage/hash glue below is a thin, fail-soft
// wrapper around it.

import { PATTERN_SIZE, VEL_ACCENT, VEL_NORMAL, VEL_OFF } from "./pattern";

// Bump when the byte layout changes; older/garbage blobs then fail to decode
// and the app starts fresh instead of loading corrupt state.
const FORMAT_VERSION = 1;

// Byte layout: version, 6 scalar knobs, 5 volumes, 5 decays, 1 mute mask,
// then the 80-cell pattern packed 2 bits per cell (20 bytes).
const HEADER_BYTES = 1 + 6 + 5 + 5 + 1;
const PATTERN_BYTES = PATTERN_SIZE / 4;
const TOTAL_BYTES = HEADER_BYTES + PATTERN_BYTES;

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
    const padded = text.replace(/-/g, "+").replace(/_/g, "/");
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

  return bytesToBase64Url(bytes);
}

// decodeState parses a base64url string back into state, returning null on any
// version/length/garbage mismatch so callers can fall back to a fresh start.
export function decodeState(text: string): PersistedState | null {
  const bytes = base64UrlToBytes(text);
  if (!bytes || bytes.length !== TOTAL_BYTES) return null;

  let offset = 0;
  if (bytes[offset++] !== FORMAT_VERSION) return null;

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

  return {
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
