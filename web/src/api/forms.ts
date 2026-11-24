// Intake forms and "My requests" (ADR-0032). Types mirror internal/store/forms.go.
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { ApiError, api, apiUpload, path } from "./client";
import type { Choice, FieldType, Status } from "./types";

export type FormAudience = "members" | "accounts";

export interface FormFieldDef {
  /** A custom field key, "description" or "due_date". */
  key: string;
  label: string;
  help?: string;
  required: boolean;
}

export interface Form {
  id: string;
  slug: string;
  project_key: string;
  item_type: { id: string; key: string; name: string };
  title: string;
  intro: string | null;
  title_template: string;
  fields: FormFieldDef[];
  default_assignee_id: string | null;
  initial_status_id: string | null;
  audience: FormAudience;
  enabled: boolean;
  max_files: number;
  submissions: number;
  created_at: string;
  updated_at: string;
}

/** The body of POST/PUT …/forms. */
export interface FormInput {
  slug: string;
  title: string;
  intro: string | null;
  item_type: string;
  title_template: string;
  fields: FormFieldDef[];
  default_assignee_id: string | null;
  initial_status_id: string | null;
  audience: FormAudience;
  enabled: boolean;
  max_files: number;
}

export interface PublicFormField {
  key: string;
  label: string;
  help?: string;
  required: boolean;
  type: Exclude<FieldType, "user">;
  choices?: Choice[];
}

export interface PublicForm {
  id: string;
  slug: string;
  title: string;
  intro: string | null;
  workspace: string;
  open: boolean;
  /** Why a closed form is closed (ADR-0039); absent when open or from an older API. */
  closed_reason?: "archived" | "disabled";
  max_files: number;
  fields: PublicFormField[];
  can_manage: boolean;
}

export interface Submission {
  id: string;
  key: string;
  title: string;
  created_at: string;
}

export interface MyRequest {
  id: string;
  form_title: string;
  workspace: string;
  key: string;
  title: string;
  status: Status | null;
  removed: boolean;
  created_at: string;
  updated_at: string;
  unseen: boolean;
}

export interface Answer {
  key: string;
  label: string;
  type: string;
  display: string;
}

export interface MyRequestDetail extends MyRequest {
  answers: Answer[];
  files: { filename: string; size: number }[];
  message: string | null;
  message_at: string | null;
}

export interface ItemRequest {
  id: string;
  form_id: string | null;
  form_title: string;
  requester: { id: string; display_name: string; email: string; member: boolean } | null;
  submitted_at: string;
  message: string | null;
  message_at: string | null;
  message_by: { id: string; display_name: string } | null;
}

export const formKeys = {
  list: (ws: string, project: string) => ["ws", ws, "p", project, "forms"] as const,
  public: (ref: string) => ["forms", ref] as const,
  myRequests: ["me", "requests"] as const,
  myRequest: (id: string) => ["me", "requests", id] as const,
  itemRequest: (ws: string, project: string, n: number) => ["ws", ws, "p", project, "item", n, "request"] as const,
};

const formsPath = (ws: string, project: string) => path`/workspaces/${ws}/projects/${project}/forms`;

export const useForms = (ws: string, project: string) =>
  useQuery({
    queryKey: formKeys.list(ws, project),
    queryFn: async () => (await api<{ forms: Form[] }>("GET", formsPath(ws, project))).forms,
  });

export function useSaveForm(ws: string, project: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, body }: { id: string | null; body: FormInput }) =>
      id ? api<Form>("PUT", formsPath(ws, project) + path`/${id}`, body) : api<Form>("POST", formsPath(ws, project), body),
    onSuccess: () => qc.invalidateQueries({ queryKey: formKeys.list(ws, project) }),
  });
}

export function useDeleteForm(ws: string, project: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api<undefined>("DELETE", formsPath(ws, project) + path`/${id}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: formKeys.list(ws, project) }),
  });
}

export const usePublicForm = (ref: string) =>
  useQuery({
    queryKey: formKeys.public(ref),
    queryFn: () => api<{ form: PublicForm; limits?: { max_file_bytes: number } }>("GET", path`/forms/${ref}`),
  });

/** Sends a submission: JSON without files, multipart (values + file parts) with them. */
export async function sendSubmission(ref: string, values: Record<string, unknown>, files: readonly File[]): Promise<Submission> {
  const url = path`/forms/${ref}/submissions`;
  if (files.length === 0) return api<Submission>("POST", url, { values });
  const form = new FormData();
  form.append("values", JSON.stringify(values));
  for (const f of files) form.append("file", f, f.name);
  return (await apiUpload(url, form)).body as Submission;
}

export interface MyRequestPage {
  requests: MyRequest[];
  next_cursor: string | null;
  unseen: number;
}

export const useMyRequests = () =>
  useInfiniteQuery({
    queryKey: formKeys.myRequests,
    initialPageParam: null as string | null,
    queryFn: ({ pageParam }) =>
      api<MyRequestPage>("GET", "/me/requests?limit=50" + (pageParam ? "&cursor=" + encodeURIComponent(pageParam) : "")),
    getNextPageParam: (last) => last.next_cursor,
  });

/** Unseen updates for the header badge (one small page, refreshed now and then). */
export const useMyRequestsUnseen = () =>
  useQuery({
    queryKey: [...formKeys.myRequests, "unseen"],
    queryFn: async () => (await api<MyRequestPage>("GET", "/me/requests?limit=1")).unseen,
    refetchInterval: 120_000,
    staleTime: 60_000,
  });

export const useMyRequest = (id: string) =>
  useQuery({ queryKey: formKeys.myRequest(id), queryFn: () => api<MyRequestDetail>("GET", path`/me/requests/${id}`) });

export function useMarkSeen() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api<undefined>("POST", path`/me/requests/${id}/seen`),
    onSuccess: () => qc.invalidateQueries({ queryKey: formKeys.myRequests }),
  });
}

export const useItemRequest = (ws: string, project: string, n: number, enabled: boolean) =>
  useQuery({
    enabled,
    queryKey: formKeys.itemRequest(ws, project, n),
    queryFn: async () => {
      try {
        return await api<ItemRequest>("GET", path`/workspaces/${ws}/projects/${project}/items/${n}/request`);
      } catch (e) {
        if (e instanceof ApiError && e.status === 404) return null; // not created by a form
        throw e;
      }
    },
  });

export function useSetRequestMessage(ws: string, project: string, n: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (message: string | null) =>
      api<ItemRequest>("PUT", path`/workspaces/${ws}/projects/${project}/items/${n}/request/message`, { message }),
    onSuccess: (data) => {
      qc.setQueryData(formKeys.itemRequest(ws, project, n), data);
    },
  });
}
