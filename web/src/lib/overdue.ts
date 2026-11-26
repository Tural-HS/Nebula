// Overdue severity (ADR-0057): how long an open item has been past its due
// date, in three bands. 1–15 days late is yellow, 16–30 orange, 31 or more
// red. The server decides whether an item is overdue (ADR-0009); this only
// grades it, on plain YYYY-MM-DD dates so no time zone can shift a day.

export type LateLevel = 0 | 1 | 2 | 3;

const DAY_MS = 86_400_000;

/** Whole days from `due` to `day` (positive when `day` is later). */
export function daysLate(due: string, day: string): number {
  return Math.round((Date.parse(day + "T00:00:00Z") - Date.parse(due + "T00:00:00Z")) / DAY_MS);
}

/** The band for a number of days late: 0 when not late. */
export function lateLevel(days: number): LateLevel {
  if (days <= 0) return 0;
  if (days <= 15) return 1;
  if (days <= 30) return 2;
  return 3;
}

/** Today's date in the browser's own time zone (YYYY-MM-DD). */
export function localToday(now: Date = new Date()): string {
  const y = now.getFullYear();
  const m = String(now.getMonth() + 1).padStart(2, "0");
  const d = String(now.getDate()).padStart(2, "0");
  return `${String(y)}-${m}-${d}`;
}

/** Chip colours per band (text, background, ring), light and dark. */
export const LATE_CHIP: Record<Exclude<LateLevel, 0>, string> = {
  1: "bg-yellow-100 font-medium text-yellow-900 ring-1 ring-yellow-400 dark:bg-yellow-400/25 dark:text-yellow-100 dark:ring-yellow-500/70",
  2: "bg-orange-100 font-medium text-orange-900 ring-1 ring-orange-400 dark:bg-orange-500/30 dark:text-orange-100 dark:ring-orange-500/80",
  3: "bg-red-100 font-medium text-red-800 ring-1 ring-red-400 dark:bg-red-600/35 dark:text-red-100 dark:ring-red-500/80",
};

/** A left edge in the band's colour, for cards. */
export const LATE_EDGE: Record<Exclude<LateLevel, 0>, string> = {
  1: "border-l-4 border-l-yellow-400",
  2: "border-l-4 border-l-orange-500",
  3: "border-l-4 border-l-red-600",
};

/** One day an item spent overdue, for the calendar. */
export interface LateDay<T> {
  item: T;
  days: number;
  level: Exclude<LateLevel, 0>;
}

/**
 * The days each overdue item has been late, from the day after its due date
 * up to and including today, limited to [first, last]. The calendar marks
 * every one of them in that day's band colour.
 */
export function lateDays<T extends { due_date: string | null; overdue: boolean }>(
  items: readonly T[], first: string, last: string, today: string,
): Map<string, LateDay<T>[]> {
  const out = new Map<string, LateDay<T>[]>();
  const end = today < last ? today : last;
  for (const item of items) {
    if (!item.overdue || !item.due_date) continue;
    const startAt = Math.max(1, daysLate(item.due_date, first));
    const stop = daysLate(item.due_date, end);
    for (let n = startAt; n <= stop; n++) {
      const day = addDays(item.due_date, n);
      const level = lateLevel(n);
      if (level === 0) continue;
      const list = out.get(day);
      const entry = { item, days: n, level };
      if (list) list.push(entry);
      else out.set(day, [entry]);
    }
  }
  return out;
}

function addDays(day: string, n: number): string {
  const d = new Date(day + "T00:00:00Z");
  d.setUTCDate(d.getUTCDate() + n);
  return d.toISOString().slice(0, 10);
}
