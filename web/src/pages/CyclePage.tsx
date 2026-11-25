import { useId, useMemo, useState, type SyntheticEvent } from "react";
import { Link, useParams, useSearchParams } from "react-router";

import { useCloses, useCycle, useCycleApi, useCyclePreview, useSnapshot, type Cycle, type CycleRefill, type PeriodClose } from "../api/cycle";
import { ApiError } from "../api/client";
import { useFields, useItemTypes, useMembers, useProjects, useWorkspace } from "../api/hooks";
import { roleAtLeast, type FieldDefinition, type FieldValue } from "../api/types";
import { useConfirm } from "../components/confirm-context";
import { Layout } from "../components/Layout";
import { useToast } from "../components/toast-context";
import { Button, EmptyState, ErrorState, FieldError, Input, Label, Select, Spinner } from "../components/ui";
import { t } from "../i18n";
import { tcy } from "../i18n-cycle";
import { cn } from "../lib/cn";
import { formatDate, formatDateTime } from "../lib/format";
import { amountText, formatFieldValue, withUnit } from "../lib/views";

const card = "rounded-xl bg-white p-4 ring-1 ring-zinc-200 dark:bg-zinc-900 dark:ring-zinc-800";

/** Monthly period close (ADR-0051): settings, a manual close and the closed months. */
export function CyclePage() {
  const { ws = "", project = "" } = useParams();
  const [params, setParams] = useSearchParams();
  const month = params.get("month");
  const workspace = useWorkspace(ws);
  const projects = useProjects(ws);
  const projectName = projects.data?.find((p) => p.key === project)?.name ?? project;
  const projectUrl = `/w/${encodeURIComponent(ws)}/p/${encodeURIComponent(project)}`;

  let body;
  if (workspace.isPending) body = <Spinner />;
  else if (workspace.isError) body = <ErrorState error={workspace.error} />;
  else if (!roleAtLeast(workspace.data.role, "member")) body = <EmptyState>{tcy.noAccess}</EmptyState>;
  else if (month) body = <Snapshot ws={ws} project={project} period={month} onBack={() => { setParams({}); }} />;
  else body = <Overview ws={ws} project={project} isAdmin={roleAtLeast(workspace.data.role, "admin")} onOpen={(p) => { setParams({ month: p }); }} />;

  return (
    <Layout
      crumbs={[
        { label: t.workspaces, to: "/" },
        { label: workspace.data?.name ?? ws, to: `/w/${encodeURIComponent(ws)}` },
        { label: projectName, to: projectUrl, switcher: { ws, project, suffix: "/cycle" } },
        { label: tcy.nav },
      ]}
    >
      <div className="h-full overflow-y-auto">
        <div className="mx-auto max-w-5xl p-4 md:p-6">
          <h1 className="text-lg font-semibold">{tcy.title}</h1>
          <p className="mt-1 mb-6 max-w-3xl text-sm text-zinc-500 dark:text-zinc-400">{tcy.intro}</p>
          {body}
        </div>
      </div>
    </Layout>
  );
}

/** Fields a close can work on: active and not computed. */
function usableFields(fields: FieldDefinition[] | undefined): FieldDefinition[] {
  return (fields ?? []).filter((f) => !f.archived && f.type !== "formula");
}

function Overview({ ws, project, isAdmin, onOpen }: { ws: string; project: string; isAdmin: boolean; onOpen: (period: string) => void }) {
  const cycle = useCycle(ws, project);
  const fields = useFields(ws);
  if (cycle.isPending || fields.isPending) return <Spinner />;
  if (cycle.isError) return <ErrorState error={cycle.error} onRetry={() => void cycle.refetch()} />;
  if (fields.isError) return <ErrorState error={fields.error} onRetry={() => void fields.refetch()} />;
  const usable = usableFields(fields.data);
  return (
    <div className="space-y-6">
      <Status cycle={cycle.data} />
      {isAdmin && <Settings ws={ws} project={project} cycle={cycle.data} fields={usable} />}
      {isAdmin && cycle.data.updated_at !== null && <CloseNow ws={ws} project={project} />}
      <History ws={ws} project={project} onOpen={onOpen} />
    </div>
  );
}

function Status({ cycle }: { cycle: Cycle }) {
  return (
    <div className="space-y-1 text-sm">
      {cycle.enabled && cycle.next_run && cycle.next_period ? (
        <p className="font-medium text-brand-800 dark:text-brand-200">{tcy.next(formatDate(cycle.next_run), tcy.month(cycle.next_period))}</p>
      ) : (
        <p className="text-zinc-500 dark:text-zinc-400">{tcy.off}</p>
      )}
      {cycle.last_error && <p role="alert" className="text-red-700 dark:text-red-300">{tcy.lastError(cycle.last_error)}</p>}
    </div>
  );
}

function Settings({ ws, project, cycle, fields }: { ws: string; project: string; cycle: Cycle; fields: FieldDefinition[] }) {
  const capi = useCycleApi(ws, project);
  const toast = useToast();
  const [enabled, setEnabled] = useState(cycle.enabled);
  const [day, setDay] = useState(cycle.day_of_month);
  const [clear, setClear] = useState<string[]>(cycle.clear_fields);
  const [refill, setRefill] = useState<CycleRefill[]>(cycle.refill);
  const id = useId();
  const byKey = useMemo(() => new Map(fields.map((f) => [f.key, f])), [fields]);
  const refillTargets = new Set(refill.map((r) => r.target));

  const submit = (e: SyntheticEvent) => {
    e.preventDefault();
    capi.save.mutate(
      { enabled, day_of_month: day, clear_fields: clear, refill: refill.filter((r) => r.target && r.source) },
      {
        onSuccess: () => { toast("success", tcy.saved); },
        onError: (err) => { toast("error", err instanceof ApiError ? `${tcy.saveFailed}: ${err.message}` : tcy.saveFailed); },
      },
    );
  };
  const setRow = (i: number, patch: Partial<CycleRefill>) => {
    setRefill((rs) => rs.map((r, j) => (j === i ? { ...r, ...patch } : r)));
  };

  if (fields.length === 0) return <EmptyState>{tcy.noFields}</EmptyState>;
  return (
    <form onSubmit={submit} className={cn(card, "space-y-5")} aria-labelledby={`${id}-h`}>
      <h2 id={`${id}-h`} className="text-sm font-semibold">{tcy.settings}</h2>
      <div className="flex flex-wrap items-end gap-6">
        <label className="flex items-center gap-2 text-sm">
          <input type="checkbox" className="size-4 accent-brand-600" checked={enabled} onChange={(e) => { setEnabled(e.target.checked); }} />
          {tcy.enabled}
        </label>
        <div>
          <Label htmlFor={`${id}-day`}>{tcy.day}</Label>
          <Select id={`${id}-day`} value={day} onChange={(e) => { setDay(Number(e.target.value)); }} aria-describedby={`${id}-dayhint`}>
            {Array.from({ length: 28 }, (_, i) => i + 1).map((d) => <option key={d} value={d}>{d}</option>)}
          </Select>
        </div>
      </div>
      <p id={`${id}-dayhint`} className="-mt-3 text-xs text-zinc-500 dark:text-zinc-400">{tcy.dayHint}</p>

      <fieldset>
        <legend className="text-sm font-medium">{tcy.clear}</legend>
        <p className="mb-2 text-xs text-zinc-500 dark:text-zinc-400">{tcy.clearHint}</p>
        <div className="grid gap-x-4 gap-y-1.5 sm:grid-cols-2 lg:grid-cols-3">
          {fields.filter((f) => !refillTargets.has(f.key)).map((f) => (
            <label key={f.key} className="flex items-center gap-2 text-sm">
              <input
                type="checkbox"
                className="size-4 accent-brand-600"
                checked={clear.includes(f.key)}
                onChange={(e) => { setClear((c) => (e.target.checked ? [...c, f.key] : c.filter((k) => k !== f.key))); }}
              />
              {f.name}
            </label>
          ))}
        </div>
      </fieldset>

      <fieldset>
        <legend className="text-sm font-medium">{tcy.refill}</legend>
        <p className="mb-2 text-xs text-zinc-500 dark:text-zinc-400">{tcy.refillHint}</p>
        <ul className="space-y-2">
          {refill.map((r, i) => {
            const target = byKey.get(r.target);
            const name = target?.name ?? r.target;
            return (
              <li key={i} className="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-2 sm:grid-cols-[minmax(0,16rem)_auto_minmax(0,16rem)_auto]">
                <Select aria-label={tcy.target} value={r.target} onChange={(e) => { setRow(i, { target: e.target.value, source: "" }); }}>
                  <option value="">{tcy.pick}</option>
                  {fields.filter((f) => !clear.includes(f.key)).map((f) => <option key={f.key} value={f.key}>{f.name}</option>)}
                </Select>
                <span aria-hidden="true" className="text-zinc-500 max-sm:hidden">{tcy.from}</span>
                <Select aria-label={tcy.source} value={r.source} onChange={(e) => { setRow(i, { source: e.target.value }); }} disabled={!target}>
                  <option value="">{tcy.pick}</option>
                  {fields.filter((f) => f.key !== r.target && f.type === target?.type).map((f) => <option key={f.key} value={f.key}>{f.name}</option>)}
                </Select>
                <Button type="button" variant="ghost" aria-label={tcy.removeRefill(name)} onClick={() => { setRefill((rs) => rs.filter((_, j) => j !== i)); }}>×</Button>
              </li>
            );
          })}
        </ul>
        <Button type="button" variant="ghost" className="mt-2" onClick={() => { setRefill((rs) => [...rs, { target: "", source: "" }]); }}>{tcy.addRefill}</Button>
      </fieldset>

      <FieldError error={capi.save.error} />
      <Button type="submit" variant="primary" disabled={capi.save.isPending}>{tcy.save}</Button>
    </form>
  );
}

function lastMonth(): string {
  const d = new Date();
  d.setDate(1);
  d.setMonth(d.getMonth() - 1);
  return `${String(d.getFullYear())}-${String(d.getMonth() + 1).padStart(2, "0")}`;
}

function thisMonth(): string {
  const d = new Date();
  return `${String(d.getFullYear())}-${String(d.getMonth() + 1).padStart(2, "0")}`;
}

function CloseNow({ ws, project }: { ws: string; project: string }) {
  const [period, setPeriod] = useState(lastMonth);
  const valid = /^\d{4}-\d{2}$/.test(period);
  const preview = useCyclePreview(ws, project, period, valid);
  const capi = useCycleApi(ws, project);
  const confirm = useConfirm();
  const toast = useToast();
  const id = useId();
  const p = preview.data;

  const close = () => {
    if (!p) return;
    void confirm({ title: tcy.confirmTitle(tcy.month(p.period)), body: tcy.confirmBody(p.will_reset), action: tcy.confirmAction, danger: true }).then((ok) => {
      if (!ok) return;
      capi.close.mutate(p.period, {
        onSuccess: (c) => { toast("success", tcy.closed(tcy.month(c.period), c.changed)); },
        onError: (err) => { toast("error", err instanceof ApiError ? `${tcy.closeFailed}: ${err.message}` : tcy.closeFailed); },
      });
    });
  };

  return (
    <section className={cn(card, "space-y-3")} aria-labelledby={`${id}-h`}>
      <h2 id={`${id}-h`} className="text-sm font-semibold">{tcy.closeTitle}</h2>
      <p className="text-xs text-zinc-500 dark:text-zinc-400">{tcy.closeHint}</p>
      <div className="flex flex-wrap items-end gap-3">
        <div>
          <Label htmlFor={`${id}-period`}>{tcy.period}</Label>
          <Input id={`${id}-period`} type="month" min="2000-01" max={thisMonth()} value={period} onChange={(e) => { setPeriod(e.target.value); }} />
        </div>
        <Button variant="primary" onClick={close} disabled={!p || p.closed || capi.close.isPending}>{tcy.closeNow}</Button>
      </div>
      <div aria-live="polite" className="text-sm">
        {preview.isError && <FieldError error={preview.error} />}
        {p && (p.closed ? <p className="text-zinc-500 dark:text-zinc-400">{tcy.alreadyClosed}</p> : <p>{tcy.preview(p.items, p.open, p.will_reset)}</p>)}
      </div>
    </section>
  );
}

function History({ ws, project, onOpen }: { ws: string; project: string; onOpen: (period: string) => void }) {
  const closes = useCloses(ws, project);
  const members = useMembers(ws);
  const byId = useMemo(() => new Map((members.data ?? []).map((m) => [m.user_id, m.display_name])), [members.data]);
  const id = useId();
  if (closes.isPending) return <Spinner />;
  if (closes.isError) return <ErrorState error={closes.error} onRetry={() => void closes.refetch()} />;
  return (
    <section aria-labelledby={`${id}-h`} className="space-y-2">
      <h2 id={`${id}-h`} className="text-sm font-semibold">{tcy.history}</h2>
      {closes.data.length === 0 ? (
        <EmptyState>{tcy.noHistory}</EmptyState>
      ) : (
        <ul className="space-y-2">
          {closes.data.map((c) => <CloseRow key={c.id} c={c} who={c.closed_by ? (byId.get(c.closed_by) ?? "—") : tcy.scheduler} onOpen={onOpen} />)}
        </ul>
      )}
    </section>
  );
}

function CloseRow({ c, who, onOpen }: { c: PeriodClose; who: string; onOpen: (period: string) => void }) {
  return (
    <li className={cn(card, "flex flex-wrap items-center gap-3")}>
      <div className="min-w-0 flex-1">
        <p className="font-medium">{tcy.month(c.period)}</p>
        <p className="text-xs text-zinc-500 dark:text-zinc-400">
          {tcy.counts(c.item_count, c.changed)} · {formatDateTime(c.created_at)} · {tcy.closedBy(who)}
        </p>
        {c.failed.length > 0 && (
          <p className="mt-1 text-xs text-amber-800 dark:text-amber-200">
            {tcy.failed(c.failed.length)}: {c.failed.map((f) => `#${String(f.number)}`).join(", ")}
          </p>
        )}
      </div>
      <Button variant="ghost" aria-label={`${tcy.open}: ${tcy.month(c.period)}`} onClick={() => { onOpen(c.period); }}>{tcy.open}</Button>
    </li>
  );
}

function Snapshot({ ws, project, period, onBack }: { ws: string; project: string; period: string; onBack: () => void }) {
  const snap = useSnapshot(ws, project, period);
  const cycle = useCycle(ws, project);
  const fields = useFields(ws);
  const types = useItemTypes(ws);
  const members = useMembers(ws);
  const memberMap = useMemo(() => new Map((members.data ?? []).map((m) => [m.user_id, m])), [members.data]);
  const statusName = useMemo(() => {
    const m = new Map<string, string>();
    for (const it of types.data ?? []) for (const s of it.statuses) m.set(s.id, s.name);
    return m;
  }, [types.data]);

  if (snap.isPending || fields.isPending) return <Spinner />;
  if (snap.isError) return <ErrorState error={snap.error} onRetry={() => void snap.refetch()} />;
  if (fields.isError) return <ErrorState error={fields.error} />;

  // Columns: the fields the cycle works on first, then every other field any row has.
  const byKey = new Map(fields.data.map((f) => [f.key, f]));
  const keysInUse = new Set(snap.data.items.flatMap((r) => Object.keys(r.fields)));
  const first = cycle.data ? [...cycle.data.refill.flatMap((r) => [r.source, r.target]), ...cycle.data.clear_fields] : [];
  const cols: FieldDefinition[] = [];
  for (const k of [...first, ...keysInUse]) {
    const f = byKey.get(k);
    if (f && keysInUse.has(k) && !cols.includes(f)) cols.push(f);
  }
  const sum = (f: FieldDefinition) => snap.data.items.reduce((a, r) => a + (typeof r.fields[f.key] === "number" ? (r.fields[f.key] as number) : 0), 0);
  const cell = (f: FieldDefinition, v: FieldValue | undefined) => {
    if (f.type === "boolean") return v === true ? <span className="font-semibold text-emerald-700 dark:text-emerald-300">{tcy.yes}</span> : "";
    if (f.type === "number" && typeof v === "number") return withUnit(amountText(v, f), f);
    return formatFieldValue(f, v, memberMap);
  };

  return (
    <div className="space-y-4">
      <Link to="?" onClick={(e) => { e.preventDefault(); onBack(); }} className="text-sm text-brand-700 hover:underline dark:text-brand-300">{tcy.back}</Link>
      <div>
        <h2 className="text-base font-semibold">{tcy.snapshotTitle(tcy.month(snap.data.close.period))}</h2>
        <p className="text-xs text-zinc-500 dark:text-zinc-400">{tcy.snapshotIntro} {tcy.counts(snap.data.close.item_count, snap.data.close.changed)} · {formatDateTime(snap.data.close.created_at)}</p>
      </div>
      <div className="overflow-x-auto rounded-xl ring-1 ring-zinc-200 dark:ring-zinc-800">
        <table className="min-w-full text-sm">
          <thead className="bg-zinc-100 text-left text-xs text-zinc-600 dark:bg-zinc-800 dark:text-zinc-300">
            <tr>
              <th scope="col" className="px-2 py-1.5 font-medium">{tcy.key}</th>
              <th scope="col" className="sticky left-0 bg-zinc-100 px-2 py-1.5 font-medium dark:bg-zinc-800">{tcy.name}</th>
              <th scope="col" className="px-2 py-1.5 font-medium">{tcy.status}</th>
              {cols.map((f) => <th key={f.key} scope="col" className={cn("px-2 py-1.5 font-medium whitespace-nowrap", f.type === "number" && "text-right")}>{f.name}</th>)}
            </tr>
          </thead>
          <tbody className="divide-y divide-zinc-200 bg-white dark:divide-zinc-800 dark:bg-zinc-900">
            {snap.data.items.map((r) => (
              <tr key={r.number}>
                <td className="px-2 py-1 text-xs text-zinc-500 tabular-nums">{project}-{r.number}</td>
                <th scope="row" className="sticky left-0 max-w-64 bg-white px-2 py-1 text-left font-normal [overflow-wrap:anywhere] dark:bg-zinc-900">{r.title}</th>
                <td className="px-2 py-1 whitespace-nowrap">{statusName.get(r.status_id) ?? ""}</td>
                {cols.map((f) => <td key={f.key} className={cn("px-2 py-1", f.type === "number" && "text-right whitespace-nowrap tabular-nums")}>{cell(f, r.fields[f.key])}</td>)}
              </tr>
            ))}
          </tbody>
          {cols.some((f) => f.type === "number") && (
            <tfoot className="bg-zinc-50 font-semibold dark:bg-zinc-800/60">
              <tr>
                <td className="px-2 py-1.5" />
                <th scope="row" className="sticky left-0 bg-zinc-50 px-2 py-1.5 text-left dark:bg-zinc-800">{tcy.total}</th>
                <td />
                {cols.map((f) => <td key={f.key} className="px-2 py-1.5 text-right whitespace-nowrap tabular-nums">{f.type === "number" ? withUnit(amountText(sum(f), f), f) : ""}</td>)}
              </tr>
            </tfoot>
          )}
        </table>
      </div>
    </div>
  );
}
