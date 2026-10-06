import { useApp } from "./store";
import { en, zh, type Key } from "./locales";

export type Lang = "en" | "zh";

const dicts: Record<Lang, Record<Key, string>> = { en, zh };

/** The language of the interface: the setting, else the system's. */
export function resolveLang(setting: string | undefined, locale: string | undefined): Lang {
  const l = (setting || locale || navigator.language || "en").toLowerCase();
  return l.startsWith("zh") ? "zh" : "en";
}

function format(s: string, params?: Record<string, string | number>): string {
  if (!params) return s;
  return s.replace(/\{(\w+)\}/g, (m, k: string) => (k in params ? String(params[k]) : m));
}

/** translate looks a key up in a language. */
export function translate(lang: Lang, key: Key, params?: Record<string, string | number>): string {
  return format(dicts[lang][key] ?? en[key] ?? key, params);
}

/** useT returns the translation function of the current language. */
export function useT() {
  const lang = useApp((s) => s.lang);
  return (key: Key, params?: Record<string, string | number>) => translate(lang, key, params);
}

/** t translates outside components, in the current language. */
export function t(key: Key, params?: Record<string, string | number>): string {
  return translate(useApp.getState().lang, key, params);
}

export type { Key };
