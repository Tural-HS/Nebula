// Russian strings for saved views, table columns and totals (ADR-0014).
import type { Messages } from "../../i18n/locale";
import { plural } from "../../i18n/plural";

const itemsGen = (n: number) => plural("ru", n, { one: "задачи", few: "задач", many: "задач", other: "задачи" });

export const tv = {
  views: "Представление",
  allItems: "Все",
  personal: "Личные",
  shared: "Общие",
  modified: "изменено",
  saveAs: "Сохранить как представление",
  saveChanges: "Сохранить изменения",
  rename: "Переименовать",
  remove: "Удалить представление",
  share: "Открыть доступ всем",
  unshare: "Закрыть общий доступ",
  viewMenu: "Действия с представлением",
  newViewTitle: "Новое представление",
  renameTitle: "Название представления",
  viewName: "Название",
  shareHint: "Общее представление видят все, у кого есть доступ к проекту.",
  confirmDelete: (name: string) => `Вы уверены, что хотите удалить представление «${name}»?`,
  savedView: (name: string) => `Представление «${name}» сохранено`,
  viewMissing: "Это представление не найдено или к нему не открыт доступ.",

  columns: "Столбцы",
  columnsHint: "Столбцы таблицы и их порядок",
  moveLeft: "Влево",
  moveRight: "Вправо",
  resetColumns: "Столбцы по умолчанию",
  sort: "Сортировка",
  sortNumber: "Сначала новые",
  sortDue: "По сроку",
  filters: "Фильтры",
  statusFilter: "Статусы",
  fieldFilter: (name: string) => name,
  anyValue: "Любое",
  clearFilters: "Сбросить фильтры",
  activeFilters: (n: number) => `Фильтры (${String(n)})`,

  total: "Итого",
  totalHint: (count: number) => `Значение есть у ${String(count)} ${itemsGen(count)} (по всем отфильтрованным результатам)`,
  created: "Создано",

  col: {
    key: "#",
    title: "Заголовок",
    type: "Тип",
    status: "Статус",
    assignee: "Исполнитель",
    due_date: "Срок",
    created_at: "Создано",
    updated_at: "Обновлено",
  },
} as const satisfies Messages["views"];
