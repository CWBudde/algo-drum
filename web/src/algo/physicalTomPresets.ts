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
 * recording's envelope, glide and attack balance closely and covers most of its
 * partials, but it misses all three adoption-gate terms — partial frequency
 * 56.9 ¢ against 25, partial decay 0.581 against 0.25, spectral envelope
 * 12.3 dB against 4. See docs/physical-measured-fit.md; the reference itself is
 * of unknown provenance and is not in the repository.
 *
 * Re-derived 2026-07-31 from the run that supersedes every earlier one: the
 * right channel rather than the combed mono downmix, and the corrected partial
 * measurement — the level is now read off each partial's own decay fit, which
 * changed both which partials the reference is found to have (14, not 7) and
 * how loud they are. 8 restarts × 150 iterations, Standard quality, prescribed
 * contact.
 *
 * QUAL is deliberately absent: the fit pinned it because mode count is a CPU
 * budget decision, so the preset leaves whatever the user has chosen.
 *
 * Total 11.252 from a 33.094 baseline.
 */
export const FITTED_PHYSICAL_TOM_PRESET: PhysicalTomPreset = {
  name: "Measured tom",
  description: "Fitted to a recorded tom — deep, long ring",
  values: {
    "physicalTom.diameter": 0.52171,
    "physicalTom.batterTension": 0.325094,
    "physicalTom.resonantTension": 0.372115,
    "physicalTom.damping": 0.375914,
    "physicalTom.strikeRadius": 0.240823,
    "physicalTom.strikeAngle": 0.506931,
    "physicalTom.hardness": 0.612176,
    "physicalTom.shellDepth": 0.480795,
    "physicalTom.cavityCoupling": 0.326593,
    "physicalTom.nonlinearity": 0.538102,
    "physicalTom.pickupRadius": 0.271435,
    "physicalTom.pickupAngle": 0.062234,
    "physicalTom.asymmetry": 0.38292,
    "physicalTom.asymmetryAxis": 0.236722,
    "physicalTom.dampingTilt": 0.103796,
    "physicalTom.attackLevel": 0.346311,
    "physicalTom.attackTone": 0.255992,
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
