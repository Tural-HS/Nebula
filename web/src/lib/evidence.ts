// Audit evidence export helpers (ADR-0029). The server does every real check
// (scope, privacy, limits, signing); the UI validates the form early and
// turns the audit log's evidence.exported events into an export history.

import { ApiError } from "../api/client";
import type { Activity } from "../api/types";
import { te } from "../i18n-evidence";

export type RangeProblem = "required" | "order" | "tooLong" | null;

const DAY = 86_400_000;
const parse = (d: string) => (/^\d{4}-\d{2}-\d{2}$/.test(d) ? Date.parse(d + "T00:00:00Z") : Number.NaN);
const iso = (ms: number) => new Date(ms).toISOString().slice(0, 10);

/** The previous full calendar month: the usual unit of a monthly control review. */
export function defaultRange(today: string): { from: string; to: string } {
  const t = parse(today);
  if (Number.isNaN(t)) return { from: today, to: today };
  const d = new Date(t);
  const firstThis = Date.UTC(d.getUTCFullYear(), d.getUTCMonth(), 1);
  const firstPrev = Date.UTC(d.getUTCFullYear(), d.getUTCMonth() - 1, 1);
  return { from: iso(firstPrev), to: iso(firstThis - DAY) };
}

/** Days in an inclusive range, or NaN when a date is invalid. */
export const rangeDays = (from: string, to: string) => (parse(to) - parse(from)) / DAY + 1;

export function rangeProblem(from: string, to: string, maxDays: number): RangeProblem {
  const days = rangeDays(from, to);
  if (Number.isNaN(days)) return "required";
  if (days < 1) return "order";
  return days > maxDays ? "tooLong" : null;
}

export function rangeProblemText(p: Exclude<RangeProblem, null>, maxDays: number): string {
  return p === "tooLong" ? te.errors.tooLong(maxDays) : te.errors[p];
}

/** The file name from Content-Disposition, reduced to safe characters. */
export function fileNameFrom(disposition: string | null, fallback: string): string {
  const m = disposition ? /filename="([^"]{1,200})"/.exec(disposition) : null;
  const name = (m?.[1] ?? "").replace(/[^A-Za-z0-9._-]/g, "_");
  return name.endsWith(".zip") && !name.startsWith(".") ? name : fallback;
}

/** A user-facing message for a failed export. */
export function evidenceErrorText(err: unknown): string | null {
  if (!(err instanceof ApiError)) return null;
  switch (err.code) {
    case "export_too_large":
      return te.errors.tooLarge(err.field ?? "");
    case "rate_limited":
      return te.errors.rateLimited;
    case "export_busy":
      return te.errors.busy;
    case "evidence_disabled":
      return te.disabled;
    default:
      return err.status === 422 ? err.message : null;
  }
}

export interface ExportRecord {
  id: number;
  time: string;
  who: string;
  what: string;
  approvalsOnly: boolean;
  items: number;
  events: number;
  manifest: string;
}

const num = (v: unknown) => (typeof v === "number" ? v : 0);
const text = (v: unknown) => (typeof v === "string" ? v : "");

/** Export history rows from evidence.exported audit events (newest first, as given). */
export function exportRecords(activity: Activity[], projectKeys: ReadonlyMap<string, string>): ExportRecord[] {
  return activity
    .filter((a) => a.action === "evidence.exported")
    .map((a) => {
      const d = a.data;
      const counts = (d.counts ?? {}) as Record<string, unknown>;
      let what: string;
      if (d.kind === "item") {
        what = te.kindItem(text(d.item_key));
      } else {
        const ids = Array.isArray(d.project_ids) ? d.project_ids.filter((x): x is string => typeof x === "string") : [];
        const known = ids.map((id) => projectKeys.get(id)).filter((k): k is string => k !== undefined);
        const scope = d.all_projects === true ? te.allProjects : known.length === ids.length && known.length > 0 ? known.join(", ") : te.projectCount(ids.length);
        what = `${te.kindScope(text(d.from), text(d.to))} · ${scope}`;
      }
      return {
        id: a.id,
        time: a.occurred_at,
        who: a.actor?.display_name ?? "—",
        what,
        approvalsOnly: d.approvals_only === true,
        items: num(counts.items),
        events: num(counts.events),
        manifest: text(d.manifest_sha256),
      };
    });
}

/** The verify command an auditor runs, with the fingerprint filled in. */
export const verifyCommand = (fingerprint: string) => `nebula evidence verify --key ${fingerprint} <zip>`;
