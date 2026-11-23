// Saved views of a project (ADR-0014).
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { api, path } from "./client";
import type { SavedView, ViewConfig } from "./types";

export const viewKeys = {
  list: (ws: string, project: string) => ["ws", ws, "p", project, "views"] as const,
};

export const useViews = (ws: string, project: string) =>
  useQuery({
    queryKey: viewKeys.list(ws, project),
    queryFn: async () => (await api<{ views: SavedView[] }>("GET", path`/workspaces/${ws}/projects/${project}/views`)).views,
  });

export interface ViewBody {
  name?: string;
  kind?: SavedView["kind"];
  shared?: boolean;
  config?: ViewConfig;
}

export function useViewMutations(ws: string, project: string) {
  const qc = useQueryClient();
  const base = path`/workspaces/${ws}/projects/${project}/views`;
  const refresh = () => qc.invalidateQueries({ queryKey: viewKeys.list(ws, project) });
  return {
    create: useMutation({
      mutationFn: (body: ViewBody) => api<SavedView>("POST", base, body),
      onSuccess: refresh,
    }),
    update: useMutation({
      mutationFn: ({ id, body }: { id: string; body: ViewBody }) => api<SavedView>("PATCH", base + path`/${id}`, body),
      onSuccess: refresh,
    }),
    remove: useMutation({
      mutationFn: (id: string) => api<undefined>("DELETE", base + path`/${id}`),
      onSuccess: refresh,
    }),
  };
}
