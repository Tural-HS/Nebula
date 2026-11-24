// Russian strings for workspace/project templates and the Mattermost
// Boards import (ADR-0020).
import type { Messages } from "../../i18n/locale";

export const tt = {
  template: "Начальный шаблон",
  templateHint: "Статусы и типы задач можно изменить позже в настройках.",

  projectTemplate: "Проект-шаблон",
  projectTemplateNone: "Нет (пустой проект)",
  projectTemplateHint: "Из выбранного проекта копируется перечисленное ниже. Задачи не копируются.",
  copyMembers: "Участники проекта",
  copyRecurrences: "Повторяющиеся задачи",
  copyViews: "Общие представления",
} satisfies Messages["templates"];
