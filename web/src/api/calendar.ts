// Working-day calendar of a workspace (ADR-0027).
import { useMutation, useQuery, useQueryClient, type Query } from "@tanstack/react-query";

import { api, path } from "./client";

export type CalendarKind = "holiday" | "workday";

export interface CalendarDay {
  date: string; // YYYY-MM-DD
  kind: CalendarKind;
  name: string;
}

export interface WorkCalendar {
  /** 0 = Sunday … 6 = Saturday, Monday first. */
  weekend_days: number[];
  from: string;
  to: string;
  days: CalendarDay[];
  /** Years whose full official Azerbaijani calendar ships with this version (ADR-0027). */
  az_official_years: number[];
}

/** Same limit as the server (recur.MaxDaysPerYear). */
export const MAX_DAYS_PER_YEAR = 60;
export const MAX_WEEKEND_DAYS = 3;

const base = (ws: string) => path`/workspaces/${ws}/calendar`;
const calKey = (ws: string) => ["ws", ws, "calendar"] as const;

/** The calendar between two dates (inclusive, at most 400 days). */
export const useWorkCalendar = (ws: string, from: string, to: string, enabled = true) =>
  useQuery({
    enabled: enabled && ws !== "" && from !== "" && to !== "",
    queryKey: [...calKey(ws), from, to],
    queryFn: () => api<WorkCalendar>("GET", `${base(ws)}?from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`),
    staleTime: 5 * 60_000,
  });

/** Calendar changes move the next dates of recurring rules: refresh those too. */
const affected = (ws: string) => (q: Query) => {
  const k = q.queryKey;
  return k[0] === "ws" && k[1] === ws && (k[2] === "calendar" || k.includes("recurrences"));
};

export function useCalendarApi(ws: string) {
  const qc = useQueryClient();
  const refresh = () => qc.invalidateQueries({ predicate: affected(ws) });
  return {
    setWeekend: useMutation({
      mutationFn: (days: number[]) => api<{ weekend_days: number[] }>("PUT", base(ws) + "/weekend", { weekend_days: days }),
      onSuccess: refresh,
    }),
    setDay: useMutation({
      mutationFn: (d: CalendarDay) => api<CalendarDay>("PUT", base(ws) + path`/days/${d.date}`, { kind: d.kind, name: d.name }),
      onSuccess: refresh,
    }),
    removeDay: useMutation({
      mutationFn: (date: string) => api<undefined>("DELETE", base(ws) + path`/days/${date}`),
      onSuccess: refresh,
    }),
    addAzerbaijanFixed: useMutation({
      mutationFn: (year: number) => api<{ added: CalendarDay[] }>("POST", base(ws) + "/presets/az-fixed", { year }),
      onSuccess: refresh,
    }),
  };
}
