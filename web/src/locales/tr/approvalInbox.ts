// Turkish strings for the approval inbox, re-approval and deadlines (ADR-0034).
import type { Messages } from "../../i18n/locale";

export const tai = {
  nav: "Onay bekleyenler",
  title: "Onay bekleyenler",
  hint: "Çalışma alanında görebildiğiniz tüm projelerden bekleyen onay talepleri, en eskisi önce.",
  tabs: { mine: "Kararımı bekleyenler", all: "Tüm bekleyenler" },
  empty: { mine: "Kararınızı bekleyen talep yok.", all: "Bekleyen onay talebi yok." },
  badge: (n: number) => `${String(n)} talep kararınızı bekliyor`,
  badgeOverdue: (n: number) => `${String(n)} talebin süresi geçti`,
  requestedBy: (name: string) => `Talep eden: ${name}`,
  age: (days: number) => (days <= 0 ? "bugün" : `${String(days)} gün önce`),
  due: (date: string) => `Son gün: ${date}`,
  overdue: (date: string) => `Süresi geçti (son gün ${date})`,
  yourTurn: "Kararınızı bekliyor",
  open: "Aç",
  more: "Daha fazla göster",
  invalidated: "Onaylanan içerik sonradan değişti: bu onay artık geçersiz.",
  reopened: "Onaylanan içerik değiştiği için yeniden onay isteniyor.",
  notify: {
    overdue: "Bir onay talebinin süresi geçti",
    invalidated: (actor: string) => `${actor} onaylanan içeriği değiştirdi, yeniden onay gerekiyor`,
  },
  act: {
    invalidated: "onaylanan içeriği değiştirdi: onay geçersiz, görev yeniden onaya gönderildi",
    overdue: "onay süresi geçti",
  },
  errors: {
    reapproval_required: "Onaylanan içeriği değiştirmek yeni onay gerektirir: bu görevi tek başına düzenleyin.",
  } as Record<string, string>,
  settings: {
    reapprove: "Onaydan sonraki değişiklik yeniden onay gerektirir",
    reapproveHint: "İzlenen içerik değişince görev onaydan önceki duruma döner ve yeni talep açılır; değiştiren kişi onaylayamaz.",
    watched: "İzlenen içerik",
    watchAll: "Tümü: başlık, açıklama ve tüm alanlar",
    titleField: "Başlık",
    descriptionField: "Açıklama",
    due: "Karar süresi, iş günü",
    dueHint: "İş günü takvimine göre hesaplanır. Boş bırakırsanız süre olmaz.",
    dueRange: "1 ile 30 arasında olmalıdır.",
    summaryDue: (n: number) => `${String(n)} iş günü`,
    summaryReapprove: "değişiklikte yeniden",
  },
  reapproveWarn: (what: string) => `Dikkat: ${what} değişirse görev yeniden onaya gönderilir.`,
  watchAllShort: "başlık, açıklama veya alanlar",
} satisfies Messages["approvalInbox"];
