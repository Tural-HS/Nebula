// Turkish strings for attachments and the Markdown editor (ADR-0022).
import type { Messages } from "../../i18n/locale";

export const ta = {
  attachments: "Dosyalar",
  attach: "Dosya ekle",
  dropHere: "Dosyaları buraya bırakın",
  dropHint: "veya sürükleyip bırakın",
  uploading: (name: string) => `${name} yükleniyor…`,
  uploaded: (name: string) => `${name} yüklendi`,
  none: "Dosya yok.",
  download: "İndir",
  remove: "Dosyayı sil",
  confirmRemove: (name: string) => `"${name}" dosyası silinsin mi? Bu işlem geri alınamaz.`,
  removed: "Dosya silindi",
  tooLarge: (name: string, mb: number) => `${name}: dosya ${String(mb)} MB'tan büyük`,
  disabled: "Bu sunucuda dosya ekleme kapalı.",
  by: (name: string) => `yükleyen: ${name}`,
  preview: "Önizleme",
  previewOf: (name: string) => `${name} önizlemesi`,
  image: "görsel",
  write: "Yaz",
  previewTab: "Önizleme",
  nothingToPreview: "Önizlenecek metin yok.",
  markdownHint: "Markdown desteklenir: **kalın**, _italik_, `kod`, listeler, tablolar, [bağlantı](https://…)",
  editDescription: "Açıklamayı düzenle",
  noDescription: "Açıklama yok.",
  save: "Kaydet",
  cancel: "İptal",
  act: {
    added: (name: string) => `"${name}" dosyasını ekledi`,
    removed: (name: string) => `"${name}" dosyasını sildi`,
  },
} satisfies Messages["attach"];
