// Turkish strings for accepting an invitation (ADR-0016).
import type { Messages } from "../../i18n/locale";

export const ti = {
  title: "Çalışma alanı daveti",
  invitedBy: (who: string, ws: string, role: string) => `${who} sizi "${ws}" çalışma alanına ${role} olarak davet etti.`,
  accept: "Kabul et",
  accepted: "Hoş geldiniz!",
  alreadyMember: "Zaten bu çalışma alanının üyesisiniz.",
  open: "Çalışma alanını aç",
  mismatch: (masked: string) => `Bu davet ${masked} adresi içindir. Bu e-postayla giriş yapın.`,
  unverified: "E-posta adresiniz kimlik sisteminde doğrulanmamış, bu yüzden daveti kabul edemezsiniz.",
  switchAccount: "Başka bir hesapla giriş yap",
  invalid: "Davet bulunamadı: bağlantı hatalı, iptal edilmiş veya yenisi gönderilmiş.",
  used: "Bu davet zaten kullanılmış.",
  expired: "Davetin süresi dolmuş. Yöneticiden yenisini isteyin.",
} satisfies Messages["invite"];
