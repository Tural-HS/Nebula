import { useState } from "react";

import type { ItemFilters } from "../api/hooks";
import { downloadItemsExport } from "../api/itemExport";
import { useToast } from "../components/toast-context";
import { Button } from "../components/ui";
import { tv } from "../i18n-views";
import { activeLocale } from "../i18n/locale";
import { exportErrorText, type ExportFormat } from "../lib/itemExport";
import { Popover } from "./ViewTools";

/**
 * "İxrac": the table as it is (filters, search, queue, sort, visible
 * columns) as Excel or CSV (ADR-0040). Inline in the phone filter sheet.
 * State lives here, outside the panel, so a download that is still running
 * when the panel closes keeps its result.
 */
export function ExportMenu({ ws, project, filters, columns, inline = false }: {
  ws: string;
  project: string;
  filters: ItemFilters;
  columns: readonly string[];
  inline?: boolean;
}) {
  const toast = useToast();
  const [semicolon, setSemicolon] = useState(false);
  const [running, setRunning] = useState<ExportFormat | null>(null);
  const [error, setError] = useState<{ format: ExportFormat; text: string } | null>(null);
  const te = tv.export;

  const run = async (format: ExportFormat) => {
    setRunning(format);
    setError(null);
    try {
      const name = await downloadItemsExport(ws, project, { filters, columns, format, lang: activeLocale(), semicolon });
      toast("success", te.done(name));
    } catch (err) {
      setError({ format, text: exportErrorText(err) });
    } finally {
      setRunning(null);
    }
  };

  return (
    <Popover label={te.action} inline={inline} align="right">
      <div className="flex flex-col gap-2" aria-busy={running !== null}>
        {!inline && <p className="font-medium">{te.title}</p>}
        <p className="text-xs text-zinc-500 dark:text-zinc-400">{te.hint}</p>
        <div className="flex flex-wrap gap-2">
          <Button variant="primary" className="flex-1" disabled={running !== null} onClick={() => void run("xlsx")}>
            {te.xlsx}
          </Button>
          <Button className="flex-1" disabled={running !== null} onClick={() => void run("csv")}>
            {te.csv}
          </Button>
        </div>
        <label className="flex items-start gap-2 text-sm pointer-coarse:min-h-11">
          <input type="checkbox" className="mt-0.5 size-4 accent-brand-600" checked={semicolon} onChange={(e) => { setSemicolon(e.target.checked); }} />
          <span>
            {te.semicolon}
            <span className="block text-xs text-zinc-500 dark:text-zinc-400">{te.semicolonHint}</span>
          </span>
        </label>
        <p aria-live="polite" className="text-xs text-zinc-600 dark:text-zinc-400">
          {running && (
            <span className="inline-flex items-center gap-2">
              <span className="size-3.5 animate-spin rounded-full border-2 border-zinc-300 border-t-brand-600 motion-reduce:animate-none" aria-hidden />
              {te.working}
            </span>
          )}
        </p>
        {error && (
          <div role="alert" className="rounded-md border border-red-200 bg-red-50 p-2 text-xs text-red-800 dark:border-red-900 dark:bg-red-950 dark:text-red-200">
            <p>{error.text}</p>
            <Button className="mt-2" disabled={running !== null} onClick={() => void run(error.format)}>
              {te.retry}
            </Button>
          </div>
        )}
      </div>
    </Popover>
  );
}
