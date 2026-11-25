// Monthly period close of a project (ADR-0051).
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { api, apiGetFile, path } from "./client";
import { saveBlob } from "./evidence";
import { activeLocale } from "../i18n/locale";
import { exportFileName, type ExportFormat } from "../lib/itemExport";
import { keys } from "./hooks";
import type { FieldValue } from "./types";

export interface CycleRefill {
  target: string;
  source: string;
}

export interface CycleInput {
  enabled: boolean;
  day_of_month: number;
  clear_fields: string[];
  refill: CycleRefill[];
  /** Reset closed (done/canceled) items too, not only open ones. */
  include_closed: boolean;
  /** Status the reset items get; null keeps their status. */
  reset_status_id: string | null;
}

export interface Cycle extends CycleInput {
  next_run: string | null;
  next_period: string | null;
  last_error: string | null;
  updated_at: string | null;
}

export interface CycleItemError {
  number: number;
  code: string;
  message: string;
}

export interface PeriodClose {
  id: string;
  period: string; // YYYY-MM
  closed_by: string | null; // null: the scheduler
  item_count: number;
  changed: number;
  failed: CycleItemError[];
  created_at: string;
}

export interface CyclePreview {
  period: string;
  closed: boolean;
  items: number;
  open: number;
  will_reset: number;
}

export interface SnapshotRow {
  number: number;
  title: string;
  status_id: string;
  /** The status's current name. */
  status_name: string;
  assignee_id: string | null;
  fields: Record<string, FieldValue>;
}

const base = (ws: string, project: string) => path`/workspaces/${ws}/projects/${project}/cycle`;
const cycleKey = (ws: string, project: string) => [...keys.items(ws, project), "cycle"] as const;

export const useCycle = (ws: string, project: string) =>
  useQuery({ queryKey: cycleKey(ws, project), queryFn: () => api<Cycle>("GET", base(ws, project)) });

export const useCloses = (ws: string, project: string) =>
  useQuery({
    queryKey: [...cycleKey(ws, project), "closes"],
    queryFn: async () => (await api<{ closes: PeriodClose[] }>("GET", base(ws, project) + "/closes")).closes,
  });

export const useSnapshot = (ws: string, project: string, period: string | null) =>
  useQuery({
    enabled: period !== null,
    queryKey: [...cycleKey(ws, project), "closes", period],
    queryFn: () => api<{ close: PeriodClose; items: SnapshotRow[] }>("GET", base(ws, project) + path`/closes/${period ?? ""}`),
  });

export const useCyclePreview = (ws: string, project: string, period: string, enabled: boolean) =>
  useQuery({
    enabled,
    queryKey: [...cycleKey(ws, project), "preview", period],
    queryFn: () => api<CyclePreview>("POST", base(ws, project) + "/preview", { period }),
  });

export function useCycleApi(ws: string, project: string) {
  const qc = useQueryClient();
  return {
    save: useMutation({
      mutationFn: (body: CycleInput) => api<Cycle>("PUT", base(ws, project), body),
      onSuccess: () => qc.invalidateQueries({ queryKey: cycleKey(ws, project) }),
    }),
    close: useMutation({
      mutationFn: (period: string) => api<PeriodClose>("POST", base(ws, project) + "/close", { period }),
      // The items changed too: refresh the lists as well.
      onSuccess: () => qc.invalidateQueries({ queryKey: keys.items(ws, project) }),
    }),
  };
}

/** Downloads one closed month as xlsx or CSV (semicolons: Excel with a comma decimal). */
export async function downloadClose(ws: string, project: string, period: string, format: ExportFormat): Promise<string> {
  const q = new URLSearchParams({ format, lang: activeLocale() });
  if (format === "csv") q.set("delimiter", "semicolon");
  const res = await apiGetFile(path`/workspaces/${ws}/projects/${project}/cycle/closes/${period}/export` + "?" + q.toString());
  const name = exportFileName(res.disposition, `${project}-${period}.${format}`, format);
  saveBlob(res.blob, name);
  return name;
}
