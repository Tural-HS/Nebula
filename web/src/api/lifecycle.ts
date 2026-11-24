// Archive, restore and delete projects and workspaces (ADR-0039).
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { api, path } from "./client";
import { homeKeys } from "./home";
import { keys } from "./hooks";
import type { Project, Workspace } from "./types";

export const lifecycleKeys = {
  archivedWorkspaces: ["workspaces", "archived"] as const,
  archivedProjects: (ws: string) => ["ws", ws, "archived-projects"] as const,
  projectState: (ws: string, project: string) => ["ws", ws, "p", project, "state"] as const,
};

/** Archived workspaces the user administers (home page section). */
export const useArchivedWorkspaces = () =>
  useQuery({
    queryKey: lifecycleKeys.archivedWorkspaces,
    queryFn: async () => (await api<{ workspaces: Workspace[] }>("GET", "/workspaces?state=archived")).workspaces,
  });

/** Archived projects of a workspace (admins only). */
export const useArchivedProjects = (ws: string, enabled: boolean) =>
  useQuery({
    queryKey: lifecycleKeys.archivedProjects(ws),
    queryFn: async () => (await api<{ projects: Project[] }>("GET", path`/workspaces/${ws}/archived-projects`)).projects,
    enabled,
  });

/** One project by key; an archived one answers 410 (lib/lifecycle projectGone). */
export const useProjectState = (ws: string, project: string, enabled: boolean) =>
  useQuery({
    queryKey: lifecycleKeys.projectState(ws, project),
    queryFn: () => api<Project>("GET", path`/workspaces/${ws}/projects/${project}`),
    enabled,
    retry: false,
  });

export interface DeleteResult {
  status: "deleted" | "deleting";
}

export function useWorkspaceLifecycle(ws: string) {
  const qc = useQueryClient();
  const refresh = async () => {
    await Promise.all([
      qc.invalidateQueries({ queryKey: keys.workspaces }),
      qc.invalidateQueries({ queryKey: keys.workspace(ws) }),
      qc.invalidateQueries({ queryKey: lifecycleKeys.archivedWorkspaces }),
      qc.invalidateQueries({ queryKey: homeKeys.all }),
    ]);
  };
  return {
    archive: useMutation({ mutationFn: () => api<Workspace>("POST", path`/workspaces/${ws}/archive`), onSuccess: refresh }),
    restore: useMutation({ mutationFn: () => api<Workspace>("POST", path`/workspaces/${ws}/restore`), onSuccess: refresh }),
    remove: useMutation({
      mutationFn: (body: { confirm: string; evidence_acknowledged: boolean }) => api<DeleteResult>("DELETE", path`/workspaces/${ws}`, body),
      onSuccess: async () => {
        qc.removeQueries({ queryKey: keys.workspace(ws) });
        await Promise.all([
          qc.invalidateQueries({ queryKey: keys.workspaces }),
          qc.invalidateQueries({ queryKey: lifecycleKeys.archivedWorkspaces }),
          qc.invalidateQueries({ queryKey: homeKeys.all }),
        ]);
      },
    }),
  };
}

export function useProjectLifecycle(ws: string, project: string) {
  const qc = useQueryClient();
  const refresh = async () => {
    await Promise.all([
      qc.invalidateQueries({ queryKey: keys.projects(ws) }),
      qc.invalidateQueries({ queryKey: lifecycleKeys.archivedProjects(ws) }),
      qc.invalidateQueries({ queryKey: lifecycleKeys.projectState(ws, project) }),
      qc.invalidateQueries({ queryKey: homeKeys.all }),
    ]);
  };
  return {
    archive: useMutation({ mutationFn: () => api<Project>("POST", path`/workspaces/${ws}/projects/${project}/archive`), onSuccess: refresh }),
    restore: useMutation({ mutationFn: () => api<Project>("POST", path`/workspaces/${ws}/projects/${project}/restore`), onSuccess: refresh }),
    remove: useMutation({
      mutationFn: (body: { confirm: string }) => api<DeleteResult>("DELETE", path`/workspaces/${ws}/projects/${project}`, body),
      onSuccess: refresh,
    }),
  };
}
