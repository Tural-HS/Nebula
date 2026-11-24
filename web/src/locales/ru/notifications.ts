// Russian strings for the in-app notification center (ADR-0017).
import type { Messages } from "../../i18n/locale";
import { plural } from "../../i18n/plural";

const unreadWord = (n: number) => plural("ru", n, { one: "непрочитанное", few: "непрочитанных", many: "непрочитанных", other: "непрочитанных" });
const unreadNotifications = (n: number) =>
  plural("ru", n, { one: "непрочитанное уведомление", few: "непрочитанных уведомления", many: "непрочитанных уведомлений", other: "непрочитанных уведомления" });

export const tn = {
  title: "Уведомления",
  empty: "Уведомлений пока нет.",
  markAllRead: "Отметить все как прочитанные",
  someone: "Пользователь",
  bellLabel: (count: number, capped: boolean) =>
    count === 0 ? "Уведомления" : capped || count > 99 ? "Уведомления, более 99 непрочитанных" : `Уведомления, ${String(count)} ${unreadWord(count)}`,
  liveCount: (count: number, capped: boolean) =>
    count === 0 ? "Нет непрочитанных уведомлений" : capped || count > 99 ? "Более 99 непрочитанных уведомлений" : `${String(count)} ${unreadNotifications(count)}`,
  mention: (actor: string) => `${actor} упомянул(а) вас`,
  assignedBy: (actor: string) => `Вам назначено пользователем ${actor}`,
  assigned: "Назначено вам",
  overdue: "Срок истёк",
  comment: (actor: string) => `${actor} оставил(а) комментарий`,
  unread: "непрочитанное",
} satisfies Messages["notifications"];
