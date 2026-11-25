import { useCallback, useState } from "react";

// Column widths the reader dragged (UI audit): per browser and project, like
// the table density (ADR-0048) — they depend on the screen, not on the list.

export const MIN_WIDTH = 3; // rem
export const MAX_WIDTH = 40;
const PREFIX = "nebula.colWidths.";

export type Widths = Readonly<Record<string, number>>;

export const clampWidth = (w: number): number => Math.round(Math.min(MAX_WIDTH, Math.max(MIN_WIDTH, w)) * 4) / 4;

/** Reads stored widths; anything malformed is dropped. */
export function parseWidths(raw: string | null): Widths {
  if (!raw) return {};
  try {
    const v: unknown = JSON.parse(raw);
    if (typeof v !== "object" || v === null || Array.isArray(v)) return {};
    const out: Record<string, number> = {};
    for (const [k, w] of Object.entries(v as Record<string, unknown>)) {
      if (typeof w === "number" && Number.isFinite(w) && k.length <= 100) out[k] = clampWidth(w);
    }
    return out;
  } catch {
    return {};
  }
}

function read(key: string): Widths {
  try {
    return parseWidths(localStorage.getItem(PREFIX + key));
  } catch {
    return {}; // storage blocked
  }
}

/** Widths for scope (e.g. "ws/PROJECT") and a setter; null resets a column. */
export function useColumnWidths(scope: string | undefined): [Widths, (col: string, w: number | null) => void] {
  const [widths, setWidths] = useState<Widths>(() => (scope ? read(scope) : {}));
  const [forScope, setForScope] = useState(scope);
  if (forScope !== scope) {
    // Another project: its own widths (state reset during render, no effect).
    setForScope(scope);
    setWidths(scope ? read(scope) : {});
  }
  const set = useCallback((col: string, w: number | null) => {
    if (!scope) return;
    setWidths((cur) => {
      const next: Record<string, number> = Object.fromEntries(Object.entries(cur).filter(([k]) => k !== col));
      if (w !== null) next[col] = clampWidth(w);
      try {
        if (Object.keys(next).length === 0) localStorage.removeItem(PREFIX + scope);
        else localStorage.setItem(PREFIX + scope, JSON.stringify(next));
      } catch {
        // Not persisted; still applied on this page.
      }
      return next;
    });
  }, [scope]);
  return [widths, set];
}
