import { useEffect, useRef, useState, type CSSProperties } from "react";
import { STEP_CAPACITY, VEL_ACCENT } from "../algo/pattern";
import { onStep } from "../engine/wasmEngine";
import TrackStrip from "./TrackStrip";
import {
  TRACK_INDEX,
  TRACKS,
  STOPPED_PLAYHEAD,
  advancePlayheadClock,
  cycleVelocity,
  nudgeVelocity,
  velocityFromPointer,
  velocityName,
} from "./patternView";

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

  const setPointerVelocity = (
    element: HTMLButtonElement,
    clientY: number,
    row: number,
    col: number,
  ) => {
    const rect = element.getBoundingClientRect();
    onCellChange(
      TRACK_INDEX[row],
      col,
      velocityFromPointer(clientY, rect.top, rect.height),
    );
  };

  return (
    <div className="dm-board">
      <p id="dm-velocity-help" className="dm-sr-only">
        Click to cycle off, normal and accent. Hold Shift and drag vertically
        for continuous velocity, or press Shift plus Up or Down Arrow in five
        percent steps. Open the context menu or press F2 for probability,
        humanize and trigger condition.
      </p>
      <div className="dm-screen" aria-hidden="true" />

      {TRACKS.map((name, row) => (
        <span
          key={name}
          className="dm-track-label"
          style={{ gridRow: row + 1, gridColumn: 1 }}
          id={`dm-track-${name}`}
        >
          {name.toUpperCase()}
        </span>
      ))}

      {TRACKS.map((name, row) =>
        Array.from({ length: STEP_CAPACITY }, (_, col) => {
          const velocity = pattern[row][col];
          return (
            <button
              key={`${name}-${col}`}
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
              data-accent={velocity >= VEL_ACCENT || undefined}
              data-beyond={col >= trackLengths[row] || undefined}
              data-playhead={
                showPlayhead &&
                playhead.clockStep >= 0 &&
                col === playhead.clockStep % trackLengths[row]
                  ? true
                  : undefined
              }
              data-bar-start={col % 4 === 0 || undefined}
              data-bar-odd={Math.floor(col / 4) % 2 === 1 || undefined}
              aria-pressed={velocity > 0}
              aria-label={`${name} step ${col + 1}: ${velocityName(velocity)}`}
              aria-describedby="dm-velocity-help"
              aria-keyshortcuts="Shift+ArrowUp Shift+ArrowDown F2"
              title={`${velocityName(velocity)} velocity · Shift-drag vertically to set precisely · F2 for trigger settings`}
              onContextMenu={(event) => {
                event.preventDefault();
                onInspectCell(TRACK_INDEX[row], col);
              }}
              onPointerDown={(event) => {
                if (!event.shiftKey) return;
                event.preventDefault();
                velocityPointer.current = event.pointerId;
                suppressClick.current = true;
                event.currentTarget.setPointerCapture?.(event.pointerId);
                event.currentTarget.focus();
                setPointerVelocity(
                  event.currentTarget,
                  event.clientY,
                  row,
                  col,
                );
              }}
              onPointerMove={(event) => {
                if (velocityPointer.current !== event.pointerId) return;
                setPointerVelocity(
                  event.currentTarget,
                  event.clientY,
                  row,
                  col,
                );
              }}
              onPointerUp={(event) => {
                if (velocityPointer.current !== event.pointerId) return;
                event.currentTarget.releasePointerCapture?.(event.pointerId);
                velocityPointer.current = null;
                window.setTimeout(() => {
                  suppressClick.current = false;
                }, 0);
              }}
              onPointerCancel={() => {
                velocityPointer.current = null;
                suppressClick.current = false;
              }}
              onKeyDown={(event) => {
                if (event.key === "F2") {
                  event.preventDefault();
                  onInspectCell(TRACK_INDEX[row], col);
                  return;
                }
                if (!event.shiftKey) return;
                const direction =
                  event.key === "ArrowUp"
                    ? 1
                    : event.key === "ArrowDown"
                      ? -1
                      : null;
                if (direction === null) return;
                event.preventDefault();
                onCellChange(
                  TRACK_INDEX[row],
                  col,
                  nudgeVelocity(velocity, direction),
                );
              }}
              onClick={(event) => {
                if (suppressClick.current) {
                  suppressClick.current = false;
                  return;
                }
                if (event.detail > 0) event.currentTarget.blur();
                onCellChange(TRACK_INDEX[row], col, cycleVelocity(velocity));
              }}
            >
              <span className="dm-led" aria-hidden="true" />
            </button>
          );
        }),
      )}

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
          volume={volumes[row]}
          decay={decays[row]}
          muted={muted[row]}
          trackLength={trackLengths[row]}
          onVolumeChange={(value) => onVolumeChange(row, value)}
          onDecayChange={(value) => onDecayChange(row, value)}
          onToggleMute={() => onToggleMute(row)}
          onTrackLengthChange={(value) => onTrackLengthChange(row, value)}
          onOpenEditor={onOpenEditor}
        />
      ))}
    </div>
  );
}
