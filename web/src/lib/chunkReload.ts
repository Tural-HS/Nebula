const RELOAD_KEY = "nebula.chunkReloadAt";
const RELOAD_GUARD_MS = 10_000;

/** A lazy-loaded chunk that no longer exists: typically an old tab after a
 *  new version was deployed (chunk names carry content hashes). */
export function isChunkLoadError(err: unknown): boolean {
  const msg = err instanceof Error ? err.message : String(err);
  return /Failed to fetch dynamically imported module|error loading dynamically imported module|Importing a module script failed|Outdated Optimize Dep/i.test(msg);
}

/** Reloads once for a missing chunk; a second failure within the guard window
 *  shows the fallback instead of looping. Storage may be unavailable. */
export function reloadOnceForChunk(now = Date.now(), storage: Pick<Storage, "getItem" | "setItem"> | null = safeSession()): boolean {
  try {
    const last = Number(storage?.getItem(RELOAD_KEY) ?? 0);
    if (now - last < RELOAD_GUARD_MS) return false;
    storage?.setItem(RELOAD_KEY, String(now));
  } catch {
    return false; // no storage: never auto-reload, the user can click
  }
  globalThis.location.reload();
  return true;
}

function safeSession(): Storage | null {
  try {
    return globalThis.sessionStorage;
  } catch {
    return null;
  }
}

