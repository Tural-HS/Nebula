// Azerbaijani strings for item links and watchers (ADR-0028).
export const tl = {
  section: "Əlaqələr",
  /** Group headings, as seen from the open item. */
  types: {
    blocks: "Bloklayır",
    blocked_by: "Bloklanır",
    relates: "Əlaqəlidir",
    duplicates: "Təkrarlayır",
    duplicated_by: "Təkrarlanır",
  },
  empty: "Əlaqə yoxdur.",
  add: "Əlaqə əlavə et",
  type: "Əlaqə növü",
  search: "Tapşırıq axtar",
  searchPlaceholder: "Açar (OPS-12) və ya başlıq",
  noResults: "Uyğun tapşırıq tapılmadı.",
  added: "Əlaqə əlavə edildi",
  exists: "Bu əlaqə artıq var.",
  remove: (key: string) => `${key} ilə əlaqəni sil`,
  closed: "bağlıdır",
  blocked: "Bloklanıb",
  blockedTitle: "Bu tapşırığı açıq tapşırıq bloklayır",
  act: {
    added: (type: string, key: string) => `əlaqə əlavə etdi: ${type} ${key}`,
    removed: (type: string, key: string) => `əlaqəni sildi: ${type} ${key}`,
    removedByDelete: (key: string) => `${key} silindiyi üçün əlaqə silindi`,
  },
  watch: "İzlə",
  unwatch: "İzləmə",
  watchLabel: (watching: boolean, count: number) =>
    watching ? `Siz izləyirsiniz; ${String(count)} izləyən. İzləməni dayandır` : `İzlə; ${String(count)} izləyən`,
  watchers: "İzləyənlər",
  watcherCount: (n: number) => `${String(n)} izləyən`,
  noWatchers: "Heç kim izləmir.",
  addWatcher: "İzləyən əlavə et",
  chooseMember: "Üzv seçin",
  removeWatcher: (name: string) => `${name} izləyənlərdən çıxar`,
};
