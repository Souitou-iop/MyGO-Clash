// Formatting of sizes, rates, durations and delays.

import type { Key } from "./locales/en";

const UNITS = ["B", "KB", "MB", "GB", "TB", "PB"];

/**
 * ltr keeps a measure such as "12 KB/s" in its order inside text written
 * from right to left, where the number would otherwise move after its unit.
 */
export function ltr(s: string): string {
  return document.documentElement.dir === "rtl" ? `\u2066${s}\u2069` : s;
}

export function bytes(n: number, digits = 1): string {
  return ltr(plainBytes(n, digits));
}

function plainBytes(n: number, digits: number): string {
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
  return ltr(`${plainBytes(n, 1)}/s`);
}

/**
 * shortRate is a rate in at most four characters, such as "512B", "9.9K"
 * or "120M", for the collapsed sidebar: per second, in units of 1024.
 */
export function shortRate(n: number): string {
  if (!Number.isFinite(n) || n <= 0) return "0";
  const units = ["B", "K", "M", "G", "T"];
  let i = 0;
  let v = n;
  // 1000 and up go to the next unit, which keeps four characters
  while (v >= 999.5 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  const digits = i > 0 && v < 9.95 ? 1 : 0;
  return ltr(`${v.toFixed(digits)}${units[i]}`);
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
  const rtf = new Intl.RelativeTimeFormat(lang, { numeric: "auto" });
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
  return ltr(`${d} ms`);
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

/** SYNTAX spells the core's rule types as configurations write them. */
const SYNTAX: Record<string, string> = {
  Domain: "DOMAIN",
  DomainSuffix: "DOMAIN-SUFFIX",
  DomainKeyword: "DOMAIN-KEYWORD",
  DomainRegex: "DOMAIN-REGEX",
  DomainWildcard: "DOMAIN-WILDCARD",
  GeoSite: "GEOSITE",
  GeoIP: "GEOIP",
  SrcGeoIP: "SRC-GEOIP",
  IPASN: "IP-ASN",
  SrcIPASN: "SRC-IP-ASN",
  IPCIDR: "IP-CIDR",
  SrcIPCIDR: "SRC-IP-CIDR",
  IPSuffix: "IP-SUFFIX",
  SrcIPSuffix: "SRC-IP-SUFFIX",
  SrcPort: "SRC-PORT",
  DstPort: "DST-PORT",
  InPort: "IN-PORT",
  InUser: "IN-USER",
  InName: "IN-NAME",
  InType: "IN-TYPE",
  ProcessName: "PROCESS-NAME",
  ProcessPath: "PROCESS-PATH",
  ProcessNameRegex: "PROCESS-NAME-REGEX",
  ProcessPathRegex: "PROCESS-PATH-REGEX",
  ProcessNameWildcard: "PROCESS-NAME-WILDCARD",
  ProcessPathWildcard: "PROCESS-PATH-WILDCARD",
  Match: "MATCH",
  RuleSet: "RULE-SET",
  Network: "NETWORK",
  DSCP: "DSCP",
  Uid: "UID",
  SubRules: "SUB-RULE",
};

/** ruleType spells a rule type of the core (DomainSuffix) as configurations
 * write it (DOMAIN-SUFFIX). */
export function ruleType(type: string, payload = ""): string {
  const s = SYNTAX[type] ?? type.toUpperCase();
  return s === "IP-CIDR" && payload.includes(":") ? "IP-CIDR6" : s;
}

const GROUP_TYPES = ["Selector", "URLTest", "Fallback", "LoadBalance", "Relay"] as const;

/** groupType names a kind of proxy group in the interface's language. */
export function groupType(t: (key: Key) => string, type: string): string {
  return (GROUP_TYPES as readonly string[]).includes(type) ? t(`proxies.type.${type}` as Key) : type;
}
