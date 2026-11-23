// Project page state: filters, table columns, sort and board grouping. The
// URL is the single source of truth, so every state (saved view or not) is a
// shareable link; a saved view (ADR-0014) is a named preset of this state.

import type { DueFilter, ItemFilters, ItemSort } from "../api/hooks";
import type { FieldDefinition, FieldValue, Member, SavedView, ViewConfig } from "../api/types";
import { tv } from "../i18n-views";
import { formatDate, formatDateTime } from "./format";
import type { GroupBy } from "./board";

export type Mode = "table" | "board" | "calendar";
export const MODES: readonly Mode[] = ["table", "board", "calendar"];

export interface PageState {
  mode: Mode;
  q: string;
  assignee: string; // "", "me", "none" or a user id
  due: DueFilter;
  status: string[]; // sorted status ids
  fields: Record<string, string>; // field key -> exact value
  columns: string[]; // table columns in order
  sort: ItemSort;
  group: GroupBy;
}

/** Built-in table columns; custom fields are "field.<key>". */
export const BUILTIN_COLUMNS = ["key", "title", "type", "status", "assignee", "due_date", "created_at", "updated_at"] as const;
export type BuiltinColumn = (typeof BUILTIN_COLUMNS)[number];
/** The table before saved views existed. */
export const DEFAULT_COLUMNS: readonly string[] = ["key", "title", "type", "status", "assignee", "updated_at"];
export const MAX_COLUMNS = 30;
/** Number columns whose totals one request may ask for (server: store.MaxSumFields). */
export const MAX_SUM_FIELDS = 10;

const DUE: readonly DueFilter[] = ["overdue", "today", "week", "none"];
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
const FIELD_KEY = /^[a-z][a-z0-9_]{0,62}$/;
const COLUMN = /^(key|title|type|status|assignee|due_date|created_at|updated_at|field\.[a-z][a-z0-9_]{0,62})$/;

/** URL parameters that make up the page state (others, like item, are kept). */
const STATE_PARAMS = ["view", "q", "assignee", "due", "status", "cols", "sort", "group"];

export const isFieldColumn = (c: string) => c.startsWith("field.");
export const fieldKey = (c: string) => c.slice("field.".length);

const splitList = (v: string | null) => (v ? v.split(",").map((s) => s.trim()).filter(Boolean) : []);
const uniqSorted = (xs: string[]) => [...new Set(xs)].sort();

export function emptyState(): PageState {
  return { mode: "table", q: "", assignee: "", due: "", status: [], fields: {}, columns: [...DEFAULT_COLUMNS], sort: "number", group: "status" };
}

/** Reads the page state from the URL, ignoring anything malformed. */
export function readState(p: URLSearchParams): PageState {
  const s = emptyState();
  s.mode = MODES.find((m) => m !== "table" && m === p.get("view")) ?? "table";
  s.q = p.get("q") ?? "";
  const a = p.get("assignee") ?? "";
  if (a === "me" || a === "none" || UUID.test(a)) s.assignee = a;
  s.due = DUE.find((d) => d === p.get("due")) ?? "";
  s.status = uniqSorted(splitList(p.get("status")).filter((id) => UUID.test(id))).slice(0, 50);
  for (const [k, v] of p) {
    if (k.startsWith("field.") && FIELD_KEY.test(fieldKey(k)) && v !== "" && Object.keys(s.fields).length < 10) s.fields[fieldKey(k)] = v;
  }
  const cols = [...new Set(splitList(p.get("cols")).filter((c) => COLUMN.test(c)))].slice(0, MAX_COLUMNS);
  if (cols.length > 0) s.columns = cols;
  if (p.get("sort") === "due") s.sort = "due";
  if (p.get("group") === "category") s.group = "category";
  return s;
}

/** Writes the state into a copy of base, keeping unrelated parameters. */
export function writeState(s: PageState, base: URLSearchParams = new URLSearchParams()): URLSearchParams {
  const p = new URLSearchParams(base);
  for (const k of STATE_PARAMS) p.delete(k);
  for (const k of [...p.keys()]) if (k.startsWith("field.")) p.delete(k);
  if (s.mode !== "table") p.set("view", s.mode);
  if (s.q.trim()) p.set("q", s.q.trim());
  if (s.assignee) p.set("assignee", s.assignee);
  if (s.due) p.set("due", s.due);
  if (s.status.length > 0) p.set("status", uniqSorted(s.status).join(","));
  for (const k of Object.keys(s.fields).sort()) {
    const v = s.fields[k];
    if (v) p.set("field." + k, v);
  }
  if (!sameList(s.columns, DEFAULT_COLUMNS)) p.set("cols", s.columns.join(","));
  if (s.sort === "due") p.set("sort", "due");
  if (s.group === "category") p.set("group", "category");
  return p;
}

/** True when the URL carries no page state at all (a bare ?saved=<id> link). */
export function hasState(p: URLSearchParams): boolean {
  return STATE_PARAMS.some((k) => p.has(k)) || [...p.keys()].some((k) => k.startsWith("field."));
}

const sameList = (a: readonly string[], b: readonly string[]) => a.length === b.length && a.every((x, i) => x === b[i]);

/** Equal states, used to tell whether the page still shows a saved view unchanged. */
export function sameState(a: PageState, b: PageState): boolean {
  return writeState(a).toString() === writeState(b).toString();
}

export function stateFromView(v: Pick<SavedView, "kind" | "config">): PageState {
  const f = v.config.filters;
  const s = emptyState();
  s.mode = v.kind;
  s.q = f.q ?? "";
  s.assignee = f.assignee ?? "";
  s.due = f.due ?? "";
  s.status = uniqSorted(f.status ?? []);
  s.fields = { ...(f.fields ?? {}) };
  if (v.config.columns.length > 0) s.columns = [...v.config.columns];
  s.sort = v.config.sort === "due" ? "due" : "number";
  s.group = v.config.group_by === "category" ? "category" : "status";
  return s;
}

export function viewFromState(s: PageState): { kind: Mode; config: ViewConfig } {
  const filters: ViewConfig["filters"] = {};
  if (s.q.trim()) filters.q = s.q.trim();
  if (s.assignee) filters.assignee = s.assignee;
  if (s.due) filters.due = s.due;
  if (s.status.length > 0) filters.status = uniqSorted(s.status);
  if (Object.keys(s.fields).length > 0) filters.fields = { ...s.fields };
  return { kind: s.mode, config: { filters, columns: [...s.columns], sort: s.sort, group_by: s.group } };
}

/** The list filters for the API (the debounced search replaces s.q). */
export function filtersOf(s: PageState, q: string): ItemFilters {
  return { q, assignee: s.assignee, due: s.due, status: s.status, fields: s.fields, sort: s.sort };
}

/**
 * Columns the table can show: unknown or archived fields (deleted since the
 * view was saved) are dropped, and the title is always there.
 */
export function visibleColumns(columns: readonly string[], fields: ReadonlyMap<string, FieldDefinition>): string[] {
  const out = columns.filter((c) => (isFieldColumn(c) ? fields.get(fieldKey(c))?.archived === false : (BUILTIN_COLUMNS as readonly string[]).includes(c)));
  if (!out.includes("title")) out.splice(out[0] === "key" ? 1 : 0, 0, "title");
  return out;
}

export function columnHeader(col: string, fields: ReadonlyMap<string, FieldDefinition>): string {
  if (isFieldColumn(col)) return fields.get(fieldKey(col))?.name ?? fieldKey(col);
  return tv.col[col as BuiltinColumn];
}

/** Number fields among the columns: these get a server-computed total. */
export function sumFields(columns: readonly string[], fields: ReadonlyMap<string, FieldDefinition>): string[] {
  return columns
    .filter((c) => {
      const f = isFieldColumn(c) ? fields.get(fieldKey(c)) : undefined;
      return f?.type === "number" && !f.archived;
    })
    .map(fieldKey)
    .slice(0, MAX_SUM_FIELDS);
}

/** Moves a column one place left (-1) or right (+1). */
export function moveColumn(columns: readonly string[], col: string, dir: -1 | 1): string[] {
  const i = columns.indexOf(col);
  const j = i + dir;
  if (i < 0 || j < 0 || j >= columns.length) return [...columns];
  const out = [...columns];
  [out[i], out[j]] = [out[j] ?? col, out[i] ?? col];
  return out;
}

// ---------------------------------------------------------------------------
// Formatting
// ---------------------------------------------------------------------------

const NBSP = " ";

/**
 * Formats a decimal ("1234567.5", "-0.25", or a JS number) the Azerbaijani
 * way: "1 234 567,5". Works on the text, so exact server sums (numeric) are
 * never rounded through a float.
 */
export function formatDecimal(v: string | number): string {
  const s = typeof v === "number" ? (Number.isFinite(v) ? numberText(v) : "") : v.trim();
  const m = /^(-?)(\d+)(?:\.(\d+))?$/.exec(s);
  if (!m) return s || "—";
  const [, sign = "", int = "", frac] = m;
  const grouped = int.replace(/\B(?=(\d{3})+(?!\d))/g, NBSP);
  return sign + grouped + (frac ? "," + frac : "");
}

// Plain notation for every number the API accepts (|v| <= 1e15), no "1e+21".
function numberText(n: number): string {
  const s = String(n);
  if (!/e/i.test(s)) return s;
  return n.toLocaleString("en-US", { useGrouping: false, maximumFractionDigits: 20 });
}

/** A field value as text, by field type ("" when empty). */
export function formatFieldValue(def: FieldDefinition | undefined, v: FieldValue | undefined, members: ReadonlyMap<string, Member>): string {
  if (v === null || v === undefined || v === "") return "";
  if (!def) return Array.isArray(v) ? v.join(", ") : String(v);
  const label = (id: string) => def.options.choices?.find((c) => c.id === id)?.label ?? "?";
  switch (def.type) {
    case "number":
      return typeof v === "number" ? formatDecimal(v) : String(v);
    case "date":
      return typeof v === "string" ? formatDate(v) : String(v);
    case "datetime":
      return typeof v === "string" ? formatDateTime(v) : String(v);
    case "boolean":
      return v === true ? "Bəli" : v === false ? "Xeyr" : String(v);
    case "select":
      return typeof v === "string" ? label(v) : String(v);
    case "multi_select":
      return Array.isArray(v) ? v.map(label).join(", ") : String(v);
    case "user":
      return typeof v === "string" ? (members.get(v)?.display_name ?? "…") : String(v);
    default:
      return Array.isArray(v) ? v.join(", ") : String(v);
  }
}
