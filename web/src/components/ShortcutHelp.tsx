import { useEffect, useId, useRef } from "react";
import "./ShortcutHelp.css";

/* eslint-disable jsx-a11y/no-noninteractive-element-interactions, jsx-a11y/click-events-have-key-events -- The native
   modal dialog owns Escape and backdrop-pointer arbitration, matching the
   established VoiceEditor dialog behavior. */

interface Props {
  onRequestClose: () => void;
}

const GLOBAL_SHORTCUTS = [
  ["?", "Show keyboard shortcuts"],
  ["Space", "Play or pause"],
  ["S", "Stop and return to step 1"],
  ["M", "Mutate the selected bank"],
  ["Shift + Backspace", "Clear the selected bank"],
  ["1–7", "Mute or unmute a track, top to bottom"],
  ["P / Shift + P", "Next or previous preset"],
  ["Ctrl/Cmd + Z", "Undo pattern action"],
  ["Ctrl/Cmd + Shift + Z", "Redo pattern action"],
] as const;

const GRID_SHORTCUTS = [
  ["Arrow keys", "Move between pattern cells"],
  ["Home / End", "First or last cell in the row"],
  ["Ctrl/Cmd + Home / End", "First or last cell in the grid"],
  ["Shift + Up / Down", "Adjust velocity by five percent"],
  ["F2", "Edit probability, humanize and trigger rules"],
] as const;

const KNOB_SHORTCUTS = [
  ["Arrow keys", "Fine adjustment"],
  ["Page Up / Down", "Coarse adjustment"],
  ["Home / End", "Minimum or maximum"],
  ["Escape / double click", "Restore the default"],
] as const;

function ShortcutList({
  title,
  rows,
}: {
  title: string;
  rows: readonly (readonly [string, string])[];
}) {
  return (
    <section className="dm-shortcuts-section">
      <h3>{title}</h3>
      <dl>
        {rows.map(([keys, description]) => (
          <div key={keys}>
            <dt>
              <kbd>{keys}</kbd>
            </dt>
            <dd>{description}</dd>
          </div>
        ))}
      </dl>
    </section>
  );
}

export default function ShortcutHelp({ onRequestClose }: Props) {
  const dialog = useRef<HTMLDialogElement>(null);
  const closeButton = useRef<HTMLButtonElement>(null);
  const titleId = useId();
  const pressedBackdrop = useRef(false);

  useEffect(() => {
    const element = dialog.current;
    if (!element || element.open) return;

    element.showModal();
    closeButton.current?.focus();

    return () => {
      if (element.open) element.close();
    };
  }, []);

  return (
    <dialog
      ref={dialog}
      className="dm-shortcuts"
      aria-labelledby={titleId}
      onCancel={(event) => {
        event.preventDefault();
        onRequestClose();
      }}
      onClose={onRequestClose}
      onPointerDown={(event) => {
        pressedBackdrop.current = event.target === dialog.current;
      }}
      onClick={(event) => {
        if (pressedBackdrop.current && event.target === dialog.current) {
          onRequestClose();
        }
      }}
    >
      <div className="dm-shortcuts-panel">
        <header>
          <h2 id={titleId}>Keyboard shortcuts</h2>
          <button
            ref={closeButton}
            type="button"
            aria-label="Close keyboard shortcuts"
            onClick={onRequestClose}
          >
            ×
          </button>
        </header>
        <div className="dm-shortcuts-columns">
          <ShortcutList title="Machine" rows={GLOBAL_SHORTCUTS} />
          <ShortcutList title="Pattern grid" rows={GRID_SHORTCUTS} />
          <ShortcutList title="Knobs" rows={KNOB_SHORTCUTS} />
        </div>
      </div>
    </dialog>
  );
}
