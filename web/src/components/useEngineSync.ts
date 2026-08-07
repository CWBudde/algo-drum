import { useCallback, useEffect, useRef, useState, type Dispatch } from "react";
import { saveLocal } from "../algo/persistence";
import type { EngineState } from "../engine/engineState";
import * as engine from "../engine/wasmEngine";
import type { DrumStateAction } from "./drumState";

function sendStateAction(action: DrumStateAction): void {
  switch (action.type) {
    case "replace":
      return;
    case "tempo":
      engine.setTempo(action.value);
      return;
    case "swing":
      engine.setSwing(action.value);
      return;
    case "stepCount":
      engine.setStepCount(action.bank, action.value);
      return;
    case "reverb":
      engine.setReverb(action.value);
      return;
    case "probability":
      engine.setProbability(action.value);
      return;
    case "humanize":
      engine.setHumanize(action.value);
      return;
    case "cell":
      engine.setCell(action.bank, action.track, action.step, action.value);
      return;
    case "cellProbability":
      engine.setCellProbability(
        action.bank,
        action.track,
        action.step,
        action.value,
      );
      return;
    case "cellHumanize":
      engine.setCellHumanize(
        action.bank,
        action.track,
        action.step,
        action.value,
      );
      return;
    case "cellCondition":
      engine.setCellCondition(
        action.bank,
        action.track,
        action.step,
        action.value,
      );
      return;
    case "trackLength":
      engine.setTrackLength(action.bank, action.track, action.value);
      return;
    case "fillMode":
      engine.setFillMode(action.value);
      return;
    case "pattern":
      engine.setPattern(action.bank, action.value);
      return;
    case "patternBank":
      engine.setPatternBank(action.bank, action.value);
      return;
    case "requestBank":
      engine.requestBank(action.value);
      return;
    case "chain":
      engine.setChain(action.value);
      return;
    case "chainEnabled":
      engine.setChainEnabled(action.value);
      return;
    case "volume":
      engine.setVolume(action.track, action.value);
      return;
    case "decay":
      engine.setDecay(action.track, action.value);
      return;
    case "muted":
      engine.setMuted(action.track, action.value);
      return;
    case "voiceParam":
      engine.setVoiceParam(action.track, action.index, action.value);
      return;
    case "voiceParams":
      action.value.forEach((value, index) => {
        engine.setVoiceParam(action.track, index, value);
      });
      return;
    case "tomModel":
      engine.setTomModel(action.track, action.value);
      return;
    case "physicalTomParam":
      engine.setPhysicalTomParam(action.track, action.index, action.value);
      return;
    case "physicalTomParams":
      action.value.forEach((value, index) => {
        engine.setPhysicalTomParam(action.track, index, value);
      });
      return;
  }

  const exhaustive: never = action;
  return exhaustive;
}

interface Options {
  wasmLoaded: boolean;
  initial: EngineState;
  state: EngineState;
  dispatch: Dispatch<DrumStateAction>;
}

export function useEngineSync({
  wasmLoaded,
  initial,
  state,
  dispatch,
}: Options) {
  const currentState = useRef(state);
  currentState.current = state;
  const [transport, setTransport] = useState<engine.TransportState>("stopped");
  const [bankPlayback, setBankPlayback] = useState<engine.BankPlayback>({
    activeBank: 0,
    queuedBank: -1,
    chainPosition: -1,
  });

  const applyStateAction = useCallback(
    (action: DrumStateAction) => {
      dispatch(action);
      sendStateAction(action);
    },
    [dispatch],
  );

  useEffect(() => {
    engine.setState(initial);
    // Mount-only: seed the engine with one complete restored/default state.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const hasLoaded = useRef(false);
  useEffect(() => {
    if (!wasmLoaded) return;
    if (hasLoaded.current) engine.setState(currentState.current);
    hasLoaded.current = true;
  }, [wasmLoaded]);

  useEffect(() => engine.onTransport(setTransport), []);
  useEffect(() => engine.onBankPlayback(setBankPlayback), []);
  useEffect(
    () => engine.onState((next) => dispatch({ type: "replace", state: next })),
    [dispatch],
  );

  useEffect(() => {
    const id = window.setTimeout(() => saveLocal(state), 300);
    return () => window.clearTimeout(id);
  }, [state]);

  return { applyStateAction, transport, bankPlayback };
}
