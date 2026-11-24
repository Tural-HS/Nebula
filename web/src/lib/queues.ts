import type { PageState } from "./views";

// ---------------------------------------------------------------------------
// Quick filters above the item list (ADR-0035). Each preset sets the four
// "queue" dimensions of the page state at once; search, statuses and field
// filters are kept, and the server counts each preset with them
// (GET …/items/queue-counts).
// ---------------------------------------------------------------------------

export const QUEUES = ["all", "mine", "overdue", "approval", "blocked", "week"] as const;
export type Queue = (typeof QUEUES)[number];

type QueueDims = Pick<PageState, "assignee" | "due" | "approval" | "blocked">;

const NONE: QueueDims = { assignee: "", due: "", approval: "", blocked: false };

export function queueState(q: Queue): QueueDims {
  switch (q) {
    case "all":
      return { ...NONE };
    case "mine":
      return { ...NONE, assignee: "me" };
    case "overdue":
      return { ...NONE, due: "overdue" };
    case "approval":
      return { ...NONE, approval: "pending" };
    case "blocked":
      return { ...NONE, blocked: true };
    case "week":
      return { ...NONE, due: "week" };
  }
}

/** The preset the page shows exactly, or null when the filters are a mix. */
export function activeQueue(s: QueueDims): Queue | null {
  return QUEUES.find((q) => {
    const p = queueState(q);
    return p.assignee === s.assignee && p.due === s.due && p.approval === s.approval && p.blocked === s.blocked;
  }) ?? null;
}

/** Severity shown by the row stripe: the most urgent signal wins. */
export type RowSignal = "overdue" | "approval" | "blocked" | null;

export function rowSignal(it: { overdue: boolean; approval?: unknown; blocked?: boolean }): RowSignal {
  if (it.overdue) return "overdue";
  if (it.approval) return "approval";
  if (it.blocked) return "blocked";
  return null;
}

/** Left edge of a row: the most urgent signal, as colour and pattern. */
export const ROW_STRIPE: Record<Exclude<RowSignal, null>, string> = {
  overdue: "bg-red-600 dark:bg-red-500",
  approval: "bg-amber-500",
  blocked: "bg-[repeating-linear-gradient(180deg,var(--color-zinc-500)_0_3px,transparent_3px_6px)]",
};

