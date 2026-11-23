import { describe, expect, it, vi } from "vitest";

import { isChunkLoadError, reloadOnceForChunk } from "./chunkReload";

describe("isChunkLoadError", () => {
  it.each([
    "Failed to fetch dynamically imported module: http://x/assets/BoardView-abc.js",
    "Importing a module script failed.",
    "error loading dynamically imported module",
  ])("recognizes %s", (m) => {
    expect(isChunkLoadError(new TypeError(m))).toBe(true);
  });
  it("ignores other errors", () => {
    expect(isChunkLoadError(new Error("Cannot read properties of undefined"))).toBe(false);
  });
});

describe("reloadOnceForChunk", () => {
  it("reloads once, then not again within the guard window", () => {
    const reload = vi.fn();
    vi.stubGlobal("location", { reload });
    const store = new Map<string, string>();
    const storage = { getItem: (k: string) => store.get(k) ?? null, setItem: (k: string, v: string) => { store.set(k, v); } };
    expect(reloadOnceForChunk(100_000, storage)).toBe(true);
    expect(reloadOnceForChunk(105_000, storage)).toBe(false);
    expect(reloadOnceForChunk(111_000, storage)).toBe(true);
    expect(reload).toHaveBeenCalledTimes(2);
    vi.unstubAllGlobals();
  });
});
