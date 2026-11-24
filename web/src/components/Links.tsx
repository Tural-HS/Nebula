// Item links (ADR-0028): the drawer's "Links" section grouped by type, an
// add-link form that finds the target with the workspace search API, and the
// small "blocked" badge used by the table and board.
import { useEffect, useState } from "react";
import { Link } from "react-router";

import { ApiError } from "../api/client";
import { LINK_TYPES, useLinkMutations, useLinks, type LinkType } from "../api/links";
import { useSearch } from "../api/search";
import type { Item } from "../api/types";
import { tl } from "../i18n-links";
import { cn } from "../lib/cn";
import { groupLinks, isClosed, linkCandidates, linkedItemHref } from "../lib/links";
import { isSearchable } from "../lib/search";
import { useToast } from "./toast-context";
import { Button, ErrorState, Input, Select, Spinner, StatusBadge } from "./ui";

export function BlockedBadge({ blocked, className }: { blocked?: boolean; className?: string }) {
  if (!blocked) return null;
  return (
    <span
      title={tl.blockedTitle}
      aria-label={tl.blockedTitle}
      data-testid="blocked-badge"
      className={cn(
        "inline-flex shrink-0 items-center gap-1 rounded bg-red-50 px-1.5 py-0.5 text-[11px] whitespace-nowrap text-red-700 ring-1 ring-red-200 dark:bg-red-950 dark:text-red-300 dark:ring-red-900",
        className,
      )}
    >
      <span aria-hidden>⛔</span>
      {tl.blocked}
    </span>
  );
}

function useDebounced<T>(value: T, ms: number): T {
  const [v, setV] = useState(value);
  useEffect(() => {
    const id = setTimeout(() => { setV(value); }, ms);
    return () => { clearTimeout(id); };
  }, [value, ms]);
  return v;
}

export function LinksSection({ ws, project, item, canEdit }: { ws: string; project: string; item: Item; canEdit: boolean }) {
  const links = useLinks(ws, project, item.number);
  const { remove } = useLinkMutations(ws, project, item.number);
  const toast = useToast();
  const [adding, setAdding] = useState(false);
  const list = links.data ?? [];

  return (
    <section aria-labelledby="links-heading">
      <div className="mb-2 flex items-center justify-between gap-3">
        <h3 id="links-heading" className="text-sm font-semibold">
          {tl.section}
        </h3>
        {canEdit && !adding && (
          <Button variant="ghost" className="text-xs" onClick={() => { setAdding(true); }}>
            + {tl.add}
          </Button>
        )}
      </div>
      {links.isPending ? (
        <Spinner />
      ) : links.isError ? (
        <ErrorState error={links.error} onRetry={() => void links.refetch()} />
      ) : list.length === 0 ? (
        <p className="text-sm text-zinc-500">{tl.empty}</p>
      ) : (
        <div className="space-y-3">
          {groupLinks(list).map((g) => (
            <div key={g.type}>
              <h4 className="mb-1 text-xs font-medium text-zinc-500">{tl.types[g.type]}</h4>
              <ul className="divide-y divide-zinc-100 rounded-md ring-1 ring-zinc-200 dark:divide-zinc-800 dark:ring-zinc-800">
                {g.links.map((l) => {
                  const closed = isClosed(l.item.status.category);
                  return (
                    <li key={l.id} className="flex min-w-0 items-center gap-1 pr-1" data-testid="item-link">
                      <Link
                        to={linkedItemHref(ws, l)}
                        className="flex min-w-0 flex-1 items-center gap-2 px-3 py-2 text-sm hover:bg-zinc-50 pointer-coarse:min-h-11 dark:hover:bg-zinc-800"
                      >
                        <span className="shrink-0 font-mono text-xs text-zinc-500">{l.item.key}</span>
                        <span className={cn("min-w-0 flex-1 truncate", closed && "text-zinc-500 line-through")}>
                          {l.item.title}
                          {closed && <span className="sr-only"> ({tl.closed})</span>}
                        </span>
                        <StatusBadge name={l.item.status.name} category={l.item.status.category} />
                      </Link>
                      {canEdit && (
                        <Button
                          variant="ghost"
                          aria-label={tl.remove(l.item.key)}
                          title={tl.remove(l.item.key)}
                          disabled={remove.isPending}
                          className="min-h-11 min-w-11 shrink-0 text-zinc-500 md:min-h-0 md:min-w-0"
                          onClick={() => {
                            remove.mutate(l.id, {
                              onError: (err) => { toast("error", err instanceof ApiError ? err.message : tl.exists); },
                            });
                          }}
                        >
                          ✕
                        </Button>
                      )}
                    </li>
                  );
                })}
              </ul>
            </div>
          ))}
        </div>
      )}
      {canEdit && adding && (
        <AddLinkForm ws={ws} project={project} item={item} onDone={() => { setAdding(false); }} />
      )}
    </section>
  );
}

const MAX_CANDIDATES = 8;

function AddLinkForm({ ws, project, item, onDone }: { ws: string; project: string; item: Item; onDone: () => void }) {
  const links = useLinks(ws, project, item.number);
  const { add } = useLinkMutations(ws, project, item.number);
  const toast = useToast();
  const [type, setType] = useState<LinkType>("relates");
  const [q, setQ] = useState("");
  const debounced = useDebounced(q.trim(), 250);
  const searchable = isSearchable(debounced);
  const search = useSearch(ws, debounced, searchable);
  const hits = search.data?.pages.flatMap((p) => p.hits) ?? [];
  const candidates = linkCandidates(hits, item.key, links.data ?? [], type).slice(0, MAX_CANDIDATES);

  const pick = (key: string) => {
    add.mutate(
      { type, target: key },
      {
        onSuccess: () => {
          toast("success", tl.added);
          onDone();
        },
        onError: (err) => {
          toast("error", err instanceof ApiError && err.isConflict ? tl.exists : err instanceof ApiError ? err.message : tl.exists);
        },
      },
    );
  };

  return (
    <div className="mt-3 space-y-2 rounded-md p-3 ring-1 ring-zinc-200 dark:ring-zinc-800">
      <div className="flex flex-wrap gap-2">
        <Select
          aria-label={tl.type}
          value={type}
          onChange={(e) => { setType(e.target.value as LinkType); }}
          className="max-w-48"
        >
          {LINK_TYPES.map((x) => (
            <option key={x} value={x}>
              {tl.types[x]}
            </option>
          ))}
        </Select>
        <Input
          aria-label={tl.search}
          placeholder={tl.searchPlaceholder}
          value={q}
          maxLength={200}
          autoFocus
          onChange={(e) => { setQ(e.target.value); }}
          onKeyDown={(e) => {
            if (e.key === "Escape") {
              e.stopPropagation(); // close the form, not the drawer
              onDone();
            }
          }}
          className="min-w-0 flex-1"
        />
        <Button variant="ghost" onClick={onDone}>
          ✕
        </Button>
      </div>
      {searchable &&
        (search.isPending ? (
          <Spinner />
        ) : search.isError ? (
          <ErrorState error={search.error} />
        ) : candidates.length === 0 ? (
          <p className="text-sm text-zinc-500">{tl.noResults}</p>
        ) : (
          <ul aria-label={tl.search} className="divide-y divide-zinc-100 dark:divide-zinc-800">
            {candidates.map((h) => (
              <li key={h.item_key}>
                <button
                  type="button"
                  disabled={add.isPending}
                  onClick={() => { pick(h.item_key); }}
                  className="flex w-full min-w-0 items-center gap-2 px-2 py-2 text-left text-sm hover:bg-zinc-50 pointer-coarse:min-h-11 dark:hover:bg-zinc-800"
                >
                  <span className="shrink-0 font-mono text-xs text-zinc-500">{h.item_key}</span>
                  <span className={cn("min-w-0 flex-1 truncate", isClosed(h.status_category) && "text-zinc-500 line-through")}>
                    {h.item_title}
                  </span>
                  <StatusBadge name={h.status_name} category={h.status_category} />
                </button>
              </li>
            ))}
          </ul>
        ))}
    </div>
  );
}
