// Turkish strings for the item list and panel layout (ADR-0035).
import type { Messages } from "../../i18n/locale";

export const tlg = {
  queues: "Hızlı filtreler",
  queue: { all: "Tümü", mine: "Bana atanan", overdue: "Gecikmiş", approval: "Onay bekleyen", blocked: "Engellenmiş", week: "Bu hafta biten" },
  queueCount: (label: string, n: number) => `${label}: ${String(n)}`,
  signals: "Sinyal",
  signalsHint: "Onay, engeller, alt görevler ve geciken bitiş tarihi",
  activeFilters: "Etkin filtreler",
  removeFilter: (label: string) => `Filtreyi kaldır: ${label}`,
  clearFilters: "Tümünü temizle",
  statuses: (names: string) => `Durum: ${names}`,
  blockedChip: "Engellenmiş",
  workflow: "İş akışı",
  awaitingApproval: (done: number, min: number, to: string) => `Onay ${String(done)}/${String(min)}, sonra: ${to}`,
  details: "Özellikler",
  collab: "Yorumlar ve geçmiş",
  approvedBy: "onayladı",
  waitingFor: "bekleniyor",
} satisfies Messages["ledger"];
