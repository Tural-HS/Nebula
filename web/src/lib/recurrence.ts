import type { ScheduleInput } from "../api/recurrences";
import { tr } from "../i18n-recur";
import { tw } from "../i18n-workdays";

/** Monday-first, as in Azerbaijan; values are JS/Go weekday numbers (0 = Sunday). */
const WEEKDAY_VALUES = [1, 2, 3, 4, 5, 6, 0] as const;

export const WEEKDAYS: readonly { value: number; short: string; long: string }[] = WEEKDAY_VALUES.map((value, i) => ({
  value,
  short: tr.weekdayShort[i] ?? "",
  long: tr.weekdayLong[i] ?? "",
}));

// Month names in {{month}} stay Azerbaijani whatever the UI language: the
// server renders the title the same way (store.RenderRecurrenceTitle).
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

/** "Hər ayın 9-u" style one-line summary of a schedule, in the UI language. */
export function describeSchedule(s: Pick<ScheduleInput, "freq" | "every" | "weekdays" | "month_day" | "month" | "work_day" | "shift">): string {
  const n = s.every;
  const workDay = s.work_day ?? null;
  if (workDay !== null && (s.freq === "monthly" || s.freq === "yearly")) {
    return s.freq === "monthly" ? tw.describe.monthly(n, workDay) : tw.describe.yearly(n, s.month ?? 1, workDay);
  }
  const shift = s.shift === "previous" || s.shift === "next" ? tw.describe.shift[s.shift] : "";
  switch (s.freq) {
    case "daily":
      return tr.describe.daily(n) + shift;
    case "weekly":
      return tr.describe.weekly(n, WEEKDAYS.filter((d) => s.weekdays.includes(d.value)).map((d) => d.long).join(", ") || "—") + shift;
    case "monthly":
      return tr.describe.monthly(n, s.month_day ?? 1) + shift;
    case "yearly":
      return tr.describe.yearly(n, s.month ?? 1, s.month_day ?? 1) + shift;
  }
}
