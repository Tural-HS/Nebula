// UI language (ADR-0025). Azerbaijani is bundled; another language is loaded
// before the app renders, so every dictionary is a plain module constant.
// Changing the language stores the choice and reloads the page.
import { az } from "../locales/az";
import type { Widen } from "./types";

export const LOCALES = ["az", "en", "tr", "ru"] as const;
export type Locale = (typeof LOCALES)[number];
export type Messages = Widen<typeof az>;

/** Each language's name in itself, as shown in the language menu. */
export const LOCALE_NAMES: Record<Locale, string> = { az: "Azərbaycanca", en: "English", tr: "Türkçe", ru: "Русский" };

const STORAGE_KEY = "nebula.locale";

const loaders: Record<Exclude<Locale, "az">, () => Promise<Messages>> = {
  en: () => import("../locales/en").then((m) => m.en),
  tr: () => import("../locales/tr").then((m) => m.tr),
  ru: () => import("../locales/ru").then((m) => m.ru),
};

let active: Locale = "az";
let current: Messages = az;

export const isLocale = (v: unknown): v is Locale => typeof v === "string" && (LOCALES as readonly string[]).includes(v);

/** A stored choice wins; then the first browser language we have; then Azerbaijani. */
export function detectLocale(stored: string | null, languages: readonly string[]): Locale {
  if (isLocale(stored)) return stored;
  for (const lang of languages) {
    const base = lang.toLowerCase().split("-")[0];
    if (isLocale(base)) return base;
  }
  return "az";
}

function readStored(): string | null {
  try {
    return globalThis.localStorage.getItem(STORAGE_KEY); // throws when storage is blocked
  } catch {
    return null;
  }
}

export const activeLocale = (): Locale => active;
export const messages = (): Messages => current;

/** Picks and loads the UI language; if the download fails the app stays in Azerbaijani. */
export async function loadMessages(): Promise<Locale> {
  const languages = navigator.languages.length > 0 ? navigator.languages : [navigator.language];
  const want = detectLocale(readStored(), languages);
  if (want !== "az") {
    try {
      current = await loaders[want]();
      active = want;
    } catch (err) {
      console.error(err);
    }
  }
  document.documentElement.lang = active;
  return active;
}

/** Stores the choice and reloads, so every screen and cached label uses it. */
export function switchLocale(locale: Locale): void {
  try {
    globalThis.localStorage.setItem(STORAGE_KEY, locale);
  } catch {
    return; // storage blocked: nothing would change after a reload
  }
  window.location.reload();
}
