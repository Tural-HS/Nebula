import type { ScheduleInput } from "../api/recurrences";

/** Monday-first, as in Azerbaijan; values are JS/Go weekday numbers (0 = Sunday). */
export const WEEKDAYS: readonly { value: number; short: string; long: string }[] = [
  { value: 1, short: "B.e.", long: "bazar ertəsi" },
  { value: 2, short: "Ç.a.", long: "çərşənbə axşamı" },
  { value: 3, short: "Ç.", long: "çərşənbə" },
  { value: 4, short: "C.a.", long: "cümə axşamı" },
  { value: 5, short: "C.", long: "cümə" },
  { value: 6, short: "Ş.", long: "şənbə" },
  { value: 0, short: "B.", long: "bazar" },
];

const MONTHS = ["yanvar", "fevral", "mart", "aprel", "may", "iyun", "iyul", "avqust", "sentyabr", "oktyabr", "noyabr", "dekabr"] as const;

/** Same placeholders as the server (store.RenderRecurrenceTitle). */
export function renderTitle(tmpl: string, date: string): string {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(date);
  if (!m) return tmpl;
  const [, y = "", mo = "", d = ""] = m;
  return tmpl
    .replaceAll("{{date}}", `${d}.${mo}.${y}`)
    .replaceAll("{{month}}", MONTHS[Number(mo) - 1] ?? "")
    .replaceAll("{{year}}", y);
}

export const MONTHS_GENITIVE = ["yanvarın", "fevralın", "martın", "aprelin", "mayın", "iyunun", "iyulun", "avqustun", "sentyabrın", "oktyabrın", "noyabrın", "dekabrın"] as const;

// Possessive suffix after a number follows how the number is read:
// bir-i, iki-si, üç-ü, dörd-ü, beş-i, altı-sı, yeddi-si, səkkiz-i, doqquz-u, on-u, iyirmi-si, otuz-u.
const DIGIT_SUFFIX = ["", "i", "si", "ü", "ü", "i", "sı", "si", "i", "u"] as const;
const TENS_SUFFIX: Record<number, string> = { 1: "u", 2: "si", 3: "u" };

export function dayWithSuffix(n: number): string {
  const suffix = n % 10 === 0 ? (TENS_SUFFIX[n / 10] ?? "u") : DIGIT_SUFFIX[n % 10];
  return `${String(n)}-${suffix ?? "i"}`;
}

function dayLabel(day: number | null): string {
  return day === -1 ? "son günü" : dayWithSuffix(day ?? 1);
}

/** "Hər ayın 9-u" style one-line summary of a schedule. */
export function describeSchedule(s: Pick<ScheduleInput, "freq" | "every" | "weekdays" | "month_day" | "month">): string {
  const n = s.every;
  switch (s.freq) {
    case "daily":
      return n === 1 ? "Hər gün" : `Hər ${String(n)} gündən bir`;
    case "weekly": {
      const days = WEEKDAYS.filter((d) => s.weekdays.includes(d.value)).map((d) => d.long).join(", ");
      return `${n === 1 ? "Hər həftə" : `Hər ${String(n)} həftədən bir`}: ${days || "—"}`;
    }
    case "monthly":
      return n === 1 ? `Hər ayın ${dayLabel(s.month_day)}` : `Hər ${String(n)} aydan bir, ayın ${dayLabel(s.month_day)}`;
    case "yearly": {
      const month = MONTHS_GENITIVE[(s.month ?? 1) - 1] ?? "";
      return `${n === 1 ? "Hər il" : `Hər ${String(n)} ildən bir`}, ${month} ${dayLabel(s.month_day)}`;
    }
  }
}
