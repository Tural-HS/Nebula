// Turkish strings for workspace/project templates and the Mattermost
// Boards import (ADR-0020).
import type { Messages } from "../../i18n/locale";

export const tt = {
  template: "Başlangıç şablonu",
  templateHint: "Durumlar ve iş türleri sonradan Ayarlar'dan değiştirilebilir.",

  projectTemplate: "Şablon proje",
  projectTemplateNone: "Yok (boş proje)",
  projectTemplateHint: "Seçilen projeden aşağıdakiler kopyalanır. Görevler kopyalanmaz.",
  copyMembers: "Proje üyeleri",
  copyRecurrences: "Tekrarlanan görevler",
  copyViews: "Paylaşılan görünümler",
} satisfies Messages["templates"];
