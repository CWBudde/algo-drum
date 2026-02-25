import { useCallback, useEffect, useRef, useState } from 'react'
import Knob from './Knob'
import * as engine from '../engine/wasmEngine'

// Visual order: Cymbal on top, Bass on bottom
const TRACKS = ['Cymbal', 'Tom', 'HiHat', 'Snare', 'Bass']
// Maps visual row index → engine track index (engine: 0=Bass,1=Snare,2=HiHat,3=Tom,4=Cymbal)
const TRACK_INDEX = [4, 3, 2, 1, 0]
const COLS = 8
const ROWS = 5

// Logical canvas dimensions — CSS scales these to fit the container
const CW = 920
const CH = 680
const GRID_X = 100
const GRID_Y = 80
const GRID_W = 600
const GRID_H = 470
const CELL_W = GRID_W / COLS
const CELL_H = GRID_H / ROWS

const WOOD_LIGHT = '#D4A853'
const WOOD_MID = '#C08838'
const WOOD_DARK = '#7A4E1E'
const PUCK_BODY = '#3D2B1F'
const PUCK_HI = '#5C4030'
const GRID_LINE = '#9B6B2A'

function drawBackground(ctx: CanvasRenderingContext2D) {
  const g = ctx.createLinearGradient(0, 0, CW, CH)
  g.addColorStop(0, WOOD_LIGHT)
  g.addColorStop(0.45, WOOD_MID)
  g.addColorStop(1, '#A86820')
  ctx.fillStyle = g
  ctx.fillRect(0, 0, CW, CH)

  // Grain lines
  ctx.save()
  ctx.strokeStyle = 'rgba(140,88,30,0.13)'
  ctx.lineWidth = 1
  for (let y = 8; y < CH; y += 16) {
    const jitter = Math.sin(y * 0.07) * 4
    ctx.beginPath()
    ctx.moveTo(0, y + jitter)
    ctx.bezierCurveTo(CW * 0.25, y + Math.sin(y * 0.11) * 6, CW * 0.75, y + Math.sin(y * 0.09) * 5, CW, y + Math.sin(y * 0.05) * 3)
    ctx.stroke()
  }
  ctx.restore()
}

function drawTitle(ctx: CanvasRenderingContext2D) {
  ctx.fillStyle = '#2A1508'
  ctx.font = 'bold 26px Georgia, serif'
  ctx.textAlign = 'left'
  ctx.textBaseline = 'middle'
  ctx.fillText('algo-drum', 22, 38)
}

function drawPuck(ctx: CanvasRenderingContext2D, cx: number, cy: number, r: number) {
  ctx.save()
  ctx.shadowColor = 'rgba(0,0,0,0.45)'
  ctx.shadowBlur = 10
  ctx.shadowOffsetY = 4
  const grad = ctx.createRadialGradient(cx - r * 0.25, cy - r * 0.25, r * 0.05, cx, cy, r)
  grad.addColorStop(0, PUCK_HI)
  grad.addColorStop(1, PUCK_BODY)
  ctx.fillStyle = grad
  ctx.beginPath()
  ctx.ellipse(cx, cy, r, r, 0, 0, Math.PI * 2)
  ctx.fill()
  ctx.restore()
}

function drawGrid(ctx: CanvasRenderingContext2D, pattern: boolean[][], activeStep: number) {
  // Outer border
  ctx.strokeStyle = WOOD_DARK
  ctx.lineWidth = 2
  ctx.beginPath()
  ctx.roundRect(GRID_X - 10, GRID_Y - 10, GRID_W + 20, GRID_H + 20, 14)
  ctx.stroke()

  // Active column highlight
  if (activeStep >= 0) {
    ctx.fillStyle = 'rgba(220,168,50,0.22)'
    ctx.fillRect(GRID_X + activeStep * CELL_W, GRID_Y, CELL_W, GRID_H)
  }

  // Cells
  for (let row = 0; row < ROWS; row++) {
    for (let col = 0; col < COLS; col++) {
      const x = GRID_X + col * CELL_W
      const y = GRID_Y + row * CELL_H

      // Beat-group shading (every 2 steps)
      if (col % 2 === 0) {
        ctx.fillStyle = 'rgba(0,0,0,0.04)'
        ctx.fillRect(x, y, CELL_W, CELL_H)
      }

      ctx.strokeStyle = GRID_LINE
      ctx.lineWidth = 1
      ctx.beginPath()
      ctx.roundRect(x + 5, y + 5, CELL_W - 10, CELL_H - 10, 6)
      ctx.stroke()

      if (pattern[row][col]) {
        drawPuck(ctx, x + CELL_W / 2, y + CELL_H / 2, Math.min(CELL_W, CELL_H) * 0.36)
      }
    }
  }

  // Track labels
  ctx.fillStyle = WOOD_DARK
  ctx.font = '12px Georgia, serif'
  ctx.textAlign = 'right'
  ctx.textBaseline = 'middle'
  for (let row = 0; row < ROWS; row++) {
    ctx.fillText(TRACKS[row], GRID_X - 16, GRID_Y + row * CELL_H + CELL_H / 2)
  }

  // Step numbers
  ctx.fillStyle = 'rgba(100,60,20,0.5)'
  ctx.font = '10px Georgia, serif'
  ctx.textAlign = 'center'
  ctx.textBaseline = 'top'
  for (let col = 0; col < COLS; col++) {
    ctx.fillText(String(col + 1), GRID_X + col * CELL_W + CELL_W / 2, GRID_Y + GRID_H + 6)
  }
}

interface Props {
  wasmLoaded: boolean
}

export default function DrumMachine({ wasmLoaded }: Props) {
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const containerRef = useRef<HTMLDivElement>(null)
  const [pattern, setPattern] = useState<boolean[][]>(
    () => Array.from({ length: ROWS }, () => Array<boolean>(COLS).fill(false)),
  )
  const [playing, setPlaying] = useState(false)
  const [tempo, setTempoState] = useState(0.43)   // ~120 BPM
  const [swing, setSwingState] = useState(0.0)
  const [volumes, setVolumes] = useState(() => Array<number>(ROWS).fill(0.75))
  const activeStepRef = useRef(-1)
  const rafRef = useRef(0)

  const bpm = Math.round(60 + tempo * 140)

  useEffect(() => { if (wasmLoaded) engine.setTempo(bpm) }, [bpm, wasmLoaded])
  useEffect(() => { if (wasmLoaded) engine.setSwing(swing * 0.5) }, [swing, wasmLoaded])
  useEffect(() => {
    volumes.forEach((v, i) => { if (wasmLoaded) engine.setVolume(TRACK_INDEX[i], v) })
  }, [volumes, wasmLoaded])

  // Animation / draw loop
  const draw = useCallback(() => {
    const canvas = canvasRef.current
    if (!canvas) return
    const ctx = canvas.getContext('2d')
    if (!ctx) return

    activeStepRef.current = playing ? engine.currentStep() : -1

    drawBackground(ctx)
    drawTitle(ctx)
    drawGrid(ctx, pattern, activeStepRef.current)

    rafRef.current = requestAnimationFrame(draw)
  }, [pattern, playing])

  useEffect(() => {
    rafRef.current = requestAnimationFrame(draw)
    return () => cancelAnimationFrame(rafRef.current)
  }, [draw])

  // Hit-test canvas click → grid cell
  const canvasToCell = useCallback((clientX: number, clientY: number) => {
    const canvas = canvasRef.current
    if (!canvas) return null
    const rect = canvas.getBoundingClientRect()
    const sx = rect.width / CW
    const sy = rect.height / CH
    const lx = (clientX - rect.left) / sx
    const ly = (clientY - rect.top) / sy
    const col = Math.floor((lx - GRID_X) / CELL_W)
    const row = Math.floor((ly - GRID_Y) / CELL_H)
    if (col < 0 || col >= COLS || row < 0 || row >= ROWS) return null
    return { row, col }
  }, [])

  const handleCanvasClick = useCallback((e: React.MouseEvent<HTMLCanvasElement>) => {
    const cell = canvasToCell(e.clientX, e.clientY)
    if (!cell) return
    const { row, col } = cell
    const next = !pattern[row][col]
    setPattern(prev => {
      const updated = prev.map(r => [...r])
      updated[row][col] = next
      return updated
    })
    if (wasmLoaded) engine.setCell(TRACK_INDEX[row], col, next)
  }, [canvasToCell, pattern, wasmLoaded])

  const handlePlayStop = useCallback(async () => {
    if (!wasmLoaded) return
    if (!playing) {
      engine.play()
      setPlaying(true)
    } else {
      engine.stop()
      setPlaying(false)
    }
  }, [playing, wasmLoaded])

  const handleVolumeChange = useCallback((track: number, v: number) => {
    setVolumes(prev => { const n = [...prev]; n[track] = v; return n })
  }, [])

  return (
    <div ref={containerRef} style={{ width: '100%', maxWidth: 920, position: 'relative' }}>
      <canvas
        ref={canvasRef}
        width={CW}
        height={CH}
        onClick={handleCanvasClick}
        style={{ width: '100%', height: 'auto', display: 'block', borderRadius: 14, boxShadow: '0 10px 50px rgba(0,0,0,0.7)', cursor: 'pointer' }}
      />

      {/* Bottom controls: play, tempo, swing */}
      <div style={{
        position: 'absolute',
        bottom: '7%',
        left: '9%',
        display: 'flex',
        alignItems: 'center',
        gap: 20,
      }}>
        <button
          onClick={handlePlayStop}
          disabled={!wasmLoaded}
          title={playing ? 'Stop' : 'Play'}
          style={{
            width: 50, height: 50,
            borderRadius: '50%',
            background: playing ? '#CC3333' : '#1A7A58',
            border: '2px solid rgba(255,255,255,0.15)',
            cursor: wasmLoaded ? 'pointer' : 'default',
            display: 'flex', alignItems: 'center', justifyContent: 'center',
            boxShadow: '0 4px 12px rgba(0,0,0,0.5)',
            transition: 'background 0.15s',
          }}
        >
          {playing
            ? <svg width={18} height={18} viewBox="0 0 18 18"><rect x={3} y={3} width={4} height={12} fill="white" rx={1} /><rect x={11} y={3} width={4} height={12} fill="white" rx={1} /></svg>
            : <svg width={18} height={18} viewBox="0 0 18 18"><polygon points="5,3 15,9 5,15" fill="white" /></svg>
          }
        </button>
        <Knob value={tempo} onChange={setTempoState} label={`${bpm} BPM`} size={54} color="#1A7A58" />
        <Knob value={swing} onChange={setSwingState} label="SWING" size={54} color="#C4903A" />
      </div>

      {/* Per-track volume knobs — right edge, aligned to grid rows */}
      {volumes.map((v, i) => {
        const topPct = (GRID_Y + i * CELL_H + CELL_H / 2) / CH * 100
        return (
          <div key={i} style={{
            position: 'absolute',
            right: '1.5%',
            top: `${topPct}%`,
            transform: 'translateY(-50%)',
          }}>
            <Knob
              value={v}
              onChange={(val) => handleVolumeChange(i, val)}
              label={TRACKS[i].slice(0, 3).toUpperCase()}
              size={42}
              color="#9B6B2A"
            />
          </div>
        )
      })}
    </div>
  )
}
