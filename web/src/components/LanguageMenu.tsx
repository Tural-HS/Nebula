import { activeLocale, isLocale, LOCALE_NAMES, LOCALES, switchLocale } from "../i18n/locale";
import { t } from "../i18n";
import { cn } from "../lib/cn";

/**
 * Language picker (ADR-0025) with short codes. The header shows it from tablet
 * width up; phones have it on the workspace list, so the header keeps its room
 * for the breadcrumb (ADR-0019).
 */
export function LanguageMenu({ className }: { className?: string }) {
  return (
    <select
      aria-label={t.language}
      title={t.languageHint}
      value={activeLocale()}
      onChange={(e) => {
        if (isLocale(e.target.value)) switchLocale(e.target.value);
      }}
      className={cn(
        "h-8 rounded-md bg-transparent px-1.5 text-xs font-medium text-zinc-600 ring-1 ring-zinc-300 hover:text-zinc-900 focus-visible:ring-2 focus-visible:ring-brand-500 focus-visible:outline-none dark:text-zinc-300 dark:ring-zinc-700 dark:hover:text-zinc-50",
        className,
      )}
    >
      {LOCALES.map((l) => (
        <option key={l} value={l} lang={l} aria-label={LOCALE_NAMES[l]}>
          {l.toUpperCase()}
        </option>
      ))}
    </select>
  );
}
