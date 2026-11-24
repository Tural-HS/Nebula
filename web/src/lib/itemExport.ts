// Item table export (ADR-0040): the request for what the table shows, the
// downloaded file's name, and user-facing errors.

import { ApiError } from "../api/client";
import { itemsQueryString, type ItemFilters } from "../api/hooks";
import { tv } from "../i18n-views";
import type { Locale } from "../i18n/locale";
import { formatDecimal } from "./views";

export type ExportFormat = "xlsx" | "csv";

/** Rows in one export (server: MaxExportRows). */
export const MAX_EXPORT_ROWS = 50_000;

/**
 * The export's query string: the list's own filters, search and sort (the
 * same parameters as GET /items, without paging), the visible columns in
 * order, the language of the headers and, for CSV, the delimiter.
 */
export function exportQuery(f: ItemFilters, columns: readonly string[], format: ExportFormat, lang: Locale, semicolon: boolean): string {
  const p = new URLSearchParams(itemsQueryString(f, null));
  p.delete("limit");
  p.set("format", format);
  if (columns.length > 0) p.set("cols", columns.join(","));
  p.set("lang", lang);
  if (format === "csv" && semicolon) p.set("delimiter", "semicolon");
  return p.toString();
}

// eslint-disable-next-line no-control-regex -- control characters are exactly what must go
const UNSAFE = /[\u0000-\u001f\u007f/\\:*?"<>|]/g;

/**
 * The file name from Content-Disposition: the UTF-8 filename* (the project
 * name) when present and sane, else the ASCII filename, else fallback. It
 * must keep the expected extension and cannot contain a path.
 */
export function exportFileName(disposition: string | null, fallback: string, ext: ExportFormat): string {
  const ok = (n: string) => n.length > ext.length + 1 && n.length <= 200 && n.toLowerCase().endsWith("." + ext) && !n.startsWith(".");
  if (disposition) {
    const star = /filename\*=UTF-8''([^;\s]+)/i.exec(disposition);
    if (star?.[1]) {
      try {
        const name = decodeURIComponent(star[1]).replace(UNSAFE, "_").trim();
        if (ok(name)) return name;
      } catch {
        // malformed percent-encoding: fall through to the ASCII name
      }
    }
    const plain = /filename="([^"]{1,200})"/.exec(disposition);
    const name = (plain?.[1] ?? "").replace(/[^A-Za-z0-9._-]/g, "_");
    if (ok(name)) return name;
  }
  return fallback;
}

/** A user-facing message (with what to do next) for a failed export. */
export function exportErrorText(err: unknown): string {
  if (err instanceof ApiError) {
    switch (err.code) {
      case "export_too_large":
        return err.field === "rows" ? tv.export.tooManyRows(formatDecimal(MAX_EXPORT_ROWS)) : tv.export.tooLarge;
      case "rate_limited":
        return tv.export.rateLimited;
      case "export_busy":
        return tv.export.busy;
    }
  }
  return tv.export.failed;
}
