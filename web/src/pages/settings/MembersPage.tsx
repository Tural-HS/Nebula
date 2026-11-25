import { useState } from "react";

import { useAbsences } from "../../api/absences";
import { useAdminApi, useInvitations } from "../../api/admin";
import { useMe, useMembers, useWorkspace } from "../../api/hooks";
import { roleAtLeast, type Role } from "../../api/types";
import { AbsenceDialog, AbsenceList } from "../../components/Absence";
import { Avatar, Button, ErrorState, Input, Select, Spinner } from "../../components/ui";
import { tab } from "../../i18n-absences";
import { ta } from "../../i18n-admin";
import { absenceOf, canBeAbsent } from "../../lib/absences";
import { formatDate, formatDateTime } from "../../lib/format";
import { Card, SettingsLayout } from "./SettingsLayout";
import { useFeedback } from "./useErrorToast";

const ROLES: Role[] = ["guest", "member", "admin", "owner"];

export function MembersPage() {
  return <SettingsLayout title={ta.nav.members} adminOnly={false}>{(ws) => <Members ws={ws} />}</SettingsLayout>;
}

function Members({ ws }: { ws: string }) {
  const members = useMembers(ws);
  const me = useMe();
  const workspace = useWorkspace(ws);
  const adm = useAdminApi(ws);
  const fb = useFeedback();
  const myRole = workspace.data?.role;
  const isAdmin = roleAtLeast(myRole, "admin");
  const isOwner = myRole === "owner";
  const [email, setEmail] = useState("");
  const [role, setRole] = useState<Role>("member");
  const invitations = useInvitations(ws, isAdmin);
  // ADR-0042: who is away (members and up see it), admins manage it.
  const absences = useAbsences(ws, myRole !== undefined && myRole !== "guest");
  const [recording, setRecording] = useState(false);

  if (members.isPending) return <Spinner />;
  if (members.isError) return <ErrorState error={members.error} />;

  return (
    <div className="space-y-4">
      <p className="text-xs text-zinc-500 dark:text-zinc-400">{ta.roleHelp}</p>
      {isAdmin && (
        <Card>
          <form
            className="flex flex-wrap items-end gap-2"
            onSubmit={(e) => {
              e.preventDefault();
              adm.invite.mutate({ email: email.trim(), role }, { onSuccess: () => { setEmail(""); fb.ok(ta.invited)(); }, onError: fb.fail });
            }}
          >
            <div className="min-w-56 flex-1">
              <label htmlFor="m-email" className="mb-1 block text-xs text-zinc-500 dark:text-zinc-400">{ta.email}</label>
              <Input id="m-email" type="email" required value={email} onChange={(e) => { setEmail(e.target.value); }} />
            </div>
            <Select aria-label="role" value={role} onChange={(e) => { setRole(e.target.value as Role); }} className="w-36">
              {ROLES.filter((r) => isOwner || r !== "owner").map((r) => <option key={r} value={r}>{ta.roles[r]}</option>)}
            </Select>
            <Button type="submit" variant="primary" disabled={adm.invite.isPending}>{ta.invite}</Button>
            <Button
              variant="ghost"
              disabled={adm.addMember.isPending || !email.trim()}
              onClick={() => { adm.addMember.mutate({ email: email.trim(), role }, { onSuccess: () => { setEmail(""); fb.ok()(); }, onError: fb.fail }); }}
            >
              {ta.addDirectly}
            </Button>
          </form>
          <p className="mt-2 text-xs text-zinc-500 dark:text-zinc-400">{ta.memberHint}</p>
        </Card>
      )}
      {isAdmin && invitations.data && invitations.data.length > 0 && (
        <Card label={ta.pendingInvites}>
          <h2 className="mb-2 text-sm font-semibold">{ta.pendingInvites}</h2>
          <ul className="divide-y divide-zinc-100 dark:divide-zinc-800">
            {invitations.data.map((inv) => (
              <li key={inv.id} className="flex flex-wrap items-center gap-3 py-2 text-sm">
                <span className="min-w-0 flex-1 truncate">{inv.email}</span>
                <span className="text-xs text-zinc-500 dark:text-zinc-400">
                  {ta.roles[inv.role]} · {ta.expires} {formatDateTime(inv.expires_at)}
                </span>
                <Button variant="ghost" onClick={() => { adm.resendInvite.mutate(inv.id, { onSuccess: fb.ok(ta.resent), onError: fb.fail }); }}>
                  {ta.resend}
                </Button>
                <Button
                  variant="ghost"
                  className="text-red-600"
                  onClick={() => { if (window.confirm(ta.confirmRevoke)) adm.revokeInvite.mutate(inv.id, { onSuccess: fb.ok(), onError: fb.fail }); }}
                >
                  {ta.revoke}
                </Button>
              </li>
            ))}
          </ul>
        </Card>
      )}
      {isAdmin && (
        <Card label={tab.admin.title}>
          <div className="flex flex-wrap items-start justify-between gap-2">
            <div className="min-w-0 flex-1">
              <h2 className="text-sm font-semibold">{tab.admin.title}</h2>
              <p className="text-xs text-zinc-500 dark:text-zinc-400">{tab.admin.hint}</p>
            </div>
            <Button onClick={() => { setRecording(true); }}>{tab.admin.set}</Button>
          </div>
          {absences.isError && <ErrorState error={absences.error} onRetry={() => void absences.refetch()} />}
          {absences.data && absences.data.length > 0 && <AbsenceList ws={ws} list={absences.data} showUser />}
          <AbsenceDialog ws={ws} open={recording} onClose={() => { setRecording(false); }} members={members.data.filter(canBeAbsent)} />
        </Card>
      )}
      <Card className="p-0">
        <ul className="divide-y divide-zinc-200 dark:divide-zinc-800">
          {members.data.map((m) => {
            const self = m.user_id === me.data?.id;
            const canChange = isAdmin && (isOwner || m.role !== "owner");
            return (
              <li key={m.user_id} className="flex flex-wrap items-center gap-3 px-4 py-3">
                <Avatar name={m.display_name} size="md" />
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-medium">{m.display_name}{self && <span className="text-zinc-500 dark:text-zinc-400"> ({ta.you})</span>}</p>
                  <p className="truncate text-xs text-zinc-500 dark:text-zinc-400">{m.email}</p>
                  <AwayLine absence={absenceOf(absences.data, m.user_id)} />
                </div>
                {canChange ? (
                  <Select
                    aria-label={`${m.display_name} role`}
                    value={m.role}
                    className="w-32"
                    onChange={(e) => { adm.setRole.mutate({ id: m.user_id, role: e.target.value as Role }, { onSuccess: fb.ok(), onError: fb.fail }); }}
                  >
                    {ROLES.filter((r) => isOwner || r !== "owner").map((r) => <option key={r} value={r}>{ta.roles[r]}</option>)}
                  </Select>
                ) : (
                  <span className="rounded-full bg-zinc-100 px-2 py-0.5 text-xs dark:bg-zinc-800">{ta.roles[m.role]}</span>
                )}
                {(canChange || self) && (
                  <Button
                    variant="ghost"
                    className="text-red-600"
                    onClick={() => {
                      if (window.confirm(ta.confirmRemoveMember(m.display_name))) adm.removeMember.mutate(m.user_id, { onSuccess: fb.ok(), onError: fb.fail });
                    }}
                  >
                    {self ? ta.leave : ta.remove}
                  </Button>
                )}
              </li>
            );
          })}
        </ul>
      </Card>
    </div>
  );
}

/** "Away until 28 Nov" / "Away from 1 Dec" under a member's name (ADR-0042). */
function AwayLine({ absence }: { absence: ReturnType<typeof absenceOf> }) {
  if (!absence) return null;
  return (
    <p data-testid="member-away" className="text-xs text-sky-800 dark:text-sky-200 [overflow-wrap:anywhere]">
      <span aria-hidden>🌴 </span>
      {absence.active ? tab.admin.awayUntil(formatDate(absence.ends_on)) : tab.admin.awayFrom(formatDate(absence.starts_on))}
      {absence.delegate && <> · {tab.panelDelegate(absence.delegate.display_name)}</>}
    </p>
  );
}
