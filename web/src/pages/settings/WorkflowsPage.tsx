import { useState } from "react";

import { useAdminApi, useWorkflows, type ApprovalPolicy, type ApproverRole, type Workflow, type WorkflowStatus } from "../../api/admin";
import { useMembers } from "../../api/hooks";
import type { StatusCategory } from "../../api/types";
import { tai } from "../../i18n-approval-inbox";
import { tap } from "../../i18n-approvals";
import { parseDue, policyExtrasFrom } from "../../lib/approvalInbox";
import { PolicyExtras } from "../../components/Approvals";
import { APPROVER_ROLES, MAX_MIN_APPROVALS, policyFor, policyProblem } from "../../lib/approvals";
import { Button, ErrorState, Input, Select, Spinner, StatusBadge } from "../../components/ui";
import { t } from "../../i18n";
import { ta } from "../../i18n-admin";
import { Card, SettingsLayout } from "./SettingsLayout";
import { useFeedback } from "./useErrorToast";

const CATEGORIES: StatusCategory[] = ["todo", "in_progress", "done", "canceled"];

export function WorkflowsPage() {
  return <SettingsLayout title={ta.nav.workflows}>{(ws) => <Workflows ws={ws} />}</SettingsLayout>;
}

function Workflows({ ws }: { ws: string }) {
  const q = useWorkflows(ws);
  if (q.isPending) return <Spinner />;
  if (q.isError) return <ErrorState error={q.error} />;
  return (
    <div className="space-y-4">
      <NewWorkflow ws={ws} />
      {q.data.map((w) => <WorkflowCard key={w.id} ws={ws} wf={w} />)}
    </div>
  );
}

function NewWorkflow({ ws }: { ws: string }) {
  const adm = useAdminApi(ws);
  const fb = useFeedback();
  const [name, setName] = useState("");
  return (
    <Card>
      <form
        className="flex flex-wrap items-end gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          // Start with a sensible default lifecycle; statuses can be edited afterwards.
          adm.createWorkflow.mutate(
            { name, statuses: [{ name: "To Do", category: "todo" }, { name: "In Progress", category: "in_progress" }, { name: "Done", category: "done" }] },
            { onSuccess: () => { setName(""); fb.ok()(); }, onError: fb.fail },
          );
        }}
      >
        <label className="text-xs text-zinc-500">
          {ta.newWorkflow}
          <Input required maxLength={200} value={name} onChange={(e) => { setName(e.target.value); }} className="w-64" />
        </label>
        <Button type="submit" variant="primary" disabled={adm.createWorkflow.isPending}>{t.create}</Button>
      </form>
    </Card>
  );
}

function WorkflowCard({ ws, wf }: { ws: string; wf: Workflow }) {
  const adm = useAdminApi(ws);
  const fb = useFeedback();
  const [name, setName] = useState(wf.name);
  const [stName, setStName] = useState("");
  const [stCat, setStCat] = useState<StatusCategory>("todo");
  const [from, setFrom] = useState("");
  const [to, setTo] = useState(wf.statuses[1]?.id ?? wf.statuses[0]?.id ?? "");
  const statusName = new Map(wf.statuses.map((s) => [s.id, s.name]));

  const move = async (index: number, dir: -1 | 1) => {
    // Renumber the whole list 0..n-1 in the new order, so arbitrary or
    // duplicate positions from earlier edits can never cause a wrong order.
    const order = [...wf.statuses];
    const [moved] = order.splice(index, 1);
    if (!moved) return;
    order.splice(index + dir, 0, moved);
    try {
      for (const [pos, st] of order.entries()) {
        if (st.position !== pos) await adm.updateStatus.mutateAsync({ wf: wf.id, id: st.id, body: { position: pos } });
      }
    } catch (err) {
      fb.fail(err);
    }
  };

  return (
    <Card>
      <Input
        aria-label={t.name}
        value={name}
        maxLength={200}
        className="mb-4 max-w-sm font-medium"
        onChange={(e) => { setName(e.target.value); }}
        onBlur={() => { if (name.trim() && name !== wf.name) adm.updateWorkflow.mutate({ id: wf.id, body: { name } }, { onSuccess: fb.ok(), onError: fb.fail }); }}
      />

      <div className="grid gap-6 lg:grid-cols-2">
        <section>
          <h3 className="mb-2 text-xs font-semibold tracking-wide text-zinc-500 uppercase">{ta.statuses}</h3>
          <ul className="space-y-1.5">
            {wf.statuses.map((s, i) => (
              <li key={s.id} className="flex flex-wrap items-center gap-2 text-sm">
                <span className="flex flex-col text-[10px] leading-none text-zinc-400">
                  <button type="button" aria-label="up" disabled={i === 0} onClick={() => void move(i, -1)} className="disabled:opacity-30">▲</button>
                  <button type="button" aria-label="down" disabled={i === wf.statuses.length - 1} onClick={() => void move(i, 1)} className="disabled:opacity-30">▼</button>
                </span>
                <StatusBadge name={s.name} category={s.category} />
                <Select
                  aria-label={`${s.name} category`}
                  value={s.category}
                  className="w-36 py-0.5 text-xs"
                  onChange={(e) => { adm.updateStatus.mutate({ wf: wf.id, id: s.id, body: { category: e.target.value as StatusCategory } }, { onSuccess: fb.ok(), onError: fb.fail }); }}
                >
                  {CATEGORIES.map((c) => <option key={c} value={c}>{t.category[c]}</option>)}
                </Select>
                {s.is_initial ? (
                  <span className="text-xs text-brand-600">{ta.initial}</span>
                ) : (
                  <>
                    <Button variant="ghost" className="px-1.5 py-0.5 text-xs" onClick={() => { adm.updateStatus.mutate({ wf: wf.id, id: s.id, body: { is_initial: true } }, { onSuccess: fb.ok(), onError: fb.fail }); }}>
                      {ta.makeInitial}
                    </Button>
                    <Button variant="ghost" className="px-1.5 py-0.5 text-xs text-red-600" onClick={() => { adm.deleteStatus.mutate({ wf: wf.id, id: s.id }, { onSuccess: fb.ok(), onError: fb.fail }); }}>
                      {t.delete}
                    </Button>
                  </>
                )}
              </li>
            ))}
          </ul>
          <form
            className="mt-3 flex flex-wrap gap-2"
            onSubmit={(e) => {
              e.preventDefault();
              adm.addStatus.mutate({ wf: wf.id, name: stName, category: stCat }, { onSuccess: () => { setStName(""); fb.ok()(); }, onError: fb.fail });
            }}
          >
            <Input required maxLength={100} placeholder={ta.newStatus} value={stName} onChange={(e) => { setStName(e.target.value); }} className="w-40" />
            <Select aria-label="category" value={stCat} onChange={(e) => { setStCat(e.target.value as StatusCategory); }} className="w-36">
              {CATEGORIES.map((c) => <option key={c} value={c}>{t.category[c]}</option>)}
            </Select>
            <Button type="submit">{ta.add}</Button>
          </form>
        </section>

        <section>
          <h3 className="mb-2 text-xs font-semibold tracking-wide text-zinc-500 uppercase">{ta.transitions}</h3>
          <label className="mb-1 flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              className="accent-brand-600"
              checked={wf.enforce_transitions}
              onChange={(e) => { adm.updateWorkflow.mutate({ id: wf.id, body: { enforce_transitions: e.target.checked } }, { onSuccess: fb.ok(), onError: fb.fail }); }}
            />
            {ta.enforce}
          </label>
          <p className="mb-3 text-xs text-zinc-500">{ta.enforceHint}</p>
          {wf.transitions.length === 0 && <p className="text-xs text-zinc-400">{ta.noTransitions}</p>}
          <ul className="space-y-1">
            {wf.transitions.map((tr) => (
              <li key={tr.id} className="flex flex-wrap items-center gap-2 text-sm">
                <span>{tr.from_status_id ? statusName.get(tr.from_status_id) : <em className="text-zinc-500">{ta.anyStatus}</em>}</span>
                <span className="text-zinc-400">→</span>
                <span className="font-medium">{statusName.get(tr.to_status_id)}</span>
                <span className="text-xs text-zinc-500">({tr.name})</span>
                <Button variant="ghost" className="ml-auto px-1.5 py-0.5 text-xs text-red-600" onClick={() => { adm.deleteTransition.mutate({ wf: wf.id, id: tr.id }, { onSuccess: fb.ok(), onError: fb.fail }); }}>
                  {t.delete}
                </Button>
              </li>
            ))}
          </ul>
          <form
            className="mt-3 flex flex-wrap items-center gap-2"
            onSubmit={(e) => {
              e.preventDefault();
              const label = `${from ? (statusName.get(from) ?? "") : ta.anyStatus} → ${statusName.get(to) ?? ""}`;
              adm.addTransition.mutate({ wf: wf.id, name: label.slice(0, 100), from_status_id: from || null, to_status_id: to }, { onSuccess: fb.ok(), onError: fb.fail });
            }}
          >
            <Select aria-label={ta.from} value={from} onChange={(e) => { setFrom(e.target.value); }} className="w-40">
              <option value="">{ta.anyStatus}</option>
              {wf.statuses.map((s) => <option key={s.id} value={s.id}>{s.name}</option>)}
            </Select>
            <span className="text-zinc-400">→</span>
            <Select aria-label={ta.to} value={to} onChange={(e) => { setTo(e.target.value); }} className="w-40">
              {wf.statuses.map((s) => <option key={s.id} value={s.id}>{s.name}</option>)}
            </Select>
            <Button type="submit">{ta.add}</Button>
          </form>
        </section>
      </div>

      <ApprovalPolicies ws={ws} wf={wf} />
    </Card>
  );
}

/** Four-eyes approval policies (ADR-0026): one optional policy per status. */
function ApprovalPolicies({ ws, wf }: { ws: string; wf: Workflow }) {
  return (
    <section className="mt-6 border-t border-zinc-200 pt-4 dark:border-zinc-800" aria-label={`${wf.name}: ${tap.settings.title}`}>
      <h3 className="mb-1 text-xs font-semibold tracking-wide text-zinc-500 uppercase">{tap.settings.title}</h3>
      <p className="mb-3 text-xs text-zinc-500">{tap.settings.hint}</p>
      {(wf.approval_policies ?? []).length === 0 && <p className="mb-2 text-xs text-zinc-400">{tap.settings.none}</p>}
      <ul className="space-y-1.5">
        {wf.statuses.map((st) => (
          <PolicyRow key={st.id + (policyFor(wf.approval_policies, st.id)?.id ?? "")} ws={ws} wf={wf} status={st} policy={policyFor(wf.approval_policies, st.id)} />
        ))}
      </ul>
    </section>
  );
}

function PolicyRow({ ws, wf, status, policy }: { ws: string; wf: Workflow; status: WorkflowStatus; policy: ApprovalPolicy | undefined }) {
  const adm = useAdminApi(ws);
  const fb = useFeedback();
  const members = useMembers(ws);
  const [min, setMin] = useState(policy?.min_approvals ?? 1);
  const [roles, setRoles] = useState<ApproverRole[]>(policy?.approver_roles ?? (policy ? [] : ["admin"]));
  const [users, setUsers] = useState<string[]>(policy?.approver_user_ids ?? []);
  const [extras, setExtras] = useState(() => policyExtrasFrom(policy));
  const due = parseDue(extras.due);
  const body = {
    min_approvals: min, approver_roles: roles, approver_user_ids: users,
    reapprove_on_change: extras.reapprove_on_change, reapprove_fields: extras.reapprove_fields, due_in_working_days: due,
  };
  const problem = policyProblem(body);
  const dueBad = due !== null && Number.isNaN(due);
  const toggle = <T,>(list: T[], v: T, on: boolean) => (on ? [...list, v] : list.filter((x) => x !== v));
  const eligible = (members.data ?? []).filter((m) => m.role !== "guest");

  return (
    <li className="text-sm">
      <details>
        <summary className="flex cursor-pointer flex-wrap items-center gap-2 pointer-coarse:min-h-11">
          <StatusBadge name={status.name} category={status.category} />
          {policy ? (
            <span className="text-xs text-amber-700 dark:text-amber-300">
              🔏 {tap.settings.summary(policy.min_approvals)}
              {policy.due_in_working_days ? ` · ⏰ ${tai.settings.summaryDue(policy.due_in_working_days)}` : ""}
              {policy.reapprove_on_change ? ` · ♻️ ${tai.settings.summaryReapprove}` : ""}
            </span>
          ) : (
            <span className="text-xs text-zinc-400">—</span>
          )}
        </summary>
        <form
          className="mt-2 space-y-3 rounded-md p-3 ring-1 ring-zinc-200 dark:ring-zinc-800"
          onSubmit={(e) => {
            e.preventDefault();
            if (problem || dueBad) return;
            adm.setApprovalPolicy.mutate({ wf: wf.id, status: status.id, body }, { onSuccess: fb.ok(), onError: fb.fail });
          }}
        >
          <label className="flex items-center gap-2 text-xs text-zinc-500">
            {tap.settings.min}
            <Input
              type="number"
              min={1}
              max={MAX_MIN_APPROVALS}
              value={min}
              onChange={(e) => { setMin(Number(e.target.value)); }}
              className="w-20"
            />
          </label>
          <fieldset>
            <legend className="mb-1 text-xs text-zinc-500">{tap.settings.roles}</legend>
            <div className="flex flex-wrap gap-3">
              {APPROVER_ROLES.map((r) => (
                <label key={r} className="flex items-center gap-1.5 pointer-coarse:min-h-11">
                  <input type="checkbox" className="accent-brand-600" checked={roles.includes(r)} onChange={(e) => { setRoles(toggle(roles, r, e.target.checked)); }} />
                  {ta.roles[r]}
                </label>
              ))}
            </div>
          </fieldset>
          <fieldset>
            <legend className="mb-1 text-xs text-zinc-500">{tap.settings.users}</legend>
            <div className="flex max-h-40 flex-wrap gap-x-3 gap-y-1 overflow-y-auto">
              {eligible.map((m) => (
                <label key={m.user_id} className="flex items-center gap-1.5 pointer-coarse:min-h-11">
                  <input type="checkbox" className="accent-brand-600" checked={users.includes(m.user_id)} onChange={(e) => { setUsers(toggle(users, m.user_id, e.target.checked)); }} />
                  {m.display_name}
                </label>
              ))}
            </div>
          </fieldset>
          <PolicyExtras ws={ws} value={extras} onChange={setExtras} />
          {problem && problem !== "range" && <p className="text-xs text-red-600">{tap.settings[problem]}</p>}
          <div className="flex flex-wrap gap-2">
            <Button type="submit" variant="primary" disabled={problem !== null || dueBad || adm.setApprovalPolicy.isPending}>
              {policy ? tap.settings.save : tap.settings.add}
            </Button>
            {policy && (
              <Button type="button" variant="ghost" className="text-red-600" onClick={() => { adm.deleteApprovalPolicy.mutate({ wf: wf.id, status: status.id }, { onSuccess: fb.ok(), onError: fb.fail }); }}>
                {tap.settings.remove}
              </Button>
            )}
          </div>
        </form>
      </details>
    </li>
  );
}
