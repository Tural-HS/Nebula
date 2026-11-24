// Russian strings for approval cards in chat webhooks (ADR-0031).
import type { Messages } from "../../i18n/locale";

export const twc = {
  approvalEvents: "Отправлять события согласования",
  approvalEventsHint: "Когда согласование запрошено, получено, отклонено или отменено, в канал приходит цветная карточка со ссылкой «Открыть в Nebula». Решения принимаются только в Nebula.",
  coveredByWildcard: "Выбрано «*»: события согласования тоже отправляются.",
} satisfies Messages["webhookChat"];
