// Four-eyes approvals UI (ADR-0026): the list badge, the drawer panel and
// the request / reject dialogs. The server decides who may do what; the
// buttons only follow its can_approve / can_cancel flags. ADR-0034 adds
// deadlines, the re-approval markers, the policy's extra options and the
// link to the workspace inbox.
import { useEffect, useRef, useState } from "react";
import { Link, useSearchParams } from "react-router";

import { useApprovalActions, useApprovalInboxCount, useApprovals, type ApprovalRequest } from "../api/approvals";
import { ApiError } from "../api/client";
import { useFields } from "../api/hooks";
import type { Item, PendingApproval } from "../api/types";
import { t } from "../i18n";
import { tai } from "../i18n-approval-inbox";
import { tap } from "../i18n-approvals";
import { parseDue, toggleWatched, watchTokens, type PolicyExtrasValue } from "../lib/approvalInbox";
import { approvalErrorCode, approvalErrorText, progress } from "../lib/approvals";
import { cn } from "../lib/cn";
import { formatDateTime } from "../lib/format";
import { badgeText } from "../lib/notifications";
import { Button, Dialog, ErrorState, Input, Spinner, StatusBadge, touchField } from "./ui";
import { useToast } from "./toast-context";

export function ApprovalBadge({ approval, className }: { approval?: PendingApproval | null; className?: string }) {
  if (!approval) return null;
  const p = progress(approval.approvals, approval.min_approvals);
  const label = tap.badge(p.done, p.min);
  const overdue = Boolean(approval.overdue && approval.due_on);
  const due = approval.due_on ? (overdue ? tai.overdue(approval.due_on) : tai.due(approval.due_on)) : "";
  return (
    <span
      title={`${label} → ${approval.to_status.name}${due ? ` · ${due}` : ""}`}
      aria-label={due ? `${label}, ${due}` : label}
      data-testid="approval-badge"
      data-overdue={overdue || undefined}
      className={cn(
        "inline-flex shrink-0 items-center gap-1 rounded px-1.5 py-0.5 text-[11px] whitespace-nowrap tabular-nums ring-1",
        overdue
          ? "bg-red-50 text-red-700 ring-red-200 dark:bg-red-950 dark:text-red-300 dark:ring-red-900"
          : "bg-amber-50 text-amber-800 ring-amber-200 dark:bg-amber-950 dark:text-amber-200 dark:ring-amber-900",
        className,
      )}
    >
      <span aria-hidden>{overdue ? "⏰" : "⏳"}</span>
      {p.done}/{p.min}
    </span>
  );
}

/** Deadline chip of a pending request: amber while due, red once passed. */
export function DueChip({ dueOn, overdue }: { dueOn?: string | null; overdue?: boolean }) {
  if (!dueOn) return null;
  return (
    <span
      data-testid="approval-due"
      className={cn(
        "rounded px-1.5 py-0.5 text-xs whitespace-nowrap tabular-nums",
        overdue ? "bg-red-50 font-medium text-red-700 dark:bg-red-950 dark:text-red-300" : "bg-zinc-100 text-zinc-600 dark:bg-zinc-800 dark:text-zinc-300",
      )}
    >
      {overdue ? `⏰ ${tai.overdue(dueOn)}` : tai.due(dueOn)}
    </span>
  );
}

/**
 * Link to the workspace inbox with the viewer's count (ADR-0034). It lives on
 * the workspace page, not in the header: the phone header is full (ADR-0019).
 */
export function ApprovalInboxLink({ ws, className }: { ws: string; className?: string }) {
  const count = useApprovalInboxCount(ws).data;
  const n = count?.count ?? 0;
  const badge = badgeText(n, count ? n >= count.cap : false);
  const overdue = count?.overdue ?? 0;
  return (
    <Link
      to={`/w/${encodeURIComponent(ws)}/approvals`}
      aria-label={n > 0 ? `${tai.nav}: ${tai.badge(n)}${overdue > 0 ? `, ${tai.badgeOverdue(overdue)}` : ""}` : tai.nav}
      className={cn(
        "inline-flex min-w-0 items-center gap-2 rounded-lg bg-white px-3 py-2 text-sm ring-1 ring-zinc-200 hover:ring-brand-500 pointer-coarse:min-h-11 dark:bg-zinc-900 dark:ring-zinc-800",
        className,
      )}
    >
      <span aria-hidden>🔏</span>
      <span className="truncate font-medium">{tai.nav}</span>
      {badge && (
        <span aria-hidden className={cn("min-w-5 rounded-full px-1.5 text-center text-xs leading-5 font-semibold text-white", overdue > 0 ? "bg-red-600" : "bg-brand-600")}>
          {badge}
        </span>
      )}
    </Link>
  );
}

const textareaClass = cn(
  touchField,
  "w-full rounded-md bg-white px-2 py-1.5 text-sm ring-1 ring-zinc-300 focus:ring-2 focus:ring-brand-500 focus:outline-none dark:bg-zinc-900 dark:ring-zinc-700",
);

/** Asks for approval of a move into a guarded status, with an optional comment. */
export function RequestApprovalDialog({ ws, project, item, version, target, onClose }: {
  ws: string; project: string; item: Item;
  /** The latest version the drawer saved (its edits run in a queue). */
  version: () => number;
  target: { id: string; name: string } | null; onClose: () => void;
}) {
  const actions = useApprovalActions(ws, project, item.number);
  const toast = useToast();
  const [comment, setComment] = useState("");
  return (
    <Dialog open={target !== null} onClose={onClose} title={target ? tap.requestTitle(target.name) : ""}>
      <form
        className="space-y-3"
        onSubmit={(e) => {
          e.preventDefault();
          if (!target) return;
          actions.request.mutate(
            { version: Math.max(item.version, version()), to_status_id: target.id, comment: comment.trim() || undefined },
            {
              onSuccess: () => { toast("success", tap.requested); setComment(""); onClose(); },
              onError: (err) => { toast("error", err instanceof ApiError && err.isConflict && !approvalErrorCode(err) ? t.conflict : approvalErrorText(err)); },
            },
          );
        }}
      >
        <p className="text-sm text-zinc-600 dark:text-zinc-400">{tap.requestHint}</p>
        <label className="block text-xs text-zinc-500">
          {tap.comment}
          <textarea value={comment} maxLength={2000} rows={3} onChange={(e) => { setComment(e.target.value); }} className={cn(textareaClass, "mt-1")} />
        </label>
        <div className="flex justify-end gap-2">
          <Button type="button" variant="ghost" onClick={onClose}>{t.cancel}</Button>
          <Button type="submit" variant="primary" disabled={actions.request.isPending}>{tap.request}</Button>
        </div>
      </form>
    </Dialog>
  );
}

/** The drawer section: the open request with actions, then earlier ones. */
/**
 * ?focus=approval (the chat card link, ADR-0031) scrolls to the panel once the
 * requests have loaded, then drops the parameter so a reload or a later
 * drawer does not jump again. A query parameter rather than a #fragment: the
 * sign-in redirect keeps only path and query (safeReturnPath).
 */
function useApprovalFocus(loaded: boolean) {
  const [params, setParams] = useSearchParams();
  const ref = useRef<HTMLElement>(null);
  const wanted = params.get("focus") === "approval";
  useEffect(() => {
    if (!wanted || !loaded) return;
    const el = ref.current;
    if (el) {
      el.scrollIntoView({ block: "start" });
      el.focus({ preventScroll: true });
    }
    setParams((prev) => {
      const next = new URLSearchParams(prev);
      next.delete("focus");
      return next;
    }, { replace: true });
  }, [wanted, loaded, setParams]);
  return ref;
}

export function ApprovalPanel({ ws, project, item }: { ws: string; project: string; item: Item }) {
  const list = useApprovals(ws, project, item.number);
  const focusRef = useApprovalFocus(!list.isPending);
  // An edit of approved content opens a new request server-side (ADR-0034):
  // the item comes back with it before this list knows it, so reload.
  const pendingId = item.approval?.id;
  const known = list.data?.some((r) => r.id === pendingId) ?? true;
  const { refetch } = list;
  useEffect(() => {
    if (pendingId && !known) void refetch();
  }, [pendingId, known, refetch]);
  if (list.isPending) return item.approval ? <Spinner /> : null;
  if (list.isError) return <ErrorState error={list.error} />;
  if (list.data.length === 0) return null;
  // The newest request stays visible: the open one, or else the latest
  // decision, so whoever just approved or rejected sees the outcome. Older
  // requests are folded into the history.
  const [latest, ...rest] = list.data;
  return (
    <section ref={focusRef} id="approval" tabIndex={-1} aria-labelledby="approval-heading" className="scroll-mt-4 space-y-3 focus:outline-none">
      <h3 id="approval-heading" className="text-sm font-semibold">{tap.section}</h3>
      {latest && <RequestCard ws={ws} project={project} n={item.number} req={latest} />}
      {latest && <ReapproveWarning ws={ws} req={latest} item={item} />}
      {rest.length > 0 && (
        <details className="text-sm">
          <summary className="cursor-pointer text-zinc-500 pointer-coarse:min-h-11 pointer-coarse:py-2">{tap.history}</summary>
          <ul className="mt-2 space-y-2">
            {rest.map((r) => <li key={r.id}><RequestCard ws={ws} project={project} n={item.number} req={r} /></li>)}
          </ul>
        </details>
      )}
    </section>
  );
}

const stateStyle: Record<ApprovalRequest["state"], string> = {
  pending: "bg-amber-50 text-amber-800 dark:bg-amber-950 dark:text-amber-200",
  approved: "bg-emerald-50 text-emerald-700 dark:bg-emerald-950 dark:text-emerald-300",
  rejected: "bg-red-50 text-red-700 dark:bg-red-950 dark:text-red-300",
  canceled: "bg-zinc-100 text-zinc-600 dark:bg-zinc-800 dark:text-zinc-400",
};

function RequestCard({ ws, project, n, req }: { ws: string; project: string; n: number; req: ApprovalRequest }) {
  const actions = useApprovalActions(ws, project, n);
  const toast = useToast();
  const [rejecting, setRejecting] = useState(false);
  const p = progress(req.approvals, req.min_approvals);
  const busy = actions.approve.isPending || actions.reject.isPending || actions.cancel.isPending;
  const fail = (err: unknown) => { toast("error", approvalErrorText(err)); };
  const pending = req.state === "pending";

  return (
    <div data-testid={pending ? "approval-open" : "approval-closed"} className="space-y-2 rounded-lg p-3 text-sm ring-1 ring-zinc-200 dark:ring-zinc-800">
      <div className="flex flex-wrap items-center gap-2">
        <span className={cn("rounded px-1.5 py-0.5 text-xs font-medium", stateStyle[req.state])}>{tap.state[req.state]}</span>
        {req.from_status && <StatusBadge name={req.from_status.name} category={req.from_status.category} />}
        <span className="text-zinc-400" aria-hidden>→</span>
        {req.to_status && <StatusBadge name={req.to_status.name} category={req.to_status.category} />}
        {pending && <DueChip dueOn={req.due_on} overdue={req.overdue} />}
        <span className="ml-auto text-xs text-zinc-500 tabular-nums">{tap.progress(p.done, p.min)}</span>
      </div>
      {req.invalidated_at && (
        <p data-testid="approval-invalidated" className="rounded bg-amber-50 px-2 py-1 text-xs text-amber-800 dark:bg-amber-950 dark:text-amber-200">
          ♻️ {tai.invalidated} · {formatDateTime(req.invalidated_at)}
        </p>
      )}
      {pending && req.reopened_from && <p className="text-xs text-amber-700 dark:text-amber-300">♻️ {tai.reopened}</p>}
      {pending && (
        <div
          role="progressbar"
          aria-valuemin={0}
          aria-valuemax={p.min}
          aria-valuenow={p.done}
          aria-label={tap.progress(p.done, p.min)}
          className="h-1.5 overflow-hidden rounded bg-zinc-100 dark:bg-zinc-800"
        >
          <div className="h-full bg-amber-500" style={{ width: `${String(Math.round(p.ratio * 100))}%` }} />
        </div>
      )}
      <p className="text-xs text-zinc-500">
        {req.requested_by && tap.requestedBy(req.requested_by.display_name)} · {formatDateTime(req.created_at)}
        {req.closed_by && req.closed_at && <> · {tap.closedBy(req.closed_by.display_name)} · {formatDateTime(req.closed_at)}</>}
      </p>
      {req.comment && <p className="[overflow-wrap:anywhere] whitespace-pre-wrap">{req.comment}</p>}

      {req.decisions.length > 0 && (
        <ul className="space-y-1">
          {req.decisions.map((d) => (
            <li key={d.user.id} className="[overflow-wrap:anywhere]">
              <span aria-hidden>{d.decision === "approve" ? "✅" : "⛔"}</span> <strong className="font-medium">{d.user.display_name}</strong>{" "}
              {tap.decided[d.decision]}
              <span className="text-xs text-zinc-500"> · {formatDateTime(d.created_at)}</span>
              {d.comment && <span className="block whitespace-pre-wrap text-zinc-600 dark:text-zinc-400">“{d.comment}”</span>}
            </li>
          ))}
        </ul>
      )}

      {pending && (
        <>
          <p className="text-xs text-zinc-500">
            {req.approvers.length > 0 ? `${tap.approvers}: ${req.approvers.map((a) => a.display_name).join(", ")}` : tap.noApprovers}
          </p>
          {(req.can_approve || req.can_cancel) && (
            <div className="flex flex-wrap gap-2">
              {req.can_approve && (
                <>
                  <Button
                    variant="primary"
                    disabled={busy}
                    className="pointer-coarse:min-h-11"
                    onClick={() => { actions.approve.mutate({ id: req.id }, { onSuccess: () => { toast("success", tap.approvedToast); }, onError: fail }); }}
                  >
                    {tap.approve}
                  </Button>
                  <Button variant="secondary" disabled={busy} className="text-red-600 pointer-coarse:min-h-11" onClick={() => { setRejecting(true); }}>
                    {tap.reject}
                  </Button>
                </>
              )}
              {req.can_cancel && (
                <Button
                  variant="ghost"
                  disabled={busy}
                  className="pointer-coarse:min-h-11"
                  onClick={() => {
                    if (!window.confirm(tap.confirmCancel)) return;
                    actions.cancel.mutate(req.id, { onSuccess: () => { toast("success", tap.canceledToast); }, onError: fail });
                  }}
                >
                  {tap.cancel}
                </Button>
              )}
            </div>
          )}
          <RejectDialog
            open={rejecting}
            busy={actions.reject.isPending}
            onClose={() => { setRejecting(false); }}
            onSubmit={(reason) => {
              actions.reject.mutate({ id: req.id, reason }, {
                onSuccess: () => { toast("success", tap.rejectedToast); setRejecting(false); },
                onError: fail,
              });
            }}
          />
        </>
      )}
    </div>
  );
}

/** Under an approval that re-approves on change: what an edit would reopen. */
function ReapproveWarning({ ws, req, item }: { ws: string; req: ApprovalRequest; item: Item }) {
  const fields = useFields(ws);
  if (req.state !== "approved" || req.invalidated_at || !req.reapprove_on_change || req.to_status?.id !== item.status.id) return null;
  const names = new Map((fields.data ?? []).map((f) => [`fields.${f.key}`, f.name]));
  const label = (tok: string) => (tok === "title" ? tai.settings.titleField : tok === "description" ? tai.settings.descriptionField : names.get(tok) ?? tok.replace(/^fields\./, ""));
  const watched = req.reapprove_fields ?? [];
  const what = watched.length === 0 ? tai.watchAllShort : watched.map(label).join(", ");
  return <p data-testid="reapprove-warning" className="text-xs text-zinc-500">{tai.reapproveWarn(what)}</p>;
}

export function PolicyExtras({ ws, value, onChange }: { ws: string; value: PolicyExtrasValue; onChange: (v: PolicyExtrasValue) => void }) {
  const fields = useFields(ws);
  const keys = (fields.data ?? []).filter((f) => !f.archived && f.type !== "formula");
  const names = new Map(keys.map((f) => [`fields.${f.key}`, f.name]));
  const tokens = watchTokens(keys.map((f) => f.key));
  const label = (tok: string) => (tok === "title" ? tai.settings.titleField : tok === "description" ? tai.settings.descriptionField : names.get(tok) ?? tok);
  const dueBad = Number.isNaN(parseDue(value.due));
  return (
    <>
      <fieldset className="space-y-2">
        <label className="flex items-start gap-1.5 text-sm pointer-coarse:min-h-11">
          <input
            type="checkbox"
            className="mt-0.5 accent-brand-600"
            checked={value.reapprove_on_change}
            onChange={(e) => { onChange({ ...value, reapprove_on_change: e.target.checked, reapprove_fields: e.target.checked ? value.reapprove_fields : [] }); }}
          />
          <span>
            {tai.settings.reapprove}
            <span className="block text-xs text-zinc-500">{tai.settings.reapproveHint}</span>
          </span>
        </label>
        {value.reapprove_on_change && (
          <div className="pl-5">
            <p className="mb-1 text-xs text-zinc-500">{tai.settings.watched}</p>
            <label className="flex items-center gap-1.5 text-sm pointer-coarse:min-h-11">
              <input type="checkbox" className="accent-brand-600" checked={value.reapprove_fields.length === 0} onChange={(e) => { if (e.target.checked) onChange({ ...value, reapprove_fields: [] }); }} />
              {tai.settings.watchAll}
            </label>
            <div className="flex max-h-40 flex-wrap gap-x-3 gap-y-1 overflow-y-auto">
              {tokens.map((tok) => (
                <label key={tok} className="flex items-center gap-1.5 text-sm pointer-coarse:min-h-11">
                  <input
                    type="checkbox"
                    className="accent-brand-600"
                    checked={value.reapprove_fields.includes(tok)}
                    onChange={(e) => { onChange({ ...value, reapprove_fields: toggleWatched(value.reapprove_fields, tok, e.target.checked) }); }}
                  />
                  {label(tok)}
                </label>
              ))}
            </div>
          </div>
        )}
      </fieldset>
      <label className="block text-xs text-zinc-500">
        {tai.settings.due}
        <Input
          type="number"
          inputMode="numeric"
          min={1}
          max={30}
          value={value.due}
          aria-invalid={dueBad}
          onChange={(e) => { onChange({ ...value, due: e.target.value }); }}
          className="mt-1 w-24"
        />
        <span className={cn("mt-1 block", dueBad && "text-red-600")}>{dueBad ? tai.settings.dueRange : tai.settings.dueHint}</span>
      </label>
    </>
  );
}

function RejectDialog({ open, busy, onClose, onSubmit }: { open: boolean; busy: boolean; onClose: () => void; onSubmit: (reason: string) => void }) {
  const [reason, setReason] = useState("");
  const [touched, setTouched] = useState(false);
  const empty = reason.trim() === "";
  return (
    <Dialog open={open} onClose={onClose} title={tap.rejectTitle}>
      <form
        className="space-y-3"
        onSubmit={(e) => {
          e.preventDefault();
          setTouched(true);
          if (!empty) onSubmit(reason.trim());
        }}
      >
        <label className="block text-xs text-zinc-500">
          {tap.reason}
          <textarea
            required
            value={reason}
            maxLength={2000}
            rows={3}
            aria-invalid={touched && empty}
            onChange={(e) => { setReason(e.target.value); }}
            className={cn(textareaClass, "mt-1")}
          />
        </label>
        {touched && empty && <p role="alert" className="text-xs text-red-600">{tap.reasonRequired}</p>}
        <div className="flex justify-end gap-2">
          <Button type="button" variant="ghost" onClick={onClose}>{t.cancel}</Button>
          <Button type="submit" variant="danger" disabled={busy}>{tap.reject}</Button>
        </div>
      </form>
    </Dialog>
  );
}
