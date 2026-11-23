/** Flattens paged results into one list, dropping duplicates by key. */
export function flattenBy<P, T>(pages: readonly P[], items: (p: P) => readonly T[], key: (t: T) => string | number): T[] {
  const seen = new Set<string | number>();
  const out: T[] = [];
  for (const p of pages) {
    for (const it of items(p)) {
      const k = key(it);
      if (!seen.has(k)) {
        seen.add(k);
        out.push(it);
      }
    }
  }
  return out;
}
