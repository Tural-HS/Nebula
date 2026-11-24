// English strings for the in-app notification center (ADR-0017).
import type { Messages } from "../../i18n/locale";

export const tn = {
  title: "Notifications",
  empty: "No notifications yet.",
  markAllRead: "Mark all as read",
  someone: "A user",
  bellLabel: (count: number, capped: boolean) =>
    count === 0 ? "Notifications" : capped || count > 99 ? "Notifications, more than 99 unread" : `Notifications, ${String(count)} unread`,
  liveCount: (count: number, capped: boolean) =>
    count === 0
      ? "No unread notifications"
      : capped || count > 99
        ? "More than 99 unread notifications"
        : `${String(count)} unread ${count === 1 ? "notification" : "notifications"}`,
  mention: (actor: string) => `${actor} mentioned you`,
  assignedBy: (actor: string) => `Assigned to you by ${actor}`,
  assigned: "Assigned to you",
  overdue: "Due date passed",
  comment: (actor: string) => `${actor} commented`,
  statusChanged: (actor: string) => `${actor} changed the status`,
  assigneeChanged: (actor: string) => `${actor} changed the assignee`,
  dueChanged: (actor: string) => `${actor} changed the due date`,
  linkAdded: (actor: string) => `${actor} added a link`,
  unread: "unread",
} satisfies Messages["notifications"];
