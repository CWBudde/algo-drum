import { describe, expect, it } from "vitest";
import { euclid } from "./euclid";

// Render a boolean pattern as a compact "10010010" string for readable asserts.
const bits = (a: boolean[]): string => a.map((x) => (x ? "1" : "0")).join("");

describe("euclid", () => {
  it("produces the classic tresillo E(3,8)", () => {
    expect(bits(euclid(3, 8))).toBe("10010010");
  });

  it("produces cinquillo E(5,8)", () => {
    expect(bits(euclid(5, 8))).toBe("10110110");
  });

  it("produces E(5,16)", () => {
    expect(bits(euclid(5, 16))).toBe("1001001001001000");
  });

  it("produces evenly spread E(4,16)", () => {
    expect(bits(euclid(4, 16))).toBe("1000100010001000");
  });

  it("always yields `pulses` onsets and `steps` length", () => {
    for (let steps = 1; steps <= 16; steps++) {
      for (let k = 0; k <= steps; k++) {
        const out = euclid(k, steps);
        expect(out).toHaveLength(steps);
        expect(out.filter(Boolean)).toHaveLength(k);
      }
    }
  });

  describe("rotation", () => {
    it("rotates left by a positive amount", () => {
      expect(bits(euclid(3, 8, 1))).toBe("00100101");
      expect(bits(euclid(3, 8, 2))).toBe("01001010");
    });

    it("rotates right for negative amounts (wraps modulo steps)", () => {
      expect(bits(euclid(3, 8, -1))).toBe("01001001");
    });

    it("is periodic in rotation with period `steps`", () => {
      expect(bits(euclid(5, 16, 3))).toBe(bits(euclid(5, 16, 3 + 16)));
    });

    it("preserves the onset count under rotation", () => {
      const out = euclid(3, 8, 5);
      expect(out.filter(Boolean)).toHaveLength(3);
    });
  });

  describe("edge cases", () => {
    it("k = 0 → all rests", () => {
      expect(bits(euclid(0, 8))).toBe("00000000");
    });

    it("k = n → all onsets", () => {
      expect(bits(euclid(8, 8))).toBe("11111111");
    });

    it("clamps pulses > steps down to steps (all onsets)", () => {
      expect(bits(euclid(10, 8))).toBe("11111111");
    });

    it("clamps negative pulses to zero", () => {
      expect(bits(euclid(-3, 8))).toBe("00000000");
    });

    it("steps = 0 → empty array (never throws)", () => {
      expect(euclid(0, 0)).toEqual([]);
      expect(euclid(3, 0)).toEqual([]);
    });

    it("floors fractional inputs", () => {
      expect(bits(euclid(3.9, 8.9, 1.9))).toBe(bits(euclid(3, 8, 1)));
    });
  });
});
