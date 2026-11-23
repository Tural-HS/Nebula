// Azerbaijani strings for project settings and privacy (ADR-0013).
export const tp = {
  settings: "Layihə ayarları",
  private: "Gizli",
  privateProject: "Gizli layihə",
  privateHint: "Yalnız layihə üzvləri və workspace adminləri görür.",
  visibility: "Görünürlük",
  visibilityOptions: { workspace: "Bütün workspace üzvləri", private: "Yalnız layihə üzvləri (gizli)" },
  members: "Layihə üzvləri",
  membersHint: "Workspace adminləri və sahibləri gizli layihələri hər zaman görür; burada siyahıda olmaya bilərlər.",
  addMember: "Üzv əlavə et",
  noMembers: "Hələ heç kim əlavə edilməyib.",
  remove: "Çıxar",
  webhookWarning:
    "Gizli layihənin hadisələri yalnız bu layihəni açıq seçmiş webhook-lara göndərilir (Ayarlar → Webhook-lar → Layihələr). Şöbə kanalı üçün webhook-da bu layihəni seçin.",
  makePrivateConfirm: "Layihə gizli olsun? Layihə üzvü olmayan adi üzvlər onu artıq görməyəcək.",
  adminOnly: "Layihə ayarlarını yalnız adminlər dəyişə bilər.",
} as const;
