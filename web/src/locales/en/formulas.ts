// English strings for formula fields (ADR-0030).
import type { Messages } from "../../i18n/locale";
import { plural } from "../../i18n/plural";

export const tf = {
  expression: "Formula",
  expressionHint: "Field keys, numbers and functions: amount - paid, round(amount * 0.18, 2), due - today()",
  help: "Functions: if, round, min, max, abs, coalesce, today, days, workdays, date. The item's due date: @due_date. An empty value gives an empty result; use coalesce(paid, 0) to count it as 0.",
  helpTitle: "Functions",
  scale: "Decimal places",
  sampleItem: "Sample item",
  sampleItemHint: "e.g. FIN-12",
  checking: "Checking…",
  valid: "The formula is valid",
  errorAt: (pos: number, msg: string) => `Position ${String(pos)}: ${msg}`,
  result: "Result",
  resultEmpty: "empty",
  resultTypes: { number: "number", date: "date", boolean: "yes/no" } as Record<string, string>,
  resultType: (type: string) => `Result type: ${type}`,
  refs: (n: number) => `Uses ${String(n)} ${plural("en", n, { one: "field", other: "fields" })}`,
  volatile: "Uses today(): refreshed every day in the workspace time zone",
  calendar: "Uses workdays(): recomputed when the working-day calendar changes",
  save: "Save formula",
  readOnly: "Computed field: its value cannot be edited",
  title: (expr: string) => `ƒ = ${expr}`,
  errors: {
    division_by_zero: "#DIV/0",
    overflow: "#TOO LARGE",
    date_out_of_range: "#DATE",
    invalid_argument: "#ERROR",
  } as Record<string, string>,
  sortAsc: (name: string) => `${name} ↑`,
  sortDesc: (name: string) => `${name} ↓`,
} satisfies Messages["formulas"];
