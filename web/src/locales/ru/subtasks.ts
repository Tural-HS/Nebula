// Russian strings for subtasks (ADR-0021).
import type { Messages } from "../../i18n/locale";
import { plural } from "../../i18n/plural";

const subtasksAcc = (n: number) => plural("ru", n, { one: "подзадачу", few: "подзадачи", many: "подзадач", other: "подзадачи" });

export const ts = {
  section: "Подзадачи",
  progress: (done: number, count: number) => `Выполнено ${String(done)}/${String(count)}`,
  badge: (done: number, count: number) => `Подзадачи: выполнено ${String(done)}/${String(count)}`,
  empty: "Подзадач нет.",
  add: "Добавить подзадачу",
  addPlaceholder: "Заголовок новой подзадачи",
  added: "Подзадача добавлена",
  parent: "Родительская задача",
  parentCrumb: "Перейти к родительской задаче",
  detach: "Отвязать от родительской задачи",
  noNesting: "У подзадачи не может быть своих подзадач.",
  changedParent: "сменил(а) родительскую задачу",
  more: (n: number) => `Показать ещё ${String(n)} ${subtasksAcc(n)}`,
} satisfies Messages["subtasks"];
