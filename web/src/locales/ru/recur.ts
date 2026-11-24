// Russian strings for recurring items (ADR-0012).
import type { Messages } from "../../i18n/locale";
import { plural } from "../../i18n/plural";

const MONTHS_GENITIVE = ["января", "февраля", "марта", "апреля", "мая", "июня", "июля", "августа", "сентября", "октября", "ноября", "декабря"] as const;

/** "Каждый" / "Каждые" / "Каждую" agree with the count that follows. */
const every = (n: number, oneForm: "Каждый" | "Каждую") => plural("ru", n, { one: oneForm, few: "Каждые", many: "Каждые", other: "Каждые" });
const unit = (n: number, one: string, few: string, many: string) => plural("ru", n, { one, few, many, other: few });

/** day: 1..31, or -1 for the last day of the month. */
const dayOfMonth = (day: number) => (day === -1 ? "в последний день" : `${String(day)}-го числа`);

export const tr = {
  nav: "Повторяющиеся",
  title: "Повторяющиеся задачи",
  intro: "Правило каждый раз создаёт только следующую задачу, за несколько дней до её срока. Срок задачи — это дата повторения, поэтому невыполненная работа автоматически становится «просроченной».",
  newRule: "Новое правило",
  editRule: "Редактировать правило",
  empty: "Повторяющихся задач пока нет.",
  titleLabel: "Шаблон заголовка",
  titleHint: "{{month}}, {{year}} и {{date}} заменяются датой: «Аренда офиса — оплата за {{month}}».",
  freq: "Частота",
  freqs: { daily: "Ежедневно", weekly: "Еженедельно", monthly: "Ежемесячно", yearly: "Ежегодно" },
  every: "Интервал",
  everyUnit: { daily: "дн.", weekly: "нед.", monthly: "мес.", yearly: "г." },
  weekdays: "Дни недели",
  monthDay: "День месяца",
  lastDay: "Последний день месяца",
  month: "Месяц",
  months: ["Январь", "Февраль", "Март", "Апрель", "Май", "Июнь", "Июль", "Август", "Сентябрь", "Октябрь", "Ноябрь", "Декабрь"],
  start: "Начало",
  end: "Окончание (необязательно)",
  lead: "За сколько дней создавать",
  enabled: "Активно",
  next: "Следующая",
  upcoming: "Ближайшие даты",
  ended: "Завершено",
  disabled: "Приостановлено",
  failed: "Ошибка: правило приостановлено",
  confirmDelete: "Удалить правило? Уже созданные задачи останутся.",
  adminOnlyEdit: "Правила могут менять только администраторы.",
  noAccess: "Повторяющиеся задачи недоступны гостям.",

  /** Monday first. */
  weekdayShort: ["Пн", "Вт", "Ср", "Чт", "Пт", "Сб", "Вс"],
  weekdayLong: ["понедельник", "вторник", "среда", "четверг", "пятница", "суббота", "воскресенье"],
  /** One-line schedule summaries ("Каждый месяц, 9-го числа"); day is 1..31 or -1 (last day), month 1..12. */
  describe: {
    daily: (n: number) => (n === 1 ? "Каждый день" : `${every(n, "Каждый")} ${String(n)} ${unit(n, "день", "дня", "дней")}`),
    weekly: (n: number, days: string) =>
      `${n === 1 ? "Каждую неделю" : `${every(n, "Каждую")} ${String(n)} ${unit(n, "неделю", "недели", "недель")}`}: ${days}`,
    monthly: (n: number, day: number) =>
      `${n === 1 ? "Каждый месяц" : `${every(n, "Каждый")} ${String(n)} ${unit(n, "месяц", "месяца", "месяцев")}`}, ${dayOfMonth(day)}`,
    yearly: (n: number, month: number, day: number) => {
      const m = MONTHS_GENITIVE[month - 1] ?? "";
      const head = n === 1 ? "Каждый год" : `${every(n, "Каждый")} ${String(n)} ${unit(n, "год", "года", "лет")}`;
      return `${head}, ${day === -1 ? `в последний день ${m}` : `${String(day)} ${m}`}`;
    },
  },
} as const satisfies Messages["recur"];
