import { useItemTotals, type ItemFilters, type SummaryItem } from "../api/hooks";
import { summaryValue } from "../lib/summary";
import { monthOf, monthTitle } from "../lib/calendar";
import { formatDecimal } from "../lib/views";
import { tv } from "../i18n-views";

const ICONS = ["💰", "📊", "💵", "🧾", "📈", "📌"];

/** The project's summary bar above the table: this month and the configured totals of the listed items. */
export function SummaryBar({ ws, project, filters, items, today }: {
  ws: string; project: string; filters: ItemFilters; items: readonly SummaryItem[]; today: string;
}) {
  const keys = [...new Set(items.flatMap((it) => (it.minus ? [it.field, it.minus] : [it.field])))];
  const totals = useItemTotals(ws, project, filters, keys, items.length > 0);
  if (items.length === 0) return null;
  return (
    <div role="status" aria-live="polite" data-testid="summary-bar"
      className="flex flex-wrap items-center gap-x-3 gap-y-1 border-b border-zinc-200 bg-brand-50/60 px-4 py-2 text-sm dark:border-zinc-800 dark:bg-zinc-900">
      <span className="font-semibold">📅 {tv.summaryTitle(monthTitle(monthOf(today)))}</span>
      {items.map((it, i) => {
        const v = summaryValue(it, totals.data?.sums);
        return (
          <span key={it.label + String(i)} className="whitespace-nowrap">
            <span aria-hidden className="text-zinc-400">· </span>
            {ICONS[i % ICONS.length]} {it.label}:{" "}
            <strong className="tabular-nums">{v === null ? "…" : formatDecimal(v.toFixed(Number.isInteger(v) ? 0 : 2))}{it.unit ? `\u00a0${it.unit}` : ""}</strong>
          </span>
        );
      })}
    </div>
  );
}
