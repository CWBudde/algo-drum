import { describe, expect, it } from "vitest";
import { TRACK_COUNT } from "../algo/pattern";
import {
  defaultPhysicalTomParams,
  defaultParamsFor,
  defaultVoiceParams,
  formatParam,
  mapParam,
  PHYSICAL_TOM_PARAM_CAPACITY,
  PHYSICAL_TOM_PARAMS,
  VOICE_NAMES,
  VOICE_PARAM_CAPACITY,
  VOICE_PARAMS,
} from "./voiceParams";

describe("the generated voice parameter table", () => {
  it("covers every engine track", () => {
    expect(VOICE_PARAMS).toHaveLength(TRACK_COUNT);
    expect(VOICE_NAMES).toHaveLength(TRACK_COUNT);
  });

  it("generates the independent physical Tom bank", () => {
    expect(PHYSICAL_TOM_PARAMS).toHaveLength(PHYSICAL_TOM_PARAM_CAPACITY);
    expect(PHYSICAL_TOM_PARAMS.map((spec) => spec.id)).toContain(
      "physicalTom.cavityCoupling",
    );
    expect(PHYSICAL_TOM_PARAMS.map((spec) => spec.id)).toContain(
      "physicalTom.quality",
    );
  });

  it("stays within the persistence capacity", () => {
    for (const params of VOICE_PARAMS) {
      expect(params.length).toBeGreaterThan(0);
      expect(params.length).toBeLessThanOrEqual(VOICE_PARAM_CAPACITY);
    }
  });

  it("has unique ids and sane ranges", () => {
    const seen = new Set<string>();

    for (const params of VOICE_PARAMS) {
      for (const spec of params) {
        expect(seen.has(spec.id), `duplicate id ${spec.id}`).toBe(false);
        seen.add(spec.id);

        expect(spec.min).toBeLessThan(spec.max);
        expect(spec.shipped).toBeGreaterThanOrEqual(spec.min);
        expect(spec.shipped).toBeLessThanOrEqual(spec.max);
        expect(spec.default).toBeGreaterThanOrEqual(0);
        expect(spec.default).toBeLessThanOrEqual(1);
        if (spec.kind === "exp") expect(spec.min).toBeGreaterThan(0);
      }
    }
  });
});

describe("mapParam", () => {
  it("returns the shipped constant at the default position", () => {
    for (const params of VOICE_PARAMS) {
      for (const spec of params) {
        expect(mapParam(spec, spec.default)).toBe(spec.shipped);
      }
    }
  });

  // The detent's whole reason for existing: persistence quantizes to a byte, so
  // a default of 0.4648 comes back as 0.4667 and would retune the voice.
  it("still returns the shipped constant after a byte round-trip", () => {
    for (const params of VOICE_PARAMS) {
      for (const spec of params) {
        const quantised = Math.round(spec.default * 255) / 255;
        expect(mapParam(spec, quantised)).toBe(spec.shipped);
      }
    }
  });

  it("maps the endpoints to min and max", () => {
    for (const params of VOICE_PARAMS) {
      for (const spec of params) {
        expect(mapParam(spec, 0)).toBe(spec.min);
        expect(mapParam(spec, 1)).toBe(spec.max);
      }
    }
  });

  it("is monotonic and clamps bad input", () => {
    for (const params of VOICE_PARAMS) {
      for (const spec of params) {
        let prev = mapParam(spec, 0);
        for (let step = 1; step <= 100; step++) {
          const got = mapParam(spec, step / 100);
          expect(got).toBeGreaterThanOrEqual(prev);
          prev = got;
        }

        for (const bad of [-1, 2, NaN, Infinity, -Infinity]) {
          const got = mapParam(spec, bad);
          expect(got).toBeGreaterThanOrEqual(spec.min);
          expect(got).toBeLessThanOrEqual(spec.max);
        }
      }
    }
  });

  // These triples are pinned identically in Go by
  // TestParamSpecMapMatchesTheGeneratedCurve. If one side's curve changes, one
  // of the two tests fails.
  it("matches the curve the Go engine applies", () => {
    const find = (id: string) => {
      const spec = VOICE_PARAMS.flat().find((p) => p.id === id);
      if (!spec) throw new Error(`no such param: ${id}`);
      return spec;
    };

    expect(mapParam(find("bass.pitchTo"), 0)).toBeCloseTo(25, 9);
    expect(mapParam(find("bass.pitchTo"), 1)).toBeCloseTo(120, 9);
    expect(mapParam(find("bass.pitchTo"), 0.25)).toBeCloseTo(
      25 * Math.pow(120 / 25, 0.25),
      9,
    );
    expect(mapParam(find("hat.gain"), 0.25)).toBeCloseTo(0.625, 9);
    expect(mapParam(find("hat.gain"), 0.8)).toBeCloseTo(2.0, 9);
    expect(mapParam(find("cym.decay"), 0.75)).toBeCloseTo(
      0.1 * Math.pow(40, 0.75),
      9,
    );
  });

  it("maps and formats the discrete quality tier", () => {
    const quality = PHYSICAL_TOM_PARAMS.find(
      (spec) => spec.id === "physicalTom.quality",
    )!;
    expect(mapParam(quality, 0)).toBe(0);
    expect(mapParam(quality, 0.5)).toBe(1);
    expect(mapParam(quality, 1)).toBe(2);
    expect(formatParam(quality, 0)).toBe("Draft");
    expect(formatParam(quality, 0.5)).toBe("Standard");
    expect(formatParam(quality, 1)).toBe("High");
  });
});

describe("formatParam", () => {
  it("renders every parameter as a non-empty readout", () => {
    for (const params of VOICE_PARAMS) {
      for (const spec of params) {
        for (const v of [0, 0.5, 1]) {
          expect(formatParam(spec, v).length).toBeGreaterThan(0);
        }
      }
    }
  });

  it("shows the shipped constants at the defaults", () => {
    const find = (id: string) => {
      const spec = VOICE_PARAMS.flat().find((p) => p.id === id);
      if (!spec) throw new Error(`no such param: ${id}`);
      return spec;
    };

    expect(
      formatParam(find("bass.pitchTo"), find("bass.pitchTo").default),
    ).toBe("50.0 Hz");
    expect(formatParam(find("bass.decay"), find("bass.decay").default)).toBe(
      "0.450 s",
    );
    expect(formatParam(find("snare.hpHz"), find("snare.hpHz").default)).toBe(
      "2.00 kHz",
    );
    expect(formatParam(find("hat.gain"), find("hat.gain").default)).toBe(
      "1.50",
    );
  });
});

describe("defaults", () => {
  it("pads each row to the persistence capacity", () => {
    for (let track = 0; track < TRACK_COUNT; track++) {
      const row = defaultParamsFor(track);
      expect(row).toHaveLength(VOICE_PARAM_CAPACITY);

      VOICE_PARAMS[track].forEach((spec, i) => {
        expect(row[i]).toBe(spec.default);
      });
    }
  });

  it("builds a full engine-major table", () => {
    const table = defaultVoiceParams();
    expect(table).toHaveLength(TRACK_COUNT);
    expect(table.every((row) => row.length === VOICE_PARAM_CAPACITY)).toBe(
      true,
    );
  });

  it("builds the physical Tom defaults from generated metadata", () => {
    expect(defaultPhysicalTomParams()).toEqual(
      PHYSICAL_TOM_PARAMS.map((spec) => spec.default),
    );
  });
});
