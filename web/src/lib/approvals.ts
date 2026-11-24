// Pure helpers for four-eyes approvals (ADR-0026): what is locked, how a
// policy reads, and whether a policy form can ever be satisfied.
import type { ApprovalPolicy, ApprovalPolicyInput, ApproverRole } from "../api/admin";
import { ApiError } from "../api/client";
import type { Item } from "../api/types";
import { t } from "../i18n";
import { tai } from "../i18n-approval-inbox";
import { tap } from "../i18n-approvals";

export const APPROVER_ROLES: readonly ApproverRole[] = ["member", "admin", "owner"];
export const MAX_MIN_APPROVALS = 5;

/**
 * While a request is pending, everything an approver signs off on is frozen
 * (the server enforces it): status, title, description, custom fields,
 * item files and deletion. Assignee, due date and comments stay editable.
 */
export function isLocked(item: Pick<Item, "approval">): boolean {
  return Boolean(item.approval);
}

/** The policy guarding entering a status, if any. */
export function policyFor(policies: readonly ApprovalPolicy[] | undefined, statusId: string): ApprovalPolicy | undefined {
  return policies?.find((p) => p.status_id === statusId);
}

export type PolicyProblem = "needApprovers" | "tooFew" | "range";

/** Mirrors the server's checks so the form explains a 422 before sending. */
export function policyProblem(p: ApprovalPolicyInput): PolicyProblem | null {
  if (!Number.isInteger(p.min_approvals) || p.min_approvals < 1 || p.min_approvals > MAX_MIN_APPROVALS) return "range";
  const users = new Set(p.approver_user_ids);
  if (p.approver_roles.length === 0 && users.size === 0) return "needApprovers";
  if (p.approver_roles.length === 0 && users.size < p.min_approvals) return "tooFew";
  return null;
}

/** "2/3"-style progress, clamped so a stale cache never shows 3/2. */
export function progress(approvals: number, min: number): { done: number; min: number; ratio: number } {
  const done = Math.max(0, Math.min(approvals, min));
  return { done, min, ratio: min > 0 ? done / min : 0 };
}

const APPROVAL_CODES = new Set(["approval_pending", "approval_required", "self_approval", "not_an_approver", "already_decided", "approval_closed",
  "reapproval_required"]);

/** The approval error code of a failed request, or null for other errors. */
export function approvalErrorCode(err: unknown): string | null {
  return err instanceof ApiError && APPROVAL_CODES.has(err.code) ? err.code : null;
}

/** Human message for a failed approval-related call. */
export function approvalErrorText(err: unknown): string {
  const code = approvalErrorCode(err);
  const known = code ? (tap.errors[code] ?? tai.errors[code]) : undefined;
  if (known) return known;
  return err instanceof ApiError ? err.message : t.errorGeneric;
}
