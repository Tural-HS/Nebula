import { useQueries } from "@tanstack/react-query";
import { useMemo, useState } from "react";

import { api, path } from "../api/client";
import { flattenPages } from "../lib/board";
import { itemsQueryString, useItemCounts, useItems, useItemTotals, type ItemFilters, type ReportConfig } from "../api/hooks";
import type { FieldDefinition, FieldSums, Status, StatusCategory } from "../api/types";
import { monthOf, monthTitle } from "../lib/calendar";
import { LATE_CHIP } from "../lib/overdue";
import { categoryCounts, overdueBreakdown, reportNumbers } from "../lib/report";
import { emptyState, filtersOf, formatDecimal } from "../lib/views";
import { tv } from "../i18n-views";

const money = (v: number) => formatDecimal(Math.round(v));
// Every live item of the project, whatever the table shows (no queue, search or column filter).
const REPORT_FILTERS: ItemFilters = filtersOf(emptyState(), "");
const DAYS_LEFT = (today: string) => {
  const [y = 1970, m = 1, d = 1] = today.split("-").map(Number);
  return new Date(Date.UTC(y, m, 0)).getUTCDate() - d;
};

/** The monthly report panel above the table (ADR-0066): plan, collected and
 * remaining, customers by state and lateness, today's payments, the biggest
 * debt and the plan's split. A fixed project-wide report: the table's filters
 * and queues do not change it. */
export function ReportPanel({ ws, project, config, statuses, fields, today, onOpen }: {
  ws: string; project: string; config: ReportConfig; statuses: readonly Status[];
  fields: ReadonlyMap<string, FieldDefinition>; today: string; onOpen: (n: number) => void;
}) {
  const filters = REPORT_FILTERS;
  const { plan_field: plan, remaining_field: rest, breakdown_field: split } = config;
  const totals = useItemTotals(ws, project, filters, [plan, rest]);
  const counts = useItemCounts(ws, project, filters);
  const overdueQ = useItems(ws, project, { ...filters, due: "overdue", sort: "due" });
  const todayQ = useItems(ws, project, { ...filters, due: "today", sort: "number" });
  const choices = (split ? fields.get(split)?.options.choices ?? [] : []).filter((c) => !c.archived);
  const splitQ = useQueries({
    queries: choices.map((c) => ({
      queryKey: ["ws", ws, "p", project, "items", "report-split", filters, split, c.id, plan],
      queryFn: () => {
        const p = new URLSearchParams(itemsQueryString({ ...filters, fields: { ...(filters.fields ?? {}), [split ?? ""]: c.id } }, null));
        p.delete("limit");
        p.delete("sort");
        p.set("fields", plan);
        return api<{ sums: FieldSums }>("GET", path`/workspaces/${ws}/projects/${project}/items/sums` + "?" + p.toString());
      },
    })),
  });

  const category = useMemo(() => new Map<string, StatusCategory>(statuses.map((s) => [s.id, s.category])), [statuses]);
  const cats = categoryCounts(counts.data?.by_status, category);
  const overdue = overdueBreakdown(flattenPages(overdueQ.data?.pages ?? []), rest, today);
  const lateCount = overdue.bands[1] + overdue.bands[2] + overdue.bands[3];
  const n = reportNumbers(totals.data?.sums[plan]?.sum, totals.data?.sums[rest]?.sum, overdue.owed);
  const dueToday = flattenPages(todayQ.data?.pages ?? []).filter((it) => it.status.category === "todo" || it.status.category === "in_progress");
  const todaySum = dueToday.reduce((s, it) => s + (Number(it.fields[rest]) || 0), 0);
  const splitParts = choices.map((c, i) => ({ label: c.label, sum: Number(splitQ[i]?.data?.sums[plan]?.sum ?? 0) }));
  const splitTotal = splitParts.reduce((s, x) => s + x.sum, 0);
  const loading = totals.isPending || counts.isPending;

  const [open, setOpen] = useState(readOpen);
  const toggle = () => { const next = !open; setOpen(next); writeOpen(next); };
  const month = monthTitle(monthOf(today));
  const late = lateCount > 0 && (
    <span className="rounded-full bg-red-50 px-2 py-0.5 text-xs text-red-800 dark:bg-red-950/60 dark:text-red-300">{tv.reportLate(lateCount)}</span>
  );
  const chevron = (
    <button type="button" onClick={toggle} aria-expanded={open} aria-label={open ? tv.reportCollapse : tv.reportExpand}
      className="ml-auto grid size-8 shrink-0 place-items-center rounded-md text-zinc-500 hover:bg-zinc-100 dark:text-zinc-400 dark:hover:bg-zinc-800">
      <svg aria-hidden viewBox="0 0 20 20" className={`size-5 transition-transform ${open ? "rotate-180" : ""}`} fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round"><path d="m5 8 5 5 5-5" /></svg>
    </button>
  );

  if (!open) {
    return (
      <section aria-label={tv.reportTitle(month)} data-testid="report-panel" className="px-3 pt-2 md:px-4">
        <div className="flex flex-wrap items-center gap-x-4 gap-y-1 rounded-xl bg-white px-4 py-1.5 text-sm ring-1 ring-zinc-200 dark:bg-zinc-900 dark:ring-zinc-800">
          <span className="font-semibold">{month}</span>
          <span className="h-1.5 w-28 overflow-hidden rounded-full bg-zinc-200 dark:bg-zinc-800" aria-hidden>
            <span className="block h-full bg-emerald-600 dark:bg-emerald-500" style={{ width: `${String(Math.min(100, n.percent))}%` }} />
          </span>
          <span className="font-semibold tabular-nums">{loading ? "…" : `${String(n.percent)}%`}</span>
          <Stat inline label={tv.reportPlanShort} value={loading ? "…" : money(n.plan)} />
          <Stat inline label={tv.reportPaid} value={loading ? "…" : money(n.paid)} tone="paid" />
          <Stat inline label={tv.reportRemainingShort} value={loading ? "…" : money(n.remaining)} tone="rest" />
          {late}
          {chevron}
        </div>
      </section>
    );
  }

  return (
    <section aria-label={tv.reportTitle(month)} data-testid="report-panel" className="px-3 pt-3 md:px-4">
      <div className="rounded-xl bg-white px-4 py-3 text-sm ring-1 ring-zinc-200 dark:bg-zinc-900 dark:ring-zinc-800">
        <div className="flex flex-wrap items-center gap-x-5 gap-y-3">
          <div className="flex min-w-48 items-center gap-3">
            <Ring percent={n.percent} forecast={n.forecastPercent} label={loading ? "…" : `${String(n.percent)}%`} />
            <div>
              <h2 className="text-[15px] font-semibold">{month}</h2>
              <p className="text-xs text-zinc-500 dark:text-zinc-400">{tv.reportForecastShort(n.forecastPercent)} · {tv.reportDaysLeft(DAYS_LEFT(today))}</p>
            </div>
          </div>
          <div className="flex min-w-72 flex-1 divide-x divide-zinc-200 dark:divide-zinc-800">
            <Stat label={tv.reportPlan} value={loading ? "…" : money(n.plan)} />
            <Stat label={tv.reportPaid} value={loading ? "…" : money(n.paid)} tone="paid" />
            <Stat label={tv.reportRemaining} value={loading ? "…" : money(n.remaining)} tone="rest" />
          </div>
          {chevron}
        </div>
        <div className="mt-3 flex flex-wrap items-center justify-between gap-x-4 gap-y-2 border-t border-zinc-200 pt-2.5 dark:border-zinc-800">
          <div className="flex flex-wrap gap-1.5 text-xs">
            <span className="rounded-full bg-emerald-50 px-2.5 py-0.5 text-emerald-800 dark:bg-emerald-950/50 dark:text-emerald-300">✓ {tv.reportPaidCount(cats.done)}</span>
            <span className="rounded-full bg-zinc-100 px-2.5 py-0.5 text-zinc-700 dark:bg-zinc-800 dark:text-zinc-300">{tv.reportWaiting(Math.max(0, cats.todo + cats.in_progress - lateCount))}</span>
            {([1, 2, 3] as const).map((b) => overdue.bands[b] > 0 && (
              <span key={b} className={`rounded-full px-2.5 py-0.5 ${LATE_CHIP[b]}`}>{tv.reportBand(overdue.bands[b], b)}</span>
            ))}
          </div>
          <div className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-zinc-500 dark:text-zinc-400">
            <span>🔔 {tv.reportToday}: <b className="font-medium text-zinc-900 tabular-nums dark:text-zinc-100">{dueToday.length} · {money(todaySum)}</b></span>
            {overdue.worst && (
              <span>⚠️ {tv.reportWorst}:{" "}
                <button type="button" className="font-medium text-red-700 hover:underline dark:text-red-400" onClick={() => { if (overdue.worst) onOpen(overdue.worst.item.number); }}>
                  {overdue.worst.item.key} · {tv.reportDays(overdue.worst.days)}
                </button>
              </span>
            )}
            {splitParts.length > 0 && splitTotal > 0 && (
              <span>💵 {splitParts.map((p, i) => (
                <span key={p.label}>{i > 0 && " · "}{p.label} <b className="font-medium text-zinc-900 tabular-nums dark:text-zinc-100">{String(Math.round((p.sum / splitTotal) * 100))}%</b></span>
              ))}</span>
            )}
          </div>
        </div>
      </div>
    </section>
  );
}

const OPEN_KEY = "nebula.report.open";

function readOpen(): boolean {
  try {
    return globalThis.localStorage.getItem(OPEN_KEY) !== "0";
  } catch {
    return true;
  }
}

function writeOpen(open: boolean): void {
  try {
    globalThis.localStorage.setItem(OPEN_KEY, open ? "1" : "0");
  } catch {
    // storage blocked: the choice lasts for this page
  }
}

/** One amount: label with a tone dot and the figure. */
function Stat({ label, value, tone, inline = false }: { label: string; value: string; tone?: "paid" | "rest"; inline?: boolean }) {
  const dot = tone === "paid" ? "bg-emerald-600" : tone === "rest" ? "bg-amber-600" : "bg-zinc-400";
  const fig = tone === "paid" ? "text-emerald-800 dark:text-emerald-300" : tone === "rest" ? "text-amber-800 dark:text-amber-300" : "text-zinc-900 dark:text-zinc-50";
  if (inline) {
    return <span className="text-zinc-500 dark:text-zinc-400">{label} <b className={`font-semibold tabular-nums ${fig}`}>{value}</b></span>;
  }
  return (
    <div className="flex-1 px-4 first:pl-0 md:first:pl-4">
      <div className="flex items-center gap-1.5 text-xs text-zinc-500 dark:text-zinc-400"><span aria-hidden className={`size-1.5 rounded-full ${dot}`} />{label}</div>
      <div className={`text-xl font-semibold tabular-nums ${fig}`}>{value}</div>
    </div>
  );
}

/** Collected share as a ring; the lighter arc runs on to the forecast. */
function Ring({ percent, forecast, label }: { percent: number; forecast: number; label: string }) {
  const r = 24;
  const c = 2 * Math.PI * r;
  const arc = (p: number) => `${String((Math.max(0, Math.min(100, p)) / 100) * c)} ${String(c)}`;
  return (
    <svg width="58" height="58" viewBox="0 0 58 58" role="img" aria-label={label} className="shrink-0">
      <circle cx="29" cy="29" r={r} fill="none" strokeWidth="6" className="stroke-zinc-200 dark:stroke-zinc-800" />
      <circle cx="29" cy="29" r={r} fill="none" strokeWidth="6" strokeDasharray={arc(forecast)} transform="rotate(-90 29 29)" className="stroke-emerald-200 dark:stroke-emerald-900" />
      <circle cx="29" cy="29" r={r} fill="none" strokeWidth="6" strokeLinecap="round" strokeDasharray={arc(percent)} transform="rotate(-90 29 29)" className="stroke-emerald-600 dark:stroke-emerald-500" />
      <text x="29" y="33" textAnchor="middle" className="fill-zinc-900 text-[13px] font-semibold dark:fill-zinc-50">{label}</text>
    </svg>
  );
}
