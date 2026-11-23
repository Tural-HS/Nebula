// Spreadsheet import API (ADR-0011). The file is parsed on the server; the
// browser only uploads it and edits the column mapping.
import { ApiError, apiUpload, path } from "./client";
import type { FieldType } from "./types";

export type BuiltinTarget = "title" | "description" | "type" | "status" | "assignee";

export interface ImportTarget {
  id: string; // "title" … or "field.<key>"
  kind: "builtin" | "field";
  required?: boolean;
  key?: string;
  name?: string;
  type?: FieldType;
}

export interface MappedColumn {
  column: number;
  target: string;
}

export interface ImportPreview {
  file_name: string;
  format: "csv" | "xlsx" | "boards";
  sheets: string[];
  sheet: string;
  delimiter: string;
  headers: string[];
  rows: { row: number; cells: string[] }[];
  row_count: number;
  targets: ImportTarget[];
  suggested: MappedColumn[];
  /** Mattermost Boards: people found in the board (ids only, ADR-0020). */
  people: BoardPerson[];
  limits: { max_file_bytes: number; max_rows: number; max_columns: number };
}

export interface BoardPerson {
  id: string;
  cards: number;
  samples: string[];
}

export interface ImportRowError {
  row: number;
  column: number | null;
  header?: string;
  target?: string;
  field?: string;
  message: string;
}

export interface ImportReport {
  dry_run: boolean;
  file_name: string;
  row_count: number;
  valid: number;
  created: number;
  first_key?: string;
  last_key?: string;
  error_count: number;
  errors: ImportRowError[];
  truncated: boolean;
}

export interface ImportMapping {
  columns: MappedColumn[];
  default_type: string;
  /** Mattermost user id -> member email; omitted when empty. */
  people?: Record<string, string>;
}

function form(file: File, sheet: string, mapping?: ImportMapping): FormData {
  const fd = new FormData();
  fd.append("file", file, file.name);
  if (sheet) fd.append("sheet", sheet);
  if (mapping) fd.append("mapping", JSON.stringify(mapping));
  return fd;
}

export async function previewImport(ws: string, project: string, file: File, sheet = ""): Promise<ImportPreview> {
  const res = await apiUpload(path`/workspaces/${ws}/projects/${project}/imports/preview`, form(file, sheet));
  return res.body as ImportPreview;
}

/** Dry run or real import. A rejected real import (422) still returns its row report. */
export async function runImport(ws: string, project: string, file: File, sheet: string, mapping: ImportMapping, dryRun: boolean): Promise<ImportReport> {
  const p = path`/workspaces/${ws}/projects/${project}/imports` + (dryRun ? "?dry_run=true" : "");
  const res = await apiUpload(p, form(file, sheet, mapping), [422]);
  const body = res.body as ImportReport | ErrorOnly;
  if (!("row_count" in body)) {
    // 422 without a report: the mapping itself was rejected.
    const e = body.error;
    throw new ApiError(res.status, e?.code ?? "validation_failed", e?.message ?? "invalid mapping", e?.field);
  }
  return body;
}

interface ErrorOnly {
  error?: { code?: string; message?: string; field?: string };
}
