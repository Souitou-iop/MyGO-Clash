import { Channel, isMyGo } from "mygo-runtime";
import { create } from "zustand";
import {
  App,
  Core,
  Profiles,
  Settings as SettingsAPI,
  Sync,
  Tailscale,
  events,
  type AppInfo,
  type AppState,
  type Notice,
  type ProfilesView,
  type Settings,
  type SyncStatus,
  type TailscaleStatus,
  type Traffic,
} from "../mygo";
import { errorText } from "./format";
import { isRTL, loadLang, resolveLang, type Lang } from "./i18n";

export type Page =
  | "home"
  | "proxies"
  | "profiles"
  | "connections"
  | "stats"
  | "rules"
  | "logs"
  | "tailscale"
  | "connectivity"
  | "settings";

export const PAGES: Page[] = ["home", "proxies", "profiles", "connections", "stats", "rules", "logs", "tailscale", "connectivity", "settings"];

/** route reads a page named by the Go side: "proxies", or "settings/sync" for a tab of the settings. */
function route(target: string): { page: Page; tab?: string } | null {
  const [page, tab] = target.split("/");
  return PAGES.includes(page as Page) ? { page: page as Page, tab: tab || undefined } : null;
}

export interface Toast extends Notice {
  id: number;
}

interface AppStore {
  booted: boolean;
  preview: boolean; // opened in a browser, without the app
  lang: Lang;
  page: Page;
  settingsTab: string;
  info: AppInfo | null;
  state: AppState | null;
  settings: Settings | null;
  profiles: ProfilesView | null;
  tailscale: TailscaleStatus | null;
  sync: SyncStatus | null;
  /** runtime counts applied configurations, for pages to refresh. */
  runtime: number;
  /** selection counts selections made outside the page. */
  selection: number;
  toasts: Toast[];
  /** focusGroup is a proxy group for the proxies page to show, once. */
  focusGroup: string | null;
  navigate: (page: Page, tab?: string) => void;
}

export const useApp = create<AppStore>((set) => ({
  booted: false,
  preview: false,
  lang: "en",
  page: "home",
  settingsTab: "general",
  info: null,
  state: null,
  settings: null,
  profiles: null,
  tailscale: null,
  sync: null,
  runtime: 0,
  selection: 0,
  toasts: [],
  focusGroup: null,
  navigate: (page, tab) => set((s) => ({ page, settingsTab: tab ?? s.settingsTab })),
}));

// ---- Toasts ----

let toastID = 0;

export function toast(n: Notice) {
  const id = ++toastID;
  useApp.setState((s) => ({ toasts: [...s.toasts.slice(-4), { ...n, id }] }));
  const ms = n.level === "error" ? 9000 : n.level === "warning" ? 7000 : 4000;
  setTimeout(() => dismiss(id), ms);
}

export function dismiss(id: number) {
  useApp.setState((s) => ({ toasts: s.toasts.filter((t) => t.id !== id) }));
}

export function toastError(message: string, err: unknown) {
  toast({ level: "error", message, detail: errorText(err) });
}

/** run awaits an action and reports its failure in a toast. */
export async function run<T>(fn: () => Promise<T>, failure: string, success?: string): Promise<T | undefined> {
  try {
    const v = await fn();
    if (success) toast({ level: "success", message: success });
    return v;
  } catch (e) {
    toastError(failure, e);
    return undefined;
  }
}

// ---- Settings ----

export type DeepPartial<T> = { [K in keyof T]?: T[K] extends object ? (T[K] extends unknown[] ? T[K] : DeepPartial<T[K]>) : T[K] };

/** patchSettings changes settings, and keeps the store in step. */
export async function patchSettings(patch: DeepPartial<Settings>): Promise<Settings> {
  const next = await SettingsAPI.patch(patch as Record<string, unknown>);
  useApp.setState({ settings: next });
  void switchLang(resolveLang(next.language, useApp.getState().info?.locale));
  applyAppearance(next);
  return next;
}

/** switchLang shows the interface in a language once its strings are loaded. */
async function switchLang(lang: Lang) {
  await loadLang(lang).catch(() => undefined);
  document.documentElement.lang = lang;
  document.documentElement.dir = isRTL(lang) ? "rtl" : "ltr";
  useApp.setState({ lang });
}

/** textOn picks the text color that reads best on a #rrggbb background. */
function textOn(hex: string): string {
  const n = Number.parseInt(hex.replace("#", "").slice(0, 6), 16);
  if (Number.isNaN(n)) return "#ffffff";
  const lin = (c: number) => {
    const v = c / 255;
    return v <= 0.04045 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4;
  };
  const l = 0.2126 * lin((n >> 16) & 255) + 0.7152 * lin((n >> 8) & 255) + 0.0722 * lin(n & 255);
  // Contrast with white against contrast with the night's navy (#08131f).
  return (1.05 / (l + 0.05) >= (l + 0.05) / 0.0567 ? "#ffffff" : "#08131f");
}

/** applyAppearance applies the theme, the accent, the font and custom CSS. */
export function applyAppearance(s: Settings) {
  const root = document.documentElement;
  if (s.theme === "light" || s.theme === "dark") root.dataset.theme = s.theme;
  else delete root.dataset.theme;
  if (s.oled) root.dataset.oled = "";
  else delete root.dataset.oled;
  if (s.accent) {
    root.style.setProperty("--accent", s.accent);
    root.style.setProperty("--accent-text", textOn(s.accent));
  } else {
    root.style.removeProperty("--accent");
    root.style.removeProperty("--accent-text");
  }
  if (s.fontFamily) root.style.setProperty("--font", `${s.fontFamily}, ${getComputedStyle(root).getPropertyValue("--font")}`);
  let style = document.getElementById("custom-css") as HTMLStyleElement | null;
  if (s.customCss) {
    if (!style) {
      style = document.createElement("style");
      style.id = "custom-css";
      document.head.appendChild(style);
    }
    style.textContent = s.customCss;
  } else style?.remove();
}

// ---- Traffic ----

const HISTORY = 120;

interface TrafficStore {
  now: Traffic;
  up: number[];
  down: number[];
  memory: number;
}

export const useTraffic = create<TrafficStore>(() => ({
  now: { up: 0, down: 0, upTotal: 0, downTotal: 0 },
  up: new Array(HISTORY).fill(0),
  down: new Array(HISTORY).fill(0),
  memory: 0,
}));

function follow<T>(start: (ch: Channel<T>) => Promise<void>, onValue: (v: T) => void) {
  const loop = () => {
    const ch = new Channel<T>(onValue);
    start(ch)
      .catch(() => {})
      .finally(() => setTimeout(loop, 2000));
  };
  loop();
}

function startStreams() {
  follow<Traffic>(
    (ch) => Core.traffic(ch),
    (t) =>
      useTraffic.setState((s) => ({
        now: t,
        up: [...s.up.slice(1), t.up],
        down: [...s.down.slice(1), t.down],
      })),
  );
  follow<{ inuse: number }>(
    (ch) => Core.memory(ch as never),
    (m) => useTraffic.setState({ memory: m.inuse }),
  );
}

// ---- Boot ----

export async function boot() {
  if (!isMyGo()) {
    useApp.setState({ booted: true, preview: true });
    return;
  }
  const [info, state, settings, profiles, tailscale, sync, target] = await Promise.all([
    App.info(),
    App.state(),
    SettingsAPI.get(),
    Profiles.list(),
    Tailscale.status(),
    Sync.status(),
    App.takePage(),
  ]);
  applyAppearance(settings);
  await switchLang(resolveLang(settings.language, info.locale));
  const start = route(target) ?? route(settings.startPage) ?? { page: "home" };
  useApp.setState({
    booted: true,
    info,
    state,
    settings,
    profiles,
    tailscale,
    sync,
    page: start.page,
    ...(start.tab ? { settingsTab: start.tab } : {}),
  });
  events.state.on((s) => useApp.setState({ state: s }));
  events.settings.on((s) => {
    useApp.setState({ settings: s });
    void switchLang(resolveLang(s.language, useApp.getState().info?.locale));
    applyAppearance(s);
  });
  events.profiles.on((p) => useApp.setState({ profiles: p }));
  events.tailscale.on((t) => useApp.setState({ tailscale: t }));
  events.sync.on((s) => useApp.setState({ sync: s }));
  events.notice.on(toast);
  events.navigate.on((p) => {
    const r = route(p);
    if (r) useApp.getState().navigate(r.page, r.tab);
  });
  events.runtime.on(() => useApp.setState((s) => ({ runtime: s.runtime + 1 })));
  events.selection.on(() => useApp.setState((s) => ({ selection: s.selection + 1 })));
  startStreams();
}
