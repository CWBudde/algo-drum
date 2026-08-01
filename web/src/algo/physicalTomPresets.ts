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
 * 48.9 ¢ against 25, partial decay 0.573 against 0.25, spectral envelope
 * 11.1 dB against 4. See docs/physical-measured-fit.md; the reference itself is
 * of unknown provenance and is not in the repository.
 *
 * Re-derived 2026-08-01 under the corrected glide estimator, which had been
 * reading a partial that was dead before the late probe and so fitted every
 * earlier bank against a reference bend of +120.4 cents instead of the true
 * +58.9. Totals from before that fix are not comparable: the bank this replaced
 * was reported as 11.252 and re-measures at 18.991. 12 restarts × 150
 * iterations over a population of 16, four of them seeded, all complete;
 * right channel, Standard quality, prescribed contact.
 *
 * QUAL is deliberately absent: the fit pinned it because mode count is a CPU
 * budget decision, so the preset leaves whatever the user has chosen. Strike
 * velocity is likewise absent — it is a search dimension, not a bank knob; the
 * fit found 0.575, so this bank is what the drum sounds like at a medium hit.
 *
 * Total 10.382 from a 32.442 baseline.
 */
export const FITTED_PHYSICAL_TOM_PRESET: PhysicalTomPreset = {
  name: "Measured tom",
  description: "Fitted to a recorded tom — deep, long ring",
  values: {
    "physicalTom.diameter": 0.286098,
    "physicalTom.batterTension": 0.117949,
    "physicalTom.resonantTension": 0.160069,
    "physicalTom.damping": 0.323183,
    "physicalTom.strikeRadius": 0.656246,
    "physicalTom.strikeAngle": 0.519759,
    "physicalTom.hardness": 0.334189,
    "physicalTom.shellDepth": 0.421374,
    "physicalTom.cavityCoupling": 0.260876,
    "physicalTom.nonlinearity": 0.260007,
    "physicalTom.pickupRadius": 0.445833,
    "physicalTom.pickupAngle": 0.64485,
    "physicalTom.asymmetry": 0.417254,
    "physicalTom.asymmetryAxis": 0.543637,
    "physicalTom.dampingTilt": 0.215175,
    "physicalTom.attackLevel": 0.044801,
    "physicalTom.attackTone": 0.52322,
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
