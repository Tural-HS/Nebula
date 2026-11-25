// Absences and approval delegation (ADR-0042).
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { approvalKeys } from "./approvals";
import { api, path } from "./client";
import type { PersonRef } from "./types";

export interface Absence {
  id: string;
  user: PersonRef;
  delegate: PersonRef | null;
  /** Workspace-local YYYY-MM-DD, inclusive. */
  starts_on: string;
  ends_on: string;
  /** Only for the absent member, the delegate and admins. */
  note: string | null;
  /** Today (workspace time zone) is within the absence. */
  active: boolean;
  /** The delegate is away on some of these days: nobody stands in then. */
  delegate_absent: boolean;
  created_by: PersonRef | null;
  created_at: string;
  updated_at: string;
  ended_at: string | null;
  can_edit: boolean;
}

export interface AbsenceInput {
  user_id?: string;
  delegate_id: string | null;
  starts_on: string;
  ends_on: string;
  note: string | null;
}

export const absenceKeys = {
  list: (ws: string) => ["ws", ws, "absences"] as const,
};

/** Current and upcoming absences of the workspace (member role and up). */
export const useAbsences = (ws: string, enabled = true) =>
  useQuery({
    queryKey: absenceKeys.list(ws),
    queryFn: async () => (await api<{ absences: Absence[] }>("GET", path`/workspaces/${ws}/absences`)).absences,
    enabled: enabled && ws !== "",
    staleTime: 60_000,
  });

/** Create, change and end; each refreshes the list and the approval views that show absences. */
export function useAbsenceActions(ws: string) {
  const qc = useQueryClient();
  const onSuccess = () => {
    void qc.invalidateQueries({ queryKey: absenceKeys.list(ws) });
    void qc.invalidateQueries({ queryKey: approvalKeys.inbox(ws) });
    // Approval panels list absent approvers and the viewer's delegated rights.
    void qc.invalidateQueries({ predicate: (q) => q.queryKey[0] === "ws" && q.queryKey[1] === ws && q.queryKey.at(-1) === "approvals" });
  };
  return {
    create: useMutation({
      mutationFn: (b: AbsenceInput) => api<Absence>("POST", path`/workspaces/${ws}/absences`, b),
      onSuccess,
    }),
    update: useMutation({
      mutationFn: ({ id, ...b }: Omit<AbsenceInput, "user_id"> & { id: string }) => api<Absence>("PATCH", path`/workspaces/${ws}/absences/${id}`, b),
      onSuccess,
    }),
    end: useMutation({
      mutationFn: (id: string) => api<Absence>("POST", path`/workspaces/${ws}/absences/${id}/end`),
      onSuccess,
    }),
  };
}
