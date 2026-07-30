// Named settings for the physical Tom's parameter bank.
//
// Each preset is a sparse map from parameter ID to normalized knob position;
// anything it does not name keeps that knob's default. Keying by ID rather
// than index is what makes the table survive the bank growing: indices are
// append-only for persistence, but a preset written before ATK.L existed
// should still mean what it meant.
//
// Pure and dependency-free, so it is unit-testable without a DOM or an engine.

import type { VoiceParamSpec } from "../engine/voiceParams";

export interface PhysicalTomPreset {
  name: string;
  /** One line, shown beside the name. */
  description: string;
  /** Normalized 0..1 positions by parameter ID. */
  values: Readonly<Record<string, number>>;
}

/**
 * The shipped defaults, named so the list can be returned to.
 *
 * Its `values` is empty on purpose — "default" means every knob at its own
 * default, whatever those become.
 */
export const DEFAULT_PHYSICAL_TOM_PRESET: PhysicalTomPreset = {
  name: "Default",
  description: "The shipped calibration",
  values: {},
};

/**
 * The best fit to `reference/tom.wav`, from `cmd/fit-physical`.
 *
 * A preset rather than the shipped default on purpose: it matches the
 * recording's pitch, ring and envelope closely, but its spectral envelope is
 * 13.6 dB off — the reference has a dense 476–700 Hz cluster this model does
 * not produce at any quality tier. See docs/physical-measured-fit.md; the
 * reference itself is of unknown provenance and is not in the repository.
 *
 * QUAL is deliberately absent: the fit pinned it to Draft because mode count is
 * a CPU budget decision, so the preset leaves whatever the user has chosen.
 */
export const FITTED_PHYSICAL_TOM_PRESET: PhysicalTomPreset = {
  name: "Measured tom",
  description: "Fitted to a recorded tom — deep, long ring",
  values: {
    "physicalTom.diameter": 0.599516,
    "physicalTom.batterTension": 0.043456,
    "physicalTom.resonantTension": 0.198698,
    "physicalTom.damping": 0.582619,
    "physicalTom.strikeRadius": 0.632891,
    "physicalTom.strikeAngle": 0.534798,
    "physicalTom.hardness": 0.426549,
    "physicalTom.shellDepth": 0.223144,
    "physicalTom.cavityCoupling": 0.454688,
    "physicalTom.nonlinearity": 0.181237,
    "physicalTom.pickupRadius": 0.599915,
    "physicalTom.pickupAngle": 0.525678,
    "physicalTom.asymmetry": 0.539759,
    "physicalTom.asymmetryAxis": 0.473703,
    "physicalTom.dampingTilt": 0.13751,
    "physicalTom.attackLevel": 0.141951,
    "physicalTom.attackTone": 0.429341,
  },
};

export const PHYSICAL_TOM_PRESETS: readonly PhysicalTomPreset[] = [
  DEFAULT_PHYSICAL_TOM_PRESET,
  FITTED_PHYSICAL_TOM_PRESET,
];

/**
 * Expand a preset to a full bank of normalized positions, in spec order.
 *
 * Unknown IDs in the preset are ignored rather than throwing: a share link or
 * a stale table naming a parameter this build does not have should degrade to
 * the parameters it does have, not fail to load.
 */
export function presetValues(
  preset: PhysicalTomPreset,
  specs: readonly VoiceParamSpec[],
): number[] {
  return specs.map((spec) => {
    const value = preset.values[spec.id];
    return typeof value === "number" && Number.isFinite(value)
      ? Math.max(0, Math.min(1, value))
      : spec.default;
  });
}
