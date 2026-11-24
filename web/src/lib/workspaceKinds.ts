// Workspace kinds (ADR-0033): what a new workspace is pre-configured with.
// The server owns the content; the UI only names the choice.
import type { Locale } from "../i18n/locale";

export const WORKSPACE_KINDS = ["general", "finance", "it", "hr"] as const;
export type WorkspaceKind = (typeof WORKSPACE_KINDS)[number];

export const isWorkspaceKind = (v: unknown): v is WorkspaceKind =>
  typeof v === "string" && (WORKSPACE_KINDS as readonly string[]).includes(v);

/** Kinds that add the Azerbaijani working-day calendar to an az workspace. */
const AZ_CALENDAR: readonly WorkspaceKind[] = ["finance", "hr"];

/** Notes shown under the choice for this kind and template language. */
export function kindNotes(kind: WorkspaceKind, template: Locale): { calendar: boolean; approval: boolean } {
  return { calendar: template === "az" && AZ_CALENDAR.includes(kind), approval: kind === "finance" };
}

export interface CreateWorkspaceBody {
  slug: string;
  name: string;
  template: Locale;
  kind?: Exclude<WorkspaceKind, "general">;
}

/** "general" is the server default and is left out, so the body stays what older servers accept. */
export function createWorkspaceBody(v: { slug: string; name: string; template: Locale; kind: WorkspaceKind }): CreateWorkspaceBody {
  const body: CreateWorkspaceBody = { slug: v.slug.trim().toLowerCase(), name: v.name.trim(), template: v.template };
  if (v.kind !== "general") body.kind = v.kind;
  return body;
}
