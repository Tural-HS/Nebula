// Russian strings for the approval inbox, re-approval and deadlines (ADR-0034).
import type { Messages } from "../../i18n/locale";
import { plural } from "../../i18n/plural";

const requests = (n: number) => `${String(n)} ${plural("ru", n, { one: "запрос", few: "запроса", many: "запросов", other: "запроса" })}`;
const days = (n: number) => `${String(n)} ${plural("ru", n, { one: "день", few: "дня", many: "дней", other: "дня" })}`;
const workdays = (n: number) => `${String(n)} ${plural("ru", n, { one: "рабочий день", few: "рабочих дня", many: "рабочих дней", other: "рабочих дня" })}`;

export const tai = {
  nav: "Ждут согласования",
  title: "Ждут согласования",
  hint: "Ожидающие запросы на согласование из всех доступных вам проектов пространства, сначала самые старые.",
  tabs: { mine: "Ждут моего решения", all: "Все ожидающие" },
  empty: { mine: "Нет запросов, ожидающих вашего решения.", all: "Нет ожидающих запросов на согласование." },
  badge: (n: number) => `${requests(n)} ждут вашего решения`,
  badgeOverdue: (n: number) => `${requests(n)} с истёкшим сроком`,
  requestedBy: (name: string) => `Запросил(а): ${name}`,
  age: (n: number) => (n <= 0 ? "сегодня" : `${days(n)} назад`),
  due: (date: string) => `Срок: ${date}`,
  overdue: (date: string) => `Срок истёк (${date})`,
  yourTurn: "Ждёт вашего решения",
  open: "Открыть",
  more: "Показать ещё",
  invalidated: "Согласованное содержание изменилось позже: это согласование больше не действует.",
  reopened: "Запрошено повторно, потому что согласованное содержание изменилось.",
  notify: {
    overdue: "Срок запроса на согласование истёк",
    invalidated: (actor: string) => `${actor} изменил(а) согласованное содержание, нужно повторное согласование`,
  },
  act: {
    invalidated: "изменил(а) согласованное содержание: согласование аннулировано, задача отправлена на повторное согласование",
    overdue: "срок согласования истёк",
  },
  errors: {
    reapproval_required: "Изменение согласованного содержания требует нового согласования: редактируйте эту задачу отдельно.",
  } as Record<string, string>,
  settings: {
    reapprove: "Изменения после согласования требуют нового согласования",
    reapproveHint: "При изменении отслеживаемого содержания задача возвращается в статус до согласования и открывается новый запрос; автор изменения не может его согласовать.",
    watched: "Отслеживаемое содержание",
    watchAll: "Всё: название, описание и все поля",
    titleField: "Название",
    descriptionField: "Описание",
    due: "Срок решения, рабочих дней",
    dueHint: "Считается по календарю рабочих дней. Оставьте пустым — без срока.",
    dueRange: "Должно быть от 1 до 30.",
    summaryDue: (n: number) => workdays(n),
    summaryReapprove: "повторно при изменении",
  },
  reapproveWarn: (what: string) => `Внимание: если изменить ${what}, задача уйдёт на повторное согласование.`,
  watchAllShort: "название, описание или поля",
} satisfies Messages["approvalInbox"];
