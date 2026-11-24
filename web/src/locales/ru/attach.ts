// Russian strings for attachments and the Markdown editor (ADR-0022).
import type { Messages } from "../../i18n/locale";

export const ta = {
  attachments: "Файлы",
  attach: "Прикрепить файл",
  dropHere: "Отпустите файлы, чтобы загрузить",
  dropHint: "Перетащите файлы сюда.",
  uploading: (name: string) => `Загрузка: ${name}…`,
  uploaded: (name: string) => `Загружено: ${name}`,
  none: "Файлов нет.",
  download: "Скачать",
  remove: "Удалить файл",
  confirmRemove: (name: string) => `Удалить файл «${name}»? Это действие нельзя отменить.`,
  removed: "Файл удалён",
  tooLarge: (name: string, mb: number) => `${name}: файл больше ${String(mb)} МБ`,
  disabled: "Прикрепление файлов на этом сервере отключено.",
  by: (name: string) => `загрузил(а): ${name}`,
  preview: "Предпросмотр",
  previewOf: (name: string) => `Предпросмотр: ${name}`,
  image: "изображение",
  write: "Текст",
  previewTab: "Предпросмотр",
  nothingToPreview: "Нет текста для предпросмотра.",
  markdownHint: "Поддерживается Markdown: **жирный**, _курсив_, `код`, списки, таблицы, [ссылка](https://…)",
  editDescription: "Редактировать описание",
  noDescription: "Описания нет.",
  save: "Сохранить",
  cancel: "Отмена",
  act: {
    added: (name: string) => `прикрепил(а) файл «${name}»`,
    removed: (name: string) => `удалил(а) файл «${name}»`,
  },
} satisfies Messages["attach"];
