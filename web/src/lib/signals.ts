import type { Item } from "../api/types";

/**
 * Whether the table's "Siqnal" cell (ADR-0035) of this row shows anything:
 * an approval, subtasks, a blocking link, an SLA, or — when the table has no
 * due date column — the due date. A table where no loaded row has one drops
 * the column, so imported ledgers with many fields keep their width.
 */
export function hasSignal(item: Item, withDue: boolean): boolean {
  return Boolean(
    item.approval || (item.subtasks && item.subtasks.count > 0) || item.blocked || item.sla || (withDue && item.due_date),
  );
}
