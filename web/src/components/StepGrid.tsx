import {
  useEffect,
  useRef,
  useState,
  type CSSProperties,
  type PointerEvent as ReactPointerEvent,
} from "react";
import { createPortal } from "react-dom";
import { STEP_CAPACITY, VEL_ACCENT } from "../algo/pattern";
import { onStep } from "../engine/wasmEngine";
import TrackStrip from "./TrackStrip";
import {
  TRACK_INDEX,
  TRACKS,
  STOPPED_PLAYHEAD,
  advancePlayheadClock,
  cycleVelocity,
  moveGridFocus,
  nudgeVelocity,
  paintVelocity,
  type PaintedCell,
  velocityFromPointer,
  velocityName,
} from "./patternView";

const BAR_COUNT = STEP_CAPACITY / 4;

interface Props {
  disabled: boolean;
  showPlayhead: boolean;
  pattern: number[][];
  stepCount: number;
  volumes: number[];
  decays: number[];
  muted: boolean[];
  trackLengths: number[];
  onCellChange: (track: number, step: number, value: number) => void;
  onPaintComplete: (cells: readonly PaintedCell[]) => void;
  onClearRow: (row: number) => void;
  onRotateRow: (row: number, direction: "left" | "right") => void;
  onCopyRow: (sourceRow: number, destinationRow: number) => void;
  onCopyBar: (row: number, sourceBar: number, destinationBar: number) => void;
  onVolumeChange: (row: number, value: number) => void;
  onDecayChange: (row: number, value: number) => void;
  onToggleMute: (row: number) => void;
  onTrackLengthChange: (row: number, value: number) => void;
  onInspectCell: (track: number, step: number) => void;
  onOpenEditor: (
    track: number,
    opener: HTMLButtonElement,
    byMouse: boolean,
  ) => void;
}

function requiredTrack(row: number): number {
  const track = TRACK_INDEX[row];
  if (track === undefined) {
    throw new RangeError(`Missing track for row ${row}`);
  }
  return track;
}

export default function StepGrid({
  disabled,
  showPlayhead,
  pattern,
  stepCount,
  volumes,
  decays,
  muted,
  trackLengths,
  onCellChange,
  onPaintComplete,
  onClearRow,
  onRotateRow,
  onCopyRow,
  onCopyBar,
  onVolumeChange,
  onDecayChange,
  onToggleMute,
  onTrackLengthChange,
  onInspectCell,
  onOpenEditor,
}: Props) {
  // Keeping the audible playhead here prevents its ~8 Hz updates from
  // re-rendering the machine header, algorithm panel, transport and editor.
  const [playhead, setPlayhead] = useState(STOPPED_PLAYHEAD);
  useEffect(
    () =>
      onStep((masterStep) => {
        setPlayhead((previous) =>
          advancePlayheadClock(previous, masterStep, stepCount),
        );
      }),
    [stepCount],
  );
  const velocityPointer = useRef<number | null>(null);
  const suppressClick = useRef(false);
  const cellRefs = useRef<Array<HTMLButtonElement | null>>([]);
  const [tabStop, setTabStop] = useState({ row: 0, col: 0 });
  const [paintPreview, setPaintPreview] = useState<Map<number, number> | null>(
    null,
  );
  const paintGesture = useRef<{
    pointerId: number;
    value: number;
    startIndex: number;
    start: PaintedCell;
    visited: Map<number, PaintedCell>;
    active: boolean;
  } | null>(null);
  const [announcement, setAnnouncement] = useState("");
  const [rowActions, setRowActions] = useState<{
    row: number;
    top: number;
    left: number;
  } | null>(null);
  const [copyDestinationRow, setCopyDestinationRow] = useState(1);
  const [sourceBar, setSourceBar] = useState(0);
  const [destinationBar, setDestinationBar] = useState(1);
  const rowActionsRef = useRef<HTMLDivElement | null>(null);
  const rowActionOpener = useRef<HTMLButtonElement | null>(null);

  useEffect(() => {
    if (!rowActions) return;
    rowActionsRef.current?.querySelector<HTMLButtonElement>("button")?.focus();

    const closeForOutsidePointer = (event: PointerEvent) => {
      const target = event.target as Node | null;
      if (
        target &&
        !rowActionsRef.current?.contains(target) &&
        !rowActionOpener.current?.contains(target)
      ) {
        setRowActions(null);
      }
    };
    const closeForEscape = (event: KeyboardEvent) => {
      if (event.key !== "Escape") return;
      event.preventDefault();
      setRowActions(null);
      rowActionOpener.current?.focus();
    };
    document.addEventListener("pointerdown", closeForOutsidePointer);
    document.addEventListener("keydown", closeForEscape);
    return () => {
      document.removeEventListener("pointerdown", closeForOutsidePointer);
      document.removeEventListener("keydown", closeForEscape);
    };
  }, [rowActions]);

  const setPointerVelocity = (
    element: HTMLButtonElement,
    clientY: number,
    row: number,
    col: number,
  ) => {
    const rect = element.getBoundingClientRect();
    const track = TRACK_INDEX[row];
    if (track === undefined) return;
    onCellChange(
      track,
      col,
      velocityFromPointer(clientY, rect.top, rect.height),
    );
  };

  const beginPaint = (
    event: ReactPointerEvent<HTMLButtonElement>,
    row: number,
    col: number,
    velocity: number,
  ) => {
    if (
      event.shiftKey ||
      !event.isPrimary ||
      event.button !== 0 ||
      (event.pointerType !== "mouse" && event.pointerType !== "pen")
    ) {
      return;
    }
    const track = TRACK_INDEX[row];
    if (track === undefined) return;
    const startIndex = row * STEP_CAPACITY + col;
    paintGesture.current = {
      pointerId: event.pointerId,
      value: paintVelocity(velocity),
      startIndex,
      start: { track, step: col, value: paintVelocity(velocity) },
      visited: new Map(),
      active: false,
    };
    event.currentTarget.setPointerCapture(event.pointerId);
  };

  const continuePaint = (event: ReactPointerEvent<HTMLButtonElement>) => {
    const gesture = paintGesture.current;
    if (!gesture || gesture.pointerId !== event.pointerId) return;
    const target = document
      .elementFromPoint(event.clientX, event.clientY)
      ?.closest<HTMLButtonElement>(".dm-cell[data-row][data-col]");
    if (!target) return;
    const row = Number(target.dataset.row);
    const col = Number(target.dataset.col);
    if (!Number.isInteger(row) || !Number.isInteger(col)) return;
    const targetIndex = row * STEP_CAPACITY + col;
    if (!gesture.active && targetIndex === gesture.startIndex) return;

    const track = TRACK_INDEX[row];
    if (track === undefined || gesture.visited.has(targetIndex)) return;
    event.preventDefault();
    if (!gesture.active) {
      gesture.active = true;
      gesture.visited.set(gesture.startIndex, gesture.start);
      suppressClick.current = true;
    }
    gesture.visited.set(targetIndex, {
      track,
      step: col,
      value: gesture.value,
    });
    setPaintPreview(
      new Map(
        Array.from(gesture.visited, ([index, cell]) => [index, cell.value]),
      ),
    );
  };

  const finishPaint = (pointerId: number, commit: boolean) => {
    const gesture = paintGesture.current;
    if (!gesture || gesture.pointerId !== pointerId) return;
    paintGesture.current = null;
    setPaintPreview(null);
    if (commit && gesture.active) {
      onPaintComplete(Array.from(gesture.visited.values()));
      setAnnouncement(
        `${gesture.value > 0 ? "Painted" : "Erased"} ${gesture.visited.size} steps.`,
      );
    }
    if (gesture.active) {
      suppressClick.current = true;
      window.setTimeout(() => {
        suppressClick.current = false;
      }, 0);
    }
  };

  const openRowActions = (
    event:
      | ReactPointerEvent<HTMLButtonElement>
      | React.MouseEvent<HTMLButtonElement>,
    row: number,
  ) => {
    const rect = event.currentTarget.getBoundingClientRect();
    const panelWidth = Math.min(360, window.innerWidth - 16);
    setCopyDestinationRow((row + 1) % TRACKS.length);
    setSourceBar(0);
    setDestinationBar(1);
    rowActionOpener.current = event.currentTarget;
    setRowActions({
      row,
      top: Math.min(rect.bottom + 6, Math.max(8, window.innerHeight - 330)),
      left: Math.max(
        8,
        Math.min(rect.left, window.innerWidth - panelWidth - 8),
      ),
    });
  };

  const finishRowAction = (message: string, action: () => void) => {
    action();
    setAnnouncement(message);
    setRowActions(null);
    rowActionOpener.current?.focus();
  };

  return (
    <div className="dm-board">
      <p id="dm-velocity-help" className="dm-sr-only">
        Click to cycle off, normal and accent. Drag across two or more steps
        with a mouse or pen to paint from an empty step or erase from an active
        step. Hold Shift and drag vertically for continuous velocity, or press
        Shift plus Up or Down Arrow in five percent steps. Open the context menu
        or press F2 for probability, humanize and trigger condition. Arrow keys
        move between cells. Home and End move to the first and last cell in a
        row; Control plus Home or End moves to the first or last cell in the
        grid. Steps outside a track's loop remain editable but do not play.
      </p>
      <div className="dm-screen" aria-hidden="true" />

      <div
        role="grid"
        aria-label="Drum pattern"
        aria-describedby="dm-velocity-help"
        aria-rowcount={TRACKS.length}
        aria-colcount={STEP_CAPACITY + 1}
        style={{ display: "contents" }}
      >
        {TRACKS.map((name, row) => (
          <div
            key={name}
            role="row"
            aria-rowindex={row + 1}
            style={{ display: "contents" }}
          >
            <span
              className="dm-track-label"
              style={{ gridRow: row + 1, gridColumn: 1 }}
              role="rowheader"
              aria-colindex={1}
            >
              <span>{name.toUpperCase()}</span>
              <button
                type="button"
                className="dm-row-actions-open"
                aria-label={`${name} row actions`}
                aria-haspopup="dialog"
                aria-expanded={rowActions?.row === row}
                disabled={disabled}
                onClick={(event) => {
                  if (rowActions?.row === row) {
                    setRowActions(null);
                    return;
                  }
                  openRowActions(event, row);
                }}
              >
                ⋯
              </button>
            </span>

            {Array.from({ length: STEP_CAPACITY }, (_, col) => {
              const track = requiredTrack(row);
              const trackLength = trackLengths[row] ?? stepCount;
              const refIndex = row * STEP_CAPACITY + col;
              const velocity =
                paintPreview?.get(refIndex) ?? pattern[row]?.[col] ?? 0;
              const beyondLoop = col >= trackLength;
              const isPlayhead =
                showPlayhead &&
                playhead.clockStep >= 0 &&
                col === playhead.clockStep % trackLength;
              const stateId = `dm-cell-state-${row}-${col}`;
              const stateDescription = [
                `${velocityName(velocity)} velocity.`,
                isPlayhead ? "Current playhead." : "",
                beyondLoop
                  ? `Outside this track's ${trackLength}-step loop; this step will not play.`
                  : "",
              ]
                .filter(Boolean)
                .join(" ");

              return (
                <span
                  key={`${name}-${col}`}
                  role="gridcell"
                  aria-colindex={col + 2}
                  style={{ display: "contents" }}
                >
                  <button
                    ref={(element) => {
                      cellRefs.current[refIndex] = element;
                    }}
                    type="button"
                    className="dm-cell"
                    style={
                      {
                        gridRow: row + 1,
                        gridColumn: col + 2,
                        "--dm-velocity": velocity,
                      } as CSSProperties
                    }
                    data-active={velocity > 0 || undefined}
                    data-row={row}
                    data-col={col}
                    data-accent={velocity >= VEL_ACCENT || undefined}
                    data-beyond={beyondLoop || undefined}
                    data-playhead={isPlayhead || undefined}
                    data-bar-start={col % 4 === 0 || undefined}
                    data-first-step={col === 0 || undefined}
                    data-bar-odd={Math.floor(col / 4) % 2 === 1 || undefined}
                    aria-pressed={
                      velocity >= VEL_ACCENT ? "mixed" : velocity > 0
                    }
                    aria-current={isPlayhead ? "step" : undefined}
                    aria-label={`${name} step ${col + 1}`}
                    aria-describedby={`${stateId} dm-velocity-help`}
                    aria-keyshortcuts="ArrowLeft ArrowRight ArrowUp ArrowDown Home End Control+Home Control+End Shift+ArrowUp Shift+ArrowDown F2"
                    tabIndex={
                      tabStop.row === row && tabStop.col === col ? 0 : -1
                    }
                    title={`${velocityName(velocity)} velocity · Shift-drag vertically to set precisely · F2 for trigger settings`}
                    onFocus={() => {
                      setTabStop({ row, col });
                    }}
                    onContextMenu={(event) => {
                      event.preventDefault();
                      onInspectCell(track, col);
                    }}
                    onPointerDown={(event) => {
                      if (event.shiftKey) {
                        event.preventDefault();
                        velocityPointer.current = event.pointerId;
                        suppressClick.current = true;
                        event.currentTarget.setPointerCapture(event.pointerId);
                        event.currentTarget.focus();
                        setPointerVelocity(
                          event.currentTarget,
                          event.clientY,
                          row,
                          col,
                        );
                        return;
                      }
                      beginPaint(event, row, col, velocity);
                    }}
                    onPointerMove={(event) => {
                      if (velocityPointer.current === event.pointerId) {
                        setPointerVelocity(
                          event.currentTarget,
                          event.clientY,
                          row,
                          col,
                        );
                        return;
                      }
                      continuePaint(event);
                    }}
                    onPointerUp={(event) => {
                      if (velocityPointer.current === event.pointerId) {
                        event.currentTarget.releasePointerCapture(
                          event.pointerId,
                        );
                        velocityPointer.current = null;
                        window.setTimeout(() => {
                          suppressClick.current = false;
                        }, 0);
                        return;
                      }
                      finishPaint(event.pointerId, true);
                    }}
                    onPointerCancel={(event) => {
                      if (velocityPointer.current === event.pointerId) {
                        velocityPointer.current = null;
                        suppressClick.current = false;
                        return;
                      }
                      finishPaint(event.pointerId, true);
                    }}
                    onKeyDown={(event) => {
                      if (event.key === "F2") {
                        event.preventDefault();
                        onInspectCell(track, col);
                        return;
                      }
                      if (
                        event.shiftKey &&
                        (event.key === "ArrowUp" || event.key === "ArrowDown")
                      ) {
                        event.preventDefault();
                        onCellChange(
                          track,
                          col,
                          nudgeVelocity(
                            velocity,
                            event.key === "ArrowUp" ? 1 : -1,
                          ),
                        );
                        return;
                      }

                      const next = moveGridFocus(
                        { row, col },
                        event.key,
                        event.ctrlKey || event.metaKey,
                      );
                      if (next === null) return;
                      event.preventDefault();
                      setTabStop(next);
                      cellRefs.current[
                        next.row * STEP_CAPACITY + next.col
                      ]?.focus();
                    }}
                    onClick={(event) => {
                      if (suppressClick.current) {
                        suppressClick.current = false;
                        return;
                      }
                      if (event.detail > 0) event.currentTarget.blur();
                      onCellChange(track, col, cycleVelocity(velocity));
                    }}
                  >
                    <span id={stateId} className="dm-sr-only">
                      {stateDescription}
                    </span>
                    <span className="dm-led" aria-hidden="true" />
                  </button>
                </span>
              );
            })}
          </div>
        ))}
      </div>

      {Array.from({ length: STEP_CAPACITY }, (_, col) => (
        <span
          key={col}
          className="dm-step-number"
          style={{ gridRow: TRACKS.length + 1, gridColumn: col + 2 }}
          data-playhead={
            showPlayhead && col === playhead.masterStep ? true : undefined
          }
          data-beyond={col >= stepCount || undefined}
          aria-hidden="true"
        >
          {col + 1}
        </span>
      ))}

      {TRACKS.map((name, row) => (
        <TrackStrip
          key={name}
          name={name}
          row={row}
          disabled={disabled}
          volume={volumes[row] ?? 0.75}
          decay={decays[row] ?? 0.5}
          muted={muted[row] ?? false}
          trackLength={trackLengths[row] ?? stepCount}
          onVolumeChange={(value) => {
            onVolumeChange(row, value);
          }}
          onDecayChange={(value) => {
            onDecayChange(row, value);
          }}
          onToggleMute={() => {
            onToggleMute(row);
          }}
          onTrackLengthChange={(value) => {
            onTrackLengthChange(row, value);
          }}
          onOpenEditor={onOpenEditor}
        />
      ))}

      <span className="dm-sr-only" role="status" aria-live="polite">
        {announcement}
      </span>

      {rowActions &&
        createPortal(
          <div
            ref={rowActionsRef}
            className="dm-row-actions"
            role="dialog"
            aria-modal="false"
            aria-label={`${TRACKS[rowActions.row]} row actions`}
            style={{ top: rowActions.top, left: rowActions.left }}
          >
            <div className="dm-row-actions-title">
              {TRACKS[rowActions.row]} row
            </div>
            <div className="dm-row-actions-buttons">
              <button
                type="button"
                onClick={() => {
                  finishRowAction(
                    `${TRACKS[rowActions.row]} row cleared.`,
                    () => {
                      onClearRow(rowActions.row);
                    },
                  );
                }}
              >
                Clear row
              </button>
              <button
                type="button"
                onClick={() => {
                  finishRowAction(
                    `${TRACKS[rowActions.row]} row rotated left.`,
                    () => {
                      onRotateRow(rowActions.row, "left");
                    },
                  );
                }}
              >
                Rotate left
              </button>
              <button
                type="button"
                onClick={() => {
                  finishRowAction(
                    `${TRACKS[rowActions.row]} row rotated right.`,
                    () => {
                      onRotateRow(rowActions.row, "right");
                    },
                  );
                }}
              >
                Rotate right
              </button>
            </div>
            <div className="dm-row-copy">
              <label>
                Copy row to
                <select
                  value={copyDestinationRow}
                  onChange={(event) => {
                    setCopyDestinationRow(Number(event.target.value));
                  }}
                >
                  {TRACKS.map((trackName, destinationRow) => (
                    <option
                      key={trackName}
                      value={destinationRow}
                      disabled={destinationRow === rowActions.row}
                    >
                      {trackName}
                    </option>
                  ))}
                </select>
              </label>
              <button
                type="button"
                disabled={copyDestinationRow === rowActions.row}
                onClick={() => {
                  finishRowAction(
                    `${TRACKS[rowActions.row]} row copied to ${TRACKS[copyDestinationRow]}.`,
                    () => {
                      onCopyRow(rowActions.row, copyDestinationRow);
                    },
                  );
                }}
              >
                Copy row
              </button>
            </div>
            <div className="dm-row-copy">
              <label>
                Copy bar
                <select
                  aria-label="Source bar"
                  value={sourceBar}
                  onChange={(event) => {
                    setSourceBar(Number(event.target.value));
                  }}
                >
                  {Array.from({ length: BAR_COUNT }, (_, bar) => (
                    <option key={bar} value={bar}>
                      {bar + 1}
                    </option>
                  ))}
                </select>
              </label>
              <label>
                to
                <select
                  aria-label="Destination bar"
                  value={destinationBar}
                  onChange={(event) => {
                    setDestinationBar(Number(event.target.value));
                  }}
                >
                  {Array.from({ length: BAR_COUNT }, (_, bar) => (
                    <option key={bar} value={bar} disabled={bar === sourceBar}>
                      {bar + 1}
                    </option>
                  ))}
                </select>
              </label>
              <button
                type="button"
                disabled={sourceBar === destinationBar}
                onClick={() => {
                  finishRowAction(
                    `${TRACKS[rowActions.row]} bar ${sourceBar + 1} copied to bar ${destinationBar + 1}.`,
                    () => {
                      onCopyBar(rowActions.row, sourceBar, destinationBar);
                    },
                  );
                }}
              >
                Copy bar
              </button>
            </div>
          </div>,
          document.body,
        )}
    </div>
  );
}
