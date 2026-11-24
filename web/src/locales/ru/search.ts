// Russian strings for workspace-wide search (ADR-0024).
import type { Messages } from "../../i18n/locale";

export const ts = {
  open: "Поиск",
  placeholder: "Поиск задач, комментариев или ключа (QA-12)…",
  dialogTitle: "Поиск по рабочему пространству",
  inputLabel: "Поисковый запрос",
  shortcut: (mac: boolean) => (mac ? "⌘K" : "Ctrl K"),
  hint: "Введите не менее 2 символов. Азербайджанские буквы ə, ö, ü, ş, ç, ğ, ı находятся и без диакритики: «odenis» → «ödəniş».",
  items: "Задачи",
  comments: "Комментарии",
  count: (shown: number, total: number) => (total > shown ? `${String(shown)} / ${String(total)}` : String(total)),
  empty: "Ничего не найдено.",
  fuzzy: "Точных совпадений нет. Похожие заголовки:",
  keyMatch: "Ключ",
  commentBy: (author: string) => `Комментарий: ${author}`,
  someone: "Пользователь",
  more: "Больше результатов",
  rateLimited: "Слишком частые запросы, подождите немного…",
  tooLong: "Запрос слишком длинный (не более 200 символов).",
  navHint: "↑↓ — выбор, Enter — открыть, Esc — закрыть",
} satisfies Messages["search"];
