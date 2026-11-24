// Russian strings for the home page "Мои задачи" (ADR-0037).
import type { Messages } from "../../i18n/locale";
import { plural } from "../../i18n/plural";

const items = (n: number) => plural("ru", n, { one: "задача", few: "задачи", many: "задач", other: "задачи" });
const projects = (n: number) => plural("ru", n, { one: "проект", few: "проекта", many: "проектов", other: "проекта" });
const approvals = (n: number) => plural("ru", n, { one: "согласование", few: "согласования", many: "согласований", other: "согласования" });

export const thm = {
  title: "Мои задачи",
  greeting: (name: string) => (name ? `Здравствуйте, ${name}` : "Здравствуйте"),
  counters: "Счётчики",
  counter: { overdue: "Просрочено", approvals: "Ждут моего решения", week: "Срок на этой неделе", assigned: "Назначено мне" },
  counterLabel: (label: string, n: number) => `${label}: ${String(n)}`,
  attention: "Требует внимания",
  showAll: (n: number) => `Показать все (${String(n)})`,
  list: {
    attention: "Всё, что требует внимания",
    overdue: "Мои просроченные задачи",
    approval: "Согласования, ждущие моего решения",
    week: "Мои задачи со сроком на этой неделе",
    assigned: "Открытые задачи, назначенные мне",
  },
  listHint: "Из всех ваших рабочих пространств, только видимые вам проекты.",
  back: "Мои задачи",
  reason: { overdue: "Просрочено", approval: "Согласование", week: "Эта неделя", assigned: "Назначено" },
  due: (date: string) => `Срок: ${date}`,
  waiting: (age: string) => `Ждёт: ${age}`,
  truncated: (shown: number, total: number) => `Показаны первые ${String(shown)} ${items(shown)} из ${String(total)}.`,
  listEmpty: "В этом списке ничего нет.",
  calm: "Сейчас ничего не требует вашего внимания",
  calmHint: "Нет просроченных задач, согласований, ждущих вас, и задач со сроком на этой неделе.",
  recentProjects: "Недавние проекты",
  searchHint: (keys: string) => `Внутри рабочего пространства нажмите ${keys}, чтобы искать задачи и комментарии.`,
  noWorkspaces: "У вас пока нет рабочего пространства",
  invitedHint: "Если вас пригласили, ссылка в вашей почте. Иначе попросите администратора добавить вас.",
  createHint: "Вы можете и сами открыть рабочее пространство для команды: с шаблоном для финансов, ИТ или HR.",
  createCta: "Создать пространство по шаблону",
  notifications: "Последние уведомления",
  noNotifications: "Уведомлений нет.",
  requests: "Мои заявки",
  allRequests: "Все заявки",
  watching: "Отслеживаемые задачи",
  card: {
    projects: (n: number) => `${String(n)} ${projects(n)}`,
    assigned: "открыто",
    overdue: "просрочено",
    approvals: "согласования",
    lastActivity: (when: string) => `Последнее изменение ${when}`,
    noActivity: "Изменений пока нет",
    stats: (open: number, overdue: number, n: number) =>
      `Мои: ${String(open)} открыто, ${String(overdue)} просрочено, ${String(n)} ${approvals(n)}`,
  },
} satisfies Messages["home"];
