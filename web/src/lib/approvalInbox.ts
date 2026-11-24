// Pure helpers for the approval inbox, re-approval and deadlines (ADR-0034).
import type { ApprovalPolicy } from "../api/admin";
import type { InboxEntry } from "../api/approvals";

/** Whole calendar days since an ISO timestamp, never negative. */
export function ageDays(createdAt: string, now: Date = new Date()): number {
  const start = new Date(createdAt);
  const a = Date.UTC(start.getFullYear(), start.getMonth(), start.getDate());
  const b = Date.UTC(now.getFullYear(), now.getMonth(), now.getDate());
  return Math.max(0, Math.round((b - a) / 86_400_000));
}

/** Opens the item drawer scrolled to its approval panel (ADR-0031 deep link). */
export function inboxHref(ws: string, e: Pick<InboxEntry, "item">): string {
  return `/w/${encodeURIComponent(ws)}/p/${encodeURIComponent(e.item.project_key)}?item=${String(e.item.number)}&focus=approval`;
}

/** Overdue requests first (they need attention), otherwise the server's oldest-first order. */
export function overdueFirst<T extends Pick<InboxEntry, "overdue">>(list: readonly T[]): T[] {
  return [...list.filter((e) => e.overdue), ...list.filter((e) => !e.overdue)];
}

export const MAX_DUE_WORKING_DAYS = 30;

/** Parses the deadline input: "" is no deadline, else an integer 1..30; NaN = invalid. */
export function parseDue(v: string): number | null {
  const s = v.trim();
  if (s === "") return null;
  const n = Number(s);
  return Number.isInteger(n) && n >= 1 && n <= MAX_DUE_WORKING_DAYS ? n : Number.NaN;
}

/** Content tokens a policy may watch: title, description and each custom field. */
export function watchTokens(fieldKeys: readonly string[]): string[] {
  return ["title", "description", ...fieldKeys.map((k) => `fields.${k}`)];
}

/** Toggles one watched token; an empty list means "watch everything". */
export function toggleWatched(list: readonly string[], token: string, on: boolean): string[] {
  const rest = list.filter((x) => x !== token);
  return on ? [...rest, token] : rest;
}

/** The re-approval and deadline part of a policy form. */
export interface PolicyExtrasValue {
  reapprove_on_change: boolean;
  reapprove_fields: string[];
  /** The raw deadline input; parseDue turns it into the API value. */
  due: string;
}

export function policyExtrasFrom(policy: Pick<ApprovalPolicy, "reapprove_on_change" | "reapprove_fields" | "due_in_working_days"> | undefined): PolicyExtrasValue {
  return {
    reapprove_on_change: policy?.reapprove_on_change ?? false,
    reapprove_fields: policy?.reapprove_fields ?? [],
    due: policy?.due_in_working_days ? String(policy.due_in_working_days) : "",
  };
}
