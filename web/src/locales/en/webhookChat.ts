// English strings for approval cards in chat webhooks (ADR-0031).
import type { Messages } from "../../i18n/locale";

export const twc = {
  approvalEvents: "Send approval events",
  approvalEventsHint: "When approval is requested, given, rejected or canceled, the channel gets a coloured card with an “Open in Nebula” link. Decisions are only made in Nebula.",
  coveredByWildcard: "“*” is selected: approval events are sent too.",
} satisfies Messages["webhookChat"];
