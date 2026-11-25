// Archive, restore and delete rules for projects and workspaces (ADR-0039).
// The API enforces all of this; the UI only avoids offering dead ends.
import { ApiError } from "../api/client";
import { roleAtLeast, type Role } from "../api/types";

/** A typed confirmation matches the expected key or slug (surrounding spaces ignored). */
export function confirmMatches(typed: string, expected: string, caseInsensitive = false): boolean {
  const a = typed.trim();
  if (a === "" || expected === "") return false;
  return caseInsensitive ? a.toLowerCase() === expected.toLowerCase() : a === expected;
}

export interface LifecycleActions {
  archive: boolean;
  restore: boolean;
  delete: boolean;
}

/** Workspace: the owner archives and deletes (archived only); admins restore. */
export function workspaceActions(role: Role | undefined, archived: boolean): LifecycleActions {
  const owner = roleAtLeast(role, "owner");
  return { archive: owner && !archived, restore: roleAtLeast(role, "admin") && archived, delete: owner && archived };
}

/** Project: admins archive and restore; only the owner deletes (archived only). */
export function projectActions(role: Role | undefined, archived: boolean): LifecycleActions {
  const admin = roleAtLeast(role, "admin");
  return { archive: admin && !archived, restore: admin && archived, delete: roleAtLeast(role, "owner") && archived };
}

/** Why a project the URL names is not among the active ones. */
export type ProjectGone = "archived" | "deleting" | "missing";

export function projectGone(err: unknown): ProjectGone {
  if (err instanceof ApiError && err.status === 410) {
    if (err.code === "project_archived") return "archived";
    if (err.code === "project_deleting") return "deleting";
  }
  return "missing";
}

/** The answer of a DELETE: finished now, or continued by the worker. */
export function deleteOutcome(body: unknown): "deleted" | "deleting" {
  return typeof body === "object" && body !== null && (body as { status?: unknown }).status === "deleted" ? "deleted" : "deleting";
}

/** The local calendar date of an archived_at timestamp, as YYYY-MM-DD (for formatDate). */
export function archivedDate(iso: string | null | undefined): string | null {
  if (!iso) return null;
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return null;
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${String(d.getFullYear())}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}
