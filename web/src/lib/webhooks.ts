export type WebhookFormat = "nebula" | "slack";

export const WEBHOOK_FORMATS: readonly WebhookFormat[] = ["nebula", "slack"];

/** Events a chat channel usually wants; the Nebula default stays "item.*". */
export function defaultEvents(format: WebhookFormat): string {
  return format === "slack" ? "item.*, comment.created" : "item.*";
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
