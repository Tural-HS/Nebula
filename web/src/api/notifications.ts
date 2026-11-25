import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { api, path } from "./client";

export type NotificationKind =
  | "mention"
  | "assigned"
  | "overdue"
  | "comment"
  // Approvals (ADR-0026)
  | "approval_requested"
  | "approval_applied"
  | "approval_rejected"
  | "approval_canceled"
  // Watchers (ADR-0028)
  | "status_changed"
  | "assignee_changed"
  | "due_changed"
  | "link_added"
  // Approval deadlines and re-approval (ADR-0034)
  | "approval_overdue"
  | "approval_invalidated"
  // SLA targets (ADR-0045)
  | "sla_at_risk"
  | "sla_breached";

/** One entry of GET /me/notifications; item key and title are current. */
export interface AppNotification {
  id: number;
  kind: NotificationKind;
  read: boolean;
  created_at: string;
  workspace_slug: string;
  workspace_name: string;
  project_key: string;
  item_number: number;
  item_key: string;
  item_title: string;
  actor_id: string | null;
  actor_name: string | null;
}

export interface NotificationPage {
  notifications: AppNotification[];
  next_cursor: string | null;
}

export interface UnreadCount {
  unread: number;
  capped: boolean;
}

export const notificationKeys = {
  all: ["notifications"] as const,
  count: ["notifications", "count"] as const,
  list: ["notifications", "list"] as const,
};

/** Polling interval of the unread badge (no push channel yet, ADR-0017). */
export const COUNT_POLL_MS = 30_000;

export const useUnreadCount = () =>
  useQuery({
    queryKey: notificationKeys.count,
    queryFn: () => api<UnreadCount>("GET", "/me/notifications/unread-count"),
    refetchInterval: COUNT_POLL_MS, // paused while the tab is hidden; refetched on focus
    refetchOnWindowFocus: true,
    staleTime: 10_000,
  });

export const useNotifications = (enabled: boolean) =>
  useQuery({
    enabled,
    queryKey: notificationKeys.list,
    queryFn: () => api<NotificationPage>("GET", "/me/notifications?limit=20"),
    staleTime: 0,
  });

export function useMarkRead() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => api<undefined>("POST", path`/me/notifications/${id}/read`),
    onSuccess: () => qc.invalidateQueries({ queryKey: notificationKeys.all }),
  });
}

export function useMarkAllRead() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => api<{ marked: number }>("POST", "/me/notifications/read-all"),
    onSuccess: () => qc.invalidateQueries({ queryKey: notificationKeys.all }),
  });
}
