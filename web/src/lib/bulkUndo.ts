// Undo for a bulk edit (ADR-0050): the previous values differ per item, so
// the undo is one bulk change per previous value, each sent with the
// versions the edit returned (a later edit by someone else then fails as a
// version conflict instead of being overwritten).
import type { BulkChange } from "../api/bulk";
import type { FieldValue, Item } from "../api/types";

export interface UndoStep {
  targets: Item[];
  change: BulkChange;
}

/** The previous value the change replaced on one item, or undefined when it is not undoable. */
function previous(change: BulkChange, it: Item): { key: string; change: BulkChange } | undefined {
  if (change.status_id !== undefined) return { key: it.status.id, change: { status_id: it.status.id } };
  if (change.assignee_id !== undefined) return { key: String(it.assignee_id), change: { assignee_id: it.assignee_id } };
  if (change.due_date !== undefined) return { key: String(it.due_date), change: { due_date: it.due_date } };
  const fields = change.fields ? Object.keys(change.fields) : [];
  if (fields.length === 1) {
    const k = fields[0] ?? "";
    const v: FieldValue = it.fields[k] ?? null;
    return { key: JSON.stringify(v), change: { fields: { [k]: v } } };
  }
  return undefined;
}

/** Steps that put the edited items back; empty when nothing (or a delete) can be undone. */
export function undoPlan(change: BulkChange, before: readonly Item[], after: readonly Item[]): UndoStep[] {
  if (change.delete) return [];
  const prior = new Map(before.map((it) => [it.number, it]));
  const groups = new Map<string, UndoStep>();
  for (const now of after) {
    const old = prior.get(now.number);
    if (!old || old.version === now.version) continue; // unchanged by this edit
    const p = previous(change, old);
    if (!p) return [];
    const g = groups.get(p.key);
    if (g) g.targets.push(now);
    else groups.set(p.key, { targets: [now], change: p.change });
  }
  return [...groups.values()];
}
