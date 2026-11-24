import { useEffect, useId, useRef, useState, type KeyboardEvent } from "react";
import { Link } from "react-router";

import { useMarkAllRead, useMarkRead, useNotifications, useUnreadCount, type AppNotification } from "../api/notifications";
import { cn } from "../lib/cn";
import { formatDateTime, formatRelative } from "../lib/format";
import { badgeText, notificationHref, notificationLead } from "../lib/notifications";
import { tn } from "../i18n-notifications";
import { EmptyState, ErrorState, Spinner } from "./ui";

/**
 * Header bell with the unread badge and a dropdown of the latest
 * notifications (ADR-0017). The count is polled every 30 s and on focus.
 */
export function NotificationBell() {
  const [open, setOpen] = useState(false);
  const count = useUnreadCount();
  const list = useNotifications(open);
  const markRead = useMarkRead();
  const markAll = useMarkAllRead();
  const rootRef = useRef<HTMLDivElement>(null);
  const buttonRef = useRef<HTMLButtonElement>(null);
  const listRef = useRef<HTMLUListElement>(null);
  const panelId = useId();
  const headingId = useId();

  const unread = count.data?.unread ?? 0;
  const capped = count.data?.capped ?? false;
  const badge = badgeText(unread, capped);

  // Announce changes of the count (not the first load) to screen readers.
  const [seen, setSeen] = useState<number | undefined>(undefined);
  const [announcement, setAnnouncement] = useState("");
  const current = count.data?.unread;
  if (current !== seen) {
    setSeen(current);
    if (seen !== undefined && current !== undefined) setAnnouncement(tn.liveCount(current, capped));
  }

  const close = (refocus: boolean) => {
    setOpen(false);
    if (refocus) buttonRef.current?.focus();
  };

  // Click outside closes the panel.
  useEffect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent) => {
      if (!rootRef.current?.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", onDown);
    return () => {
      document.removeEventListener("mousedown", onDown);
    };
  }, [open]);

  // Move focus into the list once it has loaded.
  const loaded = open && list.isSuccess;
  useEffect(() => {
    if (loaded) listRef.current?.querySelector<HTMLElement>("[data-notification]")?.focus();
  }, [loaded]);

  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    if (!open) return;
    if (e.key === "Escape") {
      e.preventDefault();
      close(true);
      return;
    }
    const links = Array.from(listRef.current?.querySelectorAll<HTMLElement>("[data-notification]") ?? []);
    if (links.length === 0) return;
    const i = links.indexOf(document.activeElement as HTMLElement);
    const next = { ArrowDown: i + 1, ArrowUp: i - 1, Home: 0, End: links.length - 1 }[e.key];
    if (next === undefined) return;
    e.preventDefault();
    links[(next + links.length) % links.length]?.focus();
  };

  const items = list.data?.notifications ?? [];
  return (
    <div ref={rootRef} className="relative" onKeyDown={onKeyDown}>
      <button
        ref={buttonRef}
        type="button"
        aria-label={tn.bellLabel(unread, capped)}
        aria-haspopup="dialog"
        aria-expanded={open}
        aria-controls={open ? panelId : undefined}
        onClick={() => {
          if (open) close(false);
          else {
            setOpen(true);
            void count.refetch();
          }
        }}
        className="relative inline-flex size-8 items-center justify-center rounded-md text-zinc-600 hover:bg-zinc-100 focus-visible:ring-2 focus-visible:ring-brand-500 focus-visible:outline-none dark:text-zinc-300 dark:hover:bg-zinc-800"
      >
        <svg viewBox="0 0 24 24" className="size-5" fill="none" stroke="currentColor" strokeWidth="1.8" aria-hidden>
          <path d="M6 8a6 6 0 1 1 12 0c0 7 3 9 3 9H3s3-2 3-9" strokeLinecap="round" strokeLinejoin="round" />
          <path d="M10.3 21a1.94 1.94 0 0 0 3.4 0" strokeLinecap="round" strokeLinejoin="round" />
        </svg>
        {badge && (
          <span
            aria-hidden
            className="absolute -top-0.5 -right-0.5 min-w-4 rounded-full bg-red-600 px-1 text-center text-2xs leading-4 font-semibold text-white"
          >
            {badge}
          </span>
        )}
      </button>
      <span className="sr-only" role="status" aria-live="polite">
        {announcement}
      </span>

      {open && (
        <div
          id={panelId}
          role="dialog"
          aria-labelledby={headingId}
          className="absolute top-full right-0 z-30 mt-2 w-96 max-w-[calc(100vw-2rem)] overflow-hidden rounded-lg bg-white shadow-xl ring-1 ring-zinc-200 dark:bg-zinc-900 dark:ring-zinc-700"
        >
          <div className="flex items-center justify-between border-b border-zinc-200 px-3 py-2 dark:border-zinc-800">
            <h2 id={headingId} className="text-sm font-semibold text-zinc-900 dark:text-zinc-100">
              {tn.title}
            </h2>
            <button
              type="button"
              disabled={unread === 0 || markAll.isPending}
              onClick={() => {
                markAll.mutate();
              }}
              className="rounded px-2 py-1 text-xs font-medium text-brand-700 hover:bg-brand-50 disabled:cursor-not-allowed disabled:text-zinc-400 disabled:hover:bg-transparent dark:text-brand-300 dark:hover:bg-zinc-800"
            >
              {tn.markAllRead}
            </button>
          </div>
          <div className="max-h-[70vh] overflow-y-auto">
            {list.isPending ? (
              <Spinner />
            ) : list.isError ? (
              <ErrorState error={list.error} onRetry={() => void list.refetch()} />
            ) : items.length === 0 ? (
              <EmptyState>{tn.empty}</EmptyState>
            ) : (
              <ul ref={listRef} className="divide-y divide-zinc-100 dark:divide-zinc-800">
                {items.map((n) => (
                  <NotificationRow
                    key={n.id}
                    n={n}
                    onOpen={() => {
                      if (!n.read) markRead.mutate(n.id);
                      close(false);
                    }}
                  />
                ))}
              </ul>
            )}
          </div>
        </div>
      )}
    </div>
  );
}

function NotificationRow({ n, onOpen }: { n: AppNotification; onOpen: () => void }) {
  return (
    <li>
      <Link
        to={notificationHref(n)}
        data-notification
        onClick={onOpen}
        className={cn(
          "flex gap-2 px-3 py-2.5 text-sm focus-visible:bg-zinc-100 focus-visible:-outline-offset-2 hover:bg-zinc-50 dark:hover:bg-zinc-800 dark:focus-visible:bg-zinc-800",
          !n.read && "bg-brand-50/60 dark:bg-brand-950/40",
        )}
      >
        <span
          aria-hidden
          className={cn("mt-1.5 size-2 shrink-0 rounded-full", n.read ? "bg-transparent" : "bg-brand-600")}
        />
        <span className="min-w-0 flex-1">
          <span className={cn("block text-zinc-800 dark:text-zinc-200", !n.read && "font-medium")}>
            {!n.read && <span className="sr-only">{tn.unread}: </span>}
            {notificationLead(n)}: <span className="font-mono text-xs text-zinc-500 dark:text-zinc-400">{n.item_key}</span> {n.item_title}
          </span>
          <span className="mt-0.5 block text-xs text-zinc-500 dark:text-zinc-400">
            {n.workspace_name} · <time dateTime={n.created_at} title={formatDateTime(n.created_at)}>{formatRelative(n.created_at)}</time>
          </span>
        </span>
      </Link>
    </li>
  );
}
