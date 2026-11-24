// GET /me/home: the home page "Mənim işlərim" in one request (ADR-0037).
import { keepPreviousData, useQuery } from "@tanstack/react-query";

import { api } from "./client";
import type { Role, Status } from "./types";

export const HOME_SHOWS = ["attention", "overdue", "approval", "week", "assigned"] as const;
export type HomeShow = (typeof HOME_SHOWS)[number];
export type HomeReason = Exclude<HomeShow, "attention">;

export const isHomeShow = (v: unknown): v is HomeShow => typeof v === "string" && (HOME_SHOWS as readonly string[]).includes(v);

export interface HomeCounters {
  overdue: number;
  approvals: number;
  week: number;
  assigned: number;
}

export interface HomeRef {
  slug: string;
  name: string;
  accent: string | null;
}

export interface HomeProject {
  key: string;
  name: string;
}

export interface HomeItem {
  reason: HomeReason;
  workspace: HomeRef;
  project: HomeProject;
  key: string;
  number: number;
  title: string;
  status: Status;
  due_date: string | null;
  assigned: boolean;
  /** The pending request waiting for the viewer's decision, if any. */
  approval: { requested_at: string; due_on: string | null; overdue: boolean } | null;
}

export interface HomeWorkspace {
  id: string;
  slug: string;
  name: string;
  role: Role;
  accent: string | null;
  timezone: string;
  today: string;
  projects: number;
  assigned: number;
  overdue: number;
  week: number;
  approvals: number;
  last_activity: string | null;
}

export interface HomeWatched {
  workspace: HomeRef;
  project: HomeProject;
  key: string;
  number: number;
  title: string;
  status: Status;
  updated_at: string;
}

export interface HomeRecentProject {
  workspace: HomeRef;
  project: HomeProject;
  last_activity: string;
}

export interface Home {
  counters: HomeCounters;
  show: HomeShow;
  items: HomeItem[];
  total: number;
  workspaces: HomeWorkspace[];
  watching: HomeWatched[];
  recent_projects: HomeRecentProject[];
}

/** Rows on the home page; "show all" asks for the server's maximum. */
export const HOME_LIMIT = 15;
export const HOME_MAX = 200;

export const homeKeys = { all: ["home"] as const, page: (show: HomeShow, limit: number) => ["home", show, limit] as const };

export const useHome = (show: HomeShow, limit: number) =>
  useQuery({
    queryKey: homeKeys.page(show, limit),
    queryFn: () => api<Home>("GET", `/me/home?show=${show}&limit=${String(limit)}`),
    // Switching between the summary and a full list keeps the cards on screen.
    placeholderData: keepPreviousData,
  });
