// Azerbaijani strings for audit evidence export (ADR-0029).

const WHAT = {
  items: "tapşırıq",
  events: "hadisə",
  comments: "şərh",
  approvals: "təsdiq sorğusu",
  attachments: "fayl",
  links: "əlaqə",
  bytes: "məlumat (100 MB-dan çox)",
} as const;

export const te = {
  title: "Audit sübut paketi",
  intro:
    "Auditor və tənzimləyici üçün imzalanmış ZIP: oxunaqlı hesabat (brauzerdə açıb PDF kimi çap edin), JSON və CSV məlumatlar, hər faylın SHA-256-sı olan manifest və onun Ed25519 imzası. Paketə yalnız sizin görə bildiyiniz tapşırıqlar düşür; hər ixrac audit log-a yazılır.",
  projects: "Layihələr",
  projectsHint: "Heç biri seçilməyibsə — görə bildiyiniz bütün layihələr.",
  from: "Başlanğıc tarixi",
  to: "Son tarix",
  approvalsOnly: "Yalnız təsdiqdən keçən tapşırıqlar",
  submit: "Sübut paketini ixrac et",
  exporting: "Paket hazırlanır…",
  downloaded: "Sübut paketi yükləndi",
  limits: (days: number, items: number, events: number) =>
    `Tarix aralığı məcburidir: ən çox ${String(days)} gün, ${String(items)} tapşırıq və ${String(events)} hadisə.`,
  key: "İmza açarı",
  fingerprint: "Açarın barmaq izi",
  keyHint: "Auditor paketi bu əmrlə yoxlayır. Barmaq izini ona ayrıca kanalla (email deyil, məsələn telefonla və ya imzalı məktubla) çatdırın.",
  copy: "Kopyala",
  copied: "Kopyalandı",
  disabled: "Sübut ixracı söndürülüb: serverdə NEBULA_EVIDENCE_SIGNING_KEY təyin edilməyib (bax: deploy/production/README.md).",
  history: "İxrac tarixçəsi",
  historyEmpty: "Hələ sübut paketi ixrac edilməyib.",
  cols: { time: "Vaxt", who: "Kim", what: "Nə", content: "Məzmun", manifest: "Manifest SHA-256" },
  kindItem: (key: string) => `Tapşırıq ${key}`,
  kindScope: (from: string, to: string) => `${from} … ${to}`,
  allProjects: "bütün layihələr",
  projectCount: (n: number) => `${String(n)} layihə`,
  approvalsTag: "yalnız təsdiqlər",
  counts: (items: number, events: number) => `${String(items)} tapşırıq, ${String(events)} hadisə`,
  itemButton: "Sübut ixracı",
  itemButtonHint: "Tapşırığın tam tarixçəsi, şərhləri, təsdiqləri, əlaqələri və faylların SHA-256 siyahısı: imzalanmış ZIP",
  errors: {
    required: "Başlanğıc və son tarixi seçin.",
    order: "Son tarix başlanğıcdan əvvəl ola bilməz.",
    tooLong: (days: number) => `Tarix aralığı ən çox ${String(days)} gün ola bilər.`,
    tooLarge: (what: string) =>
      `Bir paket üçün çox ${(WHAT as Partial<Record<string, string>>)[what] ?? "məlumat"} var: daha az layihə və ya daha qısa aralıq seçin.`,
    rateLimited: "Çox sayda ixrac: bir az sonra yenidən cəhd edin.",
    busy: "Başqa ixraclar gedir: bir neçə saniyədən sonra yenidən cəhd edin.",
  },
  act: { exported: "Sübut paketi ixrac edildi" },
};
