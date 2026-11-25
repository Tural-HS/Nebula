import { useId } from "react";

import { Input, Label, Select } from "../../components/ui";
import { tm } from "../../i18n-money";
import { parseCurrencyList } from "../../lib/money";

export interface MoneyDraft {
  currencies: string; // "AZN, USD, EUR" as typed
  default_currency: string;
  scale: number;
}

/** Allowed currencies, default currency and decimals of a money field (ADR-0044). */
export function MoneySettings({ value, error, scaleFixed, onChange }: {
  value: MoneyDraft;
  error: string;
  scaleFixed?: boolean;
  onChange: (v: MoneyDraft) => void;
}) {
  const id = useId();
  const parsed = parseCurrencyList(value.currencies);
  const codes = parsed.ok ? parsed.codes : [];
  return (
    <div className="grid gap-3 sm:grid-cols-3">
      <div>
        <Label htmlFor={`${id}-cur`}>{tm.currencies}</Label>
        <Input
          id={`${id}-cur`}
          value={value.currencies}
          aria-describedby={`${id}-hint${error ? ` ${id}-err` : ""}`}
          aria-invalid={error ? true : undefined}
          onChange={(e) => { onChange({ ...value, currencies: e.target.value }); }}
        />
        <p id={`${id}-hint`} className="mt-1 text-2xs text-zinc-500 dark:text-zinc-400">{tm.currenciesHint}</p>
        {error && <p id={`${id}-err`} role="alert" className="mt-1 text-xs text-red-600 dark:text-red-400">{error}</p>}
      </div>
      <div>
        <Label htmlFor={`${id}-def`}>{tm.defaultCurrency}</Label>
        <Select id={`${id}-def`} value={codes.includes(value.default_currency) ? value.default_currency : (codes[0] ?? "")} onChange={(e) => { onChange({ ...value, default_currency: e.target.value }); }}>
          {codes.map((c) => <option key={c} value={c}>{c}</option>)}
        </Select>
      </div>
      <div>
        <Label htmlFor={`${id}-scale`}>{tm.scale}</Label>
        <Select id={`${id}-scale`} value={String(value.scale)} disabled={scaleFixed} aria-describedby={scaleFixed ? `${id}-fixed` : undefined} onChange={(e) => { onChange({ ...value, scale: Number(e.target.value) }); }}>
          {[0, 1, 2, 3, 4].map((n) => <option key={n} value={n}>{n}</option>)}
        </Select>
        {scaleFixed && <p id={`${id}-fixed`} className="mt-1 text-2xs text-zinc-500 dark:text-zinc-400">{tm.scaleFixed}</p>}
      </div>
    </div>
  );
}
