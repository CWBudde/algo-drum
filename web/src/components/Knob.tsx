import { useCallback, useEffect, useRef, useState } from 'react'

interface KnobProps {
  value: number        // 0.0 – 1.0
  onChange: (v: number) => void
  label: string
  size?: number        // diameter in px, default 48
  color?: string
}

const MIN_ANGLE = -135
const MAX_ANGLE = 135

function valueToAngle(v: number) {
  return MIN_ANGLE + v * (MAX_ANGLE - MIN_ANGLE)
}

function polarToXY(cx: number, cy: number, r: number, angleDeg: number) {
  const rad = ((angleDeg - 90) * Math.PI) / 180
  return { x: cx + r * Math.cos(rad), y: cy + r * Math.sin(rad) }
}

function describeArc(cx: number, cy: number, r: number, startDeg: number, endDeg: number) {
  const start = polarToXY(cx, cy, r, startDeg)
  const end = polarToXY(cx, cy, r, endDeg)
  const largeArc = endDeg - startDeg > 180 ? 1 : 0
  return `M ${start.x} ${start.y} A ${r} ${r} 0 ${largeArc} 1 ${end.x} ${end.y}`
}

export default function Knob({ value, onChange, label, size = 48, color = '#C87828' }: KnobProps) {
  const dragRef = useRef<{ startY: number; startVal: number } | null>(null)
  const [dragging, setDragging] = useState(false)

  // Stable ID from label for SVG gradient references
  const id = label.replace(/[^a-z0-9]/gi, '_')

  const handleMouseDown = useCallback((e: React.MouseEvent) => {
    e.preventDefault()
    dragRef.current = { startY: e.clientY, startVal: value }
    setDragging(true)
  }, [value])

  useEffect(() => {
    if (!dragging) return
    const onMove = (e: MouseEvent) => {
      if (!dragRef.current) return
      const delta = (dragRef.current.startY - e.clientY) / 150
      onChange(Math.max(0, Math.min(1, dragRef.current.startVal + delta)))
    }
    const onUp = () => { setDragging(false); dragRef.current = null }
    window.addEventListener('mousemove', onMove)
    window.addEventListener('mouseup', onUp)
    return () => {
      window.removeEventListener('mousemove', onMove)
      window.removeEventListener('mouseup', onUp)
    }
  }, [dragging, onChange])

  const cx = size / 2
  const cy = size / 2
  const arcR = size * 0.42     // travel arc radius (outside the knob body)
  const bodyR = size * 0.33    // knob body radius
  const angle = valueToAngle(value)

  // Indicator: line from near-center to near-edge
  const indInner = polarToXY(cx, cy, bodyR * 0.14, angle)
  const indOuter = polarToXY(cx, cy, bodyR * 0.80, angle)

  return (
    <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 4, userSelect: 'none' }}>
      <svg width={size} height={size} onMouseDown={handleMouseDown} style={{ cursor: 'ns-resize', overflow: 'visible' }}>
        <defs>
          {/* Body: radial gradient — light top-left, shadow bottom-right */}
          <radialGradient id={`kb_${id}`} cx="34%" cy="28%" r="70%">
            <stop offset="0%"   stopColor="#64646E" />
            <stop offset="38%"  stopColor="#2C2C3A" />
            <stop offset="100%" stopColor="#101018" />
          </radialGradient>

          {/* Specular highlight: bright soft spot near top-left */}
          <radialGradient id={`ks_${id}`} cx="31%" cy="26%" r="52%">
            <stop offset="0%"   stopColor="rgba(255,255,255,0.40)" />
            <stop offset="60%"  stopColor="rgba(255,255,255,0.06)" />
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
          strokeOpacity={0.80}
        />

        {/* Knob body */}
        <circle cx={cx} cy={cy} r={bodyR} fill={`url(#kb_${id})`} />

        {/* Specular highlight overlay */}
        <circle cx={cx} cy={cy} r={bodyR} fill={`url(#ks_${id})`} />

        {/* Thin rim — top-left bright, bottom-right dark */}
        <circle
          cx={cx} cy={cy} r={bodyR}
          fill="none"
          stroke="rgba(255,255,255,0.13)"
          strokeWidth={0.8}
        />

        {/* Indicator line */}
        <line
          x1={indInner.x} y1={indInner.y}
          x2={indOuter.x} y2={indOuter.y}
          stroke="rgba(255,255,255,0.85)"
          strokeWidth={1.4}
          strokeLinecap="round"
        />
      </svg>

      <span style={{
        color: 'rgba(195,185,165,0.60)',
        fontSize: 9,
        fontFamily: '"Inter", "Helvetica Neue", sans-serif',
        letterSpacing: '0.09em',
        fontWeight: 500,
      }}>
        {label}
      </span>
    </div>
  )
}
