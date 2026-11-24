import type { FieldDefinition, Member, Status } from "../api/types";
import { t } from "../i18n";
import { tap } from "../i18n-approvals";
import { tlg } from "../i18n-ledger";
import { valueType } from "./formula";
import type { PageState } from "./views";

// Active filters as removable chips (ADR-0035): label and the state patch
// that removes the filter.
export interface FilterChip {
  key: string;
  label: string;
  clear: Partial<PageState>;
}

const DUE_LABEL = (): Record<Exclude<PageState["due"], "">, string> => ({
  overdue: t.dueOverdue,
  today: t.dueToday,
  week: t.dueWeek,
  none: t.dueNone,
});

export function filterChips(
  s: PageState,
  statuses: readonly Status[],
  fields: ReadonlyMap<string, FieldDefinition>,
  members: ReadonlyMap<string, Member>,
): FilterChip[] {
  const out: FilterChip[] = [];
  if (s.assignee) {
    const who = s.assignee === "me" ? t.filterMine : s.assignee === "none" ? t.filterNone : (members.get(s.assignee)?.display_name ?? "…");
    out.push({ key: "assignee", label: `${t.assignee}: ${who}`, clear: { assignee: "" } });
  }
  if (s.due) out.push({ key: "due", label: `${t.dueFilter}: ${DUE_LABEL()[s.due]}`, clear: { due: "" } });
  if (s.approval) out.push({ key: "approval", label: s.approval === "mine" ? tap.filter.mine : tap.filter.pending, clear: { approval: "" } });
  if (s.blocked) out.push({ key: "blocked", label: tlg.blockedChip, clear: { blocked: false } });
  if (s.status.length > 0) {
    const names = s.status.map((id) => statuses.find((x) => x.id === id)?.name ?? "…").join(", ");
    out.push({ key: "status", label: tlg.statuses(names), clear: { status: [] } });
  }
  for (const [k, v] of Object.entries(s.fields)) {
    const f = fields.get(k);
    const value = f && valueType(f) === "boolean" ? (v === "true" ? t.yes : t.no) : (f?.options.choices?.find((c) => c.id === v)?.label ?? v);
    const rest = Object.fromEntries(Object.entries(s.fields).filter(([x]) => x !== k));
    out.push({ key: "field." + k, label: `${f?.name ?? k}: ${value}`, clear: { fields: rest } });
  }
  return out;
}

