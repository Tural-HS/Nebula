import type { ItemSla } from "../api/types";
import { tsl } from "../i18n-sla";
import { formatDateTime } from "./format";

// SLA badge texts (ADR-0045). The server computes the state; the badge
// spells it out (never colour alone) and lists the open targets' dues.

/** Due lines: "İlk cavab: …", "Həll: …"; met targets say so instead. */
export function slaDetails(s: ItemSla, fmt: (iso: string) => string = formatDateTime): string[] {
  const out: string[] = [];
  if (s.response_due) out.push(tsl.responseDue(s.responded_at ? tsl.answered : fmt(s.response_due)));
  if (s.resolution_due) out.push(tsl.resolutionDue(s.resolved_at ? tsl.resolved : fmt(s.resolution_due)));
  return out;
}

/** Accessible name and tooltip: the state, then the due lines. */
export function slaLabel(s: ItemSla, fmt?: (iso: string) => string): string {
  return [tsl.state[s.state], ...slaDetails(s, fmt)].join(" · ");
}

export const SLA_TONE: Record<ItemSla["state"], string> = {
  on_track: "bg-zinc-50 text-zinc-700 ring-zinc-200 dark:bg-zinc-900 dark:text-zinc-300 dark:ring-zinc-700",
  at_risk: "bg-amber-50 text-amber-800 ring-amber-200 dark:bg-amber-950 dark:text-amber-200 dark:ring-amber-900",
  breached: "bg-red-50 text-red-700 ring-red-200 dark:bg-red-950 dark:text-red-300 dark:ring-red-900",
  met: "bg-emerald-50 text-emerald-800 ring-emerald-200 dark:bg-emerald-950 dark:text-emerald-300 dark:ring-emerald-900",
};
