// Per-voice synthesis parameters: the curve renderer over the generated table.
//
// The table itself (ranges, defaults, labels) is generated from the Go engine
// into voiceParams.generated.ts — see cmd/gen-voiceparams. Only the two-line
// curve and the readout formatting live here, and Go pins them with
// TestParamSpecMapMatchesTheGeneratedCurve against this module's counterpart
// test, so the two implementations cannot drift.

import {
  PHYSICAL_TOM_PARAM_CAPACITY,
  PHYSICAL_TOM_PARAMS,
  VOICE_NAMES,
  VOICE_PARAM_CAPACITY,
  VOICE_PARAMS,
  type VoiceParamSpec,
} from "./voiceParams.generated";

export {
  PHYSICAL_TOM_PARAMS,
  PHYSICAL_TOM_PARAM_CAPACITY,
  VOICE_NAMES,
  VOICE_PARAM_CAPACITY,
  VOICE_PARAMS,
};
export type { VoiceParamSpec };

// One step of the 8-bit persistence quantisation (see algo/persistence.ts).
const BYTE_STEP = 1 / 255;

function clamp01(v: number): number {
  if (!Number.isFinite(v)) return 0;
  return v < 0 ? 0 : v > 1 ? 1 : v;
}

// mapParam converts a normalized knob position to engineering units. It mirrors
// ParamSpec.Map in internal/drum/params.go, including the detent: within half a
// persistence byte step of the default it returns the shipped constant exactly,
// so a state that round-trips through the blob still reads (and sounds) like an
// untouched voice rather than a slightly detuned one.
export function mapParam(spec: VoiceParamSpec, value01: number): number {
  const v = clamp01(value01);

  if (Math.abs(v - spec.default) < BYTE_STEP / 2) return spec.shipped;

  if (spec.choices) return Math.round(v * (spec.choices.length - 1));

  if (spec.kind === "exp") return spec.min * Math.pow(spec.max / spec.min, v);

  return spec.min + (spec.max - spec.min) * v;
}

// formatParam renders a knob position for the readout, e.g. "200 Hz", "0.45 s".
// Frequencies above 1 kHz switch to kHz so the label stays short.
export function formatParam(spec: VoiceParamSpec, value01: number): string {
  const value = mapParam(spec, value01);

  if (spec.choices) return spec.choices[value] ?? spec.choices[0];

  if (spec.unit === "Hz" && value >= 1000) {
    return `${(value / 1000).toFixed(2)} kHz`;
  }

  const text = value.toFixed(spec.digits);

  return spec.unit ? `${text} ${spec.unit}` : text;
}

// defaultParamsFor returns a voice's default positions, padded to the
// persistence capacity so every row has the same width.
export function defaultParamsFor(track: number): number[] {
  const specs = VOICE_PARAMS[track] ?? [];

  return Array.from({ length: VOICE_PARAM_CAPACITY }, (_, i) =>
    i < specs.length ? specs[i].default : 0,
  );
}

// defaultVoiceParams returns the full engine-major default table, used when a
// v1 blob (or no saved state at all) leaves the voice parameters unspecified.
export function defaultVoiceParams(): number[][] {
  return VOICE_PARAMS.map((_, track) => defaultParamsFor(track));
}

export function defaultPhysicalTomParams(): number[] {
  return PHYSICAL_TOM_PARAMS.map((spec) => spec.default);
}
