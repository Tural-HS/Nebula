import type { Item, ItemType, Status, StatusCategory } from "../api/types";

export const CATEGORIES: readonly StatusCategory[] = ["todo", "in_progress", "done", "canceled"];

/** Groups items into board columns by status category, preserving order. */
export function groupByCategory(items: readonly Item[]): Record<StatusCategory, Item[]> {
  const out: Record<StatusCategory, Item[]> = { todo: [], in_progress: [], done: [], canceled: [] };
  for (const it of items) out[it.status.category].push(it);
  return out;
}

/**
 * Picks the status an item should move to when dropped on a category column.
 * Items of different types can have different workflows, so the target is the
 * first status of the item's own workflow in that category that the workflow
 * currently allows. Returns null when the move is not possible.
 */
export function pickTargetStatus(
  item: Item,
  category: StatusCategory,
  itemTypes: readonly ItemType[],
  allowed: readonly Status[],
): Status | null {
  if (item.status.category === category) return null;
  const type = itemTypes.find((t) => t.id === item.type.id);
  const ordered = type ? type.statuses : allowed;
  const allowedIds = new Set(allowed.map((s) => s.id));
  return ordered.find((s) => s.category === category && allowedIds.has(s.id)) ?? null;
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
