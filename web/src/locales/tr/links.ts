// Turkish strings for item links and watchers (ADR-0028).
import type { Messages } from "../../i18n/locale";

export const tl = {
  section: "Bağlantılar",
  types: {
    blocks: "Engelliyor",
    blocked_by: "Engelleniyor",
    relates: "İlişkili",
    duplicates: "Kopyası",
    duplicated_by: "Kopyası var",
  },
  empty: "Bağlantı yok.",
  add: "Bağlantı ekle",
  type: "Bağlantı türü",
  search: "Görev ara",
  searchPlaceholder: "Anahtar (OPS-12) veya başlık",
  noResults: "Eşleşen görev bulunamadı.",
  added: "Bağlantı eklendi",
  exists: "Bu bağlantı zaten var.",
  remove: (key: string) => `${key} ile bağlantıyı kaldır`,
  closed: "kapalı",
  blocked: "Engellendi",
  blockedTitle: "Bu görevi açık bir görev engelliyor",
  act: {
    added: (type: string, key: string) => `bağlantı ekledi: ${type} ${key}`,
    removed: (type: string, key: string) => `bağlantıyı kaldırdı: ${type} ${key}`,
    removedByDelete: (key: string) => `${key} silindiği için bağlantı kaldırıldı`,
  },
  watch: "İzle",
  /** Visible label while the viewer watches (a pressed toggle shows state, not an action). */
  watching: "Takip ediliyor",
  watchLabel: (watching: boolean, count: number) =>
    watching ? `İzliyorsunuz; ${String(count)} izleyici. İzlemeyi bırak` : `İzle; ${String(count)} izleyici`,
  watchers: "İzleyiciler",
  watcherCount: (n: number) => `${String(n)} izleyici`,
  noWatchers: "Kimse izlemiyor.",
  addWatcher: "İzleyici ekle",
  chooseMember: "Üye seçin",
  removeWatcher: (name: string) => `${name} kişisini izleyicilerden çıkar`,
} satisfies Messages["links"];
