# ADR-0037: Ana səhifə "Mənim işlərim"

- Status: Qəbul edilib
- Tarix: 2026-11-24
- Əsas: [ADR-0013](0013-project-privacy.md) (gizli layihələr), [ADR-0034](0034-approval-inbox-reapproval-deadlines.md)
  (təsdiq qutusu), [ADR-0028](0028-item-links-and-watchers.md) (izləyicilər), [ADR-0035](0035-design-and-theme.md) (Ledger dizaynı)

## Kontekst

Girişdən sonra `/` yalnız workspace siyahısını göstərirdi. İstifadəçilərin çoxu bir workspace-dədir: siyahıda bir
sətir — boş görünən, heç nə deməyən səhifə. Maliyyə/bek-ofis işçisi səhər bir sualla gəlir: **"bu gün məndən nə
gözlənilir?"** Cavab bir neçə yerə səpələnib: hər layihənin "Mənə təyin" və "Gecikmiş" filtrləri, hər workspace-in
təsdiq qutusu (ADR-0034), bildirişlər (ADR-0017), "Tələblərim" (ADR-0032). FinTez pilotu başlamazdan əvvəl `/`
bu sualın cavabı olmalıdır.

## Qərarlar

| Qərar | Səbəb | Alternativ və niyə yox |
|---|---|---|
| **Bir endpoint**: `GET /api/v1/me/home?show=attention\|overdue\|approval\|week\|assigned&limit=1..200` (standart `attention`, 15) — sayğaclar, bir siyahı (`items`, `total`), workspace kartları, izlədiyim işlər, son layihələr **bir cavabda** | Səhifə bir sorğu ilə açılır; sayğac, siyahı və kart eyni anın rəqəmləridir | Hər workspace üçün ayrıca qutu/say sorğusu: N workspace = N×4 sorğu, və gizli layihəni süzmək UI-yə qalardı |
| Server tərəfdə **bir pgx batch** (4 SQL, bir şəbəkə gedişi), hamısı eyni iki CTE-dən başlayır: `my` (üzvlüklər + workspace-in yerli tarixi) və `vis` (baxanın görə bildiyi aktiv layihələr, `visibleProjectFor` = `canSeeProject`) | Workspace sayından asılı olmayan, set əsaslı SQL — N+1 yoxdur; gizlilik qaydası bir yerdədir, hər sorğu ondan keçir | Bir böyük JSON sorğusu: oxunması və `EXPLAIN`-i çətin; Go-da workspace üzrə dövr: N+1 |
| "Təsdiqimi gözləyən" — qutunun `inboxCanDecide` predikatı (içində `approverEligible`, yəni **qərarın özünü yoxlayan qayda**: aktiv, rol ≥ member, snapshot-dakı rol/istifadəçi, layihəni görür, sorğunu göndərən deyil, hələ səs verməyib) | Sayğac qutu ilə eyni rəqəmi göstərir; qonaq, sorğu göndərən və artıq səs vermiş heç vaxt sayılmır (testlər) | Ayrıca sadələşdirilmiş şərt: iki qayda, fərqlənmə riski |
| **Sayğaclar** (hamısı mənə təyin olunmuş, açıq = `todo`/`in_progress`, görünən layihədə, silinməmiş): **gecikən** = `due_date < yerli bu gün`; **bu həftə** = `due_date` bu gündən 6 gün sonraya qədər (layihə siyahısının `due=week` filtri ilə eyni pəncərə); **təyin olunan** = hamısı. "Bu gün" hər workspace-in **öz saat qurşağında** | Workspace-lər fərqli qurşaqlarda ola bilər; eyni tarix bir workspace-də gecikmiş, digərində bu gün ola bilər (test: Kiritimati UTC+14 / Pago Pago UTC−11) | Brauzerin tarixi: Bakıdakı istifadəçi üçün başqa qurşaqdakı workspace-in son tarixi səhv sayılardı |
| Son tarix **olduğu kimi** göstərilir (tətbiqin hər yerində olduğu kimi, `formatDate`); qeyri-iş günü mənanı dəyişmir | ADR-0027 son tarixi sürüşdürmür (yalnız təkrarlanan qaydalar və təsdiq müddəti iş günü ilə hesablanır); ana səhifə başqa tarix göstərsə, panel ilə uyğunsuz olardı | Bayrama düşən son tarixi növbəti iş gününə "çəkmək": tapşırığın öz tarixi ilə ziddiyyət |
| **"Diqqət tələb edir"** siyahısı: bir tapşırıq bir sətir, ən təcili səbəbi ilə; sıra **gecikən (ən köhnə son tarix əvvəl) → təsdiqimi gözləyən (ən köhnə sorğu əvvəl) → bu həftə (ən yaxın son tarix əvvəl)**, workspace-lər qarışıq; 15 sətir və "Hamısını göstər (N)" | Gecikmiş iş artıq zərərdir, təsdiq başqasını saxlayır, həftəlik iş planlaşdırmadır. Eyni tapşırıq həm gecikmiş, həm təsdiqimi gözləyirsə — bir sətir, "Gecikib" nişanı | Workspace-lərə görə bölmələr: tək workspace-də artıq, çoxunda ən təcili iş aşağıda qala bilər |
| Sətir: açar, başlıq, səbəb nişanı, layihə · workspace (vurğu rəngi nöqtəsi), son tarix və ya "Gözləyir: N gün əvvəl" (təsdiq son günü ilə); klik `/w/:ws/p/:key?item=n` — təsdiq sətrində `&focus=approval` (ADR-0031 dərin linki) | Sıx, oxunaqlı Ledger sətri (ADR-0035); panel tapşırığın öz layihəsində açılır, yeni səhifə lazım deyil | Ana səhifədə öz paneli: ikinci panel kodu |
| **Sayğac linkləri**: bir workspace varsa "Təsdiqimi gözləyən" həmin workspace-in qutusunu (`/w/:ws/approvals`) açır — eyni sorğular, eyni qayda. Digər sayğaclar (və çox workspace-də hamısı) ana səhifənin **öz siyahısını** açır: `/?show=overdue` və s., ən çox 200 sətir | Gecikən/həftəlik/təyin olunan işlər layihələr arasındadır; workspace daxilində layihələrarası tapşırıq siyahısı yoxdur, ana səhifədəki siyahı isə sayğacla **dəqiq** eyni sətirlərdir | Bir layihənin `?assignee=me&due=week` filtri: başqa layihələrdəki işləri itirir, `week` filtri bitmiş işləri də göstərir — rəqəm uyğun gəlməzdi |
| **Yönləndirmə yoxdur**: tək workspace-də, hətta bütün sayğaclar sıfır olanda belə `/` ana səhifədir; workspace bir klik uzaqdadır (kart, sakit vəziyyətdə "Son layihələr") | İstifadəçi "boş olmasın" istədi: sakit vəziyyət də məlumatdır ("heç nə gözləmir"), bildirişlər və tələblər yenə buradadır. Səhifə məzmuna görə gah ana səhifə, gah layihə siyahısı olsa, istifadəçi `/`-un harada açılacağını bilməz; "Nebula" loqosu həmişə eyni yerə aparmalıdır. Bunun üçün gözləyən dəvətləri yoxlamaq da lazım olmur | Sıfır sayğacda layihə siyahısına yönləndirmə: gözlənilməz davranış, geri düyməsi döngəsi, iki workspace-ə keçəndə birdən dəyişən giriş nöqtəsi |
| **Yan panellər**: son bildirişlər (mövcud `GET /me/notifications`, zəng ilə eyni keş, ilk 5); "Tələblərim" (`GET /me/requests?limit=5`) — **yalnız tələb varsa**; "İzlədiyim işlər" (`/me/home`-un `watching`-i, son dəyişənlər, 5) | Mövcud API-lər təkrar istifadə olunur; bu panellər üçün `/me/home`-u şişirtmək lazım deyil | Hamısını `/me/home`-a yığmaq: bildiriş məntiqi iki yerdə |
| İzlədiyim işlərdən **mənə təyin olunanlar çıxarılır** | İcraçı tapşırığı avtomatik izləyir (ADR-0028); onlar sayğaclarda və siyahıda artıq var, panel təkrar olardı | — |
| **Workspace kartları**: vurğu rəngi zolağı (ADR-0035), ad, slug, görünən layihə sayı, rol, mənim açıq / gecikən / təsdiq saylarım, son dəyişiklik (görünən layihələrin son tapşırıq dəyişikliyi). Mənə iş gözləyən workspace-lər əvvəl, sonra ada görə. "Yeni workspace" başlıqda qalır (şablon və komanda növü ilə, ADR-0020/0033) | Kart workspace-i bir baxışda tanıdır; "son dəyişiklik" gizli layihənin fəaliyyətini sızdırmır (yalnız `vis`-dən) | Workspace-in bütün layihələrinin son fəaliyyəti: gizli layihədə iş getdiyini göstərərdi (ADR-0013) |
| "Son fəaliyyət" və "Son layihələr" hər workspace üçün **son 50 canlı tapşırıqdan** (görünən layihələrdə) hesablanır: `items_workspace_recent_idx` üzrə LATERAL indeks skanı | Ölçüdən asılı olmayan xərc; 50 tapşırıqda dəyişiklik olmayan layihə "son" deyil | Hər layihə üçün `max(updated_at)`: layihə sayı qədər skan |
| **Boş vəziyyətlər**: workspace yoxdursa — "Dəvət olunmusunuzsa, keçid e-poçtunuzdadır, yoxsa adminizdən xahiş edin" və "Şablonla workspace yarat" (workspace yaratmaq hamıya açıqdır, ADR-0002); workspace var, amma diqqət tələb edən yoxdursa — "Hazırda diqqətinizi tələb edən iş yoxdur", son layihələr və ⌘K ipucu | Yeni istifadəçi nə edəcəyini bilir; sakit gün də müsbət xəbərdir | ⌘K düyməsini ana səhifəyə gətirmək: axtarış workspace daxilindədir (ADR-0024), ana səhifədə hansı workspace-də axtaracağı bəlli deyil — ipucu ilə kifayətləndik |
| **Miqrasiya yoxdur**. Ölçmə (`TestHomeLargeDataset`, `NEBULA_HOME_BENCH=1`): 5 workspace × 20 000 tapşırıq (hər birində 1 gizli layihə), ağır istifadəçi — 5 265 təyin olunmuş, 750 təsdiq, 1 500 izlənən: bütün batch **p50 20 ms, p95 22 ms**; yüngül istifadəçi p95 7 ms. Planlar: `items_assignee_idx`, `approval_requests_inbox_idx`/`items_pkey`, `items_workspace_recent_idx`; heç bir sorğu workspace sayı ilə xətti artmır | Mövcud indekslər kifayətdir; yeni indeks yazma xərcidir | `(assignee_id, due_date)` indeksi: ölçmədə qazanc yoxdur |
| Rate limit: `/me/*` kimi, API router-in istifadəçi başına limiti; cavab `Cache-Control: no-store` (qlobal) | Səhifə hər açılışda bir sorğu göndərir, ayrıca limitə ehtiyac yoxdur | Qısa keş: şəxsi məlumat, sayğacların təzə olması vacibdir |
| UI (ADR-0025, az/en/tr/ru, yeni lüğət modulu `home`, fasad `i18n-home.ts`, en/ru üçün `plural()`): "Salam, <ad>" və bugünkü tarix UI dilində (az əl ilə, digərləri `Intl`). Telefon (ADR-0019): sayğaclar öz zolağında sağa-sola sürüşür, siyahı bir sütun, yan panellər aşağıda; dil və mövzu seçiciləri ana səhifənin altında qalır (başlıqda yer yoxdur), başlığa yeni element əlavə edilmədi | 375 px və 1024 px-də üfüqi sürüşmə yoxdur; köhnə siyahının əlçatan adları ("Yeni workspace", "Dil", mövzu) qorunur | — |

## Nəticələr

- `/` artıq `WorkspacesPage` deyil, `HomePage`-dir; workspace yaratma forması `components/CreateWorkspace.tsx`-ə köçdü
  (dəyişmədən). Breadcrumb-lardakı "Workspace-lər → /" linki ana səhifəyə aparır, orada "Workspace-lər" bölməsi var.
- Ana səhifənin siyahıları ən çox 200 sətirdir (`truncated` qeydi ilə); daha böyük həcm üçün layihə siyahısı və filtrlər qalır.
- İzlənən işlər paneli icraçı olduğum işləri göstərmir; təsdiqçinin izlədiyi işlər görünür.
- Testlər: `internal/server/home_integration_test.go` (gizli layihə, qonaq, çox workspace sırası, təsdiq uyğunluğu,
  saat qurşağı sərhədləri, boş istifadəçi), `web/src/lib/home.test.ts`, `web/src/pages/HomePage.test.tsx`,
  E2E `web/e2e/home.spec.ts`; `auth.spec.ts`, `auth.setup.ts`, `mobile.spec.ts`, `theme.spec.ts` yeniləndi.
- Rollback: miqrasiya yoxdur, yalnız image-i qaytarmaq kifayətdir.

## Açıq suallar

- Workspace daxilində layihələrarası "Mənim işlərim" siyahısı (indi yalnız ana səhifədə) — çox layihəli komandalar
  istəsə `/w/:ws/mine` kimi.
- ⌘K-nı ana səhifədə bütün workspace-lər üzrə axtarışa çevirmək (ADR-0024 workspace sərhədini dəyişir).
- Gözləyən dəvətləri ana səhifədə göstərmək (indi yalnız email linki; dəvət e-poçt ünvanına bağlıdır, ADR-0016).
- "Bu həftə" pəncərəsini iş günləri ilə saymaq (indi 7 təqvim günü, layihə filtri ilə eyni).
