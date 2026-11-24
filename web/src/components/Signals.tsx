import type { Item, StatusCategory } from "../api/types";
import { tai } from "../i18n-approval-inbox";
import { tap } from "../i18n-approvals";
import { tl } from "../i18n-links";
import { ts } from "../i18n-subtasks";
import { progress } from "../lib/approvals";
import { cn } from "../lib/cn";
import { DueChip } from "./ui";

// ---------------------------------------------------------------------------
// The table's "Siqnal" column (ADR-0035). Each signal has a fixed slot, so
// the same signal lines up down the page and the eye can scan one column.
// Accessible names and test ids are those of the full badges (Approvals,
// Links, Subtasks), which the board and the panel keep using.
// ---------------------------------------------------------------------------

const SLOT = "inline-flex h-5 items-center justify-center rounded text-[11px] leading-none whitespace-nowrap tabular-nums";

export function SignalCell({ item, withDue }: { item: Item; withDue: boolean }) {
  const a = item.approval;
  const p = a ? progress(a.approvals, a.min_approvals) : null;
  // Past the decision deadline (ADR-0034): red, like ApprovalBadge in the drawer.
  const overdue = Boolean(a?.overdue && a.due_on);
  const due = a?.due_on ? (overdue ? tai.overdue(a.due_on) : tai.due(a.due_on)) : "";
  const sub = item.subtasks && item.subtasks.count > 0 ? item.subtasks : null;
  return (
    <span className="flex items-center gap-1">
      <span className="w-11 shrink-0">
        {a && p && (
          <span
            title={`${tap.badge(p.done, p.min)} → ${a.to_status.name}${due ? ` · ${due}` : ""}`}
            aria-label={due ? `${tap.badge(p.done, p.min)}, ${due}` : tap.badge(p.done, p.min)}
            data-testid="approval-badge"
            data-overdue={overdue || undefined}
            className={cn(
              SLOT,
              "w-full gap-0.5 ring-1",
              overdue
                ? "bg-red-50 text-red-700 ring-red-200 dark:bg-red-950 dark:text-red-300 dark:ring-red-900"
                : "bg-amber-50 text-amber-800 ring-amber-200 dark:bg-amber-950 dark:text-amber-200 dark:ring-amber-900",
            )}
          >
            <span aria-hidden>{overdue ? "⏰" : "⏳"}</span>
            {p.done}/{p.min}
          </span>
        )}
      </span>
      <span className="w-6 shrink-0">
        {item.blocked && (
          <span
            title={tl.blockedTitle}
            aria-label={tl.blockedTitle}
            data-testid="blocked-badge"
            className={cn(SLOT, "w-full bg-red-50 text-red-700 ring-1 ring-red-200 dark:bg-red-950 dark:text-red-300 dark:ring-red-900")}
          >
            <span aria-hidden>⛔</span>
          </span>
        )}
      </span>
      <span className="w-10 shrink-0">
        {sub && (
          <span
            title={ts.badge(sub.done, sub.count)}
            aria-label={ts.badge(sub.done, sub.count)}
            className={cn(
              SLOT,
              "w-full",
              sub.done === sub.count ? "text-emerald-700 dark:text-emerald-400" : "text-zinc-500 dark:text-zinc-400",
            )}
          >
            {sub.done}/{sub.count}
          </span>
        )}
      </span>
      {/* Without a due-date column the date shows here, as it did in the title. */}
      {withDue && item.due_date && <DueChip date={item.due_date} overdue={item.overdue} />}
    </span>
  );
}

const DOT: Record<StatusCategory, string> = {
  todo: "bg-zinc-400 dark:bg-zinc-500",
  in_progress: "bg-brand-500",
  done: "bg-emerald-600 dark:bg-emerald-500",
  canceled: "bg-zinc-300 dark:bg-zinc-600",
};

/** A quieter status for dense rows: a square in the category colour and the name. */
export function StatusText({ name, category }: { name: string; category: StatusCategory }) {
  return (
    <span className={cn("flex min-w-0 items-center gap-1.5", category === "canceled" && "text-zinc-500 line-through")}>
      <span aria-hidden className={cn("size-2 shrink-0 rounded-[2px]", DOT[category])} />
      <span className="truncate">{name}</span>
    </span>
  );
}
