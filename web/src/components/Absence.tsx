// Absences and approval delegation UI (ADR-0042): the banner under the
// header, the absence form and list (own page and admin members settings).
// The server decides who may do what; can_edit only follows its answer.
import { useId, useState } from "react";
import { Link, useParams } from "react-router";

import { useAbsenceActions, useAbsences, type Absence, type AbsenceInput } from "../api/absences";
import { ApiError } from "../api/client";
import { useMe, useMembers, useWorkspace } from "../api/hooks";
import type { Member } from "../api/types";
import { t } from "../i18n";
import { tab } from "../i18n-absences";
import { absenceOf, absenceProblem, delegateCandidates, joinNames, MAX_ABSENCE_NOTE, standingInFor } from "../lib/absences";
import { todayIn } from "../lib/calendar";
import { cn } from "../lib/cn";
import { formatDate } from "../lib/format";
import { Button, Dialog, Input, Select, touchField } from "./ui";
import { useToast } from "./toast-context";

/** Human message for a failed absence call: known codes, else the server's text. */
function absenceErrorText(err: unknown): string {
  if (err instanceof ApiError) return tab.errors[err.code] ?? err.message;
  return t.errorGeneric;
}

/**
 * Under the header in a workspace: "you are away until …" for the absent
 * member, "today you can decide for …" for a delegate. Guests have none.
 */
export function AbsenceBanner() {
  const { ws = "" } = useParams();
  const workspace = useWorkspace(ws);
  const me = useMe();
  const member = Boolean(ws) && workspace.data !== undefined && workspace.data.role !== "guest" && !workspace.data.archived_at;
  const list = useAbsences(ws, member);
  if (!member || !list.data) return null;
  const mine = absenceOf(list.data, me.data?.id);
  const standIn = standingInFor(list.data, me.data?.id);
  const away = mine?.active ? mine : undefined;
  if (!away && standIn.length === 0) return null;
  const base = `/w/${encodeURIComponent(ws)}`;
  return (
    <div role="status" data-testid="absence-banner" className="flex flex-wrap items-center gap-x-3 gap-y-1 border-b border-sky-200 bg-sky-50 px-4 py-2 text-sm text-sky-900 dark:border-sky-900 dark:bg-sky-950 dark:text-sky-100">
      <span aria-hidden>🌴</span>
      <span className="min-w-0 flex-1 [overflow-wrap:anywhere]">
        {away && (
          <>
            {tab.banner.self(formatDate(away.ends_on))}{" "}
            {away.delegate ? tab.banner.selfDelegate(away.delegate.display_name) : tab.banner.selfNoDelegate}
          </>
        )}
        {standIn.length > 0 && <>{away ? " " : ""}{tab.banner.standIn(joinNames(standIn.map((a) => a.user)))}</>}
      </span>
      {standIn.length > 0 && <Link to={`${base}/approvals`} className="font-medium underline pointer-coarse:py-2">{tab.banner.inbox}</Link>}
      {away && <Link to={`${base}/absence`} className="font-medium underline pointer-coarse:py-2">{tab.banner.manage}</Link>}
    </div>
  );
}

const textareaClass = cn(
  touchField,
  "w-full rounded-md bg-white px-2 py-1.5 text-sm ring-1 ring-zinc-500 focus:ring-2 focus:ring-brand-500 focus:outline-none dark:bg-zinc-900 dark:ring-zinc-600",
);

/**
 * Create (absence undefined) or change an absence. With members (admins),
 * the absent member is picked in the form; otherwise forUser (default: me).
 */
export function AbsenceDialog({ ws, open, onClose, absence, forUser, members }: {
  ws: string; open: boolean; onClose: () => void; absence?: Absence; forUser?: Member;
  /** Admin form: pick any member. */
  members?: Member[];
}) {
  return (
    <Dialog open={open} onClose={onClose} title={absence ? tab.editTitle : forUser ? tab.addFor(forUser.display_name) : tab.addTitle}>
      {/* Remounted per open, so the fields start from the absence each time. */}
      {open && <AbsenceForm ws={ws} absence={absence} forUser={forUser} members={members} onDone={onClose} />}
    </Dialog>
  );
}

function AbsenceForm({ ws, absence, forUser, members, onDone }: { ws: string; absence?: Absence; forUser?: Member; members?: Member[]; onDone: () => void }) {
  const workspace = useWorkspace(ws);
  const all = useMembers(ws);
  const me = useMe();
  const actions = useAbsenceActions(ws);
  const toast = useToast();
  const today = todayIn(workspace.data?.timezone);
  const [userId, setUserId] = useState(absence?.user.id ?? forUser?.user_id ?? "");
  const [from, setFrom] = useState(absence?.starts_on ?? today);
  const [to, setTo] = useState(absence?.ends_on ?? today);
  const [delegate, setDelegate] = useState(absence?.delegate?.id ?? "");
  const [note, setNote] = useState(absence?.note ?? "");
  const [touched, setTouched] = useState(false);
  const id = useId();
  const absent = userId || me.data?.id || "";
  const startChanged = !absence || from !== absence.starts_on;
  const begun = Boolean(absence && absence.starts_on <= today);
  const value: AbsenceInput = { delegate_id: delegate || null, starts_on: from, ends_on: to, note: note.trim() || null };
  const problem = absenceProblem({ ...value, user_id: absent }, today, startChanged) ?? (members && !userId ? "invalid" : null);
  const candidates = delegateCandidates(all.data ?? [], absent);
  const busy = actions.create.isPending || actions.update.isPending;
  const fail = (err: unknown) => { toast("error", absenceErrorText(err)); };
  const done = (a: Absence) => {
    toast("success", a.delegate_absent ? `${tab.saved}. ${tab.delegateAbsent}` : tab.saved);
    onDone();
  };
  return (
    <form
      noValidate
      className="space-y-3"
      onSubmit={(e) => {
        e.preventDefault();
        setTouched(true);
        if (problem) return;
        if (absence) actions.update.mutate({ id: absence.id, ...value }, { onSuccess: done, onError: fail });
        else actions.create.mutate({ ...value, user_id: userId || undefined }, { onSuccess: done, onError: fail });
      }}
    >
      {members && !absence && (
        <label className="block text-xs text-zinc-500 dark:text-zinc-400">
          {tab.admin.member}
          <Select required value={userId} onChange={(e) => { setUserId(e.target.value); }} className="mt-1">
            <option value="" disabled>—</option>
            {members.map((m) => <option key={m.user_id} value={m.user_id}>{m.display_name}</option>)}
          </Select>
        </label>
      )}
      <div className="grid grid-cols-1 gap-3 min-[360px]:grid-cols-2">
        <label className="block text-xs text-zinc-500 dark:text-zinc-400">
          {tab.from}
          <Input type="date" required value={from} min={begun ? undefined : today} disabled={begun} onChange={(e) => { setFrom(e.target.value); }} className="mt-1" />
        </label>
        <label className="block text-xs text-zinc-500 dark:text-zinc-400">
          {tab.to}
          <Input type="date" required value={to} min={today} onChange={(e) => { setTo(e.target.value); }} className="mt-1" />
        </label>
      </div>
      <p className="text-xs text-zinc-500 dark:text-zinc-400">{tab.timezone(workspace.data?.timezone ?? "UTC")}</p>
      <div className="text-xs text-zinc-500 dark:text-zinc-400">
        <label htmlFor={`${id}-delegate`}>{tab.delegate}</label>
        <Select id={`${id}-delegate`} aria-describedby={`${id}-delegate-hint`} value={delegate} onChange={(e) => { setDelegate(e.target.value); }} className="mt-1">
          <option value="">{tab.noDelegate}</option>
          {candidates.map((m) => <option key={m.user_id} value={m.user_id}>{m.display_name}</option>)}
        </Select>
        <p id={`${id}-delegate-hint`} className="mt-1">{tab.delegateHint}</p>
      </div>
      <div className="text-xs text-zinc-500 dark:text-zinc-400">
        <label htmlFor={`${id}-note`}>{tab.note}</label>
        <textarea id={`${id}-note`} aria-describedby={`${id}-note-hint`} value={note} maxLength={MAX_ABSENCE_NOTE} rows={2}
          onChange={(e) => { setNote(e.target.value); }} className={cn(textareaClass, "mt-1")} />
        <p id={`${id}-note-hint`} className="mt-1">{tab.noteHint}</p>
      </div>
      {touched && problem && <p role="alert" className="text-sm text-red-600 dark:text-red-400">{tab.problems[problem]}</p>}
      <div className="flex justify-end gap-2">
        <Button type="button" variant="ghost" onClick={onDone}>{t.cancel}</Button>
        <Button type="submit" variant="primary" disabled={busy}>{tab.save}</Button>
      </div>
    </form>
  );
}

/** Absences as a list with Change / End; showUser for the admin view. */
export function AbsenceList({ ws, list, showUser = false }: { ws: string; list: Absence[]; showUser?: boolean }) {
  const actions = useAbsenceActions(ws);
  const toast = useToast();
  const [editing, setEditing] = useState<Absence | null>(null);
  const [ending, setEnding] = useState<Absence | null>(null);
  return (
    <>
      <ul data-testid="absence-list" className="divide-y divide-zinc-100 dark:divide-zinc-800">
        {list.map((a) => (
          <li key={a.id} className="flex flex-wrap items-center gap-x-3 gap-y-2 py-3">
            <div className="min-w-0 flex-1 space-y-0.5 text-sm">
              <p className="flex flex-wrap items-center gap-2">
                {showUser && <strong className="font-medium [overflow-wrap:anywhere]">{a.user.display_name}</strong>}
                <span className="tabular-nums">{tab.period(formatDate(a.starts_on), formatDate(a.ends_on))}</span>
                <span className={cn("rounded px-1.5 py-0.5 text-2xs font-medium", a.active ? "bg-sky-100 text-sky-800 dark:bg-sky-900 dark:text-sky-100" : "bg-zinc-100 text-zinc-700 dark:bg-zinc-800 dark:text-zinc-300")}>
                  {a.active ? tab.active : tab.upcoming}
                </span>
              </p>
              <p className="text-xs text-zinc-500 dark:text-zinc-400 [overflow-wrap:anywhere]">
                {tab.delegate}: {a.delegate?.display_name ?? tab.noDelegate}
                {a.created_by && a.created_by.id !== a.user.id && <> · {tab.by(a.created_by.display_name)}</>}
              </p>
              {a.delegate_absent && <p className="text-xs text-amber-800 dark:text-amber-300">⚠️ {tab.delegateAbsent}</p>}
              {a.note && <p className="text-xs whitespace-pre-wrap text-zinc-600 dark:text-zinc-400 [overflow-wrap:anywhere]">{a.note}</p>}
            </div>
            {a.can_edit && (
              <div className="flex gap-2">
                <Button onClick={() => { setEditing(a); }}>{tab.edit}</Button>
                <Button variant="dangerGhost" onClick={() => { setEnding(a); }}>{a.active ? tab.end : tab.cancelUpcoming}</Button>
              </div>
            )}
          </li>
        ))}
      </ul>
      <AbsenceDialog ws={ws} open={editing !== null} absence={editing ?? undefined} onClose={() => { setEditing(null); }} />
      <Dialog
        open={ending !== null}
        onClose={() => { setEnding(null); }}
        title={ending ? (ending.active ? tab.endTitle(ending.user.display_name) : tab.cancelTitle(ending.user.display_name)) : ""}
      >
        <p className="text-sm text-zinc-600 dark:text-zinc-400">{tab.endBody}</p>
        <div className="mt-4 flex justify-end gap-2">
          <Button variant="ghost" onClick={() => { setEnding(null); }}>{t.cancel}</Button>
          <Button
            variant="danger"
            disabled={actions.end.isPending}
            onClick={() => {
              if (!ending) return;
              actions.end.mutate(ending.id, {
                onSuccess: () => { toast("success", tab.ended); setEnding(null); },
                onError: (err) => { toast("error", absenceErrorText(err)); },
              });
            }}
          >
            {ending?.active === false ? tab.cancelUpcoming : tab.end}
          </Button>
        </div>
      </Dialog>
    </>
  );
}
