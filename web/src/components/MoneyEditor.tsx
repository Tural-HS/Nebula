import { useRef, useState, type FocusEvent, type KeyboardEvent } from "react";

import type { FieldDefinition, FieldValue, MoneyValue } from "../api/types";
import { tm } from "../i18n-money";
import { cn } from "../lib/cn";
import { amountDraft, amountError, isMoneyValue, moneyOptions, parseAmount } from "../lib/money";
import { Input, Select } from "./ui";

/**
 * Amount + currency editor for a money field (ADR-0044), used in the item
 * panel and inline in the table. It saves when focus leaves both controls
 * or on Enter; Escape cancels (onCancel). An amount that cannot be saved is
 * reported with onInvalid (or shown below the inputs) and nothing is sent.
 */
export function MoneyEditor({ def, value, label, id, disabled, compact, autoFocus, onCommit, onCancel, onInvalid }: {
  def: FieldDefinition;
  value: FieldValue | undefined;
  label: string;
  id?: string;
  disabled?: boolean;
  compact?: boolean;
  autoFocus?: boolean;
  onCommit: (v: MoneyValue | null) => void;
  onCancel?: () => void;
  onInvalid?: (message: string) => void;
}) {
  const opts = moneyOptions(def);
  const current = isMoneyValue(value) ? value : null;
  const initialCurrency = current?.currency ?? opts.default_currency;
  const [amount, setAmount] = useState(() => amountDraft(current));
  const [currency, setCurrency] = useState(initialCurrency);
  const [error, setError] = useState("");
  const settled = useRef(false);
  // A stored currency the field no longer allows stays selectable for this value.
  const currencies = opts.currencies.includes(initialCurrency) ? opts.currencies : [initialCurrency, ...opts.currencies];

  // The last value sent, so a blur after Enter or a currency change does not send it twice.
  const last = useRef<MoneyValue | null>(current);

  const save = () => {
    const cur = currency;
    if (amount.trim() === "") {
      if (last.current) onCommit(null);
      last.current = null;
      return;
    }
    const r = parseAmount(amount, opts.scale);
    if (!r.ok) {
      const msg = amountError(r, label, opts.scale);
      if (onInvalid) onInvalid(msg);
      else setError(msg);
      return;
    }
    setError("");
    if (r.amount !== last.current?.amount || cur !== last.current.currency) {
      last.current = { amount: r.amount, currency: cur };
      onCommit(last.current);
    }
  };
  const finish = (save_: boolean) => {
    if (settled.current) return;
    if (onCancel) {
      // Inline: the cell closes either way; an invalid amount keeps the old value.
      settled.current = true;
      if (save_) save();
      onCancel();
    } else if (save_) save();
  };
  const onBlur = (e: FocusEvent<HTMLDivElement>) => {
    if (e.currentTarget.contains(e.relatedTarget)) return; // moving between amount and currency
    finish(true);
  };
  const onKeyDown = (e: KeyboardEvent) => {
    if (e.key === "Escape" && onCancel) {
      e.preventDefault();
      e.stopPropagation();
      finish(false);
    } else if (e.key === "Enter") {
      e.preventDefault();
      if (onCancel) finish(true);
      else (e.target as HTMLElement).blur();
    }
  };
  const errId = id ? `${id}-error` : undefined;

  return (
    <div>
      <div role="group" aria-label={label} className={cn("flex min-w-0 gap-1.5", compact && "gap-1")} onBlur={onBlur} onKeyDown={onKeyDown}>
        <Input
          id={id}
          autoFocus={autoFocus}
          aria-label={tm.amountOf(label)}
          aria-invalid={error ? true : undefined}
          aria-describedby={error ? errId : undefined}
          type="text"
          inputMode="decimal"
          autoComplete="off"
          value={amount}
          disabled={disabled}
          className={cn("min-w-0 flex-1 text-right tabular-nums", compact && "h-8 py-0")}
          onChange={(e) => { setAmount(e.target.value); setError(""); }}
        />
        <Select
          aria-label={tm.currencyOf(label)}
          value={currency}
          disabled={disabled}
          className={cn("w-auto shrink-0", compact && "h-8 py-0")}
          onChange={(e) => { setCurrency(e.target.value); }}
        >
          {currencies.map((c) => <option key={c} value={c}>{c}</option>)}
        </Select>
      </div>
      {error && <p id={errId} role="alert" className="mt-1 text-xs text-red-600 dark:text-red-400">{error}</p>}
    </div>
  );
}
