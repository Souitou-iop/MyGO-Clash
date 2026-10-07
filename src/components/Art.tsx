import { useId } from "react";
import compass from "../assets/compass.svg";

// Drawings after MyGO!!!!!: the band name's five exclamation marks, and the
// compass of the app icon, out on a rainy night.

/** The image colors of the five members: Tomori, Anon, Rāna, Soyo, Taki. */
export const MEMBERS = ["#77BBDD", "#FF8899", "#77DD77", "#FFDD88", "#7777AA"];

/** Five draws the band name's five exclamation marks, one per member. */
export function Five({ height = 14 }: { height?: number }) {
  const w = height * 0.26;
  const gap = height * 0.24;
  return (
    <svg width={5 * w + 4 * gap} height={height} viewBox={`0 0 ${5 * w + 4 * gap} ${height}`} aria-label="!!!!!" className="five">
      {MEMBERS.map((c, i) => {
        const x = i * (w + gap);
        return (
          <g key={c} fill={c}>
            <rect x={x} y={0} width={w} height={height * 0.66} rx={w / 2} />
            <circle cx={x + w / 2} cy={height - w / 2} r={w / 2} />
          </g>
        );
      })}
    </svg>
  );
}

/** RainyCompass is the picture of empty states: the compass out in the rain. */
export function RainyCompass({ width = 132 }: { width?: number }) {
  const id = useId().replace(/:/g, "");
  return (
    <svg width={width} height={(width * 100) / 140} viewBox="0 0 140 100" aria-hidden="true" className="rainy-compass">
      <defs>
        <radialGradient id={`${id}g`} cx="0.5" cy="0.55" r="0.5">
          <stop offset="0" style={{ stopColor: "var(--accent-2)" }} stopOpacity="0.22" />
          <stop offset="0.55" style={{ stopColor: "var(--accent)" }} stopOpacity="0.1" />
          <stop offset="1" style={{ stopColor: "var(--accent)" }} stopOpacity="0" />
        </radialGradient>
      </defs>
      <ellipse cx="70" cy="54" rx="62" ry="42" fill={`url(#${id}g)`} />
      <g style={{ stroke: "var(--text-faint)" }} strokeOpacity="0.55" strokeWidth="1.1" strokeLinecap="round">
        <path d="M22 10l-4 14M26 36l-3 10M118 8l-4 15M124 34l-3 11M16 56l-3 11M126 62l-3 12M30 80l-3 10M112 84l-2 8" />
      </g>
      <image href={compass} x="34" y="14" width="72" height="72" />
    </svg>
  );
}
