// Russian strings for audit evidence export (ADR-0029).
import type { Messages } from "../../i18n/locale";
import { plural } from "../../i18n/plural";

// Genitive plural after "слишком много".
const WHAT = {
  items: "задач",
  events: "событий",
  comments: "комментариев",
  approvals: "запросов на согласование",
  attachments: "файлов",
  links: "связей",
  bytes: "данных (больше 100 МБ)",
} as const;

const n = (count: number, one: string, few: string, many: string) => `${String(count)} ${plural("ru", count, { one, few, many, other: few })}`;

export const te: Messages["evidence"] = {
  title: "Пакет аудиторских доказательств",
  intro:
    "Подписанный ZIP для аудиторов и регуляторов: читаемый отчёт (откройте в браузере и распечатайте в PDF), данные в JSON и CSV, манифест с SHA-256 каждого файла и его подпись Ed25519. В пакет попадают только задачи, которые видите вы; каждый экспорт записывается в журнал аудита.",
  projects: "Проекты",
  projectsHint: "Если ничего не выбрано — все проекты, которые вы видите.",
  from: "Дата начала",
  to: "Дата окончания",
  approvalsOnly: "Только задачи, прошедшие согласование",
  submit: "Экспортировать пакет доказательств",
  exporting: "Пакет готовится…",
  downloaded: "Пакет доказательств скачан",
  limits: (days, items, events) =>
    `Период обязателен: не больше ${n(days, "дня", "дней", "дней")}, ${n(items, "задачи", "задач", "задач")} и ${n(events, "события", "событий", "событий")}.`,
  key: "Ключ подписи",
  fingerprint: "Отпечаток ключа",
  keyHint: "Аудитор проверяет пакет этой командой. Передайте ему отпечаток отдельным каналом (не по email: например, по телефону или подписанным письмом).",
  copy: "Копировать",
  copied: "Скопировано",
  disabled: "Экспорт доказательств отключён: на сервере не задан NEBULA_EVIDENCE_SIGNING_KEY (см. deploy/production/README.md).",
  history: "История экспорта",
  historyEmpty: "Пакеты доказательств ещё не экспортировались.",
  cols: { time: "Время", who: "Кто", what: "Что", content: "Содержимое", manifest: "SHA-256 манифеста" },
  kindItem: (key) => `Задача ${key}`,
  kindScope: (from, to) => `${from} … ${to}`,
  allProjects: "все проекты",
  projectCount: (count) => n(count, "проект", "проекта", "проектов"),
  approvalsTag: "только согласования",
  counts: (items, events) => `${n(items, "задача", "задачи", "задач")}, ${n(events, "событие", "события", "событий")}`,
  itemButton: "Экспорт доказательств",
  itemButtonHint: "Полная история задачи, комментарии, согласования, связи и список файлов с SHA-256: подписанный ZIP",
  errors: {
    required: "Выберите даты начала и окончания.",
    order: "Дата окончания не может быть раньше даты начала.",
    tooLong: (days) => `Период может быть не больше ${n(days, "дня", "дней", "дней")}.`,
    tooLarge: (what) => `Слишком много ${(WHAT as Partial<Record<string, string>>)[what] ?? "записей"} для одного пакета: выберите меньше проектов или более короткий период.`,
    rateLimited: "Слишком много экспортов: попробуйте позже.",
    busy: "Идут другие экспорты: повторите через несколько секунд.",
  },
  act: { exported: "Экспортирован пакет доказательств" },
};
