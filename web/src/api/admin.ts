// Types and hooks for the workspace settings (admin) screens.
import { useMutation, useQuery, useQueryClient, type QueryKey } from "@tanstack/react-query";

import { api, path } from "./client";
import { keys } from "./hooks";
import type { WebhookFormat } from "../lib/webhooks";
import type { ActivityPage, FieldDefinition, FieldValue, FormulaPreview, ItemType, Member, Role, StatusCategory, Workspace } from "./types";

export interface WorkflowStatus {
  id: string;
  name: string;
  category: StatusCategory;
  position: number;
  is_initial: boolean;
}

export interface Transition {
  id: string;
  name: string;
  from_status_id: string | null;
  to_status_id: string;
}

export interface Workflow {
  id: string;
  name: string;
  enforce_transitions: boolean;
  statuses: WorkflowStatus[];
  transitions: Transition[];
  /** Statuses that need approval to enter (ADR-0026). */
  approval_policies?: ApprovalPolicy[];
}

export type ApproverRole = "member" | "admin" | "owner";

export interface ApprovalPolicy {
  id: string;
  status_id: string;
  min_approvals: number;
  approver_roles: ApproverRole[];
  approver_user_ids: string[];
  /** ADR-0034: changing watched content after approval needs a new approval. */
  reapprove_on_change?: boolean;
  /** "title" | "description" | "fields.<key>"; empty = all of them. */
  reapprove_fields?: string[];
  /** Decision deadline in working days (1..30), null for none. */
  due_in_working_days?: number | null;
}

export interface ApprovalPolicyInput {
  min_approvals: number;
  approver_roles: ApproverRole[];
  approver_user_ids: string[];
  /** ADR-0034; omitted = off / all content / no deadline. */
  reapprove_on_change?: boolean;
  reapprove_fields?: string[];
  due_in_working_days?: number | null;
}

export interface Webhook {
  id: string;
  /** Masked (host + last 4 characters) for the slack format: that URL is the credential. */
  url: string;
  format: WebhookFormat;
  events: string[];
  /** Projects delivered; empty = every workspace-visible project (private ones only when listed). */
  project_ids: string[];
  active: boolean;
  failure_count: number;
  disabled_reason: string | null;
  created_at: string;
}

export interface Invitation {
  id: string;
  email: string;
  role: Role;
  invited_by: string | null;
  expires_at: string;
  created_at: string;
}

export const useInvitations = (ws: string, enabled: boolean) =>
  useQuery({
    enabled,
    queryKey: ["ws", ws, "invitations"],
    queryFn: async () => (await api<{ invitations: Invitation[] }>("GET", ws$(ws) + "/invitations")).invitations,
  });

export interface Delivery {
  id: number;
  event_id: number;
  attempt: number;
  status_code: number | null;
  error: string | null;
  duration_ms: number;
  delivered_at: string;
}

export type ConditionOp = "eq" | "neq" | "in" | "not_in" | "contains" | "is_empty" | "is_not_empty" | "includes";

export interface Condition {
  field: string;
  op: ConditionOp;
  value?: unknown;
}

export interface RuleAction {
  type: "set_status" | "set_assignee" | "set_field" | "send_email" | "copy_fields";
  status_id?: string;
  user_id?: string | null;
  key?: string;
  value?: unknown;
  to?: string[];
  subject?: string;
  body?: string;
  /** copy_fields (ADR-0056): target project key, match field, copied fields. */
  project?: string;
  match?: string;
  fields?: string[];
  /** copy_fields: create the target item when none matches. */
  create?: boolean;
}

export interface Rule {
  id: string;
  name: string;
  trigger: "item.created" | "item.updated" | "item.overdue";
  conditions: Condition[];
  actions: RuleAction[];
  enabled: boolean;
  updated_at: string;
}

export const adminKeys = {
  workflows: (ws: string) => ["ws", ws, "workflows"] as const,
  webhooks: (ws: string) => ["ws", ws, "webhooks"] as const,
  deliveries: (ws: string, id: string) => ["ws", ws, "webhooks", id, "deliveries"] as const,
  rules: (ws: string) => ["ws", ws, "automations"] as const,
  audit: (ws: string, action: string) => ["ws", ws, "audit", action] as const,
};

const ws$ = (ws: string) => path`/workspaces/${ws}`;

export const useWorkflows = (ws: string) =>
  useQuery({ queryKey: adminKeys.workflows(ws), queryFn: async () => (await api<{ workflows: Workflow[] }>("GET", ws$(ws) + "/workflows")).workflows });

export const useWebhooks = (ws: string) =>
  useQuery({ queryKey: adminKeys.webhooks(ws), queryFn: async () => (await api<{ webhooks: Webhook[] }>("GET", ws$(ws) + "/webhooks")).webhooks });

export const useDeliveries = (ws: string, id: string | null) =>
  useQuery({
    queryKey: adminKeys.deliveries(ws, id ?? ""),
    queryFn: async () => (await api<{ deliveries: Delivery[] }>("GET", ws$(ws) + path`/webhooks/${id ?? ""}/deliveries?limit=50`)).deliveries,
    enabled: id !== null,
    refetchInterval: 3000, // deliveries are asynchronous; keep the log fresh while open
  });

export const useRules = (ws: string) =>
  useQuery({ queryKey: adminKeys.rules(ws), queryFn: async () => (await api<{ automations: Rule[] }>("GET", ws$(ws) + "/automations")).automations });

export const useAuditLog = (ws: string, action: string, before: number) =>
  useQuery({
    queryKey: [...adminKeys.audit(ws, action), before],
    queryFn: () => {
      const q = new URLSearchParams({ limit: "50" });
      if (action) q.set("action", action);
      if (before) q.set("before", String(before));
      return api<ActivityPage>("GET", ws$(ws) + "/audit-log?" + q.toString());
    },
  });

/** A mutation that invalidates the given queries on success. */
export function useAdminMutation<V, R>(fn: (vars: V) => Promise<R>, invalidate: QueryKey[]) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: fn,
    onSuccess: () => {
      for (const k of invalidate) void qc.invalidateQueries({ queryKey: k });
    },
  });
}

/** All admin write operations, each invalidating what it affects. */
export function useAdminApi(ws: string) {
  const base = ws$(ws);
  const wf = adminKeys.workflows(ws);
  const types = keys.itemTypes(ws);
  const fields = keys.fields(ws);
  const members = keys.members(ws);
  const hooks = adminKeys.webhooks(ws);
  const rules = adminKeys.rules(ws);
  return {
    updateWorkspace: useAdminMutation(
      (b: { name?: string; timezone?: string; overdue_digest_hour?: number | null; accent?: string | null; home_projects?: string[] | null }) => api<Workspace>("PATCH", base, b),
      [keys.workspace(ws), keys.workspaces, ["home"]],
    ),
    addMember: useAdminMutation((b: { email: string; role: Role }) => api<Member>("POST", base + "/members", b), [members]),
    setRole: useAdminMutation((v: { id: string; role: Role }) => api<Member>("PATCH", base + path`/members/${v.id}`, { role: v.role }), [members]),
    removeMember: useAdminMutation((id: string) => api<undefined>("DELETE", base + path`/members/${id}`), [members]),
    invite: useAdminMutation((b: { email: string; role: Role }) => api<Invitation>("POST", base + "/invitations", b), [["ws", ws, "invitations"]]),
    resendInvite: useAdminMutation((id: string) => api<undefined>("POST", base + path`/invitations/${id}/resend`), [["ws", ws, "invitations"]]),
    revokeInvite: useAdminMutation((id: string) => api<undefined>("DELETE", base + path`/invitations/${id}`), [["ws", ws, "invitations"]]),

    createField: useAdminMutation(
      (b: { key: string; name: string; type: string; options?: { choices: { label: string }[] } | { formula: FormulaInput } | { money: MoneyInput } | { unit: string } }) => api<FieldDefinition>("POST", base + "/fields", b),
      [fields],
    ),
    updateField: useAdminMutation(
      (v: { key: string; body: { name?: string; archived?: boolean; options?: { choices: { id?: string; label: string; archived?: boolean }[] } | { formula: FormulaInput } | { money: MoneyInput } | { unit: string } } }) =>
        api<FieldDefinition>("PATCH", base + path`/fields/${v.key}`, v.body),
      [fields, types],
    ),

    createType: useAdminMutation(
      (b: { key: string; name: string; icon?: string | null; workflow_id: string }) => api<ItemType>("POST", base + "/item-types", b),
      [types],
    ),
    updateType: useAdminMutation(
      (v: { key: string; body: { name?: string; icon?: string | null } }) => api<ItemType>("PATCH", base + path`/item-types/${v.key}`, v.body),
      [types],
    ),
    archiveType: useAdminMutation((key: string) => api<undefined>("DELETE", base + path`/item-types/${key}`), [types]),
    bindField: useAdminMutation(
      (v: { type: string; field: string; required: boolean; position: number }) =>
        api<unknown>("PUT", base + path`/item-types/${v.type}/fields/${v.field}`, { required: v.required, position: v.position }),
      [types],
    ),
    unbindField: useAdminMutation(
      (v: { type: string; field: string }) => api<undefined>("DELETE", base + path`/item-types/${v.type}/fields/${v.field}`),
      [types],
    ),

    createWorkflow: useAdminMutation(
      (b: { name: string; statuses: { name: string; category: StatusCategory }[] }) => api<Workflow>("POST", base + "/workflows", b),
      [wf],
    ),
    updateWorkflow: useAdminMutation(
      (v: { id: string; body: { name?: string; enforce_transitions?: boolean } }) => api<Workflow>("PATCH", base + path`/workflows/${v.id}`, v.body),
      [wf],
    ),
    addStatus: useAdminMutation(
      (v: { wf: string; name: string; category: StatusCategory }) =>
        api<Workflow>("POST", base + path`/workflows/${v.wf}/statuses`, { name: v.name, category: v.category }),
      [wf, types],
    ),
    updateStatus: useAdminMutation(
      (v: { wf: string; id: string; body: { name?: string; category?: StatusCategory; position?: number; is_initial?: boolean } }) =>
        api<Workflow>("PATCH", base + path`/workflows/${v.wf}/statuses/${v.id}`, v.body),
      [wf, types],
    ),
    deleteStatus: useAdminMutation(
      (v: { wf: string; id: string }) => api<Workflow>("DELETE", base + path`/workflows/${v.wf}/statuses/${v.id}`),
      [wf, types],
    ),
    addTransition: useAdminMutation(
      (v: { wf: string; name: string; from_status_id: string | null; to_status_id: string }) =>
        api<Workflow>("POST", base + path`/workflows/${v.wf}/transitions`, { name: v.name, from_status_id: v.from_status_id, to_status_id: v.to_status_id }),
      [wf],
    ),
    setApprovalPolicy: useAdminMutation(
      (v: { wf: string; status: string; body: ApprovalPolicyInput }) =>
        api<Workflow>("PUT", base + path`/workflows/${v.wf}/statuses/${v.status}/approval-policy`, v.body),
      [wf],
    ),
    deleteApprovalPolicy: useAdminMutation(
      (v: { wf: string; status: string }) => api<Workflow>("DELETE", base + path`/workflows/${v.wf}/statuses/${v.status}/approval-policy`),
      [wf],
    ),
    deleteTransition: useAdminMutation(
      (v: { wf: string; id: string }) => api<Workflow>("DELETE", base + path`/workflows/${v.wf}/transitions/${v.id}`),
      [wf],
    ),

    createWebhook: useAdminMutation(
      (b: { url: string; format: WebhookFormat; events: string[]; project_ids: string[] }) =>
        api<{ webhook: Webhook; secret: string | null }>("POST", base + "/webhooks", b),
      [hooks],
    ),
    updateWebhook: useAdminMutation(
      (v: { id: string; body: { url?: string; events?: string[]; project_ids?: string[]; active?: boolean } }) =>
        api<Webhook>("PATCH", base + path`/webhooks/${v.id}`, v.body),
      [hooks],
    ),
    deleteWebhook: useAdminMutation((id: string) => api<undefined>("DELETE", base + path`/webhooks/${id}`), [hooks]),
    pingWebhook: useAdminMutation((id: string) => api<{ event_id: number }>("POST", base + path`/webhooks/${id}/ping`), []),
    rotateSecret: useAdminMutation((id: string) => api<{ secret: string }>("POST", base + path`/webhooks/${id}/rotate-secret`), []),

    createRule: useAdminMutation((b: Omit<Rule, "id" | "updated_at">) => api<Rule>("POST", base + "/automations", b), [rules]),
    updateRule: useAdminMutation(
      (v: { id: string; body: Partial<Omit<Rule, "id" | "updated_at">> }) => api<Rule>("PATCH", base + path`/automations/${v.id}`, v.body),
      [rules],
    ),
    deleteRule: useAdminMutation((id: string) => api<undefined>("DELETE", base + path`/automations/${id}`), [rules]),
  };
}

/** The admin-editable part of a formula field (ADR-0030). */
/** options.money in a create/update request (ADR-0044); the scale is accepted only on create. */
export interface MoneyInput {
  currencies?: string[];
  default_currency?: string;
  scale?: number;
}

export interface FormulaInput {
  expression: string;
  scale?: number;
}

/**
 * Compiles a draft formula and computes it for a sample item and/or values,
 * without saving. Compile errors come back as valid=false with a position.
 */
export const previewFormula = (
  ws: string,
  body: FormulaInput & { key?: string; item?: string; values?: Record<string, FieldValue> },
) => api<FormulaPreview>("POST", path`/workspaces/${ws}/fields/formula-preview`, body);
