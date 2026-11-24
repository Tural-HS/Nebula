// Audit evidence export (ADR-0029): signing key, export history, downloads.
import { useQuery } from "@tanstack/react-query";

import { api, apiPostBlob, path } from "./client";
import type { ActivityPage } from "./types";
import { fileNameFrom } from "../lib/evidence";

export interface EvidenceKey {
  enabled: boolean;
  algorithm?: string;
  public_key?: string;
  fingerprint?: string;
  limits?: { max_days: number; max_items: number; max_events: number };
}

export interface ScopeExport {
  project_keys: string[];
  from: string;
  to: string;
  approvals_only: boolean;
}

export const evidenceKeys = {
  key: (ws: string) => ["evidence", ws, "key"] as const,
  history: (ws: string) => ["evidence", ws, "history"] as const,
};

export const useEvidenceKey = (ws: string) =>
  useQuery({ queryKey: evidenceKeys.key(ws), queryFn: () => api<EvidenceKey>("GET", path`/workspaces/${ws}/evidence/key`), staleTime: 300_000 });

/** The last exports, from the audit log (every export is an evidence.exported event). */
export const useEvidenceHistory = (ws: string) =>
  useQuery({
    queryKey: evidenceKeys.history(ws),
    queryFn: () => api<ActivityPage>("GET", path`/workspaces/${ws}/audit-log` + "?action=evidence.exported&limit=20"),
  });

/** Saves a downloaded blob under name (a temporary object URL). */
export function saveBlob(blob: Blob, name: string): void {
  const url = URL.createObjectURL(blob);
  try {
    const link = document.createElement("a");
    link.href = url;
    link.download = name;
    link.rel = "noopener";
    document.body.append(link);
    link.click();
    link.remove();
  } finally {
    setTimeout(() => { URL.revokeObjectURL(url); }, 30_000);
  }
}

async function download(p: string, body: unknown, fallback: string): Promise<void> {
  const res = await apiPostBlob(p, body);
  saveBlob(res.blob, fileNameFrom(res.disposition, fallback));
}

export const exportItemEvidence = (ws: string, project: string, n: number, language: string) =>
  download(path`/workspaces/${ws}/projects/${project}/items/${n}/evidence`, { language }, `nebula-evidence-${project}-${String(n)}.zip`);

export const exportScopeEvidence = (ws: string, body: ScopeExport, language: string) =>
  download(path`/workspaces/${ws}/evidence/exports`, { ...body, language }, `nebula-evidence-${body.from}_${body.to}.zip`);
