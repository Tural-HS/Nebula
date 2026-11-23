import type { SearchHit } from "../api/search";

/** Mirrors the API limits (store.ValidateSearchQuery). */
export const SEARCH_MAX = 200;
const KEY_RE = /^[A-Za-z][A-Za-z0-9]{1,9}-[1-9][0-9]{0,17}$/;
const WORD_RE = /[\p{L}\p{N}]{2,}|\p{N}/u;

/** Length in code points: the unit the API counts (Go runes), not UTF-16 units. */
export function codePoints(s: string): number {
  return Array.from(s).length;
}

/** Whether the API will accept q: an item key, or a word of 2+ letters/digits. */
export function isSearchable(q: string): boolean {
  const s = q.trim();
  if (s === "" || codePoints(s) > SEARCH_MAX) return false;
  return KEY_RE.test(s) || WORD_RE.test(s);
}

export interface HitGroups {
  items: SearchHit[];
  comments: SearchHit[];
}

/** Splits hits into items and comments, keeping the server's ranking. */
export function groupHits(hits: readonly SearchHit[]): HitGroups {
  const out: HitGroups = { items: [], comments: [] };
  for (const h of hits) (h.kind === "comment" ? out.comments : out.items).push(h);
  return out;
}

/** Deep link that opens the item drawer on its project page. */
export function hitHref(ws: string, h: Pick<SearchHit, "project_key" | "item_number">): string {
  return `/w/${encodeURIComponent(ws)}/p/${encodeURIComponent(h.project_key)}?item=${String(h.item_number)}`;
}

/** Stable DOM id of a result row (aria-activedescendant). */
export function hitId(prefix: string, h: Pick<SearchHit, "kind" | "item_key" | "comment_id">): string {
  return `${prefix}-${h.kind}-${h.comment_id ?? h.item_key}`;
}

/** The next active index for a navigation key, wrapping around; undefined for other keys. */
export function moveActive(key: string, active: number, count: number): number | undefined {
  if (count === 0) return undefined;
  switch (key) {
    case "ArrowDown":
      return (active + 1) % count;
    case "ArrowUp":
      return (active - 1 + count) % count;
    case "Home":
      return 0;
    case "End":
      return count - 1;
    default:
      return undefined;
  }
}

/** Cmd+K on macOS, Ctrl+K elsewhere (both accepted everywhere). */
export function isSearchShortcut(e: Pick<KeyboardEvent, "key" | "metaKey" | "ctrlKey" | "altKey" | "shiftKey">): boolean {
  return (e.metaKey || e.ctrlKey) && !e.altKey && !e.shiftKey && e.key.toLowerCase() === "k";
}
