# ADR-0025: İnterfeys dilləri (azərbaycanca, ingiliscə, türkcə, rusca)

- Status: Qəbul edilib
- Tarix: 2026-11-24

## Kontekst

Veb interfeysin bütün mətni azərbaycancadır (~700 sətir, 14 lüğət modulu); bəzi mətnlər komponentlərin içində
idi ("Bəli/Xeyr", audit cədvəlinin başlıqları), tarixlər isə əllə azərbaycanca formatlanırdı. FinTez-də və
gələcək müştərilərdə rus və türkdilli əməkdaşlar, həmçinin ingiliscə işləyən komandalar var. Workspace
şablonu (ADR-0020) yalnız az/en idi.

## Qərarlar

| Qərar | Səbəb | Alternativ və niyə yox |
|---|---|---|
| Dillər: `az` (default və mənbə), `en`, `tr`, `ru` | İstifadəçilərin danışdığı dillər | — |
| Lüğətlər `web/src/locales/<dil>/<modul>.ts`; köhnə `i18n-*.ts` faylları aktiv dilin lüğətini ixrac edən nazik fasaddır, komponentlər dəyişmir | 45 faylda importlar və `t.x` oxunuşu olduğu kimi qalır; diff kiçikdir | i18next/react-intl: əlavə asılılıq və bütün komponentlərin yenidən yazılması, fayda isə eynidir |
| Tip yoxlaması: `Messages = Widen<typeof az>`; hər dil `satisfies Messages[...]` | Açar, massiv uzunluğu və funksiya parametrləri tsc ilə yoxlanılır: unudulmuş tərcümə build-i dayandırır | Runtime yoxlaması və ya "fallback to az": səhv istehsalda görünərdi |
| Azərbaycanca əsas bundle-dadır; `en`/`tr`/`ru` ayrı chunk-dır və **tətbiq modulundan əvvəl** yüklənir (`main.tsx` → `loadMessages()` → `import("./bootstrap")`) | Hər istifadəçi yalnız öz dilini endirir; lüğətlər sadə modul sabitləri olaraq qalır | Bütün dilləri bundle-a qoymaq: hər kəs üçün ~3× artıq mətn |
| Dil dəyişəndə seçim `localStorage`-a yazılır və səhifə **yenilənir** | Modul sabitlərində heç bir köhnə mətn qalmır; dil nadir dəyişdirilir. Yeniləmə IdP SSO sessiyası ilə səssiz girişdir | React context ilə canlı dəyişmə: bütün komponentlər və modul səviyyəli sabitlər yenidən yazılmalı olardı |
| Seçim ardıcıllığı: saxlanılmış seçim → brauzerin ilk dəstəklənən dili (`navigator.languages`) → `az` | Standart davranış; FinTez-də dil seçilməyibsə brauzer dili götürülür | Həmişə `az`: rus/türkdilli istifadəçi əvvəlcə dili tapmalı olardı |
| Dil chunk-ı yüklənməzsə tətbiq azərbaycanca açılır (xəta konsola yazılır) | Şəbəkə xətası interfeysi bağlamamalıdır | — |
| Başlıqda dil seçici (AZ/EN/TR/RU, hər seçimin adı öz dilində ekran oxuyucu üçün) | Telefonda başlıq 375 px-ə sığır (ADR-0019) | İstifadəçi profili səhifəsi: hələ yoxdur |
| Tarix/vaxt: `az` əllə (Chromium-un `az` Intl məlumatı natamamdır), digərləri `Intl.DateTimeFormat`/`RelativeTimeFormat`; ölçülər dilin onluq ayırıcısı ilə | Hər dildə düzgün format | — |
| Rus və ingilis cəm formaları `Intl.PluralRules` ilə (`i18n/plural.ts`) | "1 задача / 3 задачи / 5 задач" | Sabit "N задач(и)": oxunmaz |
| Keycloak giriş səhifəsi eyni dildə: realm-da `internationalizationEnabled`, `supportedLocales: az,en,tr,ru` (Keycloak 26.8.2-də dördü də var), giriş sorğusunda `ui_locales` | İstifadəçi giriş və tətbiq arasında dil dəyişməsini görmür | — |
| Workspace şablonları `tr` və `ru` əlavə olundu (statuslar, iş növləri, bildiriş qaydaları və email mətni); yeni workspace formasında default — interfeysin dili | Rusca interfeysdə yaradılan workspace rusca statuslarla açılır. API default-u `en` qalır (ADR-0020) | — |

## Bu ADR-in hissəsi olmayanlar

- **Serverin yaratdığı mətnlər** azərbaycanca qalır: dəvət emaili, Mattermost/Slack mesajları, gecikmə xülasəsi,
  təkrarlanan tapşırığın `{{month}}` ay adı. Onlar istifadəçiyə yox, workspace-ə/kanala gedir; bunun üçün
  workspace dili sütunu (miqrasiya) ayrıca iş olacaq.
- Dil seçimi cihaz üzrədir (`localStorage`); hesaba bağlı seçim (Keycloak `locale` atributu) sonradan.
- İstifadəçilərin daxil etdiyi məlumat (status adları, sahələr) tərcümə olunmur.

## Nəticələr

- Yeni mətn əlavə edən hər dəyişiklik dörd lüğəti də yeniləməlidir — əks halda `tsc` xəta verir.
- E2E testləri `az-AZ` brauzer dili ilə işləyir (Playwright `locale`), vitest isə lüğət yüklənmədən `az` görür.
- Mövcud production realm-larında Keycloak ayarı əllə yenilənməlidir (realm yalnız ilk dəfə import olunur):
  `deploy/production/README.md` §3.
