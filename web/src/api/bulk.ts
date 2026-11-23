// Bulk and inline item edits (ADR-0023).

import { useMutation, useQueryClient, type InfiniteData, type QueryClient } from "@tanstack/react-query";

import { api, ApiError, path } from "./client";
import { keys, type ItemPatch } from "./hooks";
import type { FieldValue, Item, ItemPage } from "./types";

/** Items one bulk request may change (server: store.MaxBulkItems). */
export const MAX_BULK = 500;

export interface BulkChange {
  status_id?: string;
  assignee_id?: string | null;
  due_date?: string | null;
  fields?: Record<string, FieldValue>;
  delete?: true;
}

export interface BulkItemError {
  number: number;
  code: string;
  message: string;
  field?: string;
}

export interface BulkResult {
  bulk_id: string;
  items: Item[];
  changed: number;
  unchanged: number;
}

/** A rolled-back bulk change: nothing was applied; items says why, per item. */
export class BulkFailedError extends ApiError {
  readonly items: BulkItemError[];
  constructor(base: ApiError, items: BulkItemError[]) {
    super(base.status, base.code, base.message, base.field, base.retryAfter);
    this.name = "BulkFailedError";
    this.items = items;
  }
}

/** Reads error.items from a bulk_failed body; anything malformed is dropped. */
export function bulkErrors(body: unknown): BulkItemError[] {
  const list = (body as { error?: { items?: unknown } } | null)?.error?.items;
  if (!Array.isArray(list)) return [];
  return list.flatMap((x: unknown) => {
    const e = x as Partial<BulkItemError> | null;
    return e && typeof e.number === "number" && typeof e.code === "string" && typeof e.message === "string"
      ? [{ number: e.number, code: e.code, message: e.message, ...(typeof e.field === "string" ? { field: e.field } : {}) }]
      : [];
  });
}

export async function postBulk(ws: string, project: string, targets: readonly Pick<Item, "number" | "version">[], change: BulkChange): Promise<BulkResult> {
  // The shared client keeps only code and message of an error body; keep
  // the per-item list as well.
  let errorBody: unknown = null;
  const capture: typeof fetch = async (input, init) => {
    const res = await fetch(input, init);
    if (!res.ok) errorBody = await res.clone().json().catch(() => null);
    return res;
  };
  const body = { items: targets.map((t) => ({ number: t.number, version: t.version })), ...change };
  try {
    return await api<BulkResult>("POST", path`/workspaces/${ws}/projects/${project}/items/bulk`, body, capture);
  } catch (err) {
    if (err instanceof ApiError && err.code === "bulk_failed") throw new BulkFailedError(err, bulkErrors(errorBody));
    throw err;
  }
}

/** Refreshes every list and every open item of the project. */
function refreshItems(qc: QueryClient, ws: string, project: string) {
  void qc.invalidateQueries({ queryKey: keys.items(ws, project) });
  void qc.invalidateQueries({ queryKey: ["ws", ws, "p", project, "item"] });
}

/**
 * One change for many items. A 429 is retried by the app's mutation
 * defaults; that is safe here even beyond the rate limiter, because every
 * item carries its version: a request that already committed fails as a
 * whole with version_conflict instead of applying twice.
 */
export function useBulkItems(ws: string, project: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ targets, change }: { targets: readonly Item[]; change: BulkChange }) => postBulk(ws, project, targets, change),
    onSettled: () => { refreshItems(qc, ws, project); },
  });
}

type Pages = InfiniteData<ItemPage>;
const isPages = (d: unknown): d is Pages => Array.isArray((d as Pages | undefined)?.pages);

/** Replaces one item (by id) in every cached list page of the project. */
export function replaceInLists(qc: QueryClient, ws: string, project: string, item: Item) {
  qc.setQueriesData({ queryKey: keys.items(ws, project) }, (old: unknown) => {
    if (!isPages(old) || !old.pages.some((p) => p.items.some((it) => it.id === item.id))) return old;
    return { ...old, pages: old.pages.map((p) => ({ ...p, items: p.items.map((it) => (it.id === item.id ? item : it)) })) };
  });
}

/**
 * A single-item edit shown at once: the lists show `optimistic` until the
 * server answers, then the stored item; on an error the lists go back to
 * what they showed before. The version check (409) works as in the drawer.
 */
export function useOptimisticUpdate(ws: string, project: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ item, patch }: { item: Item; patch: Omit<ItemPatch, "version">; optimistic: Item }) =>
      api<Item>("PATCH", path`/workspaces/${ws}/projects/${project}/items/${item.number}`, { ...patch, version: item.version }),
    onMutate: async ({ optimistic }) => {
      await qc.cancelQueries({ queryKey: keys.items(ws, project) });
      const before = qc.getQueriesData({ queryKey: keys.items(ws, project) });
      replaceInLists(qc, ws, project, optimistic);
      return { before };
    },
    onError: (_err, _vars, ctx) => {
      for (const [key, data] of ctx?.before ?? []) qc.setQueryData(key, data);
    },
    onSuccess: (item) => {
      replaceInLists(qc, ws, project, item);
      qc.setQueryData(keys.item(ws, project, item.number), item);
    },
    onSettled: (_d, _e, vars) => {
      void qc.invalidateQueries({ queryKey: keys.items(ws, project) });
      void qc.invalidateQueries({ queryKey: keys.item(ws, project, vars.item.number) });
    },
  });
}
