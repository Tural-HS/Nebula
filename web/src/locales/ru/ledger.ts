// Russian strings for the item list and panel layout (ADR-0035).
import type { Messages } from "../../i18n/locale";

export const tlg = {
  queues: "Быстрые фильтры",
  queue: { all: "Все", mine: "Назначены мне", overdue: "Просрочены", approval: "Ждут согласования", blocked: "Заблокированы", week: "Срок на этой неделе", sla_breached: "SLA нарушен", sla_at_risk: "Риск SLA" },
  queueCount: (label: string, n: number) => `${label}: ${String(n)}`,
  signals: "Сигналы",
  signalsHint: "Согласование, блокировки, подзадачи и просроченный срок",
  activeFilters: "Активные фильтры",
  removeFilter: (label: string) => `Убрать фильтр: ${label}`,
  clearFilters: "Сбросить все",
  statuses: (names: string) => `Статус: ${names}`,
  blockedChip: "Заблокированы",
  workflow: "Процесс",
  awaitingApproval: (done: number, min: number, to: string) => `Согласование ${String(done)}/${String(min)}, затем: ${to}`,
  details: "Свойства",
  collab: "Комментарии и история",
  approvedBy: "согласовал(а)",
  waitingFor: "ожидается",
} satisfies Messages["ledger"];
