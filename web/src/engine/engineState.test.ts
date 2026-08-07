import { describe, expect, it } from "vitest";
import { PATTERN_SIZE, TRACK_COUNT } from "../algo/pattern";
import { PHYSICAL_TOM_PARAM_CAPACITY, VOICE_PARAMS } from "./voiceParams";
import {
  MAX_CHAIN_LENGTH,
  PATTERN_BANK_COUNT,
  cloneEngineState,
  createDefaultEngineState,
  validateEngineState,
} from "./engineState";

describe("EngineState", () => {
  it("builds canonical engine-major defaults", () => {
    const state = createDefaultEngineState();

    expect(state).toMatchObject({
      tempoBpm: 120,
      swing: 0,
      reverb: 0,
      probability: 1,
      humanize: 0,
      fillMode: false,
      standaloneBank: 0,
      chainEnabled: false,
    });
    expect(state.chain).toEqual(Uint8Array.of(0));
    expect(state.banks).toHaveLength(PATTERN_BANK_COUNT);
    state.banks.forEach((bank) => {
      expect(bank.stepCount).toBe(16);
      expect(bank.pattern).toEqual(new Float32Array(PATTERN_SIZE));
      expect(bank.cellProbabilities).toEqual(
        new Float32Array(PATTERN_SIZE).fill(1),
      );
      expect(bank.cellHumanize).toEqual(new Float32Array(PATTERN_SIZE).fill(1));
      expect(bank.cellConditions).toEqual(new Uint8Array(PATTERN_SIZE));
      expect(bank.trackLengths).toEqual(new Uint8Array(TRACK_COUNT).fill(16));
    });
    expect(state.banks[0].pattern).not.toBe(state.banks[1].pattern);
    expect(state.tracks).toHaveLength(TRACK_COUNT);

    state.tracks.forEach((track, index) => {
      expect(track).toMatchObject({ volume: 0.75, decay: 0.5, muted: false });
      expect(track.voiceParams).toHaveLength(VOICE_PARAMS[index].length);
      if (index === 3 || index === 5) {
        expect(track.tom?.model).toBe("procedural");
        expect(track.tom?.physicalParams).toHaveLength(
          PHYSICAL_TOM_PARAM_CAPACITY,
        );
      } else {
        expect(track).not.toHaveProperty("tom");
      }
    });

    expect(validateEngineState(state)).toBe(state);
  });

  it("deep-clones every typed array", () => {
    const state = createDefaultEngineState();
    const clone = cloneEngineState(state);

    expect(clone).not.toBe(state);
    expect(clone.banks).not.toBe(state.banks);
    clone.banks.forEach((bank, index) => {
      expect(bank).not.toBe(state.banks[index]);
      expect(bank.pattern).not.toBe(state.banks[index].pattern);
      expect(bank.cellProbabilities).not.toBe(
        state.banks[index].cellProbabilities,
      );
      expect(bank.cellHumanize).not.toBe(state.banks[index].cellHumanize);
      expect(bank.cellConditions).not.toBe(state.banks[index].cellConditions);
      expect(bank.trackLengths).not.toBe(state.banks[index].trackLengths);
    });
    expect(clone.chain).not.toBe(state.chain);
    clone.tracks.forEach((track, index) => {
      expect(track).not.toBe(state.tracks[index]);
      expect(track.voiceParams).not.toBe(state.tracks[index].voiceParams);
      if (track.tom) {
        expect(track.tom.physicalParams).not.toBe(
          state.tracks[index].tom?.physicalParams,
        );
      }
    });
  });

  it.each([
    [
      "short pattern",
      (s: ReturnType<typeof createDefaultEngineState>) => {
        s.banks[2].pattern = new Float32Array(PATTERN_SIZE - 1);
      },
    ],
    [
      "non-finite scalar",
      (s: ReturnType<typeof createDefaultEngineState>) => {
        s.humanize = Number.NaN;
      },
    ],
    [
      "fractional step count",
      (s: ReturnType<typeof createDefaultEngineState>) => {
        s.banks[0].stepCount = 8.5;
      },
    ],
    [
      "short cell probability table",
      (s: ReturnType<typeof createDefaultEngineState>) => {
        s.banks[0].cellProbabilities = new Float32Array(PATTERN_SIZE - 1);
      },
    ],
    [
      "short cell humanize table",
      (s: ReturnType<typeof createDefaultEngineState>) => {
        s.banks[0].cellHumanize = new Float32Array(PATTERN_SIZE - 1);
      },
    ],
    [
      "invalid trigger condition",
      (s: ReturnType<typeof createDefaultEngineState>) => {
        s.banks[0].cellConditions[0] = 7;
      },
    ],
    [
      "invalid track length",
      (s: ReturnType<typeof createDefaultEngineState>) => {
        s.banks[0].trackLengths[0] = 0;
      },
    ],
    [
      "missing Tom state",
      (s: ReturnType<typeof createDefaultEngineState>) => {
        delete (s.tracks[3] as { tom?: unknown }).tom;
      },
    ],
    [
      "Tom state on a non-Tom",
      (s: ReturnType<typeof createDefaultEngineState>) => {
        (s.tracks[0] as unknown as { tom: unknown }).tom = s.tracks[3].tom;
      },
    ],
    [
      "bad parameter width",
      (s: ReturnType<typeof createDefaultEngineState>) => {
        s.tracks[2].voiceParams = new Float32Array(VOICE_PARAMS[2].length - 1);
      },
    ],
  ])("rejects %s", (_name, corrupt) => {
    const state = createDefaultEngineState();
    corrupt(state);
    expect(() => validateEngineState(state)).toThrow(TypeError);
  });

  it("validates bank selection and a bounded non-empty chain", () => {
    const state = createDefaultEngineState();
    state.standaloneBank = 3;
    state.chainEnabled = true;
    state.chain = Uint8Array.from([0, 3, 1, 1]);
    expect(validateEngineState(state)).toBe(state);

    state.chain = new Uint8Array(MAX_CHAIN_LENGTH + 1);
    expect(() => validateEngineState(state)).toThrow(/chain/);
    state.chain = Uint8Array.of(4);
    expect(() => validateEngineState(state)).toThrow(/chain\[0\]/);
    state.chain = Uint8Array.of(0);
    state.standaloneBank = PATTERN_BANK_COUNT;
    expect(() => validateEngineState(state)).toThrow(/standaloneBank/);
  });
});
