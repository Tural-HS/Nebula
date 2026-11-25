// Dates, times and sizes in the UI language (ADR-0025).
//
// Azerbaijani is formatted by hand: browser Intl data for "az" is incomplete in
// Chromium (it renders "2026 M11 23" and "-31 min"). English, Turkish and
// Russian use Intl, which is complete for them.
import { activeLocale, type Locale } from "../i18n/locale";

const AZ_MONTHS = ["yan", "fev", "mar", "apr", "may", "iyn", "iyl", "avq", "sen", "okt", "noy", "dek"] as const;

const pad = (n: number) => String(n).padStart(2, "0");

const BCP47: Record<Exclude<Locale, "az">, string> = { en: "en-GB", tr: "tr-TR", ru: "ru-RU" };

export function formatDateTime(iso: string, locale: Locale = activeLocale()): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "—";
  if (locale === "az") {
    return `${String(d.getDate())} ${AZ_MONTHS[d.getMonth()] ?? ""} ${String(d.getFullYear())}, ${pad(d.getHours())}:${pad(d.getMinutes())}`;
  }
  return new Intl.DateTimeFormat(BCP47[locale], { day: "numeric", month: "short", year: "numeric", hour: "2-digit", minute: "2-digit", hourCycle: "h23" }).format(d);
}

/**
 * A calendar date ("2026-11-20" → "20 noy 2026"). Parsed by hand: new Date()
 * would read it as UTC midnight and show the previous day west of Greenwich.
 */
export function formatDate(date: string, locale: Locale = activeLocale()): string {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(date);
  const month = m ? AZ_MONTHS[Number(m[2]) - 1] : undefined;
  if (!m || !month) return "—";
  if (locale === "az") return `${String(Number(m[3]))} ${month} ${m[1] ?? ""}`;
  const utc = new Date(Date.UTC(Number(m[1]), Number(m[2]) - 1, Number(m[3])));
  return new Intl.DateTimeFormat(BCP47[locale], { day: "numeric", month: "short", year: "numeric", timeZone: "UTC" }).format(utc);
}

type Unit = "year" | "month" | "day" | "hour" | "minute";

const UNITS: [unit: Unit, az: string, seconds: number][] = [
  ["year", "il", 365 * 24 * 3600],
  ["month", "ay", 30 * 24 * 3600],
  ["day", "gün", 24 * 3600],
  ["hour", "saat", 3600],
  ["minute", "dəqiqə", 60],
];

/** "5 dəqiqə əvvəl", "2 gün sonra", "indicə" (or the same in the UI language). */
export function formatRelative(iso: string, now: number = Date.now(), locale: Locale = activeLocale()): string {
  const t = new Date(iso).getTime();
  if (Number.isNaN(t)) return "—";
  const diff = Math.round((t - now) / 1000);
  const rtf = locale === "az" ? null : new Intl.RelativeTimeFormat(BCP47[locale], { numeric: "auto" });
  for (const [unit, az, secs] of UNITS) {
    const n = Math.floor(Math.abs(diff) / secs);
    if (n >= 1) return rtf ? rtf.format(diff < 0 ? -n : n, unit) : `${String(n)} ${az} ${diff < 0 ? "əvvəl" : "sonra"}`;
  }
  return rtf ? rtf.format(0, "second") : "indicə";
}

/** Human-readable size: 912 B, 14 KB, 3,4 MB (decimal separator of the UI language). */
export function formatBytes(n: number, locale: Locale = activeLocale()): string {
  if (n < 1024) return `${String(n)} B`;
  if (n < 1024 * 1024) return `${String(Math.round(n / 1024))} KB`;
  const mb = (n / (1024 * 1024)).toFixed(1);
  return `${locale === "en" ? mb : mb.replace(".", ",")} MB`;
}

/** Whole numbers with the UI language's digit grouping ("12 345", "12,345"). */
export function formatCount(n: number, locale: Locale = activeLocale()): string {
  return n.toLocaleString(locale === "az" ? "az" : BCP47[locale]);
}

/** A calendar date without the year ("2026-01-05" → "5 yan"), for chart axes. */
export function formatShortDate(date: string, locale: Locale = activeLocale()): string {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(date);
  const month = m ? AZ_MONTHS[Number(m[2]) - 1] : undefined;
  if (!m || !month) return "—";
  if (locale === "az") return `${String(Number(m[3]))} ${month}`;
  const utc = new Date(Date.UTC(Number(m[1]), Number(m[2]) - 1, Number(m[3])));
  return new Intl.DateTimeFormat(BCP47[locale], { day: "numeric", month: "short", timeZone: "UTC" }).format(utc);
}

/** The month of a calendar date ("2026-01-01" → "yan 2026"). */
export function formatMonthYear(date: string, locale: Locale = activeLocale()): string {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(date);
  const month = m ? AZ_MONTHS[Number(m[2]) - 1] : undefined;
  if (!m || !month) return "—";
  if (locale === "az") return `${month} ${m[1] ?? ""}`;
  const utc = new Date(Date.UTC(Number(m[1]), Number(m[2]) - 1, 1));
  return new Intl.DateTimeFormat(BCP47[locale], { month: "short", year: "numeric", timeZone: "UTC" }).format(utc);
}
