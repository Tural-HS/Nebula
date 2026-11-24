import { useEffect, useId, useMemo, useRef, useState, type KeyboardEvent } from "react";
import { useNavigate, useParams } from "react-router";

import { ApiError } from "../api/client";
import { useSearch, type SearchHit, type Segment } from "../api/search";
import { t } from "../i18n";
import { ts } from "../i18n-search";
import { cn } from "../lib/cn";
import { codePoints, groupHits, hitHref, hitId, isSearchable, isSearchShortcut, moveActive, SEARCH_MAX } from "../lib/search";
import { ErrorState, Spinner, StatusBadge, touchField } from "./ui";

const DEBOUNCE_MS = 250;
const isMac = typeof navigator !== "undefined" && /Mac|iPhone|iPad/.test(navigator.platform);

function useDebounced<T>(value: T, ms: number): T {
  const [v, setV] = useState(value);
  useEffect(() => {
    const id = setTimeout(() => { setV(value); }, ms);
    return () => { clearTimeout(id); };
  }, [value, ms]);
  return v;
}

/**
 * Workspace-wide search (ADR-0024): a header button and Cmd/Ctrl+K open a
 * dialog with a combobox; results are grouped into items and comments and
 * open the item drawer. Rendered only inside a workspace.
 */
export function GlobalSearch() {
  const { ws } = useParams<{ ws: string }>();
  const [open, setOpen] = useState(false);
  const buttonRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    if (!ws) return;
    const onKey = (e: globalThis.KeyboardEvent) => {
      if (isSearchShortcut(e)) {
        e.preventDefault();
        setOpen(true);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => { window.removeEventListener("keydown", onKey); };
  }, [ws]);

  if (!ws) return null;
  return (
    <>
      <button
        ref={buttonRef}
        type="button"
        onClick={() => { setOpen(true); }}
        aria-haspopup="dialog"
        aria-keyshortcuts="Meta+K Control+K"
        aria-label={ts.open}
        className="inline-flex h-8 items-center gap-2 rounded-md px-2 text-sm text-zinc-500 dark:text-zinc-400 ring-1 ring-zinc-200 hover:bg-zinc-50 hover:text-zinc-800 focus-visible:ring-2 focus-visible:ring-brand-500 focus-visible:outline-none pointer-coarse:min-h-11 pointer-coarse:min-w-11 max-sm:justify-center max-sm:ring-0 dark:ring-zinc-700 dark:hover:bg-zinc-800 dark:hover:text-zinc-100"
      >
        <SearchIcon />
        <span className="max-sm:hidden">{ts.open}</span>
        <kbd className="rounded bg-zinc-100 px-1 font-sans text-2xs text-zinc-500 dark:text-zinc-400 max-lg:hidden dark:bg-zinc-800">{ts.shortcut(isMac)}</kbd>
      </button>
      <SearchDialog
        ws={ws}
        open={open}
        onClose={() => {
          setOpen(false);
          buttonRef.current?.focus();
        }}
      />
    </>
  );
}

function SearchIcon({ className = "size-4" }: { className?: string }) {
  return (
    <svg viewBox="0 0 24 24" className={className} fill="none" stroke="currentColor" strokeWidth="1.8" aria-hidden>
      <circle cx="11" cy="11" r="7" />
      <path d="m20 20-3.5-3.5" strokeLinecap="round" />
    </svg>
  );
}

function SearchDialog({ ws, open, onClose }: { ws: string; open: boolean; onClose: () => void }) {
  const ref = useRef<HTMLDialogElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const [query, setQuery] = useState("");
  const [active, setActive] = useState(0);
  const navigate = useNavigate();
  const ids = useId();
  const titleId = `${ids}-title`;
  const listId = `${ids}-list`;

  useEffect(() => {
    const d = ref.current;
    if (!d) return;
    if (open && !d.open) {
      d.showModal();
      inputRef.current?.focus();
      inputRef.current?.select(); // reopening keeps the last query, ready to overwrite
    }
    if (!open && d.open) d.close();
  }, [open]);

  const q = useDebounced(query.trim(), DEBOUNCE_MS);
  const searchable = isSearchable(q);
  const search = useSearch(ws, q, open && searchable);
  const hits = useMemo(() => (searchable ? (search.data?.pages.flatMap((p) => p.hits) ?? []) : []), [search.data, searchable]);
  const counts = search.data?.pages[0]?.counts ?? null;
  const groups = useMemo(() => groupHits(hits), [hits]);
  const ordered = useMemo(() => [...groups.items, ...groups.comments], [groups]);
  const fuzzy = ordered.length > 0 && ordered.every((h) => h.match === "fuzzy");

  // A new result list starts at its first row.
  const [listKey, setListKey] = useState(q);
  if (listKey !== q) {
    setListKey(q);
    setActive(0);
  }
  const activeIndex = Math.min(active, Math.max(ordered.length - 1, 0));
  const activeHit = ordered[activeIndex];

  useEffect(() => {
    if (!activeHit) return;
    document.getElementById(hitId(ids, activeHit))?.scrollIntoView({ block: "nearest" });
  }, [activeHit, ids]);

  const openHit = (h: SearchHit) => {
    onClose();
    void navigate(hitHref(ws, h));
  };

  const onKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    // A type="search" field clears itself on the first Escape and swallows
    // the key (Chrome), so the dialog would need two presses. Escape closes
    // at once, as the footer hint says; the query is kept for reopening.
    if (e.key === "Escape") {
      e.preventDefault();
      onClose();
      return;
    }
    if (e.key === "Enter") {
      if (activeHit) {
        e.preventDefault();
        openHit(activeHit);
      }
      return;
    }
    if (e.key !== "ArrowDown" && e.key !== "ArrowUp") return; // Home/End stay with the text field
    const next = moveActive(e.key, activeIndex, ordered.length);
    if (next !== undefined) {
      e.preventDefault();
      setActive(next);
    }
  };

  const tooLong = codePoints(query.trim()) > SEARCH_MAX;
  const rateLimited = search.error instanceof ApiError && search.error.status === 429;
  const status = !searchable
    ? ""
    : search.isPending
      ? ""
      : ordered.length === 0
        ? ts.empty
        : `${ts.items}: ${String(counts?.items ?? groups.items.length)}, ${ts.comments}: ${String(counts?.comments ?? groups.comments.length)}`;

  return (
    <dialog
      ref={ref}
      onClose={onClose}
      aria-labelledby={titleId}
      onClick={(e) => {
        if (e.target === ref.current) onClose(); // backdrop click
      }}
      className={cn(
        "mx-auto mt-[10vh] w-[calc(100%-2rem)] max-w-2xl overflow-hidden rounded-xl bg-white p-0 text-zinc-900 shadow-2xl backdrop:bg-zinc-950/40 dark:bg-zinc-900 dark:text-zinc-100",
        "max-h-[80dvh]",
        // Phones (ADR-0019): full screen, clear of notches and the home bar.
        "max-sm:m-0 max-sm:h-dvh max-sm:max-h-none max-sm:w-full max-sm:max-w-none max-sm:rounded-none",
      )}
    >
      {open && (
        <div className="flex h-full max-h-[inherit] flex-col pt-[env(safe-area-inset-top)] pb-[env(safe-area-inset-bottom)]">
          <h2 id={titleId} className="sr-only">
            {ts.dialogTitle}
          </h2>
          <div className="flex items-center gap-2 border-b border-zinc-200 px-3 focus-within:ring-2 focus-within:ring-brand-500 focus-within:ring-inset dark:border-zinc-800">
            <SearchIcon className="size-5 shrink-0 text-zinc-400" />
            <input
              ref={inputRef}
              type="search"
              role="combobox"
              aria-label={ts.inputLabel}
              aria-expanded={ordered.length > 0}
              aria-controls={listId}
              aria-autocomplete="list"
              aria-activedescendant={activeHit ? hitId(ids, activeHit) : undefined}
              aria-describedby={searchable ? undefined : `${ids}-hint`}
              autoComplete="off"
              spellCheck={false}
              enterKeyHint="search"
              maxLength={SEARCH_MAX + 50}
              placeholder={ts.placeholder}
              value={query}
              onChange={(e) => { setQuery(e.target.value); }}
              onKeyDown={onKeyDown}
              className={cn("h-12 min-w-0 flex-1 bg-transparent text-base outline-none placeholder:text-zinc-500 sm:text-sm dark:placeholder:text-zinc-400", touchField)}
            />
            {search.isFetching && searchable && <span className="size-4 shrink-0 animate-spin rounded-full motion-reduce:animate-none border-2 border-zinc-300 border-t-brand-600" aria-hidden />}
            <button
              type="button"
              onClick={onClose}
              className="shrink-0 rounded px-2 py-1 text-sm text-zinc-500 dark:text-zinc-400 hover:bg-zinc-100 pointer-coarse:min-h-11 dark:hover:bg-zinc-800"
            >
              <span className="sm:hidden">{t.close}</span>
              <kbd className="font-sans text-xs max-sm:hidden">Esc</kbd>
            </button>
          </div>

          <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain">
            {tooLong ? (
              <p className="p-4 text-sm text-red-600 dark:text-red-400">{ts.tooLong}</p>
            ) : !searchable ? (
              <p id={`${ids}-hint`} className="p-4 text-sm text-zinc-500 dark:text-zinc-400">
                {ts.hint}
              </p>
            ) : rateLimited ? (
              <p className="p-4 text-sm text-amber-700 dark:text-amber-400">{ts.rateLimited}</p>
            ) : search.isPending ? (
              <Spinner />
            ) : search.isError && hits.length === 0 ? (
              <ErrorState error={search.error} onRetry={() => void search.refetch()} />
            ) : ordered.length === 0 ? (
              <p className="p-4 text-sm text-zinc-500 dark:text-zinc-400">{ts.empty}</p>
            ) : (
              <div id={listId} role="listbox" aria-label={ts.dialogTitle} className="py-1">
                {fuzzy && <p className="px-4 pt-2 pb-1 text-xs text-zinc-500 dark:text-zinc-400">{ts.fuzzy}</p>}
                {(["items", "comments"] as const).map((g) =>
                  groups[g].length === 0 ? null : (
                    <div key={g} role="group" aria-labelledby={`${ids}-${g}`}>
                      <div id={`${ids}-${g}`} className="flex items-center justify-between px-4 pt-3 pb-1 text-xs font-semibold tracking-wide text-zinc-500 dark:text-zinc-400 uppercase">
                        <span>{g === "items" ? ts.items : ts.comments}</span>
                        <span className="font-normal normal-case">{ts.count(groups[g].length, counts ? (g === "items" ? counts.items : counts.comments) : groups[g].length)}</span>
                      </div>
                      {groups[g].map((h) => {
                        const i = ordered.indexOf(h);
                        return (
                          <HitRow
                            key={hitId(ids, h)}
                            id={hitId(ids, h)}
                            hit={h}
                            active={i === activeIndex}
                            onHover={() => { setActive(i); }}
                            onOpen={() => { openHit(h); }}
                          />
                        );
                      })}
                    </div>
                  ),
                )}
                {search.hasNextPage && (
                  <div className="px-4 py-2">
                    <button
                      type="button"
                      disabled={search.isFetchingNextPage}
                      onClick={() => void search.fetchNextPage()}
                      className="w-full rounded-md py-1.5 text-sm font-medium text-brand-700 hover:bg-brand-50 disabled:text-zinc-400 pointer-coarse:min-h-11 dark:text-brand-300 dark:hover:bg-zinc-800"
                    >
                      {ts.more}
                    </button>
                  </div>
                )}
              </div>
            )}
          </div>
          <div className="flex items-center justify-between gap-2 border-t border-zinc-200 px-4 py-2 text-xs text-zinc-500 dark:text-zinc-400 dark:border-zinc-800">
            <span role="status" aria-live="polite">
              {status}
            </span>
            <span className="pointer-coarse:hidden max-sm:hidden">{ts.navHint}</span>
          </div>
        </div>
      )}
    </dialog>
  );
}

function Highlighted({ segments }: { segments: readonly Segment[] }) {
  return (
    <>
      {segments.map((s, i) =>
        s.match ? (
          <mark key={i} className="rounded-sm bg-amber-200/70 px-px text-inherit dark:bg-amber-500/30">
            {s.text}
          </mark>
        ) : (
          <span key={i}>{s.text}</span>
        ),
      )}
    </>
  );
}

function HitRow({ id, hit, active, onHover, onOpen }: { id: string; hit: SearchHit; active: boolean; onHover: () => void; onOpen: () => void }) {
  const comment = hit.kind === "comment";
  return (
    <div
      id={id}
      role="option"
      aria-selected={active}
      data-search-hit={hit.kind}
      onMouseMove={onHover}
      onClick={onOpen}
      className={cn(
        "mx-1 cursor-pointer rounded-md px-3 py-2 pointer-coarse:min-h-11",
        active ? "bg-brand-50 dark:bg-zinc-800" : "hover:bg-zinc-50 dark:hover:bg-zinc-800/60",
      )}
    >
      <div className="flex min-w-0 items-center gap-2 text-sm">
        <span className="shrink-0 font-mono text-xs text-zinc-500 dark:text-zinc-400">{hit.item_key}</span>
        <span className={cn("min-w-0 flex-1 truncate", comment ? "text-zinc-600 dark:text-zinc-400" : "font-medium")}>
          <Highlighted segments={hit.title} />
        </span>
        {hit.match === "key" && (
          <span className="shrink-0 rounded bg-brand-100 px-1.5 text-2xs font-medium text-brand-800 dark:bg-brand-900 dark:text-brand-100">{ts.keyMatch}</span>
        )}
        {!comment && (
          <span className="shrink-0 max-sm:hidden">
            <StatusBadge name={hit.status_name} category={hit.status_category} />
          </span>
        )}
      </div>
      {comment && (
        <div className="mt-0.5 text-xs text-zinc-500 dark:text-zinc-400">{ts.commentBy(hit.author?.display_name ?? ts.someone)}</div>
      )}
      {hit.snippet && hit.snippet.length > 0 && (
        <p className="mt-0.5 line-clamp-2 text-xs text-zinc-600 dark:text-zinc-400">
          <Highlighted segments={hit.snippet} />
        </p>
      )}
    </div>
  );
}
