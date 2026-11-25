import { useId } from "react";

import type { FieldDefinition, FieldValue, Member } from "../api/types";
import { tlg } from "../i18n-ledger";
import { isAmount } from "../lib/formula";
import { isPhoneField, telHref } from "../lib/phone";
import { formatFieldValue } from "../lib/views";
import { cn } from "../lib/cn";

/** Field types shown in the card; long text stays in the field list (it would crowd the card). */
const SKIP = new Set<FieldDefinition["type"]>(["long_text"]);

/**
 * The record's filled fields, large and read-only, at the top of the item
 * panel: on imported ledgers (a customer card) these are what people open it
 * for. Editing stays in the field list beside it.
 */
export function KeyFacts({ keys, defs, values, members }: {
  keys: readonly string[];
  defs: ReadonlyMap<string, FieldDefinition>;
  values: Record<string, FieldValue>;
  members: ReadonlyMap<string, Member>;
}) {
  const headingId = useId();
  const facts = keys.flatMap((k) => {
    const def = defs.get(k);
    const v = values[k];
    if (!def || def.archived || SKIP.has(def.type) || v === null || v === undefined || v === "" || v === false) return [];
    const text = formatFieldValue(def, v, members);
    return text ? [{ key: k, def, text, tel: isPhoneField(def) ? telHref(v) : null }] : [];
  });
  if (facts.length === 0) return null;
  return (
    <section aria-labelledby={headingId} className="rounded-lg border border-zinc-200 p-3 dark:border-zinc-800">
      <h3 id={headingId} className="mb-2 text-xs font-semibold text-zinc-500 dark:text-zinc-400">{tlg.keyFacts}</h3>
      <dl className="grid grid-cols-2 gap-x-4 gap-y-3 @lg:grid-cols-3 @3xl:grid-cols-4">
        {facts.map((f) => (
          <div key={f.key} className="min-w-0">
            <dt className="truncate text-xs text-zinc-500 dark:text-zinc-400" title={f.def.name}>{f.def.name}</dt>
            <dd className={cn("truncate text-base font-medium", isAmount(f.def) && "tabular-nums")} title={f.text}>
              {f.tel ? (
                <a href={f.tel} className="text-brand-700 hover:underline dark:text-brand-300">{f.text}</a>
              ) : (
                f.text
              )}
            </dd>
          </div>
        ))}
      </dl>
    </section>
  );
}
