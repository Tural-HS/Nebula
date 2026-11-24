// Azerbaijani strings for the item list and panel layout (ADR-0035).
export const tlg = {
  queues: "Tez filtrlər",
  queue: { all: "Hamısı", mine: "Mənə təyin", overdue: "Gecikmiş", approval: "Təsdiq gözləyir", blocked: "Bloklanmış", week: "Bu həftə bitir" },
  queueCount: (label: string, n: number) => `${label}: ${String(n)}`,
  signals: "Siqnal",
  signalsHint: "Təsdiq, bloklanma, alt tapşırıqlar və gecikmiş son tarix",
  activeFilters: "Aktiv filtrlər",
  removeFilter: (label: string) => `Filtri götür: ${label}`,
  clearFilters: "Hamısını təmizlə",
  statuses: (names: string) => `Status: ${names}`,
  blockedChip: "Bloklanmış",
  workflow: "İş axını",
  awaitingApproval: (done: number, min: number, to: string) => `Təsdiq ${String(done)}/${String(min)}, sonra: ${to}`,
  details: "Xüsusiyyətlər",
  approvedBy: "təsdiqlədi",
  waitingFor: "gözlənilir",
} as const;
