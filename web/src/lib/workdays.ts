// Working-day calendar helpers (ADR-0027). The server computes recurrence
// dates; the UI only needs to know, per day, whether it is a weekend, a
// holiday or a transferred working day, to shade the calendar view and to
// validate the settings form. Dates are plain YYYY-MM-DD strings (UTC
// arithmetic, see lib/calendar.ts).

import type { CalendarDay, WorkCalendar } from "../api/calendar";

export type DayKind = "holiday" | "workday" | "weekend" | null;

export interface DayInfo {
  /** holiday: non-working date; workday: a weekend date declared working; weekend: a plain weekend day. */
  kind: DayKind;
  name: string | null;
  working: boolean;
}

/** JS/Go weekday (0 = Sunday) of a YYYY-MM-DD date. */
export const jsWeekday = (day: string) => new Date(day + "T00:00:00Z").getUTCDay();

/** Default when the calendar has not loaded: Saturday and Sunday. */
export const DEFAULT_WEEKEND: readonly number[] = [6, 0];

export interface DayLookup {
  info: (day: string) => DayInfo;
  isWeekend: (jsDay: number) => boolean;
}

export function dayLookup(cal: Pick<WorkCalendar, "weekend_days" | "days"> | undefined): DayLookup {
  const weekend = new Set(cal?.weekend_days ?? DEFAULT_WEEKEND);
  const byDate = new Map<string, CalendarDay>((cal?.days ?? []).map((d) => [d.date, d]));
  return {
    isWeekend: (jsDay) => weekend.has(jsDay),
    info: (day) => {
      const entry = byDate.get(day);
      if (entry) return { kind: entry.kind, name: entry.name, working: entry.kind === "workday" };
      if (weekend.has(jsWeekday(day))) return { kind: "weekend", name: null, working: false };
      return { kind: null, name: null, working: true };
    },
  };
}

/** Monday-first weekday numbers, as the server returns them. */
export function sortWeekdays(days: readonly number[]): number[] {
  return [...new Set(days)].sort((a, b) => ((a + 6) % 7) - ((b + 6) % 7));
}

/** Toggles a weekday in a weekend set, refusing more than max days. */
export function toggleWeekend(days: readonly number[], day: number, max: number): number[] {
  if (days.includes(day)) return days.filter((d) => d !== day);
  if (days.length >= max) return [...days];
  return sortWeekdays([...days, day]);
}

/** Why a new entry would be rejected by the server, or null. */
export function entryProblem(
  entry: { date: string; kind: CalendarDay["kind"]; name: string },
  weekend: readonly number[],
  yearCount: number,
  max: number,
  exists: boolean,
): "date" | "name" | "workdayNeedsWeekend" | "limit" | null {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(entry.date) || Number.isNaN(Date.parse(entry.date + "T00:00:00Z"))) return "date";
  const y = Number(entry.date.slice(0, 4));
  if (y < 2000 || y > 2100) return "date";
  const name = entry.name.trim();
  // Code points, like the server (utf8.RuneCountInString).
  if (name.length === 0 || Array.from(name).length > 100) return "name";
  if (entry.kind === "workday" && !weekend.includes(jsWeekday(entry.date))) return "workdayNeedsWeekend";
  if (!exists && yearCount >= max) return "limit";
  return null;
}

/** Days of one year, sorted. */
export function daysOfYear(days: readonly CalendarDay[], year: number): CalendarDay[] {
  const prefix = `${String(year)}-`;
  return days.filter((d) => d.date.startsWith(prefix)).sort((a, b) => a.date.localeCompare(b.date));
}
