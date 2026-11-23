import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { api, path } from "./client";
import type { FieldDefinition, FieldValue, Item, ItemPage, ItemType, Me, Member, Project, Status, Workspace } from "./types";

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

export const useProjects = (ws: string) =>
  useQuery({
    queryKey: keys.projects(ws),
    queryFn: async () => (await api<{ projects: Project[] }>("GET", path`/workspaces/${ws}/projects`)).projects,
  });

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

export interface ItemFilters {
  q: string;
  assignee: "" | "me" | "none";
}

const PAGE_SIZE = 100;

export function itemsQueryString(f: ItemFilters, cursor: string | null): string {
  const p = new URLSearchParams({ limit: String(PAGE_SIZE) });
  if (f.q.trim()) p.set("q", f.q.trim());
  if (f.assignee) p.set("assignee", f.assignee);
  if (cursor) p.set("cursor", cursor);
  return p.toString();
}

export const useItems = (ws: string, project: string, filters: ItemFilters) =>
  useInfiniteQuery({
    queryKey: [...keys.items(ws, project), filters],
    initialPageParam: null as string | null,
    queryFn: ({ pageParam }) =>
      api<ItemPage>("GET", path`/workspaces/${ws}/projects/${project}/items` + "?" + itemsQueryString(filters, pageParam)),
    getNextPageParam: (last) => last.next_cursor,
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
      (await api<{ statuses: Status[] }>("GET", path`/workspaces/${ws}/projects/${project}/items/${n ?? 0}/transitions`)).statuses,
    enabled: n !== null,
  });

export interface ItemPatch {
  version: number;
  title?: string;
  description?: string | null;
  status_id?: string;
  assignee_id?: string | null;
  fields?: Record<string, FieldValue>;
}

export function useUpdateItem(ws: string, project: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ n, patch }: { n: number; patch: ItemPatch }) =>
      api<Item>("PATCH", path`/workspaces/${ws}/projects/${project}/items/${n}`, patch),
    onSuccess: (item) => {
      qc.setQueryData(keys.item(ws, project, item.number), item);
    },
    onSettled: (_d, _e, vars) => {
      void qc.invalidateQueries({ queryKey: keys.items(ws, project) });
      void qc.invalidateQueries({ queryKey: keys.item(ws, project, vars.n) });
    },
  });
}

export interface NewItem {
  title: string;
  type?: string;
  assignee_id?: string | null;
  description?: string | null;
  fields?: Record<string, FieldValue>;
}

export function useCreateItem(ws: string, project: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: NewItem) => api<Item>("POST", path`/workspaces/${ws}/projects/${project}/items`, body),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.items(ws, project) }),
  });
}

export function useDeleteItem(ws: string, project: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (n: number) => api<undefined>("DELETE", path`/workspaces/${ws}/projects/${project}/items/${n}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.items(ws, project) }),
  });
}

export function useCreateWorkspace() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: { slug: string; name: string }) => api<Workspace>("POST", "/workspaces", body),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.workspaces }),
  });
}

export function useCreateProject(ws: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: { key: string; name: string; description?: string }) =>
      api<Project>("POST", path`/workspaces/${ws}/projects`, body),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.projects(ws) }),
  });
}
