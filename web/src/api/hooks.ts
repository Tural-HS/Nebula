import { keepPreviousData, useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { api, path } from "./client";
import type { ActivityPage, Comment, CommentPage, FieldDefinition, FieldSums, FieldValue, Item, ItemPage, ItemType, Me, Member, Project, ProjectMember, TransitionTarget, Workspace } from "./types";
import type { CreateWorkspaceBody } from "../lib/workspaceKinds";

export const keys = {
  me: ["me"] as const,
  workspaces: ["workspaces"] as const,
  workspace: (ws: string) => ["ws", ws] as const,
  members: (ws: string) => ["ws", ws, "members"] as const,
  projects: (ws: string) => ["ws", ws, "projects"] as const,
  itemTypes: (ws: string) => ["ws", ws, "item-types"] as const,
  fields: (ws: string) => ["ws", ws, "fields"] as const,
  items: (ws: string, project: string) => ["ws", ws, "p", project, "items"] as const,
  item: (ws: string, project: string, n: number) => ["ws", ws, "p", project, "item", n] as const,
  /** Every single-item query of a project (keys.item and what lives under it). */
  itemPrefix: (ws: string, project: string) => ["ws", ws, "p", project, "item"] as const,
  transitions: (ws: string, project: string, n: number) => ["ws", ws, "p", project, "item", n, "transitions"] as const,
};

export const useMe = () => useQuery({ queryKey: keys.me, queryFn: () => api<Me>("GET", "/me") });

export const useWorkspaces = () =>
  useQuery({ queryKey: keys.workspaces, queryFn: async () => (await api<{ workspaces: Workspace[] }>("GET", "/workspaces")).workspaces });

export const useWorkspace = (ws: string) =>
  useQuery({ queryKey: keys.workspace(ws), queryFn: () => api<Workspace>("GET", path`/workspaces/${ws}`) });

export const useMembers = (ws: string) =>
  useQuery({
    queryKey: keys.members(ws),
    queryFn: async () => (await api<{ members: Member[] }>("GET", path`/workspaces/${ws}/members`)).members,
    staleTime: 60_000,
  });

/** Sets or clears the view a project opens with (admin+). */
export function useSetDefaultView(ws: string, project: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string | null) => api<Project>("PATCH", path`/workspaces/${ws}/projects/${project}`, { default_view_id: id }),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.projects(ws) }),
  });
}

export const useProjects = (ws: string) =>
  useQuery({
    queryKey: keys.projects(ws),
    queryFn: async () => (await api<{ projects: Project[] }>("GET", path`/workspaces/${ws}/projects`)).projects,
  });

export const useProjectMembers = (ws: string, project: string, enabled = true) =>
  useQuery({
    enabled,
    queryKey: ["ws", ws, "p", project, "members"],
    queryFn: async () => (await api<{ members: ProjectMember[] }>("GET", path`/workspaces/${ws}/projects/${project}/members`)).members,
  });

/**
 * Workspace members who can see the project: everyone for a workspace
 * project; for a private one, its members plus workspace admins. Assignee
 * pickers and @mentions offer only these people (the server enforces it).
 */
export function useAssignable(ws: string, project: string): Member[] {
  const members = useMembers(ws);
  const projects = useProjects(ws);
  const isPrivate = projects.data?.find((p) => p.key === project)?.visibility === "private";
  const projectMembers = useProjectMembers(ws, project, isPrivate);
  return assignable(members.data ?? [], isPrivate, projectMembers.data ?? []);
}

export function assignable(members: readonly Member[], isPrivate: boolean, projectMembers: readonly { user_id: string }[]): Member[] {
  if (!isPrivate) return [...members];
  const inProject = new Set(projectMembers.map((m) => m.user_id));
  return members.filter((m) => m.role === "admin" || m.role === "owner" || inProject.has(m.user_id));
}

export const useItemTypes = (ws: string) =>
  useQuery({
    queryKey: keys.itemTypes(ws),
    queryFn: async () => (await api<{ item_types: ItemType[] }>("GET", path`/workspaces/${ws}/item-types`)).item_types,
    staleTime: 60_000,
  });

export const useFields = (ws: string) =>
  useQuery({
    queryKey: keys.fields(ws),
    queryFn: async () => (await api<{ fields: FieldDefinition[] }>("GET", path`/workspaces/${ws}/fields`)).fields,
    staleTime: 60_000,
  });

export type DueFilter = "" | "overdue" | "today" | "week" | "none";
/** Built-in sorts, or a field sort (ADR-0030): "field.<key>" ascending, "-field.<key>" descending. */
export type ItemSort = "number" | "due" | `field.${string}` | `-field.${string}`;

export interface ItemFilters {
  q: string;
  /** "", "me", "none" or a user id. */
  assignee: string;
  due?: DueFilter;
  status?: readonly string[];
  fields?: Readonly<Record<string, string>>;
  sort?: ItemSort;
  /** Inclusive due-date window (calendar view), YYYY-MM-DD. */
  dueFrom?: string;
  dueTo?: string;
  /** Only subtasks of this item number (ADR-0021). */
  parent?: number;
  /** Pending approvals (ADR-0026): any open request, or ones the viewer may decide. */
  approval?: "" | "pending" | "mine";
  /** Only items with an open blocker the viewer can see (ADR-0028). */
  blocked?: boolean;
}

const PAGE_SIZE = 100;

export function itemsQueryString(f: ItemFilters, cursor: string | null): string {
  const p = new URLSearchParams({ limit: String(PAGE_SIZE) });
  if (f.q.trim()) p.set("q", f.q.trim());
  if (f.assignee) p.set("assignee", f.assignee);
  if (f.due) p.set("due", f.due);
  if (f.status && f.status.length > 0) p.set("status", f.status.join(","));
  for (const [k, v] of Object.entries(f.fields ?? {}).sort(([a], [b]) => a.localeCompare(b))) p.set("field." + k, v);
  if (f.sort && f.sort !== "number") p.set("sort", f.sort);
  if (f.dueFrom) p.set("due_from", f.dueFrom);
  if (f.dueTo) p.set("due_to", f.dueTo);
  if (f.parent) p.set("parent", String(f.parent));
  if (f.approval) p.set("approval", f.approval);
  if (f.blocked) p.set("blocked", "true");
  if (cursor) p.set("cursor", cursor);
  return p.toString();
}

export const useItems = (ws: string, project: string, filters: ItemFilters, enabled = true) =>
  useInfiniteQuery({
    enabled,
    queryKey: [...keys.items(ws, project), filters],
    initialPageParam: null as string | null,
    queryFn: ({ pageParam }) =>
      api<ItemPage>("GET", path`/workspaces/${ws}/projects/${project}/items` + "?" + itemsQueryString(filters, pageParam)),
    getNextPageParam: (last) => last.next_cursor,
  });

const COLUMN_PAGE = 50;

/** One board column: items in any of the given statuses, newest first, paged.
 *  The key lives under keys.items, so every item mutation refreshes it. */
export const useColumnItems = (ws: string, project: string, filters: ItemFilters, statusIds: readonly string[]) =>
  useInfiniteQuery({
    enabled: statusIds.length > 0,
    queryKey: [...keys.items(ws, project), "column", statusIds, filters],
    initialPageParam: null as string | null,
    queryFn: ({ pageParam }) => {
      const p = new URLSearchParams(itemsQueryString(filters, pageParam));
      p.set("limit", String(COLUMN_PAGE));
      p.set("status", statusIds.join(",")); // the column's statuses, already narrowed to the filter
      return api<ItemPage>("GET", path`/workspaces/${ws}/projects/${project}/items` + "?" + p.toString());
    },
    getNextPageParam: (last) => last.next_cursor,
  });

/** Item counts per status for the current filters (board column sizes). */
export const useItemCounts = (ws: string, project: string, filters: ItemFilters, enabled = true) =>
  useQuery({
    enabled,
    queryKey: [...keys.items(ws, project), "counts", filters],
    queryFn: () =>
      api<{ by_status: Record<string, number>; total: number }>(
        "GET",
        path`/workspaces/${ws}/projects/${project}/items/counts` + "?" + itemsQueryString(filters, null),
      ),
  });

/** Quick-filter counts (ADR-0035): one request for all presets. */
export interface QueueCounts {
  all: number;
  mine: number;
  overdue: number;
  approval: number;
  blocked: number;
  week: number;
}

/**
 * The list's search, status and field filters apply to every count; the
 * presets set assignee, due, approval and blocked themselves, so those are
 * left out of the request (and of the cache key).
 */
export const useQueueCounts = (ws: string, project: string, filters: ItemFilters, enabled = true) => {
  const base: ItemFilters = { q: filters.q, assignee: "", ...(filters.status ? { status: filters.status } : {}), ...(filters.fields ? { fields: filters.fields } : {}) };
  return useQuery({
    enabled,
    queryKey: [...keys.items(ws, project), "queue-counts", base],
    queryFn: () => api<QueueCounts>("GET", path`/workspaces/${ws}/projects/${project}/items/queue-counts` + "?" + itemsQueryString(base, null)),
    placeholderData: keepPreviousData,
  });
};

/** Totals of number fields over every item matching the filters (the table's sum row). */
export const useItemSums = (ws: string, project: string, filters: ItemFilters, fields: readonly string[]) =>
  useQuery({
    enabled: fields.length > 0,
    queryKey: [...keys.items(ws, project), "sums", filters, fields],
    queryFn: async () => {
      const p = new URLSearchParams(itemsQueryString(filters, null));
      p.delete("limit");
      p.delete("sort");
      p.set("fields", fields.join(","));
      return (await api<{ sums: FieldSums }>("GET", path`/workspaces/${ws}/projects/${project}/items/sums` + "?" + p.toString())).sums;
    },
  });

export const useItem = (ws: string, project: string, n: number | null) =>
  useQuery({
    queryKey: keys.item(ws, project, n ?? 0),
    queryFn: () => api<Item>("GET", path`/workspaces/${ws}/projects/${project}/items/${n ?? 0}`),
    enabled: n !== null,
  });

export const useTransitions = (ws: string, project: string, n: number | null) =>
  useQuery({
    queryKey: keys.transitions(ws, project, n ?? 0),
    queryFn: async () =>
      (await api<{ statuses: TransitionTarget[] }>("GET", path`/workspaces/${ws}/projects/${project}/items/${n ?? 0}/transitions`)).statuses,
    enabled: n !== null,
  });

export interface ItemPatch {
  version: number;
  title?: string;
  description?: string | null;
  status_id?: string;
  assignee_id?: string | null;
  due_date?: string | null;
  fields?: Record<string, FieldValue>;
  /** Parent item number or key; null makes the item top-level. */
  parent?: number | string | null;
}

export function useUpdateItem(ws: string, project: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ n, patch }: { n: number; patch: ItemPatch }) =>
      api<Item>("PATCH", path`/workspaces/${ws}/projects/${project}/items/${n}`, patch),
    onSuccess: (item) => {
      qc.setQueryData(keys.item(ws, project, item.number), item);
    },
    onSettled: (item, _e, vars) => {
      void qc.invalidateQueries({ queryKey: keys.items(ws, project) });
      void qc.invalidateQueries({ queryKey: keys.item(ws, project, vars.n) });
      // A subtask's status or parent changes its parents' progress (ADR-0021).
      if (item?.parent || vars.patch.parent !== undefined) void qc.invalidateQueries({ queryKey: keys.itemPrefix(ws, project) });
    },
  });
}

export interface NewItem {
  title: string;
  type?: string;
  assignee_id?: string | null;
  due_date?: string | null;
  description?: string | null;
  fields?: Record<string, FieldValue>;
  parent?: number | string;
}

export function useCreateItem(ws: string, project: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: NewItem) => api<Item>("POST", path`/workspaces/${ws}/projects/${project}/items`, body),
    onSuccess: async (item) => {
      if (item.parent) await qc.invalidateQueries({ queryKey: keys.item(ws, project, item.parent.number) });
      await qc.invalidateQueries({ queryKey: keys.items(ws, project) });
    },
  });
}

export function useDeleteItem(ws: string, project: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (n: number) => api<undefined>("DELETE", path`/workspaces/${ws}/projects/${project}/items/${n}`),
    onSuccess: async () => {
      // Its parent's progress and its subtasks' parent links change too.
      await qc.invalidateQueries({ queryKey: keys.itemPrefix(ws, project) });
      await qc.invalidateQueries({ queryKey: keys.items(ws, project) });
    },
  });
}

export function useCreateWorkspace() {
  const qc = useQueryClient();
  return useMutation({
    /** kind: team template, ADR-0033 (omitted = general). */
    mutationFn: (body: CreateWorkspaceBody) => api<Workspace>("POST", "/workspaces", body),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.workspaces }),
  });
}

export function useCreateProject(ws: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: {
      key: string; name: string; description?: string; visibility?: Project["visibility"];
      /** Copy setup from an existing project (ADR-0020). */
      template?: { from: string; members: boolean; recurrences: boolean; views: boolean };
    }) =>
      api<Project>("POST", path`/workspaces/${ws}/projects`, body),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.projects(ws) }),
  });
}

// ---------------------------------------------------------------------------
// Comments and activity
// ---------------------------------------------------------------------------

export const collabKeys = {
  comments: (ws: string, project: string, n: number) => ["ws", ws, "p", project, "item", n, "comments"] as const,
  activity: (ws: string, project: string, n: number) => ["ws", ws, "p", project, "item", n, "activity"] as const,
};

export const useComments = (ws: string, project: string, n: number) =>
  useInfiniteQuery({
    queryKey: collabKeys.comments(ws, project, n),
    initialPageParam: null as string | null,
    queryFn: ({ pageParam }) =>
      api<CommentPage>(
        "GET",
        path`/workspaces/${ws}/projects/${project}/items/${n}/comments` + "?limit=50" + (pageParam ? "&cursor=" + encodeURIComponent(pageParam) : ""),
      ),
    getNextPageParam: (last) => last.next_cursor,
  });

export const useActivity = (ws: string, project: string, n: number) =>
  useInfiniteQuery({
    queryKey: collabKeys.activity(ws, project, n),
    initialPageParam: 0,
    queryFn: ({ pageParam }) =>
      api<ActivityPage>(
        "GET",
        path`/workspaces/${ws}/projects/${project}/items/${n}/activity` + "?limit=50" + (pageParam ? `&before=${String(pageParam)}` : ""),
      ),
    getNextPageParam: (last) => last.next_before ?? undefined,
  });

function useInvalidateCollab(ws: string, project: string, n: number) {
  const qc = useQueryClient();
  return () => {
    void qc.invalidateQueries({ queryKey: collabKeys.comments(ws, project, n) });
    void qc.invalidateQueries({ queryKey: collabKeys.activity(ws, project, n) });
    // Commenting and mentioning make people watchers (ADR-0028).
    void qc.invalidateQueries({ queryKey: [...keys.item(ws, project, n), "watchers"] });
  };
}

export function useCommentMutations(ws: string, project: string, n: number) {
  const invalidate = useInvalidateCollab(ws, project, n);
  const base = path`/workspaces/${ws}/projects/${project}/items/${n}/comments`;
  return {
    create: useMutation({
      mutationFn: (body: string) => api<Comment>("POST", base, { body }),
      onSuccess: invalidate,
    }),
    update: useMutation({
      mutationFn: ({ id, body }: { id: string; body: string }) => api<Comment>("PATCH", base + path`/${id}`, { body }),
      onSuccess: invalidate,
    }),
    remove: useMutation({
      mutationFn: (id: string) => api<undefined>("DELETE", base + path`/${id}`),
      onSuccess: invalidate,
    }),
  };
}
