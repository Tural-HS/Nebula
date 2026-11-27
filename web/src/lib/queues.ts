import type { PageState } from "./views";

// ---------------------------------------------------------------------------
// Quick filters above the item list (ADR-0035). Each preset sets the four
// "queue" dimensions of the page state at once; search, statuses and field
// filters are kept, and the server counts each preset with them
// (GET …/items/queue-counts).
// ---------------------------------------------------------------------------

export const QUEUES = ["all", "mine", "overdue", "approval", "blocked", "week", "sla_breached", "sla_at_risk", "todo", "done", "month"] as const;

/** The counters a project shows when it has not chosen (ADR-0065). */
export const DEFAULT_QUEUES: readonly Queue[] = ["all", "mine", "overdue", "approval", "blocked", "week", "sla_breached", "sla_at_risk"];
export type Queue = (typeof QUEUES)[number];

type QueueDims = Pick<PageState, "assignee" | "due" | "approval" | "blocked"> & { sla?: PageState["sla"]; category?: PageState["category"] };

const NONE: QueueDims = { assignee: "", due: "", approval: "", blocked: false, sla: "", category: "" };

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
    case "sla_breached":
      return { ...NONE, sla: "breached" };
    case "sla_at_risk":
      return { ...NONE, sla: "at_risk" };
    case "todo":
      return { ...NONE, category: "todo" };
    case "done":
      return { ...NONE, category: "done" };
    case "month":
      // Still to do and due by the end of this month.
      return { ...NONE, due: "month", category: "todo" };
  }
}

/** The preset the page shows exactly, or null when the filters are a mix. */
export function activeQueue(s: QueueDims): Queue | null {
  return QUEUES.find((q) => {
    const p = queueState(q);
    return p.assignee === s.assignee && p.due === s.due && p.approval === s.approval && p.blocked === s.blocked && p.sla === (s.sla ?? "") && p.category === (s.category ?? "");
  }) ?? null;
}

/** SLA queues are shown only where they can be non-empty or are in use. */
export function queueVisible(q: Queue, n: number | undefined, active: boolean): boolean {
  if (q !== "sla_breached" && q !== "sla_at_risk") return true;
  return active || (n ?? 0) > 0;
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

