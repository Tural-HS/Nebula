import type { Item, ItemType, Status, StatusCategory } from "../api/types";

export const CATEGORIES: readonly StatusCategory[] = ["todo", "in_progress", "done", "canceled"];

export type GroupBy = "status" | "category";

/** A board column: one or more statuses that are shown and dropped on together. */
export interface BoardColumn {
  key: string;
  name: string;
  category: StatusCategory;
  statusIds: string[];
  /** Holds statuses of no current item type (archived types); not a drop target. */
  other?: boolean;
}

const RANK: Record<StatusCategory, number> = { todo: 0, in_progress: 1, done: 2, canceled: 3 };

/**
 * The item types a project's board shows columns for: the ones that have
 * items in it. Other projects' types (and their workflows' statuses) stay
 * off this board. A project with no items yet, or counts not loaded, shows
 * every type so there is somewhere to start.
 */
export function typesInUse(types: readonly ItemType[], byStatus: Readonly<Record<string, number>> | undefined): readonly ItemType[] {
  if (!byStatus) return types;
  const used = types.filter((ty) => ty.statuses.some((s) => (byStatus[s.id] ?? 0) > 0));
  return used.length > 0 ? used : types;
}

/**
 * Builds the board's columns from the workspace's item types.
 *
 * By status, every status gets a column. Item types can share a workflow or
 * use different ones, so statuses with the same name and category in different
 * workflows are merged ("Done" in the bug and the task workflow is one column).
 * Columns are ordered by category, then by workflow position, then by the
 * order the item types list them in.
 * By category, there are exactly four columns.
 */
export function buildColumns(
  types: readonly ItemType[],
  groupBy: GroupBy,
  categoryName: (c: StatusCategory) => string,
): BoardColumn[] {
  if (groupBy === "category") {
    return CATEGORIES.map((c) => ({
      key: "c:" + c,
      name: categoryName(c),
      category: c,
      statusIds: [...new Set(types.flatMap((t) => t.statuses.filter((s) => s.category === c).map((s) => s.id)))],
    }));
  }
  const byKey = new Map<string, BoardColumn & { order: number; seq: number }>();
  for (const type of types) {
    type.statuses.forEach((s, i) => {
      const key = `s:${s.category}:${s.name.trim().toLocaleLowerCase()}`;
      const col = byKey.get(key);
      if (!col) {
        byKey.set(key, { key, name: s.name.trim(), category: s.category, statusIds: [s.id], order: i, seq: byKey.size });
      } else {
        if (!col.statusIds.includes(s.id)) col.statusIds.push(s.id);
        col.order = Math.min(col.order, i);
      }
    });
  }
  return [...byKey.values()]
    .sort((a, b) => RANK[a.category] - RANK[b.category] || a.order - b.order || a.seq - b.seq)
    .map((c) => ({ key: c.key, name: c.name, category: c.category, statusIds: c.statusIds }));
}

/**
 * Statuses that have items but belong to no column (their item type was
 * archived). They get one extra read-only column so nothing silently vanishes
 * from the board.
 */
export function otherColumn(columns: readonly BoardColumn[], countedStatusIds: Iterable<string>, name: string): BoardColumn | null {
  const known = new Set(columns.flatMap((c) => c.statusIds));
  const rest = [...countedStatusIds].filter((id) => !known.has(id));
  return rest.length ? { key: "other", name, category: "canceled", statusIds: rest, other: true } : null;
}

/**
 * Picks the status an item moves to when dropped on a column: the first status
 * of the item's own workflow that is in the column and that the workflow allows
 * right now. Null when the move is not possible (or the item is already there).
 */
export function pickTargetStatus(
  item: Item,
  column: BoardColumn,
  itemTypes: readonly ItemType[],
  allowed: readonly Status[],
): Status | null {
  if (column.other || column.statusIds.includes(item.status.id)) return null;
  const type = itemTypes.find((t) => t.id === item.type.id);
  const ordered = type ? type.statuses : allowed;
  const allowedIds = new Set(allowed.map((s) => s.id));
  const inColumn = new Set(column.statusIds);
  return ordered.find((s) => inColumn.has(s.id) && allowedIds.has(s.id)) ?? null;
}

/** Keys of the columns an item can be dropped on right now. */
export function dropTargets(item: Item, columns: readonly BoardColumn[], itemTypes: readonly ItemType[], allowed: readonly Status[]): Set<string> {
  return new Set(columns.filter((c) => pickTargetStatus(item, c, itemTypes, allowed)).map((c) => c.key));
}

/** Sum of the per-status counts of a column's statuses. */
export function columnCount(column: BoardColumn, byStatus: Readonly<Record<string, number>> | undefined): number | undefined {
  if (!byStatus) return undefined;
  return column.statusIds.reduce((n, id) => n + (byStatus[id] ?? 0), 0);
}

/** Flattens infinite-query pages, dropping duplicates that can appear when
 *  items are created while paging. */
export function flattenPages(pages: readonly { items: Item[] }[]): Item[] {
  const seen = new Set<string>();
  const out: Item[] = [];
  for (const p of pages) {
    for (const it of p.items) {
      if (!seen.has(it.id)) {
        seen.add(it.id);
        out.push(it);
      }
    }
  }
  return out;
}

export function initials(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean);
  const first = parts[0]?.[0] ?? "?";
  const second = parts.length > 1 ? (parts[parts.length - 1]?.[0] ?? "") : "";
  return (first + second).toUpperCase();
}
