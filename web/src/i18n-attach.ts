// Azerbaijani strings for attachments and the Markdown editor (ADR-0022).
export const ta = {
  attachments: "Fayllar",
  attach: "Fayl əlavə et",
  dropHere: "Faylları bura atın",
  dropHint: "və ya sürükləyib buraxın",
  uploading: (name: string) => `${name} yüklənir…`,
  uploaded: (name: string) => `${name} yükləndi`,
  none: "Fayl yoxdur.",
  download: "Yüklə",
  remove: "Faylı sil",
  confirmRemove: (name: string) => `"${name}" faylı silinsin? Bu geri qaytarıla bilməz.`,
  removed: "Fayl silindi",
  tooLarge: (name: string, mb: number) => `${name}: fayl ${String(mb)} MB-dan böyükdür`,
  disabled: "Bu serverdə fayl əlavə etmək söndürülüb.",
  by: (name: string) => `yükləyən: ${name}`,
  preview: "Önizləmə",
  previewOf: (name: string) => `${name} önizləməsi`,
  image: "şəkil",
  write: "Yaz",
  previewTab: "Önizləmə",
  nothingToPreview: "Önizləmək üçün mətn yoxdur.",
  markdownHint: "Markdown dəstəklənir: **qalın**, _kursiv_, `kod`, siyahılar, cədvəllər, [keçid](https://…)",
  editDescription: "Təsviri redaktə et",
  noDescription: "Təsvir yoxdur.",
  save: "Yadda saxla",
  cancel: "Ləğv et",
  act: {
    added: (name: string) => `"${name}" faylını əlavə etdi`,
    removed: (name: string) => `"${name}" faylını sildi`,
  },
};

/** Human-readable size: 912 B, 14 KB, 3.4 MB. */
export function formatBytes(n: number): string {
  if (n < 1024) return `${String(n)} B`;
  if (n < 1024 * 1024) return `${String(Math.round(n / 1024))} KB`;
  return `${(n / (1024 * 1024)).toFixed(1).replace(".", ",")} MB`;
}
