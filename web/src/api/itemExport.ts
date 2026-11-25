// Item table export download (ADR-0040).
import { apiGetFile, path } from "./client";
import type { ItemFilters } from "./hooks";
import { saveBlob } from "./evidence";
import { exportFileName, exportQuery, type ExportFormat } from "../lib/itemExport";
import type { Locale } from "../i18n/locale";

/** Downloads the export and saves it; resolves with the saved file name. */
export async function downloadItemsExport(
  ws: string,
  project: string,
  opts: { filters: ItemFilters; columns: readonly string[]; format: ExportFormat; lang: Locale; semicolon: boolean },
): Promise<string> {
  const q = exportQuery(opts.filters, opts.columns, opts.format, opts.lang, opts.semicolon);
  const res = await apiGetFile(path`/workspaces/${ws}/projects/${project}/items/export` + "?" + q);
  const name = exportFileName(res.disposition, `${project}.${opts.format}`, opts.format);
  saveBlob(res.blob, name);
  return name;
}
