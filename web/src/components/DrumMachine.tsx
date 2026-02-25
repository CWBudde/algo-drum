import { useCallback, useEffect, useRef, useState } from "react";
import Knob from "./Knob";
import * as engine from "../engine/wasmEngine";

// Visual order: Cymbal on top, Bass on bottom
const TRACKS = ["Cymbal", "Tom", "HiHat", "Snare", "Bass"];
// Maps visual row index → engine track index (engine: 0=Bass,1=Snare,2=HiHat,3=Tom,4=Cymbal)
const TRACK_INDEX = [4, 3, 2, 1, 0];
const COLS = 8;
const ROWS = 5;

// Logical canvas dimensions — CSS scales these to fit the container
const CW = 1020;
const CH = 700;
const GRID_X = 120;
const GRID_Y = 80;
const GRID_W = 656;
const GRID_H = 440;
const CELL_W = GRID_W / COLS;
const CELL_H = GRID_H / ROWS;

// Single warm-amber LED color — classic drum machine look
const LED_COLOR = "#C87828";

// ─── Drawing helpers ────────────────────────────────────────────────────────

function drawBackground(ctx: CanvasRenderingContext2D) {
  const g = ctx.createRadialGradient(
    CW * 0.42,
    CH * 0.38,
    30,
    CW * 0.5,
    CH * 0.5,
    CW * 0.78,
  );
  g.addColorStop(0, "#252535");
  g.addColorStop(0.55, "#111118");
  g.addColorStop(1, "#05050A");
  ctx.fillStyle = g;
  ctx.fillRect(0, 0, CW, CH);
}

function drawPanel(ctx: CanvasRenderingContext2D) {
  const x = 14,
    y = 14,
    w = CW - 28,
    h = CH - 28,
    r = 18;

  // Drop shadow beneath the panel
  ctx.save();
  ctx.shadowColor = "rgba(0,0,0,0.95)";
  ctx.shadowBlur = 55;
  ctx.shadowOffsetY = 18;
  ctx.fillStyle = "#111";
  ctx.beginPath();
  ctx.roundRect(x, y, w, h, r);
  ctx.fill();
  ctx.restore();

  // Panel surface — diagonal gradient, light source top-left
  const pg = ctx.createLinearGradient(x, y, x + w * 0.55, y + h);
  pg.addColorStop(0, "#2E2E3C");
  pg.addColorStop(0.3, "#212130");
  pg.addColorStop(1, "#141420");
  ctx.fillStyle = pg;
  ctx.beginPath();
  ctx.roundRect(x, y, w, h, r);
  ctx.fill();

  // Bevel highlights clipped to panel shape
  ctx.save();
  ctx.beginPath();
  ctx.roundRect(x, y, w, h, r);
  ctx.clip();

  // Top-edge bright highlight
  const tg = ctx.createLinearGradient(0, y, 0, y + 6);
  tg.addColorStop(0, "rgba(255,255,255,0.26)");
  tg.addColorStop(1, "rgba(255,255,255,0)");
  ctx.fillStyle = tg;
  ctx.fillRect(x, y, w, 6);

  // Left-edge soft highlight
  const lg = ctx.createLinearGradient(x, 0, x + 5, 0);
  lg.addColorStop(0, "rgba(255,255,255,0.10)");
  lg.addColorStop(1, "rgba(255,255,255,0)");
  ctx.fillStyle = lg;
  ctx.fillRect(x, y, 5, h);

  // Bottom-edge shadow
  const bg = ctx.createLinearGradient(0, y + h - 6, 0, y + h);
  bg.addColorStop(0, "rgba(0,0,0,0)");
  bg.addColorStop(1, "rgba(0,0,0,0.65)");
  ctx.fillStyle = bg;
  ctx.fillRect(x, y + h - 6, w, 6);

  // Right-edge shadow
  const rg = ctx.createLinearGradient(x + w - 5, 0, x + w, 0);
  rg.addColorStop(0, "rgba(0,0,0,0)");
  rg.addColorStop(1, "rgba(0,0,0,0.45)");
  ctx.fillStyle = rg;
  ctx.fillRect(x + w - 5, y, 5, h);

  ctx.restore();

  // Outer border
  ctx.strokeStyle = "rgba(0,0,0,0.75)";
  ctx.lineWidth = 1;
  ctx.beginPath();
  ctx.roundRect(x + 0.5, y + 0.5, w - 1, h - 1, r);
  ctx.stroke();
}

function drawScreenArea(ctx: CanvasRenderingContext2D) {
  const sx = GRID_X - 10,
    sy = GRID_Y - 10;
  const sw = GRID_W + 20,
    sh = GRID_H + 20;
  const r = 12;

  // Outer groove shadow — makes the screen look pressed into the panel
  ctx.save();
  ctx.shadowColor = "rgba(0,0,0,0.85)";
  ctx.shadowBlur = 18;
  ctx.shadowOffsetY = 4;
  ctx.fillStyle = "#060610";
  ctx.beginPath();
  ctx.roundRect(sx, sy, sw, sh, r);
  ctx.fill();
  ctx.restore();

  // Screen surface
  ctx.fillStyle = "#08080E";
  ctx.beginPath();
  ctx.roundRect(sx, sy, sw, sh, r);
  ctx.fill();

  // Inner shadow — clipped to screen
  ctx.save();
  ctx.beginPath();
  ctx.roundRect(sx, sy, sw, sh, r);
  ctx.clip();

  const ig = ctx.createLinearGradient(0, sy, 0, sy + 22);
  ig.addColorStop(0, "rgba(0,0,0,0.75)");
  ig.addColorStop(1, "rgba(0,0,0,0)");
  ctx.fillStyle = ig;
  ctx.fillRect(sx, sy, sw, 22);

  const ilg = ctx.createLinearGradient(sx, 0, sx + 16, 0);
  ilg.addColorStop(0, "rgba(0,0,0,0.45)");
  ilg.addColorStop(1, "rgba(0,0,0,0)");
  ctx.fillStyle = ilg;
  ctx.fillRect(sx, sy, 16, sh);

  // Bottom-inner bounce light
  const ibg = ctx.createLinearGradient(0, sy + sh - 8, 0, sy + sh);
  ibg.addColorStop(0, "rgba(255,255,255,0)");
  ibg.addColorStop(1, "rgba(255,255,255,0.04)");
  ctx.fillStyle = ibg;
  ctx.fillRect(sx, sy + sh - 8, sw, 8);

  ctx.restore();

  // Screen border
  ctx.strokeStyle = "rgba(0,0,0,0.9)";
  ctx.lineWidth = 1;
  ctx.beginPath();
  ctx.roundRect(sx + 0.5, sy + 0.5, sw - 1, sh - 1, r);
  ctx.stroke();
}

function drawLED(
  ctx: CanvasRenderingContext2D,
  cx: number,
  cy: number,
  r: number,
  color: string,
) {
  ctx.save();
  ctx.shadowColor = color;
  ctx.shadowBlur = 10;

  const grad = ctx.createRadialGradient(
    cx - r * 0.15,
    cy - r * 0.2,
    0,
    cx,
    cy,
    r,
  );
  grad.addColorStop(0, "rgba(255,255,255,0.92)");
  grad.addColorStop(0.25, color);
  grad.addColorStop(1, color + "40");
  ctx.fillStyle = grad;
  ctx.beginPath();
  ctx.arc(cx, cy, r, 0, Math.PI * 2);
  ctx.fill();

  ctx.restore();
}

function drawGrid(
  ctx: CanvasRenderingContext2D,
  pattern: boolean[][],
  activeStep: number,
) {
  // Active column highlight
  if (activeStep >= 0) {
    ctx.fillStyle = "rgba(200,140,40,0.07)";
    ctx.fillRect(GRID_X + activeStep * CELL_W, GRID_Y, CELL_W, GRID_H);
    // Accent top bar on active column
    ctx.fillStyle = "rgba(200,140,40,0.4)";
    ctx.fillRect(GRID_X + activeStep * CELL_W + 2, GRID_Y, CELL_W - 4, 2);
  }

  // Bar-group dividers every 4 steps
  ctx.fillStyle = "rgba(180,160,130,0.08)";
  ctx.fillRect(GRID_X + 4 * CELL_W - 1, GRID_Y, 2, GRID_H);

  for (let row = 0; row < ROWS; row++) {
    for (let col = 0; col < COLS; col++) {
      const x = GRID_X + col * CELL_W;
      const y = GRID_Y + row * CELL_H;
      const inset = 7;
      const cr = 7;

      // Cell hole body
      ctx.fillStyle = "#06060C";
      ctx.beginPath();
      ctx.roundRect(
        x + inset,
        y + inset,
        CELL_W - inset * 2,
        CELL_H - inset * 2,
        cr,
      );
      ctx.fill();

      // Top inner shadow (recessed depth)
      const tg = ctx.createLinearGradient(0, y + inset, 0, y + inset + 12);
      tg.addColorStop(0, "rgba(0,0,0,0.65)");
      tg.addColorStop(1, "rgba(0,0,0,0)");
      ctx.fillStyle = tg;
      ctx.beginPath();
      ctx.roundRect(x + inset, y + inset, CELL_W - inset * 2, 12, [
        cr,
        cr,
        0,
        0,
      ]);
      ctx.fill();

      // Left inner shadow
      const lg = ctx.createLinearGradient(x + inset, 0, x + inset + 9, 0);
      lg.addColorStop(0, "rgba(0,0,0,0.35)");
      lg.addColorStop(1, "rgba(0,0,0,0)");
      ctx.fillStyle = lg;
      ctx.beginPath();
      ctx.roundRect(x + inset, y + inset, 9, CELL_H - inset * 2, [
        cr,
        0,
        0,
        cr,
      ]);
      ctx.fill();

      // Bottom inner bounce-light
      const bng = ctx.createLinearGradient(
        0,
        y + CELL_H - inset - 7,
        0,
        y + CELL_H - inset,
      );
      bng.addColorStop(0, "rgba(255,255,255,0)");
      bng.addColorStop(1, "rgba(255,255,255,0.05)");
      ctx.fillStyle = bng;
      ctx.beginPath();
      ctx.roundRect(x + inset, y + CELL_H - inset - 7, CELL_W - inset * 2, 7, [
        0,
        0,
        cr,
        cr,
      ]);
      ctx.fill();

      const ledR = Math.min(CELL_W, CELL_H) * 0.27;
      const cx = x + CELL_W / 2;
      const cy = y + CELL_H / 2;

      if (pattern[row][col]) {
        drawLED(ctx, cx, cy, ledR, LED_COLOR);
      } else {
        // Dim off-state dot — barely visible warm hint
        ctx.fillStyle = "rgba(140,90,30,0.18)";
        ctx.beginPath();
        ctx.arc(cx, cy, ledR * 0.45, 0, Math.PI * 2);
        ctx.fill();
      }
    }
  }

  // Track labels
  ctx.textAlign = "right";
  ctx.textBaseline = "middle";
  ctx.font = '500 11px "Inter", "Helvetica Neue", sans-serif';
  for (let row = 0; row < ROWS; row++) {
    ctx.fillStyle = "rgba(200,190,170,0.55)";
    ctx.fillText(
      TRACKS[row].toUpperCase(),
      GRID_X - 22,
      GRID_Y + row * CELL_H + CELL_H / 2,
    );
  }

  // Step numbers
  ctx.fillStyle = "rgba(180,165,140,0.30)";
  ctx.font = '10px "Inter", monospace';
  ctx.textAlign = "center";
  ctx.textBaseline = "top";
  for (let col = 0; col < COLS; col++) {
    ctx.fillText(
      String(col + 1),
      GRID_X + col * CELL_W + CELL_W / 2,
      GRID_Y + GRID_H + 19,
    );
  }
}

function drawTitle(ctx: CanvasRenderingContext2D) {
  ctx.textAlign = "left";
  ctx.textBaseline = "middle";

  ctx.fillStyle = "#8888A8";
  ctx.font = '600 21px "Inter", "Helvetica Neue", sans-serif';
  ctx.fillText("algo", 34, 43);

  ctx.fillStyle = "#4468C0";
  ctx.fillText("-drum", 79, 43);
}

// ─── Component ───────────────────────────────────────────────────────────────

interface Props {
  wasmLoaded: boolean;
}

export default function DrumMachine({ wasmLoaded }: Props) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const [pattern, setPattern] = useState<boolean[][]>(() =>
    Array.from({ length: ROWS }, () => Array<boolean>(COLS).fill(false)),
  );
  const [playing, setPlaying] = useState(false);
  const [tempo, setTempoState] = useState(0.43); // ~120 BPM
  const [swing, setSwingState] = useState(0.0);
  const [reverb, setReverbState] = useState(0.0);
  const [volumes, setVolumes] = useState(() => Array<number>(ROWS).fill(0.75));
  const [muted, setMuted] = useState(() => Array<boolean>(ROWS).fill(false));
  const activeStepRef = useRef(-1);
  const rafRef = useRef(0);

  const bpm = Math.round(60 + tempo * 140);

  useEffect(() => {
    if (wasmLoaded) engine.setTempo(bpm);
  }, [bpm, wasmLoaded]);
  useEffect(() => {
    if (wasmLoaded) engine.setSwing(swing * 0.5);
  }, [swing, wasmLoaded]);
  useEffect(() => {
    if (wasmLoaded) engine.setReverb(reverb);
  }, [reverb, wasmLoaded]);
  useEffect(() => {
    volumes.forEach((v, i) => {
      if (wasmLoaded) engine.setVolume(TRACK_INDEX[i], muted[i] ? 0 : v);
    });
  }, [volumes, muted, wasmLoaded]);

  // Animation / draw loop
  const draw = useCallback(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;

    activeStepRef.current = playing ? engine.currentStep() : -1;

    drawBackground(ctx);
    drawPanel(ctx);
    drawScreenArea(ctx);
    drawTitle(ctx);
    drawGrid(ctx, pattern, activeStepRef.current);

    rafRef.current = requestAnimationFrame(draw);
  }, [pattern, playing]);

  useEffect(() => {
    rafRef.current = requestAnimationFrame(draw);
    return () => cancelAnimationFrame(rafRef.current);
  }, [draw]);

  // Hit-test canvas click → grid cell
  const canvasToCell = useCallback((clientX: number, clientY: number) => {
    const canvas = canvasRef.current;
    if (!canvas) return null;
    const rect = canvas.getBoundingClientRect();
    const sx = rect.width / CW;
    const sy = rect.height / CH;
    const lx = (clientX - rect.left) / sx;
    const ly = (clientY - rect.top) / sy;
    const col = Math.floor((lx - GRID_X) / CELL_W);
    const row = Math.floor((ly - GRID_Y) / CELL_H);
    if (col < 0 || col >= COLS || row < 0 || row >= ROWS) return null;
    return { row, col };
  }, []);

  const handleCanvasClick = useCallback(
    (e: React.MouseEvent<HTMLCanvasElement>) => {
      const cell = canvasToCell(e.clientX, e.clientY);
      if (!cell) return;
      const { row, col } = cell;
      const next = !pattern[row][col];
      setPattern((prev) => {
        const updated = prev.map((r) => [...r]);
        updated[row][col] = next;
        return updated;
      });
      if (wasmLoaded) engine.setCell(TRACK_INDEX[row], col, next);
    },
    [canvasToCell, pattern, wasmLoaded],
  );

  const handlePlayStop = useCallback(async () => {
    if (!wasmLoaded) return;
    if (!playing) {
      engine.play();
      setPlaying(true);
    } else {
      engine.stop();
      setPlaying(false);
    }
  }, [playing, wasmLoaded]);

  const handleVolumeChange = useCallback((track: number, v: number) => {
    setVolumes((prev) => {
      const n = [...prev];
      n[track] = v;
      return n;
    });
  }, []);

  const handleMuteToggle = useCallback((track: number) => {
    setMuted((prev) => {
      const n = [...prev];
      n[track] = !n[track];
      return n;
    });
  }, []);

  return (
    <div
      ref={containerRef}
      style={{ width: "100%", maxWidth: 1020, position: "relative" }}
    >
      <canvas
        ref={canvasRef}
        width={CW}
        height={CH}
        onClick={handleCanvasClick}
        style={{
          width: "100%",
          height: "auto",
          display: "block",
          borderRadius: 22,
          boxShadow: "0 24px 80px rgba(0,0,0,0.85), 0 4px 16px rgba(0,0,0,0.6)",
          cursor: "pointer",
        }}
      />

      {/* Bottom controls: play, tempo, swing */}
      <div
        style={{
          position: "absolute",
          bottom: "7%",
          left: "9%",
          display: "flex",
          alignItems: "center",
          gap: 20,
        }}
      >
        <button
          onClick={handlePlayStop}
          disabled={!wasmLoaded}
          title={playing ? "Stop" : "Play"}
          style={{
            width: 52,
            height: 52,
            borderRadius: "50%",
            background: playing
              ? "linear-gradient(145deg, #E03333, #991111)"
              : "linear-gradient(145deg, #1A9A66, #0D6644)",
            border: "none",
            outline: playing
              ? "1.5px solid rgba(255,80,80,0.45)"
              : "1.5px solid rgba(0,200,120,0.35)",
            cursor: wasmLoaded ? "pointer" : "default",
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            boxShadow: playing
              ? "0 0 18px rgba(220,50,50,0.5), 0 4px 10px rgba(0,0,0,0.6), inset 0 1px 0 rgba(255,255,255,0.15)"
              : "0 0 14px rgba(0,180,100,0.35), 0 4px 10px rgba(0,0,0,0.6), inset 0 1px 0 rgba(255,255,255,0.15)",
            transition: "background 0.15s, box-shadow 0.15s",
          }}
        >
          {playing ? (
            <svg width={18} height={18} viewBox="0 0 18 18">
              <rect x={3} y={3} width={4} height={12} fill="white" rx={1} />
              <rect x={11} y={3} width={4} height={12} fill="white" rx={1} />
            </svg>
          ) : (
            <svg width={18} height={18} viewBox="0 0 18 18">
              <polygon points="5,3 15,9 5,15" fill="white" />
            </svg>
          )}
        </button>
        <Knob
          value={tempo}
          onChange={setTempoState}
          label={`${bpm} BPM`}
          size={54}
          color="#C87828"
        />
        <Knob
          value={swing}
          onChange={setSwingState}
          label="SWING"
          size={54}
          color="#C87828"
        />
        <Knob
          value={reverb}
          onChange={setReverbState}
          label="REVERB"
          size={54}
          color="#C87828"
        />
      </div>

      {/* Per-track mute diode + volume knob */}
      {volumes.map((v, i) => {
        const topPct = ((GRID_Y + i * CELL_H + CELL_H / 2) / CH) * 100;
        const leftPct = ((GRID_X + GRID_W + CW - 14) / 2 / CW) * 100;
        return (
          <div
            key={i}
            style={{
              position: "absolute",
              left: `${leftPct}%`,
              top: `${topPct}%`,
              transform: "translate(-50%, -50%)",
              display: "flex",
              alignItems: "center",
              gap: 8,
            }}
          >
            {/* Mute LED diode */}
            <button
              onClick={() => handleMuteToggle(i)}
              title={muted[i] ? "Unmute" : "Mute"}
              style={{
                width: 11,
                height: 11,
                borderRadius: "50%",
                background: muted[i]
                  ? "radial-gradient(circle at 35% 30%, #FF7070, #CC1010)"
                  : "radial-gradient(circle at 35% 30%, #3A1010, #1A0505)",
                border: "1px solid rgba(0,0,0,0.6)",
                boxShadow: muted[i]
                  ? "0 0 7px rgba(210,20,20,0.75), inset 0 1px 0 rgba(255,200,200,0.25)"
                  : "inset 0 1px 0 rgba(255,255,255,0.04)",
                cursor: "pointer",
                padding: 0,
                flexShrink: 0,
                transition: "background 0.1s, box-shadow 0.1s",
              }}
            />
            <Knob
              value={v}
              onChange={(val) => handleVolumeChange(i, val)}
              label={TRACKS[i].slice(0, 3).toUpperCase()}
              size={42}
              color="#C87828"
            />
          </div>
        );
      })}
    </div>
  );
}
