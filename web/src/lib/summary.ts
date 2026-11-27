import type { SummaryItem } from "../api/hooks";
import type { FieldSums } from "../api/types";

/** A summary entry's value: the sum of its field, minus the sum of minus (ADR-0064). */
export function summaryValue(it: SummaryItem, sums: FieldSums | undefined): number | null {
  const sum = (k: string) => {
    const v = sums?.[k]?.sum;
    return v === undefined ? 0 : Number(v);
  };
  if (!sums) return null;
  const v = sum(it.field) - (it.minus ? sum(it.minus) : 0);
  return Number.isFinite(v) ? v : null;
}
