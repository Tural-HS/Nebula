import { useQuery } from "@tanstack/react-query";
import { useEffect, useId, useMemo, useState } from "react";

import { previewFormula, type FormulaInput } from "../../api/admin";
import type { FieldDefinition } from "../../api/types";
import { Input } from "../../components/ui";
import { tf } from "../../i18n-formulas";
import { cn } from "../../lib/cn";
import { isFormulaError, splitAt } from "../../lib/formula";
import { formatFieldValue } from "../../lib/views";

const PREVIEW_DELAY_MS = 350;
const SAMPLE_KEY = /^[A-Za-z][A-Za-z0-9]{1,9}-\d{1,12}$/;

function useDebounced(value: string, ms: number): string {
  const [v, setV] = useState(value);
  useEffect(() => {
    const id = setTimeout(() => { setV(value); }, ms);
    return () => { clearTimeout(id); };
  }, [value, ms]);
  return v;
}

interface Draft {
  expression: string;
  scale?: number;
  item: string;
}

/**
 * Formula input with live validation (ADR-0030): every pause in typing asks
 * the server to compile the draft and compute it for an optional sample
 * item. The server is the only parser, so what the editor accepts is
 * exactly what saving accepts.
 */
export function FormulaEditor({ ws, fieldKey, value, onChange, disabled }: {
  ws: string;
  /** The field being edited; omitted for a new field. */
  fieldKey?: string;
  value: FormulaInput;
  onChange: (v: FormulaInput) => void;
  disabled?: boolean;
}) {
  const id = useId();
  const [sample, setSample] = useState("");
  // Debounced as text: a new object with the same content is no change.
  const now: Draft = { expression: value.expression.trim(), scale: value.scale, item: SAMPLE_KEY.test(sample.trim()) ? sample.trim() : "" };
  const settled = useDebounced(JSON.stringify(now), PREVIEW_DELAY_MS);
  const draft = useMemo(() => JSON.parse(settled) as Draft, [settled]);
  const check = useQuery({
    queryKey: ["formula-preview", ws, fieldKey ?? "", draft],
    enabled: draft.expression !== "",
    staleTime: 30_000,
    retry: false,
    queryFn: () => previewFormula(ws, {
      expression: draft.expression,
      ...(draft.scale !== undefined ? { scale: draft.scale } : {}),
      ...(fieldKey ? { key: fieldKey } : {}),
      ...(draft.item ? { item: draft.item } : {}),
    }),
  });
  const expr = value.expression;
  const typing = JSON.stringify(now) !== settled;
  const preview = expr.trim() && !typing ? (check.data ?? null) : null;
  const failure = expr.trim() && !typing && check.error ? check.error.message : "";
  const checking = expr.trim() !== "" && (typing || check.isFetching);

  const err = preview && !preview.valid ? preview.error : undefined;
  const resultType = preview?.valid ? preview.result_type : undefined;
  const resultDef: FieldDefinition | undefined = resultType
    ? { id: "", key: "", name: "", type: "formula", archived: false, options: { formula: { expression: expr, result_type: resultType, refs: [] } } }
    : undefined;

  return (
    <div className="space-y-2">
      <label className="block text-xs text-zinc-500" htmlFor={`${id}-expr`}>
        {tf.expression} <span className="font-serif italic">ƒ</span>
      </label>
      <textarea
        id={`${id}-expr`}
        rows={2}
        required
        maxLength={1000}
        spellCheck={false}
        autoCapitalize="off"
        autoCorrect="off"
        disabled={disabled}
        value={expr}
        placeholder="amount - paid"
        aria-invalid={err ? true : undefined}
        aria-describedby={`${id}-status`}
        onChange={(e) => { onChange({ ...value, expression: e.target.value }); }}
        className={cn(
          "w-full rounded-md bg-white px-2.5 py-1.5 font-mono text-sm ring-1 ring-zinc-300 focus:ring-2 focus:ring-brand-500 dark:bg-zinc-900 dark:ring-zinc-700",
          err && "ring-red-500 dark:ring-red-500",
        )}
      />
      <p className="text-xs text-zinc-400">{tf.expressionHint}</p>
      <details className="text-xs text-zinc-500">
        <summary className="cursor-pointer select-none">{tf.helpTitle}</summary>
        <p className="mt-1">{tf.help}</p>
      </details>

      <div className="flex flex-wrap items-end gap-3">
        {resultType === "number" && (
          <label className="text-xs text-zinc-500">
            {tf.scale}
            <Input
              type="number"
              min={0}
              max={10}
              className="w-20"
              disabled={disabled}
              value={value.scale ?? 2}
              onChange={(e) => { onChange({ ...value, scale: Math.min(10, Math.max(0, Math.trunc(Number(e.target.value) || 0))) }); }}
            />
          </label>
        )}
        <label className="text-xs text-zinc-500">
          {tf.sampleItem}
          <Input value={sample} placeholder={tf.sampleItemHint} className="w-32" maxLength={24} onChange={(e) => { setSample(e.target.value); }} />
        </label>
      </div>

      <div id={`${id}-status`} role="status" aria-live="polite" className="min-h-5 text-xs" data-testid="formula-status">
        {checking ? (
          <span className="text-zinc-400">{tf.checking}</span>
        ) : failure ? (
          <span className="text-red-600 dark:text-red-400">{failure}</span>
        ) : err ? (
          <div className="space-y-1 text-red-600 dark:text-red-400">
            <p>{err.position > 0 ? tf.errorAt(err.position, err.message) : err.message}</p>
            {err.position > 0 && <ErrorMark expr={expr} position={err.position} />}
          </div>
        ) : preview?.valid && resultDef ? (
          <div className="space-y-0.5 text-zinc-600 dark:text-zinc-300">
            <p className="text-emerald-700 dark:text-emerald-400">{tf.valid}</p>
            <p>
              {tf.resultType(tf.resultTypes[resultDef.options.formula?.result_type ?? ""] ?? "")}
              {preview.refs && preview.refs.length > 0 && <> · {tf.refs(preview.refs.length)}</>}
            </p>
            {preview.item && (
              <p>
                {tf.result} ({preview.item}):{" "}
                <strong className={cn("tabular-nums", isFormulaError(preview.value) && "text-red-600 dark:text-red-400")} data-testid="formula-result">
                  {formatFieldValue(resultDef, preview.value, new Map()) || tf.resultEmpty}
                </strong>
              </p>
            )}
            {preview.volatile && <p>{tf.volatile}</p>}
            {preview.calendar && <p>{tf.calendar}</p>}
          </div>
        ) : null}
      </div>
    </div>
  );
}

function ErrorMark({ expr, position }: { expr: string; position: number }) {
  const { before, at, after } = splitAt(expr, position);
  return (
    <pre className="overflow-x-auto rounded bg-zinc-50 px-2 py-1 font-mono text-zinc-700 dark:bg-zinc-900 dark:text-zinc-300">
      {before}
      <mark className="rounded-sm bg-red-200 text-red-900 dark:bg-red-900 dark:text-red-100">{at}</mark>
      {after}
    </pre>
  );
}
