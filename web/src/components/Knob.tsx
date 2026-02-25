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

export default function Knob({ value, onChange, label, size = 48, color = '#5C8A6A' }: KnobProps) {
  const dragRef = useRef<{ startY: number; startVal: number } | null>(null)
  const [dragging, setDragging] = useState(false)

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
  const r = size * 0.38
  const angle = valueToAngle(value)
  const indicator = polarToXY(cx, cy, r * 0.65, angle)

  return (
    <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 4, userSelect: 'none' }}>
      <svg width={size} height={size} onMouseDown={handleMouseDown} style={{ cursor: 'ns-resize' }}>
        <circle cx={cx} cy={cy} r={r} fill="#3D2B1F" stroke="#6B4C38" strokeWidth={1.5} />
        <path d={describeArc(cx, cy, r, MIN_ANGLE, MAX_ANGLE)} fill="none" stroke="#5A3F2E" strokeWidth={3} strokeLinecap="round" />
        <path d={describeArc(cx, cy, r, MIN_ANGLE, angle)} fill="none" stroke={color} strokeWidth={3} strokeLinecap="round" />
        <circle cx={indicator.x} cy={indicator.y} r={2.5} fill={color} />
      </svg>
      <span style={{ color: '#C4A07A', fontSize: 10, fontFamily: 'Georgia, serif', letterSpacing: '0.05em' }}>
        {label}
      </span>
    </div>
  )
}
