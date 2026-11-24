// English strings for subtasks (ADR-0021).
import type { Messages } from "../../i18n/locale";
import { plural } from "../../i18n/plural";

export const ts = {
  section: "Subtasks",
  progress: (done: number, count: number) => `${String(done)}/${String(count)} done`,
  badge: (done: number, count: number) => `Subtasks: ${String(done)}/${String(count)} done`,
  empty: "No subtasks.",
  add: "Add subtask",
  addPlaceholder: "New subtask title",
  added: "Subtask added",
  parent: "Parent item",
  parentCrumb: "Go to parent item",
  detach: "Detach from parent item",
  noNesting: "A subtask cannot have its own subtasks.",
  changedParent: "changed the parent item",
  more: (n: number) => `Show ${String(n)} more ${plural("en", n, { one: "subtask", other: "subtasks" })}`,
} satisfies Messages["subtasks"];
