// Turkish strings for the in-app notification center (ADR-0017).
import type { Messages } from "../../i18n/locale";

export const tn = {
  title: "Bildirimler",
  empty: "Henüz bildirim yok.",
  markAllRead: "Tümünü okundu işaretle",
  someone: "Bir kullanıcı",
  bellLabel: (count: number, capped: boolean) =>
    count === 0 ? "Bildirimler" : capped || count > 99 ? "Bildirimler, 99'dan fazla okunmamış" : `Bildirimler, ${String(count)} okunmamış`,
  liveCount: (count: number, capped: boolean) =>
    count === 0 ? "Okunmamış bildirim yok" : capped || count > 99 ? "99'dan fazla okunmamış bildirim" : `${String(count)} okunmamış bildirim`,
  mention: (actor: string) => `${actor} sizi andı`,
  assignedBy: (actor: string) => `${actor} size atadı`,
  assigned: "Size atandı",
  overdue: "Bitiş tarihi geçti",
  comment: (actor: string) => `${actor} yorum yazdı`,
  statusChanged: (actor: string) => `${actor} durumu değiştirdi`,
  assigneeChanged: (actor: string) => `${actor} atanan kişiyi değiştirdi`,
  dueChanged: (actor: string) => `${actor} bitiş tarihini değiştirdi`,
  linkAdded: (actor: string) => `${actor} bağlantı ekledi`,
  unread: "okunmamış",
} satisfies Messages["notifications"];
