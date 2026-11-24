// Azerbaijani strings for subtasks (ADR-0021).
export const ts = {
  section: "Alt tapşırıqlar",
  progress: (done: number, count: number) => `${String(done)}/${String(count)} tamamlanıb`,
  badge: (done: number, count: number) => `Alt tapşırıqlar: ${String(done)}/${String(count)} tamamlanıb`,
  empty: "Alt tapşırıq yoxdur.",
  add: "Alt tapşırıq əlavə et",
  addPlaceholder: "Yeni alt tapşırığın başlığı",
  added: "Alt tapşırıq əlavə edildi",
  parent: "Əsas tapşırıq",
  parentCrumb: "Əsas tapşırığa keç",
  detach: "Əsas tapşırıqdan ayır",
  noNesting: "Alt tapşırığın öz alt tapşırıqları ola bilməz.",
  changedParent: "əsas tapşırığı dəyişdi",
  more: (n: number) => `Daha ${String(n)} alt tapşırığı göstər`,
};
