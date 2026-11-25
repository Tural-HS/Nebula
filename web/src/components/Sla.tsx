import type { ItemSla } from "../api/types";
import { tsl } from "../i18n-sla";
import { cn } from "../lib/cn";
import { SLA_TONE, slaDetails, slaLabel } from "../lib/sla";

/** Compact SLA badge for the table's signal column (ADR-0045). */
export function SlaSignal({ sla, className }: { sla: ItemSla; className?: string }) {
  const label = slaLabel(sla);
  return (
    <span
      role="img"
      title={label}
      aria-label={label}
      data-testid="sla-badge"
      data-state={sla.state}
      className={cn("inline-flex h-5 items-center justify-center rounded px-1 text-2xs leading-none whitespace-nowrap ring-1", SLA_TONE[sla.state], className)}
    >
      {tsl.short[sla.state]}
    </span>
  );
}

/** The drawer's SLA line: the state in words and every target's due. */
export function SlaPanel({ sla }: { sla: ItemSla }) {
  return (
    <div data-testid="sla-panel" className="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs">
      <span className={cn("inline-flex items-center rounded px-1.5 py-0.5 font-medium ring-1", SLA_TONE[sla.state])}>{tsl.state[sla.state]}</span>
      {slaDetails(sla).map((d) => (
        <span key={d} className="text-zinc-600 dark:text-zinc-400">{d}</span>
      ))}
    </div>
  );
}
