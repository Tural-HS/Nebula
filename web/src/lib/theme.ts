import { useSyncExternalStore } from "react";

// ---------------------------------------------------------------------------
// Colour theme per browser (ADR-0035, ADR-0049): the system setting, light,
// dark, or one of four palettes on top of them — green (FinTez green, light),
// paper (warm light, least glare), midnight (the logo's deep blue, dark), the
// dark evergreen, oled and dusk (ADR-0058) and high contrast (light).
// Stored like the language (ADR-0025) but applied live, without a reload.
// public/theme-init.js applies the stored choice before the first paint; it
// duplicates the few lines of resolveTheme/applyTheme because it must run
// as a classic same-origin script (CSP script-src 'self', no inline code).
// ---------------------------------------------------------------------------

export const THEMES = ["system", "light", "dark", "green", "paper", "midnight", "evergreen", "oled", "dusk", "contrast"] as const;
export type ThemePref = (typeof THEMES)[number];
export type ResolvedTheme = Exclude<ThemePref, "system">;

/** Palettes on a dark base get Tailwind's dark: variant too. */
export const DARK_THEMES: readonly ResolvedTheme[] = ["dark", "midnight", "evergreen", "oled", "dusk"];
export const isDarkTheme = (t: ResolvedTheme): boolean => DARK_THEMES.includes(t);

export const THEME_STORAGE_KEY = "nebula.theme";
export const DARK_QUERY = "(prefers-color-scheme: dark)";

/** Browser chrome colour (meta theme-color) per resolved theme. */
export const THEME_COLORS: Record<ResolvedTheme, string> = {
  light: "#16788b",
  dark: "#18181b",
  green: "#00524d",
  paper: "#f4ecdd",
  midnight: "#0f1a33",
  evergreen: "#0b1f1a",
  oled: "#000000",
  dusk: "#1a1430",
  contrast: "#000000",
};

export const isThemePref = (v: unknown): v is ThemePref => typeof v === "string" && (THEMES as readonly string[]).includes(v);

/** Anything unknown (nothing stored, an old or edited value) means "system". */
export function parseTheme(stored: string | null | undefined): ThemePref {
  return isThemePref(stored) ? stored : "system";
}

export function resolveTheme(pref: ThemePref, systemDark: boolean): ResolvedTheme {
  if (pref === "system") return systemDark ? "dark" : "light";
  return pref;
}

/** Sets the `dark` class Tailwind's dark: variant keys on, color-scheme and theme-color. */
export function applyTheme(doc: Document, theme: ResolvedTheme): void {
  const root = doc.documentElement;
  const dark = isDarkTheme(theme);
  root.classList.toggle("dark", dark);
  // The palette (index.css :root[data-theme=…]); light and dark need none.
  if (theme === "light" || theme === "dark") delete root.dataset.theme;
  else root.dataset.theme = theme;
  root.style.colorScheme = dark ? "dark" : "light"; // native controls and scrollbars follow
  for (const meta of doc.querySelectorAll<HTMLMetaElement>('meta[name="theme-color"]')) {
    meta.content = THEME_COLORS[theme];
  }
}

function readStored(): string | null {
  try {
    return globalThis.localStorage.getItem(THEME_STORAGE_KEY); // throws when storage is blocked
  } catch {
    return null;
  }
}

const systemDark = () => typeof window.matchMedia === "function" && window.matchMedia(DARK_QUERY).matches;

let pref: ThemePref = parseTheme(readStored());
const listeners = new Set<() => void>();
const notify = () => { for (const l of listeners) l(); };

function refresh(): void {
  applyTheme(document, resolveTheme(pref, systemDark()));
  notify();
}

export const currentTheme = (): ThemePref => pref;

/** Stores and applies the choice at once; with storage blocked it lasts until a reload. */
export function setTheme(next: ThemePref): void {
  pref = next;
  try {
    if (next === "system") globalThis.localStorage.removeItem(THEME_STORAGE_KEY);
    else globalThis.localStorage.setItem(THEME_STORAGE_KEY, next);
  } catch {
    // storage blocked: still switch for this page
  }
  refresh();
}

let started = false;

/**
 * Keeps the page in step with the OS setting (while "system" is chosen) and
 * with other tabs. Called once at start-up, before the first render.
 */
export function startTheme(): void {
  if (started) return;
  started = true;
  pref = parseTheme(readStored());
  refresh();
  if (typeof window.matchMedia === "function") {
    window.matchMedia(DARK_QUERY).addEventListener("change", () => { if (pref === "system") refresh(); });
  }
  window.addEventListener("storage", (e) => {
    if (e.key !== THEME_STORAGE_KEY && e.key !== null) return;
    pref = parseTheme(readStored());
    refresh();
  });
}

export function useTheme(): ThemePref {
  return useSyncExternalStore(
    (onChange) => {
      listeners.add(onChange);
      return () => { listeners.delete(onChange); };
    },
    currentTheme,
    () => "system" as const,
  );
}
