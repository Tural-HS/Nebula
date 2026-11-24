// Azerbaijani strings for accepting an invitation (ADR-0016).
export const ti = {
  title: "Workspace dəvəti",
  invitedBy: (who: string, ws: string, role: string) => `${who} sizi "${ws}" workspace-inə ${role} kimi dəvət edib.`,
  accept: "Qəbul et",
  accepted: "Xoş gəlmisiniz!",
  alreadyMember: "Siz artıq bu workspace-in üzvüsünüz.",
  open: "Workspace-i aç",
  mismatch: (masked: string) => `Bu dəvət ${masked} ünvanı üçündür. Həmin email ilə daxil olun.`,
  unverified: "Email ünvanınız identifikasiya sistemində təsdiqlənməyib, ona görə dəvəti qəbul etmək olmur.",
  switchAccount: "Başqa hesabla daxil ol",
  invalid: "Dəvət tapılmadı: link səhvdir, ləğv edilib və ya yenisi göndərilib.",
  used: "Bu dəvət artıq istifadə olunub.",
  expired: "Dəvətin müddəti bitib. Admindən yenisini istəyin.",
} as const;
