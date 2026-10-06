import { en, type Key } from "./locales/en";
import { useApp } from "./store";

/** Languages of the interface, as settings.language takes them; internal/app/i18n.go has the same. */
export type Lang = "en" | "zh-CN" | "zh-TW" | "ja" | "ko" | "ru" | "es" | "pt-BR" | "de" | "fr" | "tr" | "id" | "vi" | "fa" | "ar";

/** LANGUAGES names each language in itself, for the language picker. */
export const LANGUAGES: { value: Lang; label: string }[] = [
  { value: "en", label: "English" },
  { value: "zh-CN", label: "简体中文" },
  { value: "zh-TW", label: "繁體中文" },
  { value: "ja", label: "日本語" },
  { value: "ko", label: "한국어" },
  { value: "ru", label: "Русский" },
  { value: "es", label: "Español" },
  { value: "pt-BR", label: "Português (Brasil)" },
  { value: "de", label: "Deutsch" },
  { value: "fr", label: "Français" },
  { value: "tr", label: "Türkçe" },
  { value: "id", label: "Bahasa Indonesia" },
  { value: "vi", label: "Tiếng Việt" },
  { value: "fa", label: "فارسی" },
  { value: "ar", label: "العربية" },
];

type Dict = Record<Key, string>;

// The strings of each language but English load when the language is chosen.
const loaders: Record<Exclude<Lang, "en">, () => Promise<{ default: Dict }>> = {
  "zh-CN": () => import("./locales/zh-CN"),
  "zh-TW": () => import("./locales/zh-TW"),
  ja: () => import("./locales/ja"),
  ko: () => import("./locales/ko"),
  ru: () => import("./locales/ru"),
  es: () => import("./locales/es"),
  "pt-BR": () => import("./locales/pt-BR"),
  de: () => import("./locales/de"),
  fr: () => import("./locales/fr"),
  tr: () => import("./locales/tr"),
  id: () => import("./locales/id"),
  vi: () => import("./locales/vi"),
  fa: () => import("./locales/fa"),
  ar: () => import("./locales/ar"),
};

const dicts: Partial<Record<Lang, Dict>> = { en };

/** loadLang loads the strings of a language, once; until then, it shows in English. */
export async function loadLang(lang: Lang): Promise<void> {
  if (lang !== "en" && !dicts[lang]) dicts[lang] = (await loaders[lang]()).default;
}

/** isRTL reports the languages written from right to left. */
export function isRTL(lang: Lang): boolean {
  return lang === "fa" || lang === "ar";
}

/**
 * resolveLang returns the language of the interface: the setting, else the
 * system's, else English. It takes language tags such as "zh-Hant-TW" and
 * POSIX locales such as "pt_PT.UTF-8".
 */
export function resolveLang(setting: string | undefined, locale: string | undefined): Lang {
  const tag = (setting || locale || navigator.language || "en").split(/[.@]/)[0] ?? "en";
  const [base, ...rest] = tag.toLowerCase().split(/[-_]/);
  if (base === "zh") {
    if (rest.includes("hans")) return "zh-CN";
    return rest.some((p) => ["hant", "tw", "hk", "mo"].includes(p)) ? "zh-TW" : "zh-CN";
  }
  if (base === "pt") return "pt-BR";
  return LANGUAGES.find((l) => l.value === base)?.value ?? "en";
}

function format(s: string, params?: Record<string, string | number>): string {
  if (!params) return s;
  return s.replace(/\{(\w+)\}/g, (m, k: string) => (k in params ? String(params[k]) : m));
}

/** translate looks a key up in a language. */
export function translate(lang: Lang, key: Key, params?: Record<string, string | number>): string {
  return format(dicts[lang]?.[key] ?? en[key] ?? key, params);
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
