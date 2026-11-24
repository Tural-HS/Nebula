import { LINK_TYPES, type ItemLink, type LinkType } from "../api/links";
import type { SearchHit } from "../api/search";
import type { StatusCategory } from "../api/types";

export interface LinkGroup {
  type: LinkType;
  links: ItemLink[];
}

/** Groups links by type in display order (blocks, blocked by, ...); empty groups are left out. */
export function groupLinks(links: readonly ItemLink[]): LinkGroup[] {
  return LINK_TYPES.map((type) => ({ type, links: links.filter((l) => l.type === type) })).filter((g) => g.links.length > 0);
}

/** Done and canceled items are shown struck through. */
export const isClosed = (c: StatusCategory): boolean => c === "done" || c === "canceled";

/** Open blockers: links "blocked by" an item that is not done or canceled. */
export function openBlockers(links: readonly ItemLink[]): ItemLink[] {
  return links.filter((l) => l.type === "blocked_by" && !isClosed(l.item.status.category));
}

/** Where a linked item opens: the drawer on its own project page (links cross projects). */
export function linkedItemHref(ws: string, l: Pick<ItemLink, "item">): string {
  return `/w/${encodeURIComponent(ws)}/p/${encodeURIComponent(l.item.project_key)}?item=${String(l.item.number)}`;
}

/** The stored type: "A blocked by B" is "B blocks A", so both are one link kind. */
export function baseType(t: LinkType): "blocks" | "relates" | "duplicates" {
  if (t === "blocked_by") return "blocks";
  if (t === "duplicated_by") return "duplicates";
  return t;
}

/**
 * Search hits that can become a link target: one per item (a comment hit
 * stands for its item), never the item itself or one already linked with the
 * chosen type in either direction (the server refuses those).
 */
export function linkCandidates(hits: readonly SearchHit[], selfKey: string, links: readonly ItemLink[], type: LinkType): SearchHit[] {
  const taken = new Set(links.filter((l) => baseType(l.type) === baseType(type)).map((l) => l.item.key));
  const seen = new Set<string>();
  const out: SearchHit[] = [];
  for (const h of hits) {
    if (h.item_key === selfKey || taken.has(h.item_key) || seen.has(h.item_key)) continue;
    seen.add(h.item_key);
    out.push(h);
  }
  return out;
}
