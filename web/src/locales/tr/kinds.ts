// Turkish strings for workspace kinds: team templates (ADR-0033).
import type { Messages } from "../../i18n/locale";

export const tk = {
  kind: "Ekip türü",
  kindHint: "Her şey daha sonra Ayarlar'dan değiştirilebilir.",
  names: {
    general: "Genel",
    finance: "Finans",
    it: "BT",
    hr: "İK",
  },
  descriptions: {
    general: "Yalnızca durumlar ve iş türleri (Görev, Hata). Gerisini siz kurarsınız.",
    finance: "Ödeme, fatura, rapor; Tutar, KDV ve Karşı taraf alanları; \"Ödendi\" için ikinci yönetici onayı; \"Finans\" projesi ve \"Ödeme talebi\" formu.",
    it: "Hata, görev, talep; Öncelik ve Bileşen alanları; \"İşlerim\" ve \"Kritik\" görünümleri ve BT erişim talebi formu.",
    hr: "İşe alım ve izin; gizli \"İnsan Kaynakları\" projesi, izin talebi ve yeni çalışan formları.",
  },
  calendarNote: "Azerbaycanca şablonlu bir çalışma alanına bu yılın resmi iş günü takvimi de eklenir.",
  approvalNote: "\"Ödendi\" durumuna geçiş başka bir yönetici veya sahibin onayını gerektirir: en az bir yönetici daha davet edin.",
} satisfies Messages["kinds"];
