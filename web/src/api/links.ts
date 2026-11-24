// Item links and watchers (ADR-0028).
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { api, path } from "./client";
import { collabKeys, keys } from "./hooks";
import type { Status } from "./types";

/** Link types as seen from the item they are shown on. */
export const LINK_TYPES = ["blocks", "blocked_by", "relates", "duplicates", "duplicated_by"] as const;
export type LinkType = (typeof LINK_TYPES)[number];

export interface LinkedItem {
  key: string;
  project_key: string;
  number: number;
  title: string;
  status: Status;
}

export interface ItemLink {
  id: string;
  type: LinkType;
  item: LinkedItem;
  created_at: string;
}

export interface Watchers {
  watching: boolean;
  count: number;
  watchers: { id: string; display_name: string }[];
}

export const linkKeys = {
  links: (ws: string, project: string, n: number) => [...keys.item(ws, project, n), "links"] as const,
  watchers: (ws: string, project: string, n: number) => [...keys.item(ws, project, n), "watchers"] as const,
};

const itemPath = (ws: string, project: string, n: number) => path`/workspaces/${ws}/projects/${project}/items/${n}`;

export const useLinks = (ws: string, project: string, n: number) =>
  useQuery({
    queryKey: linkKeys.links(ws, project, n),
    queryFn: async () => (await api<{ links: ItemLink[] }>("GET", itemPath(ws, project, n) + "/links")).links,
  });

export function useLinkMutations(ws: string, project: string, n: number) {
  const qc = useQueryClient();
  const refresh = () => {
    void qc.invalidateQueries({ queryKey: linkKeys.links(ws, project, n) });
    void qc.invalidateQueries({ queryKey: collabKeys.activity(ws, project, n) });
    void qc.invalidateQueries({ queryKey: keys.items(ws, project) }); // "blocked" badges
  };
  return {
    add: useMutation({
      mutationFn: (body: { type: LinkType; target: string }) => api<ItemLink>("POST", itemPath(ws, project, n) + "/links", body),
      onSuccess: refresh,
    }),
    remove: useMutation({
      mutationFn: (id: string) => api<undefined>("DELETE", itemPath(ws, project, n) + path`/links/${id}`),
      onSuccess: refresh,
    }),
  };
}

export const useWatchers = (ws: string, project: string, n: number) =>
  useQuery({
    queryKey: linkKeys.watchers(ws, project, n),
    queryFn: () => api<Watchers>("GET", itemPath(ws, project, n) + "/watchers"),
  });

/** Watch or unwatch; userId "me" is the signed-in user (anyone), another id needs admin. */
export function useSetWatching(ws: string, project: string, n: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ userId, on }: { userId: string; on: boolean }) =>
      api<Watchers>(on ? "PUT" : "DELETE", itemPath(ws, project, n) + path`/watchers/${userId}`),
    onSuccess: (data) => { qc.setQueryData(linkKeys.watchers(ws, project, n), data); },
  });
}
