// Four-eyes approvals on status changes (ADR-0026).
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { api, path } from "./client";
import { collabKeys, keys } from "./hooks";
import type { Item, PersonRef, Status } from "./types";

export type ApprovalState = "pending" | "approved" | "rejected" | "canceled";

export interface ApprovalDecision {
  user: PersonRef;
  decision: "approve" | "reject";
  comment: string | null;
  created_at: string;
  /** ADR-0042: the absent approver a delegate decided for; user is who acted. */
  on_behalf_of?: PersonRef | null;
}

/** An approver of a pending request who is away today (ADR-0042). */
export interface ApproverAbsence {
  user: PersonRef;
  /** Who can stand in on this request; null = nobody can. */
  delegate: PersonRef | null;
  ends_on: string;
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
  /** ADR-0042 (pending only): absent approvers the viewer may decide for now. */
  on_behalf_of?: PersonRef[];
  /** ADR-0042 (pending only): approvers away today. */
  absences?: ApproverAbsence[];
  created_at: string;
  closed_at: string | null;
  closed_by: PersonRef | null;
  /** ADR-0034: deadline (workspace-local YYYY-MM-DD), passed (pending only). */
  due_on?: string | null;
  overdue?: boolean;
  /** An approved request whose content changed afterwards. */
  invalidated_at?: string | null;
  /** The invalidated request this one replaces. */
  reopened_from?: string | null;
  /** The snapshot's re-approval rule (ADR-0034); empty fields = all content. */
  reapprove_on_change?: boolean;
  reapprove_fields?: string[];
}

/** One pending request of the workspace inbox (ADR-0034). */
export interface InboxEntry {
  id: string;
  item: { key: string; number: number; project_key: string; title: string };
  requested_by: PersonRef;
  from_status: Status | null;
  to_status: Status | null;
  approvals: number;
  min_approvals: number;
  created_at: string;
  due_on: string | null;
  overdue: boolean;
  can_decide: boolean;
  /** ADR-0042: the absent approvers the viewer may decide for ("X adına"). */
  on_behalf_of?: PersonRef[];
}

export interface InboxPage {
  approvals: InboxEntry[];
  next_cursor: string | null;
}

export interface InboxCount {
  count: number;
  overdue: number;
  cap: number;
}

interface ApprovalResult {
  approvals: ApprovalRequest[];
  item: Item;
}

export const approvalKeys = {
  list: (ws: string, project: string, n: number) => ["ws", ws, "p", project, "item", n, "approvals"] as const,
  inbox: (ws: string) => ["ws", ws, "approval-inbox"] as const,
};

/** The workspace inbox, oldest first; mine = only what the viewer may decide on. */
export const useApprovalInbox = (ws: string, mine: boolean) =>
  useInfiniteQuery({
    queryKey: [...approvalKeys.inbox(ws), "list", mine],
    initialPageParam: null as string | null,
    queryFn: ({ pageParam }) => {
      const q = new URLSearchParams({ state: "pending", mine: String(mine), limit: "50" });
      if (pageParam) q.set("cursor", pageParam);
      return api<InboxPage>("GET", path`/workspaces/${ws}/approvals` + "?" + q.toString());
    },
    getNextPageParam: (last) => last.next_cursor,
  });

/** The badge: requests waiting for the viewer's decision (capped server-side). */
export const useApprovalInboxCount = (ws: string) =>
  useQuery({
    queryKey: [...approvalKeys.inbox(ws), "count"],
    queryFn: () => api<InboxCount>("GET", path`/workspaces/${ws}/approvals/count` + "?mine=true"),
    refetchInterval: 120_000,
    staleTime: 60_000,
  });

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
    void qc.invalidateQueries({ queryKey: approvalKeys.inbox(ws) });
  };
  return {
    request: useMutation({
      mutationFn: (b: { version: number; to_status_id: string; comment?: string }) => api<ApprovalResult>("POST", base, b),
      onSuccess,
    }),
    approve: useMutation({
      mutationFn: (v: { id: string; comment?: string; onBehalfOf?: string }) =>
        api<ApprovalResult>("POST", base + path`/${v.id}/approve`, { comment: v.comment, on_behalf_of: v.onBehalfOf }),
      onSuccess,
    }),
    reject: useMutation({
      mutationFn: (v: { id: string; reason: string; onBehalfOf?: string }) =>
        api<ApprovalResult>("POST", base + path`/${v.id}/reject`, { reason: v.reason, on_behalf_of: v.onBehalfOf }),
      onSuccess,
    }),
    cancel: useMutation({
      mutationFn: (id: string) => api<ApprovalResult>("POST", base + path`/${id}/cancel`),
      onSuccess,
    }),
  };
}
