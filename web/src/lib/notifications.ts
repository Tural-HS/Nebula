import type { AppNotification } from "../api/notifications";
import { tn } from "../i18n-notifications";

/** The sentence before the item reference, e.g. "Alice sizi qeyd etdi". */
export function notificationLead(n: Pick<AppNotification, "kind" | "actor_name">): string {
  const actor = n.actor_name ?? tn.someone;
  switch (n.kind) {
    case "mention":
      return tn.mention(actor);
    case "assigned":
      return n.actor_name ? tn.assignedBy(n.actor_name) : tn.assigned;
    case "overdue":
      return tn.overdue;
    case "comment":
      return tn.comment(actor);
    case "status_changed":
      return tn.statusChanged(actor);
    case "assignee_changed":
      return tn.assigneeChanged(actor);
    case "due_changed":
      return tn.dueChanged(actor);
    case "link_added":
      return tn.linkAdded(actor);
  }
}

/** Full plain-text message: "Alice sizi qeyd etdi: QA-12 Giriş xətası". */
export function notificationText(n: Pick<AppNotification, "kind" | "actor_name" | "item_key" | "item_title">): string {
  return `${notificationLead(n)}: ${n.item_key} ${n.item_title}`;
}

/** Deep link that opens the item drawer on the project page. */
export function notificationHref(n: Pick<AppNotification, "workspace_slug" | "project_key" | "item_number">): string {
  return `/w/${encodeURIComponent(n.workspace_slug)}/p/${encodeURIComponent(n.project_key)}?item=${String(n.item_number)}`;
}

/** Badge text: nothing at zero, "99+" above 99 (or when the server capped the count). */
export function badgeText(count: number, capped = false): string | null {
  if (count <= 0) return null;
  return capped || count > 99 ? "99+" : String(count);
}
