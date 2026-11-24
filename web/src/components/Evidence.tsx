// Audit evidence export UI (ADR-0029): the item drawer action and the
// Settings → Audit log panel. Admin-only; the API enforces it as well.
import { useQueryClient } from "@tanstack/react-query";
import { useId, useMemo, useState } from "react";

import { evidenceKeys, exportItemEvidence, exportScopeEvidence, useEvidenceHistory, useEvidenceKey } from "../api/evidence";
import { collabKeys, useProjects } from "../api/hooks";
import { t } from "../i18n";
import { te } from "../i18n-evidence";
import { activeLocale } from "../i18n/locale";
import { defaultRange, evidenceErrorText, exportRecords, rangeProblem, rangeProblemText, verifyCommand } from "../lib/evidence";
import { formatDateTime } from "../lib/format";
import { useToast } from "./toast-context";
import { Button, ErrorState, Input, Label, Spinner } from "./ui";

const todayUTC = () => new Date().toISOString().slice(0, 10);

/** "Export evidence" in the item drawer header. */
export function ItemEvidenceButton({ ws, project, n }: { ws: string; project: string; n: number }) {
  const toast = useToast();
  const qc = useQueryClient();
  const [busy, setBusy] = useState(false);
  return (
    <Button
      variant="ghost"
      title={te.itemButtonHint}
      disabled={busy}
      aria-busy={busy}
      onClick={() => {
        setBusy(true);
        exportItemEvidence(ws, project, n, activeLocale())
          .then(() => {
            toast("success", te.downloaded);
            void qc.invalidateQueries({ queryKey: collabKeys.activity(ws, project, n) }); // the export is in the item's history
            void qc.invalidateQueries({ queryKey: evidenceKeys.history(ws) });
          })
          .catch((err: unknown) => { toast("error", evidenceErrorText(err) ?? t.errorGeneric); })
          .finally(() => { setBusy(false); });
      }}
    >
      {busy ? te.exporting : te.itemButton}
    </Button>
  );
}

/** Scope export form, signing key fingerprint and export history. */
export function EvidencePanel({ ws }: { ws: string }) {
  const key = useEvidenceKey(ws);
  if (key.isPending) return <Spinner />;
  if (key.isError) return <ErrorState error={key.error} />;
  return (
    <section aria-label={te.title} className="space-y-4 rounded-xl bg-white p-4 ring-1 ring-zinc-200 dark:bg-zinc-900 dark:ring-zinc-800">
      <div>
        <h2 className="text-base font-semibold">{te.title}</h2>
        <p className="mt-1 text-sm text-zinc-600 dark:text-zinc-400">{te.intro}</p>
      </div>
      {key.data.enabled ? (
        <>
          <ScopeForm ws={ws} maxDays={key.data.limits?.max_days ?? 366} />
          {key.data.limits && (
            <p className="text-xs text-zinc-500">{te.limits(key.data.limits.max_days, key.data.limits.max_items, key.data.limits.max_events)}</p>
          )}
          <KeyInfo fingerprint={key.data.fingerprint ?? ""} />
        </>
      ) : (
        <p role="status" className="rounded-md bg-amber-50 px-3 py-2 text-sm text-amber-800 dark:bg-amber-950 dark:text-amber-200">{te.disabled}</p>
      )}
      <History ws={ws} />
    </section>
  );
}

function ScopeForm({ ws, maxDays }: { ws: string; maxDays: number }) {
  const id = useId();
  const toast = useToast();
  const qc = useQueryClient();
  const projects = useProjects(ws);
  const initial = useMemo(() => defaultRange(todayUTC()), []);
  const [from, setFrom] = useState(initial.from);
  const [to, setTo] = useState(initial.to);
  const [keys, setKeys] = useState<string[]>([]);
  const [approvalsOnly, setApprovalsOnly] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const problem = rangeProblem(from, to, maxDays);

  return (
    <form
      className="space-y-3"
      onSubmit={(e) => {
        e.preventDefault();
        if (problem) {
          setError(rangeProblemText(problem, maxDays));
          return;
        }
        setError(null);
        setBusy(true);
        exportScopeEvidence(ws, { project_keys: keys, from, to, approvals_only: approvalsOnly }, activeLocale())
          .then(() => {
            toast("success", te.downloaded);
            void qc.invalidateQueries({ queryKey: evidenceKeys.history(ws) });
          })
          .catch((err: unknown) => { setError(evidenceErrorText(err) ?? t.errorGeneric); })
          .finally(() => { setBusy(false); });
      }}
    >
      <div className="flex flex-wrap gap-3">
        <div>
          <Label htmlFor={id + "from"}>{te.from}</Label>
          <Input id={id + "from"} type="date" required value={from} onChange={(e) => { setFrom(e.target.value); }} className="w-44" />
        </div>
        <div>
          <Label htmlFor={id + "to"}>{te.to}</Label>
          <Input id={id + "to"} type="date" required value={to} onChange={(e) => { setTo(e.target.value); }} className="w-44" />
        </div>
      </div>
      <fieldset>
        <legend className="mb-1 text-xs font-medium text-zinc-600 dark:text-zinc-400">{te.projects}</legend>
        <div className="flex max-h-40 flex-wrap gap-x-4 gap-y-1 overflow-y-auto text-sm">
          {(projects.data ?? []).map((p) => (
            <label key={p.id} className="flex items-center gap-1.5">
              <input
                type="checkbox"
                className="accent-brand-600"
                checked={keys.includes(p.key)}
                onChange={(e) => { setKeys(e.target.checked ? [...keys, p.key] : keys.filter((k) => k !== p.key)); }}
              />
              <span className="font-mono text-xs">{p.key}</span> {p.name}
            </label>
          ))}
        </div>
        <p className="mt-1 text-xs text-zinc-500">{te.projectsHint}</p>
      </fieldset>
      <label className="flex items-center gap-2 text-sm">
        <input type="checkbox" className="accent-brand-600" checked={approvalsOnly} onChange={(e) => { setApprovalsOnly(e.target.checked); }} />
        {te.approvalsOnly}
      </label>
      {error && <p role="alert" className="text-sm text-red-600 dark:text-red-400">{error}</p>}
      <Button type="submit" variant="primary" disabled={busy} aria-busy={busy}>{busy ? te.exporting : te.submit}</Button>
    </form>
  );
}

function KeyInfo({ fingerprint }: { fingerprint: string }) {
  const [copied, setCopied] = useState(false);
  const cmd = verifyCommand(fingerprint);
  return (
    <div className="space-y-1 rounded-md bg-zinc-50 p-3 text-sm dark:bg-zinc-950">
      <p className="font-medium">{te.key}: Ed25519</p>
      <p>
        {te.fingerprint}: <code data-testid="evidence-fingerprint" className="font-mono text-xs break-all">{fingerprint}</code>
      </p>
      <div className="flex items-start gap-2">
        <code className="block flex-1 font-mono text-xs break-all text-zinc-600 dark:text-zinc-400">{cmd}</code>
        <Button
          variant="ghost"
          className="text-xs"
          onClick={() => {
            void navigator.clipboard.writeText(cmd).then(() => {
              setCopied(true);
              setTimeout(() => { setCopied(false); }, 2000);
            });
          }}
        >
          {copied ? te.copied : te.copy}
        </Button>
      </div>
      <p className="text-xs text-zinc-500">{te.keyHint}</p>
    </div>
  );
}

function History({ ws }: { ws: string }) {
  const q = useEvidenceHistory(ws);
  const projects = useProjects(ws);
  const byId = useMemo(() => new Map((projects.data ?? []).map((p) => [p.id, p.key])), [projects.data]);
  if (q.isPending) return <Spinner />;
  if (q.isError) return <ErrorState error={q.error} />;
  const rows = exportRecords(q.data.activity, byId);
  return (
    <div>
      <h3 className="mb-2 text-sm font-semibold">{te.history}</h3>
      {rows.length === 0 ? (
        <p className="text-sm text-zinc-500">{te.historyEmpty}</p>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full min-w-[40rem] text-left text-sm" aria-label={te.history}>
            <thead className="text-xs text-zinc-500">
              <tr className="border-b border-zinc-200 dark:border-zinc-800">
                <th className="py-1 pr-3">{te.cols.time}</th><th className="pr-3">{te.cols.who}</th><th className="pr-3">{te.cols.what}</th>
                <th className="pr-3">{te.cols.content}</th><th>{te.cols.manifest}</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((r) => (
                <tr key={r.id} className="border-b border-zinc-100 align-top dark:border-zinc-800/60">
                  <td className="py-1 pr-3 text-xs whitespace-nowrap">{formatDateTime(r.time)}</td>
                  <td className="pr-3 text-xs">{r.who}</td>
                  <td className="pr-3 text-xs">{r.what}{r.approvalsOnly && <span className="ml-1 text-zinc-500">({te.approvalsTag})</span>}</td>
                  <td className="pr-3 text-xs whitespace-nowrap">{te.counts(r.items, r.events)}</td>
                  <td className="font-mono text-[11px] break-all text-zinc-500" title={r.manifest}>{r.manifest.slice(0, 16)}…</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
