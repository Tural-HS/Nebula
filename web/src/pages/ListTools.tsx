import type { QueueCounts } from "../api/hooks";
import { tlg } from "../i18n-ledger";
import { cn } from "../lib/cn";
import { activeQueue, QUEUES, queueState, queueVisible, type Queue } from "../lib/queues";
import type { FilterChip } from "../lib/filterChips";
import type { PageState } from "../lib/views";

// ---------------------------------------------------------------------------
// Quick filters with counts (ADR-0035). The strip scrolls sideways inside its
// own box, so a 375 px page never does.
// ---------------------------------------------------------------------------

const TONE: Partial<Record<Queue, string>> = {
  overdue: "text-red-700 dark:text-red-400",
  approval: "text-amber-700 dark:text-amber-400",
  sla_breached: "text-red-700 dark:text-red-400",
  sla_at_risk: "text-amber-700 dark:text-amber-400",
};

export function QueueStrip({ state, counts, onPick }: {
  state: PageState;
  counts: QueueCounts | undefined;
  onPick: (patch: Partial<PageState>) => void;
}) {
  const active = activeQueue(state);
  return (
    <div
      role="group"
      aria-label={tlg.queues}
      className="flex shrink-0 overflow-x-auto overscroll-x-contain border-b border-zinc-200 bg-white px-2 [scrollbar-width:thin] md:px-3 dark:border-zinc-800 dark:bg-zinc-900"
    >
      {QUEUES.map((q) => {
        const n = counts?.[q];
        const on = active === q;
        if (!queueVisible(q, n, on)) return null;
        return (
          <button
            key={q}
            type="button"
            aria-pressed={on}
            aria-label={n === undefined ? tlg.queue[q] : tlg.queueCount(tlg.queue[q], n)}
            onClick={() => { onPick(queueState(q)); }}
            className={cn(
              "-mb-px flex shrink-0 flex-col items-start border-b-2 px-3 pt-2 pb-1.5 text-left whitespace-nowrap hover:bg-zinc-50 pointer-coarse:min-h-11 dark:hover:bg-zinc-800/60",
              on ? "border-brand-600 dark:border-brand-400" : "border-transparent",
            )}
          >
            <span
              aria-hidden
              className={cn(
                "text-lg leading-6 font-semibold tracking-tight tabular-nums md:text-xl",
                n ? TONE[q] ?? "text-zinc-900 dark:text-zinc-50" : "text-zinc-500 dark:text-zinc-400",
              )}
            >
              {n ?? "–"}
            </span>
            <span aria-hidden className={cn("text-xs", on ? "font-medium text-zinc-900 dark:text-zinc-50" : "text-zinc-500 dark:text-zinc-400")}>
              {tlg.queue[q]}
            </span>
          </button>
        );
      })}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Active filters as removable chips, so the filter controls can live in one
// menu without hiding what is applied.
// ---------------------------------------------------------------------------

export function FilterChips({ chips, onClear, onClearAll }: {
  chips: FilterChip[];
  onClear: (patch: Partial<PageState>) => void;
  onClearAll: () => void;
}) {
  if (chips.length === 0) return null;
  return (
    <ul aria-label={tlg.activeFilters} className="flex min-w-0 items-center gap-1.5 overflow-x-auto [scrollbar-width:thin]">
      {chips.map((c) => (
        <li key={c.key} className="shrink-0">
          <button
            type="button"
            aria-label={tlg.removeFilter(c.label)}
            title={tlg.removeFilter(c.label)}
            onClick={() => { onClear(c.clear); }}
            className="inline-flex h-7 max-w-64 items-center gap-1 rounded-md bg-zinc-100 pr-1.5 pl-2 text-xs text-zinc-800 hover:bg-zinc-200 dark:bg-zinc-800 dark:text-zinc-100 dark:hover:bg-zinc-700"
          >
            <span className="truncate">{c.label}</span>
            <span aria-hidden className="text-zinc-400">✕</span>
          </button>
        </li>
      ))}
      {chips.length > 1 && (
        <li className="shrink-0">
          <button type="button" onClick={onClearAll} className="h-7 rounded-md px-2 text-xs text-zinc-500 dark:text-zinc-400 hover:text-zinc-900 dark:hover:text-zinc-100">
            {tlg.clearFilters}
          </button>
        </li>
      )}
    </ul>
  );
}
