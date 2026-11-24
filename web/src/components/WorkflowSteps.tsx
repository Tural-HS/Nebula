import type { Item, Status } from "../api/types";
import { tlg } from "../i18n-ledger";
import { progress } from "../lib/approvals";
import { cn } from "../lib/cn";

/**
 * The item's workflow as a row of steps (ADR-0035): statuses in workflow
 * order, the current one marked, an open approval shown under it. Position
 * only: workflows may allow any transition, so earlier steps are "before",
 * not "done". In a narrow panel only the current step is named.
 */
export function WorkflowSteps({ statuses, item }: { statuses: readonly Status[]; item: Item }) {
  const at = statuses.findIndex((s) => s.id === item.status.id);
  if (at < 0 || statuses.length < 2) return null;
  const a = item.approval;
  const p = a ? progress(a.approvals, a.min_approvals) : null;
  return (
    <div>
      <ol aria-label={tlg.workflow} className="flex gap-1">
        {statuses.map((s, i) => {
          const current = i === at;
          return (
            <li key={s.id} aria-current={current ? "step" : undefined} title={s.name} className="min-w-0 flex-1">
              <span
                aria-hidden
                className={cn(
                  "block h-1.5 rounded-full",
                  current && p
                    ? "bg-[repeating-linear-gradient(90deg,var(--color-amber-500)_0_6px,var(--color-amber-300)_6px_12px)]"
                    : i <= at
                      ? "bg-brand-600 dark:bg-brand-500"
                      : "bg-zinc-200 dark:bg-zinc-700",
                )}
              />
              <span
                className={cn(
                  "mt-1 block truncate text-xs",
                  current ? "font-medium text-zinc-900 dark:text-zinc-100" : "text-zinc-500 dark:text-zinc-400 @max-xl:sr-only",
                )}
              >
                {s.name}
              </span>
            </li>
          );
        })}
      </ol>
      {a && p && <p className="mt-1 text-xs text-amber-700 dark:text-amber-300">{tlg.awaitingApproval(p.done, p.min, a.to_status.name)}</p>}
    </div>
  );
}
