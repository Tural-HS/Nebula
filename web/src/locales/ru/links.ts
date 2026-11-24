// Russian strings for item links and watchers (ADR-0028).
import type { Messages } from "../../i18n/locale";
import { plural } from "../../i18n/plural";

const watchers = (n: number) =>
  `${String(n)} ${plural("ru", n, { one: "наблюдатель", few: "наблюдателя", many: "наблюдателей", other: "наблюдателя" })}`;

export const tl = {
  section: "Связи",
  types: {
    blocks: "Блокирует",
    blocked_by: "Заблокирована",
    relates: "Связана с",
    duplicates: "Дублирует",
    duplicated_by: "Дублируется",
  },
  empty: "Связей нет.",
  add: "Добавить связь",
  type: "Тип связи",
  search: "Поиск задач",
  searchPlaceholder: "Ключ (OPS-12) или заголовок",
  noResults: "Подходящих задач нет.",
  added: "Связь добавлена",
  exists: "Такая связь уже есть.",
  remove: (key: string) => `Удалить связь с ${key}`,
  closed: "закрыта",
  blocked: "Заблокирована",
  blockedTitle: "Эту задачу блокирует открытая задача",
  act: {
    added: (type: string, key: string) => `добавил(а) связь: ${type} ${key}`,
    removed: (type: string, key: string) => `удалил(а) связь: ${type} ${key}`,
    removedByDelete: (key: string) => `связь удалена, потому что ${key} удалена`,
  },
  watch: "Следить",
  unwatch: "Не следить",
  watchLabel: (watching: boolean, count: number) =>
    watching ? `Вы следите; ${watchers(count)}. Перестать следить` : `Следить; ${watchers(count)}`,
  watchers: "Наблюдатели",
  watcherCount: watchers,
  noWatchers: "Никто не следит.",
  addWatcher: "Добавить наблюдателя",
  chooseMember: "Выберите участника",
  removeWatcher: (name: string) => `Убрать ${name} из наблюдателей`,
} satisfies Messages["links"];
