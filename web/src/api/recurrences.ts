import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { api, path } from "./client";
import { keys } from "./hooks";
import type { FieldValue } from "./types";

export type Freq = "daily" | "weekly" | "monthly" | "yearly";
/** What a plain date does when it lands on a non-working day (ADR-0027). */
export type Shift = "none" | "previous" | "next";

export interface ScheduleInput {
  freq: Freq;
  every: number;
  weekdays: number[]; // 0 = Sunday … 6 = Saturday
  month_day: number | null; // 1..31, -1 = last day; null with work_day
  /** N-th working day of the month 1..23, -1 = last working day (ADR-0027). */
  work_day?: number | null;
  month: number | null; // 1..12
  shift?: Shift; // default "none"; not with work_day
  start_date: string;
  end_date: string | null;
}

export interface Recurrence extends ScheduleInput {
  id: string;
  title: string;
  description: string | null;
  type: { id: string; key: string; name: string };
  assignee_id: string | null;
  fields: Record<string, FieldValue>;
  lead_days: number;
  next_due: string | null;
  upcoming: string[];
  enabled: boolean;
  last_error: string | null;
}

export interface RecurrenceInput extends ScheduleInput {
  title: string;
  description: string | null;
  type: string;
  assignee_id: string | null;
  fields: Record<string, FieldValue>;
  lead_days: number;
  enabled: boolean;
}

const base = (ws: string, project: string) => path`/workspaces/${ws}/projects/${project}/recurrences`;
const recKey = (ws: string, project: string) => [...keys.items(ws, project), "recurrences"] as const;

export const useRecurrences = (ws: string, project: string) =>
  useQuery({
    queryKey: recKey(ws, project),
    queryFn: async () => (await api<{ recurrences: Recurrence[] }>("GET", base(ws, project))).recurrences,
  });

export function useRecurrenceApi(ws: string, project: string) {
  const qc = useQueryClient();
  const refresh = () => qc.invalidateQueries({ queryKey: recKey(ws, project) });
  return {
    save: useMutation({
      mutationFn: (v: { id: string | null; body: RecurrenceInput }) =>
        v.id
          ? api<Recurrence>("PUT", base(ws, project) + path`/${v.id}`, v.body)
          : api<Recurrence>("POST", base(ws, project), v.body),
      onSuccess: refresh,
    }),
    remove: useMutation({
      mutationFn: (id: string) => api<undefined>("DELETE", base(ws, project) + path`/${id}`),
      onSuccess: refresh,
    }),
  };
}

/** Next dates of an unsaved schedule, computed by the server (one source of truth). */
export const useSchedulePreview = (ws: string, project: string, s: ScheduleInput | null) =>
  useQuery({
    enabled: s !== null,
    queryKey: [...recKey(ws, project), "preview", s],
    queryFn: async () => (await api<{ upcoming: string[] }>("POST", base(ws, project) + "/preview", s)).upcoming,
    retry: false,
    staleTime: 60_000,
  });
