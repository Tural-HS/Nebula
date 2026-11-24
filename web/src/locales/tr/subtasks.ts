// Turkish strings for subtasks (ADR-0021).
import type { Messages } from "../../i18n/locale";

export const ts = {
  section: "Alt görevler",
  progress: (done: number, count: number) => `${String(done)}/${String(count)} tamamlandı`,
  badge: (done: number, count: number) => `Alt görevler: ${String(done)}/${String(count)} tamamlandı`,
  empty: "Alt görev yok.",
  add: "Alt görev ekle",
  addPlaceholder: "Yeni alt görevin başlığı",
  added: "Alt görev eklendi",
  parent: "Üst görev",
  parentCrumb: "Üst göreve git",
  detach: "Üst görevden ayır",
  noNesting: "Bir alt görevin kendi alt görevleri olamaz.",
  changedParent: "üst görevi değiştirdi",
  more: (n: number) => `${String(n)} alt görev daha göster`,
} satisfies Messages["subtasks"];
