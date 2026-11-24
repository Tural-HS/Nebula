// Russian strings for the working-day calendar (ADR-0027).
import type { Messages } from "../../i18n/locale";
import { plural } from "../../i18n/plural";

const MONTHS_GENITIVE = ["января", "февраля", "марта", "апреля", "мая", "июня", "июля", "августа", "сентября", "октября", "ноября", "декабря"] as const;

const every = (n: number) => plural("ru", n, { one: "Каждый", few: "Каждые", many: "Каждые", other: "Каждые" });
const unit = (n: number, one: string, few: string, many: string) => plural("ru", n, { one, few, many, other: few });
const workDayLabel = (workDay: number) => (workDay === -1 ? "в последний рабочий день" : `в ${String(workDay)}-й рабочий день`);

export const tw = {
  nav: "Рабочие дни",
  title: "Календарь рабочих дней",
  intro:
    "Выходные, праздники и перенесённые рабочие дни определяют даты повторяющихся задач: «последний рабочий день месяца», «5-й рабочий день месяца» и перенос даты, попавшей на нерабочий день. Изменение сразу пересчитывает следующую дату правил; задачи за прошедшие даты не создаются, уже созданные задачи не меняются.",
  weekend: "Выходные дни недели",
  weekendHint: "Не больше 3 дней. В представлении «Календарь» эти дни тоже выделены.",
  year: "Год",
  prevYear: "Предыдущий год",
  nextYear: "Следующий год",
  days: "Праздники и переносы",
  empty: (year: number) => `Для ${String(year)} года даты ещё не добавлены.`,
  count: (n: number, max: number) => `${String(n)} / ${String(max)} ${unit(max, "даты", "дат", "дат")}`,
  date: "Дата",
  name: "Название",
  kind: "Тип",
  kinds: { holiday: "Нерабочий день (праздник)", workday: "Рабочий день (перенос)" },
  kindShort: { holiday: "Праздник", workday: "Рабочий день" },
  defaultWorkdayName: "Перенесённый рабочий день",
  add: "Добавить",
  remove: (date: string) => `Удалить ${date}`,
  added: "Дата добавлена",
  removed: "Дата удалена",
  workdayNeedsWeekend: "Рабочим днём можно объявить только выходной день.",
  invalidDate: "Выберите корректную дату с 2000 по 2100 год.",
  nameRequired: "Нужно название (не больше 100 символов).",
  limitReached: (max: number) => `В году может быть не больше ${String(max)} дат.`,
  onWeekend: "приходится на выходной",
  presetTitle: "Праздники Азербайджана с фиксированной датой",
  presetButton: (year: number) => `Добавить в ${String(year)} год`,
  presetHint:
    "Добавляются нерабочие дни с фиксированной датой из статьи 105 Трудового кодекса: 1–2 января, 8 марта, 9 мая, 28 мая, 15 июня, 26 июня, 8 и 9 ноября, 31 декабря. Уже добавленные вами даты не меняются.",
  presetYearly:
    "Проверяйте каждый год: дни Новруза, Рамазан и Гурбан байрамы, выходные, перенесённые из-за совпадения праздника с выходным, и переносы рабочих дней ежегодно объявляются решением Кабинета министров. Добавьте их здесь вручную после публикации решения.",
  presetAdded: (n: number) =>
    n === 0 ? "Ничего нового: все даты уже есть." : `${plural("ru", n, { one: "Добавлена", few: "Добавлены", many: "Добавлено", other: "Добавлены" })} ${String(n)} ${unit(n, "дата", "даты", "дат")}`,

  holidayLabel: (name: string) => `Нерабочий день: ${name}`,
  workdayLabel: (name: string) => `Рабочий день: ${name}`,
  weekendLabel: "Выходной",

  dayMode: "День",
  dayModes: { date: "Число месяца", workday: "Рабочий день" },
  workDay: "Какой рабочий день",
  lastWorkDay: "Последний рабочий день",
  workDayOption: (n: number) => (n === -1 ? "Последний рабочий день" : `${String(n)}-й рабочий день`),
  shift: "Если выпадает на нерабочий день",
  shifts: { none: "Не переносить", previous: "На предыдущий рабочий день", next: "На следующий рабочий день" },
  calendarHint: "Выходные и праздники берутся из календаря рабочих дней пространства.",
  describe: {
    monthly: (n: number, workDay: number) =>
      `${n === 1 ? "Каждый месяц" : `${every(n)} ${String(n)} ${unit(n, "месяц", "месяца", "месяцев")}`}, ${workDayLabel(workDay)}`,
    yearly: (n: number, month: number, workDay: number) =>
      `${n === 1 ? "Каждый год" : `${every(n)} ${String(n)} ${unit(n, "год", "года", "лет")}`}, ${workDayLabel(workDay)} ${MONTHS_GENITIVE[month - 1] ?? ""}`,
    shift: { previous: " (если нерабочий день — предыдущий рабочий день)", next: " (если нерабочий день — следующий рабочий день)" },
  },
} satisfies Messages["workdays"];
