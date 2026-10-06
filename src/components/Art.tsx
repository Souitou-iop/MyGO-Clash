import { useId } from "react";

// Drawings after MyGO!!!!!: the band name's five exclamation marks, and a
// guitar pick with a lightning bolt cut through it, out on a rainy night.

/** PICK is a guitar pick in a 24 × 24 box, point down. */
export const PICK =
  "M12 22.2c-1.1 0-2.3-1.4-4.1-4.2C5.4 14.1 3.7 10.4 3.4 8.1 3.1 5.6 4 4.3 6.5 3.3 8.3 2.6 10.1 2.2 12 2.2s3.7.4 5.5 1.1c2.5 1 3.4 2.3 3.1 4.8-.3 2.3-2 6-4.5 9.9-1.8 2.8-3 4.2-4.1 4.2Z";

/** BOLT is the lightning bolt through the pick, in the same box. */
export const BOLT = "M13.5 5.4 8.7 12.3h3l-1.3 5.5 4.9-7.3h-3l1.2-5.1Z";

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

/** RainyPick is the picture of empty states: a pick out in the rain. */
export function RainyPick({ width = 132 }: { width?: number }) {
  const id = useId().replace(/:/g, "");
  return (
    <svg width={width} height={(width * 100) / 140} viewBox="0 0 140 100" aria-hidden="true" className="rainy-pick">
      <defs>
        <radialGradient id={`${id}g`} cx="0.5" cy="0.55" r="0.5">
          <stop offset="0" style={{ stopColor: "var(--accent-2)" }} stopOpacity="0.22" />
          <stop offset="0.55" style={{ stopColor: "var(--accent)" }} stopOpacity="0.1" />
          <stop offset="1" style={{ stopColor: "var(--accent)" }} stopOpacity="0" />
        </radialGradient>
        <linearGradient id={`${id}s`} x1="0" y1="0" x2="1" y2="1">
          <stop offset="0" style={{ stopColor: "var(--accent-2)" }} />
          <stop offset="1" style={{ stopColor: "var(--accent)" }} />
        </linearGradient>
      </defs>
      <ellipse cx="70" cy="54" rx="62" ry="42" fill={`url(#${id}g)`} />
      <g style={{ stroke: "var(--text-faint)" }} strokeOpacity="0.55" strokeWidth="1.1" strokeLinecap="round">
        <path d="M22 10l-4 14M40 30l-3 10M104 8l-4 15M122 30l-3 11M16 56l-3 11M126 62l-3 12M30 80l-3 10M112 84l-2 8" />
      </g>
      <g transform="rotate(-12 70 52) translate(40 22) scale(2.5)">
        <path d={PICK} style={{ fill: "var(--surface)" }} stroke={`url(#${id}s)`} strokeWidth="0.75" strokeLinejoin="round" />
        <path d={BOLT} fill={`url(#${id}s)`} />
      </g>
      <path d="M100 52c0 0-5 6.2-5 9.4a5 5 0 0 0 10 0c0-3.2-5-9.4-5-9.4Z" style={{ fill: "var(--accent)" }} fillOpacity="0.75" />
    </svg>
  );
}
