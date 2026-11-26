import { tth } from "../i18n-theme";
import { cn } from "../lib/cn";
import { isThemePref, setTheme, THEMES, useTheme, type ThemePref } from "../lib/theme";

const ICONS: Record<ThemePref, string> = { system: "◐", light: "☀", dark: "☾", green: "🌿", paper: "📜", midnight: "✦", evergreen: "🌲", oled: "●", dusk: "🌆", contrast: "◑" };

/**
 * Colour theme picker (ADR-0035), beside the language picker. The header shows
 * it from xl up (a 1024 px tablet header has no room left, ADR-0019); smaller
 * screens have it on the workspace list. Changes apply without a reload.
 */
export function ThemeMenu({ className, labelled = false }: { className?: string; labelled?: boolean }) {
  const theme = useTheme();
  return (
    <select
      aria-label={tth.theme}
      title={tth.themeHint}
      value={theme}
      onChange={(e) => {
        if (isThemePref(e.target.value)) setTheme(e.target.value);
      }}
      className={cn(
        "h-8 rounded-md bg-transparent px-1.5 text-xs font-medium text-zinc-600 ring-1 ring-zinc-500 hover:text-zinc-900 focus-visible:ring-2 focus-visible:ring-brand-500 focus-visible:outline-none dark:text-zinc-300 dark:hover:text-zinc-50",
        className,
      )}
    >
      {THEMES.map((th) => (
        <option key={th} value={th} aria-label={tth.themes[th]}>
          {labelled ? `${ICONS[th]} ${tth.themes[th]}` : ICONS[th]}
        </option>
      ))}
    </select>
  );
}
