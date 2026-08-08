import {
  useCallback,
  useEffect,
  useMemo,
  useReducer,
  useRef,
  useState,
} from "react";
import { STEP_CAPACITY } from "../algo/pattern";
import { mutate } from "../algo/mutate";
import { PRESETS, presetToFlat } from "../algo/presets";
import {
  loadInitialState,
  replaceAddressBarWithShareUrl,
} from "../algo/persistence";
import * as engine from "../engine/wasmEngine";
import type { PatternBankState } from "../engine/engineState";
import type { TomModel } from "../engine/tomModel";
import {
  defaultPhysicalTomParams,
  PHYSICAL_TOM_PARAMS,
  VOICE_NAMES,
  VOICE_PARAMS,
} from "../engine/voiceParams";
import AlgoPanel from "./AlgoPanel";
import { matchingPresetIndex } from "./algoPanelState";
import CellInspector from "./CellInspector";
import { demoEngineState, reduceDrumState } from "./drumState";
import { usePatternHistory } from "./patternHistory";
import {
  applyPaintedCells,
  cellIndex,
  flatToVisual,
  TRACK_INDEX,
} from "./patternView";
import PatternBanks from "./PatternBanks";
import { clearRow, copyBar, copyRow, rotateRow } from "./rowOperations";
import ShortcutHelp from "./ShortcutHelp";
import { resolveGlobalShortcut } from "./shortcuts";
import StepGrid from "./StepGrid";
import Transport from "./Transport";
import { useEngineSync } from "./useEngineSync";
import VoiceEditor from "./VoiceEditor";
import "./DrumMachine.css";

const TOM_TRACK = 3;
const TOM2_TRACK = 5;
const BPM_MIN = 30;
const BPM_MAX = 300;
const TAP_RESET_MS = 2000;
const TAP_WINDOW = 4;

function requiredAt<T>(values: ArrayLike<T>, index: number): T {
  const value = values[index];
  if (value === undefined) {
    throw new RangeError(`Missing value at index ${index}`);
  }
  return value;
}

interface Props {
  wasmLoaded: boolean;
}

interface SelectedCell {
  track: number;
  step: number;
}

export default function DrumMachine({ wasmLoaded }: Props) {
  const initial = useMemo(() => loadInitialState() ?? demoEngineState(), []);
  const [drumState, dispatch] = useReducer(reduceDrumState, initial);
  const { applyStateAction, transport, bankPlayback } = useEngineSync({
    wasmLoaded,
    initial,
    state: drumState,
    dispatch,
  });
  const [editorTrack, setEditorTrack] = useState<number | null>(null);
  const [selectedCell, setSelectedCell] = useState<SelectedCell | null>(null);
  const [shortcutHelpOpen, setShortcutHelpOpen] = useState(false);
  const [shortcutAnnouncement, setShortcutAnnouncement] = useState("");
  const shortcutHelpOpener = useRef<HTMLElement | null>(null);
  const [selectedBank, setSelectedBank] = useState(() =>
    initial.chainEnabled ? (initial.chain[0] ?? 0) : initial.standaloneBank,
  );
  const currentBank = drumState.banks[selectedBank] ?? drumState.banks[0];

  const applyPatternBank = useCallback(
    (bank: number, value: PatternBankState) => {
      applyStateAction({ type: "patternBank", bank, value });
    },
    [applyStateAction],
  );
  const patternHistory = usePatternHistory(
    selectedBank,
    drumState.banks,
    applyPatternBank,
  );
  const { undo: undoPattern, redo: redoPattern } = patternHistory;

  const handleShare = useCallback(
    () => replaceAddressBarWithShareUrl(drumState),
    [drumState],
  );

  const handlePlayPause = useCallback(async () => {
    if (!wasmLoaded) return;
    if (transport === "stopped" || transport === "paused") {
      await engine.play();
    } else if (transport === "playing") {
      engine.pause();
    }
  }, [transport, wasmLoaded]);

  const handleStop = useCallback(() => {
    if (!wasmLoaded || transport === "stopped") return;
    engine.stop();
  }, [transport, wasmLoaded]);

  const tapTimes = useRef<number[]>([]);
  const handleTap = useCallback(() => {
    const now = performance.now();
    const times = tapTimes.current;
    const previous = times[times.length - 1];
    if (previous !== undefined && now - previous > TAP_RESET_MS)
      times.length = 0;
    times.push(now);
    if (times.length > TAP_WINDOW) times.splice(0, times.length - TAP_WINDOW);
    if (times.length < 2) return;

    let sum = 0;
    for (let index = 1; index < times.length; index++) {
      sum += requiredAt(times, index) - requiredAt(times, index - 1);
    }
    const bpm = Math.round(
      Math.max(BPM_MIN, Math.min(BPM_MAX, 60000 / (sum / (times.length - 1)))),
    );
    applyStateAction({ type: "tempo", value: bpm });
  }, [applyStateAction]);

  const editorOpener = useRef<HTMLButtonElement | null>(null);
  const editorOpenedByMouse = useRef(false);
  const openEditor = useCallback(
    (track: number, opener: HTMLButtonElement, byMouse: boolean) => {
      editorOpener.current = opener;
      editorOpenedByMouse.current = byMouse;
      setEditorTrack(track);
    },
    [],
  );
  const closeEditor = useCallback(() => {
    setEditorTrack(null);
    if (!editorOpenedByMouse.current) editorOpener.current?.focus();
    editorOpener.current = null;
  }, []);

  const setVoiceParam = useCallback(
    (track: number, index: number, value: number) => {
      applyStateAction({ type: "voiceParam", track, index, value });
    },
    [applyStateAction],
  );
  const resetVoice = useCallback(
    (track: number) => {
      applyStateAction({
        type: "voiceParams",
        track,
        value: Float32Array.from(
          requiredAt(VOICE_PARAMS, track),
          (parameter) => parameter.default,
        ),
      });
    },
    [applyStateAction],
  );
  const setPhysicalTomParam = useCallback(
    (track: number, index: number, value: number) => {
      applyStateAction({ type: "physicalTomParam", track, index, value });
    },
    [applyStateAction],
  );
  const resetPhysicalTom = useCallback(
    (track: number) => {
      applyStateAction({
        type: "physicalTomParams",
        track,
        value: Float32Array.from(defaultPhysicalTomParams()),
      });
    },
    [applyStateAction],
  );
  const setTomModel = useCallback(
    (track: number, value: TomModel) => {
      applyStateAction({ type: "tomModel", track, value });
    },
    [applyStateAction],
  );

  const pattern = useMemo(
    () => flatToVisual(currentBank.pattern),
    [currentBank.pattern],
  );
  const flatPattern = useMemo(
    () => Array.from(currentBank.pattern),
    [currentBank.pattern],
  );

  const applyShortcutPreset = useCallback(
    (direction: 1 | -1) => {
      const current = matchingPresetIndex(currentBank.pattern);
      const next =
        current < 0
          ? direction > 0
            ? 0
            : PRESETS.length - 1
          : (current + direction + PRESETS.length) % PRESETS.length;
      const preset = PRESETS[next];
      if (!preset) return;
      patternHistory.applyDestructivePattern(presetToFlat(preset));
      setShortcutAnnouncement(`${preset.name} preset loaded.`);
    },
    [currentBank.pattern, patternHistory],
  );

  const closeShortcutHelp = useCallback(() => {
    setShortcutHelpOpen(false);
    window.requestAnimationFrame(() => shortcutHelpOpener.current?.focus());
  }, []);

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (editorTrack !== null || shortcutHelpOpen) return;
      const target =
        event.target instanceof HTMLElement ? event.target : document.body;
      const editing = Boolean(
        target.closest("input, select, textarea, [contenteditable='true']"),
      );
      const rowActionsOpen = document.querySelector(".dm-row-actions") !== null;
      if (rowActionsOpen) return;

      if ((event.ctrlKey || event.metaKey) && !event.altKey && !editing) {
        const undo = event.code === "KeyZ" && !event.shiftKey;
        const redo =
          (event.code === "KeyZ" && event.shiftKey) || event.code === "KeyY";
        if (undo || redo) {
          event.preventDefault();
          if (undo) undoPattern();
          else redoPattern();
          return;
        }
      }

      if (editing) return;
      const shortcut = resolveGlobalShortcut(event);
      if (!shortcut) return;
      if (
        shortcut.type === "playPause" &&
        target.closest("button, [role='slider']")
      ) {
        return;
      }

      event.preventDefault();
      switch (shortcut.type) {
        case "playPause":
          void handlePlayPause();
          return;
        case "stop":
          handleStop();
          return;
        case "mutate":
          patternHistory.applyDestructivePattern(
            mutate(Array.from(currentBank.pattern), {
              stepCount: currentBank.stepCount,
            }),
          );
          setShortcutAnnouncement("Pattern mutated.");
          return;
        case "clear":
          patternHistory.applyDestructivePattern(
            new Float32Array(currentBank.pattern.length),
          );
          setShortcutAnnouncement("Pattern cleared.");
          return;
        case "muteTrack": {
          const track = TRACK_INDEX[shortcut.visualRow];
          if (track === undefined) return;
          const muted = !drumState.tracks[track].muted;
          applyStateAction({ type: "muted", track, value: muted });
          setShortcutAnnouncement(
            `${muted ? "Muted" : "Unmuted"} track ${shortcut.visualRow + 1}.`,
          );
          return;
        }
        case "nextPreset":
          applyShortcutPreset(1);
          return;
        case "previousPreset":
          applyShortcutPreset(-1);
          return;
        case "showHelp":
          shortcutHelpOpener.current = target;
          setShortcutHelpOpen(true);
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => {
      window.removeEventListener("keydown", onKeyDown);
    };
  }, [
    applyShortcutPreset,
    applyStateAction,
    currentBank.pattern,
    currentBank.stepCount,
    drumState.tracks,
    editorTrack,
    handlePlayPause,
    handleStop,
    patternHistory,
    redoPattern,
    shortcutHelpOpen,
    undoPattern,
  ]);
  const volumes = TRACK_INDEX.map((track) => drumState.tracks[track].volume);
  const decays = TRACK_INDEX.map((track) => drumState.tracks[track].decay);
  const muted = TRACK_INDEX.map((track) => drumState.tracks[track].muted);
  const trackLengths = TRACK_INDEX.map(
    (track) => currentBank.trackLengths[track] ?? STEP_CAPACITY,
  );

  const tomModel = drumState.tracks[TOM_TRACK].tom.model;
  const tom2Model = drumState.tracks[TOM2_TRACK].tom.model;
  const editorTomModel =
    editorTrack === TOM_TRACK
      ? tomModel
      : editorTrack === TOM2_TRACK
        ? tom2Model
        : undefined;
  const editorUsesPhysical = editorTomModel === "physical";
  const physicalTomParams = drumState.tracks[TOM_TRACK].tom.physicalParams;
  const physicalTom2Params = drumState.tracks[TOM2_TRACK].tom.physicalParams;
  const editorPhysicalParams =
    editorTrack === TOM2_TRACK ? physicalTom2Params : physicalTomParams;

  return (
    <div className="dm-machine">
      <a className="dm-skip-link" href="#dm-transport">
        Skip to transport
      </a>
      <span className="dm-sr-only" role="status" aria-live="polite">
        {wasmLoaded
          ? `Audio engine ready. Transport ${transport}.`
          : "Audio engine loading."}
      </span>
      <span className="dm-sr-only" role="status" aria-live="polite">
        {shortcutAnnouncement}
      </span>
      <header className="dm-header">
        <h1 className="dm-title">
          <span className="dm-title-algo">algo</span>
          <span className="dm-title-drum">-drum</span>
        </h1>
        <span
          className={`dm-status ${wasmLoaded ? "dm-status-ready" : ""}`}
          title={wasmLoaded ? "Engine ready" : "Loading engine"}
          aria-hidden="true"
        />
        <button
          type="button"
          className="dm-shortcuts-open"
          aria-label="Show keyboard shortcuts"
          aria-haspopup="dialog"
          aria-keyshortcuts="?"
          title="Keyboard shortcuts (?)"
          onClick={(event) => {
            shortcutHelpOpener.current = event.currentTarget;
            setShortcutHelpOpen(true);
          }}
        >
          ?
        </button>
        <AlgoPanel
          disabled={!wasmLoaded}
          pattern={flatPattern}
          stepCount={currentBank.stepCount}
          onApplyPattern={patternHistory.applyDestructivePattern}
          canUndo={patternHistory.canUndo}
          canRedo={patternHistory.canRedo}
          onUndo={patternHistory.undo}
          onRedo={patternHistory.redo}
          onShare={handleShare}
        />
      </header>

      <PatternBanks
        disabled={!wasmLoaded}
        transport={transport}
        selectedBank={selectedBank}
        activeBank={bankPlayback.activeBank}
        queuedBank={bankPlayback.queuedBank}
        chainPosition={bankPlayback.chainPosition}
        chainEnabled={drumState.chainEnabled}
        chain={drumState.chain}
        onSelectBank={(bank) => {
          setSelectedCell(null);
          setSelectedBank(bank);
          if (!drumState.chainEnabled) {
            applyStateAction({ type: "requestBank", value: bank });
          }
        }}
        onCopyBank={(destination) => {
          patternHistory.copyBank(selectedBank, destination);
        }}
        onChainEnabledChange={(value) => {
          applyStateAction({ type: "chainEnabled", value });
          if (value) {
            setSelectedCell(null);
            setSelectedBank(drumState.chain[0] ?? 0);
          }
        }}
        onChainChange={(value) => {
          applyStateAction({ type: "chain", value });
        }}
      />

      <StepGrid
        disabled={!wasmLoaded}
        showPlayhead={selectedBank === bankPlayback.activeBank}
        pattern={pattern}
        stepCount={currentBank.stepCount}
        volumes={volumes}
        decays={decays}
        muted={muted}
        trackLengths={trackLengths}
        onCellChange={(track, step, value) => {
          applyStateAction({
            type: "cell",
            bank: selectedBank,
            track,
            step,
            value,
          });
        }}
        onPaintComplete={(cells) => {
          patternHistory.applyDestructivePattern(
            applyPaintedCells(currentBank.pattern, cells),
          );
        }}
        onClearRow={(row) => {
          patternHistory.replaceBank(
            selectedBank,
            clearRow(currentBank, requiredAt(TRACK_INDEX, row)),
          );
        }}
        onRotateRow={(row, direction) => {
          patternHistory.replaceBank(
            selectedBank,
            rotateRow(currentBank, requiredAt(TRACK_INDEX, row), direction),
          );
        }}
        onCopyRow={(sourceRow, destinationRow) => {
          patternHistory.replaceBank(
            selectedBank,
            copyRow(
              currentBank,
              requiredAt(TRACK_INDEX, sourceRow),
              requiredAt(TRACK_INDEX, destinationRow),
            ),
          );
        }}
        onCopyBar={(row, sourceBar, destinationBar) => {
          patternHistory.replaceBank(
            selectedBank,
            copyBar(
              currentBank,
              requiredAt(TRACK_INDEX, row),
              sourceBar,
              destinationBar,
            ),
          );
        }}
        onVolumeChange={(row, value) => {
          applyStateAction({
            type: "volume",
            track: requiredAt(TRACK_INDEX, row),
            value,
          });
        }}
        onDecayChange={(row, value) => {
          applyStateAction({
            type: "decay",
            track: requiredAt(TRACK_INDEX, row),
            value,
          });
        }}
        onToggleMute={(row) => {
          const track = requiredAt(TRACK_INDEX, row);
          applyStateAction({
            type: "muted",
            track,
            value: !drumState.tracks[track].muted,
          });
        }}
        onTrackLengthChange={(row, value) => {
          applyStateAction({
            type: "trackLength",
            bank: selectedBank,
            track: requiredAt(TRACK_INDEX, row),
            value,
          });
        }}
        onInspectCell={(track, step) => {
          setSelectedCell({ track, step });
        }}
        onOpenEditor={openEditor}
      />

      {selectedCell && (
        <CellInspector
          track={selectedCell.track}
          step={selectedCell.step}
          probability={
            currentBank.cellProbabilities[
              cellIndex(selectedCell.track, selectedCell.step)
            ] ?? 1
          }
          humanize={
            currentBank.cellHumanize[
              cellIndex(selectedCell.track, selectedCell.step)
            ] ?? 0
          }
          condition={
            currentBank.cellConditions[
              cellIndex(selectedCell.track, selectedCell.step)
            ] ?? 0
          }
          repeats={
            currentBank.cellRepeats[
              cellIndex(selectedCell.track, selectedCell.step)
            ] ?? 1
          }
          onProbabilityChange={(value) => {
            applyStateAction({
              type: "cellProbability",
              bank: selectedBank,
              track: selectedCell.track,
              step: selectedCell.step,
              value,
            });
          }}
          onHumanizeChange={(value) => {
            applyStateAction({
              type: "cellHumanize",
              bank: selectedBank,
              track: selectedCell.track,
              step: selectedCell.step,
              value,
            });
          }}
          onConditionChange={(value) => {
            applyStateAction({
              type: "cellCondition",
              bank: selectedBank,
              track: selectedCell.track,
              step: selectedCell.step,
              value,
            });
          }}
          onRepeatsChange={(value) => {
            applyStateAction({
              type: "cellRepeats",
              bank: selectedBank,
              track: selectedCell.track,
              step: selectedCell.step,
              value,
            });
          }}
          onClose={() => {
            setSelectedCell(null);
          }}
        />
      )}

      {editorTrack !== null && (
        <VoiceEditor
          name={requiredAt(VOICE_NAMES, editorTrack)}
          specs={
            editorUsesPhysical
              ? PHYSICAL_TOM_PARAMS
              : requiredAt(VOICE_PARAMS, editorTrack)
          }
          values={
            editorUsesPhysical
              ? editorPhysicalParams
              : requiredAt(drumState.tracks, editorTrack).voiceParams
          }
          disabled={!wasmLoaded}
          model={editorTomModel}
          onModelChange={
            editorTrack === TOM_TRACK || editorTrack === TOM2_TRACK
              ? (model) => {
                  setTomModel(editorTrack, model);
                }
              : undefined
          }
          onChange={(index, value) => {
            if (editorUsesPhysical) {
              setPhysicalTomParam(editorTrack, index, value);
            } else {
              setVoiceParam(editorTrack, index, value);
            }
          }}
          onReset={() => {
            if (editorUsesPhysical) {
              resetPhysicalTom(editorTrack);
            } else {
              resetVoice(editorTrack);
            }
          }}
          onAudition={(amount) => void engine.triggerVoice(editorTrack, amount)}
          onRequestClose={closeEditor}
        />
      )}

      {shortcutHelpOpen && <ShortcutHelp onRequestClose={closeShortcutHelp} />}

      <Transport
        disabled={!wasmLoaded}
        transport={transport}
        bpm={drumState.tempoBpm}
        swing={drumState.swing}
        bank={selectedBank}
        stepCount={currentBank.stepCount}
        probability={drumState.probability}
        humanize={drumState.humanize}
        reverb={drumState.reverb}
        fillMode={drumState.fillMode}
        metronomeEnabled={drumState.metronomeEnabled}
        countInBars={drumState.countInBars}
        onAction={applyStateAction}
        onPlayPause={() => void handlePlayPause()}
        onStop={handleStop}
        onTap={handleTap}
      />
    </div>
  );
}
