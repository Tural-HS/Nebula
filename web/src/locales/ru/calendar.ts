// Russian strings for the calendar view, bulk editing and inline editing (ADR-0023).
import type { Messages } from "../../i18n/locale";
import { plural } from "../../i18n/plural";

const items = (n: number) => plural("ru", n, { one: "задача", few: "задачи", many: "задач", other: "задачи" });
const itemsAcc = (n: number) => plural("ru", n, { one: "задачу", few: "задачи", many: "задач", other: "задачи" });
const itemsGen = (n: number) => plural("ru", n, { one: "задачи", few: "задач", many: "задач", other: "задачи" });
const itemsPrep = (n: number) => plural("ru", n, { one: "задаче", few: "задачах", many: "задачах", other: "задачах" });

export const tc = {
  viewCalendar: "Календарь",
  prevMonth: "Предыдущий месяц",
  nextMonth: "Следующий месяц",
  thisMonth: "Текущий месяц",
  weekdays: ["Пн", "Вт", "Ср", "Чт", "Пт", "Сб", "Вс"],
  weekdaysLong: ["Понедельник", "Вторник", "Среда", "Четверг", "Пятница", "Суббота", "Воскресенье"],
  months: ["Январь", "Февраль", "Март", "Апрель", "Май", "Июнь", "Июль", "Август", "Сентябрь", "Октябрь", "Ноябрь", "Декабрь"],
  more: (n: number) => `+${String(n)} ещё`,
  dayItems: (day: string, n: number) => `${day}: ${String(n)} ${items(n)}`,
  noDue: "Без срока",
  noDueHint: "Перетащите задачу на день, чтобы назначить срок.",
  noDueEmpty: "Задач без срока нет",
  agendaEmpty: "В этом месяце нет задач со сроком",
  truncated: (n: number) => `Показаны первые ${String(n)} ${items(n)}; уточните фильтр.`,
  moved: (key: string, day: string) => `${key}: срок — ${day}`,
  today: "Сегодня",
  loadMore: "Загрузить ещё",
  dragHint: "Пробел — взять, стрелки — переместить, Пробел — отпустить",

  select: (key: string) => `Выбрать ${key}`,
  selectAll: "Выбрать все",
  selectAllCapped: (n: number) => `Выбрать все (не более ${String(n)})`,
  selected: (n: number) => `${plural("ru", n, { one: "Выбрана", few: "Выбраны", many: "Выбрано", other: "Выбрано" })} ${String(n)} ${items(n)}`,
  clearSelection: "Снять выделение",
  bulkBar: "Действия с выбранными задачами",
  setStatus: "Изменить статус",
  setAssignee: "Изменить исполнителя",
  setDue: "Изменить срок",
  clearDue: "Убрать срок",
  setField: "Значение поля",
  fieldValue: "Значение",
  apply: "Применить",
  bulkDelete: "Удалить",
  confirmBulkDelete: (n: number) => `Выбранные ${String(n)} ${items(n)}, их комментарии и файлы будут удалены. Это нельзя отменить.`,
  bulkDeleteTitle: (n: number) => `Удалить ${String(n)} ${itemsAcc(n)}?`,
  bulkDeleteAction: (n: number) => `Удалить ${String(n)} ${itemsAcc(n)}`,
  bulkDone: (n: number) => `${plural("ru", n, { one: "Обновлена", few: "Обновлены", many: "Обновлено", other: "Обновлено" })} ${String(n)} ${items(n)}`,
  bulkDeleted: (n: number) => `${plural("ru", n, { one: "Удалена", few: "Удалены", many: "Удалено", other: "Удалено" })} ${String(n)} ${items(n)}`,
  bulkFailedTitle: "Ничего не изменено",
  bulkFailed: (n: number) => `В ${String(n)} ${itemsPrep(n)} есть ошибки, поэтому изменения не внесены:`,
  bulkConflictHint: "Некоторые задачи изменил кто-то другой. Список обновлён; выберите задачи и попробуйте снова.",
  bulkCap: (n: number) => `Не более ${String(n)} ${itemsGen(n)} за раз`,

  edit: (what: string, key: string) => `${key}: изменить «${what}»`,
  invalidNumber: (what: string) => `${what}: введите число, например 1250,50. Значение не изменено.`,
  saveFailed: "Не удалось сохранить изменение",
} satisfies Messages["calendar"];
