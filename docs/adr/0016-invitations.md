# ADR-0016: Workspace dəvətləri

- Status: Qəbul edilib
- Tarix: 2026-11-23

## Kontekst

İndiyə qədər admin bir şəxsi workspace-ə yalnız o şəxs Nebula-ya **ən azı bir dəfə daxil olubsa** əlavə edə bilirdi
("no active user with this email has signed in yet"). Yeni işçi üçün bu əlavə addım idi: əvvəlcə ona giriş etdirmək,
sonra adminə xəbər vermək.

## Qərarlar

| Qərar | Səbəb |
|---|---|
| Admin email + rol ilə dəvət yaradır; Nebula email ilə **tək istifadəlik link** göndərir | Şəxsin əvvəlcədən hesabı olması lazım deyil: linkə klikləyir, IdP-də (Keycloak, SSO) daxil olur və ya qeydiyyatdan keçir, qəbul edir |
| Token: 32 təsadüfi bayt (base64url); bazada yalnız **SHA-256 heşi** saxlanılır; link yalnız email-də olur | Baza sızsa belə, aktiv dəvətlər istifadə edilə bilməz |
| Qəbul üçün **həm token, həm də IdP-nin təsdiqlədiyi email** (`email_verified = true`) dəvətdəki email ilə uyğun gəlməlidir (böyük/kiçik hərfə həssas deyil) | Link başqasına göndərilsə (yönləndirilmiş email, ekran görüntüsü) həmin şəxs qoşula bilməz; təsdiqlənməmiş email ilə özgə ünvanı "iddia etmək" mümkün deyil |
| Müddət 7 gün; tək istifadə; admin ləğv edə bilər; "yenidən göndər" köhnə tokeni etibarsız edir | Köhnəlmiş linklər işləmir |
| Rol: dəvət edən öz rolundan yüksək rol verə bilməz; `owner` rolunu yalnız sahib verə bilər | Mövcud üzv idarəetməsi qaydaları ilə eyni (ADR-0002) |
| Eyni workspace + email üçün bir gözləyən dəvət; artıq üzv olana dəvət yoxdur | Spam və qarışıqlıq yoxdur |
| Limitlər: workspace başına ən çox 200 gözləyən dəvət, gündə 100 yeni dəvət | Nebula-nın SMTP-si spam üçün istifadə olunmasın |
| Email outbox ilə (eyni tranzaksiyada) göndərilir | Dəvət yazılıbsa, email də mütləq gedir; uğursuzluqda təkrar cəhd |
| Hadisələr: `invitation.created/resent/revoked/accepted` (tokensiz) | Audit: kim kimi, hansı rolla dəvət edib, kim nə vaxt qoşulub |
| Mövcud "birbaşa əlavə et" (artıq daxil olmuş istifadəçi üçün) qalır | Komanda daxilində sürətli yol; dəvət yeni şəxslər üçündür |
| Link `NEBULA_PUBLIC_URL/invite/{token}`; public URL konfiqurasiya edilməyibsə dəvət yaradıla bilməz (aydın xəta) | Səhv hosta (məs. localhost) link göndərilməsin |

## Axın

1. Admin: Ayarlar → Üzvlər → "Dəvət et" (email, rol) → email gedir; gözləyən dəvətlər siyahıda (ləğv et / yenidən göndər).
2. Dəvət olunan: linkə klikləyir → Nebula giriş səhifəsi (IdP) → `/invite/{token}` səhifəsi: workspace adı, dəvət edən, rol → "Qəbul et".
3. Email uyğun gəlmirsə (başqa hesabla daxil olub), səhifə maskalanmış ünvanı göstərir (`t***@fintez.az`) və hesabı dəyişməyi təklif edir.

## Nəticələr

- IdP `email_verified` iddiasını access token-ə qoymalıdır (Keycloak-da `email` scope-u ilə default belədir). Qoymursa, dəvət qəbul edilmir və xəta bunu açıq deyir.
- Workspace-ə qoşulma həmişə istifadəçinin öz hərəkəti ilədir (klik); avtomatik qoşulma yoxdur.
