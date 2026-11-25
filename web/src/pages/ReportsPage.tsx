// /w/:ws/reports — workspace reports "Hesabatlar" (ADR-0041) for finance /
// back-office managers. One request (GET /reports) feeds every widget; the
// numbers only count projects the viewer can see. Filters live in the URL,
// so a report is a shareable link. Charts are hand-built SVG/CSS with a
// table or text alternative; colour never carries meaning alone.
import { useState, type ReactNode, type SyntheticEvent } from "react";
import { Link, useParams, useSearchParams } from "react-router";

import { useItemTypes, useMembers, useProjects, useWorkspace } from "../api/hooks";
import { downloadWorkloadCsv, useReports, type ReportApprovalRow, type ReportAssignee, type ReportProjectStatus, type Reports } from "../api/reports";
import type { StatusCategory } from "../api/types";
import { Layout } from "../components/Layout";
import { useToast } from "../components/toast-context";
import { Button, EmptyState, ErrorState, Input, Label, Select, Spinner } from "../components/ui";
import { t } from "../i18n";
import { activeLocale } from "../i18n/locale";
import { trp } from "../i18n-reports";
import { cn } from "../lib/cn";
import { formatDate, formatMonthYear, formatShortDate } from "../lib/format";
import {
  canDrillDown, durationParts, itemsHref, niceTicks, OPEN_CATEGORIES, PRESETS, presetRange, rangeError, readFilters, rowAssignee,
  sortWorkload, statusIds, todayIn, writeFilters, type Preset, type ReportFilters, type WorkloadSort,
} from "../lib/reports";
import { Popover } from "./ViewTools";
import { ReportAmounts } from "../components/ReportAmounts";

const CATEGORIES: readonly StatusCategory[] = ["todo", "in_progress", "done", "canceled"];

// Validated with the dataviz palette checks (ADR-0041): sky-700 / amber-600
// light, sky-600 / amber-600 dark; status categories add emerald-600 and a
// neutral gray (always labelled), canceled is hatched.
const SERIES = {
  created: "bg-sky-700 dark:bg-sky-600",
  completed: "bg-amber-600",
};
const CATEGORY_FILL: Record<StatusCategory, string> = {
  todo: "bg-zinc-400 dark:bg-zinc-500",
  in_progress: "bg-sky-700 dark:bg-sky-600",
  done: "bg-emerald-600",
  canceled: "bg-[repeating-linear-gradient(135deg,var(--color-zinc-400)_0_3px,transparent_3px_6px)] ring-1 ring-inset ring-zinc-400 dark:ring-zinc-500",
};

const card = "rounded-xl bg-white p-4 ring-1 ring-zinc-200 dark:bg-zinc-900 dark:ring-zinc-800";
const muted = "text-zinc-500 dark:text-zinc-400";
const linkClass = "rounded text-brand-700 underline-offset-2 hover:underline dark:text-brand-200";

export function ReportsPage() {
  const { ws = "" } = useParams();
  const [params, setParams] = useSearchParams();
  const workspace = useWorkspace(ws);
  const projects = useProjects(ws);
  const today = todayIn(workspace.data?.timezone ?? "UTC");
  const f = readFilters(params, today);
  const q = useReports(ws, f, workspace.isSuccess);
  const set = (next: ReportFilters) => {
    setParams(writeFilters(next), { replace: true });
  };

  const crumbs = [{ label: t.workspaces, to: "/" }, { label: workspace.data?.name ?? ws, to: `/w/${encodeURIComponent(ws)}` }, { label: trp.title }];
  if (workspace.isError) {
    return (
      <Layout crumbs={crumbs}>
        <ErrorState error={workspace.error} onRetry={() => void workspace.refetch()} />
      </Layout>
    );
  }
  const noProjects = projects.isSuccess && projects.data.length === 0;

  return (
    <Layout crumbs={crumbs}>
      <div className="h-full overflow-y-auto">
        <div className="mx-auto max-w-6xl space-y-4 p-4 md:p-6">
          <div className="flex flex-wrap items-start justify-between gap-2">
            <div className="min-w-0">
              <h1 className="text-lg font-semibold">{trp.title}</h1>
              <p className={cn("text-sm", muted)}>
                {trp.hint} {workspace.data && <span className="whitespace-nowrap">{trp.timezone(workspace.data.timezone)}</span>}
              </p>
            </div>
          </div>
          {noProjects ? (
            <EmptyState>{trp.noProjects}</EmptyState>
          ) : (
            <>
              <FilterBar ws={ws} f={f} today={today} onChange={set} />
              <div aria-live="polite" className="sr-only">{q.isFetching && !q.isPending ? trp.updating : ""}</div>
              <div className={cn("space-y-4 transition-opacity motion-reduce:transition-none", q.isPlaceholderData && "opacity-60")} aria-busy={q.isFetching}>
                <Widgets ws={ws} f={f} q={q} />
              </div>
            </>
          )}
        </div>
      </div>
    </Layout>
  );
}

type Query = ReturnType<typeof useReports>;

/** Every widget's loading and error state comes from the one shared request. */
function Widget({ title, hint, q, actions, children, testId }: {
  title: string; hint?: string; q: Query; actions?: ReactNode; children: (r: Reports) => ReactNode; testId?: string;
}) {
  return (
    <section className={card} aria-labelledby={testId ? `${testId}-h` : undefined} data-testid={testId}>
      <div className="mb-3 flex flex-wrap items-start justify-between gap-2">
        <div className="min-w-0">
          <h2 id={testId ? `${testId}-h` : undefined} className="font-semibold">{title}</h2>
          {hint && <p className={cn("text-xs", muted)}>{hint}</p>}
        </div>
        {actions}
      </div>
      {q.isPending ? <Spinner /> : q.isError ? <ErrorState error={q.error} onRetry={() => void q.refetch()} /> : children(q.data)}
    </section>
  );
}

function Widgets({ ws, f, q }: { ws: string; f: ReportFilters; q: Query }) {
  const single = q.data?.projects.length === 1 ? q.data.projects[0]?.key : undefined;
  const drill = canDrillDown(f);
  return (
    <>
      <KpiTiles ws={ws} f={f} q={q} project={drill ? single : undefined} />
      {!drill && <p className={cn("text-xs", muted)}>{trp.drillNoType}</p>}
      {drill && !single && q.isSuccess && <p className={cn("text-xs", muted)}>{trp.drillOneProject}</p>}
      <div className="grid gap-4 lg:grid-cols-2">
        <Widget title={trp.throughput} hint={q.data ? trp.throughputHint[q.data.range.bucket] : undefined} q={q} testId="w-throughput">
          {(r) => <Throughput r={r} />}
        </Widget>
        <Widget title={trp.aging} q={q} testId="w-aging">
          {(r) => <Aging r={r} />}
        </Widget>
      </div>
      {drill && single && (
        <Widget title={trp.amounts} hint={trp.amountsHint} q={q} testId="w-amounts">
          {(r) => <ReportAmounts ws={ws} project={single} statuses={r.status.find((p) => p.key === single)?.statuses ?? []} />}
        </Widget>
      )}
      <Widget title={trp.workload} hint={trp.workloadHint} q={q} testId="w-workload" actions={<CsvMenu ws={ws} f={f} />}>
        {(r) => <Workload ws={ws} f={f} rows={r.workload} drill={drill} openIds={new Map(r.status.map((p) => [p.key, statusIds(p.statuses, OPEN_CATEGORIES)]))} />}
      </Widget>
      <div className="grid gap-4 lg:grid-cols-2">
        <Widget title={trp.approvals} hint={trp.approvalsHint} q={q} testId="w-approvals">
          {(r) => <Approvals r={r} />}
        </Widget>
        <Widget title={trp.status} hint={trp.statusHint} q={q} testId="w-status">
          {(r) => <StatusDistribution ws={ws} f={f} rows={r.status} drill={drill} />}
        </Widget>
      </div>
    </>
  );
}

// ---------------------------------------------------------------------------
// Filters
// ---------------------------------------------------------------------------

function FilterBar({ ws, f, today, onChange }: { ws: string; f: ReportFilters; today: string; onChange: (f: ReportFilters) => void }) {
  const projects = useProjects(ws);
  const members = useMembers(ws);
  const types = useItemTypes(ws);
  const [custom, setCustom] = useState({ from: f.from, to: f.to });
  const err = rangeError(custom.from, custom.to);

  const pick = (preset: Preset) => {
    if (preset === "custom") {
      setCustom({ from: f.from, to: f.to });
      onChange({ ...f, preset, from: f.from, to: f.to });
      return;
    }
    onChange({ ...f, preset, ...presetRange(preset, today) });
  };
  const apply = (e: SyntheticEvent) => {
    e.preventDefault();
    if (!err) onChange({ ...f, preset: "custom", ...custom });
  };
  const toggleProject = (key: string, on: boolean) => {
    const next = on ? [...f.projects, key] : f.projects.filter((k) => k !== key);
    onChange({ ...f, projects: [...new Set(next)].sort() });
  };
  const projectsLabel = f.projects.length === 0 ? trp.allProjects : f.projects.length === 1 ? (f.projects[0] ?? "") : trp.nProjects(f.projects.length);
  const isDefault = f.preset === "this_month" && f.projects.length === 0 && !f.assignee && !f.type;

  return (
    <section aria-label={trp.filters} className={cn(card, "flex flex-wrap items-end gap-3")}>
      <div>
        <Label htmlFor="r-period">{trp.period}</Label>
        <Select id="r-period" value={f.preset} onChange={(e) => { pick(e.target.value as Preset); }}>
          {PRESETS.map((p) => <option key={p} value={p}>{trp.preset[p]}</option>)}
        </Select>
      </div>
      {f.preset === "custom" && (
        <form onSubmit={apply} className="flex flex-wrap items-end gap-2" noValidate>
          <div>
            <Label htmlFor="r-from">{trp.from}</Label>
            <Input id="r-from" type="date" value={custom.from} onChange={(e) => { setCustom({ ...custom, from: e.target.value }); }}
              aria-invalid={err !== null} aria-describedby={err ? "r-range-err" : undefined} />
          </div>
          <div>
            <Label htmlFor="r-to">{trp.to}</Label>
            <Input id="r-to" type="date" value={custom.to} onChange={(e) => { setCustom({ ...custom, to: e.target.value }); }}
              aria-invalid={err !== null} aria-describedby={err ? "r-range-err" : undefined} />
          </div>
          <Button type="submit" disabled={err !== null}>{trp.apply}</Button>
          {err && <p id="r-range-err" role="alert" className="basis-full text-xs text-red-700 dark:text-red-300">{trp.rangeError[err]}</p>}
        </form>
      )}
      <div>
        <span aria-hidden className="mb-1 block text-xs font-medium text-zinc-600 dark:text-zinc-400">{trp.projects}</span>
        <Popover label={projectsLabel} summaryLabel={`${trp.projects}: ${projectsLabel}`}>
          <fieldset className="max-h-72 min-w-56 space-y-1 overflow-y-auto">
            <legend className="sr-only">{trp.projects}</legend>
            {projects.data?.map((p) => (
              <label key={p.key} className="flex items-center gap-2 text-sm pointer-coarse:min-h-11">
                <input type="checkbox" className="accent-brand-600" checked={f.projects.includes(p.key)}
                  onChange={(e) => { toggleProject(p.key, e.target.checked); }} />
                <span className="font-mono text-xs">{p.key}</span>
                <span className="truncate">{p.name}</span>
              </label>
            ))}
            {f.projects.length > 0 && (
              <Button variant="ghost" className="mt-1" onClick={() => { onChange({ ...f, projects: [] }); }}>{trp.clearProjects}</Button>
            )}
          </fieldset>
        </Popover>
      </div>
      <div>
        <Label htmlFor="r-assignee">{trp.assignee}</Label>
        <Select id="r-assignee" value={f.assignee} onChange={(e) => { onChange({ ...f, assignee: e.target.value }); }}>
          <option value="">{trp.anyone}</option>
          <option value="me">{trp.me}</option>
          <option value="none">{trp.unassigned}</option>
          {members.data?.map((m) => <option key={m.user_id} value={m.user_id}>{m.display_name}</option>)}
        </Select>
      </div>
      <div>
        <Label htmlFor="r-type">{trp.type}</Label>
        <Select id="r-type" value={f.type} onChange={(e) => { onChange({ ...f, type: e.target.value }); }}>
          <option value="">{trp.allTypes}</option>
          {types.data?.map((ty) => <option key={ty.key} value={ty.key}>{ty.name}</option>)}
        </Select>
      </div>
      {!isDefault && (
        <Button variant="ghost" onClick={() => { onChange(readFilters(new URLSearchParams(), today)); }}>{trp.reset}</Button>
      )}
      <p className={cn("basis-full text-xs tabular-nums", muted)} data-testid="r-period-label">
        {trp.periodLabel(formatDate(f.from), formatDate(f.to))}
      </p>
    </section>
  );
}

function CsvMenu({ ws, f }: { ws: string; f: ReportFilters }) {
  const toast = useToast();
  const [busy, setBusy] = useState(false);
  const run = (semicolon: boolean) => {
    setBusy(true);
    downloadWorkloadCsv(ws, f, activeLocale(), semicolon)
      .then((name) => { toast("success", trp.csvDone(name)); })
      .catch(() => { toast("error", trp.csvFailed); })
      .finally(() => { setBusy(false); });
  };
  return (
    <div className="flex gap-1" role="group" aria-label={trp.csvHint} aria-busy={busy}>
      <Button disabled={busy} onClick={() => { run(false); }}>{trp.csv}</Button>
      <Button disabled={busy} onClick={() => { run(true); }}>{trp.csvSemicolon}</Button>
    </div>
  );
}

// ---------------------------------------------------------------------------
// KPI tiles
// ---------------------------------------------------------------------------

function KpiTiles({ ws, f, q, project }: { ws: string; f: ReportFilters; q: Query; project?: string }) {
  const k = q.data?.kpis;
  const statuses = project ? q.data?.status.find((s) => s.key === project)?.statuses ?? [] : [];
  const href = (l: Parameters<typeof itemsHref>[3]) => (project ? itemsHref(ws, project, f, l) : undefined);
  const tiles: { id: string; label: string; hint: string; when: string; value?: number; extra?: string; to?: string; alert?: boolean }[] = [
    { id: "open", label: trp.kpi.open, hint: trp.kpiHint.open, when: trp.now, value: k?.open, to: href({ status: statusIds(statuses, OPEN_CATEGORIES) }) },
    { id: "created", label: trp.kpi.created, hint: trp.kpiHint.created, when: trp.inPeriod, value: k?.created },
    { id: "completed", label: trp.kpi.completed, hint: trp.kpiHint.completed, when: trp.inPeriod, value: k?.completed },
    { id: "overdue", label: trp.kpi.overdue, hint: trp.kpiHint.overdue, when: trp.now, value: k?.overdue, to: href({ due: "overdue" }), alert: (k?.overdue ?? 0) > 0 },
    {
      id: "awaiting", label: trp.kpi.awaiting, hint: trp.kpiHint.awaiting, when: trp.now, value: k?.awaiting_approval, to: href({ approval: "pending" }),
      extra: k && k.awaiting_approval_late > 0 ? trp.awaitingLate(k.awaiting_approval_late) : undefined,
    },
  ];
  if (q.isError) return <ErrorState error={q.error} onRetry={() => void q.refetch()} />;
  return (
    <ul className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5" data-testid="kpis">
      {tiles.map((x) => (
        <li key={x.id} className={cn(card, "flex min-w-0 flex-col gap-1")} data-testid={`kpi-${x.id}`}>
          <span className="flex items-baseline justify-between gap-1 text-xs font-medium text-zinc-600 dark:text-zinc-300">
            <span className="truncate" title={x.hint}>{x.label}</span>
            <span className={cn("shrink-0 text-2xs", muted)}>{x.when}</span>
          </span>
          {x.value === undefined ? (
            <span className={cn("text-2xl font-semibold", muted)} aria-hidden>…</span>
          ) : (
            <span className={cn("text-2xl font-semibold tabular-nums", x.alert && "text-red-700 dark:text-red-300")}>
              {x.alert && <span aria-hidden className="mr-1 text-base">⏰</span>}
              <span data-testid={`kpi-${x.id}-value`}>{x.value}</span>
            </span>
          )}
          <span className={cn("text-2xs", muted)}>{x.hint}</span>
          {x.extra && <span className="text-2xs font-medium text-red-700 dark:text-red-300">{x.extra}</span>}
          {x.to && x.value !== undefined && x.value > 0 && (
            <Link to={x.to} className={cn("mt-auto text-xs", linkClass)} aria-label={`${x.label}: ${trp.openList}`}>{trp.openList} →</Link>
          )}
        </li>
      ))}
    </ul>
  );
}

// ---------------------------------------------------------------------------
// Throughput: grouped bars per week/month
// ---------------------------------------------------------------------------

function Throughput({ r }: { r: Reports }) {
  const rows = r.throughput;
  const max = Math.max(1, ...rows.flatMap((b) => [b.created, b.completed]));
  const ticks = niceTicks(max);
  const top = ticks[ticks.length - 1] ?? max;
  const label = (start: string) => (r.range.bucket === "month" ? formatMonthYear(start) : formatShortDate(start));
  const pct = (v: number) => `${String((v / top) * 100)}%`;
  // Label at most ~6 slots so text never collides on a phone.
  const every = Math.max(1, Math.ceil(rows.length / 6));
  const total = rows.reduce((s, b) => ({ c: s.c + b.created, d: s.d + b.completed }), { c: 0, d: 0 });
  const empty = total.c === 0 && total.d === 0;
  return (
    <div>
      <Legend items={[{ cls: SERIES.created, label: trp.created }, { cls: SERIES.completed, label: trp.completed }]} />
      <p className="sr-only">{trp.throughputSummary(total.c, total.d, rows.length)}</p>
      {empty ? (
        <EmptyState>{trp.throughputSummary(0, 0, rows.length)}</EmptyState>
      ) : (
        // CSS bars: crisp at any width; the table below is the accessible form.
        <div aria-hidden className="mt-3" data-testid="throughput-chart">
          <div className="flex">
            <div className="relative h-44 w-7 shrink-0">
              {ticks.map((v) => (
                <span key={v} className={cn("absolute right-1.5 translate-y-1/2 text-2xs tabular-nums", muted)} style={{ bottom: pct(v) }}>{v}</span>
              ))}
            </div>
            <div className="relative h-44 min-w-0 flex-1">
              {ticks.map((v) => (
                <span key={v} className="absolute inset-x-0 border-t border-zinc-200 dark:border-zinc-800" style={{ bottom: pct(v) }} />
              ))}
              <div className="absolute inset-0 flex items-end">
                {rows.map((b) => (
                  <div key={b.start} title={`${label(b.start)}: ${trp.created} ${String(b.created)}, ${trp.completed} ${String(b.completed)}`}
                    className="flex h-full min-w-0 flex-1 items-end justify-center gap-0.5 px-px hover:bg-zinc-100 dark:hover:bg-zinc-800">
                    <span className={cn("w-full max-w-3.5 rounded-t-sm", SERIES.created)} style={{ height: pct(b.created) }} />
                    <span className={cn("w-full max-w-3.5 rounded-t-sm", SERIES.completed)} style={{ height: pct(b.completed) }} />
                  </div>
                ))}
              </div>
            </div>
          </div>
          <div className="ml-7 flex">
            {rows.map((b, i) => (
              <span key={b.start} className={cn("min-w-0 flex-1 text-center text-2xs whitespace-nowrap", muted)}>
                {i % every === 0 ? label(b.start) : ""}
              </span>
            ))}
          </div>
        </div>
      )}
      <TableToggle>
        <table className="w-full text-sm tabular-nums">
          <thead>
            <tr className={cn("text-left text-xs", muted)}>
              <th scope="col" className="py-1 font-medium">{trp.bucketStart[r.range.bucket]}</th>
              <th scope="col" className="py-1 text-right font-medium">{trp.created}</th>
              <th scope="col" className="py-1 text-right font-medium">{trp.completed}</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((b) => (
              <tr key={b.start} className="border-t border-zinc-100 dark:border-zinc-800">
                <th scope="row" className="py-1 text-left font-normal">{r.range.bucket === "month" ? formatMonthYear(b.start) : formatDate(b.start)}</th>
                <td className="py-1 text-right">{b.created}</td>
                <td className="py-1 text-right">{b.completed}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </TableToggle>
    </div>
  );
}

function Legend({ items }: { items: { cls: string; label: string }[] }) {
  return (
    <ul className="flex flex-wrap gap-x-4 gap-y-1 text-xs">
      {items.map((x) => (
        <li key={x.label} className="flex items-center gap-1.5">
          <span aria-hidden className={cn("inline-block size-3 rounded-sm", x.cls)} />
          {x.label}
        </li>
      ))}
    </ul>
  );
}

function TableToggle({ children }: { children: ReactNode }) {
  return (
    <details className="mt-2">
      <summary className={cn("cursor-pointer text-xs pointer-coarse:min-h-11 pointer-coarse:content-center", linkClass)}>{trp.table}</summary>
      <div className="mt-2 overflow-x-auto">{children}</div>
    </details>
  );
}

// ---------------------------------------------------------------------------
// Aging
// ---------------------------------------------------------------------------

function Aging({ r }: { r: Reports }) {
  return (
    <div className="space-y-4">
      <p className="sr-only">{trp.agingSummary(r.kpis.open, r.kpis.overdue)}</p>
      <BucketBars title={trp.agingAge} rows={r.aging.age} cls="bg-sky-700 dark:bg-sky-600" testId="aging-age" />
      <BucketBars title={trp.agingLate} rows={r.aging.late} cls="bg-red-600 dark:bg-red-500" testId="aging-late" />
    </div>
  );
}

function BucketBars({ title, rows, cls, testId }: { title: string; rows: { bucket: string; count: number }[]; cls: string; testId: string }) {
  const max = Math.max(1, ...rows.map((x) => x.count));
  return (
    <div>
      <h3 className="mb-1 text-xs font-medium text-zinc-600 dark:text-zinc-300">{title}</h3>
      {/* A list of labelled numbers: the bar is decoration. */}
      <dl className="space-y-1" data-testid={testId}>
        {rows.map((x) => (
          <div key={x.bucket} className="grid grid-cols-[6.5rem_1fr_2.5rem] items-center gap-2 text-sm">
            <dt className={cn("truncate text-xs", muted)}>{trp.bucket[x.bucket] ?? x.bucket}</dt>
            <span aria-hidden className="h-3 rounded-sm bg-zinc-100 dark:bg-zinc-800">
              <span className={cn("block h-3 rounded-sm", cls)} style={{ width: `${String((x.count / max) * 100)}%` }} />
            </span>
            <dd className="text-right tabular-nums">{x.count}</dd>
          </div>
        ))}
      </dl>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Approvals
// ---------------------------------------------------------------------------

function duration(s: number | null): string {
  if (s === null) return trp.none;
  const d = durationParts(s);
  return trp.duration[d.unit](d.value);
}

function Approvals({ r }: { r: Reports }) {
  const tot = r.approvals.total;
  const decided = tot.approved + tot.rejected + tot.canceled;
  if (decided === 0) return <EmptyState>{trp.noApprovals}</EmptyState>;
  const name = (row: ReportApprovalRow) => (row.to_status ? trp.toStatus(row.to_status.workflow, row.to_status.name) : trp.deletedStatus);
  return (
    <div className="space-y-3">
      <dl className="grid grid-cols-2 gap-3 sm:grid-cols-4" data-testid="approval-stats">
        <Stat label={trp.median} value={duration(tot.median_seconds)} />
        <Stat label={trp.p90} value={duration(tot.p90_seconds)} />
        <Stat label={trp.approved} value={String(tot.approved)} />
        <Stat label={trp.rejected} value={String(tot.rejected)} />
        <Stat label={trp.canceled} value={String(tot.canceled)} />
        <Stat label={trp.late} value={String(tot.late)} />
      </dl>
      <div className="overflow-x-auto">
        <table className="w-full min-w-[28rem] text-sm tabular-nums">
          <thead>
            <tr className={cn("text-left text-xs", muted)}>
              <th scope="col" className="py-1 font-medium">{trp.transition}</th>
              <th scope="col" className="py-1 text-right font-medium">{trp.median}</th>
              <th scope="col" className="py-1 text-right font-medium">{trp.p90}</th>
              <th scope="col" className="py-1 text-right font-medium">{trp.approved}</th>
              <th scope="col" className="py-1 text-right font-medium">{trp.rejected}</th>
              <th scope="col" className="py-1 text-right font-medium">{trp.canceled}</th>
              <th scope="col" className="py-1 text-right font-medium">{trp.late}</th>
            </tr>
          </thead>
          <tbody>
            {r.approvals.by_transition.map((row, i) => (
              <tr key={row.to_status?.id ?? `deleted-${String(i)}`} className="border-t border-zinc-100 dark:border-zinc-800">
                <th scope="row" className="py-1 pr-2 text-left font-normal [overflow-wrap:anywhere]">{name(row)}</th>
                <td className="py-1 text-right">{duration(row.median_seconds)}</td>
                <td className="py-1 text-right">{duration(row.p90_seconds)}</td>
                <td className="py-1 text-right">{row.approved}</td>
                <td className="py-1 text-right">{row.rejected}</td>
                <td className="py-1 text-right">{row.canceled}</td>
                <td className="py-1 text-right">{row.late}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0">
      <dt className={cn("truncate text-xs", muted)}>{label}</dt>
      <dd className="text-lg font-semibold tabular-nums">{value}</dd>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Workload
// ---------------------------------------------------------------------------

/**
 * openIds: each project's open (todo / in progress) statuses from the same
 * response. The item list has no "open" filter, so an "open" link selects
 * those statuses.
 */
function Workload({ ws, f, rows, drill, openIds }: { ws: string; f: ReportFilters; rows: ReportAssignee[]; drill: boolean; openIds: Map<string, string[]> }) {
  const [sort, setSort] = useState<{ key: WorkloadSort; asc: boolean } | null>(null);
  const [open, setOpen] = useState<Set<string>>(new Set());
  if (rows.length === 0) return <EmptyState>{trp.noWorkload}</EmptyState>;
  const sorted = sort ? sortWorkload(rows, sort.key, sort.asc, trp.unassigned, activeLocale()) : rows;
  const cols: { key: WorkloadSort; label: string }[] = [
    { key: "name", label: trp.assignee },
    { key: "open", label: trp.kpi.open },
    { key: "overdue", label: trp.kpi.overdue },
    { key: "completed", label: trp.completedInPeriod },
  ];
  const head = (c: (typeof cols)[number]) => {
    const active = sort?.key === c.key;
    return (
      <th key={c.key} scope="col" aria-sort={active ? (sort.asc ? "ascending" : "descending") : "none"}
        className={cn("py-1 font-medium", c.key !== "name" && "text-right")}>
        <button type="button" className="inline-flex items-center gap-1 rounded hover:text-zinc-900 pointer-coarse:min-h-11 dark:hover:text-zinc-100"
          onClick={() => { setSort({ key: c.key, asc: active ? !sort.asc : c.key === "name" }); }}>
          {c.label}
          <span aria-hidden>{active ? (sort.asc ? "▲" : "▼") : "↕"}</span>
        </button>
      </th>
    );
  };
  const rowKey = (r: ReportAssignee) => r.assignee_id ?? "none";
  const openHref = (r: ReportAssignee, key: string) => itemsHref(ws, key, f, { assignee: rowAssignee(r), status: openIds.get(key) ?? [] });
  const lateHref = (r: ReportAssignee, key: string) => itemsHref(ws, key, f, { assignee: rowAssignee(r), due: "overdue" });
  // A number links to the list when the row's work lies in one project.
  const cell = (r: ReportAssignee, n: number, href: (r: ReportAssignee, key: string) => string) => {
    const only = r.projects.length === 1 ? r.projects[0] : undefined;
    if (!drill || !only || n === 0) return n;
    return <Link to={href(r, only.key)} className={linkClass}>{n}</Link>;
  };
  return (
    <div className="overflow-x-auto">
      <table className="w-full text-sm tabular-nums" data-testid="workload-table">
        <thead>
          <tr className={cn("text-left text-xs", muted)}>{cols.map(head)}</tr>
        </thead>
        <tbody>
          {sorted.map((r) => {
            const k = rowKey(r);
            const name = r.name ?? trp.unassigned;
            const expanded = open.has(k);
            return [
              <tr key={k} className="border-t border-zinc-100 dark:border-zinc-800" data-testid="workload-row">
                <th scope="row" className="py-1.5 pr-2 text-left font-normal">
                  <span className="flex items-center gap-1">
                    {r.projects.length > 1 && (
                      <button type="button" aria-expanded={expanded} aria-label={trp.showProjects(name)}
                        className="rounded px-1 text-xs pointer-coarse:min-h-11 pointer-coarse:min-w-11"
                        onClick={() => {
                          const next = new Set(open);
                          if (expanded) next.delete(k);
                          else next.add(k);
                          setOpen(next);
                        }}>
                        <span aria-hidden>{expanded ? "▾" : "▸"}</span>
                      </button>
                    )}
                    <span className={cn("[overflow-wrap:anywhere]", r.assignee_id === null && cn("italic", muted))}>{name}</span>
                  </span>
                </th>
                <td className="py-1.5 text-right">{cell(r, r.open, openHref)}</td>
                <td className={cn("py-1.5 text-right", r.overdue > 0 && "font-medium text-red-700 dark:text-red-300")}>{cell(r, r.overdue, lateHref)}</td>
                <td className="py-1.5 text-right">{r.completed}</td>
              </tr>,
              ...(expanded ? r.projects.map((p) => (
                <tr key={`${k}-${p.key}`} className={cn("text-xs", muted)}>
                  <th scope="row" className="py-1 pl-8 text-left font-mono font-normal">{p.key}</th>
                  <td className="py-1 text-right">{drill && p.open > 0 ? <Link to={openHref(r, p.key)} className={linkClass}>{p.open}</Link> : p.open}</td>
                  <td className="py-1 text-right">
                    {drill && p.overdue > 0 ? <Link to={lateHref(r, p.key)} className={linkClass}>{p.overdue}</Link> : p.overdue}
                  </td>
                  <td className="py-1 text-right">{p.completed}</td>
                </tr>
              )) : []),
            ];
          })}
        </tbody>
      </table>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Status distribution
// ---------------------------------------------------------------------------

function StatusDistribution({ ws, f, rows, drill }: { ws: string; f: ReportFilters; rows: ReportProjectStatus[]; drill: boolean }) {
  if (rows.length === 0) return <EmptyState>{trp.noStatus}</EmptyState>;
  return (
    <div className="space-y-4">
      <Legend items={CATEGORIES.map((c) => ({ cls: CATEGORY_FILL[c], label: t.category[c] }))} />
      <ul className="space-y-3" data-testid="status-list">
        {rows.map((p) => {
          const byCat = CATEGORIES.map((c) => ({ c, n: p.statuses.filter((s) => s.category === c).reduce((a, s) => a + s.count, 0) })).filter((x) => x.n > 0);
          const summary = byCat.map((x) => `${t.category[x.c]} ${String(x.n)}`).join(", ");
          return (
            <li key={p.key} data-testid={`status-${p.key}`}>
              <div className="mb-1 flex items-baseline justify-between gap-2 text-sm">
                <span className="min-w-0 truncate"><span className="font-mono text-xs">{p.key}</span> {p.name}</span>
                <span className={cn("shrink-0 text-xs tabular-nums", muted)}>{trp.items(p.total)}</span>
              </div>
              <p className="sr-only">{trp.statusSummary(p.key, summary)}</p>
              <div aria-hidden className="flex h-4 gap-0.5 overflow-hidden rounded">
                {byCat.map((x) => (
                  <span key={x.c} className={cn("h-full first:rounded-l last:rounded-r", CATEGORY_FILL[x.c])} style={{ width: `${String((x.n / p.total) * 100)}%` }}
                    title={`${t.category[x.c]}: ${String(x.n)}`} />
                ))}
              </div>
              <ul className="mt-1 flex flex-wrap gap-x-3 gap-y-0.5 text-xs tabular-nums">
                {p.statuses.map((s) => {
                  const text = `${s.name}: ${String(s.count)}`;
                  return (
                    <li key={s.id}>
                      {drill ? <Link to={itemsHref(ws, p.key, f, { status: [s.id] })} className={linkClass}>{text}</Link> : text}
                    </li>
                  );
                })}
              </ul>
            </li>
          );
        })}
      </ul>
    </div>
  );
}
