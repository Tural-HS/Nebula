import { keepPreviousData, useInfiniteQuery } from "@tanstack/react-query";

import type { StatusCategory } from "./types";
import { api, path } from "./client";

/** Plain text piece of a highlighted string: never HTML (ADR-0024). */
export interface Segment {
  text: string;
  match?: boolean;
}

export interface SearchHit {
  kind: "item" | "comment";
  match: "key" | "text" | "fuzzy";
  project_key: string;
  project_name: string;
  item_number: number;
  item_key: string;
  item_title: string;
  status_name: string;
  status_category: StatusCategory;
  title: Segment[];
  snippet: Segment[] | null;
  comment_id: string | null;
  author: { id: string; display_name: string } | null;
  created_at: string | null;
}

export interface SearchPage {
  hits: SearchHit[];
  next_cursor: string | null;
  counts: { items: number; comments: number } | null; // first page only
}

export const SEARCH_PAGE = 20;

/** GET /workspaces/{ws}/search, paged; only runs for a searchable query. */
export const useSearch = (ws: string, q: string, enabled: boolean) =>
  useInfiniteQuery({
    enabled,
    queryKey: ["search", ws, q],
    initialPageParam: null as string | null,
    queryFn: ({ pageParam }) => {
      const p = new URLSearchParams({ q, limit: String(SEARCH_PAGE) });
      if (pageParam) p.set("cursor", pageParam);
      return api<SearchPage>("GET", path`/workspaces/${ws}/search` + "?" + p.toString());
    },
    getNextPageParam: (last) => last.next_cursor,
    placeholderData: keepPreviousData, // keep the old list while the next query loads
    staleTime: 30_000,
  });
