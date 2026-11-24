// Russian strings for the spreadsheet import wizard.
import type { FieldType } from "../../api/types";
import type { Messages } from "../../i18n/locale";
import { plural } from "../../i18n/plural";

const rowsWord = (n: number) => plural("ru", n, { one: "строка", few: "строки", many: "строк", other: "строки" });
const rowsPrep = (n: number) => plural("ru", n, { one: "строке", few: "строках", many: "строках", other: "строках" });
const errorsWord = (n: number) => plural("ru", n, { one: "ошибка", few: "ошибки", many: "ошибок", other: "ошибки" });
const cards = (n: number) => plural("ru", n, { one: "карточка", few: "карточки", many: "карточек", other: "карточки" });
const itemsAcc = (n: number) => plural("ru", n, { one: "задачу", few: "задачи", many: "задач", other: "задачи" });
const itemsWord = (n: number) => plural("ru", n, { one: "задача", few: "задачи", many: "задач", other: "задачи" });
const fmt = (n: number) => n.toLocaleString("ru-RU");

export const ti = {
  importAction: "Импорт",
  title: "Импорт из таблицы",
  intro: "Добавьте в этот проект задачи из CSV (UTF-8), Excel (.xlsx) или архива Mattermost Boards (.boardarchive). Файл на сервере только читается и не сохраняется.",
  steps: ["Файл", "Столбцы", "Проверка", "Результат"] as const,
  stepLabel: (n: number, of: number) => `Шаг ${String(n)} из ${String(of)}`,

  file: "Файл",
  chooseFile: "Выберите файл",
  fileHint: (maxMb: number, maxRows: number) =>
    `.csv, .tsv, .xlsx или .boardarchive; не более: таблица — ${String(maxMb)} МБ, архив Boards — 50 МБ, ${fmt(maxRows)} ${rowsWord(maxRows)}. Первая строка таблицы — заголовки.`,
  readFile: "Прочитать файл",
  reading: "Чтение…",
  fileProblem: {
    extension: "Этот тип файла не поддерживается. Выберите .xlsx, CSV (UTF-8) или .boardarchive.",
    size: "Файл слишком большой (таблица — 10 МБ, архив Boards — 50 МБ). Разделите файл.",
    empty: "Файл пуст.",
  },

  sheet: "Лист",
  board: "Доска",
  peopleTitle: "Пользователи Mattermost",
  peopleHint: "В архиве нет имён и email, только коды пользователей. Посмотрите на карточки и сопоставьте каждого с участником Nebula: он станет исполнителем и автором комментариев. Несопоставленные останутся пустыми.",
  personLabel: (n: number, samples: string[]) => `${String(n)} ${cards(n)}: ${samples.join(", ")}${n > samples.length ? "…" : ""}`,
  personNone: "— оставить пустым —",
  personPrivateHint: "В закрытом проекте исполнитель должен быть участником проекта.",
  summary: (rows: number, file: string) => `${file}: ${fmt(rows)} ${rowsWord(rows)}`,
  delimiter: (d: string) => `Разделитель: ${d === "\t" ? "табуляция" : d}`,
  defaultType: "Тип (если не указан в столбце)",
  mappingTitle: "Сопоставьте столбцы с полями",
  mappingHint: "Предложения заполнены по названиям столбцов. Несопоставленные столбцы не учитываются.",
  column: "Столбец",
  sample: "Пример",
  target: "Поле Nebula",
  ignore: "— не учитывать —",
  builtinGroup: "Основные",
  fieldsGroup: "Пользовательские поля",
  missingTitle: "Выберите столбец для заголовка.",
  previewTitle: "Первые строки",
  row: "Строка",
  back: "Назад",
  validate: "Проверить",
  validating: "Проверка…",

  reportOk: (n: number) => `Все строки (${fmt(n)}) корректны. Можно импортировать.`,
  reportErrors: (rows: number, errors: number) =>
    `В ${fmt(rows)} ${rowsPrep(rows)} — ${fmt(errors)} ${errorsWord(errors)}. Исправьте файл или измените сопоставление; при наличии ошибок ничего не импортируется.`,
  truncated: (shown: number) => `Показаны первые ${String(shown)} ${errorsWord(shown)}.`,
  errorsTable: "Ошибки в строках",
  message: "Ошибка",
  importNow: (n: number) => `Импортировать ${fmt(n)} ${itemsAcc(n)}`,
  importing: "Импорт…",
  fixMapping: "Вернуться к сопоставлению",

  done: (n: number) => `${plural("ru", n, { one: "Создана", few: "Созданы", many: "Создано", other: "Создано" })} ${fmt(n)} ${itemsWord(n)}`,
  doneRange: (first: string, last: string) => `${first} – ${last}`,
  openProject: "Перейти в проект",
  importAnother: "Импортировать другой файл",
  rejected: "Импорт отклонён: в некоторых строках есть ошибки, ничего не записано.",
  adminOnly: "Импорт доступен только администраторам.",

  builtin: {
    title: "Заголовок",
    description: "Описание",
    type: "Тип",
    status: "Статус",
    assignee: "Исполнитель (email)",
    due_date: "Срок",
  },

  fieldType: {
    text: "текст",
    long_text: "длинный текст",
    number: "число",
    date: "дата",
    datetime: "дата и время",
    boolean: "да/нет",
    select: "список",
    multi_select: "множественный выбор",
    user: "пользователь (email)",
    url: "URL",
    email: "email",
  } satisfies Record<FieldType, string>,

  // Server error codes (ADR-0011) → messages.
  errors: {
    file_too_large: "Файл слишком большой (лимит 10 МБ, для распакованного .xlsx — 64 МБ).",
    too_many_rows: "В таблице больше 20 000 строк. Разделите файл.",
    too_many_columns: "В таблице больше 200 столбцов.",
    cell_too_long: "Одна из ячеек длиннее 32 767 символов.",
    invalid_encoding: "Файл не в кодировке UTF-8. Сохраните его в Excel как «CSV UTF-8».",
    unsupported_format: "Этот формат не поддерживается. Используйте .xlsx или CSV (UTF-8). Старые файлы .xls и зашифрованные файлы не читаются.",
    malformed_file: "Не удалось прочитать файл (повреждён или неверный формат).",
    empty_file: "В файле нет строк с данными.",
    sheet_not_found: "Выбранный лист не найден.",
    timeout: "Чтение файла заняло слишком много времени.",
    import_busy: "Сейчас выполняются другие импорты. Попробуйте ещё раз чуть позже.",
  },
} as const satisfies Messages["importer"];
