export type WebhookFormat = "nebula" | "slack";

export const WEBHOOK_FORMATS: readonly WebhookFormat[] = ["nebula", "slack"];

/** Events a chat channel usually wants; the Nebula default stays "item.*". */
export function defaultEvents(format: WebhookFormat): string {
  return format === "slack" ? "item.*, comment.created, approval.*" : "item.*";
}

/** The pattern the "send approval events" toggle adds (ADR-0031). */
export const APPROVAL_EVENTS = "approval.*";

const isApprovalPattern = (p: string) => p === APPROVAL_EVENTS || p.startsWith("approval.");

/** Whether the subscription delivers at least one approval event. */
export function sendsApprovals(events: readonly string[]): boolean {
  return events.some((p) => p === "*" || isApprovalPattern(p));
}

/** "*" delivers approvals whatever the toggle says. */
export const approvalsByWildcard = (events: readonly string[]) => events.includes("*");

/** Turns approval events on (adds "approval.*") or off (drops every approval pattern). */
export function withApprovals(events: readonly string[], on: boolean): string[] {
  if (on) return sendsApprovals(events) ? [...events] : [...events, APPROVAL_EVENTS];
  return events.filter((p) => !isApprovalPattern(p));
}

/**
 * Recognises Slack and Mattermost incoming-webhook URLs, so the form can
 * suggest the chat format. A hint only: the server accepts any valid URL.
 */
export function looksLikeChatWebhook(raw: string): boolean {
  let u: URL;
  try {
    u = new URL(raw.trim());
  } catch {
    return false;
  }
  if (u.hostname === "hooks.slack.com") return true;
  // Mattermost: https://<host>/hooks/<26-character id>
  return /^\/hooks\/[a-z0-9]{26}\/?$/i.test(u.pathname);
}
