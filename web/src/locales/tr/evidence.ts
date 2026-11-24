// Turkish strings for audit evidence export (ADR-0029).
import type { Messages } from "../../i18n/locale";

const WHAT = {
  items: "görev",
  events: "olay",
  comments: "yorum",
  approvals: "onay talebi",
  attachments: "dosya",
  links: "bağlantı",
  bytes: "veri (100 MB'tan fazla)",
} as const;

export const te: Messages["evidence"] = {
  title: "Denetim kanıt paketi",
  intro:
    "Denetçiler ve düzenleyiciler için imzalı ZIP: okunabilir rapor (tarayıcıda açıp PDF olarak yazdırın), JSON ve CSV veriler, her dosyanın SHA-256 değerini içeren manifest ve Ed25519 imzası. Pakete yalnızca görebildiğiniz görevler girer; her dışa aktarma denetim günlüğüne yazılır.",
  projects: "Projeler",
  projectsHint: "Hiçbiri seçilmezse: görebildiğiniz tüm projeler.",
  from: "Başlangıç tarihi",
  to: "Bitiş tarihi",
  approvalsOnly: "Yalnızca onaydan geçen görevler",
  submit: "Kanıt paketini dışa aktar",
  exporting: "Paket hazırlanıyor…",
  downloaded: "Kanıt paketi indirildi",
  limits: (days, items, events) => `Tarih aralığı zorunludur: en fazla ${String(days)} gün, ${String(items)} görev ve ${String(events)} olay.`,
  key: "İmza anahtarı",
  fingerprint: "Anahtar parmak izi",
  keyHint: "Denetçi paketi bu komutla doğrular. Parmak izini ayrı bir kanaldan iletin (e-posta değil: örneğin telefonla veya imzalı bir yazıyla).",
  copy: "Kopyala",
  copied: "Kopyalandı",
  disabled: "Kanıt dışa aktarma kapalı: sunucuda NEBULA_EVIDENCE_SIGNING_KEY ayarlanmamış (bkz. deploy/production/README.md).",
  history: "Dışa aktarma geçmişi",
  historyEmpty: "Henüz kanıt paketi dışa aktarılmadı.",
  cols: { time: "Zaman", who: "Kim", what: "Ne", content: "İçerik", manifest: "Manifest SHA-256" },
  kindItem: (key) => `Görev ${key}`,
  kindScope: (from, to) => `${from} … ${to}`,
  allProjects: "tüm projeler",
  projectCount: (count) => `${String(count)} proje`,
  approvalsTag: "yalnızca onaylar",
  counts: (items, events) => `${String(items)} görev, ${String(events)} olay`,
  itemButton: "Kanıt dışa aktar",
  itemButtonHint: "Görevin tüm geçmişi, yorumları, onayları, bağlantıları ve dosyalarının SHA-256 listesi: imzalı ZIP",
  errors: {
    required: "Başlangıç ve bitiş tarihini seçin.",
    order: "Bitiş tarihi başlangıçtan önce olamaz.",
    tooLong: (days) => `Tarih aralığı en fazla ${String(days)} gün olabilir.`,
    tooLarge: (what) => `Tek paket için çok fazla ${(WHAT as Partial<Record<string, string>>)[what] ?? "kayıt"} var: daha az proje veya daha kısa bir aralık seçin.`,
    rateLimited: "Çok fazla dışa aktarma: daha sonra tekrar deneyin.",
    busy: "Başka dışa aktarmalar sürüyor: birkaç saniye sonra tekrar deneyin.",
  },
  act: { exported: "Kanıt paketi dışa aktarıldı" },
};
