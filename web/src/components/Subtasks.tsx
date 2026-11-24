// Subtasks (ADR-0021): the drawer's subtask list with inline add, the parent
// breadcrumb, and the small "2/5" progress badge used by the table and board.
import { useState } from "react";
import { Link, useSearchParams } from "react-router";

import { ApiError } from "../api/client";
import { useCreateItem, useItems } from "../api/hooks";
import type { Item, ItemParent, SubtaskSummary } from "../api/types";
import { ts } from "../i18n-subtasks";
import { cn } from "../lib/cn";
import { Button, ErrorState, Input, Spinner, StatusBadge } from "./ui";
import { useToast } from "./toast-context";

/** Search string that opens item n in the drawer, keeping the page's other params. */
function useItemHref() {
  const [params] = useSearchParams();
  return (n: number) => {
    const p = new URLSearchParams(params);
    p.set("item", String(n));
    return "?" + p.toString();
  };
}

export function SubtaskBadge({ summary, className }: { summary?: SubtaskSummary; className?: string }) {
  if (!summary || summary.count === 0) return null;
  const complete = summary.done === summary.count;
  return (
    <span
      title={ts.badge(summary.done, summary.count)}
      aria-label={ts.badge(summary.done, summary.count)}
      className={cn(
        "inline-flex shrink-0 items-center gap-1 rounded px-1.5 py-0.5 text-2xs whitespace-nowrap tabular-nums ring-1",
        complete
          ? "bg-emerald-50 text-emerald-700 ring-emerald-200 dark:bg-emerald-950 dark:text-emerald-300 dark:ring-emerald-900"
          : "text-zinc-600 ring-zinc-200 dark:text-zinc-400 dark:ring-zinc-700",
        className,
      )}
    >
      <span aria-hidden>☑</span>
      {summary.done}/{summary.count}
    </span>
  );
}

export function ParentCrumb({ parent, canEdit, onDetach }: { parent: ItemParent; canEdit: boolean; onDetach: () => void }) {
  const href = useItemHref();
  return (
    <nav aria-label={ts.parent} className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1 text-sm">
      <span className="text-zinc-500 dark:text-zinc-400">{ts.parent}:</span>
      <Link
        to={{ search: href(parent.number) }}
        title={ts.parentCrumb}
        className="inline-flex min-w-0 items-center gap-1.5 rounded text-brand-700 hover:underline pointer-coarse:min-h-11 dark:text-brand-300"
      >
        <span aria-hidden>↑</span>
        <span className="font-mono text-xs">{parent.key}</span>
        <span className="truncate">{parent.title}</span>
      </Link>
      {canEdit && (
        <Button variant="ghost" className="text-xs" onClick={onDetach}>
          {ts.detach}
        </Button>
      )}
    </nav>
  );
}

export function SubtaskSection({ ws, project, item, canEdit }: { ws: string; project: string; item: Item; canEdit: boolean }) {
  const href = useItemHref();
  const toast = useToast();
  const summary = item.subtasks ?? { count: 0, done: 0 };
  const list = useItems(ws, project, { q: "", assignee: "", parent: item.number }, summary.count > 0);
  const create = useCreateItem(ws, project);
  const [title, setTitle] = useState("");

  const add = () => {
    const v = title.trim();
    if (!v) return;
    create.mutate(
      { title: v, parent: item.number },
      {
        onSuccess: () => {
          setTitle("");
          toast("success", ts.added);
        },
        onError: (err) => { toast("error", err instanceof ApiError ? err.message : ts.noNesting); },
      },
    );
  };

  const rows = list.data?.pages.flatMap((p) => p.items) ?? [];
  const pct = summary.count > 0 ? Math.round((summary.done / summary.count) * 100) : 0;

  return (
    <section aria-labelledby="subtasks-heading">
      <div className="mb-2 flex items-center justify-between gap-3">
        <h3 id="subtasks-heading" className="text-sm font-semibold">
          {ts.section}
        </h3>
        {summary.count > 0 && (
          <span className="text-xs text-zinc-500 dark:text-zinc-400 tabular-nums" data-testid="subtask-progress">
            {summary.done}/{summary.count}
          </span>
        )}
      </div>
      {summary.count > 0 && (
        <div
          role="progressbar"
          aria-label={ts.progress(summary.done, summary.count)}
          aria-valuemin={0}
          aria-valuemax={summary.count}
          aria-valuenow={summary.done}
          className="mb-3 h-1.5 overflow-hidden rounded-full bg-zinc-200 dark:bg-zinc-800"
        >
          <div className="h-full rounded-full bg-emerald-500 transition-[width]" style={{ width: `${String(pct)}%` }} />
        </div>
      )}

      {summary.count === 0 ? (
        <p className="text-sm text-zinc-500 dark:text-zinc-400">{ts.empty}</p>
      ) : list.isPending ? (
        <Spinner />
      ) : list.isError ? (
        <ErrorState error={list.error} onRetry={() => void list.refetch()} />
      ) : (
        <ul className="divide-y divide-zinc-100 rounded-md ring-1 ring-zinc-200 dark:divide-zinc-800 dark:ring-zinc-800">
          {rows.map((s) => (
            <li key={s.id}>
              <Link
                to={{ search: href(s.number) }}
                className="flex min-w-0 items-center gap-2 px-3 py-2 text-sm hover:bg-zinc-50 pointer-coarse:min-h-11 dark:hover:bg-zinc-800"
              >
                <span className="shrink-0 font-mono text-xs text-zinc-500 dark:text-zinc-400">{s.key}</span>
                <span className={cn("min-w-0 flex-1 truncate", (s.status.category === "done" || s.status.category === "canceled") && "text-zinc-500 dark:text-zinc-400 line-through")}>
                  {s.title}
                </span>
                <StatusBadge name={s.status.name} category={s.status.category} />
              </Link>
            </li>
          ))}
        </ul>
      )}
      {list.hasNextPage && (
        <Button variant="ghost" className="mt-1 text-xs" disabled={list.isFetchingNextPage} onClick={() => void list.fetchNextPage()}>
          {ts.more(summary.count - rows.length)}
        </Button>
      )}

      {canEdit && (
        <form
          className="mt-3 flex gap-2"
          onSubmit={(e) => {
            e.preventDefault();
            add();
          }}
        >
          <Input
            aria-label={ts.add}
            placeholder={ts.addPlaceholder}
            value={title}
            maxLength={500}
            onChange={(e) => { setTitle(e.target.value); }}
            className="min-w-0 flex-1"
          />
          <Button type="submit" disabled={!title.trim() || create.isPending}>
            {ts.add}
          </Button>
        </form>
      )}
    </section>
  );
}
