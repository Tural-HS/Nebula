import { useSyncExternalStore } from "react";

import type { PageState } from "./views";

// ---------------------------------------------------------------------------
// Phone and tablet layout helpers (ADR-0019). Layout switches that change the
// DOM (not just styles) use useMediaQuery, so the page never renders two
// copies of the same control where only one is visible.
// ---------------------------------------------------------------------------

/** Below Tailwind's `md` breakpoint (768 px): phones and small tablets in portrait. */
export const COMPACT_QUERY = "(max-width: 767.98px)";

/** Whether a CSS media query matches; follows changes (rotation, resize). */
export function useMediaQuery(query: string): boolean {
  return useSyncExternalStore(
    (onChange) => {
      const mq = window.matchMedia(query);
      mq.addEventListener("change", onChange);
      return () => { mq.removeEventListener("change", onChange); };
    },
    () => window.matchMedia(query).matches,
    () => false,
  );
}

export const useCompact = () => useMediaQuery(COMPACT_QUERY);

/** A mouse or trackpad is the primary pointer (not a touch screen). */
export const FINE_POINTER = "(pointer: fine)";

/**
 * Columns of the table as shown. On a narrow screen the key moves into the
 * title cell (a second line), and the title comes first so it stays pinned to
 * the left while the other columns scroll sideways. Every row and the totals
 * row use the same list, so cells stay aligned.
 */
export function displayColumns(columns: readonly string[], compact: boolean): { columns: string[]; keyInTitle: boolean } {
  if (!compact || !columns.includes("title")) return { columns: [...columns], keyInTitle: false };
  const keyInTitle = columns.includes("key");
  return { columns: ["title", ...columns.filter((c) => c !== "title" && c !== "key")], keyInTitle };
}

/** How many filters the compact "Filtrlər" button stands for (search excluded: it stays visible). */
export function activeFilterCount(s: Pick<PageState, "assignee" | "due" | "status" | "fields"> & { approval?: string }): number {
  return (s.assignee ? 1 : 0) + (s.due ? 1 : 0) + (s.approval ? 1 : 0) + s.status.length + Object.keys(s.fields).length;
}

/**
 * Height of the on-screen keyboard (or anything else covering the bottom of
 * the layout viewport), from the Visual Viewport API. Zero where the browser
 * already shrinks the layout viewport (Android with interactive-widget=resizes-content).
 */
export function keyboardInset(layoutHeight: number, visual: { height: number; offsetTop: number } | null | undefined): number {
  if (!visual) return 0;
  const inset = Math.round(layoutHeight - visual.height - visual.offsetTop);
  // Small differences are browser chrome (URL bar) animating, not a keyboard.
  return inset > 80 ? inset : 0;
}

/** Follows keyboardInset() while mounted; 0 where the API is missing. */
export function useKeyboardInset(): number {
  return useSyncExternalStore(
    (onChange) => {
      const vv = window.visualViewport;
      vv?.addEventListener("resize", onChange);
      vv?.addEventListener("scroll", onChange);
      return () => {
        vv?.removeEventListener("resize", onChange);
        vv?.removeEventListener("scroll", onChange);
      };
    },
    () => keyboardInset(window.innerHeight, window.visualViewport),
    () => 0,
  );
}
