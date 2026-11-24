// Turkish strings for approval cards in chat webhooks (ADR-0031).
import type { Messages } from "../../i18n/locale";

export const twc = {
  approvalEvents: "Onay olaylarını gönder",
  approvalEventsHint: "Onay istendiğinde, verildiğinde, reddedildiğinde veya iptal edildiğinde kanala renkli bir kart ve “Nebula'da aç” bağlantısı gider. Kararlar yalnızca Nebula'da verilir.",
  coveredByWildcard: "“*” seçili: onay olayları da gönderilir.",
} satisfies Messages["webhookChat"];
