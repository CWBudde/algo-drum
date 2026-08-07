import {
  createDefaultEngineState,
  type EngineState,
  type EngineTracks,
  type PatternBankState,
} from "../engine/engineState";
import type { TomModel } from "../engine/tomModel";
import { STEP_CAPACITY, TRACK_COUNT } from "../algo/pattern";

export const DEFAULT_TEMPO_BPM = 120;

export type DrumStateAction =
  | { type: "replace"; state: EngineState }
  | { type: "tempo"; value: number }
  | { type: "swing"; value: number }
  | { type: "stepCount"; bank: number; value: number }
  | { type: "reverb"; value: number }
  | { type: "probability"; value: number }
  | { type: "humanize"; value: number }
  | { type: "cell"; bank: number; track: number; step: number; value: number }
  | {
      type: "cellProbability";
      bank: number;
      track: number;
      step: number;
      value: number;
    }
  | {
      type: "cellHumanize";
      bank: number;
      track: number;
      step: number;
      value: number;
    }
  | {
      type: "cellCondition";
      bank: number;
      track: number;
      step: number;
      value: number;
    }
  | {
      type: "cellRepeats";
      bank: number;
      track: number;
      step: number;
      value: number;
    }
  | { type: "trackLength"; bank: number; track: number; value: number }
  | { type: "fillMode"; value: boolean }
  | { type: "pattern"; bank: number; value: Float32Array }
  | { type: "patternBank"; bank: number; value: PatternBankState }
  | { type: "requestBank"; value: number }
  | { type: "chain"; value: Uint8Array }
  | { type: "chainEnabled"; value: boolean }
  | { type: "volume"; track: number; value: number }
  | { type: "decay"; track: number; value: number }
  | { type: "muted"; track: number; value: boolean }
  | { type: "voiceParam"; track: number; index: number; value: number }
  | { type: "voiceParams"; track: number; value: Float32Array }
  | { type: "tomModel"; track: number; value: TomModel }
  | { type: "physicalTomParam"; track: number; index: number; value: number }
  | { type: "physicalTomParams"; track: number; value: Float32Array };

function updateTrack(
  state: EngineState,
  track: number,
  update: (
    current: EngineState["tracks"][number],
  ) => EngineState["tracks"][number],
): EngineState {
  if (track < 0 || track >= state.tracks.length) return state;

  const tracks = [...state.tracks];
  tracks[track] = update(tracks[track]);
  return { ...state, tracks: tracks as EngineTracks };
}

function clonePatternBank(bank: PatternBankState): PatternBankState {
  return {
    stepCount: bank.stepCount,
    pattern: bank.pattern.slice(),
    cellProbabilities: bank.cellProbabilities.slice(),
    cellHumanize: bank.cellHumanize.slice(),
    cellConditions: bank.cellConditions.slice(),
    cellRepeats: bank.cellRepeats.slice(),
    trackLengths: bank.trackLengths.slice(),
  };
}

function updateBank(
  state: EngineState,
  bank: number,
  update: (current: PatternBankState) => PatternBankState,
): EngineState {
  if (bank < 0 || bank >= state.banks.length) return state;
  const banks = [...state.banks];
  banks[bank] = update(banks[bank]);
  return { ...state, banks: banks as EngineState["banks"] };
}

export function reduceDrumState(
  state: EngineState,
  action: DrumStateAction,
): EngineState {
  switch (action.type) {
    case "replace":
      return action.state;
    case "tempo":
      return { ...state, tempoBpm: action.value };
    case "swing":
      return { ...state, swing: action.value };
    case "stepCount":
      return updateBank(state, action.bank, (bank) => ({
        ...bank,
        stepCount: action.value,
        trackLengths: new Uint8Array(TRACK_COUNT).fill(action.value),
      }));
    case "reverb":
      return { ...state, reverb: action.value };
    case "probability":
      return { ...state, probability: action.value };
    case "humanize":
      return { ...state, humanize: action.value };
    case "cell": {
      if (
        action.track < 0 ||
        action.track >= TRACK_COUNT ||
        action.step < 0 ||
        action.step >= STEP_CAPACITY
      ) {
        return state;
      }
      const index = action.track * STEP_CAPACITY + action.step;
      return updateBank(state, action.bank, (bank) => {
        const pattern = new Float32Array(bank.pattern);
        pattern[index] = action.value;
        return { ...bank, pattern };
      });
    }
    case "cellProbability": {
      if (
        action.track < 0 ||
        action.track >= TRACK_COUNT ||
        action.step < 0 ||
        action.step >= STEP_CAPACITY
      ) {
        return state;
      }
      return updateBank(state, action.bank, (bank) => {
        const cellProbabilities = bank.cellProbabilities.slice();
        cellProbabilities[action.track * STEP_CAPACITY + action.step] =
          action.value;
        return { ...bank, cellProbabilities };
      });
    }
    case "cellHumanize": {
      if (
        action.track < 0 ||
        action.track >= TRACK_COUNT ||
        action.step < 0 ||
        action.step >= STEP_CAPACITY
      ) {
        return state;
      }
      return updateBank(state, action.bank, (bank) => {
        const cellHumanize = bank.cellHumanize.slice();
        cellHumanize[action.track * STEP_CAPACITY + action.step] = action.value;
        return { ...bank, cellHumanize };
      });
    }
    case "cellCondition": {
      if (
        action.track < 0 ||
        action.track >= TRACK_COUNT ||
        action.step < 0 ||
        action.step >= STEP_CAPACITY
      ) {
        return state;
      }
      return updateBank(state, action.bank, (bank) => {
        const cellConditions = bank.cellConditions.slice();
        cellConditions[action.track * STEP_CAPACITY + action.step] =
          action.value;
        return { ...bank, cellConditions };
      });
    }
    case "cellRepeats": {
      if (
        action.track < 0 ||
        action.track >= TRACK_COUNT ||
        action.step < 0 ||
        action.step >= STEP_CAPACITY
      ) {
        return state;
      }
      return updateBank(state, action.bank, (bank) => {
        const cellRepeats = bank.cellRepeats.slice();
        cellRepeats[action.track * STEP_CAPACITY + action.step] = action.value;
        return { ...bank, cellRepeats };
      });
    }
    case "trackLength": {
      if (action.track < 0 || action.track >= TRACK_COUNT) return state;
      return updateBank(state, action.bank, (bank) => {
        const trackLengths = bank.trackLengths.slice();
        trackLengths[action.track] = action.value;
        return { ...bank, trackLengths };
      });
    }
    case "fillMode":
      return { ...state, fillMode: action.value };
    case "pattern":
      return updateBank(state, action.bank, (bank) => ({
        ...bank,
        pattern: new Float32Array(action.value),
      }));
    case "patternBank":
      return updateBank(state, action.bank, () =>
        clonePatternBank(action.value),
      );
    case "requestBank":
      return { ...state, standaloneBank: action.value };
    case "chain":
      return { ...state, chain: action.value.slice() };
    case "chainEnabled":
      return { ...state, chainEnabled: action.value };
    case "volume":
      return updateTrack(state, action.track, (track) => ({
        ...track,
        volume: action.value,
      }));
    case "decay":
      return updateTrack(state, action.track, (track) => ({
        ...track,
        decay: action.value,
      }));
    case "muted":
      return updateTrack(state, action.track, (track) => ({
        ...track,
        muted: action.value,
      }));
    case "voiceParam":
      return updateTrack(state, action.track, (track) => {
        if (action.index < 0 || action.index >= track.voiceParams.length) {
          return track;
        }
        const voiceParams = new Float32Array(track.voiceParams);
        voiceParams[action.index] = action.value;
        return { ...track, voiceParams };
      });
    case "voiceParams":
      return updateTrack(state, action.track, (track) => ({
        ...track,
        voiceParams: new Float32Array(action.value),
      }));
    case "tomModel":
      return updateTrack(state, action.track, (track) =>
        track.tom
          ? { ...track, tom: { ...track.tom, model: action.value } }
          : track,
      );
    case "physicalTomParam":
      return updateTrack(state, action.track, (track) => {
        if (
          !track.tom ||
          action.index < 0 ||
          action.index >= track.tom.physicalParams.length
        ) {
          return track;
        }
        const physicalParams = new Float32Array(track.tom.physicalParams);
        physicalParams[action.index] = action.value;
        return { ...track, tom: { ...track.tom, physicalParams } };
      });
    case "physicalTomParams":
      return updateTrack(state, action.track, (track) =>
        track.tom
          ? {
              ...track,
              tom: {
                ...track.tom,
                physicalParams: new Float32Array(action.value),
              },
            }
          : track,
      );
  }
}

export function defaultEngineState(): EngineState {
  return createDefaultEngineState();
}
