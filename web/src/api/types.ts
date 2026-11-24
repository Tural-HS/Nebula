// Types mirror the Go API (internal/store). Keep in sync when the API changes.

export type Role = "guest" | "member" | "admin" | "owner";
export type StatusCategory = "todo" | "in_progress" | "done" | "canceled";

export interface Me {
  id: string;
  email: string;
  display_name: string;
}

export interface Workspace {
  id: string;
  slug: string;
  name: string;
  role: Role;
  created_at: string;
  /** IANA zone that defines "today" for due dates and digests. */
  timezone: string;
  /** Local hour of the daily overdue digest; null = off. */
  overdue_digest_hour: number | null;
}

export interface Member {
  user_id: string;
  email: string;
  display_name: string;
  role: Role;
}

export interface Project {
  id: string;
  key: string;
  name: string;
  description: string | null;
  created_at: string;
  updated_at: string;
  /** "private": only project members and workspace admins see it (ADR-0013). */
  visibility: "workspace" | "private";
}

export interface ProjectMember {
  user_id: string;
  display_name: string;
  email: string;
  role: Role;
  added_at: string;
}

export interface Status {
  id: string;
  name: string;
  category: StatusCategory;
}

export interface ItemTypeField {
  key: string;
  required: boolean;
  position: number;
}

export interface ItemType {
  id: string;
  key: string;
  name: string;
  icon: string | null;
  workflow_id: string;
  statuses: Status[];
  fields: ItemTypeField[];
}

export type FieldType =
  | "text" | "long_text" | "number" | "date" | "datetime" | "boolean"
  | "select" | "multi_select" | "user" | "url" | "email";

export interface Choice {
  id: string;
  label: string;
  color?: string | null;
  archived?: boolean;
}

export interface FieldDefinition {
  id: string;
  key: string;
  name: string;
  type: FieldType;
  options: { choices?: Choice[] };
  archived: boolean;
}

export type FieldValue = string | number | boolean | string[] | null;

export interface Item {
  id: string;
  key: string;
  number: number;
  project_id: string;
  type: { id: string; key: string; name: string };
  status: Status;
  title: string;
  description: string | null;
  assignee_id: string | null;
  reporter_id: string | null;
  fields: Record<string, FieldValue>;
  /** YYYY-MM-DD in the workspace's time zone. */
  due_date: string | null;
  /** Past due and not done/canceled, computed by the server. */
  overdue: boolean;
  /** The parent item (ADR-0021); null for a top-level item. */
  parent?: ItemParent | null;
  /** Live subtasks; done counts done and canceled ones. */
  subtasks?: SubtaskSummary;
  /** An open item the viewer can see blocks this one (ADR-0028); list responses only. */
  blocked?: boolean;
  version: number;
  created_at: string;
  updated_at: string;
}

export interface ViewConfig {
  filters: {
    q?: string;
    assignee?: string;
    due?: "" | "overdue" | "today" | "week" | "none";
    status?: string[];
    fields?: Record<string, string>;
  };
  columns: string[];
  sort: "number" | "due";
  group_by: "status" | "category";
}

/** A saved view (ADR-0014): personal (owner only) or shared with the project. */
export interface SavedView {
  id: string;
  project_id: string;
  owner_id: string;
  name: string;
  kind: "table" | "board" | "calendar";
  shared: boolean;
  config: ViewConfig;
  /** The caller may rename, change or delete it. */
  can_edit: boolean;
  created_at: string;
  updated_at: string;
}

/** Totals of number fields over the whole filtered list; sum is an exact decimal string. */
export type FieldSums = Record<string, { sum: string; count: number }>;

export interface ItemPage {
  items: Item[];
  next_cursor: string | null;
}

export const ROLE_RANK: Record<Role, number> = { guest: 0, member: 1, admin: 2, owner: 3 };

export function roleAtLeast(role: Role | undefined, min: Role): boolean {
  return role !== undefined && ROLE_RANK[role] >= ROLE_RANK[min];
}

export interface PersonRef {
  id: string;
  display_name: string;
}

export interface Comment {
  id: string;
  item_id: string;
  author: PersonRef | null;
  body: string | null; // null when deleted
  mentions: string[];
  edited: boolean;
  deleted: boolean;
  created_at: string;
  updated_at: string;
}

export interface CommentPage {
  comments: Comment[];
  next_cursor: string | null;
}

export interface Activity {
  id: number;
  action: string;
  entity_type: string;
  entity_id: string;
  actor: PersonRef | null; // null for automation/system
  source: "user" | "automation" | "system" | "import";
  automation_rule_id: string | null;
  data: Record<string, unknown>;
  occurred_at: string;
}

export interface ActivityPage {
  activity: Activity[];
  next_before: number | null;
}

export interface ItemParent {
  key: string;
  number: number;
  title: string;
}

export interface SubtaskSummary {
  count: number;
  done: number;
}
