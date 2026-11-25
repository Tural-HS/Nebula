import { useSyncExternalStore } from "react";

// Table density (ADR-0048): a per-browser choice, not a view setting — it
// depends on the reader's screen and eyes, not on the list.
export type Density = "normal" | "compact";

export const DENSITY_KEY = "nebula.tableDensity";
const listeners = new Set<() => void>();

export function readDensity(): Density {
  try {
    return localStorage.getItem(DENSITY_KEY) === "compact" ? "compact" : "normal";
  } catch {
    return "normal"; // storage blocked (private mode, policy)
  }
}

export function setDensity(d: Density): void {
  try {
    localStorage.setItem(DENSITY_KEY, d);
  } catch {
    // Not persisted; still applied for this page via the listeners.
  }
  current = d;
  for (const l of listeners) l();
}

let current: Density | null = null;

export function useDensity(): Density {
  return useSyncExternalStore(
    (cb) => {
      listeners.add(cb);
      const onStorage = (e: StorageEvent) => {
        if (e.key === DENSITY_KEY) {
          current = readDensity();
          cb();
        }
      };
      window.addEventListener("storage", onStorage);
      return () => {
        listeners.delete(cb);
        window.removeEventListener("storage", onStorage);
      };
    },
    () => (current ??= readDensity()),
    () => "normal",
  );
}
