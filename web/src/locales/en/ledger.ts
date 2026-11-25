// English strings for the item list and panel layout (ADR-0035).
import type { Messages } from "../../i18n/locale";

export const tlg = {
  queues: "Quick filters",
  queue: { all: "All", mine: "Assigned to me", overdue: "Overdue", approval: "Awaiting approval", blocked: "Blocked", week: "Due this week", sla_breached: "SLA breached", sla_at_risk: "SLA at risk" },
  queueCount: (label: string, n: number) => `${label}: ${String(n)}`,
  signals: "Signals",
  signalsHint: "Approval, blockers, subtasks and overdue due date",
  activeFilters: "Active filters",
  removeFilter: (label: string) => `Remove filter: ${label}`,
  clearFilters: "Clear all",
  statuses: (names: string) => `Status: ${names}`,
  blockedChip: "Blocked",
  workflow: "Workflow",
  awaitingApproval: (done: number, min: number, to: string) => `Approval ${String(done)}/${String(min)}, then: ${to}`,
  details: "Details",
  keyFacts: "Key facts",
  addMore: "Add:",
  addDescription: "Description",
  addSubtask: "Subtask",
  addLink: "Link",
  addFile: "File",
  collab: "Comments and activity",
  approvedBy: "approved",
  waitingFor: "waiting",
} satisfies Messages["ledger"];
