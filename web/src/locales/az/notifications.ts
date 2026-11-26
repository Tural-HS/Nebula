// Azerbaijani strings for the in-app notification center (ADR-0017).
export const tn = {
  title: "Bildirişlər",
  empty: "Hələ bildiriş yoxdur.",
  markAllRead: "Hamısını oxunmuş et",
  someone: "Bir istifadəçi",
  bellLabel: (count: number, capped: boolean) =>
    count === 0 ? "Bildirişlər" : capped || count > 99 ? "Bildirişlər, 99-dan çox oxunmamış" : `Bildirişlər, ${String(count)} oxunmamış`,
  liveCount: (count: number, capped: boolean) =>
    count === 0 ? "Oxunmamış bildiriş yoxdur" : capped || count > 99 ? "99-dan çox oxunmamış bildiriş" : `${String(count)} oxunmamış bildiriş`,
  mention: (actor: string) => `${actor} sizi qeyd etdi`,
  assignedBy: (actor: string) => `${actor} tərəfindən sizə təyin edildi`,
  assigned: "Sizə təyin edildi",
  overdue: "Son tarixi keçdi",
  anniversary: "🎂 Bu gün ad günüdür",
  comment: (actor: string) => `${actor} şərh yazdı`,
  statusChanged: (actor: string) => `${actor} statusu dəyişdi`,
  assigneeChanged: (actor: string) => `${actor} icraçını dəyişdi`,
  dueChanged: (actor: string) => `${actor} son tarixi dəyişdi`,
  linkAdded: (actor: string) => `${actor} əlaqə əlavə etdi`,
  unread: "oxunmamış",
};
