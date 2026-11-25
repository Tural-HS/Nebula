// GET /workspaces/{ws}/reports: every widget of the reports page in one
// request (ADR-0041). Numbers only cover projects the viewer can see.
import { keepPreviousData, useQuery } from "@tanstack/react-query";

import { api, apiGetFile, path } from "./client";
import { saveBlob } from "./evidence";
import type { StatusCategory } from "./types";
import type { Locale } from "../i18n/locale";
import { exportFileName } from "../lib/itemExport";
import { reportsQuery, type ReportFilters } from "../lib/reports";

export interface ReportWorkCounts {
  open: number;
  overdue: number;
  completed: number;
}

export interface ReportAssignee extends ReportWorkCounts {
  /** null: items without an assignee. */
  assignee_id: string | null;
  name: string | null;
  projects: (ReportWorkCounts & { key: string })[];
}

export interface ReportApprovalRow {
  /** null on the total row, or when the target status was deleted. */
  to_status: { id: string; name: string; workflow: string } | null;
  approved: number;
  rejected: number;
  canceled: number;
  late: number;
  median_seconds: number | null;
  p90_seconds: number | null;
}

export interface ReportStatusCount {
  id: string;
  name: string;
  category: StatusCategory;
  count: number;
}

export interface ReportProjectStatus {
  key: string;
  name: string;
  total: number;
  statuses: ReportStatusCount[];
}

export interface ReportCount {
  bucket: string;
  count: number;
}

export interface Reports {
  range: { from: string; to: string; today: string; timezone: string; bucket: "week" | "month" };
  projects: { key: string; name: string }[];
  kpis: { open: number; created: number; completed: number; overdue: number; awaiting_approval: number; awaiting_approval_late: number };
  throughput: { start: string; created: number; completed: number }[];
  aging: { age: ReportCount[]; late: ReportCount[] };
  approvals: { total: ReportApprovalRow; by_transition: ReportApprovalRow[] };
  workload: ReportAssignee[];
  status: ReportProjectStatus[];
}

export const reportKeys = { all: (ws: string) => ["reports", ws] as const, page: (ws: string, q: string) => ["reports", ws, q] as const };

export const useReports = (ws: string, f: ReportFilters, enabled = true) => {
  const q = reportsQuery(f);
  return useQuery({
    queryKey: reportKeys.page(ws, q),
    queryFn: () => api<Reports>("GET", path`/workspaces/${ws}/reports` + "?" + q),
    enabled,
    // Changing a filter keeps the previous numbers on screen (dimmed) until
    // the new ones arrive.
    placeholderData: keepPreviousData,
    staleTime: 30_000,
  });
};

/** Downloads the workload table as CSV; resolves with the saved file name. */
export async function downloadWorkloadCsv(ws: string, f: ReportFilters, lang: Locale, semicolon: boolean): Promise<string> {
  const p = new URLSearchParams(reportsQuery(f));
  p.set("lang", lang);
  if (semicolon) p.set("delimiter", "semicolon");
  const res = await apiGetFile(path`/workspaces/${ws}/reports/workload.csv` + "?" + p.toString());
  const name = exportFileName(res.disposition, `${ws}-workload.csv`, "csv");
  saveBlob(res.blob, name);
  return name;
}
