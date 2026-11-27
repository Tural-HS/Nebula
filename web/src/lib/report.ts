import type { Item, StatusCategory } from "../api/types";
import { daysLate, lateLevel, type LateLevel } from "./overdue";

// Monthly report panel arithmetic (ADR-0066), kept pure for tests.

export interface ReportNumbers {
  plan: number;
  remaining: number;
  paid: number;
  /** Share of the plan collected, 0-100 (rounded). */
  percent: number;
  /** Expected by month end: the plan minus what overdue items still owe. */
  forecast: number;
  forecastPercent: number;
}

const num = (v: unknown): number => {
  const n = typeof v === "number" ? v : typeof v === "string" ? Number(v) : 0;
  return Number.isFinite(n) ? n : 0;
};

export function reportNumbers(plan: unknown, remaining: unknown, overdueRemaining: number): ReportNumbers {
  const p = num(plan);
  const r = num(remaining);
  const paid = p - r;
  const forecast = p - overdueRemaining;
  const pct = (v: number) => (p > 0 ? Math.round((v / p) * 100) : 0);
  return { plan: p, remaining: r, paid, percent: pct(paid), forecast, forecastPercent: pct(forecast) };
}

/** Overdue items by band (1–15, 16–30, 31+ days), their unpaid total and the longest-late one. */
export function overdueBreakdown(items: readonly Item[], remainingField: string, today: string) {
  const bands: Record<Exclude<LateLevel, 0>, number> = { 1: 0, 2: 0, 3: 0 };
  let owed = 0;
  let worst: { item: Item; days: number } | null = null;
  for (const it of items) {
    if (!it.overdue || !it.due_date) continue;
    const days = Math.max(1, daysLate(it.due_date, today));
    const level = lateLevel(days);
    if (level) bands[level]++;
    owed += num(it.fields[remainingField]);
    if (!worst || days > worst.days) worst = { item: it, days };
  }
  return { bands, owed, worst };
}

/** Items per status category from per-status counts. */
export function categoryCounts(byStatus: Readonly<Record<string, number>> | undefined, category: ReadonlyMap<string, StatusCategory>) {
  const out: Record<StatusCategory, number> = { todo: 0, in_progress: 0, done: 0, canceled: 0 };
  for (const [id, n] of Object.entries(byStatus ?? {})) {
    const c = category.get(id);
    if (c) out[c] += n;
  }
  return out;
}
