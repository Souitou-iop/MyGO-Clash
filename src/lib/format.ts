// Formatting of sizes, rates, durations and delays.

const UNITS = ["B", "KB", "MB", "GB", "TB", "PB"];

export function bytes(n: number, digits = 1): string {
  if (!Number.isFinite(n) || n <= 0) return "0 B";
  let i = 0;
  let v = n;
  while (v >= 1024 && i < UNITS.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v.toFixed(i === 0 ? 0 : v < 10 ? digits : v < 100 ? 1 : 0)} ${UNITS[i]}`;
}

export function rate(n: number): string {
  return `${bytes(n)}/s`;
}

export function duration(ms: number): string {
  const s = Math.max(0, Math.floor(ms / 1000));
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m ${s % 60}s`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h}h ${m % 60}m`;
  return `${Math.floor(h / 24)}d ${h % 24}h`;
}

/** Relative time, such as "3 minutes ago" or "in 2 hours". */
export function relative(when: string | number | Date | undefined | null, lang: string): string {
  if (!when) return "—";
  const t = new Date(when).getTime();
  if (!Number.isFinite(t) || t <= 0) return "—";
  const diff = (t - Date.now()) / 1000;
  const rtf = new Intl.RelativeTimeFormat(lang === "zh" ? "zh-CN" : "en", { numeric: "auto" });
  const abs = Math.abs(diff);
  if (abs < 45) return rtf.format(Math.round(diff), "second");
  if (abs < 2700) return rtf.format(Math.round(diff / 60), "minute");
  if (abs < 79200) return rtf.format(Math.round(diff / 3600), "hour");
  if (abs < 2592000) return rtf.format(Math.round(diff / 86400), "day");
  if (abs < 31536000) return rtf.format(Math.round(diff / 2592000), "month");
  return rtf.format(Math.round(diff / 31536000), "year");
}

export function dateTime(when: string | number | Date | undefined | null): string {
  if (!when) return "—";
  const d = new Date(when);
  if (!Number.isFinite(d.getTime()) || d.getTime() <= 0) return "—";
  return d.toLocaleString(undefined, { year: "numeric", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit" });
}

export function dateOnly(unixSeconds: number): string {
  if (!unixSeconds) return "—";
  return new Date(unixSeconds * 1000).toLocaleDateString(undefined, { year: "numeric", month: "2-digit", day: "2-digit" });
}

export type DelayClass = "good" | "ok" | "bad" | "none";

/** The class of a delay: -1 untested, 0 failed. */
export function delayClass(d: number | undefined): DelayClass {
  if (d === undefined || d < 0) return "none";
  if (d === 0) return "bad";
  if (d < 200) return "good";
  if (d < 600) return "ok";
  return "bad";
}

export function delayText(d: number | undefined, timeout: string): string {
  if (d === undefined || d < 0) return "—";
  if (d === 0) return timeout;
  return `${d} ms`;
}

/** The emoji flag of a country code. */
export function flag(code: string | undefined): string {
  if (!code || code.length !== 2) return "🌐";
  const up = code.toUpperCase();
  return String.fromCodePoint(...[...up].map((c) => 0x1f1e6 + c.charCodeAt(0) - 65));
}

export function percent(a: number, b: number): number {
  if (!b) return 0;
  return Math.max(0, Math.min(100, (a / b) * 100));
}

export function errorText(e: unknown): string {
  if (e instanceof Error) return e.message;
  if (typeof e === "string") return e;
  return String(e);
}
