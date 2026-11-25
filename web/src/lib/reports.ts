// Reports page (ADR-0041): date presets in the workspace's time zone, the
// URL state of the filters, the API query and the drill-down links into a
// project's item list. Pure functions, unit tested.

import type { ReportAssignee, ReportStatusCount } from "../api/reports";
import type { StatusCategory } from "../api/types";

export const PRESETS = ["this_month", "last_month", "last_30", "last_90", "this_quarter", "custom"] as const;
export type Preset = (typeof PRESETS)[number];
export const DEFAULT_PRESET: Preset = "this_month";

/** The longest range the server accepts (store.MaxReportRangeDays). */
export const MAX_RANGE_DAYS = 731;

export interface ReportFilters {
  preset: Preset;
  from: string; // YYYY-MM-DD, inclusive
  to: string;
  projects: string[]; // sorted keys; [] = all visible
  assignee: string; // "", "me", "none" or a user id
  type: string; // item type key; "" = all
}

const DATE = /^(\d{4})-(\d{2})-(\d{2})$/;
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
const PROJECT_KEY = /^[A-Z][A-Z0-9]{1,9}$/;
const TYPE_KEY = /^[a-z][a-z0-9_]{0,62}$/;

/** Today's date in an IANA time zone (the workspace's), as YYYY-MM-DD. */
export function todayIn(timezone: string, now: Date = new Date()): string {
  try {
    // en-CA formats as YYYY-MM-DD.
    return new Intl.DateTimeFormat("en-CA", { timeZone: timezone, year: "numeric", month: "2-digit", day: "2-digit" }).format(now);
  } catch {
    return now.toISOString().slice(0, 10);
  }
}

const parse = (d: string): Date | null => {
  const m = DATE.exec(d);
  if (!m) return null;
  const t = new Date(Date.UTC(Number(m[1]), Number(m[2]) - 1, Number(m[3])));
  return t.toISOString().slice(0, 10) === d ? t : null; // rejects 2026-02-30
};
const fmt = (t: Date) => t.toISOString().slice(0, 10);
const addDays = (d: string, n: number) => {
  const t = parse(d);
  if (!t) return d;
  t.setUTCDate(t.getUTCDate() + n);
  return fmt(t);
};

export const isDate = (d: string) => parse(d) !== null;

/** Days from..to inclusive (0 when either is not a date). */
export function rangeDays(from: string, to: string): number {
  const f = parse(from);
  const t = parse(to);
  if (!f || !t) return 0;
  return Math.round((t.getTime() - f.getTime()) / 86_400_000) + 1;
}

/** The dates of a preset, relative to the workspace's today. */
export function presetRange(preset: Exclude<Preset, "custom">, today: string): { from: string; to: string } {
  const t = parse(today) ?? new Date();
  const y = t.getUTCFullYear();
  const m = t.getUTCMonth();
  switch (preset) {
    case "this_month":
      return { from: fmt(new Date(Date.UTC(y, m, 1))), to: today };
    case "last_month":
      return { from: fmt(new Date(Date.UTC(y, m - 1, 1))), to: fmt(new Date(Date.UTC(y, m, 0))) };
    case "last_30":
      return { from: addDays(today, -29), to: today };
    case "last_90":
      return { from: addDays(today, -89), to: today };
    case "this_quarter":
      return { from: fmt(new Date(Date.UTC(y, m - (m % 3), 1))), to: today };
  }
}

export type RangeError = "invalid" | "order" | "tooLong";

/** Why a custom range would be refused, or null when it is fine. */
export function rangeError(from: string, to: string): RangeError | null {
  if (!isDate(from) || !isDate(to)) return "invalid";
  if (to < from) return "order";
  if (rangeDays(from, to) > MAX_RANGE_DAYS) return "tooLong";
  return null;
}

const splitList = (v: string | null) => (v ? v.split(",").map((s) => s.trim()).filter(Boolean) : []);

/** Reads the filters from the URL; anything malformed falls back to the default. */
export function readFilters(p: URLSearchParams, today: string): ReportFilters {
  const projects = [...new Set(splitList(p.get("projects")).filter((k) => PROJECT_KEY.test(k)))].sort().slice(0, 50);
  const a = p.get("assignee") ?? "";
  const assignee = a === "me" || a === "none" || UUID.test(a) ? a : "";
  const ty = p.get("type") ?? "";
  const type = TYPE_KEY.test(ty) ? ty : "";
  const range = p.get("range");
  let preset: Preset = PRESETS.find((x) => x === range) ?? DEFAULT_PRESET;
  let from = p.get("from") ?? "";
  let to = p.get("to") ?? "";
  if (preset === "custom" && rangeError(from, to) !== null) preset = DEFAULT_PRESET;
  if (preset !== "custom") ({ from, to } = presetRange(preset, today));
  return { preset, from, to, projects, assignee, type };
}

/** Writes the filters into the page URL (defaults are left out). */
export function writeFilters(f: ReportFilters): URLSearchParams {
  const p = new URLSearchParams();
  if (f.preset !== DEFAULT_PRESET) p.set("range", f.preset);
  if (f.preset === "custom") {
    p.set("from", f.from);
    p.set("to", f.to);
  }
  if (f.projects.length > 0) p.set("projects", [...f.projects].sort().join(","));
  if (f.assignee) p.set("assignee", f.assignee);
  if (f.type) p.set("type", f.type);
  return p;
}

/** The API query of GET /reports (dates always explicit). */
export function reportsQuery(f: ReportFilters): string {
  const p = new URLSearchParams({ from: f.from, to: f.to });
  if (f.projects.length > 0) p.set("projects", [...f.projects].sort().join(","));
  if (f.assignee) p.set("assignee", f.assignee);
  if (f.type) p.set("type", f.type);
  return p.toString();
}

/**
 * Drill-down links go to a project's item list, which can filter by status,
 * assignee, due and approval but not by item type: with a type filter the
 * list would show more than the number, so no links are offered then.
 */
export const canDrillDown = (f: ReportFilters) => f.type === "";

export interface ItemsLink {
  status?: string[];
  due?: "overdue";
  approval?: "pending";
  /** A list assignee value; defaults to the report's assignee filter. */
  assignee?: string;
}

/** /w/:ws/p/:key with the item list's own URL parameters (lib/views.ts). */
export function itemsHref(ws: string, project: string, f: ReportFilters, l: ItemsLink): string {
  const p = new URLSearchParams();
  const assignee = l.assignee ?? f.assignee;
  if (assignee) p.set("assignee", assignee);
  if (l.due) p.set("due", l.due);
  if (l.status && l.status.length > 0) p.set("status", [...new Set(l.status)].sort().join(","));
  if (l.approval) p.set("approval", l.approval);
  const q = p.toString();
  return `/w/${encodeURIComponent(ws)}/p/${encodeURIComponent(project)}${q ? `?${q}` : ""}`;
}

export const OPEN_CATEGORIES: readonly StatusCategory[] = ["todo", "in_progress"];

/** Ids of the statuses in the given categories. */
export const statusIds = (statuses: readonly ReportStatusCount[], categories: readonly StatusCategory[]) =>
  statuses.filter((s) => categories.includes(s.category)).map((s) => s.id);

/** The item list's assignee value for a workload row. */
export const rowAssignee = (r: Pick<ReportAssignee, "assignee_id">) => r.assignee_id ?? "none";

export type WorkloadSort = "name" | "open" | "overdue" | "completed";

/** Sorts workload rows; numbers descend by default, names ascend; unassigned stays last on name sort. */
export function sortWorkload(rows: readonly ReportAssignee[], key: WorkloadSort, asc: boolean, unassigned: string, locale: string): ReportAssignee[] {
  const name = (r: ReportAssignee) => r.name ?? unassigned;
  const coll = new Intl.Collator(locale);
  return [...rows].sort((a, b) => {
    let d: number;
    if (key === "name") {
      if ((a.assignee_id === null) !== (b.assignee_id === null)) return a.assignee_id === null ? 1 : -1;
      d = coll.compare(name(a), name(b));
    } else {
      d = a[key] - b[key];
    }
    if (d === 0) d = coll.compare(name(a), name(b));
    return asc ? d : -d;
  });
}

/** A duration in the largest sensible unit: minutes under 2 h, hours under 48 h, else days. */
export function durationParts(seconds: number): { unit: "minutes" | "hours" | "days"; value: number } {
  if (seconds < 7200) return { unit: "minutes", value: Math.max(0, Math.round(seconds / 60)) };
  if (seconds < 48 * 3600) return { unit: "hours", value: Math.round(seconds / 360) / 10 };
  return { unit: "days", value: Math.round(seconds / 8640) / 10 };
}

/** Ticks for a count axis: 0, a round step, up to at least max. */
export function niceTicks(max: number, count = 4): number[] {
  if (max <= 0) return [0];
  const raw = max / count;
  const mag = 10 ** Math.floor(Math.log10(raw));
  const step = Math.max(1, [1, 2, 5, 10].map((m) => m * mag).find((s) => s >= raw) ?? 10 * mag);
  const out: number[] = [];
  for (let v = 0; v < max + step; v += step) out.push(v);
  return out;
}
