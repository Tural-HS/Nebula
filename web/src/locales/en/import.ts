// English strings for the spreadsheet import wizard.
import type { FieldType } from "../../api/types";
import type { Messages } from "../../i18n/locale";
import { plural } from "../../i18n/plural";

const num = (n: number) => n.toLocaleString("en-GB");
const rowsOf = (n: number) => `${num(n)} ${plural("en", n, { one: "row", other: "rows" })}`;
const itemsOf = (n: number) => `${num(n)} ${plural("en", n, { one: "item", other: "items" })}`;

export const ti = {
  importAction: "Import",
  title: "Import from spreadsheet",
  intro: "Add items to this project from a CSV (UTF-8), Excel (.xlsx) or Mattermost Boards archive (.boardarchive) file. The server only reads the file; it is not stored.",
  steps: ["File", "Columns", "Review", "Result"],
  stepLabel: (n: number, of: number) => `Step ${String(n)} of ${String(of)}`,

  file: "File",
  chooseFile: "Choose file",
  fileHint: (maxMb: number, maxRows: number) =>
    `.csv, .tsv, .xlsx or .boardarchive; spreadsheets up to ${String(maxMb)} MB, Boards archives 50 MB, ${num(maxRows)} rows. The first row of a spreadsheet is the header.`,
  readFile: "Read file",
  reading: "Reading…",
  fileProblem: {
    extension: "This file type is not supported. Choose an .xlsx, CSV (UTF-8) or .boardarchive file.",
    size: "The file is too large (spreadsheets 10 MB, Boards archives 50 MB). Split the file.",
    empty: "The file is empty.",
  },

  sheet: "Sheet",
  board: "Board",
  peopleTitle: "Mattermost users",
  peopleHint: "The archive has no names or emails, only user IDs. Look at the cards and match each one to a Nebula member: they will be used as assignee and comment author. Unmatched users are left empty.",
  personLabel: (n: number, samples: string[]) => `${String(n)} ${plural("en", n, { one: "card", other: "cards" })}: ${samples.join(", ")}${n > samples.length ? "…" : ""}`,
  personNone: "— leave empty —",
  personPrivateHint: "In a private project, the assignee must be a project member.",
  summary: (rows: number, file: string) => `${file}: ${rowsOf(rows)}`,
  delimiter: (d: string) => `Delimiter: ${d === "\t" ? "tab" : d}`,
  defaultType: "Type (if not set in a column)",
  mappingTitle: "Map columns to fields",
  mappingHint: "Suggestions are based on column names. Unmapped columns are ignored.",
  column: "Column",
  sample: "Sample",
  target: "Nebula field",
  ignore: "— ignore —",
  builtinGroup: "Built-in",
  fieldsGroup: "Custom fields",
  missingTitle: "Choose a column for the title.",
  previewTitle: "First rows",
  row: "Row",
  back: "Back",
  validate: "Validate",
  validating: "Validating…",

  reportOk: (n: number) => `${n === 1 ? "The row is" : `All ${rowsOf(n)} are`} valid. Ready to import.`,
  reportErrors: (rows: number, errors: number) =>
    `${num(errors)} ${plural("en", errors, { one: "error", other: "errors" })} in ${rowsOf(rows)}. Fix the file or change the mapping; nothing is imported while there are errors.`,
  truncated: (shown: number) => `Showing the first ${String(shown)} ${plural("en", shown, { one: "error", other: "errors" })}.`,
  errorsTable: "Row errors",
  message: "Error",
  importNow: (n: number) => `Import ${itemsOf(n)}`,
  importing: "Importing…",
  fixMapping: "Back to mapping",

  done: (n: number) => `${itemsOf(n)} created`,
  doneRange: (first: string, last: string) => `${first} – ${last}`,
  openProject: "Go to project",
  importAnother: "Import another file",
  rejected: "Import rejected: some rows have errors, nothing was written.",
  adminOnly: "Import is for admins only.",

  builtin: {
    title: "Title",
    description: "Description",
    type: "Type",
    status: "Status",
    assignee: "Assignee (email)",
    due_date: "Due date",
  } as Record<string, string>,

  fieldType: {
    text: "text",
    long_text: "long text",
    number: "number",
    date: "date",
    datetime: "date and time",
    boolean: "yes/no",
    select: "select",
    multi_select: "multi-select",
    user: "user (email)",
    url: "URL",
    email: "email",
    formula: "formula (not imported)",
  } satisfies Record<FieldType, string>,

  // Server error codes (ADR-0011) → messages.
  errors: {
    file_too_large: "The file is too large (10 MB limit, 64 MB for an unpacked .xlsx).",
    too_many_rows: "The spreadsheet has more than 20,000 rows. Split the file.",
    too_many_columns: "The spreadsheet has more than 200 columns.",
    cell_too_long: "A cell is longer than 32,767 characters.",
    invalid_encoding: "The file is not UTF-8. In Excel, save it as \"CSV UTF-8\".",
    unsupported_format: "This format is not supported. Use .xlsx or CSV (UTF-8). Legacy .xls and encrypted files cannot be read.",
    malformed_file: "The file could not be read (corrupted or wrong format).",
    empty_file: "The file has no data rows.",
    sheet_not_found: "The selected sheet was not found.",
    timeout: "Reading the file took too long.",
    import_busy: "Other imports are running. Try again in a moment.",
  } as Record<string, string>,
} satisfies Messages["importer"];
