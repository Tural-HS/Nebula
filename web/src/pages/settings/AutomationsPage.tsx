import { useState } from "react";

import { useAdminApi, useRules, type Condition, type ConditionOp, type Rule, type RuleAction } from "../../api/admin";
import { useFields, useItemTypes, useMembers, useProjects } from "../../api/hooks";
import type { FieldDefinition, ItemType, Member, Project } from "../../api/types";
import { Button, ErrorState, Input, Select, Spinner } from "../../components/ui";
import { t } from "../../i18n";
import { ta } from "../../i18n-admin";
import { cn } from "../../lib/cn";
import {
  conditionText, conditionValue, copyableFields, copyMatchFields, copySummary, fieldValue, MAX_COPY_FIELDS, NO_VALUE_OPS, recipients, toggleKey,
} from "../../lib/rules";
import { Card, SettingsLayout } from "./SettingsLayout";
import { useFeedback } from "./useErrorToast";
import { useConfirm } from "../../components/confirm-context";

const BASE_FIELDS = ["type", "project", "status", "status_category", "assignee", "reporter", "title", "due_date"];
const OPS: ConditionOp[] = ["eq", "neq", "in", "not_in", "contains", "is_empty", "is_not_empty"];
const CHANGEABLE = ["title", "description", "status_id", "assignee_id"];

export function AutomationsPage() {
  return <SettingsLayout title={ta.nav.automations}>{(ws) => <Automations ws={ws} />}</SettingsLayout>;
}

interface Ctx {
  types: ItemType[];
  members: Member[];
  fields: FieldDefinition[];
  /** Active projects: copy_fields targets (ADR-0056). */
  projects: Project[];
}

function Automations({ ws }: { ws: string }) {
  const confirm = useConfirm();
  const rules = useRules(ws);
  const types = useItemTypes(ws);
  const members = useMembers(ws);
  const fields = useFields(ws);
  const projects = useProjects(ws);
  const adm = useAdminApi(ws);
  const fb = useFeedback();
  const [editing, setEditing] = useState<Rule | "new" | null>(null);

  if (rules.isPending || types.isPending || members.isPending || fields.isPending || projects.isPending) return <Spinner />;
  if (rules.isError) return <ErrorState error={rules.error} />;
  const ctx: Ctx = {
    types: types.data ?? [],
    members: members.data ?? [],
    fields: (fields.data ?? []).filter((f) => !f.archived),
    projects: (projects.data ?? []).filter((p) => !p.archived_at && !p.deleting),
  };
  const fieldName = (key: string) => ctx.fields.find((f) => f.key === key)?.name ?? key;

  return (
    <div className="space-y-4">
      {editing === null ? (
        <Button variant="primary" onClick={() => { setEditing("new"); }}>{ta.newRule}</Button>
      ) : (
        <RuleEditor
          key={editing === "new" ? "new" : editing.id}
          ws={ws}
          ctx={ctx}
          rule={editing === "new" ? null : editing}
          onDone={() => { setEditing(null); }}
        />
      )}
      {rules.data.map((r) => (
        <Card key={r.id} className={cn(!r.enabled && "opacity-60")}>
          <div className="flex flex-wrap items-center gap-3">
            <label className="flex items-center gap-1.5 text-xs">
              <input
                type="checkbox"
                className="accent-brand-600"
                checked={r.enabled}
                onChange={(e) => { adm.updateRule.mutate({ id: r.id, body: { enabled: e.target.checked } }, { onSuccess: fb.ok(), onError: fb.fail }); }}
              />
              {r.enabled ? ta.enabled : ta.disabled}
            </label>
            <span className="font-medium">{r.name}</span>
            <span className="text-xs text-zinc-500 dark:text-zinc-400">
              {ta.triggers[r.trigger]} · {r.conditions.length} / {r.actions.map((a) => (a.type === "copy_fields" ? `${ta.actionTypes[a.type]} (${copySummary(a.project, a.fields, fieldName)})` : ta.actionTypes[a.type])).join(", ")}
            </span>
            <span className="ml-auto flex gap-2">
              <Button variant="ghost" onClick={() => { setEditing(r); }}>{ta.editRule}</Button>
              <Button variant="ghost" className="text-red-600" onClick={() => { void confirm({ title: ta.confirm, action: t.delete, danger: true }).then((ok) => { if (ok) adm.deleteRule.mutate(r.id, { onSuccess: fb.ok(), onError: fb.fail }); }); }}>
                {t.delete}
              </Button>
            </span>
          </div>
        </Card>
      ))}
    </div>
  );
}

// Form state keeps raw strings; they are converted to API values on save.
interface CondRow { field: string; op: ConditionOp; text: string }
interface ActRow {
  type: RuleAction["type"]; status_id: string; user_id: string; key: string; text: string; to: string; subject: string; body: string;
  project: string; match: string; fields: string[]; create: boolean;
}

const emptyAction = (): ActRow => ({
  type: "set_assignee", status_id: "", user_id: "", key: "", text: "", to: "assignee", subject: "[{{item.key}}] {{item.title}}", body: "{{item.key}}: {{item.status}}",
  project: "", match: "", fields: [], create: false,
});

function toRows(r: Rule | null): { conds: CondRow[]; acts: ActRow[] } {
  if (!r) return { conds: [], acts: [emptyAction()] };
  return {
    conds: r.conditions.map((c) => ({ field: c.field, op: c.op, text: conditionText(c.value) })),
    acts: r.actions.map((a) => ({
      ...emptyAction(),
      type: a.type,
      status_id: a.status_id ?? "",
      user_id: a.user_id ?? "",
      key: a.key ?? "",
      text: conditionText(a.value),
      to: (a.to ?? []).join(", "),
      subject: a.subject ?? "",
      body: a.body ?? "",
      project: a.project ?? "",
      match: a.match ?? "",
      fields: a.fields ?? [],
      create: a.create ?? false,
    })),
  };
}

function RuleEditor({ ws, ctx, rule, onDone }: { ws: string; ctx: Ctx; rule: Rule | null; onDone: () => void }) {
  const adm = useAdminApi(ws);
  const fb = useFeedback();
  const initial = toRows(rule);
  const [name, setName] = useState(rule?.name ?? "");
  const [trigger, setTrigger] = useState<Rule["trigger"]>(rule?.trigger ?? "item.created");
  const [conds, setConds] = useState<CondRow[]>(initial.conds);
  const [acts, setActs] = useState<ActRow[]>(initial.acts);
  const fieldByKey = new Map(ctx.fields.map((f) => [f.key, f]));

  const build = (): Omit<Rule, "id" | "updated_at"> => ({
    name,
    trigger,
    enabled: rule?.enabled ?? true,
    conditions: conds.map((c): Condition => {
      if (c.field === "changed") return { field: "changed", op: "includes", value: c.text };
      const v = conditionValue(c.op, c.text);
      return v === undefined ? { field: c.field, op: c.op } : { field: c.field, op: c.op, value: v };
    }),
    actions: acts.map((a): RuleAction => {
      switch (a.type) {
        case "set_status":
          return { type: "set_status", status_id: a.status_id };
        case "set_assignee":
          return { type: "set_assignee", user_id: a.user_id || null };
        case "set_field":
          return { type: "set_field", key: a.key, value: fieldValue(fieldByKey.get(a.key), a.text) };
        case "send_email":
          return { type: "send_email", to: recipients(a.to), subject: a.subject, body: a.body };
        case "copy_fields":
          return { type: "copy_fields", project: a.project, match: a.match, fields: a.fields.filter((k) => k !== a.match), ...(a.create ? { create: true } : {}) };
      }
    }),
  });

  const save = () => {
    const body = build();
    const opts = { onSuccess: () => { fb.ok()(); onDone(); }, onError: fb.fail };
    if (rule) adm.updateRule.mutate({ id: rule.id, body }, opts);
    else adm.createRule.mutate(body, opts);
  };

  const statuses = ctx.types.flatMap((ty) => ty.statuses.map((s) => ({ id: s.id, label: `${ty.name} / ${s.name}` })));
  const condFieldOptions = [...BASE_FIELDS, ...ctx.fields.map((f) => "fields." + f.key), ...(trigger === "item.updated" ? ["changed"] : [])];

  return (
    <Card className="space-y-4 ring-2 ring-brand-500">
      <div className="flex flex-wrap gap-3">
        <label className="text-xs text-zinc-500 dark:text-zinc-400">
          {t.name}
          <Input required maxLength={200} value={name} onChange={(e) => { setName(e.target.value); }} className="w-64" />
        </label>
        <label className="text-xs text-zinc-500 dark:text-zinc-400">
          {ta.trigger}
          <Select value={trigger} onChange={(e) => { setTrigger(e.target.value as Rule["trigger"]); }} className="w-56">
            {Object.entries(ta.triggers).map(([k, v]) => <option key={k} value={k}>{v}</option>)}
          </Select>
        </label>
      </div>

      <section>
        <h3 className="mb-2 text-xs font-semibold text-zinc-500 dark:text-zinc-400">{ta.conditions}</h3>
        {conds.length === 0 && <p className="mb-2 text-xs text-zinc-500 dark:text-zinc-400">{ta.noConditions}</p>}
        <div className="space-y-2">
          {conds.map((c, i) => (
            <div key={i} className="flex flex-wrap items-center gap-2">
              <Select aria-label="field" value={c.field} className="w-48" onChange={(e) => {
                // A value only makes sense for the field it was picked for: reset it.
                setConds(update(conds, i, { field: e.target.value, op: e.target.value === "changed" ? "includes" : c.op === "includes" ? "eq" : c.op, text: "" }));
              }}>
                {condFieldOptions.map((f) => <option key={f} value={f}>{ta.condFields[f] ?? fieldByKey.get(f.replace("fields.", ""))?.name ?? f}</option>)}
              </Select>
              {c.field !== "changed" && (
                <Select aria-label="operator" value={c.op} className="w-40" onChange={(e) => { setConds(update(conds, i, { op: e.target.value as ConditionOp })); }}>
                  {OPS.map((o) => <option key={o} value={o}>{ta.ops[o]}</option>)}
                </Select>
              )}
              {c.field === "changed" ? (
                <Select aria-label="changed field" value={c.text} className="w-48" onChange={(e) => { setConds(update(conds, i, { text: e.target.value })); }}>
                  <option value="">—</option>
                  {[...CHANGEABLE, ...ctx.fields.map((f) => "fields." + f.key)].map((f) => <option key={f} value={f}>{f}</option>)}
                </Select>
              ) : (
                !NO_VALUE_OPS.includes(c.op) && (
                  <ValueInput field={c.field} value={c.text} ctx={ctx} onChange={(v) => { setConds(update(conds, i, { text: v })); }} />
                )
              )}
              <Button variant="ghost" onClick={() => { setConds(conds.filter((_, j) => j !== i)); }}>×</Button>
            </div>
          ))}
        </div>
        <Button className="mt-2" onClick={() => { setConds([...conds, { field: "type", op: "eq", text: ctx.types[0]?.key ?? "" }]); }}>{ta.addCondition}</Button>
      </section>

      <section>
        <h3 className="mb-2 text-xs font-semibold text-zinc-500 dark:text-zinc-400">{ta.ruleActions}</h3>
        <div className="space-y-3">
          {acts.map((a, i) => (
            <div key={i} className="flex flex-wrap items-start gap-2 rounded-lg bg-zinc-50 p-2 dark:bg-zinc-950">
              <Select aria-label="action" value={a.type} className="w-44" onChange={(e) => { setActs(update(acts, i, { type: e.target.value as RuleAction["type"] })); }}>
                {Object.entries(ta.actionTypes).map(([k, v]) => <option key={k} value={k}>{v}</option>)}
              </Select>
              {a.type === "set_status" && (
                <Select aria-label={t.status} value={a.status_id} className="w-64" onChange={(e) => { setActs(update(acts, i, { status_id: e.target.value })); }}>
                  <option value="">—</option>
                  {statuses.map((s) => <option key={s.id} value={s.id}>{s.label}</option>)}
                </Select>
              )}
              {a.type === "set_assignee" && (
                <Select aria-label={t.assignee} value={a.user_id} className="w-56" onChange={(e) => { setActs(update(acts, i, { user_id: e.target.value })); }}>
                  <option value="">{t.unassigned}</option>
                  {ctx.members.map((m) => <option key={m.user_id} value={m.user_id}>{m.display_name}</option>)}
                </Select>
              )}
              {a.type === "set_field" && (
                <>
                  <Select aria-label="field" value={a.key} className="w-48" onChange={(e) => { setActs(update(acts, i, { key: e.target.value, text: "" })); }}>
                    <option value="">—</option>
                    {ctx.fields.map((f) => <option key={f.key} value={f.key}>{f.name}</option>)}
                  </Select>
                  <ValueInput field={"fields." + a.key} value={a.text} ctx={ctx} onChange={(v) => { setActs(update(acts, i, { text: v })); }} />
                </>
              )}
              {a.type === "copy_fields" && <CopyFieldsEditor row={a} ctx={ctx} onChange={(patch) => { setActs(update(acts, i, patch)); }} />}
              {a.type === "send_email" && (
                <div className="grid min-w-[min(18rem,100%)] flex-1 gap-2">
                  <Input aria-label={ta.emailTo} placeholder={ta.emailToHint} value={a.to} onChange={(e) => { setActs(update(acts, i, { to: e.target.value })); }} />
                  <Input aria-label={ta.subject} placeholder={ta.subject} maxLength={200} value={a.subject} onChange={(e) => { setActs(update(acts, i, { subject: e.target.value })); }} />
                  <textarea
                    aria-label={ta.body}
                    rows={3}
                    maxLength={10_000}
                    value={a.body}
                    onChange={(e) => { setActs(update(acts, i, { body: e.target.value })); }}
                    className="w-full rounded-md bg-white px-2.5 py-1.5 text-sm ring-1 ring-zinc-300 dark:bg-zinc-900 dark:ring-zinc-700"
                  />
                  <p className="text-xs text-zinc-500 dark:text-zinc-400">{ta.templateHint}</p>
                </div>
              )}
              <Button variant="ghost" disabled={acts.length === 1} onClick={() => { setActs(acts.filter((_, j) => j !== i)); }}>×</Button>
            </div>
          ))}
        </div>
        <Button className="mt-2" onClick={() => { setActs([...acts, emptyAction()]); }}>{ta.addAction}</Button>
      </section>

      <div className="flex justify-end gap-2">
        <Button onClick={onDone}>{t.cancel}</Button>
        <Button variant="primary" onClick={save} disabled={!name.trim() || adm.createRule.isPending || adm.updateRule.isPending}>{t.save}</Button>
      </div>
    </Card>
  );
}

/** copy_fields (ADR-0056): target project, match field and the fields to copy. */
function CopyFieldsEditor({ row, ctx, onChange }: { row: ActRow; ctx: Ctx; onChange: (patch: Partial<ActRow>) => void }) {
  const copyable = copyableFields(ctx.fields, row.match);
  const full = row.fields.length >= MAX_COPY_FIELDS;
  return (
    <div className="grid min-w-[min(18rem,100%)] flex-1 gap-2">
      <div className="flex flex-wrap gap-2">
        <label className="text-xs text-zinc-500 dark:text-zinc-400">
          {ta.copyProject}
          <Select value={row.project} className="w-56" onChange={(e) => { onChange({ project: e.target.value }); }}>
            <option value="">—</option>
            {ctx.projects.map((p) => <option key={p.key} value={p.key}>{`${p.key} · ${p.name}`}</option>)}
          </Select>
        </label>
        <label className="text-xs text-zinc-500 dark:text-zinc-400">
          {ta.copyMatch}
          <Select value={row.match} className="w-56" onChange={(e) => { onChange({ match: e.target.value, fields: row.fields.filter((k) => k !== e.target.value) }); }}>
            <option value="">—</option>
            {copyMatchFields(ctx.fields).map((f) => <option key={f.key} value={f.key}>{f.name}</option>)}
          </Select>
        </label>
      </div>
      <fieldset>
        <legend className="mb-1 text-xs text-zinc-500 dark:text-zinc-400">{ta.copyFields} ({row.fields.length}/{MAX_COPY_FIELDS})</legend>
        <div className="flex max-h-48 flex-wrap gap-x-4 gap-y-1 overflow-y-auto">
          {copyable.map((f) => {
            const checked = row.fields.includes(f.key);
            return (
              <label key={f.key} className="flex items-center gap-1.5 text-sm">
                <input
                  type="checkbox"
                  className="accent-brand-600"
                  checked={checked}
                  disabled={!checked && full}
                  onChange={() => { onChange({ fields: toggleKey(row.fields, f.key) }); }}
                />
                {f.name}
              </label>
            );
          })}
        </div>
      </fieldset>
      <label className="flex items-center gap-1.5 text-sm">
        <input type="checkbox" className="accent-brand-600" checked={row.create} onChange={(e) => { onChange({ create: e.target.checked }); }} />
        {ta.copyCreate}
      </label>
      <p className="text-xs text-zinc-500 dark:text-zinc-400">{ta.copyHint}</p>
    </div>
  );
}

/** Value input that offers pickers where the domain is known. */
function ValueInput({ field, value, ctx, onChange }: { field: string; value: string; ctx: Ctx; onChange: (v: string) => void }) {
  const def = field.startsWith("fields.") ? ctx.fields.find((f) => "fields." + f.key === field) : undefined;
  const pick = (options: [string, string][]) => (
    <Select aria-label={ta.value} value={value} className="w-56" onChange={(e) => { onChange(e.target.value); }}>
      <option value="">—</option>
      {options.map(([v, l]) => <option key={v} value={v}>{l}</option>)}
    </Select>
  );
  if (field === "type") return pick(ctx.types.map((x) => [x.key, x.name]));
  if (field === "status") return pick(ctx.types.flatMap((x) => x.statuses.map((s): [string, string] => [s.id, `${x.name} / ${s.name}`])));
  if (field === "status_category") return pick(Object.entries(t.category));
  if (field === "assignee" || field === "reporter") return pick(ctx.members.map((m) => [m.user_id, m.display_name]));
  if (def && (def.type === "select" || def.type === "multi_select")) return pick((def.options.choices ?? []).filter((c) => !c.archived).map((c) => [c.id, c.label]));
  if (def?.type === "boolean") return pick([["true", t.yes], ["false", t.no]]);
  if (def?.type === "user") return pick(ctx.members.map((m) => [m.user_id, m.display_name]));
  return <Input aria-label={ta.value} value={value} className="w-56" onChange={(e) => { onChange(e.target.value); }} />;
}

function update<T>(list: T[], i: number, patch: Partial<T>): T[] {
  return list.map((x, j) => (j === i ? { ...x, ...patch } : x));
}
