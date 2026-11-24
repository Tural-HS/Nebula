// Four-eyes approvals on status changes (ADR-0026).
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { api, path } from "./client";
import { collabKeys, keys } from "./hooks";
import type { Item, PersonRef, Status } from "./types";

export type ApprovalState = "pending" | "approved" | "rejected" | "canceled";

export interface ApprovalDecision {
  user: PersonRef;
  decision: "approve" | "reject";
  comment: string | null;
  created_at: string;
}

export interface ApprovalRequest {
  id: string;
  state: ApprovalState;
  from_status: Status | null;
  to_status: Status | null;
  requested_by: PersonRef | null;
  comment: string | null;
  min_approvals: number;
  approvals: number;
  approver_roles: string[];
  decisions: ApprovalDecision[];
  /** Pending only: people who may still decide (never the requester). */
  approvers: PersonRef[];
  can_approve: boolean;
  can_cancel: boolean;
  created_at: string;
  closed_at: string | null;
  closed_by: PersonRef | null;
}

interface ApprovalResult {
  approvals: ApprovalRequest[];
  item: Item;
}

export const approvalKeys = {
  list: (ws: string, project: string, n: number) => ["ws", ws, "p", project, "item", n, "approvals"] as const,
};

export const useApprovals = (ws: string, project: string, n: number) =>
  useQuery({
    queryKey: approvalKeys.list(ws, project, n),
    queryFn: async () => (await api<{ approvals: ApprovalRequest[] }>("GET", path`/workspaces/${ws}/projects/${project}/items/${n}/approvals`)).approvals,
  });

/** Request, approve, reject and cancel; each refreshes the item, lists and history. */
export function useApprovalActions(ws: string, project: string, n: number) {
  const qc = useQueryClient();
  const base = path`/workspaces/${ws}/projects/${project}/items/${n}/approvals`;
  const onSuccess = (res: ApprovalResult) => {
    qc.setQueryData(approvalKeys.list(ws, project, n), res.approvals);
    qc.setQueryData(keys.item(ws, project, n), res.item);
    void qc.invalidateQueries({ queryKey: keys.items(ws, project) });
    void qc.invalidateQueries({ queryKey: keys.transitions(ws, project, n) });
    void qc.invalidateQueries({ queryKey: collabKeys.activity(ws, project, n) });
  };
  return {
    request: useMutation({
      mutationFn: (b: { version: number; to_status_id: string; comment?: string }) => api<ApprovalResult>("POST", base, b),
      onSuccess,
    }),
    approve: useMutation({
      mutationFn: (v: { id: string; comment?: string }) => api<ApprovalResult>("POST", base + path`/${v.id}/approve`, { comment: v.comment }),
      onSuccess,
    }),
    reject: useMutation({
      mutationFn: (v: { id: string; reason: string }) => api<ApprovalResult>("POST", base + path`/${v.id}/reject`, { reason: v.reason }),
      onSuccess,
    }),
    cancel: useMutation({
      mutationFn: (id: string) => api<ApprovalResult>("POST", base + path`/${id}/cancel`),
      onSuccess,
    }),
  };
}
