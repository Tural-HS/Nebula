import { useMemo, useState, type SyntheticEvent } from "react";
import { useParams } from "react-router";

import { useRecurrenceApi, useRecurrences, useSchedulePreview, type Freq, type Recurrence, type RecurrenceInput, type ScheduleInput } from "../api/recurrences";
import { useAssignable, useFields, useItemTypes, useMembers, useProjects, useWorkspace } from "../api/hooks";
import { roleAtLeast, type FieldValue, type Member } from "../api/types";
import { FieldEditor } from "../components/FieldEditor";
import { Layout } from "../components/Layout";
import { useToast } from "../components/toast-context";
import { Avatar, Button, Dialog, EmptyState, ErrorState, FieldError, Input, Label, Select, Spinner } from "../components/ui";
import { t } from "../i18n";
import { tr } from "../i18n-recur";
import { cn } from "../lib/cn";
import { formatDate } from "../lib/format";
import { describeSchedule, renderTitle, WEEKDAYS } from "../lib/recurrence";

export function RecurringPage() {
  const { ws = "", project = "" } = useParams();
  const workspace = useWorkspace(ws);
  const projects = useProjects(ws);
  const projectName = projects.data?.find((p) => p.key === project)?.name ?? project;
  const projectUrl = `/w/${encodeURIComponent(ws)}/p/${encodeURIComponent(project)}`;

  let body;
  if (workspace.isPending) body = <Spinner />;
  else if (workspace.isError) body = <ErrorState error={workspace.error} />;
  else if (!roleAtLeast(workspace.data.role, "member")) body = <EmptyState>{tr.noAccess}</EmptyState>;
  else body = <Rules ws={ws} project={project} isAdmin={roleAtLeast(workspace.data.role, "admin")} />;

  return (
    <Layout
      crumbs={[
        { label: t.workspaces, to: "/" },
        { label: workspace.data?.name ?? ws, to: `/w/${encodeURIComponent(ws)}` },
        { label: projectName, to: projectUrl },
        { label: tr.nav },
      ]}
    >
      <div className="h-full overflow-y-auto">
        <div className="mx-auto max-w-5xl p-4 md:p-6">
          <h1 className="text-lg font-semibold">{tr.title}</h1>
          <p className="mt-1 mb-6 max-w-3xl text-sm text-zinc-500">{tr.intro}</p>
          {body}
        </div>
      </div>
    </Layout>
  );
}

function Rules({ ws, project, isAdmin }: { ws: string; project: string; isAdmin: boolean }) {
  const rules = useRecurrences(ws, project);
  const members = useMembers(ws);
  const assignable = useAssignable(ws, project);
  const [editing, setEditing] = useState<Recurrence | "new" | null>(null);
  const byId = useMemo(() => new Map((members.data ?? []).map((m) => [m.user_id, m])), [members.data]);

  if (rules.isPending) return <Spinner />;
  if (rules.isError) return <ErrorState error={rules.error} onRetry={() => void rules.refetch()} />;
  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between gap-2">
        {!isAdmin && <p className="text-xs text-zinc-500">{tr.adminOnlyEdit}</p>}
        {isAdmin && <Button variant="primary" className="ml-auto" onClick={() => { setEditing("new"); }}>{tr.newRule}</Button>}
      </div>
      {rules.data.length === 0 ? (
        <EmptyState>{tr.empty}</EmptyState>
      ) : (
        <ul className="space-y-2">
          {rules.data.map((r) => (
            <RuleRow key={r.id} rule={r} assignee={r.assignee_id ? byId.get(r.assignee_id) : undefined} isAdmin={isAdmin} onEdit={() => { setEditing(r); }} ws={ws} project={project} />
          ))}
        </ul>
      )}
      <Dialog open={editing !== null} onClose={() => { setEditing(null); }} title={editing === "new" ? tr.newRule : tr.editRule}>
        {editing !== null && (
          <RuleForm ws={ws} project={project} rule={editing === "new" ? null : editing} members={assignable} onDone={() => { setEditing(null); }} />
        )}
      </Dialog>
    </div>
  );
}

function RuleRow({ rule, assignee, isAdmin, onEdit, ws, project }: {
  rule: Recurrence; assignee: Member | undefined; isAdmin: boolean; onEdit: () => void; ws: string; project: string;
}) {
  const rapi = useRecurrenceApi(ws, project);
  const toast = useToast();
  const state = rule.last_error ? tr.failed : !rule.enabled ? tr.disabled : rule.next_due === null ? tr.ended : null;
  return (
    <li aria-label={rule.title} className={cn("rounded-xl bg-white p-4 ring-1 ring-zinc-200 dark:bg-zinc-900 dark:ring-zinc-800", !rule.enabled && "opacity-70")}>
      <div className="flex flex-wrap items-start gap-3">
        <div className="min-w-0 flex-1">
          <p className="font-medium">{rule.title}</p>
          {rule.next_due && rule.title.includes("{{") && (
            <p className="text-sm text-zinc-500">→ {renderTitle(rule.title, rule.next_due)}</p>
          )}
          <p className="mt-0.5 text-sm text-zinc-600 dark:text-zinc-400">
            {describeSchedule(rule)} · {rule.type.name}
          </p>
          {state && <p className={cn("mt-1 text-xs font-medium", rule.last_error ? "text-red-700 dark:text-red-300" : "text-zinc-500")}>{state}{rule.last_error ? `: ${rule.last_error}` : ""}</p>}
        </div>
        {assignee && <span className="flex items-center gap-1.5 text-sm"><Avatar name={assignee.display_name} />{assignee.display_name}</span>}
        {isAdmin && (
          <span className="flex gap-1">
            <Button variant="ghost" onClick={onEdit}>{t.edit}</Button>
            <Button
              variant="ghost"
              className="text-red-600"
              onClick={() => {
                if (window.confirm(tr.confirmDelete)) rapi.remove.mutate(rule.id, { onError: () => { toast("error", t.errorGeneric); } });
              }}
            >
              {t.delete}
            </Button>
          </span>
        )}
      </div>
      {rule.enabled && rule.upcoming.length > 0 && (
        <p className="mt-2 flex flex-wrap items-center gap-1.5 text-xs text-zinc-500">
          <span>{tr.upcoming}:</span>
          {rule.upcoming.map((d, i) => (
            <span key={d} className={cn("rounded px-1.5 py-0.5 tabular-nums", i === 0 ? "bg-brand-50 font-medium text-brand-700 dark:bg-brand-700/30 dark:text-brand-100" : "bg-zinc-100 dark:bg-zinc-800")}>
              {formatDate(d)}
            </span>
          ))}
        </p>
      )}
    </li>
  );
}

function todayISO(): string {
  const d = new Date();
  return `${String(d.getFullYear())}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}

function RuleForm({ ws, project, rule, members, onDone }: { ws: string; project: string; rule: Recurrence | null; members: Member[]; onDone: () => void }) {
  const types = useItemTypes(ws);
  const fields = useFields(ws);
  const rapi = useRecurrenceApi(ws, project);
  const toast = useToast();
  const [title, setTitle] = useState(rule?.title ?? "");
  const [typeKey, setTypeKey] = useState(rule?.type.key ?? "task");
  const [assignee, setAssignee] = useState(rule?.assignee_id ?? "");
  const [values, setValues] = useState<Record<string, FieldValue>>(rule?.fields ?? {});
  const [freq, setFreq] = useState<Freq>(rule?.freq ?? "monthly");
  const [every, setEvery] = useState(String(rule?.every ?? 1));
  const [weekdays, setWeekdays] = useState<number[]>(rule?.weekdays.length ? rule.weekdays : [1]);
  const [monthDay, setMonthDay] = useState(String(rule?.month_day ?? 1));
  const [month, setMonth] = useState(String(rule?.month ?? 1));
  const [start, setStart] = useState(rule?.start_date ?? todayISO());
  const [end, setEnd] = useState(rule?.end_date ?? "");
  const [lead, setLead] = useState(String(rule?.lead_days ?? 3));
  const [enabled, setEnabled] = useState(rule?.enabled ?? true);

  const type = types.data?.find((x) => x.key === typeKey) ?? types.data?.[0];
  const defs = new Map((fields.data ?? []).map((f) => [f.key, f]));
  const required = (type?.fields ?? []).filter((f) => f.required);

  const schedule: ScheduleInput = {
    freq,
    every: Math.max(1, Number(every) || 1),
    weekdays: freq === "weekly" ? weekdays : [],
    month_day: freq === "monthly" || freq === "yearly" ? Number(monthDay) : null,
    month: freq === "yearly" ? Number(month) : null,
    start_date: start,
    end_date: end || null,
  };
  const preview = useSchedulePreview(ws, project, start ? schedule : null);

  const submit = (e: SyntheticEvent) => {
    e.preventDefault();
    const body: RecurrenceInput = {
      ...schedule,
      title,
      description: rule?.description ?? null,
      type: type?.key ?? "",
      assignee_id: assignee || null,
      fields: Object.fromEntries(Object.entries(values).filter(([k, v]) => v !== null && required.some((r) => r.key === k))),
      lead_days: Math.min(365, Math.max(0, Number(lead) || 0)),
      enabled,
    };
    rapi.save.mutate({ id: rule?.id ?? null, body }, {
      onSuccess: () => { toast("success", t.saved); onDone(); },
    });
  };

  return (
    <form onSubmit={submit} className="space-y-4">
      <div>
        <Label htmlFor="rec-title">{tr.titleLabel}</Label>
        <Input id="rec-title" required maxLength={500} value={title} onChange={(e) => { setTitle(e.target.value); }} autoFocus />
        <p className="mt-1 text-xs text-zinc-500">{tr.titleHint}</p>
      </div>
      <div className="grid grid-cols-2 gap-3">
        <div>
          <Label htmlFor="rec-type">{t.type}</Label>
          <Select id="rec-type" value={type?.key ?? ""} onChange={(e) => { setTypeKey(e.target.value); setValues({}); }}>
            {types.data?.map((x) => <option key={x.id} value={x.key}>{x.name}</option>)}
          </Select>
        </div>
        <div>
          <Label htmlFor="rec-assignee">{t.assignee}</Label>
          <Select id="rec-assignee" value={assignee} onChange={(e) => { setAssignee(e.target.value); }}>
            <option value="">{t.unassigned}</option>
            {members.map((m) => <option key={m.user_id} value={m.user_id}>{m.display_name}</option>)}
          </Select>
        </div>
      </div>
      {required.map((r) => {
        const def = defs.get(r.key);
        if (!def) return null;
        const id = `rec-field-${r.key}`;
        return (
          <div key={r.key}>
            <Label htmlFor={id}>{def.name} *</Label>
            <FieldEditor id={id} def={def} value={values[r.key]} members={members} onCommit={(v) => { setValues((s) => ({ ...s, [r.key]: v })); }} />
          </div>
        );
      })}

      <fieldset className="space-y-3 rounded-lg p-3 ring-1 ring-zinc-200 dark:ring-zinc-800">
        <div className="grid grid-cols-2 gap-3">
          <div>
            <Label htmlFor="rec-freq">{tr.freq}</Label>
            <Select id="rec-freq" value={freq} onChange={(e) => { setFreq(e.target.value as Freq); }}>
              {(Object.keys(tr.freqs) as Freq[]).map((f) => <option key={f} value={f}>{tr.freqs[f]}</option>)}
            </Select>
          </div>
          <div>
            <Label htmlFor="rec-every">{tr.every}</Label>
            <div className="flex items-center gap-2">
              <Input id="rec-every" type="number" min={1} max={366} value={every} onChange={(e) => { setEvery(e.target.value); }} className="w-20" />
              <span className="text-sm text-zinc-500">{tr.everyUnit[freq]}</span>
            </div>
          </div>
        </div>
        {freq === "weekly" && (
          <div role="group" aria-label={tr.weekdays} className="flex flex-wrap gap-1.5">
            {WEEKDAYS.map((d) => (
              <label key={d.value} title={d.long} className={cn("cursor-pointer rounded-md px-2.5 py-1 text-sm ring-1", weekdays.includes(d.value) ? "bg-brand-600 text-white ring-brand-600" : "ring-zinc-300 dark:ring-zinc-700")}>
                <input
                  type="checkbox"
                  className="sr-only"
                  aria-label={d.long}
                  checked={weekdays.includes(d.value)}
                  onChange={(e) => { setWeekdays((w) => (e.target.checked ? [...w, d.value] : w.filter((x) => x !== d.value))); }}
                />
                {d.short}
              </label>
            ))}
          </div>
        )}
        {(freq === "monthly" || freq === "yearly") && (
          <div className="grid grid-cols-2 gap-3">
            {freq === "yearly" && (
              <div>
                <Label htmlFor="rec-month">{tr.month}</Label>
                <Select id="rec-month" value={month} onChange={(e) => { setMonth(e.target.value); }}>
                  {tr.months.map((m, i) => <option key={m} value={String(i + 1)}>{m}</option>)}
                </Select>
              </div>
            )}
            <div>
              <Label htmlFor="rec-day">{tr.monthDay}</Label>
              <Select id="rec-day" value={monthDay} onChange={(e) => { setMonthDay(e.target.value); }}>
                {Array.from({ length: 31 }, (_, i) => <option key={i + 1} value={String(i + 1)}>{i + 1}</option>)}
                <option value="-1">{tr.lastDay}</option>
              </Select>
            </div>
          </div>
        )}
        <div className="grid grid-cols-3 gap-3">
          <div>
            <Label htmlFor="rec-start">{tr.start}</Label>
            <Input id="rec-start" type="date" required value={start} onChange={(e) => { setStart(e.target.value); }} />
          </div>
          <div>
            <Label htmlFor="rec-end">{tr.end}</Label>
            <Input id="rec-end" type="date" value={end} min={start} onChange={(e) => { setEnd(e.target.value); }} />
          </div>
          <div>
            <Label htmlFor="rec-lead">{tr.lead}</Label>
            <Input id="rec-lead" type="number" min={0} max={365} value={lead} onChange={(e) => { setLead(e.target.value); }} />
          </div>
        </div>
        <p aria-live="polite" className="text-xs text-zinc-500">
          {describeSchedule(schedule)}
          {preview.data && preview.data.length > 0 && <> · {tr.upcoming}: {preview.data.map(formatDate).join(", ")}</>}
          {preview.isError && <span className="text-red-600"> · <FieldErrorText error={preview.error} /></span>}
        </p>
      </fieldset>

      <label className="flex items-center gap-2 text-sm">
        <input type="checkbox" className="accent-brand-600" checked={enabled} onChange={(e) => { setEnabled(e.target.checked); }} />
        {tr.enabled}
      </label>
      <FieldError error={rapi.save.error} />
      <div className="flex justify-end gap-2">
        <Button onClick={onDone}>{t.cancel}</Button>
        <Button type="submit" variant="primary" disabled={rapi.save.isPending}>{t.save}</Button>
      </div>
    </form>
  );
}

function FieldErrorText({ error }: { error: unknown }) {
  return <>{error instanceof Error ? error.message : t.errorGeneric}</>;
}
