import { useEffect, useRef } from "react";

/** rgba turns #rgb or #rrggbb into rgba(), which every canvas understands. */
function rgba(hex: string, alpha: number): string {
  let h = hex.replace("#", "");
  if (h.length === 3) h = [...h].map((c) => c + c).join("");
  const n = Number.parseInt(h.slice(0, 6), 16);
  if (Number.isNaN(n)) return `rgba(124, 92, 255, ${alpha})`;
  return `rgba(${(n >> 16) & 255}, ${(n >> 8) & 255}, ${n & 255}, ${alpha})`;
}

/** TrafficGraph draws upload and download over time on a canvas. */
export function TrafficGraph({ up, down, height = 140, minimal }: { up: number[]; down: number[]; height?: number; minimal?: boolean }) {
  const canvas = useRef<HTMLCanvasElement>(null);
  useEffect(() => {
    const c = canvas.current;
    if (!c) return;
    const dpr = window.devicePixelRatio || 1;
    const w = c.clientWidth;
    const h = c.clientHeight;
    if (c.width !== Math.round(w * dpr) || c.height !== Math.round(h * dpr)) {
      c.width = Math.round(w * dpr);
      c.height = Math.round(h * dpr);
    }
    const ctx = c.getContext("2d");
    if (!ctx) return;
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.clearRect(0, 0, w, h);
    const style = getComputedStyle(c);
    const accent = style.getPropertyValue("--accent").trim() || "#7c5cff";
    const accent2 = style.getPropertyValue("--accent-2").trim() || "#ec6aa6";
    const grid = style.getPropertyValue("--border").trim() || "#ddd";
    const max = Math.max(1024, ...up, ...down) * 1.15;
    const pad = minimal ? 1 : 4;
    if (!minimal) {
      ctx.strokeStyle = grid;
      ctx.lineWidth = 1;
      ctx.setLineDash([3, 4]);
      for (let i = 1; i < 4; i++) {
        const y = Math.round((h * i) / 4) + 0.5;
        ctx.beginPath();
        ctx.moveTo(0, y);
        ctx.lineTo(w, y);
        ctx.stroke();
      }
      ctx.setLineDash([]);
    }
    const draw = (data: number[], color: string, fill: boolean) => {
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
      ctx.lineWidth = minimal ? 1.25 : 1.75;
      ctx.stroke();
      if (fill) {
        ctx.lineTo(w, h);
        ctx.lineTo(0, h);
        ctx.closePath();
        const g = ctx.createLinearGradient(0, 0, 0, h);
        g.addColorStop(0, rgba(color, 0.28));
        g.addColorStop(1, rgba(color, 0));
        ctx.fillStyle = g;
        ctx.fill();
      }
    };
    draw(down, accent, true);
    draw(up, accent2, !minimal);
  }, [up, down]);
  return <canvas ref={canvas} className={minimal ? "side-graph" : ""} style={{ width: "100%", height, display: "block" }} />;
}
