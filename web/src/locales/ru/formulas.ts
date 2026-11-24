// Russian strings for formula fields (ADR-0030).
import type { Messages } from "../../i18n/locale";
import { plural } from "../../i18n/plural";

export const tf = {
  expression: "Формула",
  expressionHint: "Ключи полей, числа и функции: amount - paid, round(amount * 0.18, 2), due - today()",
  help: "Функции: if, round, min, max, abs, coalesce, today, days, workdays, date. Срок задачи: @due_date. Пустое значение даёт пустой результат; чтобы считать его нулём, используйте coalesce(paid, 0).",
  helpTitle: "Функции",
  scale: "Знаков после запятой",
  sampleItem: "Пример задачи",
  sampleItemHint: "напр. FIN-12",
  checking: "Проверка…",
  valid: "Формула корректна",
  errorAt: (pos: number, msg: string) => `Позиция ${String(pos)}: ${msg}`,
  result: "Результат",
  resultEmpty: "пусто",
  resultTypes: { number: "число", date: "дата", boolean: "да/нет" } as Record<string, string>,
  resultType: (type: string) => `Тип результата: ${type}`,
  refs: (n: number) => `Использует ${String(n)} ${plural("ru", n, { one: "поле", few: "поля", many: "полей", other: "поля" })}`,
  volatile: "Использует today(): обновляется каждый день по часовому поясу пространства",
  calendar: "Использует workdays(): пересчитывается при изменении производственного календаря",
  save: "Сохранить формулу",
  readOnly: "Вычисляемое поле: значение нельзя изменить вручную",
  title: (expr: string) => `ƒ = ${expr}`,
  errors: {
    division_by_zero: "#ДЕЛ/0",
    overflow: "#СЛИШКОМ БОЛЬШОЕ",
    date_out_of_range: "#ДАТА",
    invalid_argument: "#ОШИБКА",
  } as Record<string, string>,
  sortAsc: (name: string) => `${name} ↑`,
  sortDesc: (name: string) => `${name} ↓`,
} satisfies Messages["formulas"];
