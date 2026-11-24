# ADR-0035: Rəng rejimi, workspace vurğu rəngi və tapşırıq siyahısı/panelinin yeni dizaynı

- Status: Qəbul edilib — rəng rejimi, vurğu rəngi (miqrasiya 00028) və "Ledger" dizaynı (B-dən iki elementlə)
- Tarix: 2026-11-24

## Kontekst

İstifadəçilər (maliyyə və back-office) bütün günü cədvəldə işləyir. Üç problem var:

1. Tünd rejim yalnız əməliyyat sisteminin parametrinə (`prefers-color-scheme`) bağlıdır: gün ərzində açıq, axşam tünd
   işləmək istəyən, ya da sistemi tünd olub Nebula-nı açıq görmək istəyən seçə bilmir.
2. Bir neçə workspace-də işləyən istifadəçi onları rəngdən ayırd edə bilmir (hamısı eyni firuzəyi).
3. Tapşırıq siyahısı və paneli zəif yerləri: filtr paneli sətirlərə bölünür (görünüş seçimi və "yadda saxla" üst-üstə,
   bir sırada çoxlu select), başlıq xanasında nişanlar (gecikmə, bloklanma, təsdiq, alt tapşırıqlar) bir-biri ilə
   yarışır, panel isə qeyri-bərabər iyerarxiyalı uzun bir sütundur.

Bu ADR (1) və (2)-ni həll edir; (3) üçün iki dizayn istiqaməti təqdim edir, seçim istifadəçinindir.

## Rəng rejimi (həyata keçirilib)

| Qərar | Səbəb | Alternativ və niyə yox |
|---|---|---|
| Üç seçim: **Sistem** (standart), **Açıq**, **Tünd**; brauzer başına `localStorage` `nebula.theme` (dil kimi, ADR-0025). "Sistem" seçiləndə açar silinir | Serverə, sxemə və sinxronizasiyaya ehtiyac yoxdur; seçim cihaza aiddir (ofis monitoru açıq, noutbuk axşam tünd) | İstifadəçi profilində saxlamaq: miqrasiya və API lazımdır, cihaz fərqini itirir |
| Tailwind 4 `dark:` variantı **`<html>`-də `dark` sinfinə** bağlanır (`@custom-variant dark (&:where(.dark, .dark *))`); "Sistem" üçün sinfi `matchMedia` qoyur | Mövcud ~300 `dark:` sinfi dəyişmədən işləyir; "Sistem" canlı izlənir (OS axşam tünd rejimə keçəndə səhifə də keçir) | Media query strategiyası: istifadəçi seçimi mümkün deyil. `data-theme` atributu: eyni effekt, amma sinif Tailwind sənədlərindəki standartdır |
| Dəyişiklik **yenidən yükləmədən** tətbiq olunur (`setTheme` → sinif, `color-scheme`, `theme-color`) | Dildən fərqli olaraq heç bir mətn modul sabitində saxlanmır | Dil kimi reload: lazımsız, açıq formalar itər |
| İlk boyanmadan əvvəl tətbiq: `index.html`-də `<head>` içində **ayrıca same-origin klassik skript** `/theme-init.js` (sinxron, ~0,6 KB) | CSP `script-src 'self'` inline skripti bloklayır. Same-origin fayl CSP-ni dəyişmədən icazəlidir; sinxron olduğu üçün tünd rejimdə ağ yanıb-sönmə olmur | Hash-li inline skript: hər dəyişiklikdə Caddyfile-da hash-i yeniləmək lazımdır (unudulsa, səssizcə FOUC). Rejimi React bootstrap-da qoymaq: config.json və dil yüklənənə qədər ağ ekran görünür |
| `theme-init.js` `src/lib/theme.ts`-in məntiqini təkrarlayır; vitest **həmin faylı** eyni hallarla icra edib nəticəni `resolveTheme` ilə müqayisə edir | İki kopya ayrıla bilməz — test sınar | Vite ilə ayrıca bundle: bir neçə sətir üçün build mürəkkəbliyi |
| Seçici: başlıqda dil seçicisinin yanında, **yalnız xl (≥1280 px)**; daha dar ekranlarda workspace siyahısının altında (telefonda dil seçicisi ilə yanaşı) | ADR-0019: telefon başlığı dolu; 1024 px planşet başlığı layihə əməliyyatları ilə artıq doludur (etiketlər də yalnız xl-dən görünür) | Başlıqda hər ölçüdə: 1024 px-də üfüqi daşma riski |
| `localStorage` bloklananda seçim səhifə açıq qaldıqca işləyir, reload-dan sonra "Sistem"-ə qayıdır; digər tabların `storage` hadisəsi izlənir | Gizli rejim / sərt məxfilik parametrləri tətbiqi sındırmamalıdır | — |

`meta theme-color` (mobil brauzer zolağı) seçilmiş rejimə uyğun yenilənir.

## Workspace vurğu rəngi

| Qərar | Səbəb | Alternativ və niyə yox |
|---|---|---|
| Admin **7 seçilmiş** rəngdən birini seçir: Firuzəyi (standart, loqo), Okean, İndiqo, Bənövşəyi, Gavalı, Meşə, Qrafit. Sərbəst HEX yoxdur | Hər şkala əvvəlcədən kontrast üçün yoxlanıb; sərbəst rəng kontrastı və brendi poza bilər | Rəng seçicisi (HEX): əlçatanlığı serverdə yoxlamaq lazımdır, nəticə çox vaxt oxunmur |
| **Qırmızı və kəhrəba tonları yoxdur** | Cədvəldə qırmızı = gecikmə, kəhrəba = təsdiq gözləyir. Vurğu siqnalla qarışmamalıdır | — |
| Hər şkala 11 addımdır (50…950, OKLCH, sRGB gamut daxilində) və `accent.test.ts` WCAG 2.x yoxlayır: ağ mətn 600 üzərində və 600 ağ üzərində ≥ 4,5:1; 700 mətn 50 fonunda ≥ 4,5:1; tünd rejimdə 300/400 mətn zinc-900 üzərində ≥ 4,5:1, 100 zinc-950 üzərində ≥ 4,5:1; 500 fokus halqası ağ və zinc-950 üzərində ≥ 3:1 | Yoxlama koddadır, rəng dəyişəndə CI sınar | Əl ilə yoxlama: unudulur |
| Tətbiq: workspace marşrutunda (`/w/:ws/...`) `<html>`-in `--color-brand-*` dəyişənləri **CSSOM ilə** əvəzlənir; başqa səhifələrdə silinir | Tailwind 4 utilitləri `var(--color-brand-600)` oxuyur — heç bir komponent dəyişmir. CSSOM CSP `style-src 'self'` ilə icazəlidir (inline `style=""` atributu deyil) | Hər komponentə `accent` prop: yüzlərlə dəyişiklik. Workspace başına ayrıca CSS faylı: build-də dinamik deyil |
| Firuzəyi şkala `index.css`-dəki brend tokenləri ilə **eynidir** (test yoxlayır). 200/300/400/800/950 addımları əlavə olundu | Əvvəl `dark:text-brand-300`, `dark:bg-brand-950/40` kimi siniflər mövcud olmayan tokenə istinad edirdi və heç nə çəkmirdi (tünd rejimdə bəzi seçilmiş elementlər fonsuz idi) | — |
| Saxlama: `workspaces.accent text NULL` + adlı CHECK `workspaces_accent_check` (standartdan başqa 6 açar; firuzəyi = NULL), `PATCH /workspaces/{ws}` `{accent}` (admin+, mövcud yol), açarı əvvəlcə server yoxlayır (422), CHECK ehtiyat xəttdir; `GET` cavabında `accent`; `workspace.updated` hadisəsi `{"accent": {"from": "teal", "to": "violet"}}` yazır (standart `null` yox, `teal` adlanır) | Hamı üçün eyni olmalıdır (admin seçir) — `localStorage` olmaz. Mövcud ümumi "workspace settings" mexanizmi (jsonb) yoxdur | jsonb `settings` sütunu: bir sahə üçün sxemsiz konteyner, validasiya zəifləyir |

Seçici "Ümumi" parametrlər səhifəsindədir (admin+) və seçimdə dərhal saxlayır. API `accent` sahəsini qaytarmırsa
(köhnə backend) seçici gizlənir.

### Miqrasiya 00028

`ALTER TABLE workspaces ADD COLUMN accent text NULL CONSTRAINT workspaces_accent_check CHECK (accent IN ('ocean',
'indigo','violet','plum','forest','graphite'))`. Default-suz NULL sütun **yalnız metadata dəyişikliyidir**: `workspaces`-ə
millisaniyəlik `ACCESS EXCLUSIVE`, cədvəl yenidən yazılmır və skan edilmir; `lock_timeout` 5 s. Down: sütun silinir,
yalnız vurğu seçimi itir (iz `events`-də qalır); köhnə image sütunu oxumur, hər iki sxemlə işləyir.

## Tapşırıq siyahısı və panel — iki istiqamət

Maketlər: `docs/design/round7/a-ledger.html`, `b-queue.html` (statik, sintetik məlumat; `?theme=dark`,
`?view=list|drawer`). Ekran görüntüləri (1280 və 375 px, açıq və tünd) eyni qovluqdadır və repoya commit edilmir.

Hər ikisində ümumi: brend palitrası və loqo qalır; başlıqlar böyük hərflə deyil; açar (FIN-209) monospace deyil,
`tabular-nums`; məbləğlər sağa düzlənir, az-AZ formatı (`184 250,40`), cəmi sabit aşağı sətirdədir; "gecikib"
**iş günləri** ilə (ADR-0027); nişanlar başlıqdan çıxarılır.

### A — "Ledger" (sıx, cədvəl mərkəzli)

- Toolbar **bir sətir, heç vaxt sınmır**: saxlanmış görünüşlər tab kimi (dəyişibsə nöqtə), filtrlər axtarış sahəsinin
  içində token kimi (`Status: İcrada, Yoxlamada ✕`), sıralama, sütunlar, say, "Yeni tapşırıq".
- Sətrin solunda **siqnal relsi** (4 px): qırmızı = gecikib, kəhrəba = təsdiq gözləyir, ştrixli = bloklanıb.
  Detallar ayrıca "Siqnal" sütununda **sabit yuvalarda** (təsdiq / bloklanma / alt tapşırıqlar): eyni siqnal
  səhifə boyu bir xətt üzrə düzülür, göz sütunu aşağı süzür.
- 34 px sətirlər, status = rəngli kvadrat + mətn (pill yox).
- Panel **yan-yana** (split): açılanda cədvəl gizlənmir, sətri müəyyən edən sütunlara (açar, başlıq, siqnal, məbləğ)
  qısalır. Panelin içi iki sütundur: sol — başlıq, təsdiq zolağı, təsvir, alt tapşırıqlar, əlaqələr, fayllar,
  şərhlər/fəaliyyət; sağ **faktlar relsi** — məbləğ, status, cavabdeh, son tarix, fərdi sahələr, qeyd məlumatı.
- Telefon: iki sətirli siyahı (başlıq; açar + status + siqnallar + avatar; sağda məbləğ və son tarix), filtr
  tokenləri üfüqi sürüşür; panel tam ekran, faktlar başlığın altında 2×2 tor.
- Güclü tərəf: maksimum sıxlıq, Excel-ə öyrəşmiş istifadəçi üçün tanış; dəyişiklik mövcud strukturu saxlayır.
  Risk: siqnal yuvaları öyrənmə tələb edir (tooltip və legend lazımdır).

Fayllar: `a-desktop-light.png`, `a-desktop-light-drawer.png`, `a-desktop-dark.png`, `a-desktop-dark-drawer.png`,
`a-phone-light.png`, `a-phone-light-drawer.png`, `a-phone-dark.png`, `a-phone-dark-drawer.png`.

### B — "Queue" (növbə/triaj mərkəzli)

- Başlığın altında tünd-göy (brend 900) **növbə zolağı**: böyük rəqəmlər filtrdir — Hamısı 12, Mənə təyin 3,
  Gecikmiş 2, Təsdiq gözləyir 3, Bloklanmış 2, Bu həftə bitir 4. Səhər ilk sual "nə yanır?" bir baxışda cavablanır.
- İkinci sətir: görünüş seçicisi (dəyişibsə işarə), axtarış, "Filtrlər (2)" popover, qruplaşdırma, sıralama,
  sütunlar.
- Cədvəl **statusa görə qruplaşdırılıb**; qrup başlığında say və **ara cəm** (məbləğ). "Vəziyyət" sütununda mətnli
  pill-lər: "3 gün gecikib", "Təsdiq 1/2", "Bloklanıb".
- Panel üstə çıxan vərəq: başlıq, altında **iş axını zolağı** (Görüləcək → İcrada → Yoxlamada [təsdiq 1/2] →
  Tamamlandı), sonra tablar — Ümumi / Alt tapşırıqlar 1/3 / Əlaqələr 2 / Fayllar 2 / Şərhlər 4 / Tarixçə. "Ümumi"də
  sahələr 2 sütunlu cədvəldə (məbləğ böyük), təsdiq kartı təsdiq edənlərin avatarları ilə.
- Telefon: növbə zolağı üfüqi sürüşür, qruplu kart siyahısı, panel tam ekran, tablar sürüşür.
- Güclü tərəf: triaj və bağlanış dövrü üçün; ara cəmlər mühasibə birbaşa lazımdır. Risk: qruplaşdırma virtual
  siyahı ilə (TanStack Virtual) və səhifələmə ilə daha mürəkkəbdir (ara cəmlər serverdən gəlməlidir — ADR-0014
  cəmləri hazırda yalnız ümumi verir); növbə sayları əlavə sorğu tələb edir; tablı panel bir baxışda hər şeyi göstərmir.

Fayllar: `b-desktop-light.png`, `b-desktop-light-drawer.png`, `b-desktop-dark.png`, `b-desktop-dark-drawer.png`,
`b-phone-light.png`, `b-phone-light-drawer.png`, `b-phone-dark.png`, `b-phone-dark-drawer.png`.

### Seçim

İstifadəçi **A ("Ledger")** istiqamətini əsas götürdü, B-dən iki element əlavə olundu: tez filtr sayları zolağı və
paneldə iş axını addımları + təsdiq qutusu. Qruplu ara cəmlər (B) götürülmədi.

## Qərar: həyata keçirilən dizayn

| Qərar | Səbəb | Alternativ və niyə yox |
|---|---|---|
| Toolbar-ın üstündə **tez filtrlər zolağı**: Hamısı, Mənə təyin, Gecikmiş, Təsdiq gözləyir, Bloklanmış, Bu həftə bitir — hər biri say + düymə (`aria-pressed`); kliklə dörd ölçünü (icraçı, son tarix, təsdiq, bloklanma) birlikdə qoyur, axtarış/status/sahə filtrləri qalır | Back-office-in ilk sualı "nə yanır?" bir baxışda cavablanır; preset düymələri mövcud URL vəziyyətini (ADR-0014) yazır, ona görə link paylaşıla bilir | Hər preset üçün ayrıca select: köhnə problemi (çoxlu select) qaytarır |
| Saylar **bir sorğu** ilə: `GET …/items/queue-counts` → `{all, mine, overdue, approval, blocked, week}`. Server siyahının filtr qurucusunu (`itemFilter`) hər preset üçün işlədir və 6 `count(*)`-u **bir pgx batch**-də (bir round trip) göndərir | Mövcud endpoint-lərlə 6 paralel sorğu: hər səhifə açılışında və hər dəyişiklikdən sonra 6 sorğu, istifadəçi rate limit-ini yeyir. Eyni filtr kodu → saylar siyahı ilə həmişə uyğundur (test yoxlayır), gizlilik (ADR-0013) və "görünən bloklayıcı" qaydası eynidir | `count(*) FILTER (WHERE …)` ilə tək SQL: filtr qurucusunu iki dəfə yazmaq lazım gələrdi |
| `blocked=true` səhifə vəziyyətinə və linkə əlavə olundu (təsdiq filtri kimi saxlanmış görünüşə yazılmır) | "Bloklanmış" preset-i və çip üçün; API filtri (ADR-0028) artıq var idi | — |
| Toolbar **bir sətir**: görünüş seçicisi, axtarış, **"Filtrlər" menyusu** (icraçı, son tarix, təsdiq, bloklanmış, statuslar, sahələr), sağda sıralama, sütunlar, rejim tabları. Tətbiq olunmuş filtrlər **silinə bilən çiplər** kimi ayrıca nazik sətirdə (yalnız filtr varsa) | Əvvəl 5–6 select bir sırada sınırdı. Menyu + çip: nə tətbiq olunduğu görünür, bir kliklə silinir | Selectləri toolbar-da saxlamaq: 1024 px-də sınır |
| Cədvəl: siçan üçün 36 px sətir (toxunuşda 44 px qalır, ADR-0019), başlıqlar böyük hərfsiz, açar monospace deyil (`tabular-nums`), status = rəngli kvadrat + ad | Sıxlıq və oxunaqlıq; nişan-pill-lər sətirdə rəqabət aparmır | — |
| Sətrin solunda **vəziyyət zolağı** (qırmızı gecikmə, kəhrəba təsdiq, ştrixli bloklanma; ən təcili qalib gəlir), `sticky` — sətir yana sürüşəndə də görünür; `aria-hidden` (məlumat Siqnal sütununda da var) | Siyahını yuxarıdan aşağı süzəndə problemli sətirlər rəngdən tapılır | Sətir fonunu rəngləmək: seçilmiş/hover vəziyyətləri ilə qarışır |
| Nişanlar başlıqdan **"Siqnal" sütununa** keçdi (istifadəçi sütunu deyil, həmişə başlıqdan sonra): sabit yuvalar — təsdiq, bloklanma, alt tapşırıqlar; son tarix sütunu yoxdursa tarix də burada. Accessible adlar və `data-testid`-lər əvvəlki nişanlarla eynidir | Eyni siqnal hər sətirdə eyni yerdədir; başlıq yalnız başlıqdır | Başlıqda saxlamaq: problemin özü |
| Panel: lg-dən (≥1024 px) **cədvəlin yanında** (en `min(60rem, 64vw)`), kölgə pərdəsi yoxdur, `aria-modal` yoxdur — başqa sətrə klik paneli dəyişir; cədvəl panelin enini boşaldır. lg-dən aşağı əvvəlki kimi modal; telefonda tam ekran | Maliyyəçi siyahı ilə detal arasında tez-tez keçir; table görünən qalır | Ayrı səhifə: siyahı konteksti itir |
| Panelin içi: başlıq + **iş axını addımları** (workflow statusları sırası ilə, hazırkı `aria-current="step"`, açıq təsdiq varsa zolaq ştrixli və altında "Təsdiq 1/2, sonra: …"), sonra **iki sütun** — solda məzmun (təsdiq, təsvir, alt tapşırıqlar, əlaqələr, tələb, fayllar, şərhlər), sağda **xüsusiyyətlər** (status, icraçı, son tarix, müəllif, sahələr). Sütunlar container query ilə (`@2xl`, ≈672 px panel eni); dar paneldə və telefonda bir sütun, xüsusiyyətlər əvvəl | Məzmun oxunur, faktlar bir yerdə; tablet paneli dar olduğu üçün media query yox, container query | Tablar (B): bir baxışda hər şeyi göstərmir |
| Addımlar "mövqedir", "tamamlanıb" deyil | İş axını istənilən keçidə icazə verə bilər | — |
| Təsdiq qutusu: açıq sorğuda **kim təsdiqlədi** (yaşıl, ✓) və **kim gözlənilir** (server-in `approvers` siyahısı) avatarlarla. `Approvals.tsx`-də yalnız bir sətir dəyişdi (`ApproverRoster` komponenti) | approvals-plus branch-i həmin faylı dəyişir — konflikt minimal | — |
| Sətirlər yeni `ledger` modulundadır (`locales/<dil>/ledger.ts`, `i18n-ledger.ts`, az/en/tr/ru) | ADR-0025 | — |

**E2E:** `due`, `views`, `approvals` spec-ləri toolbar selectlərini birbaşa seçirdi — indi əvvəlcə "Filtrlər" menyusunu
açırlar (etiketlər eynidir). Yeni `e2e/ledger.spec.ts`: saylar, preset, çip, Siqnal sütunu, panel yan-yana.

## Miqrasiya

00028 (yuxarıda). Round 7 nömrələri: A (ADR-0033) = 00026 (istifadə olunmaya bilər), B (ADR-0034) = 00027,
C (bu ADR) = 00028.

## Nəticələr

- Rəng rejimi cihaza bağlıdır; istifadəçi başqa brauzerdə yenidən seçməlidir.
- `theme-init.js` sinxron, keşlənən (Caddy `no-cache` + ETag ilə yenidən yoxlanır) kiçik bir sorğudur.
- Vurğu rəngi yalnız brend tokenlərini dəyişir; status, gecikmə və təsdiq rəngləri sabitdir.
- Workspace siyahısı, qəbul formaları (`/f/...`) və "Tələblərim" Nebula firuzəyisində qalır (workspace kontekstindən
  kənardır).

## Açıq suallar

- Qruplu ara cəmlər (B) — serverdə qrup üzrə `sums` lazımdır; tələb olsa ayrıca iş.
- Panel açıqkən toolbar-ın sağ hissəsi panelin altında qalır (sıralama/sütunlar üçün panel bağlanmalıdır).
- Şrift: `--font-sans` "Inter"-i göstərir, amma şrift yüklənmir (sistem şrifti işləyir). Rəqəmlər üçün self-host
  edilmiş şrift (məs. Inter və ya IBM Plex Sans, `tabular-nums` + `slashed-zero`) ayrıca qərar ola bilər — CSP
  `font-src 'self'` ilə uyğundur.
