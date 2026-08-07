import { describe, expect, it } from "vitest";
import { PATTERN_SIZE, TRACK_COUNT } from "../algo/pattern";
import { PHYSICAL_TOM_PARAM_CAPACITY, VOICE_PARAMS } from "./voiceParams";
import {
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
      stepCount: 16,
      reverb: 0,
      probability: 1,
      humanize: 0,
      fillMode: false,
    });
    expect(state.pattern).toEqual(new Float32Array(PATTERN_SIZE));
    expect(state.cellProbabilities).toEqual(
      new Float32Array(PATTERN_SIZE).fill(1),
    );
    expect(state.cellConditions).toEqual(new Uint8Array(PATTERN_SIZE));
    expect(state.trackLengths).toEqual(new Uint8Array(TRACK_COUNT).fill(16));
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
    expect(clone.pattern).not.toBe(state.pattern);
    expect(clone.cellProbabilities).not.toBe(state.cellProbabilities);
    expect(clone.cellConditions).not.toBe(state.cellConditions);
    expect(clone.trackLengths).not.toBe(state.trackLengths);
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
        s.pattern = new Float32Array(PATTERN_SIZE - 1);
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
        s.stepCount = 8.5;
      },
    ],
    [
      "short cell probability table",
      (s: ReturnType<typeof createDefaultEngineState>) => {
        s.cellProbabilities = new Float32Array(PATTERN_SIZE - 1);
      },
    ],
    [
      "invalid trigger condition",
      (s: ReturnType<typeof createDefaultEngineState>) => {
        s.cellConditions[0] = 7;
      },
    ],
    [
      "invalid track length",
      (s: ReturnType<typeof createDefaultEngineState>) => {
        s.trackLengths[0] = 0;
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
});
