// Calendar view helpers (ADR-0023): month grids, Monday first, on plain
// YYYY-MM-DD dates. Due dates are calendar dates in the workspace's time
// zone (ADR-0009), so all arithmetic here is on UTC dates: no local time
// zone or DST can shift a day.

import type { Item } from "../api/types";
import { tc } from "../i18n-calendar";

/** A month as "YYYY-MM". */
export type Month = string;

const MONTH = /^(\d{4})-(0[1-9]|1[0-2])$/;
const DAY = /^\d{4}-\d{2}-\d{2}$/;

/** The month in the URL (?cal=YYYY-MM) or null when absent or malformed. */
export function parseMonth(v: string | null | undefined): Month | null {
  const m = v ? MONTH.exec(v) : null;
  if (!m) return null;
  const y = Number(m[1]);
  return y >= 1900 && y <= 2200 ? v ?? null : null;
}

/** Today's date (YYYY-MM-DD) in an IANA time zone; falls back to UTC for an unknown zone. */
export function todayIn(timeZone: string | undefined, now: Date = new Date()): string {
  try {
    // en-CA formats as YYYY-MM-DD.
    return new Intl.DateTimeFormat("en-CA", { timeZone: timeZone || "UTC", year: "numeric", month: "2-digit", day: "2-digit" }).format(now);
  } catch {
    return now.toISOString().slice(0, 10);
  }
}

const utc = (d: string) => new Date(d + "T00:00:00Z");
const iso = (d: Date) => d.toISOString().slice(0, 10);

export function addDays(day: string, n: number): string {
  const d = utc(day);
  d.setUTCDate(d.getUTCDate() + n);
  return iso(d);
}

export function shiftMonth(m: Month, n: number): Month {
  const [y = 1970, mo = 1] = m.split("-").map(Number);
  const d = new Date(Date.UTC(y, mo - 1 + n, 1));
  return iso(d).slice(0, 7);
}

export const monthOf = (day: string): Month => day.slice(0, 7);

/** Monday = 0 … Sunday = 6. */
export const weekday = (day: string) => (utc(day).getUTCDay() + 6) % 7;

export interface MonthGrid {
  /** Every day shown, whole weeks Monday to Sunday (5 or 6 rows). */
  days: string[];
  first: string;
  last: string;
}

/** The weeks that cover a month, Monday first. */
export function monthGrid(m: Month): MonthGrid {
  const start = m + "-01";
  const first = addDays(start, -weekday(start));
  const end = addDays(shiftMonth(m, 1) + "-01", -1);
  const last = addDays(end, 6 - weekday(end));
  const days: string[] = [];
  for (let d = first; d <= last; d = addDays(d, 1)) days.push(d);
  return { days, first, last };
}

/** Days of the month itself (the phone agenda lists only these). */
export function monthDays(m: Month): string[] {
  const out: string[] = [];
  for (let d = m + "-01"; monthOf(d) === m; d = addDays(d, 1)) out.push(d);
  return out;
}

/** Items by due date, each day ordered as given (the list is sorted by due, then number). */
export function byDay(items: readonly Item[]): Map<string, Item[]> {
  const out = new Map<string, Item[]>();
  for (const it of items) {
    if (!it.due_date || !DAY.test(it.due_date)) continue;
    const list = out.get(it.due_date);
    if (list) list.push(it);
    else out.set(it.due_date, [it]);
  }
  return out;
}

/** Splits a day's items into the ones shown and how many are left for "+N more". */
export function overflow<T>(list: readonly T[], max: number): { shown: T[]; more: number } {
  if (list.length <= max) return { shown: [...list], more: 0 };
  // "+1 more" takes the place of an item, so it never hides just one.
  const keep = Math.max(0, max - 1);
  return { shown: list.slice(0, keep), more: list.length - keep };
}

/**
 * Whether an item would be overdue on a new due date, as the server decides
 * (ADR-0009): past due and not done or canceled. Used for the optimistic
 * card while a drop is saved.
 */
export function overdueOn(it: Pick<Item, "status">, due: string | null, today: string): boolean {
  return due !== null && due < today && (it.status.category === "todo" || it.status.category === "in_progress");
}

/** "Dekabr 2026". */
export function monthTitle(m: Month): string {
  const [y, mo] = m.split("-").map(Number);
  return `${tc.months[(mo ?? 1) - 1] ?? ""} ${String(y)}`;
}
