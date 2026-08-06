import { PATTERN_SIZE, STEP_CAPACITY, TRACK_COUNT } from "../algo/pattern";
import {
  PHYSICAL_TOM_PARAM_CAPACITY,
  VOICE_PARAMS,
  defaultPhysicalTomParams,
} from "./voiceParams";
import type { TomModel } from "./tomModel";

export interface TrackState {
  volume: number;
  decay: number;
  muted: boolean;
  voiceParams: Float32Array;
  tom?: never;
}

export interface TomState {
  model: TomModel;
  physicalParams: Float32Array;
}

export interface TomTrackState extends Omit<TrackState, "tom"> {
  tom: TomState;
}

// The tuple makes the engine-major ordering and the two Tom-only state banks
// part of the type instead of a convention each consumer has to remember.
export type EngineTracks = [
  TrackState,
  TrackState,
  TrackState,
  TomTrackState,
  TrackState,
  TomTrackState,
  TrackState,
];

export interface EngineState {
  tempoBpm: number;
  swing: number;
  stepCount: number;
  reverb: number;
  probability: number;
  humanize: number;
  pattern: Float32Array;
  tracks: EngineTracks;
}

export const TOM_TRACKS = [3, 5] as const;

export const CONFIGURATION_METHODS = [
  "setState",
  "setTempo",
  "setSwing",
  "setStepCount",
  "setCell",
  "setPattern",
  "setVolume",
  "setDecay",
  "setMuted",
  "setVoiceParam",
  "setPhysicalTomParam",
  "setTomModel",
  "setReverb",
  "setProbability",
  "setHumanize",
] as const;

export type ConfigurationMethod = (typeof CONFIGURATION_METHODS)[number];

const configurationMethods = new Set<string>(CONFIGURATION_METHODS);

export function isConfigurationMethod(
  name: PropertyKey,
): name is ConfigurationMethod {
  return typeof name === "string" && configurationMethods.has(name);
}

export function createDefaultEngineState(): EngineState {
  const tracks = Array.from({ length: TRACK_COUNT }, (_, track) => {
    const base: TrackState = {
      volume: 0.75,
      decay: 0.5,
      muted: false,
      voiceParams: Float32Array.from(
        VOICE_PARAMS[track].map((spec) => spec.default),
      ),
    };

    if (track !== TOM_TRACKS[0] && track !== TOM_TRACKS[1]) return base;

    return {
      ...base,
      tom: {
        model: "procedural" as const,
        physicalParams: Float32Array.from(defaultPhysicalTomParams()),
      },
    };
  }) as EngineTracks;

  return {
    tempoBpm: 120,
    swing: 0,
    stepCount: STEP_CAPACITY,
    reverb: 0,
    probability: 1,
    humanize: 0,
    pattern: new Float32Array(PATTERN_SIZE),
    tracks,
  };
}

export function cloneEngineState(state: EngineState): EngineState {
  return {
    tempoBpm: state.tempoBpm,
    swing: state.swing,
    stepCount: state.stepCount,
    reverb: state.reverb,
    probability: state.probability,
    humanize: state.humanize,
    pattern: state.pattern.slice(),
    tracks: state.tracks.map((track) => {
      const base: TrackState = {
        volume: track.volume,
        decay: track.decay,
        muted: track.muted,
        voiceParams: track.voiceParams.slice(),
      };

      if (!track.tom) return base;

      return {
        ...base,
        tom: {
          model: track.tom.model,
          physicalParams: track.tom.physicalParams.slice(),
        },
      };
    }) as EngineTracks,
  };
}

function isRecord(value: unknown): value is Record<PropertyKey, unknown> {
  return typeof value === "object" && value !== null;
}

function requireNumber(
  record: Record<PropertyKey, unknown>,
  key: string,
  min: number,
  max: number,
  integer = false,
): number {
  const value = record[key];
  if (
    typeof value !== "number" ||
    !Number.isFinite(value) ||
    value < min ||
    value > max ||
    (integer && !Number.isInteger(value))
  ) {
    throw new TypeError(
      `engine state ${key} must be ${integer ? "an integer " : ""}in [${min}, ${max}]`,
    );
  }

  return value;
}

function requireUnitArray(
  value: unknown,
  length: number,
  path: string,
): Float32Array {
  if (!(value instanceof Float32Array) || value.length !== length) {
    throw new TypeError(`${path} must be a Float32Array of length ${length}`);
  }

  for (let i = 0; i < value.length; i++) {
    if (!Number.isFinite(value[i]) || value[i] < 0 || value[i] > 1) {
      throw new TypeError(`${path}[${i}] must be finite and in [0, 1]`);
    }
  }

  return value;
}

// validateEngineState is the trust boundary for state returned by the foreign
// Go object and for bulk state supplied by application code. It returns the
// original object after validation so typed-array identity is preserved.
export function validateEngineState(value: unknown): EngineState {
  if (!isRecord(value)) throw new TypeError("engine state must be an object");

  requireNumber(value, "tempoBpm", 30, 300);
  requireNumber(value, "swing", 0, 0.5);
  requireNumber(value, "stepCount", 1, STEP_CAPACITY, true);
  requireNumber(value, "reverb", 0, 1);
  requireNumber(value, "probability", 0, 1);
  requireNumber(value, "humanize", 0, 1);
  requireUnitArray(value.pattern, PATTERN_SIZE, "engine state pattern");

  if (!Array.isArray(value.tracks) || value.tracks.length !== TRACK_COUNT) {
    throw new TypeError(`engine state tracks must have length ${TRACK_COUNT}`);
  }

  value.tracks.forEach((candidate, track) => {
    if (!isRecord(candidate)) {
      throw new TypeError(`engine state tracks[${track}] must be an object`);
    }

    requireNumber(candidate, "volume", 0, 1);
    requireNumber(candidate, "decay", 0, 1);
    if (typeof candidate.muted !== "boolean") {
      throw new TypeError(
        `engine state tracks[${track}].muted must be boolean`,
      );
    }
    requireUnitArray(
      candidate.voiceParams,
      VOICE_PARAMS[track].length,
      `engine state tracks[${track}].voiceParams`,
    );

    const isTom = track === TOM_TRACKS[0] || track === TOM_TRACKS[1];
    if (!isTom) {
      if ("tom" in candidate) {
        throw new TypeError(
          `engine state tracks[${track}] must not contain Tom state`,
        );
      }
      return;
    }

    if (!isRecord(candidate.tom)) {
      throw new TypeError(
        `engine state tracks[${track}].tom must be an object`,
      );
    }
    if (
      candidate.tom.model !== "procedural" &&
      candidate.tom.model !== "physical"
    ) {
      throw new TypeError(`engine state tracks[${track}].tom.model is invalid`);
    }
    requireUnitArray(
      candidate.tom.physicalParams,
      PHYSICAL_TOM_PARAM_CAPACITY,
      `engine state tracks[${track}].tom.physicalParams`,
    );
  });

  return value as unknown as EngineState;
}
