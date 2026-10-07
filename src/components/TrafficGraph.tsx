import { useEffect, useRef, useState } from "react";
import { useApp } from "../lib/store";

/** rgba turns #rgb or #rrggbb into rgba(), which every canvas understands. */
function rgba(hex: string, alpha: number): string {
  let h = hex.replace("#", "");
  if (h.length === 3) h = [...h].map((c) => c + c).join("");
  const n = Number.parseInt(h.slice(0, 6), 16);
  if (Number.isNaN(n)) return `rgba(124, 92, 255, ${alpha})`;
  return `rgba(${(n >> 16) & 255}, ${(n >> 8) & 255}, ${n & 255}, ${alpha})`;
}

/** useThemeKey changes when the theme or the accent does, for canvases to redraw. */
function useThemeKey(): number {
  const [key, setKey] = useState(0);
  useEffect(() => {
    const bump = () => setKey((k) => k + 1);
    const mq = matchMedia("(prefers-color-scheme: dark)");
    mq.addEventListener("change", bump);
    const mo = new MutationObserver(bump);
    mo.observe(document.documentElement, { attributes: true, attributeFilter: ["data-theme", "data-oled", "style"] });
    return () => {
      mq.removeEventListener("change", bump);
      mo.disconnect();
    };
  }, []);
  return key;
}

/**
 * usePaused tells whether graphs should stop drawing: the window is in the
 * background and the setting to pause there is on.
 */
function usePaused(): boolean {
  const on = useApp((s) => s.settings?.ui.pauseOnBlur ?? true);
  const away = () => document.hidden || !document.hasFocus();
  const [gone, setGone] = useState(away);
  useEffect(() => {
    const check = () => setGone(away());
    window.addEventListener("focus", check);
    window.addEventListener("blur", check);
    document.addEventListener("visibilitychange", check);
    return () => {
      window.removeEventListener("focus", check);
      window.removeEventListener("blur", check);
      document.removeEventListener("visibilitychange", check);
    };
  }, []);
  return on && gone;
}

/**
 * TrafficGraph draws upload and download over time on a canvas. Without a
 * height it fills its parent, which must have one.
 */
export function TrafficGraph({ up, down, height, minimal }: { up: number[]; down: number[]; height?: number; minimal?: boolean }) {
  const canvas = useRef<HTMLCanvasElement>(null);
  const [size, setSize] = useState(0);
  const theme = useThemeKey();
  const paused = usePaused();
  const drawn = useRef(false);
  useEffect(() => {
    const c = canvas.current;
    if (!c) return;
    const ro = new ResizeObserver(() => setSize(c.clientWidth * 10000 + c.clientHeight));
    ro.observe(c);
    return () => ro.disconnect();
  }, []);
  useEffect(() => {
    const c = canvas.current;
    // In the background, keep the last picture; the data still comes in, so
    // coming back draws it up to date.
    if (!c || (paused && drawn.current)) return;
    const dpr = window.devicePixelRatio || 1;
    const w = c.clientWidth;
    const h = c.clientHeight;
    if (!w || !h) return;
    drawn.current = true;
    if (c.width !== Math.round(w * dpr) || c.height !== Math.round(h * dpr)) {
      c.width = Math.round(w * dpr);
      c.height = Math.round(h * dpr);
    }
    const ctx = c.getContext("2d");
    if (!ctx) return;
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.clearRect(0, 0, w, h);
    const style = getComputedStyle(c);
    const down0 = style.getPropertyValue("--graph-down").trim() || style.getPropertyValue("--accent").trim() || "#7c5cff";
    const up0 = style.getPropertyValue("--graph-up").trim() || style.getPropertyValue("--accent-2").trim() || "#ec6aa6";
    const grid = style.getPropertyValue("--border").trim() || "#ddd";
    const max = Math.max(1024, ...up, ...down) * 1.15;
    const pad = minimal ? 1.5 : 3;
    if (!minimal) {
      ctx.strokeStyle = grid;
      ctx.lineWidth = 1;
      ctx.setLineDash([2, 4]);
      for (let i = 1; i < 4; i++) {
        const y = Math.round((h * i) / 4) + 0.5;
        ctx.beginPath();
        ctx.moveTo(0, y);
        ctx.lineTo(w, y);
        ctx.stroke();
      }
      ctx.setLineDash([]);
    }
    const draw = (data: number[], color: string, width: number, fill: number) => {
      const n = data.length;
      if (n < 2) return;
      const step = w / (n - 1);
      const y = (v: number) => h - pad - (v / max) * (h - pad * 2);
      ctx.beginPath();
      ctx.moveTo(0, y(data[0] ?? 0));
      for (let i = 1; i < n; i++) {
        const x0 = (i - 1) * step;
        const x1 = i * step;
        const xm = (x0 + x1) / 2;
        ctx.bezierCurveTo(xm, y(data[i - 1] ?? 0), xm, y(data[i] ?? 0), x1, y(data[i] ?? 0));
      }
      ctx.strokeStyle = color;
      ctx.lineWidth = width;
      ctx.lineJoin = "round";
      ctx.stroke();
      if (fill > 0) {
        ctx.lineTo(w, h);
        ctx.lineTo(0, h);
        ctx.closePath();
        const g = ctx.createLinearGradient(0, 0, 0, h);
        g.addColorStop(0, rgba(color, fill));
        g.addColorStop(1, rgba(color, 0));
        ctx.fillStyle = g;
        ctx.fill();
      }
    };
    draw(down, down0, minimal ? 1.25 : 1.75, 0.26);
    draw(up, up0, minimal ? 1 : 1.4, minimal ? 0 : 0.12);
  }, [up, down, size, theme, minimal, paused]);
  return (
    <canvas
      ref={canvas}
      className={minimal ? "side-graph" : "graph"}
      style={{ width: "100%", height: height ?? "100%", display: "block" }}
    />
  );
}
