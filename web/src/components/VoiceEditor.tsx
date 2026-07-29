import { useCallback, useEffect, useId, useRef, useState } from "react";
import Knob from "./Knob";
import { formatParam, type VoiceParamSpec } from "../engine/voiceParams";
import type { TomModel } from "../engine/tomModel";
import "./VoiceEditor.css";

const AMBER = "#C87828";
const BLUE = "#6D95C8";

// How long the reset confirmation stays in the live region.
const RESET_NOTICE_MS = 2000;

interface Props {
  /** Display name of the voice, e.g. "Snare". */
  name: string;
  /** The voice's parameter descriptors, in index order. */
  specs: readonly VoiceParamSpec[];
  /** Current normalized positions, parallel to `specs`. */
  values: readonly number[];
  /** True while the engine is still loading — disables the audition button. */
  disabled: boolean;
  /** Present only for the Tom, whose implementation can be selected. */
  model?: TomModel;
  onModelChange?: (model: TomModel) => void;
  onChange: (index: number, value: number) => void;
  onReset: () => void;
  onAudition: () => void;
  onRequestClose: () => void;
}

export default function VoiceEditor({
  name,
  specs,
  values,
  disabled,
  model,
  onModelChange,
  onChange,
  onReset,
  onAudition,
  onRequestClose,
}: Props) {
  const ref = useRef<HTMLDialogElement>(null);
  const titleId = useId();
  const [resetNotice, setResetNotice] = useState("");
  const noticeTimer = useRef<number | undefined>(undefined);
  const showProceduralParams = model !== "physical";

  // The component is mounted only while open, so showModal() runs exactly once.
  // It brings the focus trap, the background `inert`, ::backdrop and top-layer
  // promotion with it — none of which is worth hand-rolling here.
  useEffect(() => {
    const dialog = ref.current;
    if (!dialog || dialog.open) return;

    dialog.showModal();

    return () => {
      if (dialog.open) dialog.close();
    };
  }, []);

  useEffect(
    () => () => {
      if (noticeTimer.current !== undefined) {
        window.clearTimeout(noticeTimer.current);
      }
    },
    [],
  );

  // Escape is contested: Knob resets to its default on Escape, and only calls
  // preventDefault when that actually changes something. So a defaultPrevented
  // Escape means a knob consumed it; anything else is a close request.
  //
  // preventDefault runs unconditionally so the UA's own close watcher never
  // fires — the close decision lives here alone, identically in every browser.
  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLDialogElement>) => {
      if (e.key !== "Escape") return;

      // Read this before cancelling below, or we would only ever see our own
      // preventDefault.
      const consumedByKnob = e.defaultPrevented;

      e.preventDefault();

      if (!consumedByKnob) onRequestClose();
    },
    [onRequestClose],
  );

  // A knob drag released over the backdrop must not close the dialog, so a
  // backdrop click only counts when the press started there too.
  const pressedBackdrop = useRef(false);

  const handleReset = useCallback(() => {
    onReset();
    setResetNotice(`${name} voice reset to defaults`);

    if (noticeTimer.current !== undefined) {
      window.clearTimeout(noticeTimer.current);
    }

    noticeTimer.current = window.setTimeout(
      () => setResetNotice(""),
      RESET_NOTICE_MS,
    );
  }, [name, onReset]);

  return (
    <dialog
      ref={ref}
      className="dm-voice"
      aria-labelledby={titleId}
      // No aria-modal: showModal() already establishes modal semantics, and
      // adding it to a native dialog makes some screen readers double-handle it.
      onKeyDown={handleKeyDown}
      onClose={onRequestClose}
      onPointerDown={(e) => {
        pressedBackdrop.current = e.target === ref.current;
      }}
      onClick={(e) => {
        if (pressedBackdrop.current && e.target === ref.current) {
          onRequestClose();
        }
      }}
    >
      <div className="dm-voice-head">
        <h2 className="dm-voice-title" id={titleId}>
          {name} voice
        </h2>
        <button
          type="button"
          className="dm-voice-close"
          autoFocus
          aria-label={`Close ${name} voice settings`}
          onClick={onRequestClose}
        >
          <span aria-hidden="true">×</span>
        </button>
      </div>

      {model !== undefined && onModelChange && (
        <fieldset className="dm-voice-model">
          <legend>Tom synthesis model</legend>
          <label>
            <input
              type="radio"
              name={`${titleId}-model`}
              value="procedural"
              checked={model === "procedural"}
              onChange={() => onModelChange("procedural")}
            />
            <span>Algorithmic</span>
          </label>
          <label>
            <input
              type="radio"
              name={`${titleId}-model`}
              value="physical"
              checked={model === "physical"}
              onChange={() => onModelChange("physical")}
            />
            <span>
              Physical <small>EXPERIMENTAL</small>
            </span>
          </label>
        </fieldset>
      )}

      {showProceduralParams ? (
        <>
          <div className="dm-voice-knobs" data-params={specs.length}>
            {specs.map((spec, i) => (
              <div className="dm-voice-param" key={spec.id}>
                <Knob
                  value={values[i]}
                  onChange={(v) => onChange(i, v)}
                  label={spec.label}
                  ariaLabel={`${name} ${spec.name}`}
                  valueText={(v) => formatParam(spec, v)}
                  defaultValue={spec.default}
                  size={54}
                  color={spec.unit === "s" ? BLUE : AMBER}
                />
                <span className="dm-voice-value">
                  {formatParam(spec, values[i])}
                </span>
              </div>
            ))}
          </div>

          <p className="dm-voice-hint">
            The strip’s DEC knob trims these decay times by ±50%.
          </p>
        </>
      ) : (
        <div className="dm-voice-physical-note">
          <strong>Single circular head</strong>
          <p>
            A 48-mode Fourier–Bessel model with position-dependent strike and
            pickup, frequency-dependent loss, modal radiation, and a filtered
            microphone response. The strip’s DEC knob controls modal loss;
            detailed physical parameters will follow in the dedicated lab.
          </p>
        </div>
      )}

      <div className="dm-voice-foot">
        <button
          type="button"
          className="dm-algo-btn dm-voice-audition"
          disabled={disabled}
          onClick={onAudition}
          aria-label={`Audition ${name}`}
          title="Play this voice once"
        >
          <svg width={9} height={9} viewBox="0 0 10 10" aria-hidden="true">
            <polygon points="2,1 9,5 2,9" fill="currentColor" />
          </svg>
          AUDITION
        </button>
        <span className="dm-voice-foot-spacer" />
        {showProceduralParams && (
          <button
            type="button"
            className="dm-algo-btn dm-algo-btn-warn"
            onClick={handleReset}
            aria-label={`Reset ${name} voice to defaults`}
          >
            RESET
          </button>
        )}
        <button type="button" className="dm-algo-btn" onClick={onRequestClose}>
          CLOSE
        </button>
      </div>

      {/* RESET moves every knob with nothing focused, so announce it. */}
      <span className="dm-voice-notice" role="status" aria-live="polite">
        {resetNotice}
      </span>
    </dialog>
  );
}
