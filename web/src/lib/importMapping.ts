// Pure helpers for the import wizard's column mapping (unit-tested).
import type { ImportMapping, ImportPreview, ImportTarget } from "../api/imports";

/** Target per column index; "" means the column is ignored. */
export type ColumnMapping = string[];

export const ACCEPTED_EXTENSIONS = [".csv", ".tsv", ".txt", ".xlsx", ".xlsm", ".boardarchive"] as const;
export const MAX_UPLOAD_BYTES = 10 * 1024 * 1024; // mirrors the server limit (ADR-0011)
/** Mattermost Boards archives also carry the board's images (ADR-0020). */
export const MAX_BOARDS_UPLOAD_BYTES = 50 * 1024 * 1024;

export const isBoardsArchive = (name: string) => name.toLowerCase().endsWith(".boardarchive");

/** The server's suggestions as a full per-column array. */
export function initialMapping(preview: Pick<ImportPreview, "headers" | "suggested" | "targets">): ColumnMapping {
  const known = new Set(preview.targets.map((t) => t.id));
  const out: ColumnMapping = preview.headers.map(() => "");
  for (const s of preview.suggested) {
    if (s.column >= 0 && s.column < out.length && known.has(s.target)) out[s.column] = s.target;
  }
  return out;
}

/** Sets one column's target. A target maps to one column only, so it is taken away from any other column. */
export function setTarget(mapping: ColumnMapping, column: number, target: string): ColumnMapping {
  return mapping.map((t, i) => (i === column ? target : target !== "" && t === target ? "" : t));
}

export interface MappingProblems {
  missingTitle: boolean;
  duplicates: string[];
}

export function mappingProblems(mapping: ColumnMapping): MappingProblems {
  const seen = new Set<string>();
  const dup = new Set<string>();
  for (const t of mapping) {
    if (!t) continue;
    if (seen.has(t)) dup.add(t);
    seen.add(t);
  }
  return { missingTitle: !seen.has("title"), duplicates: [...dup] };
}

export function isMappingValid(mapping: ColumnMapping): boolean {
  const p = mappingProblems(mapping);
  return !p.missingTitle && p.duplicates.length === 0;
}

export function toPayload(mapping: ColumnMapping, defaultType: string, people: Record<string, string> = {}): ImportMapping {
  const chosen = Object.fromEntries(Object.entries(people).filter(([, email]) => email !== ""));
  return {
    columns: mapping.flatMap((target, column) => (target ? [{ column, target }] : [])),
    default_type: defaultType,
    ...(Object.keys(chosen).length > 0 ? { people: chosen } : {}),
  };
}

/** Spreadsheet column letter: 0 → A, 25 → Z, 26 → AA. */
export function columnLetter(index: number): string {
  let name = "";
  for (let i = index + 1; i > 0; i = Math.floor((i - 1) / 26)) {
    name = String.fromCharCode(65 + ((i - 1) % 26)) + name;
  }
  return name;
}

/** "B · Filial", or just the letter for a blank header. */
export function columnLabel(index: number, header: string | undefined): string {
  const h = header?.trim();
  return h ? `${columnLetter(index)} · ${h}` : columnLetter(index);
}

export type FileProblem = "extension" | "size" | "empty" | null;

export function checkFile(file: Pick<File, "name" | "size">, maxBytes = isBoardsArchive(file.name) ? MAX_BOARDS_UPLOAD_BYTES : MAX_UPLOAD_BYTES): FileProblem {
  const name = file.name.toLowerCase();
  if (!ACCEPTED_EXTENSIONS.some((ext) => name.endsWith(ext))) return "extension";
  if (file.size === 0) return "empty";
  if (file.size > maxBytes) return "size";
  return null;
}

export { formatBytes } from "./format";

/** Display label for a target: custom fields by their name, built-ins via the dictionary. */
export function targetLabel(target: ImportTarget | undefined, builtin: Record<string, string>): string {
  if (!target) return "";
  if (target.kind === "field") return target.name ?? target.key ?? target.id;
  return builtin[target.id] ?? target.id;
}

/** Sample values of a column from the preview rows (non-blank, at most n). */
export function sampleValues(rows: { cells: string[] }[], column: number, n = 3): string[] {
  const out: string[] = [];
  for (const r of rows) {
    const v = r.cells[column]?.trim();
    if (v) out.push(v);
    if (out.length >= n) break;
  }
  return out;
}
