import { useCallback, useEffect, useId, useRef, useState } from "react";
import {
  MAX_ANGLE,
  MIN_ANGLE,
  dragValue,
  keyValue,
  valueToAngle,
  wheelValue,
} from "./knobMath";
import "./Knob.css";

interface KnobProps {
  value: number; // 0.0 – 1.0
  onChange: (v: number) => void;
  label: string;
  /** Accessible name; falls back to label. */
  ariaLabel?: string;
  /** Human-readable value, e.g. "120 BPM"; defaults to a percentage. */
  valueText?: (v: number) => string;
  /** Double-click / Escape resets to this value. */
  defaultValue?: number;
  size?: number; // diameter in px, default 48
  color?: string;
}

function polarToXY(cx: number, cy: number, r: number, angleDeg: number) {
  const rad = ((angleDeg - 90) * Math.PI) / 180;
  return { x: cx + r * Math.cos(rad), y: cy + r * Math.sin(rad) };
}

function describeArc(
  cx: number,
  cy: number,
  r: number,
  startDeg: number,
  endDeg: number,
) {
  const start = polarToXY(cx, cy, r, startDeg);
  const end = polarToXY(cx, cy, r, endDeg);
  const largeArc = endDeg - startDeg > 180 ? 1 : 0;
  return `M ${start.x} ${start.y} A ${r} ${r} 0 ${largeArc} 1 ${end.x} ${end.y}`;
}

export default function Knob({
  value,
  onChange,
  label,
  ariaLabel,
  valueText,
  defaultValue,
  size = 48,
  color = "#C87828",
}: KnobProps) {
  const svgRef = useRef<SVGSVGElement>(null);
  const dragRef = useRef<{
    pointerId: number;
    startY: number;
    startVal: number;
  } | null>(null);
  const [dragging, setDragging] = useState(false);

  // Unique per-instance ID for SVG gradient references — labels repeat
  // across knobs (e.g. "DEC") and can change per render (tempo readout).
  const id = useId().replace(/[^a-zA-Z0-9_-]/g, "");

  const readout = valueText ? valueText(value) : `${Math.round(value * 100)}%`;

  const handlePointerDown = useCallback(
    (e: React.PointerEvent<SVGSVGElement>) => {
      e.preventDefault();
      dragRef.current = {
        pointerId: e.pointerId,
        startY: e.clientY,
        startVal: value,
      };
      e.currentTarget.setPointerCapture?.(e.pointerId);
      e.currentTarget.focus();
      setDragging(true);
    },
    [value],
  );

  useEffect(() => {
    if (!dragging) return;
    const onMove = (e: PointerEvent) => {
      if (!dragRef.current || e.pointerId !== dragRef.current.pointerId) return;
      onChange(
        dragValue(
          dragRef.current.startVal,
          dragRef.current.startY,
          e.clientY,
          e.shiftKey,
        ),
      );
    };
    const onUp = (e: PointerEvent) => {
      if (!dragRef.current || e.pointerId !== dragRef.current.pointerId) return;
      setDragging(false);
      dragRef.current = null;
    };
    window.addEventListener("pointermove", onMove);
    window.addEventListener("pointerup", onUp);
    window.addEventListener("pointercancel", onUp);
    return () => {
      window.removeEventListener("pointermove", onMove);
      window.removeEventListener("pointerup", onUp);
      window.removeEventListener("pointercancel", onUp);
    };
  }, [dragging, onChange]);

  // Wheel support needs a non-passive listener to preventDefault scrolling.
  useEffect(() => {
    const svg = svgRef.current;
    if (!svg) return;
    const onWheel = (e: WheelEvent) => {
      e.preventDefault();
      onChange(wheelValue(value, e.deltaY));
    };
    svg.addEventListener("wheel", onWheel, { passive: false });
    return () => svg.removeEventListener("wheel", onWheel);
  }, [value, onChange]);

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<SVGSVGElement>) => {
      const next = keyValue(value, e.key, e.shiftKey, defaultValue);
      if (next === null) return;

      // An Escape that would not change anything is not a reset, so let it
      // bubble: an enclosing <dialog> (the voice editor) treats it as a close
      // request. Restricted to Escape on purpose — skipping preventDefault for
      // every key would let ArrowUp at value 1 scroll the page.
      if (e.key === "Escape" && next === value) return;

      e.preventDefault();
      onChange(next);
    },
    [value, onChange, defaultValue],
  );

  const handleDoubleClick = useCallback(() => {
    if (defaultValue !== undefined) onChange(defaultValue);
  }, [defaultValue, onChange]);

  const cx = size / 2;
  const cy = size / 2;
  const arcR = size * 0.42; // travel arc radius (outside the knob body)
  const bodyR = size * 0.33; // knob body radius
  const angle = valueToAngle(value);

  // Indicator: line from near-center to near-edge
  const indInner = polarToXY(cx, cy, bodyR * 0.14, angle);
  const indOuter = polarToXY(cx, cy, bodyR * 0.8, angle);

  return (
    <div className="knob">
      {dragging && <span className="knob-readout">{readout}</span>}
      <svg
        ref={svgRef}
        width={size}
        height={size}
        role="slider"
        aria-label={ariaLabel ?? label}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={Math.round(value * 100)}
        aria-valuetext={readout}
        tabIndex={0}
        onPointerDown={handlePointerDown}
        onKeyDown={handleKeyDown}
        onDoubleClick={handleDoubleClick}
        style={{ overflow: "visible" }}
      >
        <title>{readout}</title>
        <defs>
          {/* Body: radial gradient — light top-left, shadow bottom-right */}
          <radialGradient id={`kb_${id}`} cx="34%" cy="28%" r="70%">
            <stop offset="0%" stopColor="#7A7A88" />
            <stop offset="35%" stopColor="#363644" />
            <stop offset="100%" stopColor="#131320" />
          </radialGradient>

          {/* Specular highlight: bright soft spot near top-left */}
          <radialGradient id={`ks_${id}`} cx="31%" cy="26%" r="48%">
            <stop offset="0%" stopColor="rgba(255,255,255,0.52)" />
            <stop offset="55%" stopColor="rgba(255,255,255,0.08)" />
            <stop offset="100%" stopColor="rgba(255,255,255,0)" />
          </radialGradient>
        </defs>

        {/* Outer shadow ring — gives depth beneath the knob */}
        <circle cx={cx} cy={cy} r={arcR + 1.5} fill="rgba(0,0,0,0.55)" />

        {/* Travel arc groove — dark recessed track */}
        <path
          d={describeArc(cx, cy, arcR, MIN_ANGLE, MAX_ANGLE)}
          fill="none"
          stroke="rgba(0,0,0,0.70)"
          strokeWidth={4}
          strokeLinecap="round"
        />
        {/* Groove inner highlight — subtle rim of the recess */}
        <path
          d={describeArc(cx, cy, arcR, MIN_ANGLE, MAX_ANGLE)}
          fill="none"
          stroke="rgba(255,255,255,0.055)"
          strokeWidth={2}
          strokeLinecap="round"
        />

        {/* Value arc */}
        <path
          d={describeArc(cx, cy, arcR, MIN_ANGLE, angle)}
          fill="none"
          stroke={color}
          strokeWidth={2.5}
          strokeLinecap="round"
          strokeOpacity={0.8}
        />

        {/* Knob body */}
        <circle cx={cx} cy={cy} r={bodyR} fill={`url(#kb_${id})`} />

        {/* Specular highlight overlay */}
        <circle cx={cx} cy={cy} r={bodyR} fill={`url(#ks_${id})`} />

        {/* Thin rim — top-left bright, bottom-right dark */}
        <circle
          cx={cx}
          cy={cy}
          r={bodyR}
          fill="none"
          stroke="rgba(255,255,255,0.13)"
          strokeWidth={0.8}
        />

        {/* Indicator line */}
        <line
          x1={indInner.x}
          y1={indInner.y}
          x2={indOuter.x}
          y2={indOuter.y}
          stroke="rgba(255,255,255,0.85)"
          strokeWidth={1.4}
          strokeLinecap="round"
        />
      </svg>

      <span className="knob-label">{label}</span>
    </div>
  );
}
