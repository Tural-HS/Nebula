import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate, useParams } from "react-router";

import { api, ApiError } from "../api/client";
import { keys } from "../api/hooks";
import type { Role } from "../api/types";
import { useAuth } from "../auth/context";
import { Layout } from "../components/Layout";
import { Button, Spinner } from "../components/ui";
import { t } from "../i18n";
import { ta } from "../i18n-admin";
import { ti } from "../i18n-invite";

interface Preview {
  workspace_slug: string;
  workspace_name: string;
  role: Role;
  invited_by: string;
  email: string; // the invited address, masked
  email_matches: boolean;
  already_member: boolean;
}

/** Shows an invitation to the signed-in user and lets them accept it. The
 *  token goes in a POST body, never in an API URL (ADR-0016). */
export function InvitePage() {
  const { token = "" } = useParams();
  const qc = useQueryClient();
  const navigate = useNavigate();
  const { logout } = useAuth();
  const preview = useQuery({
    queryKey: ["invitation", token],
    queryFn: () => api<Preview>("POST", "/invitations/preview", { token }),
    retry: false,
  });
  const accept = useMutation({
    mutationFn: () => api<{ workspace: string }>("POST", "/invitations/accept", { token }),
    onSuccess: async ({ workspace }) => {
      await qc.invalidateQueries({ queryKey: keys.workspaces });
      void navigate(`/w/${encodeURIComponent(workspace)}`, { replace: true });
    },
  });

  let body;
  if (preview.isPending) body = <Spinner />;
  else if (preview.isError) body = <p role="alert" className="text-sm text-red-700 dark:text-red-300">{explain(preview.error)}</p>;
  else {
    const p = preview.data;
    if (!p.email_matches) {
      body = (
        <div className="space-y-3">
          <p role="alert" className="text-sm">{ti.mismatch(p.email)}</p>
          <Button onClick={() => void logout()}>{ti.switchAccount}</Button>
        </div>
      );
    } else if (p.already_member) {
      body = (
        <div className="space-y-3">
          <p className="text-sm">{ti.alreadyMember}</p>
          <Button variant="primary" onClick={() => void navigate(`/w/${encodeURIComponent(p.workspace_slug)}`)}>{ti.open}</Button>
        </div>
      );
    } else {
      body = (
        <div className="space-y-4">
          <p className="text-sm">{ti.invitedBy(p.invited_by || "—", p.workspace_name, ta.roles[p.role].toLowerCase())}</p>
          {accept.isError && <p role="alert" className="text-sm text-red-700 dark:text-red-300">{explain(accept.error)}</p>}
          <Button variant="primary" disabled={accept.isPending} onClick={() => { accept.mutate(); }}>{ti.accept}</Button>
        </div>
      );
    }
  }

  return (
    <Layout crumbs={[{ label: ti.title }]}>
      <div className="mx-auto max-w-lg p-6">
        <div className="rounded-xl bg-white p-6 ring-1 ring-zinc-200 dark:bg-zinc-900 dark:ring-zinc-800">
          <h1 className="mb-4 text-lg font-semibold">{ti.title}</h1>
          {body}
        </div>
      </div>
    </Layout>
  );
}

function explain(err: unknown): string {
  if (err instanceof ApiError) {
    if (err.status === 404) return ti.invalid;
    if (err.code === "invitation_used") return ti.used;
    if (err.code === "invitation_expired") return ti.expired;
    if (err.code === "invitation_email_mismatch") return err.message.includes("verified") ? ti.unverified : err.message;
  }
  return err instanceof ApiError ? err.message : t.errorGeneric;
}
