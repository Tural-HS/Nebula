// Turkish strings for project settings and privacy (ADR-0013).
import type { Messages } from "../../i18n/locale";

export const tp = {
  settings: "Proje ayarları",
  private: "Gizli",
  privateProject: "Gizli proje",
  privateHint: "Yalnızca proje üyeleri ve çalışma alanı yöneticileri görür.",
  visibility: "Görünürlük",
  visibilityOptions: { workspace: "Tüm çalışma alanı üyeleri", private: "Yalnızca proje üyeleri (gizli)" },
  members: "Proje üyeleri",
  membersHint: "Çalışma alanı yöneticileri ve sahipleri gizli projeleri her zaman görür; burada listede olmayabilirler.",
  addMember: "Üye ekle",
  noMembers: "Henüz kimse eklenmedi.",
  remove: "Kaldır",
  webhookWarning:
    "Gizli projenin olayları yalnızca bu projeyi açıkça seçmiş webhook'lara gönderilir (Ayarlar → Webhook'lar → Projeler). Bir departman kanalı için webhook'ta bu projeyi seçin.",
  makePrivateConfirm: "Proje gizli yapılsın mı? Proje üyesi olmayan normal üyeler artık onu göremeyecek.",
  adminOnly: "Proje ayarlarını yalnızca yöneticiler değiştirebilir.",
  switchProject: "Proje değiştir",
  privateMark: "gizli proje",
} satisfies Messages["project"];
